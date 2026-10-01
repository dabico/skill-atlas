package repo

import (
	"archive/tar"
	"context"
	"fmt"
	"net"
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

// setTokens sets GITHUB_TOKEN and GH_TOKEN; "" means unset. fakeRemote and fakeAPI unset both,
// so call it after them.
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

// fakeGitHub serves every host name from handler: the HTTP client dials the test server whatever
// the host, so requests keep their real names (api.github.com, codeload.github.com) and the
// default tokenHosts apply. apiBase and archiveBase become plain-HTTP github.com URLs.
func fakeGitHub(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	addr := srv.Listener.Addr().String()
	oldAPI, oldArchive, oldClient := apiBase, archiveBase, httpClient
	t.Cleanup(func() { apiBase, archiveBase, httpClient = oldAPI, oldArchive, oldClient })
	apiBase, archiveBase = "http://api.github.com", "http://github.com"
	httpClient = &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, addr)
		},
	}}
}

// request is what a fake server saw.
type request struct{ host, path, auth string }

// reqLog records requests.
type reqLog struct {
	mu   sync.Mutex
	seen []request
}

func (l *reqLog) add(r *http.Request) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seen = append(l.seen, request{r.Host, r.URL.Path, r.Header.Get("Authorization")})
}

func (l *reqLog) all() []request {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]request(nil), l.seen...)
}

func (l *reqLog) auths() []string {
	var out []string
	for _, r := range l.all() {
		out = append(out, r.auth)
	}
	return out
}

func TestToken(t *testing.T) {
	tests := []struct{ name, github, gh, want, wantVar string }{
		{"none", "", "", "", ""},
		{"GITHUB_TOKEN", "a", "", "a", "GITHUB_TOKEN"},
		{"GH_TOKEN fallback", "", "b", "b", "GH_TOKEN"},
		{"GITHUB_TOKEN wins", "a", "b", "a", "GITHUB_TOKEN"},
		{"blank GITHUB_TOKEN falls back", "  \n", "b", "b", "GH_TOKEN"},
		{"trimmed", " a\n", "", "a", "GITHUB_TOKEN"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setTokens(t, tt.github, tt.gh)
			if got, name := token(); got != tt.want || name != tt.wantVar {
				t.Errorf("token() = %q, %q; want %q, %q", got, name, tt.want, tt.wantVar)
			}
		})
	}
}

var tokenCases = []struct{ name, github, gh, want string }{
	{"without token", "", "", ""},
	{"GITHUB_TOKEN", secret, "", "Bearer " + secret},
	{"GH_TOKEN fallback", "", "gh_tok", "Bearer gh_tok"},
	{"GITHUB_TOKEN wins", secret, "gh_tok", "Bearer " + secret},
}

func TestListOrgSendsToken(t *testing.T) {
	for _, tt := range tokenCases {
		t.Run(tt.name, func(t *testing.T) {
			var log reqLog
			fakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
				log.add(r)
				fmt.Fprint(w, repoJSON("a"))
			})
			setTokens(t, tt.github, tt.gh)
			trustTestHosts(t)
			if _, err := ListOrg(context.Background(), testOrg(t)); err != nil {
				t.Fatal(err)
			}
			if got := log.auths(); len(got) != 1 || got[0] != tt.want {
				t.Errorf("Authorization headers = %q, want [%q]", got, tt.want)
			}
		})
	}
}

