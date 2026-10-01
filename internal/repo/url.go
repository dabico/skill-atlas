package repo

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport"
)

// githubHost is the only supported host.
const githubHost = "github.com"

var (
	winDrive = regexp.MustCompile(`^[A-Za-z]:[\\/]`)
	// pathPart is a GitHub owner or repository name.
	pathPart = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

var errLocalPath = errors.New("local paths aren't supported, pass a remote Git URL")

// ParseURL accepts a GitHub repository URL over HTTPS or SSH, or an organization URL over HTTPS
// (https://github.com/<org>), and rejects everything else. SSH URLs are mapped to their HTTPS form.
func ParseURL(raw string) (Target, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Target{}, errors.New("empty repository URL")
	}
	if isLocalPath(raw) {
		return Target{}, errLocalPath
	}

	ep, err := transport.NewEndpoint(raw)
	if err != nil {
		return Target{}, fmt.Errorf("invalid repository URL: %w", err)
	}

	switch ep.Protocol {
	case "https", "ssh":
	case "http":
		return Target{}, errors.New("http:// URLs aren't supported, only HTTPS and SSH URLs are")
	case "file":
		if strings.Contains(raw, "://") {
			return Target{}, errors.New(`unsupported scheme "file", only HTTPS and SSH URLs are supported`)
		}
		return Target{}, errLocalPath
	default:
		return Target{}, fmt.Errorf("unsupported scheme %q, only HTTPS and SSH URLs are supported", ep.Protocol)
	}

	if ep.Host == "" {
		return Target{}, errors.New("repository URL has no host")
	}
	if !strings.EqualFold(ep.Host, githubHost) {
		return Target{}, fmt.Errorf("unsupported host %q, only %s is supported", ep.Host, githubHost)
	}

	p := ep.Path
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	full := strings.Trim(p, "/")
	p = strings.Trim(strings.TrimSuffix(full, ".git"), "/")
	if p == "" {
		return Target{}, errors.New("repository URL has no repository path")
	}
	owner, name, ok := strings.Cut(p, "/")
	if !ok {
		return parseOrg(raw, ep.Protocol, full)
	}
	if strings.Contains(name, "/") || !validPart(owner) || !validPart(name) {
		return Target{}, fmt.Errorf("repository path %q isn't <owner>/<repo>", p)
	}
	t := repoTarget(owner, name)
	t.URL = raw
	return t, nil
}

// parseOrg checks the path of a URL that names only an owner, e.g. https://github.com/org.
func parseOrg(raw, protocol, owner string) (Target, error) {
	if !validPart(owner) || strings.HasSuffix(owner, ".git") {
		return Target{}, fmt.Errorf("path %q isn't <owner>/<repo> or <org>", owner)
	}
	if protocol != "https" {
		return Target{}, fmt.Errorf("SSH URLs can't name an organization, use https://%s/%s for an organization", githubHost, owner)
	}
	return Target{URL: raw, Display: githubHost + "/" + owner, Owner: owner, Org: true}, nil
}

// repoTarget is the Target of github.com/<owner>/<name>, without URL.
func repoTarget(owner, name string) Target {
	return Target{
		Remote:  "https://" + githubHost + "/" + owner + "/" + name + ".git",
		Display: githubHost + "/" + owner + "/" + name,
		Owner:   owner,
		Name:    name,
	}
}

func validPart(s string) bool {
	return pathPart.MatchString(s) && s != "." && s != ".."
}

func isLocalPath(s string) bool {
	return strings.HasPrefix(s, ".") || strings.HasPrefix(s, "/") ||
		strings.HasPrefix(s, "~") || winDrive.MatchString(s)
}
