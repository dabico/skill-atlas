package scan

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// scanPaths returns the kept paths and the excluded count for root.
func scanPaths(t *testing.T, root string, opts Options) ([]string, int) {
	t.Helper()
	res, err := Dir(root, "repo", opts)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range res.Skills {
		got = append(got, s.Path)
	}
	return got, res.Excluded
}

// tree writes a SKILL.md at each given directory ("" is the root) and returns the root.
func tree(t *testing.T, dirs ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range dirs {
		rel := "SKILL.md"
		name := "repo"
		if d != "" {
			rel = d + "/SKILL.md"
			name = filepath.Base(d)
		}
		write(t, root, rel, doc(name))
	}
	return root
}

func TestDirExcludePatterns(t *testing.T) {
	dirs := []string{
		"docs/a", "docs/deep/b", "src/docs/c", "skills/old/d", "skills/new/e", "other/skills/old/f",
		"vendor/g", "pkg/vendor/h", "examples/i", "pkg/examples/j", "keep/k",
	}
	tests := []struct {
		name     string
		patterns []string
		excluded []string // directories expected to be skipped
	}{
		{"directory with slash", []string{"docs/"}, []string{"docs/a", "docs/deep/b", "src/docs/c"}},
		{"directory without slash", []string{"docs"}, []string{"docs/a", "docs/deep/b", "src/docs/c"}},
		{"anchored with leading slash", []string{"/docs"}, []string{"docs/a", "docs/deep/b"}},
		{"anchored by inner slash", []string{"skills/old"}, []string{"skills/old/d"}},
		{"anchored nested directory", []string{"docs/deep/"}, []string{"docs/deep/b"}},
		{"double star prefix", []string{"**/old"}, []string{"skills/old/d", "other/skills/old/f"}},
		{"double star middle", []string{"skills/**/d"}, []string{"skills/old/d"}},
		{"double star suffix", []string{"docs/**"}, []string{"docs/a", "docs/deep/b"}},
		{"single star segment", []string{"skills/*/d"}, []string{"skills/old/d"}},
		{"star name", []string{"ex*"}, []string{"examples/i", "pkg/examples/j"}},
		{"question mark", []string{"vendo?"}, []string{"vendor/g", "pkg/vendor/h"}},
		{"file pattern", []string{"keep/k/SKILL.md"}, []string{"keep/k"}},
		{"directory-only pattern misses skill file", []string{"SKILL.md/"}, nil},
		{"several patterns", []string{"docs/", "vendor", "examples"},
			[]string{"docs/a", "docs/deep/b", "src/docs/c", "vendor/g", "pkg/vendor/h", "examples/i", "pkg/examples/j"}},
		{"negation re-includes", []string{"docs/", "!docs/deep/"}, []string{"docs/a", "src/docs/c"}},
		{"negation before pattern has no effect", []string{"!docs/deep/", "docs/"}, []string{"docs/a", "docs/deep/b", "src/docs/c"}},
		{"negation alone excludes nothing", []string{"!docs/"}, nil},
		{"negation re-includes file in excluded directory", []string{"vendor", "!vendor/g/SKILL.md"}, []string{"pkg/vendor/h"}},
		{"no match", []string{"nothing-here"}, nil},
		{"trailing spaces are ignored", []string{"docs/   "}, []string{"docs/a", "docs/deep/b", "src/docs/c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := tree(t, dirs...)
			got, excluded := scanPaths(t, root, Options{Exclude: tt.patterns})

			gone := map[string]bool{}
			for _, d := range tt.excluded {
				gone[d+"/SKILL.md"] = true
			}
			var want []string
			for _, d := range dirs {
				if !gone[d+"/SKILL.md"] {
					want = append(want, d+"/SKILL.md")
				}
			}
			slices.Sort(want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("paths = %v\nwant    %v", got, want)
			}
			if excluded != len(tt.excluded) {
				t.Errorf("excluded = %d, want %d", excluded, len(tt.excluded))
			}
		})
	}
}

