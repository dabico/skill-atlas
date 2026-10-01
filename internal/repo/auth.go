package repo

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/go-git/go-git/v5"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// tokenHosts are the only hosts that receive the GitHub token. Tests replace it.
var tokenHosts = []string{githubHost, "api.github.com"}

// token returns the GitHub token from GITHUB_TOKEN, or else GH_TOKEN, and the name of the
// variable it came from. Both are "" when neither is set. The environment is read on every call,
// and the token is never written to an error, a warning or a log.
func token() (tok, name string) {
	for _, name := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v, name
		}
	}
	return "", ""
}

// errBadToken is the error for a token GitHub answered with 401. It names the variable, not the value.
func errBadToken(name string) error {
	return fmt.Errorf("GitHub rejected the token in %s", name)
}

// sentToken reports whether req carries the token, and the name of its variable.
func sentToken(req *http.Request) (bool, string) {
	if req.Header.Get("Authorization") == "" {
		return false, ""
	}
	_, name := token()
	return true, name
}

// tokenHost reports whether the token may be sent to the host of rawURL.
func tokenHost(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return slices.Contains(tokenHosts, strings.ToLower(u.Hostname()))
}

// authorize adds the token to req as a bearer token, when there is one and req goes to a GitHub
// host. It reports whether the header was set. The HTTP client drops the header on a redirect to
// a host outside the original domain, e.g. from api.github.com to codeload.github.com.
func authorize(req *http.Request) bool {
	tok, _ := token()
	if tok == "" || !tokenHost(req.URL.String()) {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return true
}

// listOptions are the ls-remote options for the remote URL. With a token and a GitHub host they carry
// basic auth, which is how GitHub takes a token over the Git protocol.
func listOptions(remote string) *git.ListOptions {
	opts := &git.ListOptions{PeelingOption: git.AppendPeeled}
	if tok, _ := token(); tok != "" && tokenHost(remote) {
		opts.Auth = &githttp.BasicAuth{Username: "x-access-token", Password: tok}
	}
	return opts
}

// sentListToken reports whether ls-remote for t sends the token.
func sentListToken(t Target) bool {
	return listOptions(t.Remote).Auth != nil
}

// do sends req with httpClient. A redirect to a host outside tokenHosts loses the token. Go drops
// it already for a host outside the original domain; this also covers other subdomains.
func do(req *http.Request) (*http.Response, error) {
	c := *httpClient
	next := c.CheckRedirect
	c.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if !tokenHost(r.URL.String()) {
			r.Header.Del("Authorization")
		}
		if next != nil {
			return next(r, via)
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}
		return nil
	}
	return c.Do(req)
}
