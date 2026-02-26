// Package tabs contains individual tab models for the Bramble TUI.
package tabs

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	bramble "github.com/justinlindh/bramble-go"
)

// SortColumn identifies which column to sort neighbors by.
type SortColumn int

const (
	SortByAddress  SortColumn = iota
	SortByRSSI                // descending (higher = better)
	SortByLastSeen            // ascending (most recent first)
	sortColumnCount
)

// SwitchToChatMsg is sent when the user presses 'm' on a neighbor,
// requesting that the TUI switch to the Chat tab with a dm conversation.
type SwitchToChatMsg struct {
	Conversation string // e.g. "dm:AABBCCDD"
}

// NodesRefreshMsg triggers a data reload.
type NodesRefreshMsg struct{}

// NodesDataMsg carries updated neighbor/route data from the root model.
type NodesDataMsg struct {
	Neighbors []bramble.Neighbor
	Routes    []bramble.Route
}

// NodesModel manages the Nodes tab state.
type NodesModel struct {
	client    *bramble.Client
	neighbors []bramble.Neighbor
	routes    []bramble.Route
	selected  int
	sortCol   SortColumn
	width     int
	height    int
	resolver  PeerResolver

	// alias edit state
	editingAlias bool
	aliasInput   string

	// styles
	styleHeader    lipgloss.Style
	styleSelected  lipgloss.Style
	styleNormal    lipgloss.Style
	styleDot       map[string]lipgloss.Style
	styleRouteState map[string]lipgloss.Style
}

// SetResolver attaches a name resolver to the nodes tab.
func (m *NodesModel) SetResolver(r PeerResolver) {
	m.resolver = r
}

// NewNodes creates a new NodesModel.
func NewNodes(client *bramble.Client) NodesModel {
	green := lipgloss.Color("#00FF87")
	yellow := lipgloss.Color("#FFD700")
	gray := lipgloss.Color("#888888")
	red := lipgloss.Color("#FF5555")
	blue := lipgloss.Color("#5599FF")

	return NodesModel{
		client:  client,
		sortCol: SortByLastSeen,
		styleHeader: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#aaaacc")).
			Underline(true),
		styleSelected: lipgloss.NewStyle().
			Background(lipgloss.Color("#1a1a3e")).
			Foreground(lipgloss.Color("#ffffff")).
			Bold(true),
		styleNormal: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ccccdd")),
		styleDot: map[string]lipgloss.Style{
			"online":    lipgloss.NewStyle().Foreground(green).Bold(true),
			"reachable": lipgloss.NewStyle().Foreground(yellow).Bold(true),
			"stale":     lipgloss.NewStyle().Foreground(gray),
		},
		styleRouteState: map[string]lipgloss.Style{
			"ACTIVE":      lipgloss.NewStyle().Foreground(green).Bold(true),
			"STALE":       lipgloss.NewStyle().Foreground(yellow),
			"BROKEN":      lipgloss.NewStyle().Foreground(red).Bold(true),
			"DISCOVERING": lipgloss.NewStyle().Foreground(blue),
		},
	}
}

// Init implements tea.Model.
func (m NodesModel) Init() tea.Cmd {
	return fetchNodesCmd(m.client)
}

