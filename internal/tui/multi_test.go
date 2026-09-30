package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"skill-atlas/internal/skill"
)

func mk(name string) skill.Skill {
	return skill.Skill{Path: "skills/" + name + "/SKILL.md", Dir: name, Name: name, Description: "About " + name + "."}
}

// multiReport has repos a (2 skills, 1 invalid), b (none, 2 excluded) and c (2 skills).
func multiReport() Report {
	bad := mk("a-two")
	bad.Errors = []string{"broken"}
	return Report{Repos: []Repo{
		{Name: "github.com/org/a", Ref: "main", SHA: "aaaaaaa1111111", Skills: []skill.Skill{mk("a-one"), bad}},
		{Name: "github.com/org/b", Ref: "v2", SHA: "bbbbbbb2222222", Excluded: 2},
		{Name: "github.com/org/c", Ref: "dev", SHA: "ccccccc3333333", Skills: []skill.Skill{mk("c-one"), mk("c-two")}},
	}}
}

func newMulti(w, h int) *model {
	m := newModel(multiReport())
	m.Update(sizeMsg(w, h))
	return m
}

func current(m *model) string { return m.skills[m.selected()].DisplayName() }

func TestMultiHeader(t *testing.T) {
	first := strings.Split(view(newMulti(100, 30)), "\n")[0]
	if !strings.HasPrefix(first, " 3 repositories") {
		t.Errorf("header left: %q", first)
	}
	if !strings.HasSuffix(strings.TrimRight(first, " "), "4 skills, 1 invalid, 2 excluded") {
		t.Errorf("header totals: %q", first)
	}
	if w := ansi.StringWidth(first); w != 100 {
		t.Errorf("header width = %d, want 100", w)
	}
}

func TestMultiGroupsAndEmptyRepo(t *testing.T) {
	v := view(newMulti(120, 30))
	order := []string{
		"github.com/org/a @ main (aaaaaaa)", "> a-one", "! a-two",
		"github.com/org/b @ v2 (bbbbbbb)", "No skills found (2 excluded)",
		"github.com/org/c @ dev (ccccccc)", "  c-one", "  c-two",
	}
	at := 0
	for _, want := range order {
		i := strings.Index(v[at:], want)
		if i < 0 {
			t.Fatalf("view lacks %q after offset %d:\n%s", want, at, v)
		}
		at += i + len(want)
	}
}

func TestMultiCursorSkipsHeadings(t *testing.T) {
	m := newMulti(120, 30)
	var seen []string
	for range 3 {
		press(m, "j")
		seen = append(seen, current(m))
		if m.rows[m.rowOf[m.cursor]].kind != rowSkill {
			t.Fatal("cursor on a non-skill row")
		}
	}
	if got := strings.Join(seen, ","); got != "a-two,c-one,c-two" {
		t.Errorf("j order = %s", got)
	}
	press(m, "j")
	if current(m) != "c-two" {
		t.Errorf("j at the end moved to %s", current(m))
	}
	press(m, "k", "k")
	if current(m) != "a-two" {
		t.Errorf("k over the empty repo landed on %s", current(m))
	}
	press(m, "G")
	if current(m) != "c-two" {
		t.Errorf("G = %s", current(m))
	}
	press(m, "g")
	if current(m) != "a-one" {
		t.Errorf("g = %s", current(m))
	}
	press(m, "pgdown")
	if current(m) != "c-two" {
		t.Errorf("pgdown = %s", current(m))
	}
	press(m, "pgup")
	if current(m) != "a-one" {
		t.Errorf("pgup = %s", current(m))
	}
}

func TestMultiScrollShowsGroupHeading(t *testing.T) {
	// 8 inner rows; the list has 6+1 and 6+1 rows.
	var a, c []skill.Skill
	for i := range 6 {
		a = append(a, mk(fmt.Sprintf("a-%d", i)))
		c = append(c, mk(fmt.Sprintf("c-%d", i)))
	}
	m := newModel(Report{Repos: []Repo{{Name: "github.com/org/a", Skills: a}, {Name: "github.com/org/c", Skills: c}}})
	m.Update(sizeMsg(80, 12))
	press(m, "j", "j", "j", "j", "j", "j") // c-0, first skill of c
	v := view(m)
	if !strings.Contains(v, "github.com/org/c") || !strings.Contains(v, "> c-0") {
		t.Errorf("heading of the selected group not shown:\n%s", v)
	}
	press(m, "j", "j", "j", "j", "j")
	if !strings.Contains(view(m), "> c-5") {
		t.Errorf("cursor scrolled out of view:\n%s", view(m))
	}
	press(m, "g")
	v = view(m)
	if !strings.Contains(v, "github.com/org/a") || !strings.Contains(v, "> a-0") {
		t.Errorf("g didn't scroll back to the top:\n%s", v)
	}
	press(m, "G", "k", "k", "k", "k", "k") // c-0 again, coming from below
	if v := view(m); !strings.Contains(v, "github.com/org/c") || !strings.Contains(v, "> c-0") {
		t.Errorf("heading hidden when moving up to the first skill:\n%s", v)
	}
}

