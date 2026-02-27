// Package tui provides the Bubble Tea v2 terminal UI for bramble.
package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/justinlindh/bramble-cli/internal/tui/tabs"
	bramble "github.com/justinlindh/bramble-go"
)

// NodeInfo holds the fetched node identity/status for display.
type NodeInfo struct {
	Address   string
	Name      string
	Transport string
	Connected bool
}

// ConnectFn is a factory function that creates and connects a new client.
type ConnectFn func(ctx context.Context) (*bramble.Client, error)

// ── Tick Msgs ────────────────────────────────────────────────────────────────

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Every(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

type clockTickMsg time.Time

func clockTickCmd() tea.Cmd {
	return tea.Every(30*time.Second, func(t time.Time) tea.Msg {
		return clockTickMsg(t)
	})
}

// ── Fetch result Msgs ─────────────────────────────────────────────────────────

type fetchStatusResult struct {
	status *bramble.StatusResponse
	err    error
}
type fetchNeighborsResult struct {
	neighbors []bramble.Neighbor
	err       error
}
type fetchRoutesResult struct {
	routes []bramble.Route
	err    error
}
type fetchAirtimeResult struct {
	airtime *bramble.AirtimeStats
	err     error
}
type fetchPeerLocsResult struct {
	peers []bramble.LocationPeer
	err   error
}

// ── Reconnect Msgs ────────────────────────────────────────────────────────────

type reconnectMsg struct{}
type reconnectResult struct {
	client *bramble.Client
	node   NodeInfo
	err    error
}

// ── Send result ───────────────────────────────────────────────────────────────

type sendResultMsg struct {
	convID string
	text   string
	msgID  string
	err    error
}

// ── Model ─────────────────────────────────────────────────────────────────────

// Model is the root Bubble Tea model.
type Model struct {
	client    *bramble.Client
	connectFn ConnectFn
	bridge    *Bridge
	store     *Store

	scroll     *Scrollback
	statusBar  StatusBar
	input      InputLine
	cmdHandler *CommandHandler

	width  int
	height int
	node   NodeInfo
	ready  bool

	connected  bool
	activeConv string // "broadcast", "ch:N", "dm:ADDR"

	pollCount  int
	backoffSec int
	retryIn    int

	pendingConfirm bool
}

// New creates a new IRC-style TUI model.
func New(client *bramble.Client, node NodeInfo, connectFn ConnectFn, msgdb *MsgDB) Model {
	store := NewStore()
	if msgdb != nil {
		store.SetMsgDB(msgdb)
	}
	if msgdb != nil && node.Address != "" {
		resolver := NewNameResolver(msgdb, node.Address)
		_ = resolver.LoadAliases()
		store.Resolver = resolver
	} else if node.Address != "" {
		store.Resolver = NewNameResolver(nil, node.Address)
	}

	sb := NewScrollback()
	scroll := &sb
	statusBar := NewStatusBar()
	input := NewInputLine()

	var resolver tabs.PeerResolver
	if store.Resolver != nil {
		resolver = store.Resolver
	}
	cmdHandler := NewCommandHandler(client, store, scroll, resolver)

	m := Model{
		client:     client,
		connectFn:  connectFn,
		store:      store,
		scroll:     scroll,
		statusBar:  statusBar,
		input:      input,
		cmdHandler: cmdHandler,
		node:       node,
		connected:  true,
		activeConv: "broadcast",
		backoffSec: 1,
	}

	return m
}

// ClassifyMessageConvID returns the conversation ID for a bramble.Message.
func ClassifyMessageConvID(msg bramble.Message, selfAddr string) string {
	if msg.To == "" || msg.To == "broadcast" || msg.To == "FFFFFFFF" {
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

func (m *Model) selfDisplayName() string {
	if m.node.Name != "" {
		return fmt.Sprintf("%s(%s)", m.node.Name, shortHash(m.node.Address))
	}
	if m.store != nil && m.store.Resolver != nil {
		if named := m.store.Resolver.ResolveWithHash(m.node.Address); named != "" && named != m.node.Address {
			return named
		}
	}
	return m.node.Address
}

func (m *Model) peerDisplayName(addr string) string {
	if m.store != nil && m.store.Resolver != nil {
		return m.store.Resolver.ResolveWithHash(addr)
	}
	return addr
}

// PreloadFromDB loads recent messages from the database into the scrollback.
func (m *Model) PreloadFromDB(db *MsgDB) {
	recent, err := db.LoadRecent(200)
	if err != nil {
		return
	}
	for _, sm := range recent {
		msg := sm.ToBramble()
		m.store.AddMessage(msg)
		convID := ClassifyMessageConvID(msg, m.node.Address)
		if convID == m.activeConv {
			outgoing := sm.Direction == "out"
			addr := msg.From
			if outgoing && addr == "" {
				addr = m.node.Address
			}
			sender := m.peerDisplayName(addr)
			if outgoing {
				sender = m.selfDisplayName()
				addr = m.node.Address
			}
			badge := ""
			if outgoing {
				badge = badgeFor(sm.Status)
			}
			m.scroll.AddChat(sender, addr, msg.Text, badge, outgoing)
		}
	}
}

// SetProgram wires the Bubble Tea program reference for the bridge.
func (m *Model) SetProgram(p *tea.Program) {
	m.bridge = NewBridge(p)
	if m.client != nil && m.connected {
		m.bridge.Start(m.client)
	}
}

func (m *Model) switchBuffer(convID string) {
	m.activeConv = convID
	m.store.SetActiveConv(convID)
	label := m.convDisplayLabel(convID)
	m.input.SetPrompt("[" + label + "]")
	m.reloadScrollback()
	m.addSystem(convID, fmt.Sprintf("Switched to %s", label))
}

func (m *Model) addConversationLine(convID string, kind LineKind, rendered string) {
	line := ScrollLine{Kind: kind, Timestamp: time.Now(), Text: rendered}
	m.store.AddConversationLine(convID, line)
}

func (m *Model) addSystem(convID, text string) {
	rendered := m.scroll.theme.System.Render("-- " + text + " --")
	m.addConversationLine(convID, LineSystem, rendered)
	if convID == m.activeConv {
		m.scroll.AddSystem(text)
	}
}

func (m *Model) addError(convID, text string) {
	rendered := m.scroll.theme.Error.Render("!! " + text)
	m.addConversationLine(convID, LineError, rendered)
	if convID == m.activeConv {
		m.scroll.AddError(text)
	}
}

func (m *Model) addInfo(convID, text string) {
	rendered := m.scroll.theme.Info.Render(text)
	m.addConversationLine(convID, LineInfo, rendered)
	if convID == m.activeConv {
		m.scroll.AddInfo(text)
	}
}

func (m *Model) addDelivery(convID, text string) {
	rendered := m.scroll.theme.Delivery.Render("-- " + text + " --")
	m.addConversationLine(convID, LineDelivery, rendered)
	if convID == m.activeConv {
		m.scroll.AddDelivery(text)
	}
}

func (m *Model) convDisplayLabel(convID string) string {
	switch {
	case convID == "broadcast":
		return "all (broadcast)"
	case strings.HasPrefix(convID, "dm:"):
		addr := convID[3:]
		return "@" + m.peerDisplayName(addr)
	case strings.HasPrefix(convID, "ch:"):
		return "mesh#" + strings.TrimPrefix(convID, "ch:")
	default:
		return convID
	}
}

func (m *Model) reloadScrollback() {
	m.scroll.Clear()
	m.store.mu.RLock()
	conv := m.store.Conversations[m.activeConv]
	m.store.mu.RUnlock()
	if conv == nil {
		return
	}
	type replayItem struct {
		ts    time.Time
		kind  string
		msg   bramble.Message
		line  ScrollLine
		order int
	}
	items := make([]replayItem, 0, len(conv.Messages)+len(conv.Events))
	for i, msg := range conv.Messages {
		ts := time.Unix(msg.Timestamp, 0)
		if msg.Timestamp <= 0 {
			ts = time.Now()
		}
		items = append(items, replayItem{ts: ts, kind: "msg", msg: msg, order: i})
	}
	for i, line := range conv.Events {
		items = append(items, replayItem{ts: line.Timestamp, kind: "evt", line: line, order: i})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].ts.Equal(items[j].ts) {
			return items[i].kind < items[j].kind
		}
		return items[i].ts.Before(items[j].ts)
	})
	for _, item := range items {
		if item.kind == "evt" {
			m.scroll.AddStoredLine(item.line)
			continue
		}
		msg := item.msg
		outgoing := msg.From == m.node.Address || msg.From == ""
		addr := msg.From
		if outgoing && addr == "" {
			addr = m.node.Address
		}
		sender := m.peerDisplayName(addr)
		if outgoing {
			sender = m.selfDisplayName()
			addr = m.node.Address
		}
		badge := ""
		if outgoing {
			badge = "*"
		}
		m.scroll.AddChatAt(item.ts, sender, addr, msg.Text, badge, outgoing)
	}
}

func (m *Model) updateStatusBar() {
	m.statusBar.SetConnection(m.connected, m.node.Address, m.node.Name)
	m.store.mu.RLock()
	peerCount := len(m.store.Neighbors)
	m.store.mu.RUnlock()
	m.statusBar.SetPeerCount(peerCount)
	m.statusBar.SetScrolled(m.scroll.IsScrolled())

	convs := m.store.GetConversations()
	var bufs []BufferInfo
	for _, c := range convs {
		bufs = append(bufs, BufferInfo{
			ID:     c.ID,
			Label:  c.Label,
			Unread: c.Unread,
			Active: c.ID == m.activeConv,
		})
	}
	m.statusBar.SetBuffers(bufs)
}

func (m *Model) cycleBuffer(delta int) {
	convs := m.store.GetConversations()
	if len(convs) == 0 {
		return
	}
	cur := 0
	for i, c := range convs {
		if c.ID == m.activeConv {
			cur = i
			break
		}
	}
	next := (cur + delta + len(convs)) % len(convs)
	m.switchBuffer(convs[next].ID)
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	m.addSystem("broadcast", fmt.Sprintf("Connected to %s via %s", m.node.Address, m.node.Transport))
	if m.node.Name != "" {
		m.addSystem("broadcast", fmt.Sprintf("Node: %s", m.node.Name))
	}
	m.addSystem("broadcast", "Type /help for commands")

	return tea.Batch(
		tickCmd(),
		clockTickCmd(),
		m.input.Focus(),
		m.fetchInitialData(),
	)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		sbH := msg.Height - 4
		if sbH < 1 {
			sbH = 1
		}
		m.scroll.SetSize(msg.Width, sbH)
		m.statusBar.SetWidth(msg.Width)
		m.input.SetWidth(msg.Width)
		m.updateStatusBar()

	case tea.KeyPressMsg:
		key := msg.String()

		// pendingConfirm is now handled via /reboot-confirm command

		switch key {
		case "ctrl+c":
			return m, tea.Quit

		case "alt+1", "alt+2", "alt+3", "alt+4", "alt+5",
			"alt+6", "alt+7", "alt+8", "alt+9":
			idx := int(key[4] - '1')
			convs := m.store.GetConversations()
			if idx < len(convs) {
				m.switchBuffer(convs[idx].ID)
			}
			return m, nil

		case "ctrl+n":
			m.cycleBuffer(1)
			return m, nil
		case "ctrl+p":
			m.cycleBuffer(-1)
			return m, nil

		case "pgup", "pgdown", "home", "end":
			m.scroll.Update(msg)
			return m, nil
		}

		// Forward everything else to input
		var inputCmd tea.Cmd
		m.input, inputCmd = m.input.Update(msg)
		cmds = append(cmds, inputCmd)

	case InputMsg:
		if msg.IsCommand {
			cmd := ParseCommand(msg.Text)
			if cmd != nil {
				if cmd.Name == "reboot-confirm" && m.pendingConfirm {
					m.pendingConfirm = false
					m.cmdHandler.DoReboot()
					return m, nil
				}
				action := m.cmdHandler.Execute(cmd)
				if action.Quit {
					return m, tea.Quit
				}
				if action.SwitchBuffer != "" {
					m.switchBuffer(action.SwitchBuffer)
				}
				if action.SendText != "" {
					return m, m.sendMessage(action.SendText)
				}
				if action.Reboot {
					m.addSystem(m.activeConv, "Reboot node? Type /reboot-confirm to proceed")
					m.pendingConfirm = true
				}
			}
		} else if strings.TrimSpace(msg.Text) != "" {
			return m, m.sendMessage(msg.Text)
		}

	case sendResultMsg:
		if msg.err != nil {
			errText := msg.err.Error()
			if strings.HasPrefix(msg.convID, "ch:") && strings.Contains(strings.ToLower(errText), "invalid params") {
				m.addError(msg.convID, fmt.Sprintf("Send failed on mesh channel %s: invalid channel or params. Try /config to list channels, or /b for broadcast.", strings.TrimPrefix(msg.convID, "ch:")))
			} else {
				m.addError(msg.convID, fmt.Sprintf("Send failed: %v", msg.err))
			}
		} else {
			m.scroll.AddChat(m.selfDisplayName(), m.node.Address, msg.text, "*", true)
			raw := bramble.Message{
				From:      m.node.Address,
				To:        convIDToAddr(m.activeConv),
				Text:      msg.text,
				MsgID:     msg.msgID,
				Timestamp: time.Now().Unix(),
			}
			m.store.AddMessage(raw)
		}

	case tickMsg:
		if !m.connected {
			return m, tickCmd()
		}
		m.pollCount++
		fetchCmds := []tea.Cmd{
			tickCmd(),
			m.fetchStatus(),
			m.fetchNeighbors(),
		}
		if m.pollCount%2 == 0 {
			fetchCmds = append(fetchCmds, m.fetchRoutes(), m.fetchAirtime(), m.fetchPeerLocs())
		}
		return m, tea.Batch(fetchCmds...)

	case clockTickMsg:
		m.updateStatusBar()
		return m, clockTickCmd()

	case fetchStatusResult:
		if msg.err != nil && m.connected {
			m.connected = false
			m.addError(m.activeConv, "Connection lost")
			return m, m.scheduleReconnect()
		}
		if msg.err == nil {
			m.store.UpdateStatus(msg.status)
			m.updateStatusBar()
		}

	case fetchNeighborsResult:
		if msg.err == nil {
			oldNeighbors := make(map[string]bramble.Neighbor)
			m.store.mu.RLock()
			for _, n := range m.store.Neighbors {
				oldNeighbors[n.Address] = n
			}
			m.store.mu.RUnlock()

			newAddrs := make(map[string]bool)
			for _, n := range msg.neighbors {
				newAddrs[n.Address] = true
			}

			m.store.UpdateNeighbors(msg.neighbors)
			m.updateStatusBar()

			// Announce joins
			for _, n := range msg.neighbors {
				if _, wasKnown := oldNeighbors[n.Address]; !wasKnown {
					name := n.Address
					if m.store.Resolver != nil {
						name = m.store.Resolver.Resolve(n.Address)
					}
					m.addSystem("broadcast", fmt.Sprintf("%s joined [RSSI %d, SNR %.1f]", name, n.RSSI, n.SNR))
				}
			}

			// Announce departures (only if we had neighbors before)
			if len(oldNeighbors) > 0 {
				for addr, old := range oldNeighbors {
					if !newAddrs[addr] {
						name := addr
						if m.store.Resolver != nil {
							name = m.store.Resolver.Resolve(addr)
						}
						lastSeen := fmtDurationShort(time.Duration(old.LastSeenAgoMs) * time.Millisecond)
						m.addSystem("broadcast", fmt.Sprintf("%s left [last seen %s ago]", name, lastSeen))
					}
				}
			}
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

	// ── Bridge Msgs ───────────────────────────────────────────────────────────

	case MsgReceived:
		convID := ClassifyMessageConvID(msg.Msg, m.node.Address)
		isNew := m.store.IsNewConversation(convID)
		m.store.AddMessage(msg.Msg)
		if isNew && strings.HasPrefix(convID, "dm:") {
			peerAddr := convID[3:]
			peerName := peerAddr
			if m.store.Resolver != nil {
				peerName = m.store.Resolver.Resolve(peerAddr)
			}
			m.addSystem(convID, fmt.Sprintf("New DM from %s", peerName))
		}
		if convID == m.activeConv {
			addr := msg.Msg.From
			sender := m.peerDisplayName(addr)
			m.scroll.AddChat(sender, addr, msg.Msg.Text, "", false)
		}
		m.updateStatusBar()

	case AckReceived:
		m.store.UpdateAck(msg.Ack)

	case BroadcastDeliveryReceived:
		if m.store.msgdb != nil {
			d := msg.Delivery
			go func() { _ = m.store.msgdb.UpdateStatus(d.BroadcastID, d.Status) }()
		}
		if m.activeConv == "broadcast" {
			d := msg.Delivery
			status := "✓"
			if d.Status == "failed" {
				status = "✗"
			}
			peer := d.Recipient
			if m.store.Resolver != nil {
				peer = m.store.Resolver.Resolve(peer)
			}
			m.addDelivery("broadcast", fmt.Sprintf("%s %s", peer, status))
			m.scroll.AddDeliveryGrouped(d.BroadcastID, fmt.Sprintf("%s %s", peer, status))
		}

	case NeighborChanged:
		return m, m.fetchNeighbors()

	case GpsEventReceived:
		m.store.UpdateOwnGPS(msg.Event)

	case TrafficEventReceived:
		// informational; no inline display by default

	case ProbeResultReceived:
		r := msg.Result
		name := r.Address
		if m.store.Resolver != nil {
			name = m.store.Resolver.Resolve(r.Address)
		}
		m.addInfo(m.activeConv, fmt.Sprintf("  Probe: %s  %dms  %d hops  RSSI %d",
			name, r.LatencyMs, r.Hops, r.RSSI))

	case ProbeCompleteReceived:
		m.addSystem(m.activeConv, "Probe complete")

	case WifiEventReceived, LocationEventReceived:
		// forwarded to location in future

	// ── Reconnect ─────────────────────────────────────────────────────────────

	case reconnectMsg:
		return m, m.doReconnect()

	case reconnectResult:
		if msg.err != nil {
			m.addError(m.activeConv, fmt.Sprintf("Reconnect failed: %v", msg.err))
			return m, m.scheduleReconnect()
		}
		if m.client != nil {
			_ = m.client.Close()
		}
		m.client = msg.client
		m.node = msg.node
		m.connected = true
		m.backoffSec = 1
		m.retryIn = 0
		m.addSystem(m.activeConv, "Reconnected")
		if m.bridge != nil {
			m.bridge.Start(m.client)
		}
		m.updateStatusBar()
		return m, m.fetchInitialData()

	default:
		// Forward unhandled messages to input (e.g. FocusMsg from textarea)
		var inputCmd tea.Cmd
		m.input, inputCmd = m.input.Update(msg)
		cmds = append(cmds, inputCmd)
	}

	return m, tea.Batch(cmds...)
}

// View implements tea.Model.
func (m Model) View() tea.View {
	if !m.ready {
		v := tea.NewView("Connecting...")
		v.AltScreen = true
		return v
	}

	var sb strings.Builder

	scrollView := m.scroll.View()
	scrollView = padToSize(scrollView, m.width, m.height-4)
	sb.WriteString(scrollView)
	sb.WriteString("\n")

	m.updateStatusBar()
	sb.WriteString(m.statusBar.View())
	sb.WriteString("\n")

	sb.WriteString(m.input.View())

	v := tea.NewView(sb.String())
	v.AltScreen = true
	return v
}

// scheduleReconnect schedules the next reconnect attempt with backoff.
func (m *Model) scheduleReconnect() tea.Cmd {
	delay := time.Duration(m.backoffSec) * time.Second
	m.retryIn = m.backoffSec
	m.backoffSec *= 2
	if m.backoffSec > 30 {
		m.backoffSec = 30
	}
	m.addSystem(m.activeConv, fmt.Sprintf("Reconnecting in %ds...", m.retryIn))
	return tea.Tick(delay, func(t time.Time) tea.Msg { return reconnectMsg{} })
}

// ── Send ───────────────────────────────────────────────────────────────────────

func (m Model) sendMessage(text string) tea.Cmd {
	client := m.client
	convID := m.activeConv
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		var msgID string
		var err error

		switch {
		case convID == "broadcast" || convID == "":
			var res *bramble.SendResult
			res, err = client.SendBroadcast(ctx, text)
			if err == nil {
				msgID = res.MessageID
			}
		case strings.HasPrefix(convID, "ch:"):
			chStr := strings.TrimPrefix(convID, "ch:")
			ch := 0
			fmt.Sscanf(chStr, "%d", &ch)
			var res *bramble.SendResult
			res, err = client.BroadcastOnChannel(ctx, ch, text)
			if err == nil {
				msgID = res.MessageID
			}
		case strings.HasPrefix(convID, "dm:"):
			addrStr := strings.TrimPrefix(convID, "dm:")
			var addr uint64
			_, parseErr := fmt.Sscanf(addrStr, "%x", &addr)
			if parseErr != nil {
				err = fmt.Errorf("invalid address %q", addrStr)
			} else {
				var res *bramble.SendResult
				res, err = client.Send(ctx, uint32(addr), text)
				if err == nil {
					msgID = res.MessageID
				}
			}
		default:
			err = fmt.Errorf("unknown buffer %q", convID)
		}

		return sendResultMsg{convID: convID, text: text, msgID: msgID, err: err}
	}
}

