package repo

import (
	"archive/tar"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

const secret = "ghp_s3cr3t"

// setTokens sets GITHUB_TOKEN and GH_TOKEN; "" means unset.
func setTokens(t *testing.T, github, gh string) {
	t.Helper()
	t.Setenv("GITHUB_TOKEN", github)
	t.Setenv("GH_TOKEN", gh)
}

// trustTestHosts lets the token go to 127.0.0.1, where httptest servers listen.
func trustTestHosts(t *testing.T) {
	t.Helper()
	old := tokenHosts
	t.Cleanup(func() { tokenHosts = old })
	tokenHosts = []string{"127.0.0.1"}
}

// authLog records the Authorization header of every request.
type authLog struct {
	mu   sync.Mutex
	seen []string
}

func (l *authLog) add(r *http.Request) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seen = append(l.seen, r.Header.Get("Authorization"))
}

func (l *authLog) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.seen...)
}

func TestToken(t *testing.T) {
	tests := []struct{ name, github, gh, want string }{
		{"none", "", "", ""},
		{"GITHUB_TOKEN", "a", "", "a"},
		{"GH_TOKEN fallback", "", "b", "b"},
		{"GITHUB_TOKEN wins", "a", "b", "a"},
		{"blank GITHUB_TOKEN falls back", "  \n", "b", "b"},
		{"trimmed", " a\n", "", "a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setTokens(t, tt.github, tt.gh)
			if got := token(); got != tt.want {
				t.Errorf("token() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestListOrgSendsToken(t *testing.T) {
	tests := []struct{ name, github, gh, want string }{
		{"without token", "", "", ""},
		{"GITHUB_TOKEN", secret, "", "Bearer " + secret},
		{"GH_TOKEN fallback", "", "gh_tok", "Bearer gh_tok"},
		{"GITHUB_TOKEN wins", secret, "gh_tok", "Bearer " + secret},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setTokens(t, tt.github, tt.gh)
			trustTestHosts(t)
			var log authLog
			fakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
				log.add(r)
				fmt.Fprint(w, repoJSON("a"))
			})
			if _, err := ListOrg(context.Background(), testOrg(t)); err != nil {
				t.Fatal(err)
			}
			if got := log.all(); len(got) != 1 || got[0] != tt.want {
				t.Errorf("Authorization headers = %q, want [%q]", got, tt.want)
			}
		})
	}
}

func TestTokenOnlyToGitHubHosts(t *testing.T) {
	setTokens(t, secret, "")
	// tokenHosts keeps its default: the test servers on 127.0.0.1 aren't GitHub.
	var log authLog
	fakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		log.add(r)
		fmt.Fprint(w, repoJSON("a"))
	})
	if _, err := ListOrg(context.Background(), testOrg(t)); err != nil {
		t.Fatal(err)
	}
	if got := log.all(); len(got) != 1 || got[0] != "" {
		t.Errorf("Authorization headers = %q, want none", got)
	}

	for _, u := range []string{
		"https://github.com/o/r", "https://GitHub.com/o/r", "https://api.github.com/orgs/o/repos",
	} {
		if !tokenHost(u) {
			t.Errorf("tokenHost(%q) = false", u)
		}
	}
	for _, u := range []string{
		"https://evil.test/github.com", "https://github.com.evil.test/o/r", "https://notgithub.com/o/r",
		"https://codeload.github.com/o/r", "http://[::1", "",
	} {
		if tokenHost(u) {
			t.Errorf("tokenHost(%q) = true", u)
		}
	}
}

func TestDownloadSendsToken(t *testing.T) {
	tests := []struct{ name, github, gh, want string }{
		{"without token", "", "", ""},
		{"GITHUB_TOKEN", secret, "", "Bearer " + secret},
		{"GH_TOKEN fallback", "", "gh_tok", "Bearer gh_tok"},
		{"GITHUB_TOKEN wins", secret, "gh_tok", "Bearer " + secret},
	}
	data := tarGz(t, testSHA, entry{name: testPrefix + "a", typeflag: tar.TypeReg, body: "a"})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setTokens(t, tt.github, tt.gh)
			trustTestHosts(t)
			var log authLog
			serveData := serve(data, nil)
			fakeRemote(t, testRefs(), nil, func(w http.ResponseWriter, r *http.Request) {
				log.add(r)
				serveData(w, r)
			})
			if _, err := Download(context.Background(), testTarget(t), "", t.TempDir(), nil); err != nil {
				t.Fatal(err)
			}
			if got := log.all(); len(got) != 1 || got[0] != tt.want {
				t.Errorf("Authorization headers = %q, want [%q]", got, tt.want)
			}
		})
	}
}

