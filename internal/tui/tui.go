// Package tui renders scan results in an interactive terminal UI.
package tui

import (
	tea "charm.land/bubbletea/v2"

	"skill-atlas/internal/skill"
)

// Report is everything the TUI shows. Built in memory before the TUI starts.
type Report struct {
	Repo   string // repository display name, e.g. "github.com/org/repo"
	Ref    string // branch or tag name
	SHA    string // full commit SHA
	Skills []skill.Skill
	// Excluded counts SKILL.md files the scan skipped.
	Excluded int
}

// Run shows r until the user quits.
func Run(r Report) error {
	_, err := tea.NewProgram(newModel(r)).Run()
	return err
}
