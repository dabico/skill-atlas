//go:build e2e

// Package e2e runs the built skill-atlas binary and the download+scan pipeline against real remote repositories.
package e2e

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"skill-atlas/internal/repo"
	"skill-atlas/internal/scan"
	"skill-atlas/internal/skill"
)

var update = flag.Bool("update", false, "rewrite golden files")

var (
	binPath string // built skill-atlas binary
	workDir string // scratch root, removed at exit
	seq     int    // guarded by seqMu
	seqMu   sync.Mutex
)

func TestMain(m *testing.M) { os.Exit(run(m)) }

func run(m *testing.M) int {
	flag.Parse()

	root, err := moduleRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}
	workDir, err = os.MkdirTemp("", "skill-atlas-e2e-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}
	defer os.RemoveAll(workDir)
	// Playwright looks for browsers in $XDG_CACHE_HOME/ms-playwright too.
	// Pin it to the cache that screenshots.sh installed Chromium into, before XDG_CACHE_HOME moves.
	if err := keepPlaywrightBrowsers(); err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}
	// Keep the archive cache in the scratch root, for this process and the binaries it starts.
	// os.UserCacheDir reads XDG_CACHE_HOME on Linux only.
	if err := os.Setenv("XDG_CACHE_HOME", filepath.Join(workDir, "cache")); err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}

	binPath = filepath.Join(workDir, "skill-atlas")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/skill-atlas")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: build failed: %v\n%s", err, out)
		return 1
	}
	return m.Run()
}

// keepPlaywrightBrowsers sets PLAYWRIGHT_BROWSERS_PATH to <user cache dir>/ms-playwright unless it is set.
func keepPlaywrightBrowsers() error {
	if os.Getenv("PLAYWRIGHT_BROWSERS_PATH") != "" {
		return nil
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil // no default location for Playwright either
	}
	return os.Setenv("PLAYWRIGHT_BROWSERS_PATH", filepath.Join(cache, "ms-playwright"))
}

// moduleRoot resolves the module root from the test's working dir (<root>/e2e).
func moduleRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root := filepath.Dir(wd)
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return "", fmt.Errorf("go.mod not found in %s: %w", root, err)
	}
	return root, nil
}

