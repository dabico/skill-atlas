//go:build e2e

package e2e

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"skill-atlas/internal/scan"
	"skill-atlas/internal/skill"
)

// koog 1.3.0 has 2 real skills in .claude/skills and 2 test fixtures under integration-tests.
// It is the fixture for the exclude tests; ideavim stays the baseline.
var koog = fixture{
	url: "https://github.com/JetBrains/koog.git", tag: "1.3.0",
	sha:     "3acc88cf8ce70b87d8afbd3cf184844a50aa504e",
	display: "github.com/JetBrains/koog", name: "koog",
	paths: []string{
		".claude/skills/add-java-code-snippets-in-docs/SKILL.md",
		".claude/skills/split-jvm-nonjvm/SKILL.md",
	},
}

const koogSplit = ".claude/skills/split-jvm-nonjvm/SKILL.md"

// koogFixtures are the test-data skills that the default rule skips.
var koogFixtures = []string{
	"integration-tests/src/jvmTest/resources/skills/arithmetic-evaluator/SKILL.md",
	"integration-tests/src/jvmTest/resources/skills/weather-retrieval/SKILL.md",
}

func skillPaths(skills []skill.Skill) []string {
	var got []string
	for _, s := range skills {
		got = append(got, s.Path)
	}
	return got
}

func invalidCount(skills []skill.Skill) int {
	n := 0
	for _, s := range skills {
		if !s.Valid() {
			n++
		}
	}
	return n
}

// checkKoogCheckout asserts the scan hit the pinned tag and commit.
func checkKoogCheckout(t *testing.T, res scanned) {
	t.Helper()
	if res.checkout.Ref != koog.tag {
		t.Errorf("Ref = %q, want %q", res.checkout.Ref, koog.tag)
	}
	if res.checkout.SHA != koog.sha {
		t.Errorf("SHA = %q, want %q", res.checkout.SHA, koog.sha)
	}
}

func checkScan(t *testing.T, res scanned, wantPaths []string, wantExcluded int) {
	t.Helper()
	checkInvariants(t, res.skills, koog.name)
	if got := skillPaths(res.skills); !slices.Equal(got, wantPaths) {
		t.Errorf("paths = %v, want %v", got, wantPaths)
	}
	if res.excluded != wantExcluded {
		t.Errorf("excluded = %d, want %d", res.excluded, wantExcluded)
	}
}

// AC1: the default rule skips the 2 fixtures under integration-tests.
func TestExcludeKoogDefault(t *testing.T) {
	t.Parallel()
	res := cloneScan(t, koog.url, koog.tag, scan.Options{})
	checkKoogCheckout(t, res)
	checkScan(t, res, koog.paths, 2)
}

// AC2: --include-tests keeps all 4.
func TestExcludeKoogIncludeTests(t *testing.T) {
	t.Parallel()
	res := cloneScan(t, koog.url, koog.tag, scan.Options{IncludeTests: true})
	checkKoogCheckout(t, res)
	want := slices.Sorted(slices.Values(append(slices.Clone(koog.paths), koogFixtures...)))
	checkScan(t, res, want, 0)
}

// AC3: one --exclude directory pattern on top of the default rule.
func TestExcludeKoogPattern(t *testing.T) {
	t.Parallel()
	res := cloneScan(t, koog.url, koog.tag, scan.Options{Exclude: []string{".claude/skills/split-jvm-nonjvm/"}})
	checkKoogCheckout(t, res)
	checkScan(t, res, koog.paths[:1], 3)
}

// --exclude and --include-tests combine: patterns still apply once the default rule is off.
func TestExcludeKoogPatternWithIncludeTests(t *testing.T) {
	t.Parallel()
	all := slices.Sorted(slices.Values(append(slices.Clone(koog.paths), koogFixtures...)))
	cases := []struct {
		name     string
		patterns []string
		want     []string
		excluded int
	}{
		{"one skill", []string{".claude/skills/split-jvm-nonjvm/"}, slices.DeleteFunc(slices.Clone(all), func(p string) bool { return p == koogSplit }), 1},
		{"fixtures directory", []string{"integration-tests/"}, koog.paths, 2},
		{"double star", []string{"**/jvmTest"}, koog.paths, 2},
		{"name at any depth", []string{"skills"}, nil, 4},
		{"negation", []string{"skills", "!.claude/"}, koog.paths, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res := cloneScan(t, koog.url, koog.tag, scan.Options{IncludeTests: true, Exclude: c.patterns})
			checkScan(t, res, c.want, c.excluded)
		})
	}
}

