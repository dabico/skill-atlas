package htmlreport

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// FilePrefix starts the name of every report file; the clone directory uses a different prefix.
const FilePrefix = "skill-atlas-report-"

// WriteFile writes page to a new file in the OS temp dir and returns its absolute path.
func WriteFile(page []byte) (string, error) {
	f, err := os.CreateTemp("", FilePrefix+"*.html") // mode 0600
	if err != nil {
		return "", err
	}
	if _, err := f.Write(page); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return filepath.Abs(f.Name())
}

// FileURL returns the file:// URL for the absolute path on this OS.
func FileURL(path string) string {
	return fileURL(filepath.Separator == '\\', path)
}

// fileURL builds the URL; windows means path is a drive path such as C:\dir\a.html.
func fileURL(windows bool, path string) string {
	if windows {
		path = strings.ReplaceAll(path, `\`, "/")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}
