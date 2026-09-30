// Package repo validates Git URLs and makes shallow clones with go-git.
package repo

// Target is a validated remote Git URL.
type Target struct {
	// URL is the URL as given by the user.
	URL string
	// Display is a short form for the TUI header, e.g. "github.com/org/repo".
	Display string
	// Name is the repository name, e.g. "repo". Used as the directory name of a root-level SKILL.md.
	Name string
	// SSH is true for ssh:// and scp-like (git@host:path) URLs.
	SSH bool
}

// Checkout describes what a clone checked out.
type Checkout struct {
	// Ref is the short branch or tag name, e.g. "main" or "v1.2.0".
	Ref string
	// SHA is the full commit SHA of the checked-out commit.
	SHA string
}
