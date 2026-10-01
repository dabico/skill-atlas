package repo

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// counting answers every request with data and counts the requests.
func counting(data []byte, n *atomic.Int32) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		serve(data, nil)(w, r)
	}
}

func goodArchive(t *testing.T) []byte {
	t.Helper()
	return tarGz(t, testSHA,
		entry{name: testPrefix, typeflag: tar.TypeDir},
		entry{name: testPrefix + "skills/a/SKILL.md", typeflag: tar.TypeReg, body: "---\nname: a\n---\n"},
		entry{name: testPrefix + "README.md", typeflag: tar.TypeReg, body: "hi"},
	)
}

const goodTree = "README.md skills/ skills/a/ skills/a/SKILL.md"

// cacheEntries lists the names in the cache directory.
func cacheEntries(t *testing.T, cache string) []string {
	t.Helper()
	entries, err := os.ReadDir(cache)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// noWarn fails the test when Download warns.
func noWarn(t *testing.T) func(error) {
	return func(err error) { t.Errorf("unexpected warning: %v", err) }
}

func TestCacheMissThenHit(t *testing.T) {
	data := goodArchive(t)
	var hits atomic.Int32
	fakeRemote(t, testRefs(), nil, counting(data, &hits))
	cache := useCache(t)

	first := filepath.Join(t.TempDir(), "repo-0")
	if _, err := Download(context.Background(), testTarget(t), "", first, noWarn(t)); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(cache, testSHA+".tar.gz")
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("no cache entry: %v", err)
	}
	if !bytes.Equal(b, data) {
		t.Error("cache entry differs from the downloaded tarball")
	}
	if got := cacheEntries(t, cache); len(got) != 1 {
		t.Errorf("cache holds %v, want only %s.tar.gz", got, testSHA)
	}

	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(file, old, old); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(t.TempDir(), "repo-1")
	co, err := Download(context.Background(), testTarget(t), "", second, noWarn(t))
	if err != nil {
		t.Fatal(err)
	}
	if co != (Checkout{Ref: "main", SHA: testSHA}) {
		t.Errorf("checkout = %+v", co)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("%d archive requests, want 1: the second download should come from the cache", n)
	}
	for _, dir := range []string{first, second} {
		if got := strings.Join(dirTree(t, dir), " "); got != goodTree {
			t.Errorf("%s: tree = %s, want %s", dir, got, goodTree)
		}
	}
	if fi, err := os.Stat(file); err != nil || !fi.ModTime().After(old.Add(time.Hour)) {
		t.Errorf("a cache hit didn't update the mtime: %v, %v", fi.ModTime(), err)
	}
}

