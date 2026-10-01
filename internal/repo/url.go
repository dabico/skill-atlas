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

// ParseURL accepts a GitHub repository URL over HTTPS or SSH and rejects everything else.
// SSH URLs are mapped to their HTTPS form.
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
	p = strings.TrimSuffix(strings.Trim(p, "/"), ".git")
	p = strings.Trim(p, "/")
	if p == "" {
		return Target{}, errors.New("repository URL has no repository path")
	}
	owner, name, ok := strings.Cut(p, "/")
	if !ok || strings.Contains(name, "/") {
		return Target{}, fmt.Errorf("repository path %q isn't <owner>/<repo>", p)
	}
	for _, part := range []string{owner, name} {
		if !pathPart.MatchString(part) || part == "." || part == ".." {
			return Target{}, fmt.Errorf("repository path %q isn't <owner>/<repo>", p)
		}
	}

	return Target{
		URL:     raw,
		Remote:  "https://" + githubHost + "/" + owner + "/" + name + ".git",
		Display: githubHost + "/" + owner + "/" + name,
		Owner:   owner,
		Name:    name,
	}, nil
}

func isLocalPath(s string) bool {
	return strings.HasPrefix(s, ".") || strings.HasPrefix(s, "/") ||
		strings.HasPrefix(s, "~") || winDrive.MatchString(s)
}