// mkdir creates a fresh directory under the scratch root.
func mkdir(t testing.TB, prefix string) string {
	t.Helper()
	seqMu.Lock()
	seq++
	n := seq
	seqMu.Unlock()
	dir := filepath.Join(workDir, fmt.Sprintf("%s%d", prefix, n))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// fixture describes a pinned remote repository.
type fixture struct {
	url, tag, sha, display, name string
	paths                        []string // expected SKILL.md paths, sorted
	golden                       string
}

// ideavim is the baseline repository; 2.47.1 is an annotated tag.
var ideavim = fixture{
	url: "https://github.com/JetBrains/ideavim.git", tag: "2.47.1",
	sha:     "c1ae565cfb98be30ea75e4b351e823846c69c3c8",
	display: "github.com/JetBrains/ideavim", name: "ideavim",
	paths: []string{
		".claude/skills/changelog/SKILL.md",
		".claude/skills/doc-sync/SKILL.md",
		".claude/skills/extensions-api-migration/SKILL.md",
		".claude/skills/git-workflow/SKILL.md",
		".claude/skills/issues-deduplication/SKILL.md",
		".claude/skills/tests-maintenance/SKILL.md",
	},
	golden: "ideavim-2.47.1.golden.json",
}

// scanned is a downloaded url+ref and the result of scanning it.
type scanned struct {
	target      repo.Target
	checkout    repo.Checkout
	dir         string // the download, kept until the scratch root is removed
	skills      []skill.Skill
	excluded    int
	downloadDur time.Duration
	err         error
}

type cacheEntry struct {
	once sync.Once
	res  scanned
}

var (
	cache   = map[string]*cacheEntry{}
	cacheMu sync.Mutex
)

// downloadScan downloads url@ref once per process and scans the download with opts.
func downloadScan(t testing.TB, url, ref string, opts scan.Options) scanned {
	t.Helper()
	key := url + "|" + ref
	cacheMu.Lock()
	e, ok := cache[key]
	if !ok {
		e = &cacheEntry{}
		cache[key] = e
	}
	cacheMu.Unlock()

	e.once.Do(func() {
		dir := mkdir(t, "download-")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		target, err := repo.ParseURL(url)
		if err != nil {
			e.res.err = err
			return
		}
		e.res.target = target
		start := time.Now()
		co, err := repo.Download(ctx, target, ref, dir, nil)
		e.res.downloadDur = time.Since(start)
		if err != nil {
			e.res.err = err
			return
		}
		e.res.checkout = co
		e.res.dir = dir
	})
	if e.res.err != nil {
		t.Fatalf("download %s@%q: %v", url, ref, e.res.err)
	}
	res := e.res
	r, err := scan.Dir(res.dir, res.target.Name, opts)
	if err != nil {
		t.Fatalf("scan %s@%q: %v", url, ref, err)
	}
	res.skills, res.excluded = r.Skills, r.Excluded
	return res
}

// goldenSkill is the stable per-skill view stored in golden files.
type goldenSkill struct {
	Path          string       `json:"path"`
	Dir           string       `json:"dir"`
	Name          string       `json:"name"`
	Description   string       `json:"description"`
	License       string       `json:"license"`
	Compatibility string       `json:"compatibility"`
	AllowedTools  string       `json:"allowedTools"`
	Metadata      []goldenMeta `json:"metadata"`
	Extensions    []goldenExt  `json:"extensions"`
	Errors        []string     `json:"errors"`
	Valid         bool         `json:"valid"`
	BodySHA256    string       `json:"bodySha256"`
}

type goldenMeta struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type goldenExt struct {
	Provider string `json:"provider"`
	Key      string `json:"key"`
	Value    string `json:"value"`
}

func toGolden(skills []skill.Skill) []goldenSkill {
	out := make([]goldenSkill, 0, len(skills))
	for _, s := range skills {
		sum := sha256.Sum256([]byte(s.Body))
		g := goldenSkill{
			Path: s.Path, Dir: s.Dir, Name: s.Name, Description: s.Description,
			License: s.License, Compatibility: s.Compatibility, AllowedTools: s.AllowedTools,
			Metadata: []goldenMeta{}, Extensions: []goldenExt{}, Errors: []string{}, Valid: s.Valid(),
			BodySHA256: hex.EncodeToString(sum[:]),
		}
		for _, e := range s.Metadata {
			g.Metadata = append(g.Metadata, goldenMeta{e.Key, e.Value})
		}
		for _, e := range s.Extensions {
			g.Extensions = append(g.Extensions, goldenExt{e.Provider, e.Key, e.Value})
		}
		g.Errors = append(g.Errors, s.Errors...)
		out = append(out, g)
	}
	return out
}

func goldenPath(name string) string { return filepath.Join("testdata", name) }

func readGolden(t testing.TB, name string) []goldenSkill {
	t.Helper()
	data, err := os.ReadFile(goldenPath(name))
	if err != nil {
		t.Fatalf("read golden (regenerate with: go test -tags e2e ./e2e/... -update): %v", err)
	}
	var g []goldenSkill
	if err := json.Unmarshal(data, &g); err != nil {
		t.Fatalf("parse golden %s: %v", name, err)
	}
	return g
}

// checkGolden compares got with the golden file, or rewrites it under -update.
func checkGolden(t *testing.T, name string, skills []skill.Skill) {
	t.Helper()
	got := toGolden(skills)
	if *update {
		data, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath(name), append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want := readGolden(t, name)
	if diff := diffGolden(want, got); diff != "" {
		t.Errorf("scan differs from %s (regenerate with -update if intended):\n%s", name, diff)
	}
}

// diffGolden reports per-skill field differences, empty when equal.
func diffGolden(want, got []goldenSkill) string {
	var b strings.Builder
	wm := map[string]goldenSkill{}
	for _, s := range want {
		wm[s.Path] = s
	}
	gm := map[string]goldenSkill{}
	for _, s := range got {
		gm[s.Path] = s
	}
	for _, s := range want {
		g, ok := gm[s.Path]
		if !ok {
			fmt.Fprintf(&b, "- missing skill: %s\n", s.Path)
			continue
		}
		for _, d := range fieldDiffs(s, g) {
			fmt.Fprintf(&b, "~ %s: %s\n", s.Path, d)
		}
	}
	for _, s := range got {
		if _, ok := wm[s.Path]; !ok {
			fmt.Fprintf(&b, "+ unexpected skill: %s\n", s.Path)
		}
	}
	if b.Len() == 0 && len(want) == len(got) {
		for i := range want {
			if want[i].Path != got[i].Path {
				return "skill order differs\n"
			}
		}
	}
	return b.String()
}

func fieldDiffs(w, g goldenSkill) []string {
	var out []string
	str := func(name, a, b string) {
		if a != b {
			out = append(out, fmt.Sprintf("%s: want %q, got %q", name, clip(a), clip(b)))
		}
	}
	str("dir", w.Dir, g.Dir)
	str("name", w.Name, g.Name)
	str("description", w.Description, g.Description)
	str("license", w.License, g.License)
	str("compatibility", w.Compatibility, g.Compatibility)
	str("allowedTools", w.AllowedTools, g.AllowedTools)
	str("bodySha256", w.BodySHA256, g.BodySHA256)
	if w.Valid != g.Valid {
		out = append(out, fmt.Sprintf("valid: want %v, got %v", w.Valid, g.Valid))
	}
	if a, b := jsonStr(w.Metadata), jsonStr(g.Metadata); a != b {
		out = append(out, fmt.Sprintf("metadata: want %s, got %s", clip(a), clip(b)))
	}
	if a, b := jsonStr(w.Extensions), jsonStr(g.Extensions); a != b {
		out = append(out, fmt.Sprintf("extensions: want %s, got %s", clip(a), clip(b)))
	}
	if a, b := jsonStr(w.Errors), jsonStr(g.Errors); a != b {
		out = append(out, fmt.Sprintf("errors: want %s, got %s", clip(a), clip(b)))
	}
	return out
}

func jsonStr(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func clip(s string) string {
	if r := []rune(s); len(r) > 200 {
		return string(r[:200]) + "…"
	}
	return s
}
