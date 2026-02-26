// Package widgets provides reusable TUI components for bramble.
package widgets

import (
	"time"

	"charm.land/lipgloss/v2"
)

// ToastKind determines the styling of a toast message.
type ToastKind int

const (
	ToastInfo ToastKind = iota
	ToastSuccess
	ToastError
	ToastWarning
)

// toastDuration is how long a toast is displayed.
const toastDuration = 3 * time.Second

// Toast represents a single notification message.
type Toast struct {
	Kind    ToastKind
	Message string
	Expire  time.Time
}

// StatusLine manages the bottom status bar: key hints (left) and toasts (right).
type StatusLine struct {
	toasts []Toast
	width  int

	styleInfo    lipgloss.Style
	styleSuccess lipgloss.Style
	styleError   lipgloss.Style
	styleWarning lipgloss.Style
	styleHints   lipgloss.Style
	styleBar     lipgloss.Style
}

// NewStatusLine creates a StatusLine with default styles.
func NewStatusLine() StatusLine {
	return StatusLine{
		styleInfo:    lipgloss.NewStyle().Foreground(lipgloss.Color("#ccccdd")),
		styleSuccess: lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF87")).Bold(true),
		styleError:   lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Bold(true),
		styleWarning: lipgloss.NewStyle().Foreground(lipgloss.Color("#FFAA00")).Bold(true),
		styleHints:   lipgloss.NewStyle().Foreground(lipgloss.Color("#555577")),
		styleBar:     lipgloss.NewStyle().Background(lipgloss.Color("#0d0d1a")).Foreground(lipgloss.Color("#555577")).Padding(0, 1),
	}
}

// SetWidth sets the rendered width.
func (s *StatusLine) SetWidth(w int) { s.width = w }

// AddToast enqueues a new toast message.
func (s *StatusLine) AddToast(kind ToastKind, msg string) {
	s.toasts = append(s.toasts, Toast{
		Kind:    kind,
		Message: msg,
		Expire:  time.Now().Add(toastDuration),
	})
}

// Tick removes expired toasts. Call on each render or tick.
func (s *StatusLine) Tick() {
	now := time.Now()
	kept := s.toasts[:0]
	for _, t := range s.toasts {
		if now.Before(t.Expire) {
			kept = append(kept, t)
		}
	}
	s.toasts = kept
}

// HasActiveToast returns true if there's at least one non-expired toast.
func (s *StatusLine) HasActiveToast() bool {
	return len(s.toasts) > 0
}

// Render returns the rendered status line string.
func (s *StatusLine) Render(hints string) string {
	s.Tick()

	left := s.styleHints.Render(hints)

	var right string
	if len(s.toasts) > 0 {
		// Show most recent (last) toast.
		t := s.toasts[len(s.toasts)-1]
		switch t.Kind {
		case ToastSuccess:
			right = s.styleSuccess.Render("✓ " + t.Message)
		case ToastError:
			right = s.styleError.Render("✗ " + t.Message)
		case ToastWarning:
			right = s.styleWarning.Render("⚠ " + t.Message)
		default:
			right = s.styleInfo.Render("ℹ " + t.Message)
		}
	}

	// Pad between left and right.
	leftLen := lipgloss.Width(left)
	rightLen := lipgloss.Width(right)
	pad := s.width - leftLen - rightLen - 2 // -2 for padding
	if pad < 1 {
		pad = 1
	}
	padStr := ""
	for i := 0; i < pad; i++ {
		padStr += " "
	}

	content := left + padStr + right
	return s.styleBar.Width(s.width).Render(content)
}
