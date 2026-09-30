package repo

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/transport"
)

var winDrive = regexp.MustCompile(`^[A-Za-z]:[\\/]`)

var errLocalPath = errors.New("local paths aren't supported, pass a remote Git URL")

// ParseURL accepts a remote Git URL over HTTPS or SSH and rejects everything else.
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

	p := ep.Path
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	p = strings.TrimSuffix(strings.Trim(p, "/"), ".git")
	p = strings.Trim(p, "/")
	if p == "" {
		return Target{}, errors.New("repository URL has no repository path")
	}

	return Target{
		URL:     raw,
		Display: ep.Host + "/" + p,
		Name:    path.Base(p),
		SSH:     ep.Protocol == "ssh",
	}, nil
}

func isLocalPath(s string) bool {
	return strings.HasPrefix(s, ".") || strings.HasPrefix(s, "/") ||
		strings.HasPrefix(s, "~") || winDrive.MatchString(s)
}
