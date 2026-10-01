package skill

import (
	"slices"
	"testing"
)

func TestCompare(t *testing.T) {
	skills := []Skill{
		{Path: "z/pdf-tool/SKILL.md", Dir: "pdf-tool", Name: "pdf-tool"},
		{Path: "skills/PDF-Tool/SKILL.md", Dir: "PDF-Tool"}, // no name: sorts by directory
		{Path: "b/code-review/SKILL.md", Dir: "code-review", Name: "code-review"},
		{Path: "a/code-review/SKILL.md", Dir: "code-review", Name: "code-review"},
		{Path: "Beta/SKILL.md", Dir: "Beta", Name: "Beta"},
		{Path: "alpha/SKILL.md", Dir: "alpha", Name: "alpha"},
	}
	slices.SortStableFunc(skills, Compare)
	var got []string
	for _, s := range skills {
		got = append(got, s.Path)
	}
	// Case is ignored (alpha < Beta < code-review). The same name sorts by path.
	// Names equal up to case sort by the exact name, "P" before "p".
	want := []string{
		"alpha/SKILL.md",
		"Beta/SKILL.md",
		"a/code-review/SKILL.md",
		"b/code-review/SKILL.md",
		"skills/PDF-Tool/SKILL.md",
		"z/pdf-tool/SKILL.md",
	}
	if !slices.Equal(got, want) {
		t.Errorf("order = %q\nwant    %q", got, want)
	}
	if Compare(skills[0], skills[0]) != 0 {
		t.Error("a skill doesn't compare equal to itself")
	}
}

func TestCompareRepos(t *testing.T) {
	type repo struct{ name, ref string }
	repos := []repo{
		{"github.com/org/b", ""},
		{"github.com/Org/a", "v2"},
		{"github.com/org/a", "main"},
		{"github.com/Org/a", "v1"},
		{"github.com/org/C", ""},
	}
	slices.SortStableFunc(repos, func(a, b repo) int { return CompareRepos(a.name, a.ref, b.name, b.ref) })
	want := []repo{
		{"github.com/Org/a", "v1"},
		{"github.com/Org/a", "v2"},
		{"github.com/org/a", "main"},
		{"github.com/org/b", ""},
		{"github.com/org/C", ""},
	}
	if !slices.Equal(repos, want) {
		t.Errorf("order = %v\nwant    %v", repos, want)
	}
}
