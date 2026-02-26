package tabs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	bramble "github.com/justinlindh/bramble-go"
)

// ── Tea messages ─────────────────────────────────────────────────────────────

// ChatMsgReceived notifies the chat tab of a new incoming message.
// The root tui.Model forwards MsgReceived here.
type ChatMsgReceived struct{ Msg bramble.Message }

// ChatAckReceived notifies the chat tab of a delivery ack.
// The root tui.Model forwards AckReceived here.
type ChatAckReceived struct{ Ack bramble.Ack }

// ChatBroadcastDelivery notifies the chat tab of a broadcast delivery receipt.
type ChatBroadcastDelivery struct{ D bramble.BroadcastDelivery }

// chatMsgsLoaded carries the initial message fetch result.
type chatMsgsLoaded struct {
	msgs []bramble.Message
	err  error
}

// chatChannelsLoaded carries a config fetch result for channel detail.
type chatChannelsLoaded struct {
	channels []bramble.Channel
	err      error
}

// locationShareResult is the result of a location share attempt.
type locationShareResult struct {
	err error
}

// ── Delivery badge ────────────────────────────────────────────────────────────

type deliveryStatus int

const (
	deliveryPending   deliveryStatus = iota
	deliveryDelivered                // direct ack
	deliveryMultiHop                 // ack with relay path
	deliveryFailed                   // failed / timeout
)

func (d deliveryStatus) Badge() string {
	switch d {
	case deliveryPending:
		return "*"
	case deliveryDelivered:
		return "+"
	case deliveryMultiHop:
		return "++"
	case deliveryFailed:
		return "x"
	}
	return ""
}

// ── Stored message ────────────────────────────────────────────────────────────

type storedMsg struct {
	msg      bramble.Message
	outgoing bool
	delivery deliveryStatus
	// Route annotation (populated from Ack.RelayPath).
	relayPath []bramble.RelayHop
	// Broadcast delivery receipts.
	deliveries []bramble.BroadcastDelivery
	// Whether the delivery/route detail is expanded.
	expanded bool
}

// ── ChatConv stores messages and metadata for a single conversation ───────────

type chatConv struct {
	id     string // "broadcast", "ch:N", "dm:ADDR"
	label  string
	msgs   []storedMsg
	unread int
}

// ── focus areas ──────────────────────────────────────────────────────────────

type chatFocus int

const (
	chatFocusList    chatFocus = iota
	chatFocusViewport
	chatFocusCompose
)

// ── ChatModel ─────────────────────────────────────────────────────────────────

const detailPanelWidth = 26

// ChatModel is the Bubble Tea sub-model for the Chat tab.
type ChatModel struct {
	client   *bramble.Client
	selfAddr string
	width    int
	height   int

	convs     []*chatConv
	activeIdx int

	viewport viewport.Model
	compose  ComposeModel
	focus    chatFocus

	pendingNew int // new msgs arrived while scrolled up
	ready      bool

	// Feature flags / UI state.
	showRoutes        bool
	showChannelDetail bool
	configChannels    []bramble.Channel

	// Message selection for broadcast delivery expansion.
	selectedMsgIdx int // -1 = none

	// Toast notification.
	toast       string
	toastExpiry time.Time

	// styles
	styleListSelected lipgloss.Style
	styleListNormal   lipgloss.Style
	styleOutgoing     lipgloss.Style
	styleIncoming     lipgloss.Style
	styleSender       lipgloss.Style
	styleTimestamp    lipgloss.Style
	styleSeparator    lipgloss.Style
	styleStatus       lipgloss.Style
	styleBadgeUnread  lipgloss.Style
	styleRoute        lipgloss.Style
	styleDetailPanel  lipgloss.Style
	styleDetailHeader lipgloss.Style
	styleDetailKey    lipgloss.Style
	styleDetailVal    lipgloss.Style
	styleToast        lipgloss.Style
	styleDelivery     lipgloss.Style
}

