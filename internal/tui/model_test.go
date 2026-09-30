package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"skill-atlas/internal/skill"
)

func testSkills() []skill.Skill {
	return []skill.Skill{
		{
			Path: "skills/pdf-processing/SKILL.md", Dir: "pdf-processing", Name: "pdf-processing",
			Description: "Extract PDF text, fill forms, merge files.", License: "Apache-2.0",
			Metadata: []skill.MetadataEntry{{Key: "version", Value: "1.0"}},
			Body:     "# Heading One\n\nSome **bold** body text.\n",
		},
		{
			Path: "skills/code-review/SKILL.md", Dir: "code-review", Name: "code-review",
			Description: "Review code for bugs.",
		},
		{
			Path: "skills/PDF-Tool/SKILL.md", Dir: "PDF-Tool", Name: "",
			Description: "Broken skill.",
			Errors:      []string{`name "PDF-Tool" contains uppercase letters`},
		},
		{
			Path: "skills/data-analysis/SKILL.md", Dir: "data-analysis", Name: "data-analysis",
			Description: "Crunch numbers.",
		},
	}
}

func newTest(skills []skill.Skill, w, h int) *model {
	m := newModel(Report{Repo: "github.com/org/repo", Ref: "main", SHA: "a1b2c3d4e5f6", Skills: skills})
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

func press(m *model, keys ...string) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		var msg tea.KeyPressMsg
		switch k {
		case "tab":
			msg = tea.KeyPressMsg{Code: tea.KeyTab}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "down":
			msg = tea.KeyPressMsg{Code: tea.KeyDown}
		case "ctrl+c":
			msg = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
		default:
			r := []rune(k)[0]
			msg = tea.KeyPressMsg{Code: r, Text: k}
		}
		_, cmd = m.Update(msg)
	}
	return cmd
}

func typeText(m *model, s string) {
	for _, r := range s {
		press(m, string(r))
	}
}

func view(m *model) string { return ansi.Strip(m.View().Content) }

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestHeader(t *testing.T) {
	m := newTest(testSkills(), 100, 30)
	v := view(m)
	first := strings.Split(v, "\n")[0]
	if !strings.Contains(first, "github.com/org/repo @ main (a1b2c3d)") {
		t.Errorf("header missing repo/ref/sha: %q", first)
	}
	if !strings.HasSuffix(strings.TrimRight(first, " "), "4 skills, 1 invalid") {
		t.Errorf("header missing counts: %q", first)
	}
	if w := ansi.StringWidth(first); w != 100 {
		t.Errorf("header width = %d, want 100", w)
	}

	one := newTest(testSkills()[:1], 100, 30)
	if !strings.Contains(view(one), "1 skill, 0 invalid") {
		t.Errorf("singular count missing:\n%s", view(one))
	}
}

