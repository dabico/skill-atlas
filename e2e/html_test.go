//go:build e2e

package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var reportLine = regexp.MustCompile(`(?m)^Report: (.+)$`)

// htmlRun is one finished "skill-atlas scan --html" process. It has no TTY: stdin is /dev/null, stdout and stderr are pipes.
type htmlRun struct {
	code   int
	stderr string
	tmpDir string // TMPDIR handed to the binary
}

// runHTML runs "scan --html args..." with BROWSER=browser, extra env entries and its own TMPDIR.
func runHTML(t *testing.T, browser string, env []string, args ...string) htmlRun {
	t.Helper()
	tmp := mkdir(t, "tmpdir-")
	cmd := exec.Command(binPath, append([]string{"scan", "--html"}, args...)...)
	cmd.Env = append(os.Environ(), append([]string{"TMPDIR=" + tmp, "BROWSER=" + browser}, env...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(downloadWait + shortWait):
		cmd.Process.Kill()
		<-done
		t.Fatalf("process didn't exit in time; stderr:\n%s", &stderr)
	}
	return htmlRun{code: cmd.ProcessState.ExitCode(), stderr: stderr.String(), tmpDir: tmp}
}

// reportPath returns the path from the "Report: " line on stderr.
func (r htmlRun) reportPath(t *testing.T) string {
	t.Helper()
	m := reportLine.FindStringSubmatch(r.stderr)
	if m == nil {
		t.Fatalf("no Report line on stderr:\n%s", r.stderr)
	}
	return m[1]
}

// assertOnlyReport fails unless TMPDIR holds the report file and nothing else, i.e. no download directory is left.
func assertOnlyReport(t *testing.T, dir, report string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), "skill-atlas-report-") || filepath.Base(report) != entries[0].Name() {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("TMPDIR holds %q, want only %s", names, filepath.Base(report))
	}
}

// TestHTMLReport runs --html without a TTY; a shell script stands in for the browser and copies the file it is given.
func TestHTMLReport(t *testing.T) {
	t.Parallel()
	f := ideavim
	skills := readGolden(t, f.golden)

	work := mkdir(t, "html-")
	saved, gotURL := filepath.Join(work, "report.html"), filepath.Join(work, "url")
	browser := filepath.Join(work, "browser.sh")
	script := "#!/bin/sh\nprintf '%s' \"$1\" > \"$E2E_URL\" && cp \"${1#file://}\" \"$E2E_REPORT\"\n"
	if err := os.WriteFile(browser, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	r := runHTML(t, browser, []string{"E2E_REPORT=" + saved, "E2E_URL=" + gotURL}, f.url+"#"+f.tag)
	if r.code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", r.code, r.stderr)
	}
	path := r.reportPath(t)
	if !filepath.IsAbs(path) {
		t.Errorf("report path %q isn't absolute", path)
	}
	if urlArg, _ := os.ReadFile(gotURL); string(urlArg) != "file://"+path {
		t.Errorf("browser got %q, want file://%s", urlArg, path)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("report file: %v", err)
	}
	page, err := os.ReadFile(saved)
	if err != nil {
		t.Fatalf("the browser script saved no report; stderr:\n%s", r.stderr)
	}
	if !bytes.Equal(page, onDisk) {
		t.Error("the copy the browser got differs from the file on disk")
	}
	html := string(page)

	invalid := 0
	for _, g := range skills {
		if !g.Valid {
			invalid++
		}
	}
	want := []string{
		f.display, f.tag, f.sha[:7], fmt.Sprintf(`title="%s"`, f.sha),
		fmt.Sprintf("%d skills, %d invalid", len(skills), invalid),
		`<link rel="icon" href="data:,">`,
	}
	for _, g := range skills {
		want = append(want, displayName(g), g.Path)
	}
	for _, w := range want {
		if !strings.Contains(html, w) {
			t.Errorf("report lacks %q", w)
		}
	}
	if n := strings.Count(html, "<section "); n != len(skills) {
		t.Errorf("report has %d skill sections, want %d", n, len(skills))
	}
	// Sections are in name order A–Z and numbered in that order.
	for i, g := range nameOrder(skills) {
		id := fmt.Sprintf("skill-%d", i+1)
		if sec := sectionText(html, id); !strings.Contains(sec, "<code>"+g.Path+"</code>") {
			t.Errorf("section %s isn't %s", id, g.Path)
		}
	}
	checkSortSelect(t, html)
	checkScriptPinned(t, html)
	assertOnlyReport(t, r.tmpDir, path)
}

// sectionText returns the markup of the skill section with the given id, or "".
func sectionText(page, id string) string {
	_, after, ok := strings.Cut(page, `<section class="skill" id="`+id+`"`)
	if !ok {
		return ""
	}
	sec, _, _ := strings.Cut(after, "</section>")
	return sec
}