// NewChatModel creates a new ChatModel.
func NewChatModel(client *bramble.Client, selfAddr string) ChatModel {
	vp := viewport.New(viewport.WithWidth(60), viewport.WithHeight(20))
	vp.SoftWrap = true

	broadcast := &chatConv{id: "broadcast", label: "Broadcast"}

	m := ChatModel{
		client:         client,
		selfAddr:       selfAddr,
		convs:          []*chatConv{broadcast},
		viewport:       vp,
		compose:        NewCompose(client),
		selectedMsgIdx: -1,

		styleListSelected: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00FF87")),
		styleListNormal: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#aaaacc")),
		styleOutgoing: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#88BBFF")),
		styleIncoming: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ccccdd")),
		styleSender: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFAA44")).
			Bold(true),
		styleTimestamp: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#555577")),
		styleSeparator: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#333355")),
		styleStatus: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#555577")),
		styleBadgeUnread: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555")).
			Bold(true),
		styleRoute: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#444466")),
		styleDetailPanel: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#445566")).
			Padding(0, 1),
		styleDetailHeader: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF87")).
			Bold(true),
		styleDetailKey: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888899")),
		styleDetailVal: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ccccdd")),
		styleToast: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF87")).
			Background(lipgloss.Color("#112233")).
			Bold(true).
			Padding(0, 1),
		styleDelivery: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666688")),
	}
	return m
}

// SetSize updates dimensions of the chat tab.
func (m *ChatModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.applyLayout()
}

// applyLayout recalculates viewport dimensions based on current flags.
func (m *ChatModel) applyLayout() {
	w, h := m.width, m.height

	listW := w / 4
	if listW < 16 {
		listW = 16
	}
	vpW := w - listW - 1 // 1 col for separator
	if m.showChannelDetail {
		vpW -= detailPanelWidth + 1 // extra separator
	}
	if vpW < 10 {
		vpW = 10
	}

	// Compose bar is 5 rows at bottom; viewport gets rest
	composeH := 5
	vpH := h - composeH - 1 // 1 for status bar
	if vpH < 3 {
		vpH = 3
	}

	m.viewport.SetWidth(vpW)
	m.viewport.SetHeight(vpH)
	m.compose.SetSize(vpW)
	m.ready = true
	m.refreshViewport()
}

// Init fetches initial messages.
func (m ChatModel) Init() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		msgs, err := client.Messages(ctx)
		return chatMsgsLoaded{msgs: msgs, err: err}
	}
}

// activeConv returns the currently selected conversation.
func (m *ChatModel) activeConv() *chatConv {
	if m.activeIdx < len(m.convs) {
		return m.convs[m.activeIdx]
	}
	return m.convs[0]
}

