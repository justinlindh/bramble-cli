package tui

import "charm.land/lipgloss/v2"

// Theme holds styles for the IRC-style TUI.
type Theme struct {
	ScrollTheme ScrollTheme
	StatusBar   StatusBarStyle
	Input       InputStyle
}

// DefaultTheme returns the default IRC-style theme.
func DefaultTheme() Theme {
	return Theme{
		ScrollTheme: NewScrollTheme(),
	}
}

var _ = lipgloss.NewStyle() // keep import
