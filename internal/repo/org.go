package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// apiBase is the GitHub REST API. Tests replace it.
var apiBase = "https://api.github.com"

const (
	userAgent = "skill-atlas"
	// maxPage caps the size of 1 API response.
	maxPage = 32 << 20
)

// ListOrg returns the repositories of the organization org (a Target with Org set), sorted by
// full name, as the GitHub REST API lists them. Forks and archived repositories are included.
// With GITHUB_TOKEN or GH_TOKEN set it lists the private repositories the token can see as well;
// without a token only public ones. An organization without repositories is an error.
func ListOrg(ctx context.Context, org Target) ([]Target, error) {
	var out []Target
	next := apiBase + "/orgs/" + org.Owner + "/repos?type=all&sort=full_name&per_page=100"
	for next != "" {
		page, link, err := listPage(ctx, org, next)
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
		if next, err = nextPage(link); err != nil {
			return nil, fmt.Errorf("list repositories in %s: %w", org.Display, err)
		}
	}
	if len(out) == 0 {
		return nil, NoRepositories(org)
	}
	return out, nil
}

// NoRepositories is the error for an organization with nothing to scan.
func NoRepositories(org Target) error {
	return fmt.Errorf("no repositories found in %s", org.Display)
}

// apiRequest builds a GET request to the GitHub REST API.
func apiRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", userAgent)
	authorize(req)
	return req, nil
}

// listPage fetches 1 page of repositories and returns them with the Link header.
func listPage(ctx context.Context, org Target, url string) ([]Target, string, error) {
	req, err := apiRequest(ctx, url)
	if err != nil {
		return nil, "", fmt.Errorf("list repositories in %s: %w", org.Display, err)
	}
	resp, err := do(req)
	if err != nil {
		var netErr net.Error
		switch {
		case ctx.Err() != nil:
			return nil, "", fmt.Errorf("list repositories in %s: %w", org.Display, ctx.Err())
		case errors.As(err, &netErr):
			return nil, "", fmt.Errorf("can't reach %s: %w", strings.TrimPrefix(apiBase, "https://"), err)
		}
		return nil, "", fmt.Errorf("list repositories in %s: %w", org.Display, err)
	}
	defer resp.Body.Close()

	sent, name := sentToken(req)
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, "", fmt.Errorf("%s isn't a GitHub organization or doesn't exist", org.Display)
	case resp.StatusCode == http.StatusUnauthorized && sent:
		return nil, "", errBadToken(name)
	case rateLimited(resp):
		return nil, "", rateLimitError(resp.Header.Get("X-RateLimit-Reset"), sent)
	case resp.StatusCode != http.StatusOK:
		return nil, "", fmt.Errorf("list repositories in %s: %s", org.Display, resp.Status)
	}

	var repos []struct {
		FullName string `json:"full_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxPage)).Decode(&repos); err != nil {
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		return nil, "", fmt.Errorf("list repositories in %s: %w", org.Display, err)
	}
	out := make([]Target, 0, len(repos))
	for _, r := range repos {
		owner, name, ok := strings.Cut(r.FullName, "/")
		if !ok || !validPart(owner) || !validPart(name) {
			return nil, "", fmt.Errorf("list repositories in %s: unexpected repository name %q", org.Display, r.FullName)
		}
		t := repoTarget(owner, name)
		t.URL = "https://" + githubHost + "/" + owner + "/" + name
		out = append(out, t)
	}
	return out, resp.Header.Get("Link"), nil
}

// rateLimited reports whether resp is GitHub's answer to a request over the API rate limit.
func rateLimited(resp *http.Response) bool {
	return (resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests) &&
		resp.Header.Get("X-RateLimit-Remaining") == "0"
}

// rateLimitError names the local time when the limit resets, if the API sent it. Without a token
// it adds a hint, because a token raises the limit from 60 to 5,000 requests per hour.
func rateLimitError(reset string, hadToken bool) error {
	msg := "GitHub API rate limit exceeded"
	if sec, err := strconv.ParseInt(reset, 10, 64); err == nil {
		msg += ", resets at " + time.Unix(sec, 0).Local().Format("15:04")
	}
	if !hadToken {
		msg += "; set GITHUB_TOKEN to raise the limit"
	}
	return errors.New(msg)
}

// nextPage returns the rel="next" URL of a Link header, or "" on the last page.
// The URL must point at the API, so a later token is never sent anywhere else.
func nextPage(link string) (string, error) {
	for _, part := range strings.Split(link, ",") {
		target, params, _ := strings.Cut(part, ";")
		if strings.TrimSpace(params) != `rel="next"` {
			continue
		}
		u := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(target), "<"), ">")
		if !strings.HasPrefix(u, apiBase+"/") {
			return "", fmt.Errorf("next page %q isn't on %s", u, apiBase)
		}
		return u, nil
	}
	return "", nil
}
