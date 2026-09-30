// Package tui renders scan results in an interactive terminal UI.
package tui

import (
	tea "charm.land/bubbletea/v2"

	"skill-atlas/internal/skill"
)

// Repo is one scanned repository.
type Repo struct {
	Name   string // repository display name, e.g. "github.com/org/repo"
	Ref    string // branch or tag name
	SHA    string // full commit SHA
	Skills []skill.Skill
	// Excluded counts SKILL.md files the scan skipped.
	Excluded int
	// Err is why the clone or scan failed; it may contain remote text. Empty when the repository was scanned.
	Err string
}

// Report is everything the TUI shows. Built in memory before the TUI starts.
// With 2 or more repositories the list groups skills by repository.
type Report struct {
	Repos []Repo
}

// label is the "name @ ref (sha7)" text that identifies a repository, without empty parts.
func (r Repo) label() string {
	label := r.Name
	if r.Ref != "" {
		label += " @ " + r.Ref
	}
	if r.SHA != "" {
		label += " (" + r.SHA[:min(7, len(r.SHA))] + ")"
	}
	return label
}

// Run shows r until the user quits.
func Run(r Report) error {
	_, err := tea.NewProgram(newModel(r)).Run()
	return err
}
