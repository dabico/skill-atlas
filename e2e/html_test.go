//go:build e2e

package e2e

import (
	"bytes"
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
	case <-time.After(cloneWait + shortWait):
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

// assertOnlyReport fails unless TMPDIR holds the report file and nothing else, i.e. no clone directory is left.
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

	r := runHTML(t, browser, []string{"E2E_REPORT=" + saved, "E2E_URL=" + gotURL}, "--ref", f.tag, f.url)
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
		`<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src data:">`,
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
	if strings.Contains(strings.ToLower(html), "<script") {
		t.Error("report has a script element")
	}
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
			r := runHTML(t, browser, nil, "--ref", ideavim.tag, ideavim.url)
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
