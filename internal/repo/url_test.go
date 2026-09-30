package repo

import (
	"strings"
	"testing"
)

func TestParseURL(t *testing.T) {
	tests := []struct {
		in   string
		want Target
	}{
		{"https://github.com/org/repo.git", Target{URL: "https://github.com/org/repo.git", Display: "github.com/org/repo", Name: "repo"}},
		{"  https://github.com/org/repo/  ", Target{URL: "https://github.com/org/repo/", Display: "github.com/org/repo", Name: "repo"}},
		{"https://user:secret@github.com/org/repo", Target{URL: "https://user:secret@github.com/org/repo", Display: "github.com/org/repo", Name: "repo"}},
		{"https://gitlab.com:8443/a/b/c.git", Target{URL: "https://gitlab.com:8443/a/b/c.git", Display: "gitlab.com/a/b/c", Name: "c"}},
		{"git@github.com:org/repo.git", Target{URL: "git@github.com:org/repo.git", Display: "github.com/org/repo", Name: "repo", SSH: true}},
		{"github.com:org/repo", Target{URL: "github.com:org/repo", Display: "github.com/org/repo", Name: "repo", SSH: true}},
		{"ssh://git@github.com/org/repo.git", Target{URL: "ssh://git@github.com/org/repo.git", Display: "github.com/org/repo", Name: "repo", SSH: true}},
		{"ssh://bob@example.com:2222/srv/repo.git", Target{URL: "ssh://bob@example.com:2222/srv/repo.git", Display: "example.com/srv/repo", Name: "repo", SSH: true}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseURL(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
			if strings.Contains(got.Display, "secret") || strings.Contains(got.Display, "@") {
				t.Errorf("display leaks userinfo: %q", got.Display)
			}
		})
	}
}

func TestParseURLErrors(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", "empty"},
		{"   ", "empty"},
		{"http://github.com/org/repo", "only HTTPS and SSH"},
		{"git://github.com/org/repo.git", `"git"`},
		{"file:///tmp/repo", `"file"`},
		{"ftp://example.com/repo", `"ftp"`},
		{"./repo", "local paths aren't supported"},
		{"/tmp/repo", "local paths aren't supported"},
		{"~/repo", "local paths aren't supported"},
		{`C:\repo`, "local paths aren't supported"},
		{"repo", "local paths aren't supported"},
		{"https:///org/repo", "no host"},
		{"https://github.com", "no repository path"},
		{"https://github.com/", "no repository path"},
		{"ssh://git@github.com/.git", "no repository path"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			_, err := ParseURL(tt.in)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q doesn't contain %q", err, tt.want)
			}
		})
	}
}
