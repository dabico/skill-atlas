package tui

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"skill-atlas/internal/skill"
)

const (
	minWidth  = 60
	minHeight = 12
)

type pane int

const (
	paneList pane = iota
	paneDetail
)

type model struct {
	report  Report
	invalid int

	visible []int // indices into report.Skills that match the filter
	cursor  int   // position within visible
	offset  int   // first visible list row

	focus     pane
	filtering bool
	filter    string

	vp    viewport.Model
	cache map[cacheKey]string
	dark  bool

	width, height int
}

func newModel(r Report) *model {
	m := &model{
		report: r,
		vp:     viewport.New(),
		cache:  map[cacheKey]string{},
		dark:   true,
	}
	for _, s := range r.Skills {
		if !s.Valid() {
			m.invalid++
		}
	}
	m.applyFilter()
	return m
}

func (m *model) Init() tea.Cmd { return tea.RequestBackgroundColor }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.relayout()
	case tea.BackgroundColorMsg:
		m.dark = msg.IsDark()
		m.refreshDetail(false)
	case tea.MouseWheelMsg:
		if !m.tooSmall() && msg.X >= m.listWidth() {
			m.vp, _ = m.vp.Update(msg)
		}
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.filtering {
			return m, m.updateFilterKey(msg)
		}
		return m, m.updateNavKey(msg)
	}
	return m, nil
}

func (m *model) updateNavKey(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()
	switch key {
	case "q":
		return tea.Quit
	case "tab":
		m.focus = 1 - m.focus
		return nil
	case "/":
		m.focus = paneList
		m.filtering = true
		m.clampOffset()
		m.refreshDetail(false)
		return nil
	case "esc":
		if m.filter != "" {
			m.clearFilter()
		}
		return nil
	}
	if m.focus == paneList {
		m.listKey(key)
	} else {
		m.detailKey(key)
	}
	return nil
}

func (m *model) listKey(key string) {
	switch key {
	case "j", "down":
		m.setCursor(m.cursor + 1)
	case "k", "up":
		m.setCursor(m.cursor - 1)
	case "g", "home":
		m.setCursor(0)
	case "G", "end":
		m.setCursor(len(m.visible) - 1)
	case "pgdown":
		m.setCursor(m.cursor + m.listRows())
	case "pgup":
		m.setCursor(m.cursor - m.listRows())
	}
}

func (m *model) detailKey(key string) {
	switch key {
	case "j", "down":
		m.vp.ScrollDown(1)
	case "k", "up":
		m.vp.ScrollUp(1)
	case "pgdown", "space":
		m.vp.PageDown()
	case "pgup":
		m.vp.PageUp()
	case "g", "home":
		m.vp.GotoTop()
	case "G", "end":
		m.vp.GotoBottom()
	}
}

func (m *model) updateFilterKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		m.filtering = false
		m.relayout()
		return nil
	case "esc":
		m.clearFilter()
		return nil
	}
	old := m.filter
	switch {
	case msg.String() == "backspace":
		if r := []rune(m.filter); len(r) > 0 {
			m.filter = string(r[:len(r)-1])
		}
	case msg.String() == "ctrl+u":
		m.filter = ""
	case msg.Text != "":
		m.filter += msg.Text
	}
	if m.filter != old {
		m.applyFilter()
	}
	return nil
}

func (m *model) clearFilter() {
	m.filter = ""
	m.filtering = false
	m.applyFilter()
}

// selected returns the index into report.Skills of the current row, or -1.
func (m *model) selected() int {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return -1
	}
	return m.visible[m.cursor]
}

// applyFilter recomputes visible, keeping the current skill selected when it still matches.
func (m *model) applyFilter() {
	prev := m.selected()
	q := strings.ToLower(m.filter)
	m.visible = m.visible[:0]
	for i, s := range m.report.Skills {
		if q == "" || matches(s, q) {
			m.visible = append(m.visible, i)
		}
	}
	m.cursor = 0
	for i, idx := range m.visible {
		if idx == prev {
			m.cursor = i
			break
		}
	}
	m.clampOffset()
	m.refreshDetail(m.selected() != prev)
}

func matches(s skill.Skill, q string) bool {
	for _, f := range []string{s.DisplayName(), s.Description, s.Path} {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}

func (m *model) setCursor(n int) {
	n = max(0, min(n, len(m.visible)-1))
	if n == m.cursor {
		return
	}
	m.cursor = n
	m.clampOffset()
	m.refreshDetail(true)
}

// clampOffset scrolls the list so the cursor stays visible.
func (m *model) clampOffset() {
	rows := max(m.listRows(), 1)
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
	m.offset = max(0, min(m.offset, max(len(m.visible)-rows, 0)))
}

func (m *model) relayout() {
	m.vp.SetWidth(m.detailInnerWidth())
	m.vp.SetHeight(m.innerHeight())
	m.clampOffset()
	m.refreshDetail(false)
}

func (m *model) tooSmall() bool { return m.width < minWidth || m.height < minHeight }
