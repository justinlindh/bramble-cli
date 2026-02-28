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
	// NickAddr is the raw hex address of the sender (for click-to-DM).
	// Empty for non-chat lines.
	NickAddr string
}

type Scrollback struct {
	lines      []ScrollLine
	viewport   viewport.Model
	width      int
	height     int
	theme      ScrollTheme
	autoscroll bool // track if user is at bottom

	deliveryGroups map[string]int
	deliveryItems  map[string][]string

	// clickMap is rebuilt on each rebuildContent for nick hit testing.
	clickMap ClickMap
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
	Action    lipgloss.Style
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
		Action:    lipgloss.NewStyle().Foreground(lipgloss.Color("#da77f2")).Italic(true),
	}
}

func NewScrollback() Scrollback {
	vp := viewport.New()
	return Scrollback{
		viewport:       vp,
		theme:          NewScrollTheme(),
		autoscroll:     true,
		deliveryGroups: make(map[string]int),
		deliveryItems:  make(map[string][]string),
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
// sender is the resolved name, addr is the raw hex address (for short suffix).
func (s *Scrollback) AddChat(sender, addr string, text, badge string, outgoing bool) {
	s.AddChatAt(time.Now(), sender, addr, text, badge, outgoing)
}

func (s *Scrollback) addChatWithTimestamp(ts time.Time, sender, addr string, text, badge string, outgoing bool) {
	tsLabel := s.theme.Timestamp.Render(fmt.Sprintf("[%s]", ts.Format("15:04")))

	// Check for CTCP ACTION: ACTION text
	if actionText, ok := parseAction(text); ok {
		s.addActionLine(tsLabel, sender, actionText, outgoing)
		return
	}

	// Build IRC-style nick tag from pre-resolved sender label.
	nick := sender

	var line string
	if outgoing {
		nickStr := s.theme.SelfBadge.Render("<" + nick + ">")
		line = fmt.Sprintf("%s %s %s %s", tsLabel, nickStr, text, s.theme.SelfBadge.Render(badge))
	} else {
		nickStr := s.theme.Sender.Render("<" + nick + ">")
		line = fmt.Sprintf("%s %s %s", tsLabel, nickStr, text)
	}
	kind := LineChat
	if outgoing {
		kind = LineChatOut
	}
	s.lines = append(s.lines, ScrollLine{Kind: kind, Timestamp: ts, Text: line, NickAddr: addr})
	s.rebuildContent()
	if s.autoscroll {
		s.viewport.GotoBottom()
	}
}

// parseAction detects CTCP ACTION format: \x01ACTION text\x01
func parseAction(text string) (string, bool) {
	if len(text) > 9 && text[0] == '\x01' && strings.HasPrefix(text[1:], "ACTION ") {
		action := text[8:]
		if len(action) > 0 && action[len(action)-1] == '\x01' {
			action = action[:len(action)-1]
		}
		return action, true
	}
	return "", false
}

// addActionLine renders an IRC /me action: * Nick does something
func (s *Scrollback) addActionLine(ts, sender, actionText string, outgoing bool) {
	nick := sender

	// Render as: [12:42] * Nick does something
	star := s.theme.Action.Render("*")
	nickStr := s.theme.Action.Render(nick)
	actionStr := s.theme.Action.Render(actionText)
	line := fmt.Sprintf("%s %s %s %s", ts, star, nickStr, actionStr)

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

// AddDeliveryGrouped appends delivery details into a stable line per group key.
func (s *Scrollback) AddDeliveryGrouped(groupKey, text string) {
	if groupKey == "" {
		s.AddDelivery(text)
		return
	}
	items := append(s.deliveryItems[groupKey], text)
	s.deliveryItems[groupKey] = items
	rendered := s.theme.Delivery.Render("-- " + strings.Join(items, "  ") + " --")
	if idx, ok := s.deliveryGroups[groupKey]; ok && idx >= 0 && idx < len(s.lines) {
		s.lines[idx].Text = rendered
		s.rebuildContent()
		if s.autoscroll {
			s.viewport.GotoBottom()
		}
		return
	}
	line := ScrollLine{Kind: LineDelivery, Timestamp: time.Now(), Text: rendered}
	s.lines = append(s.lines, line)
	s.deliveryGroups[groupKey] = len(s.lines) - 1
	s.rebuildContent()
	if s.autoscroll {
		s.viewport.GotoBottom()
	}
}

// AddStoredLine appends a pre-rendered line preserving original timestamp/kind.
func (s *Scrollback) AddStoredLine(line ScrollLine) {
	s.lines = append(s.lines, line)
	s.rebuildContent()
	if s.autoscroll {
		s.viewport.GotoBottom()
	}
}

// Clear removes all lines.
func (s *Scrollback) Clear() {
	s.lines = nil
	s.deliveryGroups = make(map[string]int)
	s.deliveryItems = make(map[string][]string)
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
	s.clickMap.Reset()

	var sb strings.Builder
	for i, line := range s.lines {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(line.Text)

		// Build click map for chat lines with a known sender address.
		if line.NickAddr != "" && (line.Kind == LineChat || line.Kind == LineChatOut) {
			// The format is: [HH:MM] <nick> text
			// Timestamp "[HH:MM]" has visual width 7, then space = col 8.
			// The nick tag starts at col 8 with "<" and ends after ">".
			// We use lipgloss.Width to find the nick tag width from the rendered text.
			// Simpler: just find the visual position of < and > in the line.
			text := line.Text
			// Strip ANSI to find visual positions of < and >
			plain := stripAnsi(text)
			ltIdx := strings.Index(plain, "<")
			gtIdx := strings.Index(plain, ">")
			if ltIdx >= 0 && gtIdx > ltIdx {
				s.clickMap.AddNick(i, ltIdx, gtIdx+1, line.NickAddr)
			}
		}
	}
	s.viewport.SetContent(sb.String())
}

// HitTestNick returns the address at viewport-relative row/col, or "".
func (s *Scrollback) HitTestNick(viewRow, col int) string {
	// Convert viewport row to content row.
	contentRow := viewRow + s.viewport.YOffset()
	return s.clickMap.HitTestNick(contentRow, col)
}

// LineCount returns the number of lines in the scrollback.
func (s *Scrollback) LineCount() int {
	return len(s.lines)
}

// IsScrolled returns true if the user has scrolled up (autoscroll is off).
func (s *Scrollback) IsScrolled() bool {
	return !s.autoscroll
}

// AddChatAt adds a formatted chat message line with explicit timestamp metadata.
func (s *Scrollback) AddChatAt(ts time.Time, sender, addr, text, badge string, outgoing bool) {
	s.addChatWithTimestamp(ts, sender, addr, text, badge, outgoing)
}

// stripAnsi removes ANSI escape sequences from a string.
func stripAnsi(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			// Skip until we find the terminator (a letter)
			j := i + 2
			for j < len(s) && !((s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= 'a' && s[j] <= 'z')) {
				j++
			}
			if j < len(s) {
				j++ // skip the terminator
			}
			i = j
		} else {
			out.WriteByte(s[i])
			i++
		}
	}
	return out.String()
}
