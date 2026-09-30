//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

const (
	paneCols   = 120
	paneRows   = 40
	listCols   = 40 // listWidth at 120 columns
	pollEvery  = 100 * time.Millisecond
	cloneWait  = 3 * time.Minute
	shortWait  = 20 * time.Second
	sessionTag = "main"
)

var sockSeq atomic.Int64

// session is one skill-atlas process running inside a private tmux server.
type session struct {
	t        *testing.T
	sock     string // socket path under the scratch root, removed with it
	tmpDir   string // TMPDIR handed to the binary
	exitFile string
	last     string // last pane capture
}

func requireTmux(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("tmux")
	if err != nil {
		if os.Getenv("CI") == "true" {
			t.Fatal("tmux not found (required when CI=true)")
		}
		t.Skip("tmux not found")
	}
	return p
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// startScan runs "skill-atlas scan <args...>" in a 120x40 tmux pane.
func startScan(t *testing.T, args ...string) *session {
	t.Helper()
	requireTmux(t)
	s := &session{
		t:      t,
		sock:   filepath.Join(workDir, fmt.Sprintf("tmux-%d", sockSeq.Add(1))),
		tmpDir: mkdir(t, "tmpdir-"),
	}
	s.exitFile = filepath.Join(mkdir(t, "exit-"), "code")
	t.Cleanup(s.close)

	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = shellQuote(a)
	}
	// "trap : INT" keeps the wrapper shell alive on Ctrl+C so the exit code is recorded.
	script := fmt.Sprintf("trap : INT; TMPDIR=%s %s scan %s; echo $? > %s.tmp; mv %s.tmp %s; sleep 600",
		shellQuote(s.tmpDir), shellQuote(binPath), strings.Join(quoted, " "),
		shellQuote(s.exitFile), shellQuote(s.exitFile), shellQuote(s.exitFile))

	cmd := exec.Command("tmux", "-S", s.sock, "-f", "/dev/null", "new-session", "-d",
		"-x", strconv.Itoa(paneCols), "-y", strconv.Itoa(paneRows), "-s", sessionTag, "sh", "-c", script)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("tmux new-session: %v\n%s", err, out)
	}
	return s
}

func (s *session) tmux(args ...string) (string, error) {
	out, err := exec.Command("tmux", append([]string{"-S", s.sock, "-f", "/dev/null"}, args...)...).CombinedOutput()
	return string(out), err
}

// close saves the pane for failing tests, then kills the private tmux server.
func (s *session) close() {
	if s.t.Failed() {
		if out, err := s.tmux("capture-pane", "-p", "-J", "-t", sessionTag); err == nil {
			s.last = out
		}
		if dir := os.Getenv("E2E_ARTIFACTS_DIR"); dir != "" {
			name := strings.ReplaceAll(s.t.Name(), "/", "_") + ".txt"
			if os.MkdirAll(dir, 0o755) == nil {
				os.WriteFile(filepath.Join(dir, name), []byte(s.last), 0o644)
			}
		}
	}
	s.tmux("kill-server")
}

// capture returns the current pane text (wrapped lines joined).
func (s *session) capture() string {
	out, err := s.tmux("capture-pane", "-p", "-J", "-t", sessionTag)
	if err != nil {
		return s.last
	}
	s.last = out
	return out
}

// waitFor polls the pane until re matches and returns the pane text.
func (s *session) waitFor(re *regexp.Regexp, timeout time.Duration) string {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if out := s.capture(); re.MatchString(out) {
			return out
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("timed out after %s waiting for %v; last pane:\n%s", timeout, re, s.last)
		}
		time.Sleep(pollEvery)
	}
}

// waitUntil polls the pane until ok returns true.
func (s *session) waitUntil(what string, timeout time.Duration, ok func(pane string) bool) string {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if out := s.capture(); ok(out) {
			return out
		}
		if time.Now().After(deadline) {
			s.t.Fatalf("timed out after %s waiting for: %s; last pane:\n%s", timeout, what, s.last)
		}
		time.Sleep(pollEvery)
	}
}

// keys sends tmux key names (e.g. "j", "Tab", "Escape", "C-c").
func (s *session) keys(names ...string) {
	s.t.Helper()
	if out, err := s.tmux(append([]string{"send-keys", "-t", sessionTag}, names...)...); err != nil {
		s.t.Fatalf("send-keys %v: %v\n%s", names, err, out)
	}
}

func (s *session) typeText(text string) {
	s.t.Helper()
	if out, err := s.tmux("send-keys", "-t", sessionTag, "-l", text); err != nil {
		s.t.Fatalf("send-keys -l: %v\n%s", err, out)
	}
}

// waitExit polls the exit file and returns the recorded exit code.
func (s *session) waitExit(timeout time.Duration) int {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if data, err := os.ReadFile(s.exitFile); err == nil {
			code, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				s.t.Fatalf("bad exit file %q", data)
			}
			s.capture()
			return code
		}
		if time.Now().After(deadline) {
			s.capture()
			s.t.Fatalf("process didn't exit within %s; last pane:\n%s", timeout, s.last)
		}
		time.Sleep(pollEvery)
	}
}

// assertTmpClean fails when the binary left a skill-atlas-* entry in its TMPDIR.
func (s *session) assertTmpClean() {
	s.t.Helper()
	entries, err := os.ReadDir(s.tmpDir)
	if err != nil {
		s.t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "skill-atlas-") {
			s.t.Errorf("leftover temp entry %s in TMPDIR", e.Name())
		}
	}
}

// leftPane returns the list pane columns of a capture.
func leftPane(pane string) string {
	lines := strings.Split(pane, "\n")
	for i, l := range lines {
		if utf8.RuneCountInString(l) > listCols {
			l = string([]rune(l)[:listCols])
		}
		lines[i] = l
	}
	return strings.Join(lines, "\n")
}

