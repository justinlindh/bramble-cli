// Package tui — scrollback.go
// A scrollback buffer that wraps a Bubbles viewport and manages styled lines.

package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	"charm.land/lipgloss/v2"
)

type LineKind int

const (
	LineChat     LineKind = iota // incoming message
	LineChatOut                  // outgoing message
	LineSystem                   // system event (join/part/connect/disconnect)
	LineDelivery                 // delivery receipt summary
	LineError                    // error message
	LineInfo                     // informational (command output)
	LineCommand                  // slash command echo
)

type ScrollLine struct {
	Kind      LineKind
	Timestamp time.Time
	Text      string // pre-rendered ANSI string
}

type Scrollback struct {
	lines      []ScrollLine
	viewport   viewport.Model
	width      int
	height     int
	theme      ScrollTheme
	autoscroll bool // track if user is at bottom
}

type ScrollTheme struct {
	Timestamp lipgloss.Style
	Incoming  lipgloss.Style
	Outgoing  lipgloss.Style
	System    lipgloss.Style
	Delivery  lipgloss.Style
	Error     lipgloss.Style
	Info      lipgloss.Style
	Command   lipgloss.Style
	Sender    lipgloss.Style
	SelfBadge lipgloss.Style
}

func NewScrollTheme() ScrollTheme {
	return ScrollTheme{
		Timestamp: lipgloss.NewStyle().Foreground(lipgloss.Color("#666688")),
		Incoming:  lipgloss.NewStyle().Foreground(lipgloss.Color("#ccccdd")),
		Outgoing:  lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF87")),
		System:    lipgloss.NewStyle().Foreground(lipgloss.Color("#FFAA00")).Faint(true),
		Delivery:  lipgloss.NewStyle().Foreground(lipgloss.Color("#5599FF")).Faint(true),
		Error:     lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Bold(true),
		Info:      lipgloss.NewStyle().Foreground(lipgloss.Color("#aaaacc")),
		Command:   lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Faint(true),
		Sender:    lipgloss.NewStyle().Foreground(lipgloss.Color("#5599FF")).Bold(true),
		SelfBadge: lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF87")),
	}
}

func NewScrollback() Scrollback {
	vp := viewport.New()
	return Scrollback{
		viewport:   vp,
		theme:      NewScrollTheme(),
		autoscroll: true,
	}
}

func (s *Scrollback) SetSize(w, h int) {
	s.width = w
	s.height = h
	s.viewport.SetWidth(w)
	s.viewport.SetHeight(h)
	s.rebuildContent()
}

// AddLine appends a styled line to the scrollback.
func (s *Scrollback) AddLine(kind LineKind, text string) {
	line := ScrollLine{
		Kind:      kind,
		Timestamp: time.Now(),
		Text:      text,
	}
	s.lines = append(s.lines, line)
	s.rebuildContent()
	if s.autoscroll {
		s.viewport.GotoBottom()
	}
}

// AddChat adds a formatted chat message line.
func (s *Scrollback) AddChat(sender, text, badge string, outgoing bool) {
	ts := s.theme.Timestamp.Render(fmt.Sprintf("[%s]", time.Now().Format("15:04")))
	var line string
	if outgoing {
		line = fmt.Sprintf("> %s %s %s", ts, text, s.theme.SelfBadge.Render(badge))
	} else {
		senderStr := s.theme.Sender.Render(sender + ":")
		line = fmt.Sprintf("< %s %s %s", ts, senderStr, text)
	}
	kind := LineChat
	if outgoing {
		kind = LineChatOut
	}
	s.AddLine(kind, line)
}

// AddSystem adds a system event line.
func (s *Scrollback) AddSystem(text string) {
	rendered := s.theme.System.Render("-- " + text + " --")
	s.AddLine(LineSystem, rendered)
}

// AddError adds an error line.
func (s *Scrollback) AddError(text string) {
	rendered := s.theme.Error.Render("!! " + text)
	s.AddLine(LineError, rendered)
}

// AddInfo adds an info/command-output line.
func (s *Scrollback) AddInfo(text string) {
	rendered := s.theme.Info.Render(text)
	s.AddLine(LineInfo, rendered)
}

// AddDelivery adds a delivery receipt summary.
func (s *Scrollback) AddDelivery(text string) {
	rendered := s.theme.Delivery.Render("-- " + text + " --")
	s.AddLine(LineDelivery, rendered)
}

// Clear removes all lines.
func (s *Scrollback) Clear() {
	s.lines = nil
	s.rebuildContent()
}

// View returns the viewport view.
func (s *Scrollback) View() string {
	return s.viewport.View()
}

// Update forwards messages to the viewport (for scroll keys).
func (s *Scrollback) Update(msg interface{}) {
	s.viewport, _ = s.viewport.Update(msg)
	s.autoscroll = s.viewport.AtBottom()
}

func (s *Scrollback) rebuildContent() {
	var sb strings.Builder
	for i, line := range s.lines {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(line.Text)
	}
	s.viewport.SetContent(sb.String())
}

// LineCount returns the number of lines in the scrollback.
func (s *Scrollback) LineCount() int {
	return len(s.lines)
}

// IsScrolled returns true if the user has scrolled up (autoscroll is off).
func (s *Scrollback) IsScrolled() bool {
	return !s.autoscroll
}
