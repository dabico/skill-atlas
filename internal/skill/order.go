package skill

import "strings"

// Compare orders skills by name A–Z for display: DisplayName ignoring case, then the exact DisplayName, then Path.
// Z–A is this order reversed. The TUI and the HTML report both use it, so they list skills the same way.
func Compare(a, b Skill) int {
	an, bn := a.DisplayName(), b.DisplayName()
	if c := strings.Compare(strings.ToLower(an), strings.ToLower(bn)); c != 0 {
		return c
	}
	if c := strings.Compare(an, bn); c != 0 {
		return c
	}
	return strings.Compare(a.Path, b.Path)
}

// CompareRepos orders repositories by name A–Z for display: the display name ignoring case, then the exact name, then the ref.
// Callers sort stably, so repositories that still tie keep the command-line order.
func CompareRepos(aName, aRef, bName, bRef string) int {
	if c := strings.Compare(strings.ToLower(aName), strings.ToLower(bName)); c != 0 {
		return c
	}
	if c := strings.Compare(aName, bName); c != 0 {
		return c
	}
	return strings.Compare(aRef, bRef)
}