func TestDirExcludeMatchesOwnDirAndRoot(t *testing.T) {
	root := tree(t, "", "tests-maintenance", "sub/tests-maintenance")
	got, excluded := scanPaths(t, root, Options{Exclude: []string{"tests-maintenance"}})
	if want := []string{"SKILL.md"}; !reflect.DeepEqual(got, want) || excluded != 2 {
		t.Errorf("got %v, excluded %d", got, excluded)
	}
	got, excluded = scanPaths(t, root, Options{Exclude: []string{"/SKILL.md"}})
	if want := []string{"sub/tests-maintenance/SKILL.md", "tests-maintenance/SKILL.md"}; !reflect.DeepEqual(got, want) || excluded != 1 {
		t.Errorf("got %v, excluded %d", got, excluded)
	}
}

func TestDirNoPatternsExcludesNothing(t *testing.T) {
	root := tree(t, "", "tests/a", "src/jvmTest/b", "testdata/c", "fixtures/d", "keep/e")
	got, excluded := scanPaths(t, root, Options{})
	want := []string{"SKILL.md", "fixtures/d/SKILL.md", "keep/e/SKILL.md", "src/jvmTest/b/SKILL.md", "testdata/c/SKILL.md", "tests/a/SKILL.md"}
	if !reflect.DeepEqual(got, want) || excluded != 0 {
		t.Errorf("got %v, excluded %d", got, excluded)
	}
}

func TestDirExcludedNotParsed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes unsupported")
	}
	root := tree(t, "keep/a")
	write(t, root, "skip/b/SKILL.md", doc("b"))
	p := filepath.Join(root, "skip", "b", "SKILL.md")
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(p, 0o644) })
	if _, err := os.ReadFile(p); err == nil {
		t.Skip("file is readable despite mode 0 (running as root?)")
	}

	got, excluded := scanPaths(t, root, Options{Exclude: []string{"skip/"}})
	if want := []string{"keep/a/SKILL.md"}; !reflect.DeepEqual(got, want) || excluded != 1 {
		t.Errorf("got %v, excluded %d", got, excluded)
	}
	if _, err := Dir(root, "repo", Options{}); err == nil {
		t.Error("the unreadable file should be read once it isn't excluded")
	}
}

func TestDirSymlinksAndGitNotCounted(t *testing.T) {
	root := tree(t, "keep/a")
	write(t, root, "skip/real/SKILL.md", doc("real"))
	write(t, root, ".git/skip/SKILL.md", doc("hooks"))
	if err := os.Symlink(filepath.Join(root, "skip", "real", "SKILL.md"), filepath.Join(root, "skip", "link.md")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "skip", "linked"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "skip", "real", "SKILL.md"), filepath.Join(root, "skip", "linked", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	got, excluded := scanPaths(t, root, Options{Exclude: []string{"skip"}})
	if want := []string{"keep/a/SKILL.md"}; !reflect.DeepEqual(got, want) || excluded != 1 {
		t.Errorf("got %v, excluded %d (want only skip/real counted)", got, excluded)
	}
}

func TestDirResultOnlyExcluded(t *testing.T) {
	root := tree(t, "skip/a", "skip/b")
	res, err := Dir(root, "repo", Options{Exclude: []string{"skip"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Skills != nil || res.Excluded != 2 {
		t.Errorf("got %+v, want nil skills and 2 excluded", res)
	}
}

func TestDirRejectsBadPatterns(t *testing.T) {
	root := tree(t, "a")
	for _, p := range []string{"", "  ", "!", "/", "[", "a[", "a**b", "**x/y"} {
		if _, err := Dir(root, "repo", Options{Exclude: []string{"ok", p}}); err == nil {
			t.Errorf("pattern %q: expected error", p)
		}
	}
}

func TestCheckPattern(t *testing.T) {
	good := []string{"docs", "docs/", "/docs", "!docs/deep", "**/x", "a/**/b", "a/**", "*.md", "a?b", "[ab]c", `\!x`, "a b"}
	for _, p := range good {
		if err := CheckPattern(p); err != nil {
			t.Errorf("CheckPattern(%q) = %v", p, err)
		}
	}
	bad := map[string]string{
		"":      "empty",
		" \t":   "empty",
		"!":     "empty",
		"//":    "empty",
		"[":     "malformed",
		"a[b":   "malformed",
		"x**":   "whole path segment",
		"a/b**": "whole path segment",
	}
	for p, want := range bad {
		err := CheckPattern(p)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("CheckPattern(%q) = %v, want error containing %q", p, err, want)
		}
	}
}