// checkSortSelect asserts the page has the sort select inside the filter box, with A–Z selected.
func checkSortSelect(t *testing.T, page string) {
	t.Helper()
	const sel = `<select id="sort" aria-label="Sort skills" autocomplete="off">
<option value="asc" selected>Name A–Z</option>
<option value="desc">Name Z–A</option>
</select>`
	if n := strings.Count(page, "<select"); n != 1 {
		t.Errorf("report has %d select elements, want 1", n)
	}
	_, box, ok := strings.Cut(page, `<div class="filter" id="filter" hidden>`)
	if !ok {
		t.Fatal("report has no hidden filter box")
	}
	box, _, _ = strings.Cut(box, "</div>")
	if !strings.Contains(box, sel) {
		t.Errorf("filter box lacks the sort select %s:\n%s", sel, box)
	}
}

var scriptRE = regexp.MustCompile(`(?s)<script>(.*?)</script>`)

// checkScriptPinned asserts the page has 1 script and its CSP pins that script by hash.
func checkScriptPinned(t *testing.T, page string) {
	t.Helper()
	if n := strings.Count(strings.ToLower(page), "<script"); n != 1 {
		t.Fatalf("report has %d script elements, want 1", n)
	}
	m := scriptRE.FindStringSubmatch(page)
	if m == nil {
		t.Fatal("report has no plain <script> element")
	}
	sum := sha256.Sum256([]byte(m[1]))
	csp := fmt.Sprintf(`<meta http-equiv="Content-Security-Policy" content="default-src 'none'; script-src 'sha256-%s'; style-src 'unsafe-inline'; img-src data:">`,
		base64.StdEncoding.EncodeToString(sum[:]))
	if !strings.Contains(page, csp) {
		t.Errorf("report lacks the CSP %s", csp)
	}
}

// TestHTMLExclude runs --html with --exclude; the page counts the excluded skill like the TUI header.
func TestHTMLExclude(t *testing.T) {
	t.Parallel()
	f := ideavim
	const excluded = ".claude/skills/changelog/SKILL.md"
	invalid := 0
	var kept []goldenSkill
	for _, g := range readGolden(t, f.golden) {
		if g.Path == excluded {
			continue
		}
		kept = append(kept, g)
		if !g.Valid {
			invalid++
		}
	}
	if len(kept) != 5 {
		t.Fatalf("golden leaves %d skills, want 5", len(kept))
	}

	r := runHTML(t, requireTool(t, "true"), nil, "--exclude", ".claude/skills/changelog/", f.url+"#"+f.tag)
	if r.code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", r.code, r.stderr)
	}
	path := r.reportPath(t)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)

	if want := fmt.Sprintf(">5 skills, %d invalid, 1 excluded<", invalid); !strings.Contains(page, want) {
		t.Errorf("report lacks %q", want)
	}
	if n := strings.Count(page, "<section "); n != 5 {
		t.Errorf("report has %d skill sections, want 5", n)
	}
	if strings.Contains(page, excluded) {
		t.Errorf("report lists the excluded skill %s", excluded)
	}
	for _, g := range kept {
		if !strings.Contains(page, g.Path) {
			t.Errorf("report lacks %s", g.Path)
		}
	}
	checkScriptPinned(t, page)
	assertOnlyReport(t, r.tmpDir, path)
}

// TestHTMLBrowserFails checks that a browser that can't start or exits non-zero only produces a warning.
func TestHTMLBrowserFails(t *testing.T) {
	t.Parallel()
	failing := requireTool(t, "false")
	cases := map[string]string{
		"exits 1":         failing,
		"missing program": filepath.Join(mkdir(t, "nobrowser-"), "no-such-browser"),
	}
	for name, browser := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := runHTML(t, browser, nil, ideavim.url+"#"+ideavim.tag)
			if r.code != 0 {
				t.Fatalf("exit code = %d, want 0; stderr:\n%s", r.code, r.stderr)
			}
			path := r.reportPath(t)
			for _, w := range []string{"couldn't open a browser", "Open the report yourself: " + path} {
				if !strings.Contains(r.stderr, w) {
					t.Errorf("stderr lacks %q:\n%s", w, r.stderr)
				}
			}
			page, err := os.ReadFile(path)
			if err != nil || !strings.Contains(string(page), ideavim.display) {
				t.Errorf("report file unreadable or lacks the repo (err %v)", err)
			}
			assertOnlyReport(t, r.tmpDir, path)
		})
	}
}

func requireTool(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		if os.Getenv("CI") == "true" {
			t.Fatalf("%s not found (required when CI=true)", name)
		}
		t.Skipf("%s not found", name)
	}
	return p
}
