// Package repo validates GitHub repository and organization URLs, lists the repositories of an
// organization and downloads the tarball of 1 commit, keeping it in the archive cache.
package repo

// Target is a validated GitHub repository or organization URL.
type Target struct {
	// URL is the URL as given by the user.
	URL string
	// Remote is the HTTPS URL that refs are listed from, e.g. "https://github.com/org/repo.git".
	// SSH URLs map to the same HTTPS URL.
	Remote string
	// Display is a short form for the TUI header, e.g. "github.com/org/repo".
	Display string
	// Owner is the GitHub user or organization, e.g. "org".
	Owner string
	// Name is the repository name, e.g. "repo". Used as the directory name of a root-level SKILL.md.
	Name string
	// Org is true for an organization URL, e.g. "https://github.com/org". Remote and Name are empty
	// then, and Display is "github.com/org".
	Org bool
}

// Checkout describes what a download wrote.
type Checkout struct {
	// Ref is the short branch or tag name, e.g. "main" or "v1.2.0".
	Ref string
	// SHA is the full commit SHA of the downloaded commit.
	SHA string
}
