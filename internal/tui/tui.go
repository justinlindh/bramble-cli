// Package tui provides the Bubble Tea v2 terminal UI for bramble.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	bramble "github.com/justinlindh/bramble-go"
	"github.com/justinlindh/bramble-cli/internal/tui/tabs"
	"github.com/justinlindh/bramble-cli/internal/tui/widgets"
)

// Tab identifiers.
const (
	TabChat     = 0
	TabNodes    = 1
	TabLocation = 2
	TabConfig   = 3
	TabStats    = 4
	TabCount    = 5
)

var tabNames = [TabCount]string{"Chat", "Nodes", "Location", "Config", "Stats"}

// tabHints maps each tab to its key hints shown in the status line.
var tabHints = [TabCount]string{
	"[/] Compose  [↑↓] Navigate  [Enter] Open  [r] Routes",
	"[↑↓] Navigate  [m] DM  [n] Set alias",
	"[↑↓] Navigate",
	"[↑↓] Navigate  [Enter] Edit",
	"[r] Refresh  [p] Probe  [t] Traffic  [Tab] Focus",
}

// NodeInfo holds the fetched node identity/status for display.
type NodeInfo struct {
	Address   string
	Name      string
	Transport string
	Connected bool
}

// ConnectFn is a factory function that creates and connects a new client.
type ConnectFn func(ctx context.Context) (*bramble.Client, error)

// ── Poll tick ────────────────────────────────────────────────────────────────

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Every(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// ── Countdown tick ───────────────────────────────────────────────────────────

type countdownMsg time.Time

func countdownCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return countdownMsg(t)
	})
}

// ── Fetch result Msgs ────────────────────────────────────────────────────────

type fetchStatusResult struct{ status *bramble.StatusResponse; err error }
type fetchNeighborsResult struct{ neighbors []bramble.Neighbor; err error }
type fetchRoutesResult struct{ routes []bramble.Route; err error }
type fetchAirtimeResult struct{ airtime *bramble.AirtimeStats; err error }
type fetchPeerLocsResult struct{ peers []bramble.LocationPeer; err error }

// ── Reconnect Msgs ───────────────────────────────────────────────────────────

type reconnectMsg struct{}      // trigger a reconnect attempt
type reconnectResult struct {   // result of reconnect attempt
	client *bramble.Client
	node   NodeInfo
	err    error
}

// ── Model ────────────────────────────────────────────────────────────────────

// Model is the root Bubble Tea model.
type Model struct {
	client    *bramble.Client
	connectFn ConnectFn
	bridge    *Bridge
	store     *Store

	activeTab int
	width     int
	height    int
	theme     Theme
	node      NodeInfo
	ready     bool
	connected bool

	pollCount  int
	backoffSec int // current reconnect backoff in seconds
	retryIn    int // countdown seconds until next retry (0 = not counting)

	// Help overlay
	showHelp bool

	// Status line (toasts + hints)
	statusLine widgets.StatusLine

	// Tab submodels
	statsTab    tabs.StatsModel
	nodesTab    tabs.NodesModel
	chatTab     tabs.ChatModel
	configTab   tabs.ConfigModel
	locationTab tabs.LocationModel
}

// New creates a new TUI model with reconnect support.
func New(client *bramble.Client, node NodeInfo, connectFn ConnectFn, msgdb *MsgDB) Model {
	store := NewStore()
	if msgdb != nil {
		store.SetMsgDB(msgdb)
	}
	// Initialize name resolver; load aliases if DB is ready and we have an address.
	if msgdb != nil && node.Address != "" {
		resolver := NewNameResolver(msgdb, node.Address)
		_ = resolver.LoadAliases()
		store.Resolver = resolver
	} else if node.Address != "" {
		store.Resolver = NewNameResolver(nil, node.Address)
	}
	chatTab := tabs.NewChatModel(client, node.Address)
	chatTab.SetLoadOlderFn(func(convID string, beforeTs int64, limit int) []bramble.Message {
		return store.LoadOlderMessages(convID, beforeTs, limit)
	})
	if store.Resolver != nil {
		chatTab.SetResolver(store.Resolver)
	}
	nodesTab := tabs.NewNodes(client)
	statsTab := tabs.NewStats(client)
	locationTab := tabs.NewLocation()
	if store.Resolver != nil {
		nodesTab.SetResolver(store.Resolver)
		statsTab.SetResolver(store.Resolver)
		locationTab.SetResolver(store.Resolver)
	}
	return Model{
		client:     client,
		connectFn:  connectFn,
		store:      store,
		activeTab:  TabChat,
		theme:      DefaultTheme(),
		node:       node,
		connected:  true,
		backoffSec: 1,
		statusLine: widgets.NewStatusLine(),
		statsTab:    statsTab,
		nodesTab:    nodesTab,
		chatTab:     chatTab,
		configTab:   tabs.NewConfig(client),
		locationTab: locationTab,
	}
}

