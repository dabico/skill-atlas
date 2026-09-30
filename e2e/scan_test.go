//go:build e2e

package e2e

import (
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"

	"skill-atlas/internal/repo"
	"skill-atlas/internal/skill"
)

var sha40 = regexp.MustCompile(`^[0-9a-f]{40}$`)

func TestPinnedScan(t *testing.T) {
	t.Parallel()
	f := ideavim
	target, err := repo.ParseURL(f.url)
	if err != nil {
		t.Fatal(err)
	}
	if target.Display != f.display {
		t.Errorf("Display = %q, want %q", target.Display, f.display)
	}
	if target.Name != f.name {
		t.Errorf("Name = %q, want %q", target.Name, f.name)
	}

	res := cloneScan(t, f.url, f.tag)
	t.Logf("clone %s@%s took %s", f.name, f.tag, res.cloneDur.Round(100e6))
	if res.checkout.Ref != f.tag {
		t.Errorf("Ref = %q, want %q", res.checkout.Ref, f.tag)
	}
	if res.checkout.SHA != f.sha {
		t.Errorf("SHA = %q, want %q", res.checkout.SHA, f.sha)
	}

	checkInvariants(t, res.skills, f.name)
	var got []string
	for _, s := range res.skills {
		got = append(got, s.Path)
	}
	if !slices.Equal(got, f.paths) {
		t.Errorf("paths = %v, want %v", got, f.paths)
	}

	checkGolden(t, f.golden, res.skills)
}

// checkInvariants asserts path shape, ordering and Dir for any scan result.
func checkInvariants(t *testing.T, skills []skill.Skill, rootName string) {
	t.Helper()
	for i, s := range skills {
		if !strings.HasSuffix(s.Path, "/SKILL.md") && s.Path != "SKILL.md" {
			t.Errorf("path %q doesn't end in /SKILL.md", s.Path)
		}
		if strings.Contains(s.Path, `\`) {
			t.Errorf("path %q isn't slash-separated", s.Path)
		}
		if slices.Contains(strings.Split(s.Path, "/"), ".git") {
			t.Errorf("path %q has a .git component", s.Path)
		}
		parent := path.Dir(s.Path)
		wantDir := rootName
		if parent != "." {
			wantDir = path.Base(parent)
		}
		if s.Dir != wantDir {
			t.Errorf("%s: Dir = %q, want %q", s.Path, s.Dir, wantDir)
		}
		if i > 0 && skills[i-1].Path >= s.Path {
			t.Errorf("not sorted/unique: %q before %q", skills[i-1].Path, s.Path)
		}
	}
}

// TestFloatingScan tracks the default branch: invariants only, no golden.
func TestFloatingScan(t *testing.T) {
	if os.Getenv("SKILL_ATLAS_E2E_HEAD") != "1" {
		t.Skip("set SKILL_ATLAS_E2E_HEAD=1 to run")
	}
	t.Parallel()
	f := ideavim
	res := cloneScan(t, f.url, "")
	t.Logf("clone %s@HEAD took %s", f.name, res.cloneDur.Round(100e6))
	if res.checkout.Ref == "" {
		t.Error("Ref is empty")
	}
	if !sha40.MatchString(res.checkout.SHA) {
		t.Errorf("SHA = %q, want 40 lowercase hex", res.checkout.SHA)
	}
	if len(res.skills) < 1 {
		t.Fatal("no skills found")
	}
	checkInvariants(t, res.skills, f.name)
	for _, s := range res.skills {
		for _, e := range s.Errors {
			if strings.TrimSpace(e) == "" {
				t.Errorf("%s: empty error string", s.Path)
			}
		}
		if s.Valid() != (len(s.Errors) == 0) {
			t.Errorf("%s: Valid() disagrees with Errors", s.Path)
		}
	}
}