// rightPane returns the detail pane columns of a capture.
func rightPane(pane string) string {
	lines := strings.Split(pane, "\n")
	for i, l := range lines {
		if r := []rune(l); len(r) > listCols {
			lines[i] = string(r[listCols:])
		} else {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

// namePrefix is the part of a skill name that stays visible in the list, even with the [invalid] badge.
func namePrefix(name string) string {
	if r := []rune(name); len(r) > 16 {
		return string(r[:16])
	}
	return name
}

// displayName mirrors skill.Skill.DisplayName for golden entries.
func displayName(g goldenSkill) string {
	if g.Name != "" {
		return g.Name
	}
	return g.Dir
}

// filterMatch mirrors the TUI filter: case-insensitive substring of name, description or path.
func filterMatch(g goldenSkill, q string) bool {
	q = strings.ToLower(q)
	for _, f := range []string{displayName(g), g.Description, g.Path} {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}

func TestTUI(t *testing.T) {
	t.Parallel()
	f := ideavim
	skills := readGolden(t, f.golden)
	invalid := 0
	for _, g := range skills {
		if !g.Valid {
			invalid++
		}
	}
	s := startScan(t, "--ref", f.tag, f.url)

	header := regexp.MustCompile(regexp.QuoteMeta(fmt.Sprintf("%s @ %s (%s)", f.display, f.tag, f.sha[:7])) +
		`\s+` + fmt.Sprintf("%d skills, %d invalid", len(skills), invalid))
	pane := s.waitFor(header, cloneWait)
	if !strings.Contains(leftPane(pane), namePrefix(displayName(skills[0]))) {
		t.Errorf("list doesn't show first skill %q:\n%s", displayName(skills[0]), pane)
	}
	if !strings.Contains(rightPane(pane), skills[0].Path) {
		t.Errorf("detail doesn't show first path %q:\n%s", skills[0].Path, pane)
	}

	// j moves the cursor and the detail pane follows.
	s.keys("j")
	pane = s.waitFor(regexp.MustCompile(regexp.QuoteMeta(skills[1].Path)), shortWait)
	if strings.Contains(rightPane(pane), skills[0].Path) {
		t.Errorf("detail still shows first skill after j:\n%s", pane)
	}

	// Tab focuses the detail pane, where j scrolls.
	s.keys("Tab")
	s.waitFor(regexp.MustCompile(`j/k scroll`), shortWait)
	s.keys("j", "j", "j", "j", "j")
	s.waitUntil("detail scrolled past the path line", shortWait, func(pane string) bool {
		return !strings.Contains(rightPane(pane), skills[1].Path)
	})
	s.keys("Tab")
	s.waitFor(regexp.MustCompile(`j/k move`), shortWait)

	// Filter to the skills matching query; the expected set comes from the golden data.
	const query = "dedup"
	matched := 0
	for _, g := range skills {
		if filterMatch(g, query) {
			matched++
		}
	}
	if matched == 0 || matched == len(skills) {
		t.Fatalf("query %q matches %d of %d skills; pick one that matches some", query, matched, len(skills))
	}
	counter := fmt.Sprintf("Skills %d/%d", matched, len(skills))
	s.keys("/")
	s.typeText(query)
	pane = s.waitFor(regexp.MustCompile(regexp.QuoteMeta(counter)), shortWait)
	left := leftPane(pane)
	for _, g := range skills {
		if shown, want := strings.Contains(left, namePrefix(displayName(g))), filterMatch(g, query); shown != want {
			t.Errorf("filter %q: %q shown = %v, want %v:\n%s", query, displayName(g), shown, want, pane)
		}
	}

	// Esc clears the filter.
	s.keys("Escape")
	deadline := time.Now().Add(shortWait)
	for {
		pane = s.capture()
		left = leftPane(pane)
		all := !strings.Contains(pane, counter)
		for _, g := range skills {
			all = all && strings.Contains(left, namePrefix(displayName(g)))
		}
		if all {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("filter not cleared; last pane:\n%s", pane)
		}
		time.Sleep(pollEvery)
	}

	s.keys("q")
	if code := s.waitExit(shortWait); code != 0 {
		t.Errorf("exit code = %d, want 0\n%s", code, s.last)
	}
	s.assertTmpClean()
}

func TestTUIErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"unknown-ref", []string{"--ref", "no-such-ref-e2e", ideavim.url},
			`ref "no-such-ref-e2e" not found in ` + ideavim.display},
		{"unknown-repo", []string{"https://github.com/JetBrains/this-repo-does-not-exist-e2e.git"},
			`authentication failed for github.com/JetBrains/this-repo-does-not-exist-e2e: the repository may be private or may not exist`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s := startScan(t, c.args...)
			if code := s.waitExit(2 * time.Minute); code != 1 {
				t.Errorf("exit code = %d, want 1\n%s", code, s.last)
			}
			if !strings.Contains(s.last, "skill-atlas: "+c.want) {
				t.Errorf("pane lacks %q:\n%s", c.want, s.last)
			}
			s.assertTmpClean()
		})
	}
}

func TestTUIInterrupt(t *testing.T) {
	t.Parallel()
	s := startScan(t, "--ref", ideavim.tag, ideavim.url)
	s.waitFor(regexp.MustCompile(regexp.QuoteMeta("Cloning "+ideavim.display+" @ "+ideavim.tag)), shortWait)
	// The clone takes seconds, so Ctrl+C sent right away lands mid-clone.
	s.keys("C-c")
	if code := s.waitExit(30 * time.Second); code != 130 {
		t.Errorf("exit code = %d, want 130\n%s", code, s.last)
	}
	s.assertTmpClean()
}