// ClassifyMessageConvID returns the conversation ID for a bramble.Message.
func ClassifyMessageConvID(msg bramble.Message, selfAddr string) string {
	if msg.To == "" || msg.To == "broadcast" {
		return "broadcast"
	}
	if len(msg.To) > 3 && msg.To[:3] == "ch:" {
		return msg.To
	}
	if msg.From == selfAddr || msg.From == "" {
		return fmt.Sprintf("dm:%s", msg.To)
	}
	return fmt.Sprintf("dm:%s", msg.From)
}

// PreloadFromDB loads recent messages from the database into the chat tab and store.
func (m *Model) PreloadFromDB(db *MsgDB) {
	recent, err := db.LoadRecent(200)
	if err != nil {
		return
	}
	for _, sm := range recent {
		msg := sm.ToBramble()
		m.store.AddMessage(msg)
		m.chatTab.IngestMessage(msg)
	}
	m.chatTab.RefreshViewport()
}

// SetProgram wires the Bubble Tea program reference for the bridge.
// Must be called before the program is Run().
func (m *Model) SetProgram(p *tea.Program) {
	m.bridge = NewBridge(p)
	if m.client != nil && m.connected {
		m.bridge.Start(m.client)
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tickCmd(),
		m.fetchInitialData(),
		m.statsTab.RefreshCmd(),
		m.chatTab.Init(),
		m.configTab.Init(),
	)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.statsTab.SetSize(msg.Width, msg.Height)
		m.statusLine.SetWidth(msg.Width)
		contentH := msg.Height - 4
		if contentH < 1 {
			contentH = 1
		}
		m.chatTab.SetSize(msg.Width, contentH)

	case tea.KeyPressMsg:
		// Help overlay absorbs all key presses.
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.showHelp = true
			return m, nil
		case "ctrl+r":
			m.retryIn = 0
			m.statusLine.AddToast(widgets.ToastInfo, "Reconnecting…")
			return m, m.doReconnect()
		case "tab":
			m.activeTab = (m.activeTab + 1) % TabCount
			if m.activeTab == TabStats {
				return m, m.statsTab.RefreshCmd()
			}
		case "shift+tab":
			m.activeTab = (m.activeTab - 1 + TabCount) % TabCount
			if m.activeTab == TabStats {
				return m, m.statsTab.RefreshCmd()
			}
		case "1":
			m.activeTab = TabChat
		case "2":
			m.activeTab = TabNodes
		case "3":
			m.activeTab = TabLocation
		case "4":
			m.activeTab = TabConfig
		case "5":
			m.activeTab = TabStats
			return m, m.statsTab.RefreshCmd()
		default:
			// Forward to active tab
			switch m.activeTab {
			case TabStats:
				next, cmd := m.statsTab.Update(msg)
				m.statsTab = next.(tabs.StatsModel)
				return m, cmd
			case TabChat:
				var cmd tea.Cmd
				m.chatTab, cmd = m.chatTab.Update(msg)
				return m, cmd
			case TabNodes:
				var cmd tea.Cmd
				m.nodesTab, cmd = m.nodesTab.Update(msg)
				return m, cmd
			case TabLocation:
				var cmd tea.Cmd
				m.locationTab, cmd = m.locationTab.Update(msg)
				return m, cmd
			case TabConfig:
				var cmd tea.Cmd
				m.configTab, cmd = m.configTab.Update(msg)
				return m, cmd
			}
		}

	// ── Poll tick ───────────────────────────────────────────────────────────
	case tickMsg:
		if !m.connected {
			return m, tickCmd()
		}
		m.pollCount++
		cmds := []tea.Cmd{
			tickCmd(),
			m.fetchStatus(),
			m.fetchNeighbors(),
		}
		if m.pollCount%2 == 0 {
			cmds = append(cmds, m.fetchRoutes(), m.fetchAirtime(), m.fetchPeerLocs())
		}
		return m, tea.Batch(cmds...)

	// ── Countdown tick ──────────────────────────────────────────────────────
	case countdownMsg:
		if m.retryIn > 0 {
			m.retryIn--
		}
		if m.retryIn > 0 {
			return m, countdownCmd()
		}
		return m, nil

	// ── Fetch results ────────────────────────────────────────────────────────
	case fetchStatusResult:
		if msg.err != nil {
			m.connected = false
			m.statusLine.AddToast(widgets.ToastError, "Connection lost")
			return m, m.scheduleReconnect()
		}
		m.store.UpdateStatus(msg.status)
	case fetchNeighborsResult:
		if msg.err == nil {
			m.store.UpdateNeighbors(msg.neighbors)
		}
	case fetchRoutesResult:
		if msg.err == nil {
			m.store.UpdateRoutes(msg.routes)
		}
	case fetchAirtimeResult:
		if msg.err == nil {
			m.store.UpdateAirtime(msg.airtime)
		}
	case fetchPeerLocsResult:
		if msg.err == nil {
			m.store.UpdatePeerLocations(msg.peers)
		}

	// ── Stats tab Msgs ────────────────────────────────────────────────────────
	case tabs.StatsDataMsg:
		if msg.FetchErr == nil {
			m.store.UpdateStatus(&msg.Status)
			m.store.UpdateIdentity(&msg.Identity)
			m.store.UpdateAirtime(&msg.Airtime)
		}
		next, cmd := m.statsTab.Update(msg)
		m.statsTab = next.(tabs.StatsModel)
		return m, cmd

	// ── Bridge Msgs ──────────────────────────────────────────────────────────
	case tabs.SwitchToChatMsg:
		m.activeTab = TabChat
		var chatCmd tea.Cmd
		m.chatTab, chatCmd = m.chatTab.Update(msg)
		return m, chatCmd

	case MsgReceived:
		m.store.AddMessage(msg.Msg)
		var chatCmd tea.Cmd
		m.chatTab, chatCmd = m.chatTab.Update(tabs.ChatMsgReceived{Msg: msg.Msg})
		return m, chatCmd
	case AckReceived:
		m.store.UpdateAck(msg.Ack)
		var chatCmd tea.Cmd
		m.chatTab, chatCmd = m.chatTab.Update(tabs.ChatAckReceived{Ack: msg.Ack})
		return m, chatCmd
	case BroadcastDeliveryReceived:
		if m.store.msgdb != nil {
			d := msg.Delivery
			go func() { _ = m.store.msgdb.UpdateStatus(d.BroadcastID, d.Status) }()
		}
		var chatCmd tea.Cmd
		m.chatTab, chatCmd = m.chatTab.Update(tabs.ChatBroadcastDelivery{D: msg.Delivery})
		return m, chatCmd
	case NeighborChanged:
		return m, m.fetchNeighbors()
	// Other bridge msgs are informational; handled in future phases.
	case GpsEventReceived:
		m.store.UpdateOwnGPS(msg.Event)
	case TrafficEventReceived:
		next, cmd := m.statsTab.Update(tabs.StatsTrafficEventMsg{Event: msg.Event})
		m.statsTab = next.(tabs.StatsModel)
		return m, cmd
	case ProbeResultReceived:
		next, cmd := m.statsTab.Update(tabs.StatsProbeResultMsg{Result: msg.Result})
		m.statsTab = next.(tabs.StatsModel)
		return m, cmd
	case ProbeCompleteReceived:
		next, cmd := m.statsTab.Update(tabs.StatsProbeCompleteMsg{})
		m.statsTab = next.(tabs.StatsModel)
		return m, cmd
	case tabs.SendResultMsg:
		var chatCmd tea.Cmd
		m.chatTab, chatCmd = m.chatTab.Update(msg)
		return m, chatCmd

	case WifiEventReceived, LocationEventReceived:
		// TODO: forward to location panel

	// ── Reconnect ────────────────────────────────────────────────────────────
	case reconnectMsg:
		return m, m.doReconnect()

	case reconnectResult:
		if msg.err != nil {
			m.statusLine.AddToast(widgets.ToastError, fmt.Sprintf("Connection failed: %v", msg.err))
			return m, m.scheduleReconnect()
		}
		// Reconnect succeeded.
		if m.client != nil {
			_ = m.client.Close()
		}
		m.client = msg.client
		m.node = msg.node
		m.connected = true
		m.backoffSec = 1
		m.retryIn = 0
		m.statusLine.AddToast(widgets.ToastSuccess, "Reconnected")
		if m.bridge != nil {
			m.bridge.Start(m.client)
		}
		return m, m.fetchInitialData()
	}

	return m, nil
}

