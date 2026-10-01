package repo

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/storage/memory"
)

// Tests replace these.
var (
	// archiveBase is where tarballs come from without a token:
	// <archiveBase>/<owner>/<repo>/archive/<sha>.tar.gz. GitHub redirects these to codeload.github.com.
	archiveBase = "https://" + githubHost
	httpClient  = http.DefaultClient
	listRefs    = lsRemote
	// cacheDir returns the archive cache directory, which holds <sha>.tar.gz files.
	cacheDir = userCacheDir
)

// userCacheDir is skill-atlas/archives in the user's cache directory, e.g. ~/.cache on Linux.
func userCacheDir() (string, error) {
	d, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "skill-atlas", "archives"), nil
}

// ErrEmptyRepository is wrapped by the Download error for a repository without commits.
var ErrEmptyRepository = errors.New("empty")

// Download resolves ref with ls-remote and extracts the tarball of that commit into dir.
// An empty ref means the remote's default branch; otherwise ref is a branch or tag name.
// It never prompts. If GITHUB_TOKEN or GH_TOKEN is set, the token goes to GitHub hosts only, so
// private repositories the token can read work.
//
// Tarballs are kept between runs in the archive cache, keyed by commit SHA. A cached commit is
// unpacked without a request to the archive host. When the cache can't be used, Download still
// streams the tarball and calls warn, if it isn't nil, with the reason.
func Download(ctx context.Context, t Target, ref, dir string, warn func(error)) (Checkout, error) {
	co, err := resolve(ctx, t, ref)
	if err != nil {
		return Checkout{}, err
	}
	if warn == nil {
		warn = func(error) {}
	}
	cache, err := openCache()
	if err != nil {
		warn(err)
	}
	if cache != "" {
		hit, err := unpackCached(ctx, filepath.Join(cache, co.SHA+".tar.gz"), co.SHA, dir)
		if err != nil {
			return Checkout{}, fmt.Errorf("download %s: %w", t.Display, err)
		}
		if hit {
			return co, nil
		}
	}
	if err := fetchArchive(ctx, t, co.SHA, dir, cache, warn); err != nil {
		return Checkout{}, err
	}
	return co, nil
}

func lsRemote(ctx context.Context, url string) ([]*plumbing.Reference, error) {
	rem := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{Name: "origin", URLs: []string{url}})
	return rem.ListContext(ctx, listOptions(url))
}

// resolve finds the short ref name and the commit SHA to download.
func resolve(ctx context.Context, t Target, ref string) (Checkout, error) {
	refs, err := listRefs(ctx, t.Remote)
	if err != nil {
		if errors.Is(err, transport.ErrEmptyRemoteRepository) {
			return Checkout{}, fmt.Errorf("repository %s is %w", t.Display, ErrEmptyRepository)
		}
		return Checkout{}, mapError(ctx, t, err)
	}
	if len(refs) == 0 {
		return Checkout{}, fmt.Errorf("repository %s is %w", t.Display, ErrEmptyRepository)
	}
	// Peeled entries ("refs/tags/x^{}") hold the commit of an annotated tag.
	byName := make(map[plumbing.ReferenceName]*plumbing.Reference, len(refs))
	peeled := map[plumbing.ReferenceName]*plumbing.Reference{}
	for _, r := range refs {
		if base, ok := strings.CutSuffix(r.Name().String(), "^{}"); ok {
			peeled[plumbing.ReferenceName(base)] = r
			continue
		}
		byName[r.Name()] = r
	}

	if ref == "" {
		head := byName[plumbing.HEAD]
		if head == nil || head.Type() != plumbing.SymbolicReference || !head.Target().IsBranch() {
			return Checkout{}, fmt.Errorf("can't determine the default branch of %s", t.Display)
		}
		branch := byName[head.Target()]
		if branch == nil || branch.Type() != plumbing.HashReference {
			return Checkout{}, fmt.Errorf("can't determine the default branch of %s", t.Display)
		}
		return Checkout{Ref: head.Target().Short(), SHA: branch.Hash().String()}, nil
	}

	if r := byName[plumbing.NewBranchReferenceName(ref)]; r != nil && r.Type() == plumbing.HashReference {
		return Checkout{Ref: ref, SHA: r.Hash().String()}, nil
	}
	tag := plumbing.NewTagReferenceName(ref)
	if r := byName[tag]; r != nil && r.Type() == plumbing.HashReference {
		// An annotated tag points at a tag object; the peeled entry has the commit.
		if p := peeled[tag]; p != nil && p.Type() == plumbing.HashReference {
			return Checkout{Ref: ref, SHA: p.Hash().String()}, nil
		}
		return Checkout{Ref: ref, SHA: r.Hash().String()}, nil
	}
	return Checkout{}, fmt.Errorf("ref %q not found in %s", ref, t.Display)
}