// SetSize sets the available width/height.
func (m *NodesModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// Update implements tea.Model.
func (m NodesModel) Update(msg tea.Msg) (NodesModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case NodesRefreshMsg:
		return m, fetchNodesCmd(m.client)

	case NodesDataMsg:
		if msg.Neighbors != nil {
			m.neighbors = msg.Neighbors
		}
		if msg.Routes != nil {
			m.routes = msg.Routes
		}
		m.sortNeighbors()
		if m.selected >= len(m.neighbors) && len(m.neighbors) > 0 {
			m.selected = len(m.neighbors) - 1
		}

	case nodesDataMsg:
		m.neighbors = msg.neighbors
		m.routes = msg.routes
		m.sortNeighbors()
		// Clamp selection
		if m.selected >= len(m.neighbors) && len(m.neighbors) > 0 {
			m.selected = len(m.neighbors) - 1
		}

	case tea.KeyPressMsg:
		// Alias editing mode — capture characters.
		if m.editingAlias {
			switch msg.String() {
			case "enter":
				// Save alias
				if len(m.neighbors) > 0 && m.selected < len(m.neighbors) && m.resolver != nil {
					addr := m.neighbors[m.selected].Address
					_ = m.resolver.SetAlias(addr, m.aliasInput)
				}
				m.editingAlias = false
				m.aliasInput = ""
			case "esc", "ctrl+c":
				m.editingAlias = false
				m.aliasInput = ""
			case "backspace", "ctrl+h":
				if len(m.aliasInput) > 0 {
					m.aliasInput = m.aliasInput[:len(m.aliasInput)-1]
				}
			default:
				if t := msg.Text; t != "" {
					m.aliasInput += t
				}
			}
			return m, nil
		}

		switch msg.String() {
		case "j", "down":
			if m.selected < len(m.neighbors)-1 {
				m.selected++
			}
		case "k", "up":
			if m.selected > 0 {
				m.selected--
			}
		case "s":
			m.sortCol = (m.sortCol + 1) % sortColumnCount
			m.sortNeighbors()
		case "n":
			// Open alias edit for selected neighbor
			if len(m.neighbors) > 0 && m.selected < len(m.neighbors) {
				addr := m.neighbors[m.selected].Address
				existing := ""
				if m.resolver != nil {
					existing, _ = m.resolver.GetAlias(addr)
				}
				m.editingAlias = true
				m.aliasInput = existing
			}
		case "m":
			if len(m.neighbors) > 0 && m.selected < len(m.neighbors) {
				addr := m.neighbors[m.selected].Address
				return m, func() tea.Msg {
					return SwitchToChatMsg{Conversation: "dm:" + addr}
				}
			}
		}
	}
	return m, nil
}

