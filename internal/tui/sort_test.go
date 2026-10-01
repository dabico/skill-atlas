package tui

import (
	"fmt"
	"strings"
	"testing"

	"skill-atlas/internal/skill"
)

// order returns the visible skill names in list order.
func order(m *model) string {
	var names []string
	for _, i := range m.visible {
		names = append(names, m.skills[i].DisplayName())
	}
	return strings.Join(names, ",")
}

// rowsText returns the list rows: "#<repo>@<ref>" for headings, "-" for the empty and error rows, the name for skills.
func rowsText(m *model) string {
	var out []string
	for _, r := range m.rows {
		switch r.kind {
		case rowHeading:
			out = append(out, "#"+m.report.Repos[r.repo].Name+"@"+m.report.Repos[r.repo].Ref)
		case rowEmpty, rowError:
			out = append(out, "-")
		default:
			out = append(out, m.skills[m.visible[r.pos]].DisplayName())
		}
	}
	return strings.Join(out, " ")
}

const (
	testAZ = "code-review,data-analysis,pdf-processing,PDF-Tool"
	testZA = "PDF-Tool,pdf-processing,data-analysis,code-review"
)

func TestSortDefaultIsNameAZ(t *testing.T) {
	m := newTest(testSkills(), 100, 30) // input in path order: pdf-processing, code-review, PDF-Tool, data-analysis
	if got := order(m); got != testAZ {
		t.Errorf("order = %s, want %s", got, testAZ)
	}
	if current(m) != "code-review" {
		t.Errorf("first selected = %s, want code-review", current(m))
	}
	if v := view(m); !strings.Contains(v, "┌ Skills A–Z ─") {
		t.Errorf("list title lacks the order:\n%s", v)
	}
}

func TestSortToggle(t *testing.T) {
	m := newTest(testSkills(), 100, 30)
	press(m, "s")
	if got := order(m); got != testZA {
		t.Errorf("order after s = %s, want %s", got, testZA)
	}
	v := view(m)
	if !strings.Contains(v, "┌ Skills Z–A ─") {
		t.Errorf("list title doesn't say Z–A:\n%s", v)
	}
	if i, j := strings.Index(v, "! PDF-Tool"), strings.Index(v, "> code-review"); i < 0 || j < 0 || i > j {
		t.Errorf("list isn't drawn Z–A:\n%s", v)
	}
	press(m, "s")
	if got := order(m); got != testAZ {
		t.Errorf("order after s s = %s, want %s", got, testAZ)
	}
	if !strings.Contains(view(m), "┌ Skills A–Z ─") {
		t.Errorf("list title doesn't say A–Z again:\n%s", view(m))
	}
}

func TestSortKeepsSelection(t *testing.T) {
	skills := testSkills()
	skills[0].Body = strings.Repeat("line of text\n\n", 100) // pdf-processing
	m := newTest(skills, 100, 20)
	press(m, "j", "j")
	if current(m) != "pdf-processing" {
		t.Fatalf("selected %s, want pdf-processing", current(m))
	}
	press(m, "tab", "G") // scroll the detail pane, then sort from it
	off := m.vp.YOffset()
	if off == 0 {
		t.Fatal("detail did not scroll")
	}
	press(m, "s")
	if current(m) != "pdf-processing" || m.cursor != 1 {
		t.Errorf("selected %s at %d after s, want pdf-processing at 1", current(m), m.cursor)
	}
	if m.focus != paneDetail {
		t.Error("s moved the focus")
	}
	if m.vp.YOffset() != off {
		t.Errorf("detail offset = %d after s, want %d", m.vp.YOffset(), off)
	}
	if v := view(m); !strings.Contains(v, "> pdf-processing") {
		t.Errorf("selection not drawn after s:\n%s", v)
	}
}

func TestSortScrollsToSelection(t *testing.T) {
	var skills []skill.Skill
	for i := range 30 {
		n := fmt.Sprintf("skill-%02d", i)
		skills = append(skills, skill.Skill{Path: n + "/SKILL.md", Dir: n, Name: n, Description: "d"})
	}
	m := newTest(skills, 80, 14) // 10 list rows
	press(m, "s")                // skill-00 stays selected and is now last
	v := view(m)
	if current(m) != "skill-00" || !strings.Contains(v, "> skill-00") {
		t.Errorf("selection not visible after s:\n%s", v)
	}
	if strings.Contains(v, "skill-29") {
		t.Errorf("list did not scroll to the selection:\n%s", v)
	}
	press(m, "g")
	if v := view(m); current(m) != "skill-29" || !strings.Contains(v, "> skill-29") {
		t.Errorf("g after s selected %s:\n%s", current(m), v)
	}
}

func TestSortKeyWhileTypingFilters(t *testing.T) {
	m := newTest(testSkills(), 100, 30)
	press(m, "/", "s")
	if m.desc || m.filter != "s" {
		t.Errorf("s while typing: desc = %v, filter = %q; want the letter in the filter", m.desc, m.filter)
	}
}

