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

// Dir walks root and parses every SKILL.md it finds, sorted by path.
// rootName is used as the directory name for a SKILL.md at the root.
// It returns a nil slice when no skills are found.
func Dir(root, rootName string) ([]skill.Skill, error) {
	var skills []skill.Skill
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
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
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		skills = append(skills, skill.Parse(rel, dirName(rel, rootName), content))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", root, err)
	}
	slices.SortFunc(skills, func(a, b skill.Skill) int { return strings.Compare(a.Path, b.Path) })
	return skills, nil
}

// dirName returns the parent directory's base name, or rootName for a root-level file.
func dirName(rel, rootName string) string {
	parent := path.Dir(rel)
	if parent == "." {
		return rootName
	}
	return path.Base(parent)
}
