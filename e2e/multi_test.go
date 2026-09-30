//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"skill-atlas/internal/scan"
)

const zoomDisplay = "github.com/zcaceres/skills"

// multiArgs are the two fixtures, each pinned with #<ref>.
func multiArgs() []string {
	return []string{ideavim.url + "#" + ideavim.tag, claudeSkills.url + "#" + claudeSkills.tag}
}

func ideavimLabel() string {
	return fmt.Sprintf("%s @ %s (%s)", ideavim.display, ideavim.tag, ideavim.sha[:7])
}

func zoomLabel() string {
	return fmt.Sprintf("%s @ %s (%s)", zoomDisplay, claudeSkills.tag, claudeSkills.sha[:7])
}

// multiTotals returns the skill and invalid counts over both fixtures.
func multiTotals(t *testing.T) (skills, invalid int) {
	t.Helper()
	for _, g := range readGolden(t, ideavim.golden) {
		skills++
		if !g.Valid {
			invalid++
		}
	}
	for _, s := range cloneScan(t, claudeSkills.url, claudeSkills.tag, scan.Options{}).skills {
		skills++
		if !s.Valid() {
			invalid++
		}
	}
	return skills, invalid
}

func TestTUIMultiRepo(t *testing.T) {
	t.Parallel()
	golden := readGolden(t, ideavim.golden)
	zoom := cloneScan(t, claudeSkills.url, claudeSkills.tag, scan.Options{}).skills
	total, invalid := multiTotals(t)
	s := startScan(t, multiArgs()...)

	header := regexp.MustCompile(`2 repositories\s+` + fmt.Sprintf("%d skills, %d invalid", total, invalid))
	pane := s.waitFor(header, cloneWait)
	if !strings.Contains(leftPane(pane), ideavim.display) {
		t.Errorf("list lacks the ideavim group heading:\n%s", pane)
	}
	// The list pane cuts the heading, so the full label shows in the detail pane.
	if !strings.Contains(rightPane(pane), ideavimLabel()) {
		t.Errorf("detail lacks %q:\n%s", ideavimLabel(), pane)
	}
	if !strings.Contains(rightPane(pane), golden[0].Path) {
		t.Errorf("detail doesn't show the first skill %q:\n%s", golden[0].Path, pane)
	}

	// j from the last ideavim skill crosses the zoom heading onto the first zoom skill.
	for range len(golden) - 1 {
		s.keys("j")
	}
	last := golden[len(golden)-1].Path
	s.waitFor(regexp.MustCompile(regexp.QuoteMeta(last)), shortWait)
	s.keys("j")
	pane = s.waitUntil("detail shows the first zoom skill", shortWait, func(p string) bool {
		return strings.Contains(rightPane(p), zoomLabel()) && strings.Contains(rightPane(p), zoom[0].Path)
	})
	if strings.Contains(rightPane(pane), last) {
		t.Errorf("detail still shows the last ideavim skill:\n%s", pane)
	}
	if !strings.Contains(leftPane(pane), zoomDisplay) {
		t.Errorf("list lacks the zoom group heading next to its first skill:\n%s", pane)
	}

	// k goes back over the heading.
	s.keys("k")
	s.waitUntil("detail shows the last ideavim skill", shortWait, func(p string) bool {
		return strings.Contains(rightPane(p), ideavimLabel()) && strings.Contains(rightPane(p), last)
	})

	s.keys("q")
	if code := s.waitExit(shortWait); code != 0 {
		t.Errorf("exit code = %d, want 0\n%s", code, s.last)
	}
	s.assertTmpClean()
}

const (
	badURL     = "https://github.com/JetBrains/this-repo-does-not-exist-e2e.git"
	badDisplay = "github.com/JetBrains/this-repo-does-not-exist-e2e"
	badError   = "authentication failed for " + badDisplay
)

// ideavimTotals returns the skill and invalid counts of the ideavim golden file.
func ideavimTotals(t *testing.T) (skills, invalid int) {
	t.Helper()
	for _, g := range readGolden(t, ideavim.golden) {
		skills++
		if !g.Valid {
			invalid++
		}
	}
	return skills, invalid
}

