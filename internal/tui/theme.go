package tui

import (
	"charm.land/lipgloss/v2"
)

// Theme holds all Lip Gloss styles for the TUI.
type Theme struct {
	TabActive   lipgloss.Style
	TabInactive lipgloss.Style
	TabBar      lipgloss.Style
	Header      lipgloss.Style
	Footer      lipgloss.Style
	Content     lipgloss.Style
	Border      lipgloss.Style
	StatusOK    lipgloss.Style
	StatusErr   lipgloss.Style
}

// DefaultTheme returns the default dark theme.
func DefaultTheme() Theme {
	return Theme{
		TabActive: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00FF87")).
			Background(lipgloss.Color("#1a1a2e")).
			Padding(0, 2).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(lipgloss.Color("#00FF87")),

		TabInactive: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Background(lipgloss.Color("#0d0d1a")).
			Padding(0, 2),

		TabBar: lipgloss.NewStyle().
			Background(lipgloss.Color("#0d0d1a")).
			BorderBottom(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#333355")),

		Header: lipgloss.NewStyle().
			Background(lipgloss.Color("#0d0d1a")).
			Foreground(lipgloss.Color("#aaaacc")).
			Padding(0, 1).
			Width(0),

		Footer: lipgloss.NewStyle().
			Background(lipgloss.Color("#0d0d1a")).
			Foreground(lipgloss.Color("#555577")).
			Padding(0, 1).
			Width(0),

		Content: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ccccdd")).
			Padding(1, 2),

		Border: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#333355")),

		StatusOK: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF87")).
			Bold(true),

		StatusErr: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555")).
			Bold(true),
	}
}