func newExcluded(skills []skill.Skill, excluded, w, h int) *model {
	m := newModel(Report{Repo: "github.com/org/repo", Ref: "main", SHA: "a1b2c3d4e5f6", Skills: skills, Excluded: excluded})
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

func TestHeaderExcluded(t *testing.T) {
	first := func(m *model) string { return strings.TrimRight(strings.Split(view(m), "\n")[0], " ") }

	m := newExcluded(testSkills()[:2], 2, 100, 30)
	if f := first(m); !strings.HasSuffix(f, "2 skills, 0 invalid, 2 excluded") {
		t.Errorf("header missing excluded count: %q", f)
	}
	if w := ansi.StringWidth(strings.Split(view(m), "\n")[0]); w != 100 {
		t.Errorf("header width = %d, want 100", w)
	}

	if f := first(newExcluded(testSkills(), 1, 100, 30)); !strings.HasSuffix(f, "4 skills, 1 invalid, 1 excluded") {
		t.Errorf("header with invalid and excluded: %q", f)
	}
	if f := first(newExcluded(testSkills()[:1], 3, 100, 30)); !strings.HasSuffix(f, "1 skill, 0 invalid, 3 excluded") {
		t.Errorf("singular with excluded: %q", f)
	}

	// With nothing excluded the header is unchanged.
	if f := first(newExcluded(testSkills(), 0, 100, 30)); !strings.HasSuffix(f, "4 skills, 1 invalid") {
		t.Errorf("header changed without exclusions: %q", f)
	}
}

func TestEmptyReportExcluded(t *testing.T) {
	m := newExcluded(nil, 2, 100, 30)
	v := view(m)
	if !strings.Contains(v, "No skills found (2 excluded)") {
		t.Errorf("missing empty message with count:\n%s", v)
	}
	if !strings.Contains(v, "0 skills, 0 invalid, 2 excluded") {
		t.Errorf("missing counts:\n%s", v)
	}
	if v := view(newExcluded(nil, 0, 100, 30)); strings.Contains(v, "excluded") {
		t.Errorf("empty message mentions exclusions when there are none:\n%s", v)
	}

	// A filter with no hits keeps its own message.
	m = newExcluded(testSkills(), 2, 100, 30)
	press(m, "/")
	typeText(m, "zzz")
	if v := view(m); !strings.Contains(v, "No matches") || strings.Contains(v, "No skills found") {
		t.Errorf("filter message wrong:\n%s", v)
	}
}

func TestLayoutSize(t *testing.T) {
	m := newTest(testSkills(), 100, 30)
	lines := strings.Split(view(m), "\n")
	if len(lines) != 30 {
		t.Errorf("got %d lines, want 30", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w > 100 {
			t.Errorf("line %d width %d > 100", i, w)
		}
	}
}

func TestListRows(t *testing.T) {
	m := newTest(testSkills(), 100, 30)
	v := view(m)
	for _, want := range []string{"> pdf-processing", "  code-review", "! PDF-Tool", "[invalid]", "  data-analysis"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
}

func TestNavigationBounds(t *testing.T) {
	m := newTest(testSkills(), 100, 30)
	press(m, "k")
	if m.cursor != 0 {
		t.Errorf("cursor = %d after k at top", m.cursor)
	}
	press(m, "j", "j", "j", "j", "j")
	if m.cursor != 3 {
		t.Errorf("cursor = %d, want 3", m.cursor)
	}
	press(m, "g")
	if m.cursor != 0 {
		t.Errorf("cursor = %d after g", m.cursor)
	}
	press(m, "G")
	if m.cursor != 3 {
		t.Errorf("cursor = %d after G", m.cursor)
	}
	press(m, "down")
	if m.cursor != 3 {
		t.Errorf("cursor = %d after down at bottom", m.cursor)
	}
}

func TestListScrollsWithSelection(t *testing.T) {
	var skills []skill.Skill
	for i := range 30 {
		n := fmt.Sprintf("skill-%02d", i)
		skills = append(skills, skill.Skill{Path: n + "/SKILL.md", Dir: n, Name: n, Description: "d"})
	}
	m := newTest(skills, 80, 14) // 10 inner rows
	press(m, "G")
	v := view(m)
	if !strings.Contains(v, "> skill-29") {
		t.Errorf("selection not visible:\n%s", v)
	}
	if strings.Contains(v, "  skill-00") {
		t.Errorf("list did not scroll:\n%s", v)
	}
}

func TestDisplayNameFallback(t *testing.T) {
	m := newTest(testSkills(), 100, 30)
	press(m, "j", "j")
	v := view(m)
	if !strings.Contains(v, "! PDF-Tool") {
		t.Errorf("dir name fallback missing in list:\n%s", v)
	}
	if !strings.Contains(v, " PDF-Tool ┐") {
		t.Errorf("detail title should use dir name:\n%s", v)
	}
	if !strings.Contains(v, "Invalid") || !strings.Contains(v, `• name "PDF-Tool" contains uppercase letters`) {
		t.Errorf("invalid errors missing:\n%s", v)
	}
}

func TestDetailContent(t *testing.T) {
	m := newTest(testSkills(), 120, 40)
	v := view(m)
	for _, want := range []string{
		"skills/pdf-processing/SKILL.md",
		"Extract PDF text, fill forms, merge files.",
		"license: Apache-2.0",
		"metadata:",
		"  version: 1.0",
		"Heading One",
		"Some bold body text.",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("detail missing %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "# Heading One") || strings.Contains(v, "**bold**") {
		t.Errorf("markdown not rendered:\n%s", v)
	}
}

func TestTabFocus(t *testing.T) {
	m := newTest(testSkills(), 100, 30)
	if m.focus != paneList {
		t.Fatal("initial focus should be list")
	}
	press(m, "tab")
	if m.focus != paneDetail {
		t.Error("tab should focus detail")
	}
	press(m, "j")
	if m.cursor != 0 {
		t.Error("j in detail focus must not move the list")
	}
	press(m, "tab")
	if m.focus != paneList {
		t.Error("tab should return to list")
	}
}

func TestDetailScrollResetsOnSelection(t *testing.T) {
	skills := testSkills()
	skills[0].Body = strings.Repeat("line of text\n\n", 100)
	m := newTest(skills, 100, 20)
	press(m, "tab", "G")
	if m.vp.YOffset() == 0 {
		t.Fatal("detail did not scroll")
	}
	press(m, "tab", "j")
	if m.vp.YOffset() != 0 {
		t.Errorf("offset = %d after selection change, want 0", m.vp.YOffset())
	}
}

func TestFilter(t *testing.T) {
	tests := []struct {
		name, query string
		want        []string
	}{
		{"name", "code", []string{"code-review"}},
		{"case-insensitive", "DATA", []string{"data-analysis"}},
		{"description", "crunch", []string{"data-analysis"}},
		{"path", "skills/pdf-processing", []string{"pdf-processing"}},
		{"none", "zzz", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTest(testSkills(), 100, 30)
			press(m, "/")
			typeText(m, tt.query)
			var got []string
			for _, i := range m.visible {
				got = append(got, m.report.Skills[i].DisplayName())
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("visible = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterUI(t *testing.T) {
	m := newTest(testSkills(), 100, 30)
	press(m, "tab", "/")
	if !m.filtering || m.focus != paneList {
		t.Fatal("/ should focus list and open the filter")
	}
	typeText(m, "a")
	v := view(m)
	if !strings.Contains(v, "enter apply · esc clear") {
		t.Errorf("filter footer missing:\n%s", v)
	}
	press(m, "enter")
	if m.filtering {
		t.Error("enter should close the input")
	}
	v = view(m)
	if !strings.Contains(v, "Skills 2/4") || !strings.Contains(v, "/ a") {
		t.Errorf("active filter not shown:\n%s", v)
	}
	press(m, "esc")
	if m.filter != "" || len(m.visible) != 4 {
		t.Errorf("esc should clear the filter, got %q / %d visible", m.filter, len(m.visible))
	}
}

func TestFilterEscClearsWhileTyping(t *testing.T) {
	m := newTest(testSkills(), 100, 30)
	press(m, "/")
	typeText(m, "code")
	press(m, "esc")
	if m.filtering || m.filter != "" || len(m.visible) != 4 {
		t.Errorf("esc did not reset filter state: %+v", m.filter)
	}
}

func TestQuitKeys(t *testing.T) {
	m := newTest(testSkills(), 100, 30)
	if !isQuit(press(m, "q")) {
		t.Error("q should quit")
	}
	m = newTest(testSkills(), 100, 30)
	if !isQuit(press(m, "ctrl+c")) {
		t.Error("ctrl+c should quit")
	}

	m = newTest(testSkills(), 100, 30)
	press(m, "/")
	if isQuit(press(m, "q")) {
		t.Error("q while filtering must not quit")
	}
	if m.filter != "q" {
		t.Errorf("filter = %q, want q", m.filter)
	}
	if !isQuit(press(m, "ctrl+c")) {
		t.Error("ctrl+c while filtering should quit")
	}
}

func TestEmptyReport(t *testing.T) {
	m := newTest(nil, 100, 30)
	v := view(m)
	if !strings.Contains(v, "No skills found") {
		t.Errorf("missing empty message:\n%s", v)
	}
	if !strings.Contains(v, "0 skills, 0 invalid") {
		t.Errorf("missing zero counts:\n%s", v)
	}
	press(m, "j", "G", "tab", "j", "/", "x", "esc") // must not panic
}

func TestNoMatches(t *testing.T) {
	m := newTest(testSkills(), 100, 30)
	press(m, "/")
	typeText(m, "zzz")
	if v := view(m); !strings.Contains(v, "No matches") {
		t.Errorf("missing No matches:\n%s", v)
	}
}

func TestTooSmall(t *testing.T) {
	m := newTest(testSkills(), 50, 10)
	v := view(m)
	if !strings.Contains(v, "terminal too small") {
		t.Errorf("missing too-small message:\n%s", v)
	}
	if strings.Contains(v, "pdf-processing") {
		t.Error("layout should be hidden")
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if v := view(m); !strings.Contains(v, "pdf-processing") {
		t.Error("layout should return after resize")
	}
}

func TestBackgroundColorSelectsStyle(t *testing.T) {
	m := newTest(testSkills(), 100, 30)
	if !m.dark {
		t.Fatal("default should be dark")
	}
	m.Update(tea.BackgroundColorMsg{Color: ansi.XParseColor("rgb:ffff/ffff/ffff")})
	if m.dark {
		t.Error("white background should select the light style")
	}
	if len(m.cache) < 2 {
		t.Errorf("cache should hold a render per style, has %d", len(m.cache))
	}
}

func TestRenderMarkdownFallbackOnBadStyle(t *testing.T) {
	m := newTest(testSkills(), 100, 30)
	s := m.report.Skills[0]
	if out := m.renderBody(s, 40); out == "" {
		t.Error("empty render")
	}
}
