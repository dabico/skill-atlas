package scan

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// Options controls which SKILL.md files Dir skips.
type Options struct {
	// Exclude holds gitignore-style patterns matched against slash-separated paths relative to the root.
	Exclude []string
}

// CheckPattern reports why p is not a usable exclude pattern, or nil.
func CheckPattern(p string) error {
	body := strings.TrimSpace(strings.TrimPrefix(p, "!"))
	if strings.Trim(body, "/") == "" {
		return errors.New("pattern is empty")
	}
	for seg := range strings.SplitSeq(body, "/") {
		if strings.Contains(seg, "**") && seg != "**" {
			return errors.New("** must be a whole path segment")
		}
		if _, err := path.Match(seg, ""); err != nil {
			return fmt.Errorf("malformed pattern: %w", err)
		}
	}
	return nil
}

// filter decides which SKILL.md paths Dir skips.
type filter struct {
	matcher gitignore.Matcher // nil without --exclude patterns
}

func newFilter(o Options) (*filter, error) {
	f := &filter{}
	if len(o.Exclude) == 0 {
		return f, nil
	}
	ps := make([]gitignore.Pattern, len(o.Exclude))
	for i, p := range o.Exclude {
		if err := CheckPattern(p); err != nil {
			return nil, fmt.Errorf("exclude %q: %w", p, err)
		}
		ps[i] = gitignore.ParsePattern(p, nil)
	}
	f.matcher = gitignore.NewMatcher(ps)
	return f, nil
}

// excluded reports whether the SKILL.md at rel (slash-separated) is skipped.
func (f *filter) excluded(rel string) bool {
	return f.matcher != nil && f.matcher.Match(strings.Split(rel, "/"), false)
}