func convIDToAddr(id string) string {
	switch {
	case id == "broadcast" || id == "":
		return "broadcast"
	case strings.HasPrefix(id, "ch:"):
		return id
	case strings.HasPrefix(id, "dm:"):
		return id[3:]
	}
	return id
}

func badgeFor(status string) string {
	switch status {
	case "delivered":
		return "✓"
	case "read":
		return "✓✓"
	case "failed":
		return "✗"
	default:
		return "*"
	}
}

// ── Fetch commands ─────────────────────────────────────────────────────────────

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

// ── Reconnect ──────────────────────────────────────────────────────────────────

func (m Model) doReconnect() tea.Cmd {
	connectFn := m.connectFn
	node := m.node
	if connectFn == nil {
		return nil
	}
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
		if cfg, err := client.Config(idCtx); err == nil {
			node.Name = strings.TrimSpace(cfg.NodeName)
		}
		node.Connected = true
		return reconnectResult{client: client, node: node}
	}
}

// padToSize ensures rendered content fills exactly width × height.
func padToSize(content string, width, height int) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		w := lipgloss.Width(line)
		if w < width {
			lines[i] = line + strings.Repeat(" ", width-w)
		}
	}
	emptyLine := strings.Repeat(" ", width)
	for len(lines) < height {
		lines = append(lines, emptyLine)
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
}