// scheduleReconnect schedules the next reconnect attempt with backoff.
func (m *Model) scheduleReconnect() tea.Cmd {
	delay := time.Duration(m.backoffSec) * time.Second
	m.retryIn = m.backoffSec
	m.backoffSec *= 2
	if m.backoffSec > 30 {
		m.backoffSec = 30
	}
	cmds := []tea.Cmd{
		tea.Tick(delay, func(t time.Time) tea.Msg { return reconnectMsg{} }),
		countdownCmd(),
	}
	return tea.Batch(cmds...)
}

// View implements tea.Model.
func (m Model) View() tea.View {
	if !m.ready {
		v := tea.NewView("Initializing...")
		v.AltScreen = true
		return v
	}

	var sb strings.Builder

	// Header
	sb.WriteString(m.renderHeader())
	sb.WriteString("\n")

	// Tab bar
	sb.WriteString(m.renderTabBar())
	sb.WriteString("\n")

	// Content area (leave 1 row for footer/status line)
	contentHeight := m.height - 4
	if contentHeight < 1 {
		contentHeight = 1
	}
	content := m.renderContent(contentHeight)
	// Pad content to exactly contentHeight lines and full width
	content = padToSize(content, m.width, contentHeight)
	sb.WriteString(content)

	// Footer / status line
	sb.WriteString(m.renderStatusLine())

	// Help overlay (rendered on top via lipgloss Place)
	output := sb.String()
	if m.showHelp {
		output = m.renderHelpOverlay(output)
	}

	v := tea.NewView(output)
	v.AltScreen = true
	return v
}

