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

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/storage/memory"
)

// Tests replace these.
var (
	// archiveBase is where tarballs come from: <archiveBase>/<owner>/<repo>/archive/<sha>.tar.gz.
	// GitHub redirects these to codeload.github.com.
	archiveBase = "https://" + githubHost
	httpClient  = http.DefaultClient
	listRefs    = lsRemote
)

// Download resolves ref with ls-remote and extracts the tarball of that commit into dir.
// An empty ref means the remote's default branch; otherwise ref is a branch or tag name.
// It never prompts and sends no credentials.
func Download(ctx context.Context, t Target, ref, dir string) (Checkout, error) {
	co, err := resolve(ctx, t, ref)
	if err != nil {
		return Checkout{}, err
	}
	if err := fetchArchive(ctx, t, co.SHA, dir); err != nil {
		return Checkout{}, err
	}
	return co, nil
}

func lsRemote(ctx context.Context, url string) ([]*plumbing.Reference, error) {
	rem := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{Name: "origin", URLs: []string{url}})
	return rem.ListContext(ctx, &git.ListOptions{PeelingOption: git.AppendPeeled})
}

// resolve finds the short ref name and the commit SHA to download.
func resolve(ctx context.Context, t Target, ref string) (Checkout, error) {
	refs, err := listRefs(ctx, t.Remote)
	if err != nil {
		if errors.Is(err, transport.ErrEmptyRemoteRepository) {
			return Checkout{}, fmt.Errorf("repository %s is empty", t.Display)
		}
		return Checkout{}, mapError(ctx, t, err)
	}
	if len(refs) == 0 {
		return Checkout{}, fmt.Errorf("repository %s is empty", t.Display)
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

// fetchArchive streams the tarball of commit sha into dir.
func fetchArchive(ctx context.Context, t Target, sha, dir string) error {
	url := archiveBase + "/" + t.Owner + "/" + t.Name + "/archive/" + sha + ".tar.gz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("download %s: %w", t.Display, err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return mapError(ctx, t, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("repository %s not found", t.Display)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("download %s: %s", t.Display, resp.Status)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("download %s: %w", t.Display, err)
	}
	if err := extract(resp.Body, dir, sha); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("download %s: %w", t.Display, ctx.Err())
		}
		return fmt.Errorf("download %s: %w", t.Display, err)
	}
	return nil
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
	case errors.Is(err, transport.ErrAuthenticationRequired), errors.Is(err, transport.ErrAuthorizationFailed):
		return fmt.Errorf("authentication failed for %s: the repository may be private or may not exist", t.Display)
	case errors.Is(err, transport.ErrRepositoryNotFound):
		return fmt.Errorf("repository %s not found", t.Display)
	case errors.As(err, &netErr):
		return fmt.Errorf("can't reach %s: %w", githubHost, err)
	}
	return fmt.Errorf("download %s: %w", t.Display, err)
}
