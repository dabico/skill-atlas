package repo

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

const (
	testSHA    = "0123456789abcdef0123456789abcdef01234567"
	tagObject  = "1111111111111111111111111111111111111111"
	tagCommit  = "2222222222222222222222222222222222222222"
	otherSHA   = "3333333333333333333333333333333333333333"
	testPrefix = "repo-" + testSHA + "/"
)

func testTarget(t *testing.T) Target {
	t.Helper()
	tg, err := ParseURL("https://github.com/org/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	return tg
}

// testRefs is what ls-remote returns for org/repo: HEAD -> main, branch "v1" and tag "v1",
// an annotated tag "v2" and a lightweight tag "v3".
func testRefs() []*plumbing.Reference {
	return []*plumbing.Reference{
		plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName("main")),
		plumbing.NewHashReference(plumbing.NewBranchReferenceName("main"), plumbing.NewHash(testSHA)),
		plumbing.NewHashReference(plumbing.NewBranchReferenceName("v1"), plumbing.NewHash(otherSHA)),
		plumbing.NewHashReference(plumbing.NewTagReferenceName("v1"), plumbing.NewHash(tagObject)),
		plumbing.NewHashReference(plumbing.NewTagReferenceName("v2"), plumbing.NewHash(tagObject)),
		plumbing.NewReferenceFromStrings("refs/tags/v2^{}", tagCommit),
		plumbing.NewHashReference(plumbing.NewTagReferenceName("v3"), plumbing.NewHash(otherSHA)),
	}
}

// useCache points the archive cache at a new empty directory and returns it.
func useCache(t *testing.T) string {
	t.Helper()
	old := cacheDir
	t.Cleanup(func() { cacheDir = old })
	dir := filepath.Join(t.TempDir(), "skill-atlas", "archives")
	cacheDir = func() (string, error) { return dir, nil }
	return dir
}

// fakeRemote replaces ls-remote with refs (or err) and the archive host with handler.
// The archive cache starts empty.
func fakeRemote(t *testing.T, refs []*plumbing.Reference, err error, handler http.HandlerFunc) {
	t.Helper()
	useCache(t)
	oldList, oldBase, oldClient := listRefs, archiveBase, httpClient
	t.Cleanup(func() { listRefs, archiveBase, httpClient = oldList, oldBase, oldClient })
	listRefs = func(context.Context, string) ([]*plumbing.Reference, error) { return refs, err }
	if handler != nil {
		srv := httptest.NewServer(handler)
		t.Cleanup(srv.Close)
		archiveBase, httpClient = srv.URL, srv.Client()
	}
}

type entry struct {
	name     string
	typeflag byte
	body     string
	link     string
	mode     int64
}