// openCache creates the archive cache directory and returns its path.
func openCache() (string, error) {
	d, err := cacheDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(d, 0o700); err != nil {
		return "", err
	}
	return d, nil
}

// unpackCached extracts the cached tarball at file into dir and reports whether it was there.
// A cached tarball that fails to extract is deleted and dir is cleared, so the caller downloads
// the commit again. The error is non-nil only when ctx is cancelled or dir can't be cleared.
func unpackCached(ctx context.Context, file, sha, dir string) (bool, error) {
	f, err := os.Open(file)
	if err != nil {
		return false, nil // not cached; a file that can't be read is replaced by the download
	}
	defer f.Close()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	err = extract(ctxReader{ctx, f}, dir, sha)
	if err == nil {
		now := time.Now()
		os.Chtimes(file, now, now) // the last use, for a future eviction
		return true, nil
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	os.Remove(file)
	return false, os.RemoveAll(dir)
}

// ctxReader stops reading once ctx is cancelled.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

// fetchArchive streams the tarball of commit sha into dir. Unless cache is "", the tarball is
// also written to the cache once it has been extracted in full; a failure there goes to warn.
//
// With a token the tarball comes from the REST API, the only route that takes a token for a private
// repository. Without one it comes from the github.com web route, which doesn't count against the
// API rate limit. The token is never sent to the web route.
func fetchArchive(ctx context.Context, t Target, sha, dir, cache string, warn func(error)) error {
	req, err := archiveRequest(ctx, t, sha)
	if err != nil {
		return fmt.Errorf("download %s: %w", t.Display, err)
	}
	resp, err := do(req)
	if err != nil {
		return mapError(ctx, t, err)
	}
	defer resp.Body.Close()

	sent, name := sentToken(req)
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("repository %s not found", t.Display)
	case resp.StatusCode == http.StatusUnauthorized && sent:
		return errBadToken(name)
	case rateLimited(resp):
		return rateLimitError(resp.Header.Get("X-RateLimit-Reset"), sent)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("download %s: %s", t.Display, resp.Status)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("download %s: %w", t.Display, err)
	}
	var body io.Reader = resp.Body
	var keep *cacheFile
	if cache != "" {
		if keep, err = newCacheFile(cache, sha); err != nil {
			warn(err)
		} else {
			defer keep.discard()
			body = io.TeeReader(resp.Body, keep)
		}
	}
	if err := extract(body, dir, sha); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("download %s: %w", t.Display, ctx.Err())
		}
		return fmt.Errorf("download %s: %w", t.Display, err)
	}
	if keep != nil {
		if err := keep.commit(); err != nil {
			warn(err)
		}
	}
	return nil
}

// archiveRequest builds the request for the tarball of commit sha. With a token it is
// GET <apiBase>/repos/<owner>/<repo>/tarball/<sha>, which answers 302 to codeload.github.com with
// a short-lived token in the URL. Without one it is <archiveBase>/<owner>/<repo>/archive/<sha>.tar.gz.
func archiveRequest(ctx context.Context, t Target, sha string) (*http.Request, error) {
	if tok, _ := token(); tok != "" {
		return apiRequest(ctx, apiBase+"/repos/"+t.Owner+"/"+t.Name+"/tarball/"+sha)
	}
	url := archiveBase + "/" + t.Owner + "/" + t.Name + "/archive/" + sha + ".tar.gz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	return req, nil
}