// Update handles messages for the chat tab.
func (m ChatModel) Update(msg tea.Msg) (ChatModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case chatMsgsLoaded:
		if msg.err == nil {
			for _, raw := range msg.msgs {
				m.ingestMessage(raw)
			}
		}
		m.refreshViewport()
		m.viewport.GotoBottom()

	case ChatMsgReceived:
		prevActive := m.activeConv().id
		convIdx := m.ingestMessage(msg.Msg)
		conv := m.convs[convIdx]
		if conv.id != prevActive {
			conv.unread++
		} else {
			atBottom := m.viewport.AtBottom()
			m.refreshViewport()
			if atBottom {
				m.viewport.GotoBottom()
				m.pendingNew = 0
			} else {
				m.pendingNew++
			}
		}

	case ChatAckReceived:
		m.handleAck(msg.Ack)
		m.refreshViewport()

	case ChatBroadcastDelivery:
		m.handleBroadcastDelivery(msg.D)
		m.refreshViewport()

	case chatChannelsLoaded:
		if msg.err == nil {
			m.configChannels = msg.channels
		}

	case locationShareResult:
		if msg.err == nil {
			m.toast = "📍 Location shared"
		} else {
			m.toast = fmt.Sprintf("✗ Location: %v", msg.err)
		}
		m.toastExpiry = time.Now().Add(3 * time.Second)

	case SendResultMsg:
		// Message we sent — add to active conv as outgoing.
		if msg.Err == nil {
			raw := bramble.Message{
				From:      m.selfAddr,
				To:        convIDToTo(msg.ConvID),
				Text:      msg.Text,
				MsgID:     msg.MsgID,
				Timestamp: time.Now().Unix(),
			}
			m.ingestMessage(raw)
			m.refreshViewport()
			m.viewport.GotoBottom()
			m.pendingNew = 0
		}

	case SwitchToChatMsg:
		// Switch to the requested conversation.
		idx := m.findConvByID(msg.Conversation)
		if idx >= 0 {
			m.activeIdx = idx
			m.convs[idx].unread = 0
			m.compose.SetConvID(msg.Conversation)
			m.selectedMsgIdx = -1
			m.refreshViewport()
			m.viewport.GotoBottom()
		}

	case tea.KeyPressMsg:
		// Global keys handled before focus delegation.
		switch msg.String() {
		case "r":
			if m.focus != chatFocusCompose {
				m.showRoutes = !m.showRoutes
				m.refreshViewport()
				return m, tea.Batch(cmds...)
			}
		case "d":
			if m.focus != chatFocusCompose {
				conv := m.activeConv()
				if strings.HasPrefix(conv.id, "ch:") || conv.id == "broadcast" {
					m.showChannelDetail = !m.showChannelDetail
					if m.showChannelDetail && m.configChannels == nil {
						cmds = append(cmds, m.fetchChannelsCmd())
					}
					m.applyLayout()
					return m, tea.Batch(cmds...)
				}
			}
		case "esc":
			if m.showChannelDetail {
				m.showChannelDetail = false
				m.applyLayout()
				return m, tea.Batch(cmds...)
			}
		case "ctrl+l":
			if m.focus == chatFocusCompose || m.focus == chatFocusViewport {
				conv := m.activeConv()
				if strings.HasPrefix(conv.id, "dm:") {
					addrStr := strings.TrimPrefix(conv.id, "dm:")
					var addr uint64
					if _, err := fmt.Sscanf(addrStr, "%x", &addr); err == nil {
						cmds = append(cmds, shareLocationCmd(m.client, uint32(addr)))
						return m, tea.Batch(cmds...)
					}
				} else {
					m.toast = "Location sharing only available in DM conversations"
					m.toastExpiry = time.Now().Add(3 * time.Second)
					return m, tea.Batch(cmds...)
				}
			}
		}

		switch msg.String() {
		case "tab":
			// Cycle focus: list → viewport → compose → list
			switch m.focus {
			case chatFocusList:
				m.focus = chatFocusViewport
			case chatFocusViewport:
				m.focus = chatFocusCompose
				cmd := m.compose.Focus()
				cmds = append(cmds, cmd)
			case chatFocusCompose:
				m.compose.Blur()
				m.focus = chatFocusList
			}

		case "left":
			if m.focus != chatFocusList {
				m.compose.Blur()
				m.focus = chatFocusList
			}

		case "right", "enter":
			if m.focus == chatFocusList {
				m.focus = chatFocusViewport
				m.convs[m.activeIdx].unread = 0
				m.compose.SetConvID(m.activeConv().id)
				m.selectedMsgIdx = -1
				m.refreshViewport()
				m.viewport.GotoBottom()
				m.pendingNew = 0
				return m, tea.Batch(cmds...)
			}
		}

		// Delegate key to focused component.
		switch m.focus {
		case chatFocusList:
			prevIdx := m.activeIdx
			switch msg.String() {
			case "j", "down":
				if m.activeIdx < len(m.convs)-1 {
					m.activeIdx++
				}
			case "k", "up":
				if m.activeIdx > 0 {
					m.activeIdx--
				}
			}
			if m.activeIdx != prevIdx {
				m.convs[m.activeIdx].unread = 0
				m.compose.SetConvID(m.activeConv().id)
				m.selectedMsgIdx = -1
				m.refreshViewport()
				m.viewport.GotoBottom()
				m.pendingNew = 0
			}

		case chatFocusViewport:
			conv := m.activeConv()
			switch msg.String() {
			case "[":
				// Select previous outgoing message.
				m.moveMsgSelection(conv, -1)
				m.refreshViewport()
			case "]":
				// Select next outgoing message.
				m.moveMsgSelection(conv, 1)
				m.refreshViewport()
			case "enter":
				// Toggle expansion of selected message.
				if m.selectedMsgIdx >= 0 && m.selectedMsgIdx < len(conv.msgs) {
					conv.msgs[m.selectedMsgIdx].expanded = !conv.msgs[m.selectedMsgIdx].expanded
					m.refreshViewport()
				}
			default:
				var vpCmd tea.Cmd
				m.viewport, vpCmd = m.viewport.Update(msg)
				cmds = append(cmds, vpCmd)
			}

		case chatFocusCompose:
			var cCmd tea.Cmd
			m.compose, cCmd = m.compose.Update(msg)
			cmds = append(cmds, cCmd)
		}
	}

	return m, tea.Batch(cmds...)
}