func TestSortWithFilter(t *testing.T) {
	m := newTest(testSkills(), 100, 30)
	press(m, "/", "a", "enter") // pdf-processing (description) and data-analysis
	if got := order(m); got != "data-analysis,pdf-processing" {
		t.Fatalf("filtered order = %s", got)
	}
	press(m, "s")
	if got := order(m); got != "pdf-processing,data-analysis" {
		t.Errorf("filtered order after s = %s", got)
	}
	v := view(m)
	if !strings.Contains(v, "┌ Skills 2/4 Z–A ─") || !strings.Contains(v, "/ a") {
		t.Errorf("filter or order not shown:\n%s", v)
	}
	press(m, "esc")
	if got := order(m); got != testZA {
		t.Errorf("order after clearing the filter = %s, want %s", got, testZA)
	}
}

func TestSortFooterAndTitleAtMinimumSize(t *testing.T) {
	m := newTest(testSkills(), 60, 12)
	v := view(m)
	if !strings.Contains(v, "┌ Skills A–Z ─") || !strings.Contains(v, "/ filter · s sort · q quit") {
		t.Errorf("title or footer at 60x12:\n%s", v)
	}
	press(m, "tab")
	if v := view(m); !strings.Contains(v, "j/k scroll · tab focus · / filter · s sort · q quit") {
		t.Errorf("detail footer:\n%s", v)
	}
	press(m, "tab", "/", "a")
	if v := view(m); !strings.Contains(v, "enter apply · esc clear") || strings.Contains(v, "s sort") {
		t.Errorf("typing footer:\n%s", v)
	}
	press(m, "enter", "s")
	v = view(m)
	if !strings.Contains(v, "┌ Skills 2/4 Z–A ─") {
		t.Errorf("filtered title at 60x12:\n%s", v)
	}
	lines := strings.Split(v, "\n")
	if footer := lines[len(lines)-1]; !strings.HasSuffix(footer, "esc clear filter · s sort · q quit") {
		t.Errorf("filtered footer doesn't fit 60 columns: %q", footer)
	}
}

// sortReport lists repositories out of name order, with skills out of name order inside them.
func sortReport() Report {
	return Report{Repos: []Repo{
		{Name: "github.com/org/c", Ref: "dev", SHA: "ccccccc3333333", Skills: []skill.Skill{mk("c-two"), mk("c-one")}},
		{Name: "github.com/org/x", Ref: "v2", Err: "boom"},
		{Name: "github.com/Org/a", Ref: "main", SHA: "aaaaaaa1111111", Skills: []skill.Skill{mk("a-two"), mk("A-one")}},
		{Name: "github.com/org/b", Ref: "v1", SHA: "bbbbbbb2222222", Excluded: 2},
		{Name: "github.com/org/x", Ref: "v1", SHA: "ddddddd4444444", Skills: []skill.Skill{mk("x-one")}},
	}}
}

func TestMultiSortRepositoriesThenSkills(t *testing.T) {
	m := newModel(sortReport())
	m.Update(sizeMsg(120, 30))
	az := "#github.com/Org/a@main A-one a-two #github.com/org/b@v1 - #github.com/org/c@dev c-one c-two " +
		"#github.com/org/x@v1 x-one #github.com/org/x@v2 -"
	if got := rowsText(m); got != az {
		t.Errorf("rows A–Z:\n got %s\nwant %s", got, az)
	}
	if current(m) != "A-one" {
		t.Errorf("first selected = %s, want A-one", current(m))
	}

	press(m, "s")
	za := "#github.com/org/x@v2 - #github.com/org/x@v1 x-one #github.com/org/c@dev c-two c-one " +
		"#github.com/org/b@v1 - #github.com/Org/a@main a-two A-one"
	if got := rowsText(m); got != za {
		t.Errorf("rows Z–A:\n got %s\nwant %s", got, za)
	}
	if current(m) != "A-one" {
		t.Errorf("selected %s after s, want A-one", current(m))
	}
	// The cursor still walks the skills only, now Z–A.
	press(m, "g")
	var seen []string
	for range 5 {
		seen = append(seen, current(m))
		press(m, "j")
	}
	if got := strings.Join(seen, ","); got != "x-one,c-two,c-one,a-two,A-one" {
		t.Errorf("j order Z–A = %s", got)
	}

	v := view(m)
	at := 0
	for _, want := range []string{"github.com/org/x @ v2", "boom", "github.com/org/x @ v1 (ddddddd)", "github.com/org/c", "github.com/org/b", "No skills found (2 excluded)", "github.com/Org/a"} {
		i := strings.Index(v[at:], want)
		if i < 0 {
			t.Fatalf("Z–A view lacks %q after offset %d:\n%s", want, at, v)
		}
		at += i + len(want)
	}

	press(m, "s")
	if got := rowsText(m); got != az {
		t.Errorf("rows after s s:\n got %s\nwant %s", got, az)
	}
}

func TestMultiSortWithFilter(t *testing.T) {
	m := newModel(sortReport())
	m.Update(sizeMsg(120, 30))
	press(m, "/")
	typeText(m, "one")
	press(m, "enter", "s")
	if got, want := rowsText(m), "#github.com/org/x@v1 x-one #github.com/org/c@dev c-one #github.com/Org/a@main A-one"; got != want {
		t.Errorf("filtered rows Z–A:\n got %s\nwant %s", got, want)
	}
	if v := view(m); !strings.Contains(v, "Skills 3/5 Z–A") {
		t.Errorf("counter or order missing:\n%s", v)
	}
}