func TestDownloadTokenNotSentToUnrelatedHost(t *testing.T) {
	setTokens(t, secret, "")
	var other authLog
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		other.add(r)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(elsewhere.Close)
	// The same machine under another name, so the client sees an unrelated host.
	u, err := url.Parse(elsewhere.URL)
	if err != nil {
		t.Fatal(err)
	}
	target := "http://localhost:" + u.Port() + "/loot"

	var first authLog
	fakeRemote(t, testRefs(), nil, func(w http.ResponseWriter, r *http.Request) {
		first.add(r)
		http.Redirect(w, r, target, http.StatusFound)
	})
	trustTestHosts(t) // the first hop is trusted, the second is not
	if _, err := Download(context.Background(), testTarget(t), "", t.TempDir(), nil); err == nil {
		t.Fatal("download from a 404 succeeded")
	}
	if got := first.all(); len(got) != 1 || got[0] != "Bearer "+secret {
		t.Fatalf("first hop got %q, want the token", got)
	}
	if got := other.all(); len(got) != 1 || got[0] != "" {
		t.Errorf("redirect target got Authorization %q, want none", got)
	}
}

func TestListOptions(t *testing.T) {
	setTokens(t, "", "")
	if got := listOptions("https://github.com/o/r").Auth; got != nil {
		t.Errorf("Auth without token = %v, want nil", got)
	}

	setTokens(t, secret, "")
	auth, ok := listOptions("https://github.com/o/r").Auth.(*githttp.BasicAuth)
	if !ok || auth.Username != "x-access-token" || auth.Password != secret {
		t.Errorf("Auth = %#v, want basic auth x-access-token with the token", auth)
	}
	if got := listOptions("https://gitlab.com/o/r").Auth; got != nil {
		t.Errorf("Auth for another host = %v, want nil", got)
	}

	setTokens(t, "", "gh_tok")
	auth, ok = listOptions("https://github.com/o/r").Auth.(*githttp.BasicAuth)
	if !ok || auth.Password != "gh_tok" {
		t.Errorf("GH_TOKEN fallback: Auth = %#v", auth)
	}
}

func TestTokenNotInErrors(t *testing.T) {
	setTokens(t, secret, "")
	trustTestHosts(t)
	check := func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("no error")
		}
		if strings.Contains(err.Error(), secret) {
			t.Errorf("error %q has the token", err)
		}
	}

	t.Run("org listing", func(t *testing.T) {
		for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError} {
			fakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				fmt.Fprint(w, "echo: "+r.Header.Get("Authorization"))
			})
			_, err := ListOrg(context.Background(), testOrg(t))
			check(t, err)
		}
	})
	t.Run("rate limit", func(t *testing.T) {
		fakeAPI(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusForbidden)
		})
		_, err := ListOrg(context.Background(), testOrg(t))
		check(t, err)
	})
	t.Run("unreachable", func(t *testing.T) {
		fakeRemote(t, testRefs(), nil, nil)
		archiveBase = "http://127.0.0.1:1"
		_, err := Download(context.Background(), testTarget(t), "", t.TempDir(), nil)
		check(t, err)
	})
	t.Run("archive status", func(t *testing.T) {
		fakeRemote(t, testRefs(), nil, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		})
		_, err := Download(context.Background(), testTarget(t), "", t.TempDir(), nil)
		check(t, err)
	})
	t.Run("ls-remote auth failure", func(t *testing.T) {
		fakeRemote(t, nil, transport.ErrAuthenticationRequired, nil)
		_, err := Download(context.Background(), testTarget(t), "", t.TempDir(), nil)
		check(t, err)
		if !strings.Contains(err.Error(), "authentication failed for") {
			t.Errorf("got %q", err)
		}
	})
}

func TestRateLimitHint(t *testing.T) {
	const hint = "set GITHUB_TOKEN to raise the limit"
	for _, reset := range []string{"1790000000", ""} {
		withToken := rateLimitError(reset, true).Error()
		without := rateLimitError(reset, false).Error()
		if strings.Contains(withToken, hint) {
			t.Errorf("reset %q: error with a token has the hint: %q", reset, withToken)
		}
		if !strings.Contains(without, hint) {
			t.Errorf("reset %q: error without a token lacks the hint: %q", reset, without)
		}
		if !strings.HasPrefix(without, "GitHub API rate limit exceeded") || !strings.HasPrefix(withToken, "GitHub API rate limit exceeded") {
			t.Errorf("reset %q: prefix changed: %q, %q", reset, withToken, without)
		}
		if (reset != "") != strings.Contains(without, ", resets at ") {
			t.Errorf("reset %q: reset time wrong in %q", reset, without)
		}
	}
}

func TestListOrgRateLimitHint(t *testing.T) {
	tests := []struct {
		name, token string
		hint        bool
	}{
		{"without token", "", true},
		{"with token", secret, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setTokens(t, tt.token, "")
			trustTestHosts(t)
			fakeAPI(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("X-RateLimit-Reset", "1790000000")
				w.WriteHeader(http.StatusForbidden)
			})
			_, err := ListOrg(context.Background(), testOrg(t))
			if err == nil || !strings.Contains(err.Error(), "GitHub API rate limit exceeded, resets at ") {
				t.Fatalf("got %v", err)
			}
			if got := strings.Contains(err.Error(), "set GITHUB_TOKEN to raise the limit"); got != tt.hint {
				t.Errorf("hint present = %v, want %v: %q", got, tt.hint, err)
			}
		})
	}
}
