package repo

import (
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

// token returns the GitHub token from GITHUB_TOKEN, or else GH_TOKEN. It is "" when neither is
// set. The environment is read on every call, and the token is never written to an error,
// a warning or a log.
func token() string {
	if v := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("GH_TOKEN"))
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
// a host outside the original domain; codeload.github.com, a subdomain, keeps it.
func authorize(req *http.Request) bool {
	tok := token()
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
	if tok := token(); tok != "" && tokenHost(remote) {
		opts.Auth = &githttp.BasicAuth{Username: "x-access-token", Password: tok}
	}
	return opts
}