// ls-remote still runs on a cache hit, so a moved branch gets the new commit.
func TestCacheHitStillResolves(t *testing.T) {
	var hits atomic.Int32
	fakeRemote(t, testRefs(), nil, counting(goodArchive(t), &hits))
	useCache(t)
	if _, err := Download(context.Background(), testTarget(t), "", t.TempDir(), noWarn(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := Download(context.Background(), testTarget(t), "nope", t.TempDir(), noWarn(t)); err == nil || !strings.Contains(err.Error(), `ref "nope" not found`) {
		t.Errorf("got %v, want the ls-remote error", err)
	}
}

// The cache key is the commit SHA alone: a fork at the same commit uses the same entry.
func TestCacheSharedBySHA(t *testing.T) {
	var hits atomic.Int32
	fakeRemote(t, testRefs(), nil, counting(goodArchive(t), &hits))
	cache := useCache(t)
	fork, err := ParseURL("https://github.com/someone/fork")
	if err != nil {
		t.Fatal(err)
	}
	for _, tg := range []Target{testTarget(t), fork} {
		dir := t.TempDir()
		if _, err := Download(context.Background(), tg, "", dir, noWarn(t)); err != nil {
			t.Fatal(err)
		}
		if got := strings.Join(dirTree(t, dir), " "); got != goodTree {
			t.Errorf("%s: tree = %s", tg.Display, got)
		}
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("%d archive requests, want 1", n)
	}
	if got := cacheEntries(t, cache); len(got) != 1 || got[0] != testSHA+".tar.gz" {
		t.Errorf("cache holds %v", got)
	}
}

func TestCacheCorruptEntryIsReplaced(t *testing.T) {
	good := goodArchive(t)
	tests := []struct {
		name string
		data []byte
	}{
		{"not gzip", []byte("garbage")},
		{"truncated", good[:len(good)/2]},
		{"other commit", tarGz(t, otherSHA, entry{name: testPrefix + "a", typeflag: tar.TypeReg, body: "a"})},
		{"path escape", tarGz(t, testSHA,
			entry{name: testPrefix + "stale.txt", typeflag: tar.TypeReg, body: "stale"},
			entry{name: testPrefix + "../evil", typeflag: tar.TypeReg, body: "x"})},
		{"empty", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hits atomic.Int32
			fakeRemote(t, testRefs(), nil, counting(good, &hits))
			cache := useCache(t)
			if err := os.MkdirAll(cache, 0o700); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(cache, testSHA+".tar.gz")
			if err := os.WriteFile(file, tt.data, 0o600); err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			dir := filepath.Join(root, "repo-0")
			if _, err := Download(context.Background(), testTarget(t), "", dir, noWarn(t)); err != nil {
				t.Fatal(err)
			}
			if n := hits.Load(); n != 1 {
				t.Errorf("%d archive requests, want 1", n)
			}
			if got := strings.Join(dirTree(t, dir), " "); got != goodTree {
				t.Errorf("tree = %s, want %s: the bad entry's files must be cleared", got, goodTree)
			}
			if b, err := os.ReadFile(file); err != nil || !bytes.Equal(b, good) {
				t.Errorf("cache entry wasn't replaced with the download: %v", err)
			}
			if _, err := os.Lstat(filepath.Join(root, "evil")); err == nil {
				t.Error("evil was written outside the download directory")
			}
		})
	}
}

// A cached entry that escapes the directory is refetched; the same download then fails as usual.
func TestCacheCorruptEntryAndBadDownload(t *testing.T) {
	bad := tarGz(t, testSHA, entry{name: testPrefix + "../evil", typeflag: tar.TypeReg, body: "x"})
	var hits atomic.Int32
	fakeRemote(t, testRefs(), nil, counting(bad, &hits))
	cache := useCache(t)
	if err := os.MkdirAll(cache, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, testSHA+".tar.gz"), bad, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Download(context.Background(), testTarget(t), "", t.TempDir(), noWarn(t))
	if err == nil || !strings.Contains(err.Error(), "is outside the archive") {
		t.Errorf("got %v, want the path escape error", err)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("%d archive requests, want 1", n)
	}
	if got := cacheEntries(t, cache); len(got) != 0 {
		t.Errorf("cache holds %v, want nothing", got)
	}
}

// A download that fails or is cancelled leaves nothing in the cache, not even a temporary file.
func TestCacheNothingKeptOnFailure(t *testing.T) {
	good := goodArchive(t)
	tests := []struct {
		name    string
		handler func(cancel context.CancelFunc) http.HandlerFunc
	}{
		{"bad archive", func(context.CancelFunc) http.HandlerFunc {
			return serve(tarGz(t, otherSHA, entry{name: testPrefix + "a", typeflag: tar.TypeReg, body: "a"}), nil)
		}},
		{"not gzip", func(context.CancelFunc) http.HandlerFunc { return serve([]byte("<html>"), nil) }},
		{"truncated", func(context.CancelFunc) http.HandlerFunc { return serve(good[:len(good)-4], nil) }},
		{"status", func(context.CancelFunc) http.HandlerFunc {
			return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }
		}},
		{"cancelled mid-stream", func(cancel context.CancelFunc) http.HandlerFunc {
			big := tarGz(t, testSHA, entry{name: testPrefix + "a", typeflag: tar.TypeReg, body: strings.Repeat("x", 1<<20)})
			return func(w http.ResponseWriter, r *http.Request) {
				w.Write(big[:len(big)/2])
				w.(http.Flusher).Flush()
				cancel()
				<-r.Context().Done()
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fakeRemote(t, testRefs(), nil, tt.handler(cancel))
			cache := useCache(t)
			if _, err := Download(ctx, testTarget(t), "", t.TempDir(), noWarn(t)); err == nil {
				t.Fatal("expected error")
			}
			if got := cacheEntries(t, cache); len(got) != 0 {
				t.Errorf("cache holds %v, want nothing", got)
			}
		})
	}
}

// A cancelled unpack from the cache keeps the entry: it isn't corrupt.
func TestCacheHitCancelledKeepsEntry(t *testing.T) {
	var hits atomic.Int32
	fakeRemote(t, testRefs(), nil, counting(goodArchive(t), &hits))
	cache := useCache(t)
	if _, err := Download(context.Background(), testTarget(t), "", t.TempDir(), noWarn(t)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Download(ctx, testTarget(t), "", t.TempDir(), noWarn(t))
	if !errors.Is(err, context.Canceled) || err.Error() != "download github.com/org/repo: context canceled" {
		t.Errorf("got %v, want download github.com/org/repo: context canceled", err)
	}
	if got := cacheEntries(t, cache); len(got) != 1 {
		t.Errorf("cache holds %v, want the entry kept", got)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("%d archive requests, want 1", n)
	}
}

// Without a usable cache the download streams as before and warn hears why.
func TestCacheUnavailable(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T) (string, error)
	}{
		{"file in the way", func(t *testing.T) (string, error) {
			f := filepath.Join(t.TempDir(), "cache")
			if err := os.WriteFile(f, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			return filepath.Join(f, "skill-atlas", "archives"), nil
		}},
		{"no cache dir", func(*testing.T) (string, error) {
			return "", errors.New("neither $XDG_CACHE_HOME nor $HOME are defined")
		}},
		{"entry can't be replaced", func(t *testing.T) (string, error) {
			// A non-empty directory where the tarball goes: it can't be read, removed or renamed over.
			cache := filepath.Join(t.TempDir(), "archives")
			if err := os.MkdirAll(filepath.Join(cache, testSHA+".tar.gz", "x"), 0o700); err != nil {
				t.Fatal(err)
			}
			return cache, nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hits atomic.Int32
			fakeRemote(t, testRefs(), nil, counting(goodArchive(t), &hits))
			cache, cerr := tt.setup(t)
			cacheDir = func() (string, error) { return cache, cerr }
			var warnings []error
			dir := t.TempDir()
			for range 2 {
				warnings = nil
				if _, err := Download(context.Background(), testTarget(t), "", dir, func(err error) { warnings = append(warnings, err) }); err != nil {
					t.Fatal(err)
				}
				if got := strings.Join(dirTree(t, dir), " "); got != goodTree {
					t.Errorf("tree = %s, want %s", got, goodTree)
				}
				if len(warnings) != 1 || warnings[0] == nil {
					t.Errorf("warnings = %v, want 1", warnings)
				}
			}
			if n := hits.Load(); n != 2 {
				t.Errorf("%d archive requests, want 2", n)
			}
			if fi, err := os.Stat(cache); err == nil && fi.IsDir() {
				for _, name := range cacheEntries(t, cache) {
					if strings.HasSuffix(name, ".tmp") {
						t.Errorf("temporary file %s left in the cache", name)
					}
				}
			}
		})
	}
}

func TestCachePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no Unix permissions")
	}
	fakeRemote(t, testRefs(), nil, serve(goodArchive(t), nil))
	cache := useCache(t)
	if _, err := Download(context.Background(), testTarget(t), "", t.TempDir(), noWarn(t)); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{cache, filepath.Dir(cache)} {
		if fi, err := os.Stat(dir); err != nil || fi.Mode().Perm() != 0o700 {
			t.Errorf("%s: mode %v, %v; want 0700", dir, fi.Mode().Perm(), err)
		}
	}
	if fi, err := os.Stat(filepath.Join(cache, testSHA+".tar.gz")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("cache entry mode %v, %v; want 0600", fi.Mode().Perm(), err)
	}
}

func TestUserCacheDir(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("XDG_CACHE_HOME is read on Linux")
	}
	t.Setenv("XDG_CACHE_HOME", "/x/cache")
	if d, err := userCacheDir(); err != nil || d != filepath.Join("/x/cache", "skill-atlas", "archives") {
		t.Errorf("got %q, %v", d, err)
	}
}