func TestTUIPartialFailure(t *testing.T) {
	t.Parallel()
	skills, invalid := ideavimTotals(t)
	s := startScan(t, ideavim.url+"#"+ideavim.tag, badURL)

	header := regexp.MustCompile(`2 repositories\s+` + fmt.Sprintf("%d skills, %d invalid, 1 failed", skills, invalid))
	pane := s.waitFor(header, cloneWait)
	if !strings.Contains(leftPane(pane), ideavim.display) || !strings.Contains(leftPane(pane), badDisplay[:30]) { // the list pane cuts long headings
		t.Errorf("list lacks a group heading:\n%s", pane)
	}
	// The list pane is narrow, so the error row shows the start of the message.
	if !strings.Contains(leftPane(pane), "authentication failed for") {
		t.Errorf("list lacks the error row:\n%s", pane)
	}

	// G can't reach the failed repository's rows: the last skill of ideavim stays selected.
	s.keys("G")
	golden := readGolden(t, ideavim.golden)
	s.waitFor(regexp.MustCompile(regexp.QuoteMeta(golden[len(golden)-1].Path)), shortWait)

	s.keys("q")
	if code := s.waitExit(shortWait); code != 1 {
		t.Errorf("exit code = %d, want 1\n%s", code, s.last)
	}
	s.assertTmpClean()
}

func TestTUIAllReposFail(t *testing.T) {
	t.Parallel()
	s := startScan(t, badURL+"#no-such-ref-e2e", badURL)
	if code := s.waitExit(2 * cloneWait); code != 1 {
		t.Errorf("exit code = %d, want 1\n%s", code, s.last)
	}
	if !strings.Contains(s.last, "skill-atlas: "+badDisplay+" @ no-such-ref-e2e: ") || strings.Contains(s.last, "repositories") {
		t.Errorf("pane lacks the failure line, or the TUI started:\n%s", s.last)
	}
	s.assertTmpClean()
}

func TestHTMLPartialFailure(t *testing.T) {
	t.Parallel()
	skills, invalid := ideavimTotals(t)
	r := runHTML(t, requireTool(t, "true"), nil, ideavim.url+"#"+ideavim.tag, badURL)
	if r.code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr:\n%s", r.code, r.stderr)
	}
	if want := "skill-atlas: " + badDisplay + ": " + badError; !strings.Contains(r.stderr, want) {
		t.Errorf("stderr lacks %q:\n%s", want, r.stderr)
	}
	path := r.reportPath(t)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)
	for _, w := range []string{
		"<h1>2 repositories</h1>",
		fmt.Sprintf(">%d skills, %d invalid, 1 failed<", skills, invalid),
		badDisplay,
		badError,
	} {
		if !strings.Contains(page, w) {
			t.Errorf("report lacks %q", w)
		}
	}
	checkScriptPinned(t, page)
	assertOnlyReport(t, r.tmpDir, path)
}

func TestHTMLAllReposFail(t *testing.T) {
	t.Parallel()
	r := runHTML(t, requireTool(t, "true"), nil, badURL+"#no-such-ref-e2e", badURL)
	if r.code != 1 {
		t.Fatalf("exit code = %d, want 1; stderr:\n%s", r.code, r.stderr)
	}
	if reportLine.MatchString(r.stderr) {
		t.Errorf("a report was written:\n%s", r.stderr)
	}
	if entries, _ := os.ReadDir(r.tmpDir); len(entries) != 0 {
		t.Errorf("TMPDIR holds %d entries, want none", len(entries))
	}
}

func TestHTMLMultiRepo(t *testing.T) {
	t.Parallel()
	golden := readGolden(t, ideavim.golden)
	zoom := cloneScan(t, claudeSkills.url, claudeSkills.tag, scan.Options{}).skills
	total, invalid := multiTotals(t)

	r := runHTML(t, requireTool(t, "true"), nil, multiArgs()...)
	if r.code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", r.code, r.stderr)
	}
	path := r.reportPath(t)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)

	for _, w := range []string{
		"<h1>2 repositories</h1>",
		fmt.Sprintf(">%d skills, %d invalid<", total, invalid),
		fmt.Sprintf(`%s @ %s (<code title="%s">%s</code>)`, ideavim.display, ideavim.tag, ideavim.sha, ideavim.sha[:7]),
		fmt.Sprintf(`%s @ %s (<code title="%s">%s</code>)`, zoomDisplay, claudeSkills.tag, claudeSkills.sha, claudeSkills.sha[:7]),
		fmt.Sprintf(`id="repo-1-skill-%d"`, len(golden)),
		fmt.Sprintf(`id="repo-2-skill-%d"`, len(zoom)),
		zoom[0].Path,
	} {
		if !strings.Contains(page, w) {
			t.Errorf("report lacks %q", w)
		}
	}
	if strings.Contains(page, `id="skill-1"`) {
		t.Error("multi-repo report uses single-repo ids")
	}
	if n := strings.Count(page, "<section "); n != total {
		t.Errorf("report has %d skill sections, want %d", n, total)
	}
	checkScriptPinned(t, page)
	assertOnlyReport(t, r.tmpDir, path)
}