func TestTokenOnlyToGitHubHosts(t *testing.T) {
	var log reqLog
	fakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		log.add(r)
		fmt.Fprint(w, repoJSON("a"))
	})
	setTokens(t, secret, "")
	// tokenHosts keeps its default: the test server on 127.0.0.1 isn't GitHub.
	if _, err := ListOrg(context.Background(), testOrg(t)); err != nil {
		t.Fatal(err)
	}
	if got := log.auths(); len(got) != 1 || got[0] != "" {
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

// Without a token the tarball comes from the github.com web route, with no Authorization header.
func TestDownloadWithoutTokenUsesWebRoute(t *testing.T) {
	data := tarGz(t, testSHA, entry{name: testPrefix + "a", typeflag: tar.TypeReg, body: "a"})
	var log reqLog
	fakeRemote(t, testRefs(), nil, nil)
	fakeGitHub(t, func(w http.ResponseWriter, r *http.Request) {
		log.add(r)
		switch r.Host {
		case "github.com":
			http.Redirect(w, r, "http://codeload.github.com/org/repo/tar.gz/"+testSHA, http.StatusFound)
		case "codeload.github.com":
			w.Write(data)
		default:
			http.NotFound(w, r)
		}
	})
	if _, err := Download(context.Background(), testTarget(t), "", t.TempDir(), nil); err != nil {
		t.Fatal(err)
	}
	want := []request{
		{"github.com", "/org/repo/archive/" + testSHA + ".tar.gz", ""},
		{"codeload.github.com", "/org/repo/tar.gz/" + testSHA, ""},
	}
	if got := log.all(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("requests = %v, want %v", got, want)
	}
}

// With a token the tarball comes from the REST API with the token. The redirect to codeload,
// whose URL carries its own token, doesn't get the header, and the web route is never used.
func TestDownloadWithTokenUsesAPI(t *testing.T) {
	data := tarGz(t, testSHA, entry{name: testPrefix + "a", typeflag: tar.TypeReg, body: "a"})
	for _, tt := range tokenCases[1:] {
		for _, codeload := range []string{"codeload.github.com", "codeload.api.github.com"} {
			t.Run(tt.name+" to "+codeload, func(t *testing.T) {
				var log reqLog
				var accept string
				fakeRemote(t, testRefs(), nil, nil)
				fakeGitHub(t, func(w http.ResponseWriter, r *http.Request) {
					log.add(r)
					switch r.Host {
					case "api.github.com":
						accept = r.Header.Get("Accept")
						http.Redirect(w, r, "http://"+codeload+"/org/repo/legacy.tar.gz/"+testSHA+"?token=short", http.StatusFound)
					case codeload:
						w.Write(data)
					default:
						http.NotFound(w, r)
					}
				})
				setTokens(t, tt.github, tt.gh)
				if _, err := Download(context.Background(), testTarget(t), "", t.TempDir(), nil); err != nil {
					t.Fatal(err)
				}
				// codeload.api.github.com is a subdomain, so Go would forward the header; do() drops it.
				want := []request{
					{"api.github.com", "/repos/org/repo/tarball/" + testSHA, tt.want},
					{codeload, "/org/repo/legacy.tar.gz/" + testSHA, ""},
				}
				if got := log.all(); fmt.Sprint(got) != fmt.Sprint(want) {
					t.Errorf("requests = %v, want %v", got, want)
				}
				if accept != "application/vnd.github+json" {
					t.Errorf("Accept = %q", accept)
				}
			})
		}
	}
}

func TestDownloadAPIStatus(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{"not found", http.NotFound, "repository github.com/org/repo not found"},
		{"bad token", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
			"GitHub rejected the token in GITHUB_TOKEN"},
		{"rate limit", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", "1790000000")
			w.WriteHeader(http.StatusForbidden)
		}, "GitHub API rate limit exceeded, resets at "},
		{"forbidden", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) },
			"download github.com/org/repo: 403 Forbidden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeRemote(t, testRefs(), nil, nil)
			fakeGitHub(t, tt.handler)
			setTokens(t, secret, "")
			_, err := Download(context.Background(), testTarget(t), "", t.TempDir(), nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
			if strings.Contains(err.Error(), "set GITHUB_TOKEN") || strings.Contains(err.Error(), secret) {
				t.Errorf("error %q has the hint or the token", err)
			}
		})
	}
}

