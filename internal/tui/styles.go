package tui

import "charm.land/lipgloss/v2"

var (
	accentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	selectStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	boldStyle   = lipgloss.NewStyle().Bold(true)
)
