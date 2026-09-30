// Package scan finds skills in a checked-out repository.
package scan

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"skill-atlas/internal/skill"
)

// Result is the outcome of a scan.
type Result struct {
	Skills   []skill.Skill // sorted by path; nil when none were found
	Excluded int           // SKILL.md files skipped by Options, never parsed
}

// Dir walks root and parses every SKILL.md that opts don't exclude, sorted by path.
// rootName is used as the directory name for a SKILL.md at the root.
func Dir(root, rootName string, opts Options) (Result, error) {
	f, err := newFilter(opts)
	if err != nil {
		return Result{}, fmt.Errorf("scan %s: %w", root, err)
	}
	var res Result
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != skill.FileName || !d.Type().IsRegular() {
			return nil
		}

		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if f.excluded(rel) {
			res.Excluded++
			return nil
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		res.Skills = append(res.Skills, skill.Parse(rel, dirName(rel, rootName), content))
		return nil
	})
	if err != nil {
		return Result{}, fmt.Errorf("scan %s: %w", root, err)
	}
	slices.SortFunc(res.Skills, func(a, b skill.Skill) int { return strings.Compare(a.Path, b.Path) })
	return res, nil
}

// dirName returns the parent directory's base name, or rootName for a root-level file.
func dirName(rel, rootName string) string {
	parent := path.Dir(rel)
	if parent == "." {
		return rootName
	}
	return path.Base(parent)
}