// tarGz builds a .tar.gz with an optional pax global comment, like git archive writes.
func tarGz(t *testing.T, comment string, entries ...entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if comment != "" {
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": comment}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, e := range entries {
		mode := e.mode
		if mode == 0 {
			mode = 0o644
		}
		h := &tar.Header{Name: e.name, Typeflag: e.typeflag, Linkname: e.link, Mode: mode, Size: int64(len(e.body)), Format: tar.FormatPAX}
		if e.typeflag != tar.TypeReg {
			h.Size = 0
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// serve answers every request with data and records the request paths.
func serve(data []byte, paths *[]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if paths != nil {
			*paths = append(*paths, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/x-gzip")
		w.Write(data)
	}
}

func dirTree(t *testing.T, root string) []string {
	t.Helper()
	var got []string
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			rel += "/"
		}
		got = append(got, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestDownloadExtracts(t *testing.T) {
	data := tarGz(t, testSHA,
		entry{name: testPrefix, typeflag: tar.TypeDir},
		entry{name: testPrefix + "README.md", typeflag: tar.TypeReg, body: "hi"},
		entry{name: testPrefix + "skills/", typeflag: tar.TypeDir},
		entry{name: testPrefix + "skills/a/SKILL.md", typeflag: tar.TypeReg, body: "---\nname: a\n---\n"},
		entry{name: testPrefix + "bin/run.sh", typeflag: tar.TypeReg, body: "#!/bin/sh\n", mode: 0o755},
		entry{name: testPrefix + "empty/", typeflag: tar.TypeDir},
		entry{name: testPrefix + "link", typeflag: tar.TypeSymlink, link: "/etc/passwd"},
		entry{name: testPrefix + "skills/b", typeflag: tar.TypeSymlink, link: "a"},
		entry{name: testPrefix + "hard", typeflag: tar.TypeLink, link: testPrefix + "README.md"},
		entry{name: testPrefix + "fifo", typeflag: tar.TypeFifo},
	)
	var paths []string
	fakeRemote(t, testRefs(), nil, serve(data, &paths))

	dir := filepath.Join(t.TempDir(), "repo-0") // Download creates it
	co, err := Download(context.Background(), testTarget(t), "", dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if co != (Checkout{Ref: "main", SHA: testSHA}) {
		t.Errorf("checkout = %+v, want main @ %s", co, testSHA)
	}
	if want := "/org/repo/archive/" + testSHA + ".tar.gz"; len(paths) != 1 || paths[0] != want {
		t.Errorf("requests = %v, want [%s]", paths, want)
	}

	want := []string{"README.md", "bin/", "bin/run.sh", "empty/", "skills/", "skills/a/", "skills/a/SKILL.md"}
	if got := dirTree(t, dir); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("tree = %v, want %v", got, want)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "skills", "a", "SKILL.md")); err != nil || string(b) != "---\nname: a\n---\n" {
		t.Errorf("SKILL.md = %q, %v", b, err)
	}
	if fi, err := os.Stat(filepath.Join(dir, "bin", "run.sh")); err != nil || fi.Mode().Perm()&0o100 == 0 {
		t.Errorf("run.sh lost its executable bit: %v, %v", fi, err)
	}
}

func TestDownloadWithoutComment(t *testing.T) {
	data := tarGz(t, "", entry{name: testPrefix + "a.txt", typeflag: tar.TypeReg, body: "a"})
	fakeRemote(t, testRefs(), nil, serve(data, nil))
	dir := t.TempDir()
	if _, err := Download(context.Background(), testTarget(t), "", dir, nil); err != nil {
		t.Fatal(err)
	}
	if got := dirTree(t, dir); len(got) != 1 || got[0] != "a.txt" {
		t.Errorf("tree = %v", got)
	}
}

func TestDownloadRejectsBadArchives(t *testing.T) {
	tests := []struct {
		name    string
		data    func(t *testing.T, root string) []byte
		wantErr string
	}{
		{"dot dot", func(t *testing.T, _ string) []byte {
			return tarGz(t, testSHA, entry{name: testPrefix + "../evil", typeflag: tar.TypeReg, body: "x"})
		}, `archive entry "` + testPrefix + `../evil" is outside the archive`},
		{"dot dot deeper", func(t *testing.T, _ string) []byte {
			return tarGz(t, testSHA, entry{name: testPrefix + "a/../../../evil", typeflag: tar.TypeReg, body: "x"})
		}, "is outside the archive"},
		{"leading dot dot", func(t *testing.T, _ string) []byte {
			return tarGz(t, testSHA, entry{name: "../evil", typeflag: tar.TypeReg, body: "x"})
		}, "is outside the archive"},
		{"dot dot symlink", func(t *testing.T, _ string) []byte {
			return tarGz(t, testSHA, entry{name: "../evil", typeflag: tar.TypeSymlink, link: "x"})
		}, "is outside the archive"},
		{"absolute", func(t *testing.T, root string) []byte {
			return tarGz(t, testSHA, entry{name: filepath.ToSlash(filepath.Join(root, "evil")), typeflag: tar.TypeReg, body: "x"})
		}, "has an absolute path"},
		{"two top-level directories", func(t *testing.T, _ string) []byte {
			return tarGz(t, testSHA,
				entry{name: testPrefix + "a", typeflag: tar.TypeReg, body: "a"},
				entry{name: "other/b", typeflag: tar.TypeReg, body: "b"})
		}, "more than 1 top-level directory"},
		{"comment mismatch", func(t *testing.T, _ string) []byte {
			return tarGz(t, otherSHA, entry{name: testPrefix + "a", typeflag: tar.TypeReg, body: "a"})
		}, "archive is of commit " + otherSHA + ", want " + testSHA},
		{"not gzip", func(*testing.T, string) []byte { return []byte("<html>not a tarball</html>") }, "gzip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "out")
			fakeRemote(t, testRefs(), nil, serve(tt.data(t, root), nil))
			_, err := Download(context.Background(), testTarget(t), "", dir, nil)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.HasPrefix(err.Error(), "download github.com/org/repo: ") || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q, want download prefix and %q", err, tt.wantErr)
			}
			if _, err := os.Lstat(filepath.Join(root, "evil")); err == nil {
				t.Error("evil was written outside the download directory")
			}
		})
	}
}

func TestDownloadStatus(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{http.StatusNotFound, "repository github.com/org/repo not found"},
		{http.StatusInternalServerError, "download github.com/org/repo: 500 Internal Server Error"},
		{http.StatusForbidden, "download github.com/org/repo: 403 Forbidden"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.code), func(t *testing.T) {
			fakeRemote(t, testRefs(), nil, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tt.code) })
			_, err := Download(context.Background(), testTarget(t), "", t.TempDir(), nil)
			if err == nil || err.Error() != tt.want {
				t.Errorf("error %v, want %q", err, tt.want)
			}
		})
	}
}

