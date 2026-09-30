package scan

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func doc(name string) string {
	return "---\nname: " + name + "\ndescription: d\n---\nbody\n"
}

func paths(t *testing.T, root string) []string {
	t.Helper()
	skills, err := Dir(root, "repo")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, s := range skills {
		got = append(got, s.Path)
	}
	return got
}

func TestDirNestedAndSorted(t *testing.T) {
	root := t.TempDir()
	write(t, root, "zeta/SKILL.md", doc("zeta"))
	write(t, root, "alpha/SKILL.md", doc("alpha"))
	write(t, root, "alpha/inner/SKILL.md", doc("inner"))
	write(t, root, "b/c/d/SKILL.md", doc("d"))

	want := []string{"alpha/SKILL.md", "alpha/inner/SKILL.md", "b/c/d/SKILL.md", "zeta/SKILL.md"}
	if got := paths(t, root); !reflect.DeepEqual(got, want) {
		t.Errorf("paths = %v, want %v", got, want)
	}
}

func TestDirEmpty(t *testing.T) {
	root := t.TempDir()
	write(t, root, "README.md", "hi")
	skills, err := Dir(root, "repo")
	if err != nil || len(skills) != 0 {
		t.Errorf("got %v, %v", skills, err)
	}
}

func TestDirIgnores(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	write(t, outside, "SKILL.md", doc("outside"))
	write(t, outside, "sub/SKILL.md", doc("sub"))

	write(t, root, "real/SKILL.md", doc("real"))
	write(t, root, ".git/hooks/SKILL.md", doc("hooks"))
	write(t, root, "lower/skill.md", doc("lower"))
	write(t, root, "asdir/SKILL.md/readme.txt", "x")
	write(t, root, "target/SKILL.md", doc("target"))
	if err := os.Symlink(outside, filepath.Join(root, "linkdir")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if err := os.Symlink(filepath.Join(root, "target", "SKILL.md"), filepath.Join(root, "linkfile")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "linked"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "target", "SKILL.md"), filepath.Join(root, "linked", "SKILL.md")); err != nil {
		t.Fatal(err)
	}

	want := []string{"real/SKILL.md", "target/SKILL.md"}
	if got := paths(t, root); !reflect.DeepEqual(got, want) {
		t.Errorf("paths = %v, want %v", got, want)
	}
}

func TestDirRootUsesRootName(t *testing.T) {
	root := t.TempDir()
	write(t, root, "SKILL.md", doc("repo"))
	skills, err := Dir(root, "repo")
	if err != nil || len(skills) != 1 {
		t.Fatalf("got %v, %v", skills, err)
	}
	if s := skills[0]; s.Path != "SKILL.md" || s.Dir != "repo" || !s.Valid() {
		t.Errorf("got %+v", s)
	}
}

func TestDirMismatchSurfaces(t *testing.T) {
	root := t.TempDir()
	write(t, root, "pdf-tools/SKILL.md", doc("pdf"))
	skills, err := Dir(root, "repo")
	if err != nil || len(skills) != 1 {
		t.Fatalf("got %v, %v", skills, err)
	}
	want := []string{`name "pdf" doesn't match directory "pdf-tools"`}
	if !reflect.DeepEqual(skills[0].Errors, want) {
		t.Errorf("errors = %q", skills[0].Errors)
	}
}

func TestDirMissingRoot(t *testing.T) {
	if _, err := Dir(filepath.Join(t.TempDir(), "nope"), "repo"); err == nil {
		t.Error("expected error")
	}
}
