//go:build e2e

package e2e

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

var reportLine = regexp.MustCompile(`(?m)^Report: (http://127\.0\.0\.1:\d+/[0-9a-f]{32})$`)

// lockedBuffer is written by exec while the test polls it.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// htmlProc is one "skill-atlas scan --html" process. It has no TTY: stdin is /dev/null, stdout and stderr are pipes.
type htmlProc struct {
	cmd    *exec.Cmd
	stderr *lockedBuffer
	tmpDir string // TMPDIR handed to the binary
	done   chan struct{}
}

// startHTML runs "scan --html args..." with BROWSER=browser and extra env entries.
func startHTML(t *testing.T, browser string, env []string, args ...string) *htmlProc {
	t.Helper()
	p := &htmlProc{stderr: &lockedBuffer{}, tmpDir: mkdir(t, "tmpdir-"), done: make(chan struct{})}
	p.cmd = exec.Command(binPath, append([]string{"scan", "--html"}, args...)...)
	p.cmd.Env = append(os.Environ(), append([]string{"TMPDIR=" + p.tmpDir, "BROWSER=" + browser}, env...)...)
	p.cmd.Stdout = &lockedBuffer{}
	p.cmd.Stderr = p.stderr
	if err := p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		p.cmd.Wait()
		close(p.done)
	}()
	t.Cleanup(func() {
		p.cmd.Process.Kill()
		<-p.done
	})
	return p
}

// exit waits for the process and returns its exit code.
func (p *htmlProc) exit(t *testing.T, timeout time.Duration) int {
	t.Helper()
	select {
	case <-p.done:
	case <-time.After(timeout):
		t.Fatalf("process didn't exit within %s; stderr:\n%s", timeout, p.stderr)
	}
	return p.cmd.ProcessState.ExitCode()
}

// reportURL waits for the "Report: <url>" line on stderr.
func (p *htmlProc) reportURL(t *testing.T, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if m := reportLine.FindStringSubmatch(p.stderr.String()); m != nil {
			return m[1]
		}
		select {
		case <-p.done:
			t.Fatalf("process exited before printing the report url; stderr:\n%s", p.stderr)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("no report url within %s; stderr:\n%s", timeout, p.stderr)
		}
		time.Sleep(pollEvery)
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

// assertTmpDirClean fails when the binary left a skill-atlas-* entry in dir.
func assertTmpDirClean(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "skill-atlas-") {
			t.Errorf("leftover temp entry %s in TMPDIR", e.Name())
		}
	}
}

// assertServerGone fails when url still answers.
func assertServerGone(t *testing.T, url string) {
	t.Helper()
	c := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := c.Get(url)
	if err == nil {
		resp.Body.Close()
		t.Errorf("server still answers at %s: %s", url, resp.Status)
	}
}

// TestHTMLReport runs --html without a TTY and lets a curl script stand in for the browser.
func TestHTMLReport(t *testing.T) {
	t.Parallel()
	f := ideavim
	skills := readGolden(t, f.golden)
	curl := requireTool(t, "curl")

	saved := filepath.Join(mkdir(t, "html-"), "report.html")
	browser := filepath.Join(mkdir(t, "browser-"), "browser.sh")
	script := fmt.Sprintf("#!/bin/sh\n%s -fsS -o \"$E2E_REPORT.tmp\" \"$1\" && mv \"$E2E_REPORT.tmp\" \"$E2E_REPORT\"\n", shellQuote(curl))
	if err := os.WriteFile(browser, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	p := startHTML(t, browser, []string{"E2E_REPORT=" + saved}, "--ref", f.tag, f.url)
	if code := p.exit(t, cloneWait+shortWait); code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", code, p.stderr)
	}
	url := p.reportURL(t, shortWait)

	// The script may still be renaming its output when the binary exits.
	var page []byte
	deadline := time.Now().Add(10 * time.Second)
	for {
		var err error
		if page, err = os.ReadFile(saved); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the browser script saved no report; stderr:\n%s", p.stderr)
		}
		time.Sleep(pollEvery)
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

	assertTmpDirClean(t, p.tmpDir)
	assertServerGone(t, url)
}

// TestHTMLInterrupt sends SIGINT while the process waits for a browser that never loads the page.
func TestHTMLInterrupt(t *testing.T) {
	t.Parallel()
	noop := requireTool(t, "true")

	p := startHTML(t, noop, nil, "--ref", ideavim.tag, ideavim.url)
	url := p.reportURL(t, cloneWait)
	if err := p.cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	if code := p.exit(t, 30*time.Second); code != 130 {
		t.Errorf("exit code = %d, want 130; stderr:\n%s", code, p.stderr)
	}
	assertTmpDirClean(t, p.tmpDir)
	assertServerGone(t, url)
}