func (m Model) renderHeader() string {
	t := m.theme

	var connStatus string
	switch {
	case m.connected && m.node.Connected:
		connStatus = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF87")).Bold(true).Render("● CONNECTED")
	case !m.connected && m.retryIn > 0:
		connStatus = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFAA00")).Bold(true).
			Render(fmt.Sprintf("◌ Reconnecting in %ds…", m.retryIn))
	case !m.connected:
		connStatus = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFAA00")).Bold(true).Render("◌ RECONNECTING…")
	default:
		connStatus = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Bold(true).Render("○ DISCONNECTED")
	}

	var identity string
	if m.node.Address != "" {
		identity = fmt.Sprintf("  %s", m.node.Address)
		if m.node.Name != "" {
			identity += fmt.Sprintf(" (%s)", m.node.Name)
		}
	}

	var transport string
	if m.node.Transport != "" {
		transport = fmt.Sprintf("  via %s", m.node.Transport)
	}

	line := connStatus + identity + transport
	style := t.Header.Width(m.width)
	return style.Render(line)
}

func (m Model) renderTabBar() string {
	t := m.theme
	var parts []string
	for i, name := range tabNames {
		label := fmt.Sprintf(" %d:%s ", i+1, name)
		if i == m.activeTab {
			parts = append(parts, t.TabActive.Render(label))
		} else {
			parts = append(parts, t.TabInactive.Render(label))
		}
	}
	bar := strings.Join(parts, "")
	// Pad to full width with the tab bar background
	barLen := lipgloss.Width(bar)
	if barLen < m.width {
		pad := strings.Repeat(" ", m.width-barLen)
		bar += t.TabInactive.Render(pad)
	}
	return bar
}

func (m Model) renderContent(height int) string {
	switch m.activeTab {
	case TabStats:
		return m.statsTab.Render()
	case TabChat:
		return m.chatTab.View()
	case TabNodes:
		return m.nodesTab.View()
	case TabLocation:
		m.locationTab.SetData(m.store.GetOwnGPS(), m.store.GetPeerLocations())
		return m.locationTab.View()
	case TabConfig:
		return m.configTab.View()
	}

	tabName := tabNames[m.activeTab]
	lines := []string{
		"",
		fmt.Sprintf("  ┌─ %s ─────────────────────────────┐", tabName),
		"  │                                      │",
		"  │   Coming soon                        │",
		"  │                                      │",
		"  └──────────────────────────────────────┘",
	}

	for len(lines) < height {
		lines = append(lines, "")
	}

	return m.theme.Content.Render(strings.Join(lines[:height], "\n"))
}