// View renders the full chat tab.
func (m ChatModel) View() string {
	if !m.ready {
		return "  Loading chat...\n"
	}

	listW := m.width / 4
	if listW < 16 {
		listW = 16
	}

	listView := m.renderList(listW)
	vpView := m.viewport.View()
	composeView := m.compose.View()

	listLines := strings.Split(listView, "\n")
	vpLines := strings.Split(vpView, "\n")

	// Remove trailing empty string from split
	if len(listLines) > 0 && listLines[len(listLines)-1] == "" {
		listLines = listLines[:len(listLines)-1]
	}
	if len(vpLines) > 0 && vpLines[len(vpLines)-1] == "" {
		vpLines = vpLines[:len(vpLines)-1]
	}

	vpH := m.viewport.Height()
	maxRows := vpH
	if len(listLines) > maxRows {
		maxRows = len(listLines)
	}

	// Extend both to maxRows
	for len(listLines) < maxRows {
		listLines = append(listLines, "")
	}
	for len(vpLines) < maxRows {
		vpLines = append(vpLines, "")
	}

	// Build channel detail panel lines if needed.
	var detailLines []string
	if m.showChannelDetail {
		detailLines = m.renderDetailPanel(m.viewport.Height())
		for len(detailLines) < maxRows {
			detailLines = append(detailLines, "")
		}
	}

	sep := m.styleSeparator.Render("│")
	var sb strings.Builder
	for i := 0; i < maxRows; i++ {
		ll := padRight(listLines[i], listW)
		sb.WriteString(ll)
		sb.WriteString(sep)
		vpLine := ""
		if i < len(vpLines) {
			vpLine = vpLines[i]
		}
		sb.WriteString(vpLine)
		if m.showChannelDetail {
			sb.WriteString(sep)
			if i < len(detailLines) {
				sb.WriteString(detailLines[i])
			}
		}
		sb.WriteString("\n")
	}

	// Status bar row
	sb.WriteString(m.statusBar(listW))
	sb.WriteString("\n")

	// Compose bar (below viewport)
	sb.WriteString(strings.Repeat(" ", listW+1))
	sb.WriteString(composeView)
	sb.WriteString("\n")

	// Toast overlay (if active)
	if m.toast != "" && time.Now().Before(m.toastExpiry) {
		sb.WriteString(m.styleToast.Render("  " + m.toast + "  "))
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderList renders the conversation list pane.
func (m ChatModel) renderList(width int) string {
	var sb strings.Builder
	for i, conv := range m.convs {
		icon := convIcon(conv.id)
		label := conv.label
		badge := ""
		if conv.unread > 0 {
			badge = m.styleBadgeUnread.Render(fmt.Sprintf("[%d]", conv.unread))
		}

		// Truncate label
		maxLabel := width - 4 - len(icon) // prefix + icon + space + badge
		if maxLabel < 1 {
			maxLabel = 1
		}
		if len(label) > maxLabel {
			label = label[:maxLabel-1] + "…"
		}

		row := fmt.Sprintf("%s%s %s", convPrefix(i == m.activeIdx, m.focus == chatFocusList), icon, label)
		if badge != "" {
			row += " " + badge
		}

		if i == m.activeIdx {
			row = m.styleListSelected.Render(row)
		} else {
			row = m.styleListNormal.Render(row)
		}
		sb.WriteString(row)
		sb.WriteString("\n")

		// Preview: last message
		if len(conv.msgs) > 0 {
			last := conv.msgs[len(conv.msgs)-1]
			preview := chatTruncate(last.msg.Text, width-4)
			sb.WriteString(m.styleStatus.Render("    " + preview))
			sb.WriteString("\n")
		}
	}
	if len(m.convs) == 0 {
		sb.WriteString(m.styleStatus.Render("  (no conversations)"))
		sb.WriteString("\n")
	}
	return sb.String()
}

// renderDetailPanel renders the channel detail overlay panel.
func (m ChatModel) renderDetailPanel(height int) []string {
	conv := m.activeConv()
	inner := detailPanelWidth - 4 // subtract border + padding

	var sb strings.Builder
	sb.WriteString(m.styleDetailHeader.Render("Channel Detail"))
	sb.WriteString("\n")
	sb.WriteString(strings.Repeat("─", inner))
	sb.WriteString("\n")

	// Name
	name := conv.label
	sb.WriteString(m.styleDetailKey.Render("Name: "))
	sb.WriteString(m.styleDetailVal.Render(truncateStr(name, inner-6)))
	sb.WriteString("\n")

	// Index (for ch: convs)
	if strings.HasPrefix(conv.id, "ch:") {
		idx := strings.TrimPrefix(conv.id, "ch:")
		sb.WriteString(m.styleDetailKey.Render("Index: "))
		sb.WriteString(m.styleDetailVal.Render(idx))
		sb.WriteString("\n")
	}

	// PSK status from configChannels
	pskStr := "unknown"
	if m.configChannels != nil {
		if conv.id == "broadcast" {
			if len(m.configChannels) > 0 {
				if m.configChannels[0].HasPsk {
					pskStr = "PSK set"
				} else {
					pskStr = "open"
				}
			}
		} else if strings.HasPrefix(conv.id, "ch:") {
			var chIdx int
			fmt.Sscanf(strings.TrimPrefix(conv.id, "ch:"), "%d", &chIdx)
			for _, ch := range m.configChannels {
				if ch.ID == chIdx {
					if ch.HasPsk {
						pskStr = "PSK set"
					} else {
						pskStr = "open"
					}
					break
				}
			}
		}
	}
	sb.WriteString(m.styleDetailKey.Render("PSK: "))
	sb.WriteString(m.styleDetailVal.Render(pskStr))
	sb.WriteString("\n")

	// Hint
	sb.WriteString("\n")
	sb.WriteString(m.styleDetailKey.Render("[d/Esc] close"))
	sb.WriteString("\n")

	content := sb.String()
	rendered := m.styleDetailPanel.Width(detailPanelWidth - 2).Render(content)
	lines := strings.Split(rendered, "\n")
	// Pad to height
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", detailPanelWidth))
	}
	return lines
}

func convIcon(id string) string {
	switch {
	case id == "broadcast":
		return "📡"
	case strings.HasPrefix(id, "ch:"):
		return "#"
	default:
		return "@"
	}
}

func convPrefix(selected, focused bool) string {
	if !selected {
		return "  "
	}
	if focused {
		return "> "
	}
	return "→ "
}

// statusBar renders the one-line status bar between viewport and compose.
func (m ChatModel) statusBar(listW int) string {
	conv := m.activeConv()
	hint := ""
	if m.focus == chatFocusViewport && m.pendingNew > 0 {
		hint = fmt.Sprintf("  ↓ %d new", m.pendingNew)
	}
	extras := ""
	if m.showRoutes {
		extras += " [r:routes]"
	}
	isDM := strings.HasPrefix(conv.id, "dm:")
	focusStr := "[Tab] focus  [j/k] nav  [↑/↓] scroll  [r] routes"
	if isDM {
		focusStr += "  [^L] location"
	}
	if strings.HasPrefix(conv.id, "ch:") || conv.id == "broadcast" {
		focusStr += "  [d] detail"
	}
	line := fmt.Sprintf("  %s%s%s  %s", conv.label, hint, extras, focusStr)
	return m.styleStatus.Render(strings.Repeat(" ", listW+1) + line)
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// ingestMessage classifies and stores a message. Returns the conv index.
func (m *ChatModel) ingestMessage(raw bramble.Message) int {
	convID := m.classifyMessage(raw)
	idx := m.findOrCreateConv(convID)

	outgoing := raw.From == m.selfAddr || (raw.From == "" && raw.To != "")

	sm := storedMsg{
		msg:      raw,
		outgoing: outgoing,
		delivery: deliveryPending,
	}
	if !outgoing {
		sm.delivery = deliveryDelivered
	}

	m.convs[idx].msgs = append(m.convs[idx].msgs, sm)
	return idx
}

// classifyMessage returns the conv ID for a message.
func (m *ChatModel) classifyMessage(msg bramble.Message) string {
	if msg.To == "" || msg.To == "broadcast" {
		return "broadcast"
	}
	if strings.HasPrefix(msg.To, "ch:") {
		return msg.To
	}
	// DM: key by peer (not self)
	if msg.From == m.selfAddr || msg.From == "" {
		return fmt.Sprintf("dm:%s", msg.To)
	}
	return fmt.Sprintf("dm:%s", msg.From)
}

// findOrCreateConv ensures a conv with the given ID exists.
func (m *ChatModel) findOrCreateConv(id string) int {
	for i, c := range m.convs {
		if c.id == id {
			return i
		}
	}
	label := convIDToLabel(id)
	m.convs = append(m.convs, &chatConv{id: id, label: label})
	return len(m.convs) - 1
}

// findConvByID returns the index of a conv by ID, or -1.
func (m *ChatModel) findConvByID(id string) int {
	for i, c := range m.convs {
		if c.id == id {
			return i
		}
	}
	return -1
}

// handleAck updates delivery badge and relay path for matching outgoing message.
func (m *ChatModel) handleAck(ack bramble.Ack) {
	for _, conv := range m.convs {
		for i, sm := range conv.msgs {
			if sm.outgoing && sm.msg.MsgID == ack.PacketID {
				switch ack.Status {
				case "delivered":
					if len(ack.RelayPath) > 0 {
						conv.msgs[i].delivery = deliveryMultiHop
						conv.msgs[i].relayPath = ack.RelayPath
					} else {
						conv.msgs[i].delivery = deliveryDelivered
					}
				case "failed", "timeout":
					conv.msgs[i].delivery = deliveryFailed
				}
				return
			}
		}
	}
}

// handleBroadcastDelivery stores a broadcast delivery receipt.
func (m *ChatModel) handleBroadcastDelivery(d bramble.BroadcastDelivery) {
	for _, conv := range m.convs {
		for i, sm := range conv.msgs {
			if sm.outgoing && sm.msg.MsgID == d.BroadcastID {
				conv.msgs[i].deliveries = append(conv.msgs[i].deliveries, d)
				return
			}
		}
	}
}

// moveMsgSelection moves the message selection index by delta, clamping to outgoing messages.
func (m *ChatModel) moveMsgSelection(conv *chatConv, delta int) {
	if len(conv.msgs) == 0 {
		return
	}
	// Collect indices of outgoing messages.
	var outIdxs []int
	for i, sm := range conv.msgs {
		if sm.outgoing {
			outIdxs = append(outIdxs, i)
		}
	}
	if len(outIdxs) == 0 {
		return
	}

	if m.selectedMsgIdx < 0 {
		// Start at last outgoing.
		if delta >= 0 {
			m.selectedMsgIdx = outIdxs[0]
		} else {
			m.selectedMsgIdx = outIdxs[len(outIdxs)-1]
		}
		return
	}

	// Find current position in outIdxs.
	cur := -1
	for j, idx := range outIdxs {
		if idx == m.selectedMsgIdx {
			cur = j
			break
		}
	}
	if cur < 0 {
		m.selectedMsgIdx = outIdxs[len(outIdxs)-1]
		return
	}
	next := cur + delta
	if next < 0 {
		next = 0
	}
	if next >= len(outIdxs) {
		next = len(outIdxs) - 1
	}
	m.selectedMsgIdx = outIdxs[next]
}

// refreshViewport rebuilds the viewport content for the active conversation.
func (m *ChatModel) refreshViewport() {
	if !m.ready {
		return
	}
	conv := m.activeConv()
	if len(conv.msgs) == 0 {
		m.viewport.SetContent("  No messages yet.\n")
		return
	}

	w := m.viewport.Width()
	var lines []string
	for i, sm := range conv.msgs {
		selected := (i == m.selectedMsgIdx)
		lines = append(lines, m.renderMessage(sm, w, selected))
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
}

// renderMessage formats a stored message for display.
func (m ChatModel) renderMessage(sm storedMsg, width int, selected bool) string {
	ts := time.Unix(sm.msg.Timestamp, 0).Format("15:04")
	tsStr := m.styleTimestamp.Render("[" + ts + "]")

	sender := sm.msg.From
	if sender == "" || sender == m.selfAddr {
		sender = "me"
	}
	if len(sender) > 10 {
		sender = sender[:8] + ".."
	}

	text := sm.msg.Text
	var lines []string

	if sm.outgoing {
		badge := sm.delivery.Badge()
		selMark := " "
		if selected {
			selMark = "●"
		}
		inner := fmt.Sprintf("%s %s %s%s", tsStr, text, badge, selMark)
		line := m.styleOutgoing.Render("> " + inner)
		// Right-align within viewport width
		vis := visLen(line)
		if vis < width {
			line = strings.Repeat(" ", width-vis) + line
		}
		lines = append(lines, line)

		// Route annotation.
		if m.showRoutes && len(sm.relayPath) > 0 {
			lines = append(lines, m.renderRoute(sm.relayPath, width))
		}

		// Broadcast delivery expansion.
		if selected && sm.expanded && len(sm.deliveries) > 0 {
			lines = append(lines, m.renderDeliveries(sm.deliveries, width))
		} else if selected && len(sm.deliveries) > 0 {
			hint := m.styleDelivery.Render(fmt.Sprintf("  [Enter] show %d delivery receipts", len(sm.deliveries)))
			lines = append(lines, hint)
		}
	} else {
		senderStr := m.styleSender.Render(sender + ":")
		lines = append(lines, m.styleIncoming.Render("< "+tsStr+" "+senderStr+" "+text))
	}

	return strings.Join(lines, "\n")
}

// renderRoute renders a dim relay path annotation line.
func (m ChatModel) renderRoute(hops []bramble.RelayHop, width int) string {
	var parts []string
	for _, h := range hops {
		parts = append(parts, h.Addr)
	}
	hopCount := len(hops)
	hopWord := "hop"
	if hopCount != 1 {
		hopWord = "hops"
	}
	routeStr := fmt.Sprintf("  → %s (%d %s)", strings.Join(parts, " → "), hopCount, hopWord)
	return m.styleRoute.Render(routeStr)
}

// renderDeliveries renders the broadcast delivery receipt list.
func (m ChatModel) renderDeliveries(deliveries []bramble.BroadcastDelivery, width int) string {
	var sb strings.Builder
	for _, d := range deliveries {
		name := d.Recipient
		if len(name) > 10 {
			name = name[:8] + ".."
		}
		statusIcon := "·"
		if d.Status == "delivered" {
			statusIcon = "✓"
		}
		line := fmt.Sprintf("    %s %-12s %s", statusIcon, name, d.Status)
		sb.WriteString(m.styleDelivery.Render(line))
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// fetchChannelsCmd returns a Cmd that fetches channel config.
func (m *ChatModel) fetchChannelsCmd() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cfg, err := client.Config(ctx)
		if err != nil {
			return chatChannelsLoaded{err: err}
		}
		return chatChannelsLoaded{channels: cfg.Channels}
	}
}

// shareLocationCmd returns a Cmd that shares location once with addr.
func shareLocationCmd(client *bramble.Client, addr uint32) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := client.ShareLocationOnce(ctx, addr)
		return locationShareResult{err: err}
	}
}

// convIDToLabel converts a conv ID to a human-readable label.
func convIDToLabel(id string) string {
	switch {
	case id == "broadcast":
		return "Broadcast"
	case strings.HasPrefix(id, "ch:"):
		return id // "ch:N"
	case strings.HasPrefix(id, "dm:"):
		return id[3:] // just the addr
	}
	return id
}

// convIDToTo converts a conversation ID to a message To field.
func convIDToTo(id string) string {
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

// padRight pads s to width w (visual width approximate).
func padRight(s string, w int) string {
	v := visLen(s)
	if v >= w {
		return s
	}
	return s + strings.Repeat(" ", w-v)
}

// visLen approximates the visible width of s (strips ANSI escapes crudely).
func visLen(s string) int {
	n := 0
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		n++
	}
	return n
}

// chatTruncate truncates s to maxLen with "…" suffix.
func chatTruncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	if maxLen == 1 {
		return "…"
	}
	return s[:maxLen-1] + "…"
}


