package tui

import (
	"fmt"
	"strings"

	"charm.land/glamour/v2"
	"github.com/charmbracelet/x/ansi"

	"skill-atlas/internal/skill"
)

type cacheKey struct {
	path  string
	width int
	dark  bool
}

// refreshDetail rebuilds the viewport content; toTop resets the scroll position.
func (m *model) refreshDetail(toTop bool) {
	w := m.vp.Width()
	if w <= 0 {
		return
	}
	off := m.vp.YOffset()
	m.vp.SetContent(m.detailContent(w))
	if toTop {
		m.vp.GotoTop()
	} else {
		m.vp.SetYOffset(off)
	}
}

func (m *model) detailContent(w int) string {
	i := m.selected()
	if i < 0 {
		if len(m.report.Skills) == 0 {
			return dimStyle.Render("Nothing to show.")
		}
		return dimStyle.Render("No skill selected.")
	}
	s := m.report.Skills[i]

	var b strings.Builder
	b.WriteString(dimStyle.Render(ansi.Truncate(s.Path, w, "…")))
	b.WriteString("\n\n")
	if s.Description != "" {
		b.WriteString(ansi.Wrap(s.Description, w, ""))
		b.WriteString("\n\n")
	}
	if f := fieldLines(s, w); f != "" {
		b.WriteString(f)
		b.WriteString("\n\n")
	}
	if !s.Valid() {
		b.WriteString(warnStyle.Bold(true).Render("Invalid"))
		b.WriteString("\n")
		for _, e := range s.Errors {
			b.WriteString(bullet(e, w))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if strings.TrimSpace(s.Body) != "" {
		b.WriteString(dimStyle.Render(strings.Repeat("─", w)))
		b.WriteString("\n")
		b.WriteString(m.renderBody(s, w))
	}
	return strings.TrimRight(b.String(), "\n")
}

func fieldLines(s skill.Skill, w int) string {
	var lines []string
	add := func(k, v string) {
		if v != "" {
			lines = append(lines, ansi.Wrap(k+": "+v, w, ""))
		}
	}
	add("license", s.License)
	add("compatibility", s.Compatibility)
	add("allowed-tools", s.AllowedTools)
	if len(s.Metadata) > 0 {
		lines = append(lines, "metadata:")
		for _, e := range s.Metadata {
			lines = append(lines, ansi.Wrap(fmt.Sprintf("  %s: %s", e.Key, e.Value), w, ""))
		}
	}
	return strings.Join(lines, "\n")
}

func bullet(text string, w int) string {
	wrapped := ansi.Wrap(text, max(w-2, 1), "")
	lines := strings.Split(wrapped, "\n")
	for i, l := range lines {
		if i == 0 {
			lines[i] = "• " + l
		} else {
			lines[i] = "  " + l
		}
	}
	return strings.Join(lines, "\n")
}

// renderBody renders the Markdown body with glamour, cached per skill, width and style.
func (m *model) renderBody(s skill.Skill, w int) string {
	key := cacheKey{s.Path, w, m.dark}
	if out, ok := m.cache[key]; ok {
		return out
	}
	out, err := renderMarkdown(s.Body, w, m.dark)
	if err != nil {
		out = ansi.Wrap(strings.TrimSpace(s.Body), w, "")
	}
	m.cache[key] = out
	return out
}

func renderMarkdown(body string, w int, dark bool) (string, error) {
	style := "light"
	if dark {
		style = "dark"
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(w),
	)
	if err != nil {
		return "", err
	}
	out, err := r.Render(strings.ReplaceAll(body, "\t", "    "))
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.Trim(out, "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(ansi.Truncate(strings.ReplaceAll(l, "\t", "    "), w, ""), " ")
	}
	return strings.Join(lines, "\n"), nil
}