func (m Model) renderStatusLine() string {
	hints := tabHints[m.activeTab] + "  [?] Help  [q] Quit"
	return m.statusLine.Render(hints)
}

// padToSize ensures the rendered content fills exactly the given width × height,
// preventing ghost text bleed-through in the alt screen.
func padToSize(content string, width, height int) string {
	lines := strings.Split(content, "\n")
	// Pad each line to full width
	for i, line := range lines {
		w := lipgloss.Width(line)
		if w < width {
			lines[i] = line + strings.Repeat(" ", width-w)
		}
	}
	// Pad to full height
	emptyLine := strings.Repeat(" ", width)
	for len(lines) < height {
		lines = append(lines, emptyLine)
	}
	// Truncate if too many lines
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}

// ── Help overlay ──────────────────────────────────────────────────────────────

var helpContent = `
 ╔══════════════════════════════════════════════╗
 ║              KEYBOARD SHORTCUTS              ║
 ╠══════════════════════════════════════════════╣
 ║  GLOBAL                                      ║
 ║    Tab / Shift+Tab   Cycle tabs              ║
 ║    1-5               Switch to tab           ║
 ║    ?                 Toggle this help        ║
 ║    Ctrl+R            Force reconnect         ║
 ║    q / Ctrl+C        Quit                    ║
 ╠══════════════════════════════════════════════╣
 ║  CHAT                                        ║
 ║    /                 Focus compose bar       ║
 ║    Enter             Send message / Open DM  ║
 ║    ↑↓                Navigate conversations  ║
 ║    r                 Toggle route details    ║
 ╠══════════════════════════════════════════════╣
 ║  NODES                                       ║
 ║    ↑↓                Navigate neighbors      ║
 ║    d                 Open DM to node         ║
 ║    Enter             Node details            ║
 ╠══════════════════════════════════════════════╣
 ║  STATS                                       ║
 ║    r                 Refresh stats           ║
 ╠══════════════════════════════════════════════╣
 ║           Press any key to dismiss           ║
 ╚══════════════════════════════════════════════╝
`

func (m Model) renderHelpOverlay(base string) string {
	overlayStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#ccccdd")).
		Background(lipgloss.Color("#0d0d1a")).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#00FF87")).
		Padding(0, 1)

	overlay := overlayStyle.Render(strings.TrimRight(helpContent, "\n"))

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, overlay)
}

// ── Fetch commands ────────────────────────────────────────────────────────────

func (m Model) fetchInitialData() tea.Cmd {
	return tea.Batch(
		m.fetchStatus(),
		m.fetchNeighbors(),
		m.fetchRoutes(),
		m.fetchAirtime(),
		m.fetchPeerLocs(),
	)
}

func (m Model) fetchStatus() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		st, err := client.Status(ctx)
		return fetchStatusResult{status: st, err: err}
	}
}

func (m Model) fetchNeighbors() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ns, err := client.Neighbors(ctx)
		return fetchNeighborsResult{neighbors: ns, err: err}
	}
}

func (m Model) fetchRoutes() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		rs, err := client.Routes(ctx)
		return fetchRoutesResult{routes: rs, err: err}
	}
}

func (m Model) fetchAirtime() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		a, err := client.Airtime(ctx)
		return fetchAirtimeResult{airtime: a, err: err}
	}
}

func (m Model) fetchPeerLocs() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ps, err := client.PeerLocations(ctx)
		return fetchPeerLocsResult{peers: ps, err: err}
	}
}

// ── Reconnect ─────────────────────────────────────────────────────────────────

func (m Model) doReconnect() tea.Cmd {
	connectFn := m.connectFn
	node := m.node
	if connectFn == nil {
		return nil
	}
	m.connected = false
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		client, err := connectFn(ctx)
		if err != nil {
			return reconnectResult{err: err}
		}
		idCtx, idCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer idCancel()
		if identity, err := client.Identity(idCtx); err == nil {
			node.Address = identity.Address
		}
		node.Connected = true
		return reconnectResult{client: client, node: node}
	}
}
