package htmlreport

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)

	page := []byte("<!doctype html><title>x</title>")
	path, err := WriteFile(page)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(path) {
		t.Errorf("path %q isn't absolute", path)
	}
	if !samePath(filepath.Dir(path), tmp) {
		t.Errorf("path %q isn't in the temp dir %q", path, tmp)
	}
	name := filepath.Base(path)
	if !strings.HasPrefix(name, FilePrefix) || !strings.HasSuffix(name, ".html") {
		t.Errorf("name %q, want prefix %q and suffix .html", name, FilePrefix)
	}
	if strings.HasPrefix(name, "skill-atlas-") && !strings.HasPrefix(name, "skill-atlas-report-") {
		t.Errorf("name %q could be mistaken for the download directory", name)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(page) {
		t.Errorf("file holds %q (err %v), want %q", got, err, page)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := fi.Mode().Perm(); perm&0o077 != 0 {
			t.Errorf("mode %v is open to group or others", perm)
		}
	}

	other, err := WriteFile(page)
	if err != nil {
		t.Fatal(err)
	}
	if other == path {
		t.Error("two reports share one file")
	}
}

func TestWriteFileBadTempDir(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "missing")
	t.Setenv("TMPDIR", bad)
	t.Setenv("TMP", bad)
	t.Setenv("TEMP", bad)
	if p, err := WriteFile([]byte("x")); err == nil {
		t.Errorf("WriteFile succeeded at %q with a missing temp dir", p)
	}
}

// samePath compares two directories after resolving symlinks (macOS /var vs /private/var).
func samePath(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	return err1 == nil && err2 == nil && ra == rb
}

func TestFileURL(t *testing.T) {
	tests := []struct {
		name    string
		windows bool
		path    string
		want    string
	}{
		{"plain", false, "/tmp/skill-atlas-report-1.html", "file:///tmp/skill-atlas-report-1.html"},
		{"spaces", false, "/tmp/my dir/r.html", "file:///tmp/my%20dir/r.html"},
		{"special characters", false, "/tmp/a#b?c%d&e;f/r.html", "file:///tmp/a%23b%3Fc%25d&e;f/r.html"},
		{"non-ASCII", false, "/tmp/é/r.html", "file:///tmp/%C3%A9/r.html"},
		{"windows drive path", true, `C:\Users\me\AppData\Local\Temp\skill-atlas-report-1.html`, "file:///C:/Users/me/AppData/Local/Temp/skill-atlas-report-1.html"},
		{"windows spaces", true, `C:\Users\Jane Doe\Temp\r.html`, "file:///C:/Users/Jane%20Doe/Temp/r.html"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fileURL(tt.windows, tt.path)
			if got != tt.want {
				t.Errorf("fileURL(%v, %q) = %q, want %q", tt.windows, tt.path, got, tt.want)
			}
			u, err := url.Parse(got)
			if err != nil {
				t.Fatal(err)
			}
			want := tt.path
			if tt.windows {
				want = "/" + strings.ReplaceAll(tt.path, `\`, "/")
			}
			if u.Scheme != "file" || u.Host != "" || u.Path != want {
				t.Errorf("parsed back to %q %q %q, want file, empty host, %q", u.Scheme, u.Host, u.Path, want)
			}
		})
	}
}