// AC4: a repository with no test directories above its skills scans as before.
func TestExcludeIdeavimUnchanged(t *testing.T) {
	t.Parallel()
	f := ideavim
	for _, opts := range []scan.Options{{}, {IncludeTests: true}} {
		res := cloneScan(t, f.url, f.tag, opts)
		if got := skillPaths(res.skills); !slices.Equal(got, f.paths) {
			t.Errorf("%+v: paths = %v, want %v", opts, got, f.paths)
		}
		if res.excluded != 0 {
			t.Errorf("%+v: excluded = %d, want 0", opts, res.excluded)
		}
		checkGolden(t, f.golden, res.skills)
	}
}

// headerRE matches the TUI header line for f with the given counts text.
func headerRE(f fixture, counts string) *regexp.Regexp {
	return regexp.MustCompile(regexp.QuoteMeta(fmt.Sprintf("%s @ %s (%s)", f.display, f.tag, f.sha[:7])) +
		`\s+` + regexp.QuoteMeta(counts))
}

// AC5: the TUI header counts the excluded skills, and q exits 0.
func TestTUIExcludedDefault(t *testing.T) {
	t.Parallel()
	ref := cloneScan(t, koog.url, koog.tag, scan.Options{})
	counts := fmt.Sprintf("%d skills, %d invalid, %d excluded", len(ref.skills), invalidCount(ref.skills), ref.excluded)
	if want := "2 skills, 0 invalid, 2 excluded"; counts != want {
		t.Fatalf("scan API counts = %q, want %q", counts, want)
	}

	s := startScan(t, "--ref", koog.tag, koog.url)
	pane := s.waitFor(headerRE(koog, counts), cloneWait)
	if !strings.Contains(leftPane(pane), ref.skills[0].DisplayName()) {
		t.Errorf("list doesn't show first skill %q:\n%s", ref.skills[0].DisplayName(), pane)
	}
	if strings.Contains(pane, "weather-retrieval") || strings.Contains(pane, "arithmetic-evaluator") {
		t.Errorf("an excluded fixture is listed:\n%s", pane)
	}

	s.keys("q")
	if code := s.waitExit(shortWait); code != 0 {
		t.Errorf("exit code = %d, want 0\n%s", code, s.last)
	}
	s.assertTmpClean()
}

// The flags reach the scan through the binary.
func TestTUIExcludeFlags(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		counts string
		empty  string // list pane message when no skills remain
	}{
		{"include-tests", []string{"--include-tests"}, "4 skills, 0 invalid", ""},
		{"exclude", []string{"--exclude", ".claude/skills/split-jvm-nonjvm/"}, "1 skill, 0 invalid, 3 excluded", ""},
		{"exclude equals with include-tests", []string{"--include-tests", "--exclude=integration-tests"}, "2 skills, 0 invalid, 2 excluded", ""},
		{"everything excluded", []string{"--exclude", "SKILL.md"}, "0 skills, 0 invalid, 4 excluded", "No skills found (4 excluded)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s := startScan(t, append(append([]string{"--ref", koog.tag}, c.args...), koog.url)...)
			pane := s.waitFor(headerRE(koog, c.counts), cloneWait)
			if c.empty != "" && !strings.Contains(leftPane(pane), c.empty) {
				t.Errorf("list pane lacks %q:\n%s", c.empty, pane)
			}
			s.keys("q")
			if code := s.waitExit(shortWait); code != 0 {
				t.Errorf("exit code = %d, want 0\n%s", code, s.last)
			}
			s.assertTmpClean()
		})
	}
}
