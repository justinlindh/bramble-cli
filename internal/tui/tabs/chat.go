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

// chatMsgsLoaded carries the initial message fetch result.
type chatMsgsLoaded struct {
	msgs []bramble.Message
	err  error
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
}

// ── ChatConv stores messages and metadata for a single conversation ───────────

type chatConv struct {
	id       string // "broadcast", "ch:N", "dm:ADDR"
	label    string
	msgs     []storedMsg
	unread   int
}

// ── focus areas ──────────────────────────────────────────────────────────────

type chatFocus int

const (
	chatFocusList    chatFocus = iota
	chatFocusViewport
	chatFocusCompose
)

// ── ChatModel ─────────────────────────────────────────────────────────────────

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

	pendingNew  int // new msgs arrived while scrolled up
	ready       bool

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
}

// NewChatModel creates a new ChatModel.
func NewChatModel(client *bramble.Client, selfAddr string) ChatModel {
	vp := viewport.New(viewport.WithWidth(60), viewport.WithHeight(20))
	vp.SoftWrap = true

	broadcast := &chatConv{id: "broadcast", label: "Broadcast"}

	m := ChatModel{
		client:   client,
		selfAddr: selfAddr,
		convs:    []*chatConv{broadcast},
		viewport: vp,
		compose:  NewCompose(client),

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
	}
	return m
}

// SetSize updates dimensions of the chat tab.
func (m *ChatModel) SetSize(w, h int) {
	m.width = w
	m.height = h

	listW := w / 4
	if listW < 16 {
		listW = 16
	}
	vpW := w - listW - 1 // 1 col for separator
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
			m.refreshViewport()
			m.viewport.GotoBottom()
		}

	case tea.KeyPressMsg:
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
				m.refreshViewport()
				m.viewport.GotoBottom()
				m.pendingNew = 0
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
				m.refreshViewport()
				m.viewport.GotoBottom()
				m.pendingNew = 0
			}

		case chatFocusViewport:
			var vpCmd tea.Cmd
			m.viewport, vpCmd = m.viewport.Update(msg)
			cmds = append(cmds, vpCmd)

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

	sep := m.styleSeparator.Render("│")
	var sb strings.Builder
	for i := 0; i < maxRows; i++ {
		ll := padRight(listLines[i], listW)
		sb.WriteString(ll)
		sb.WriteString(sep)
		if i < len(vpLines) {
			sb.WriteString(vpLines[i])
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
	focusStr := "[Tab] switch focus  [j/k] list nav  [↑/↓] scroll msgs  [Enter] focus msgs"
	line := fmt.Sprintf("  %s%s  %s", conv.label, hint, focusStr)
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

// handleAck updates delivery badge for matching outgoing message.
func (m *ChatModel) handleAck(ack bramble.Ack) {
	for _, conv := range m.convs {
		for i, sm := range conv.msgs {
			if sm.outgoing && sm.msg.MsgID == ack.PacketID {
				switch ack.Status {
				case "delivered":
					if len(ack.RelayPath) > 0 {
						conv.msgs[i].delivery = deliveryMultiHop
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
	for _, sm := range conv.msgs {
		lines = append(lines, m.renderMessage(sm, w))
	}
	m.viewport.SetContent(strings.Join(lines, "\n"))
}

// renderMessage formats a stored message for display.
func (m ChatModel) renderMessage(sm storedMsg, width int) string {
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

	if sm.outgoing {
		badge := sm.delivery.Badge()
		inner := fmt.Sprintf("%s %s %s", tsStr, text, badge)
		line := m.styleOutgoing.Render("> " + inner)
		// Right-align within viewport width
		vis := visLen(line)
		if vis < width {
			line = strings.Repeat(" ", width-vis) + line
		}
		return line
	}

	senderStr := m.styleSender.Render(sender + ":")
	return m.styleIncoming.Render("< " + tsStr + " " + senderStr + " " + text)
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
