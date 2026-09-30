package scan

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// Options controls which SKILL.md files Dir skips.
type Options struct {
	// IncludeTests turns off the default rule that skips skills under test directories.
	IncludeTests bool
	// Exclude holds gitignore-style patterns matched against slash-separated paths relative to the root.
	Exclude []string
}

// testDirNames are whole directory names that mark test data, compared case-insensitively.
var testDirNames = map[string]bool{
	"test": true, "tests": true, "testdata": true, "test-data": true, "test_data": true,
	"testfixtures": true, "__tests__": true, "fixtures": true, "__fixtures__": true,
}

// testDirSuffixes end a directory name, compared case-insensitively.
var testDirSuffixes = []string{"-test", "-tests", "_test", "_tests"}

// isTestDir reports whether a directory name marks test data.
func isTestDir(name string) bool {
	lower := strings.ToLower(name)
	if testDirNames[lower] {
		return true
	}
	for _, s := range testDirSuffixes {
		if strings.HasSuffix(lower, s) {
			return true
		}
	}
	// Kotlin and Gradle source sets: jvmTest, commonTests, integrationTest.
	for _, s := range []string{"Tests", "Test"} {
		if base, ok := strings.CutSuffix(name, s); ok && base != "" {
			r, _ := utf8.DecodeLastRuneInString(base)
			return unicode.IsLower(r) || unicode.IsDigit(r)
		}
	}
	return false
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
	tests   bool              // skip skills under test directories
	matcher gitignore.Matcher // nil without --exclude patterns
}

func newFilter(o Options) (*filter, error) {
	f := &filter{tests: !o.IncludeTests}
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
	parts := strings.Split(rel, "/")
	if f.matcher != nil && f.matcher.Match(parts, false) {
		return true
	}
	if !f.tests {
		return false
	}
	// parts ends with the skill's own directory and SKILL.md; only the ancestors above them are checked.
	if n := len(parts) - 2; n > 0 {
		for _, dir := range parts[:n] {
			if isTestDir(dir) {
				return true
			}
		}
	}
	return false
}
