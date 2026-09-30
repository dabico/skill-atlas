package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// listWidth is the outer width of the left pane.
func (m *model) listWidth() int { return max(24, min(48, m.width/3)) }

func (m *model) detailWidth() int      { return m.width - m.listWidth() }
func (m *model) detailInnerWidth() int { return max(m.detailWidth()-4, 1) }
func (m *model) listInnerWidth() int   { return max(m.listWidth()-4, 1) }

// innerHeight is the content height of a pane (screen minus header, footer and borders).
func (m *model) innerHeight() int { return max(m.height-4, 1) }

// listRows is the number of skill rows that fit in the list pane.
func (m *model) listRows() int {
	if m.filterRowShown() {
		return max(m.innerHeight()-1, 1)
	}
	return m.innerHeight()
}

func (m *model) filterRowShown() bool { return m.filtering || m.filter != "" }

func (m *model) View() tea.View {
	var content string
	if m.tooSmall() {
		content = fmt.Sprintf("terminal too small (need %dx%d)", minWidth, minHeight)
	} else {
		content = strings.Join([]string{m.header(), m.panes(), m.footer()}, "\n")
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m *model) header() string {
	left := " " + m.report.Repo
	if m.report.Ref != "" {
		left += " @ " + m.report.Ref
	}
	if sha := m.report.SHA; sha != "" {
		left += " (" + sha[:min(7, len(sha))] + ")"
	}
	noun := "skills"
	if len(m.report.Skills) == 1 {
		noun = "skill"
	}
	right := fmt.Sprintf("%d %s, %d invalid", len(m.report.Skills), noun, m.invalid)
	if m.report.Excluded > 0 {
		right += fmt.Sprintf(", %d excluded", m.report.Excluded)
	}
	right += " "

	rw := ansi.StringWidth(right)
	left = ansi.Truncate(left, max(m.width-rw-1, 0), "…")
	gap := max(m.width-ansi.StringWidth(left)-rw, 1)
	return boldStyle.Render(left) + strings.Repeat(" ", gap) + right
}

func (m *model) footer() string {
	var hint string
	switch {
	case m.filtering:
		hint = "enter apply · esc clear"
	case m.filter != "":
		hint = "j/k move · tab focus · / filter · esc clear filter · q quit"
	case m.focus == paneDetail:
		hint = "j/k scroll · tab focus · / filter · q quit"
	default:
		hint = "j/k move · tab focus · / filter · q quit"
	}
	return dimStyle.Render(ansi.Truncate(" "+hint, m.width, "…"))
}

func (m *model) panes() string {
	lw, dw, h := m.listWidth(), m.detailWidth(), m.innerHeight()

	listTitle := "Skills"
	if m.filter != "" {
		listTitle = fmt.Sprintf("Skills %d/%d", len(m.visible), len(m.report.Skills))
	}
	var detailTitle string
	if i := m.selected(); i >= 0 {
		detailTitle = m.report.Skills[i].DisplayName()
	}

	left := box(listTitle, "", m.listLines(), lw, h, m.focus == paneList)
	right := box("", detailTitle, strings.Split(m.vp.View(), "\n"), dw, h, m.focus == paneDetail)

	rows := make([]string, len(left))
	for i := range left {
		rows[i] = left[i] + right[i]
	}
	return strings.Join(rows, "\n")
}

// listLines renders the list pane content: optional filter row, then skill rows.
func (m *model) listLines() []string {
	w := m.listInnerWidth()
	var lines []string
	if m.filterRowShown() {
		if m.filtering {
			lines = append(lines, ansi.Truncate("/ "+m.filter+"▏", w, ""))
		} else {
			lines = append(lines, accentStyle.Render(ansi.Truncate("/ "+m.filter, w, "…")))
		}
	}
	if len(m.visible) == 0 {
		msg := "No skills found"
		switch {
		case len(m.report.Skills) > 0:
			msg = "No matches"
		case m.report.Excluded > 0:
			msg += fmt.Sprintf(" (%d excluded)", m.report.Excluded)
		}
		return append(lines, dimStyle.Render(msg))
	}
	end := min(m.offset+m.listRows(), len(m.visible))
	for pos := m.offset; pos < end; pos++ {
		lines = append(lines, m.row(pos, w))
	}
	return lines
}

func (m *model) row(pos, w int) string {
	s := m.report.Skills[m.visible[pos]]
	prefix := "  "
	if pos == m.cursor {
		prefix = "> "
	}
	name := s.DisplayName()
	if s.Valid() {
		text := ansi.Truncate(prefix+name, w, "…")
		if pos == m.cursor {
			return selectStyle.Render(text)
		}
		return text
	}

	const badge = "[invalid]"
	name = ansi.Truncate("! "+name, max(w-len(prefix)-len(badge)-1, 1), "…")
	text := prefix + name
	pad := max(w-ansi.StringWidth(text)-len(badge), 1)
	style := warnStyle
	if pos == m.cursor {
		style = selectStyle
	}
	return style.Render(text) + strings.Repeat(" ", pad) + warnStyle.Render(badge)
}

// box draws a bordered pane of outer width w and inner height h, with titles in the top border.
func box(left, right string, lines []string, w, h int, focused bool) []string {
	border := dimStyle
	if focused {
		border = accentStyle
	}
	inner := w - 2

	title := func(t string, limit int) string {
		if t == "" {
			return ""
		}
		return " " + ansi.Truncate(t, max(limit-2, 1), "…") + " "
	}
	l := title(left, inner)
	r := title(right, inner-ansi.StringWidth(l))
	fill := max(inner-ansi.StringWidth(l)-ansi.StringWidth(r), 0)

	out := make([]string, 0, h+2)
	out = append(out, border.Render("┌")+boldStyle.Render(l)+border.Render(strings.Repeat("─", fill))+boldStyle.Render(r)+border.Render("┐"))
	side := border.Render("│")
	for i := range h {
		var line string
		if i < len(lines) {
			line = ansi.Truncate(lines[i], inner-2, "")
		}
		pad := max(inner-2-ansi.StringWidth(line), 0)
		out = append(out, side+" "+line+strings.Repeat(" ", pad)+" "+side)
	}
	out = append(out, border.Render("└"+strings.Repeat("─", inner)+"┘"))
	return out
}
