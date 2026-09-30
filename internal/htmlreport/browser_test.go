package htmlreport

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"
)

func TestBrowserCommand(t *testing.T) {
	const u = "file:///tmp/skill-atlas-report-1.html"
	tests := []struct {
		name     string
		goos     string
		env      string
		wantName string
		wantArgs []string
	}{
		{"darwin", "darwin", "", "open", []string{u}},
		{"windows", "windows", "", "rundll32", []string{"url.dll,FileProtocolHandler", u}},
		{"linux", "linux", "", "xdg-open", []string{u}},
		{"freebsd", "freebsd", "", "xdg-open", []string{u}},
		{"unknown", "plan9", "", "xdg-open", []string{u}},
		{"env on darwin", "darwin", "/opt/b/browse", "/opt/b/browse", []string{u}},
		{"env on windows", "windows", `C:\b\browse.exe`, `C:\b\browse.exe`, []string{u}},
		{"env on linux", "linux", "firefox", "firefox", []string{u}},
		{"env is one program, not a command line", "linux", "/opt/my browser/run --flag", "/opt/my browser/run --flag", []string{u}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, args := browserCommand(tt.goos, tt.env, u)
			if name != tt.wantName || !slices.Equal(args, tt.wantArgs) {
				t.Errorf("browserCommand(%q, %q) = %q %q, want %q %q", tt.goos, tt.env, name, args, tt.wantName, tt.wantArgs)
			}
		})
	}
}

// TestOpenBrowserUsesEnv runs a stand-in for $BROWSER; no real browser starts.
func TestOpenBrowserUsesEnv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell script")
	}
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	script := filepath.Join(dir, "fake-browser")
	body := "#!/bin/sh\nprintf '%s\\n' \"$#\" \"$1\" > \"$FAKE_OUT.tmp\" && mv \"$FAKE_OUT.tmp\" \"$FAKE_OUT\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BROWSER", script)
	t.Setenv("FAKE_OUT", out)

	const u = "file:///tmp/a%20b/r.html?x=1&y=2;echo hacked"
	if err := OpenBrowser(u); err != nil {
		t.Fatal(err)
	}
	var got []byte
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		var err error
		if got, err = os.ReadFile(out); err == nil {
			break
		}
	}
	if want := "1\n" + u + "\n"; string(got) != want {
		t.Errorf("browser saw %q, want %q (the url as its only argument)", got, want)
	}
}

func TestOpenBrowserMissingProgram(t *testing.T) {
	t.Setenv("BROWSER", filepath.Join(t.TempDir(), "no-such-browser"))
	if err := OpenBrowser("file:///tmp/x.html"); err == nil {
		t.Error("OpenBrowser returned nil for a program that doesn't exist")
	}
}

// script writes an executable shell script and returns its path.
func script(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell script")
	}
	p := filepath.Join(t.TempDir(), "fake")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLaunch(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wait    time.Duration
		wantErr bool
		maxTook time.Duration
	}{
		{"exits 0", "exit 0", 10 * time.Second, false, 5 * time.Second},
		{"exits 1", "exit 1", 10 * time.Second, true, 5 * time.Second},
		{"still running counts as launched", "sleep 30", 100 * time.Millisecond, false, 5 * time.Second},
		{"exits 1 after the window counts as launched", "sleep 1; exit 1", 100 * time.Millisecond, false, 900 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Now()
			err := launch(script(t, tt.body), nil, tt.wait)
			if (err != nil) != tt.wantErr {
				t.Errorf("launch error = %v, wantErr %v", err, tt.wantErr)
			}
			if took := time.Since(start); took > tt.maxTook {
				t.Errorf("launch took %s, want under %s", took, tt.maxTook)
			}
		})
	}
	t.Run("missing program", func(t *testing.T) {
		if err := launch(filepath.Join(t.TempDir(), "nope"), nil, time.Second); err == nil {
			t.Error("launch returned nil for a missing program")
		}
	})
}