// A redirect to an unrelated host never carries the token.
func TestTokenNotSentToUnrelatedHost(t *testing.T) {
	var other reqLog
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

	var first reqLog
	fakeRemote(t, testRefs(), nil, nil)
	fakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		first.add(r)
		http.Redirect(w, r, target, http.StatusFound)
	})
	setTokens(t, secret, "")
	trustTestHosts(t) // the first hop is trusted, the second is not
	if _, err := Download(context.Background(), testTarget(t), "", t.TempDir(), nil); err == nil {
		t.Fatal("download from a 404 succeeded")
	}
	if got := first.auths(); len(got) != 1 || got[0] != "Bearer "+secret {
		t.Fatalf("first hop got %q, want the token", got)
	}
	if got := other.auths(); len(got) != 1 || got[0] != "" {
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

// A 401 to a request that carried the token names the variable; without a token the errors stay.
func TestBadToken(t *testing.T) {
	unauthorized := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"message":"Bad credentials"}`)
	}
	tokens := []struct{ name, github, gh, want string }{
		{"GITHUB_TOKEN", secret, "", "GitHub rejected the token in GITHUB_TOKEN"},
		{"GH_TOKEN", "", secret, "GitHub rejected the token in GH_TOKEN"},
	}
	paths := []struct {
		name    string
		run     func(t *testing.T, github, gh string) error
		noToken string
	}{
		{"ls-remote", func(t *testing.T, github, gh string) error {
			fakeRemote(t, nil, transport.ErrAuthenticationRequired, nil)
			setTokens(t, github, gh)
			_, err := Download(context.Background(), testTarget(t), "", t.TempDir(), nil)
			return err
		}, "authentication failed for github.com/org/repo: the repository may be private or may not exist"},
		{"org listing", func(t *testing.T, github, gh string) error {
			fakeAPI(t, unauthorized)
			setTokens(t, github, gh)
			trustTestHosts(t)
			_, err := ListOrg(context.Background(), testOrg(t))
			return err
		}, "list repositories in github.com/acme: 401 Unauthorized"},
		{"archive", func(t *testing.T, github, gh string) error {
			fakeRemote(t, testRefs(), nil, nil)
			fakeGitHub(t, unauthorized)
			setTokens(t, github, gh)
			_, err := Download(context.Background(), testTarget(t), "", t.TempDir(), nil)
			return err
		}, "download github.com/org/repo: 401 Unauthorized"},
	}
	for _, p := range paths {
		for _, tok := range tokens {
			t.Run(p.name+" "+tok.name, func(t *testing.T) {
				err := p.run(t, tok.github, tok.gh)
				if err == nil || err.Error() != tok.want {
					t.Errorf("got %v, want %q", err, tok.want)
				}
			})
		}
		t.Run(p.name+" without token", func(t *testing.T) {
			err := p.run(t, "", "")
			if err == nil || err.Error() != p.noToken {
				t.Errorf("got %v, want %q", err, p.noToken)
			}
		})
	}
}

func TestTokenNotInErrors(t *testing.T) {
	check := func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("no error")
		}
		if strings.Contains(err.Error(), secret) {
			t.Errorf("error %q has the token", err)
		}
	}
	echo := func(status int) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			fmt.Fprint(w, "echo: "+r.Header.Get("Authorization"))
		}
	}

	t.Run("org listing", func(t *testing.T) {
		for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError} {
			fakeAPI(t, echo(status))
			setTokens(t, secret, "")
			trustTestHosts(t)
			_, err := ListOrg(context.Background(), testOrg(t))
			check(t, err)
		}
	})
	t.Run("rate limit", func(t *testing.T) {
		fakeAPI(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusForbidden)
		})
		setTokens(t, secret, "")
		trustTestHosts(t)
		_, err := ListOrg(context.Background(), testOrg(t))
		check(t, err)
	})
	t.Run("unreachable", func(t *testing.T) {
		fakeRemote(t, testRefs(), nil, nil)
		old := apiBase
		t.Cleanup(func() { apiBase = old })
		apiBase = "http://127.0.0.1:1"
		setTokens(t, secret, "")
		_, err := Download(context.Background(), testTarget(t), "", t.TempDir(), nil)
		check(t, err)
	})
	t.Run("archive status", func(t *testing.T) {
		for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError} {
			fakeRemote(t, testRefs(), nil, nil)
			fakeGitHub(t, echo(status))
			setTokens(t, secret, "")
			_, err := Download(context.Background(), testTarget(t), "", t.TempDir(), nil)
			check(t, err)
		}
	})
	t.Run("ls-remote auth failure", func(t *testing.T) {
		fakeRemote(t, nil, transport.ErrAuthenticationRequired, nil)
		setTokens(t, secret, "")
		_, err := Download(context.Background(), testTarget(t), "", t.TempDir(), nil)
		check(t, err)
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
			fakeAPI(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("X-RateLimit-Reset", "1790000000")
				w.WriteHeader(http.StatusForbidden)
			})
			setTokens(t, tt.token, "")
			trustTestHosts(t)
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