// cacheFile is a tarball on its way into the cache. It is written to a temporary file next to
// its final name and renamed once complete, so readers never see a partial tarball.
// A failed write doesn't fail the download; commit returns the error instead.
type cacheFile struct {
	f    *os.File // nil after commit or discard
	name string   // <cache>/<sha>.tar.gz
	err  error    // the first write error
}

func newCacheFile(cache, sha string) (*cacheFile, error) {
	f, err := os.CreateTemp(cache, sha+"-*.tmp") // mode 0600
	if err != nil {
		return nil, err
	}
	return &cacheFile{f: f, name: filepath.Join(cache, sha+".tar.gz")}, nil
}

// Write always succeeds, so a full disk in the cache doesn't stop the extraction.
func (c *cacheFile) Write(p []byte) (int, error) {
	if c.err == nil {
		_, c.err = c.f.Write(p)
	}
	return len(p), nil
}

// commit moves the complete tarball to its final name. On error the temporary file is removed.
func (c *cacheFile) commit() error {
	f := c.f
	c.f = nil
	err := c.err
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), c.name)
	}
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}

// discard removes the temporary file unless commit ran.
func (c *cacheFile) discard() {
	if c.f != nil {
		c.f.Close()
		os.Remove(c.f.Name())
		c.f = nil
	}
}

// extract writes the regular files and directories of a .tar.gz stream into dir, without the
// archive's single top-level directory. Symlinks, hard links and other entry types are skipped.
// A path that is absolute or leaves dir is an error, and so is a pax global comment other than sha.
func extract(r io.Reader, dir, sha string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	top := ""
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag == tar.TypeXGlobalHeader {
			// git archive stores the commit SHA here.
			if c, ok := hdr.PAXRecords["comment"]; ok && c != sha {
				return fmt.Errorf("archive is of commit %s, want %s", c, sha)
			}
			continue
		}

		first, rest, err := entryPath(hdr.Name)
		if err != nil {
			return err
		}
		if top == "" {
			top = first
		} else if first != top {
			return fmt.Errorf("archive has more than 1 top-level directory: %q and %q", top, first)
		}
		if rest == "" {
			continue // the top-level directory itself
		}
		target := filepath.Join(dir, filepath.FromSlash(rest))

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := writeFile(target, tr, hdr.Mode); err != nil {
				return err
			}
		}
	}
	// Read to the end so gzip checks its checksum.
	_, err = io.Copy(io.Discard, gz)
	return err
}

// entryPath splits a tar entry name into its top-level directory and the rest.
func entryPath(name string) (first, rest string, err error) {
	if path.IsAbs(name) || filepath.IsAbs(name) || filepath.VolumeName(name) != "" {
		return "", "", fmt.Errorf("archive entry %q has an absolute path", name)
	}
	// Split before cleaning: "top/../x" would leave dir once top is stripped.
	first, rest, _ = strings.Cut(name, "/")
	if rest != "" {
		if rest = path.Clean(rest); rest == "." {
			rest = ""
		}
	}
	if first == "" || first == "." || first == ".." || (rest != "" && !filepath.IsLocal(filepath.FromSlash(rest))) {
		return "", "", fmt.Errorf("archive entry %q is outside the archive", name)
	}
	return first, rest, nil
}

func writeFile(target string, r io.Reader, mode int64) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	perm := os.FileMode(0o644)
	if mode&0o111 != 0 {
		perm = 0o755
	}
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func mapError(ctx context.Context, t Target, err error) error {
	var netErr net.Error
	switch {
	case ctx.Err() != nil:
		return fmt.Errorf("download %s: %w", t.Display, ctx.Err())
	case errors.Is(err, transport.ErrAuthenticationRequired) && sentListToken(t):
		_, name := token()
		return errBadToken(name)
	case errors.Is(err, transport.ErrAuthenticationRequired), errors.Is(err, transport.ErrAuthorizationFailed):
		return fmt.Errorf("authentication failed for %s: the repository may be private or may not exist", t.Display)
	case errors.Is(err, transport.ErrRepositoryNotFound):
		return fmt.Errorf("repository %s not found", t.Display)
	case errors.As(err, &netErr):
		return fmt.Errorf("can't reach %s: %w", githubHost, err)
	}
	return fmt.Errorf("download %s: %w", t.Display, err)
}
