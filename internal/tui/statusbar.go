package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// BufferInfo represents a buffer for the status bar display.
type BufferInfo struct {
	ID     string
	Label  string
	Unread int
	Active bool
}

// StatusBar renders the IRC-style status line.
type StatusBar struct {
	width     int
	connected bool
	nodeAddr  string
	nodeName  string
	peerCount int
	buffers   []BufferInfo
	scrolled  bool
	style     StatusBarStyle
}

type StatusBarStyle struct {
	Bar      lipgloss.Style
	ConnOK   lipgloss.Style
	ConnFail lipgloss.Style
	Active   lipgloss.Style
	Inactive lipgloss.Style
	Unread   lipgloss.Style
	Info     lipgloss.Style
	Clock    lipgloss.Style
}

func NewStatusBar() StatusBar {
	return StatusBar{
		style: StatusBarStyle{
			Bar: lipgloss.NewStyle().
				Background(lipgloss.Color("#1a1a3e")).
				Foreground(lipgloss.Color("#aaaacc")).
				Width(0),
			ConnOK: lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00FF87")).
				Background(lipgloss.Color("#1a1a3e")).
				Bold(true),
			ConnFail: lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF5555")).
				Background(lipgloss.Color("#1a1a3e")).
				Bold(true),
			Active: lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00FF87")).
				Background(lipgloss.Color("#1a1a3e")).
				Bold(true),
			Inactive: lipgloss.NewStyle().
				Foreground(lipgloss.Color("#666688")).
				Background(lipgloss.Color("#1a1a3e")),
			Unread: lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFAA00")).
				Background(lipgloss.Color("#1a1a3e")).
				Bold(true),
			Info: lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888899")).
				Background(lipgloss.Color("#1a1a3e")),
			Clock: lipgloss.NewStyle().
				Foreground(lipgloss.Color("#666688")).
				Background(lipgloss.Color("#1a1a3e")),
		},
	}
}

func (sb *StatusBar) SetWidth(w int) {
	sb.width = w
}

func (sb *StatusBar) SetConnection(connected bool, addr, name string) {
	sb.connected = connected
	sb.nodeAddr = addr
	sb.nodeName = name
}

func (sb *StatusBar) SetPeerCount(n int) {
	sb.peerCount = n
}

func (sb *StatusBar) SetBuffers(bufs []BufferInfo) {
	sb.buffers = bufs
}

func (sb *StatusBar) SetScrolled(scrolled bool) {
	sb.scrolled = scrolled
}

func (sb StatusBar) View() string {
	var parts []string

	// Connection status
	if sb.connected {
		parts = append(parts, sb.style.ConnOK.Render("●"))
	} else {
		parts = append(parts, sb.style.ConnFail.Render("○"))
	}

	// Buffer indicators: [1:all] [2:@lily(3079) +3]
	for i, buf := range sb.buffers {
		label := fmt.Sprintf("%d:%s", i+1, buf.Label)
		if buf.Active {
			parts = append(parts, sb.style.Active.Render("["+label+"]"))
		} else if buf.Unread > 0 {
			parts = append(parts, sb.style.Unread.Render(fmt.Sprintf("[%s +%d]", label, buf.Unread)))
		} else {
			parts = append(parts, sb.style.Inactive.Render("["+label+"]"))
		}
	}

	// Node identity
	nodeStr := sb.nodeAddr
	if sb.nodeName != "" {
		nodeStr = fmt.Sprintf("%s (%s)", sb.nodeAddr, sb.nodeName)
	}
	parts = append(parts, sb.style.Info.Render(nodeStr))

	// Peer count
	parts = append(parts, sb.style.Info.Render(fmt.Sprintf("%d peers", sb.peerCount)))

	// Scroll indicator
	if sb.scrolled {
		scrollStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFAA00")).
			Background(lipgloss.Color("#1a1a3e")).
			Bold(true)
		parts = append(parts, scrollStyle.Render("[↓ new]"))
	}

	left := strings.Join(parts, " ")

	// Clock on right
	clock := sb.style.Clock.Render(time.Now().Format("15:04"))

	// Pad middle
	leftW := lipgloss.Width(left)
	clockW := lipgloss.Width(clock)
	padW := sb.width - leftW - clockW - 1
	if padW < 1 {
		padW = 1
	}
	pad := sb.style.Info.Render(strings.Repeat(" ", padW))

	return sb.style.Bar.Width(sb.width).Render(left + pad + clock)
}
