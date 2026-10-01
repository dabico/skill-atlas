package repo

import (
	"strings"
	"testing"
)

func TestParseURL(t *testing.T) {
	gh := func(url string) Target {
		return Target{URL: url, Remote: "https://github.com/org/repo.git", Display: "github.com/org/repo", Owner: "org", Name: "repo"}
	}
	tests := []struct {
		in   string
		want Target
	}{
		{"https://github.com/org/repo.git", gh("https://github.com/org/repo.git")},
		{"https://github.com/org/repo", gh("https://github.com/org/repo")},
		{"  https://github.com/org/repo/  ", gh("https://github.com/org/repo/")},
		{"https://github.com/org/repo.git/", gh("https://github.com/org/repo.git/")},
		{"https://user:secret@github.com/org/repo", gh("https://user:secret@github.com/org/repo")},
		{"https://GitHub.COM/org/repo", gh("https://GitHub.COM/org/repo")},
		{"https://github.com:443/org/repo.git", gh("https://github.com:443/org/repo.git")},
		{"git@github.com:org/repo.git", gh("git@github.com:org/repo.git")},
		{"git@github.com:org/repo", gh("git@github.com:org/repo")},
		{"github.com:org/repo", gh("github.com:org/repo")},
		{"ssh://git@github.com/org/repo.git", gh("ssh://git@github.com/org/repo.git")},
		{"ssh://git@github.com/org/repo", gh("ssh://git@github.com/org/repo")},
		{"https://github.com/JetBrains/ideavim.git", Target{
			URL: "https://github.com/JetBrains/ideavim.git", Remote: "https://github.com/JetBrains/ideavim.git",
			Display: "github.com/JetBrains/ideavim", Owner: "JetBrains", Name: "ideavim",
		}},
		{"https://github.com/my-org/my.repo_v2", Target{
			URL: "https://github.com/my-org/my.repo_v2", Remote: "https://github.com/my-org/my.repo_v2.git",
			Display: "github.com/my-org/my.repo_v2", Owner: "my-org", Name: "my.repo_v2",
		}},
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
			for _, s := range []string{got.Display, got.Remote} {
				if strings.Contains(s, "secret") || strings.Contains(s, "@") {
					t.Errorf("%q leaks userinfo", s)
				}
			}
		})
	}
}

func TestParseURLOrg(t *testing.T) {
	org := func(url, owner string) Target {
		return Target{URL: url, Display: "github.com/" + owner, Owner: owner, Org: true}
	}
	tests := []struct {
		in   string
		want Target
	}{
		{"https://github.com/JetBrains", org("https://github.com/JetBrains", "JetBrains")},
		{"https://github.com/JetBrains/", org("https://github.com/JetBrains/", "JetBrains")},
		{"  https://github.com/agentskills  ", org("https://github.com/agentskills", "agentskills")},
		{"https://GitHub.com/my-org", org("https://GitHub.com/my-org", "my-org")},
		{"https://github.com/org?tab=repositories", org("https://github.com/org?tab=repositories", "org")},
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
		})
	}
}

// The duplicate check compares Display, so every form of 1 repository must give the same one.
func TestParseURLSameRepo(t *testing.T) {
	forms := []string{
		"https://github.com/org/repo",
		"https://github.com/org/repo.git",
		"git@github.com:org/repo.git",
		"ssh://git@github.com/org/repo",
		"https://GITHUB.com/org/repo/",
	}
	for _, f := range forms {
		got, err := ParseURL(f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if got.Display != "github.com/org/repo" || got.Remote != "https://github.com/org/repo.git" {
			t.Errorf("%s: display %q, remote %q", f, got.Display, got.Remote)
		}
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
		{"https://gitlab.com/org/repo.git", `unsupported host "gitlab.com", only github.com is supported`},
		{"https://gitlab.com:8443/a/b/c.git", `unsupported host "gitlab.com", only github.com is supported`},
		{"git@gitlab.com:org/repo.git", `unsupported host "gitlab.com"`},
		{"ssh://bob@example.com:2222/srv/repo.git", `unsupported host "example.com"`},
		{"https://www.github.com/org/repo", `unsupported host "www.github.com"`},
		{"https://github.com.evil.test/org/repo", `unsupported host "github.com.evil.test"`},
		{"https://127.0.0.1/org/repo", `unsupported host "127.0.0.1"`},
		{"https://github.com", "no repository path"},
		{"https://github.com/", "no repository path"},
		{"ssh://git@github.com/.git", "no repository path"},
		{"https://github.com/org.git", `path "org.git" isn't <owner>/<repo> or <org>`},
		{"https://github.com/my%20org", `path "my org" isn't <owner>/<repo> or <org>`},
		{"https://github.com/..", `isn't <owner>/<repo> or <org>`},
		{"git@github.com:org", "SSH URLs can't name an organization, use https://github.com/org for an organization"},
		{"git@github.com:org/", "use https://github.com/org for an organization"},
		{"ssh://git@github.com/org", "use https://github.com/org for an organization"},
		{"github.com:JetBrains", "use https://github.com/JetBrains for an organization"},
		{"http://github.com/org", "only HTTPS and SSH"},
		{"https://github.com/org/repo/tree/main", `repository path "org/repo/tree/main" isn't <owner>/<repo>`},
		{"https://github.com/a/b/c.git", `repository path "a/b/c" isn't <owner>/<repo>`},
		{"git@github.com:a/b/c.git", `repository path "a/b/c" isn't <owner>/<repo>`},
		{"https://github.com/org//repo", "isn't <owner>/<repo>"},
		{"https://github.com/../repo", "isn't <owner>/<repo>"},
		{"https://github.com/org/..", "isn't <owner>/<repo>"},
		{"https://github.com/org/re%20po", "isn't <owner>/<repo>"},
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
