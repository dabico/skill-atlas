package tui

import (
	"strings"
	"testing"

	"skill-atlas/internal/skill"
)

// failedReport has repos a (2 skills), b (failed before the clone finished) and c (failed after it, with a SHA).
func failedReport() Report {
	return Report{Repos: []Repo{
		{Name: "github.com/org/a", Ref: "main", SHA: "aaaaaaa1111111", Skills: []skill.Skill{mk("a-one"), mk("a-two")}},
		{Name: "github.com/org/b", Ref: "v9", Err: "auth failed: no access"},
		{Name: "github.com/org/c", Ref: "dev", SHA: "ccccccc3333333", Err: "walk failed"},
	}}
}

func newFailed(w, h int) *model {
	m := newModel(failedReport())
	m.Update(sizeMsg(w, h))
	return m
}

func TestFailedRepoHeadingAndErrorRow(t *testing.T) {
	v := view(newFailed(120, 30))
	order := []string{
		"github.com/org/a @ main (aaaaaaa)", "> a-one", "  a-two",
		"github.com/org/b @ v9", "auth failed: no access",
		"github.com/org/c @ dev (ccccccc)", "walk failed",
	}
	at := 0
	for _, want := range order {
		i := strings.Index(v[at:], want)
		if i < 0 {
			t.Fatalf("view lacks %q after offset %d:\n%s", want, at, v)
		}
		at += i + len(want)
	}
	if strings.Contains(v, "org/b @ v9 (") {
		t.Errorf("a repository that failed to clone shows a SHA:\n%s", v)
	}
}

func TestFailedRepoHeaderCount(t *testing.T) {
	first := strings.Split(view(newFailed(100, 30)), "\n")[0]
	if !strings.HasSuffix(strings.TrimRight(first, " "), "2 skills, 0 invalid, 2 failed") {
		t.Errorf("header: %q", first)
	}
	// Nothing failed: no failed count.
	first = strings.Split(view(newMulti(100, 30)), "\n")[0]
	if strings.Contains(first, "failed") {
		t.Errorf("header without failures mentions them: %q", first)
	}
}

func TestFailedRepoCannotBeSelected(t *testing.T) {
	m := newFailed(120, 30)
	var seen []string
	for range 4 {
		seen = append(seen, current(m))
		press(m, "j")
	}
	if strings.Join(seen, ",") != "a-one,a-two,a-two,a-two" {
		t.Errorf("cursor visited %v, want it to stay on the skills", seen)
	}
	press(m, "G")
	if current(m) != "a-two" {
		t.Errorf("G selected %q", current(m))
	}
	for i, r := range m.rows {
		if r.kind == rowError || r.kind == rowHeading {
			for _, at := range m.rowOf {
				if at == i {
					t.Errorf("row %d (kind %d) can hold the cursor", i, r.kind)
				}
			}
		}
	}
}

func TestFailedRepoHiddenWhileFiltering(t *testing.T) {
	m := newFailed(120, 30)
	press(m, "/")
	typeText(m, "org/")
	v := view(m)
	if strings.Contains(v, "auth failed") || strings.Contains(v, "walk failed") || strings.Contains(v, "org/b @") {
		t.Errorf("failed repositories shown while filtering:\n%s", v)
	}
	if !strings.Contains(v, "Skills 2/2") {
		t.Errorf("counter should count skills only:\n%s", v)
	}
	press(m, "esc")
	if v := view(m); !strings.Contains(v, "auth failed: no access") || !strings.Contains(v, "walk failed") {
		t.Errorf("failed repositories missing after clearing the filter:\n%s", v)
	}
}

func TestFailedRepoScrollShowsHeadingWithError(t *testing.T) {
	m := newFailed(120, 12) // 4 list rows in a 60x12-ish pane
	press(m, "G")
	if r := m.rowOf[m.cursor]; r < m.offset || r >= m.offset+m.listRows() {
		t.Errorf("cursor row %d outside offset %d", r, m.offset)
	}
	press(m, "g")
	if m.offset != 0 {
		t.Errorf("offset = %d after g, want 0", m.offset)
	}
}

func TestFailedRepoErrorIsStripped(t *testing.T) {
	r := failedReport()
	r.Repos[1].Err = "boom\x1b]0;evil-title\x07 \x1b[31mred\x1b[0m\nsecond line"
	m := newModel(r)
	m.Update(sizeMsg(120, 30))
	raw := m.View().Content
	if strings.Contains(raw, "\x1b]") || strings.Contains(raw, "evil-title") {
		t.Errorf("remote escape sequence reached the screen: %q", raw)
	}
	if v := view(m); !strings.Contains(v, "boom red second line") {
		t.Errorf("error not shown on one line:\n%s", v)
	}
}

func TestFailedReposOnly(t *testing.T) {
	m := newModel(Report{Repos: []Repo{{Name: "x/one", Err: "e1"}, {Name: "x/two", Ref: "v1", Err: "e2"}}})
	m.Update(sizeMsg(120, 20))
	v := view(m)
	for _, want := range []string{"2 repositories", "0 skills, 0 invalid, 2 failed", "x/one", "x/two @ v1", "e1", "e2"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	press(m, "j", "G", "g", "/", "x", "esc") // must not panic
}