func TestDownloadFollowsRedirect(t *testing.T) {
	data := tarGz(t, testSHA, entry{name: testPrefix + "a", typeflag: tar.TypeReg, body: "a"})
	mux := http.NewServeMux()
	mux.HandleFunc("/org/repo/archive/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/codeload/org/repo/tar.gz/"+testSHA, http.StatusFound)
	})
	mux.HandleFunc("/codeload/", serve(data, nil))
	fakeRemote(t, testRefs(), nil, mux.ServeHTTP)
	dir := t.TempDir()
	if _, err := Download(context.Background(), testTarget(t), "", dir, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a")); err != nil {
		t.Error(err)
	}
}

func TestDownloadUnreachable(t *testing.T) {
	fakeRemote(t, testRefs(), nil, nil)
	archiveBase = "http://127.0.0.1:1" // refused at once
	_, err := Download(context.Background(), testTarget(t), "", t.TempDir(), nil)
	if err == nil || !strings.HasPrefix(err.Error(), "can't reach github.com: ") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDownloadCanceled(t *testing.T) {
	fakeRemote(t, testRefs(), nil, serve(nil, nil))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Download(ctx, testTarget(t), "", t.TempDir(), nil)
	if !errors.Is(err, context.Canceled) || err.Error() != "download github.com/org/repo: context canceled" {
		t.Errorf("got %v, want download github.com/org/repo: context canceled", err)
	}
}

// TestDownloadCanceledMidStream cancels while the body is still arriving.
func TestDownloadCanceledMidStream(t *testing.T) {
	data := tarGz(t, testSHA, entry{name: testPrefix + "a", typeflag: tar.TypeReg, body: strings.Repeat("x", 1<<20)})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fakeRemote(t, testRefs(), nil, func(w http.ResponseWriter, r *http.Request) {
		w.Write(data[:len(data)/2])
		w.(http.Flusher).Flush()
		cancel()
		<-r.Context().Done()
	})
	_, err := Download(ctx, testTarget(t), "", t.TempDir(), nil)
	if !errors.Is(err, context.Canceled) || err.Error() != "download github.com/org/repo: context canceled" {
		t.Errorf("got %v, want download github.com/org/repo: context canceled", err)
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name    string
		ref     string
		refs    []*plumbing.Reference
		err     error
		want    Checkout
		wantErr string
	}{
		{name: "default branch", refs: testRefs(), want: Checkout{Ref: "main", SHA: testSHA}},
		{name: "branch", ref: "main", refs: testRefs(), want: Checkout{Ref: "main", SHA: testSHA}},
		{name: "branch wins over tag", ref: "v1", refs: testRefs(), want: Checkout{Ref: "v1", SHA: otherSHA}},
		{name: "annotated tag is peeled", ref: "v2", refs: testRefs(), want: Checkout{Ref: "v2", SHA: tagCommit}},
		{name: "lightweight tag", ref: "v3", refs: testRefs(), want: Checkout{Ref: "v3", SHA: otherSHA}},
		{name: "unknown ref", ref: "nope", refs: testRefs(), wantErr: `ref "nope" not found in github.com/org/repo`},
		{name: "peeled name isn't a ref", ref: "v2^{}", refs: testRefs(), wantErr: `ref "v2^{}" not found in github.com/org/repo`},
		{name: "commit SHA", ref: testSHA, refs: testRefs(), wantErr: `ref "` + testSHA + `" not found`},
		{name: "full ref name", ref: "refs/heads/main", refs: testRefs(), wantErr: `ref "refs/heads/main" not found`},
		{name: "no HEAD", refs: testRefs()[1:], wantErr: "can't determine the default branch of github.com/org/repo"},
		{name: "empty list", refs: nil, wantErr: "repository github.com/org/repo is empty"},
		{name: "empty remote", err: transport.ErrEmptyRemoteRepository, wantErr: "repository github.com/org/repo is empty"},
		{name: "auth required", err: transport.ErrAuthenticationRequired,
			wantErr: "authentication failed for github.com/org/repo: the repository may be private or may not exist"},
		{name: "not found", err: transport.ErrRepositoryNotFound, wantErr: "repository github.com/org/repo not found"},
		{name: "other", err: errors.New("boom"), wantErr: "download github.com/org/repo: boom"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeRemote(t, tt.refs, tt.err, nil)
			got, err := resolve(context.Background(), testTarget(t), tt.ref)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %v, want %q", err, tt.wantErr)
				}
				if empty := strings.HasSuffix(tt.wantErr, " is empty"); errors.Is(err, ErrEmptyRepository) != empty {
					t.Errorf("errors.Is(%v, ErrEmptyRepository) = %v, want %v", err, !empty, empty)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// A failed ls-remote never reaches the archive host.
func TestDownloadStopsWhenResolveFails(t *testing.T) {
	var paths []string
	fakeRemote(t, testRefs(), nil, serve(nil, &paths))
	if _, err := Download(context.Background(), testTarget(t), "nope", t.TempDir(), nil); err == nil {
		t.Fatal("expected error")
	}
	if len(paths) != 0 {
		t.Errorf("archive requested: %v", paths)
	}
}