func TestMultiScrollKeepsCursorVisibleWhenOneRow(t *testing.T) {
	m := newMulti(60, 12)
	m.height = 5 // 1 inner row
	m.relayout()
	for range 3 {
		press(m, "j")
		if r := m.rowOf[m.cursor]; r < m.offset || r >= m.offset+m.listRows() {
			t.Fatalf("cursor row %d outside offset %d", r, m.offset)
		}
	}
}

func TestMultiFilter(t *testing.T) {
	tests := []struct {
		query    string
		want     []string
		headings []string
	}{
		{"org/c", []string{"c-one", "c-two"}, []string{"org/c"}},
		{"a-two", []string{"a-two"}, []string{"org/a"}},
		{"ORG/", []string{"a-one", "a-two", "c-one", "c-two"}, []string{"org/a", "org/c"}},
		{"about c-", []string{"c-one", "c-two"}, []string{"org/c"}},
		{"zzz", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			m := newMulti(120, 30)
			press(m, "/")
			typeText(m, tt.query)
			var got []string
			for _, i := range m.visible {
				got = append(got, m.skills[i].DisplayName())
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("visible = %v, want %v", got, tt.want)
			}
			v := view(m)
			for _, r := range []string{"org/a", "org/b", "org/c"} {
				shown := strings.Contains(leftOf(v), r+" @")
				want := false
				for _, h := range tt.headings {
					want = want || h == r
				}
				if shown != want {
					t.Errorf("heading %s shown = %v, want %v:\n%s", r, shown, want, v)
				}
			}
			if strings.Contains(v, "No skills found") {
				t.Errorf("empty-repo note shown while filtering:\n%s", v)
			}
			if tt.want == nil && !strings.Contains(v, "No matches") {
				t.Errorf("missing No matches:\n%s", v)
			}
			if tt.want != nil && !strings.Contains(v, fmt.Sprintf("Skills %d/4", len(tt.want))) {
				t.Errorf("counter should count skills only:\n%s", v)
			}
		})
	}
}

// leftOf returns the list pane columns of the body lines.
func leftOf(v string) string {
	var b strings.Builder
	for _, l := range strings.Split(v, "\n")[1:] {
		b.WriteString(string([]rune(l)[:min(len([]rune(l)), 40)]) + "\n")
	}
	return b.String()
}

func TestMultiFilterEscRestoresGroups(t *testing.T) {
	m := newMulti(120, 30)
	press(m, "/")
	typeText(m, "org/c")
	press(m, "esc")
	v := view(m)
	if !strings.Contains(v, "org/b @ v2") || !strings.Contains(v, "No skills found (2 excluded)") {
		t.Errorf("empty repo missing after clearing the filter:\n%s", v)
	}
}

func TestMultiDetailShowsRepository(t *testing.T) {
	m := newMulti(120, 30)
	press(m, "j", "j")
	lines := strings.Split(m.detailContent(60), "\n")
	if got := ansi.Strip(lines[0]); got != "github.com/org/c @ dev (ccccccc)" {
		t.Errorf("first detail line = %q", got)
	}
	if got := ansi.Strip(lines[1]); got != "skills/c-one/SKILL.md" {
		t.Errorf("second detail line = %q", got)
	}
	if one := newTest(testSkills(), 100, 30); strings.Contains(one.detailContent(60), "github.com/org/repo") {
		t.Error("single-repo detail shows the repository")
	}
}

func TestMultiSameSkillPathInTwoRepos(t *testing.T) {
	s := mk("same")
	s.Body = "hello from body\n"
	other := s
	other.Body = "different body\n"
	m := newModel(Report{Repos: []Repo{
		{Name: "x/one", Skills: []skill.Skill{s}},
		{Name: "x/two", Skills: []skill.Skill{other}},
	}})
	m.Update(sizeMsg(120, 30))
	if !strings.Contains(view(m), "hello from body") {
		t.Fatal("first body missing")
	}
	press(m, "j")
	if v := view(m); !strings.Contains(v, "different body") || strings.Contains(v, "hello from body") {
		t.Errorf("body cache mixed up repos:\n%s", v)
	}
}

func TestMultiAllReposEmpty(t *testing.T) {
	m := newModel(Report{Repos: []Repo{{Name: "x/one"}, {Name: "x/two", Excluded: 1}}})
	m.Update(sizeMsg(120, 20))
	v := view(m)
	for _, want := range []string{"2 repositories", "0 skills, 0 invalid, 1 excluded", "x/one", "x/two", "No skills found (1 excluded)"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	press(m, "j", "G", "g", "/", "x", "esc") // must not panic
}

func TestMultiMinimumSize(t *testing.T) {
	m := newMulti(60, 12)
	v := view(m)
	if strings.Contains(v, "too small") || !strings.Contains(v, "github.com/org/a") {
		t.Errorf("60x12 should still render:\n%s", v)
	}
}