// View renders the nodes tab.
func (m NodesModel) View() string {
	var sb strings.Builder

	// Neighbor section
	sb.WriteString(m.styleHeader.Render(fmt.Sprintf("Neighbors (%d)", len(m.neighbors))))
	sb.WriteString("\n")
	sb.WriteString(m.renderNeighborHeader())
	sb.WriteString("\n")
	if len(m.neighbors) == 0 {
		sb.WriteString(m.styleNormal.Render("  (no neighbors)"))
		sb.WriteString("\n")
	} else {
		for i, n := range m.neighbors {
			sb.WriteString(m.renderNeighborRow(i, n))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")

	// Route section
	sb.WriteString(m.styleHeader.Render(fmt.Sprintf("Routes (%d)", len(m.routes))))
	sb.WriteString("\n")
	sb.WriteString(m.renderRouteHeader())
	sb.WriteString("\n")
	if len(m.routes) == 0 {
		sb.WriteString(m.styleNormal.Render("  (no routes)"))
		sb.WriteString("\n")
	} else {
		for _, r := range m.routes {
			sb.WriteString(m.renderRouteRow(r))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")

	// Alias edit overlay
	if m.editingAlias && len(m.neighbors) > 0 && m.selected < len(m.neighbors) {
		addr := m.neighbors[m.selected].Address
		fwName := ""
		if m.resolver != nil {
			// Show firmware name if available for context
			resolved := m.resolver.Resolve(addr)
			if resolved != addr {
				fwName = " (fw: " + resolved + ")"
			}
		}
		sb.WriteString(m.styleSelected.Render(
			fmt.Sprintf("  Set alias for %s%s: %s▌", addr, fwName, m.aliasInput),
		))
		sb.WriteString("\n")
	} else {
		sortLabel := [sortColumnCount]string{"addr", "RSSI", "last-seen"}[m.sortCol]
		sb.WriteString(m.styleNormal.Faint(true).Render(
			fmt.Sprintf("  [j/k] select  [m] open chat  [n] set alias  [s] sort by %s", sortLabel),
		))
	}

	return sb.String()
}

func (m NodesModel) renderNeighborHeader() string {
	return m.styleNormal.Faint(true).Render(
		fmt.Sprintf("  %-3s  %-12s  %-16s  %6s  %6s  %s",
			"●", "Address", "Name", "RSSI", "SNR", "Last Seen"),
	)
}

func (m NodesModel) renderNeighborRow(idx int, n bramble.Neighbor) string {
	status, dot := neighborStatus(n.LastSeenAgoMs)
	dotStyle := m.styleDot[status]

	// Resolve name via resolver (alias > firmware name > short hex)
	r := m.resolver
	if r == nil {
		r = defaultResolver
	}
	name := truncateStr(r.Resolve(n.Address), 16)
	lastSeen := fmtDuration(time.Duration(n.LastSeenAgoMs) * time.Millisecond)
	snr := fmt.Sprintf("%.1f", n.SNR)

	dotRendered := dotStyle.Render(dot)
	line := fmt.Sprintf("  %s  %-12s  %-16s  %5ddBm  %6s  %s",
		dotRendered, truncateStr(n.Address, 12), name,
		n.RSSI, snr, lastSeen)

	if idx == m.selected {
		return m.styleSelected.Render(fmt.Sprintf("  %s  %-12s  %-16s  %5ddBm  %6s  %s",
			dot, truncateStr(n.Address, 12), name, n.RSSI, snr, lastSeen))
	}
	return line
}

func (m NodesModel) renderRouteHeader() string {
	return m.styleNormal.Faint(true).Render(
		fmt.Sprintf("  %-12s  %-12s  %5s  %7s  %s",
			"Destination", "Next Hop", "Hops", "Metric", "State"),
	)
}

func (m NodesModel) renderRouteRow(r bramble.Route) string {
	stateStyle, ok := m.styleRouteState[strings.ToUpper(r.State)]
	if !ok {
		stateStyle = m.styleNormal
	}
	stateRendered := stateStyle.Render(r.State)
	return fmt.Sprintf("  %-12s  %-12s  %5d  %7d  %s",
		truncateStr(r.Dest, 12), truncateStr(r.NextHop, 12),
		r.HopCount, r.Metric, stateRendered)
}

// --- helpers ---

func neighborStatus(lastSeenMs int64) (string, string) {
	d := time.Duration(lastSeenMs) * time.Millisecond
	switch {
	case d < 30*time.Second:
		return "online", "●"
	case d < 5*time.Minute:
		return "reachable", "●"
	default:
		return "stale", "○"
	}
}

func fmtDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// sortNeighbors sorts m.neighbors in place according to m.sortCol.
func (m *NodesModel) sortNeighbors() {
	ns := m.neighbors
	switch m.sortCol {
	case SortByAddress:
		for i := 1; i < len(ns); i++ {
			for j := i; j > 0 && ns[j].Address < ns[j-1].Address; j-- {
				ns[j], ns[j-1] = ns[j-1], ns[j]
			}
		}
	case SortByRSSI:
		// descending
		for i := 1; i < len(ns); i++ {
			for j := i; j > 0 && ns[j].RSSI > ns[j-1].RSSI; j-- {
				ns[j], ns[j-1] = ns[j-1], ns[j]
			}
		}
	case SortByLastSeen:
		// ascending (lowest ms = most recent)
		for i := 1; i < len(ns); i++ {
			for j := i; j > 0 && ns[j].LastSeenAgoMs < ns[j-1].LastSeenAgoMs; j-- {
				ns[j], ns[j-1] = ns[j-1], ns[j]
			}
		}
	}
}

// --- commands ---

type nodesDataMsg struct {
	neighbors []bramble.Neighbor
	routes    []bramble.Route
}

func fetchNodesCmd(client *bramble.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		neighbors, err := client.Neighbors(ctx)
		if err != nil {
			neighbors = nil
		}

		routes, err := client.Routes(ctx)
		if err != nil {
			routes = nil
		}

		return nodesDataMsg{neighbors: neighbors, routes: routes}
	}
}
