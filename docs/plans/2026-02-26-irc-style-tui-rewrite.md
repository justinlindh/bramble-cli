# IRC-Style TUI Rewrite (BitchX-inspired)

> **For Agent:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Replace the current tab-based split-pane TUI with a single-buffer IRC-style interface inspired by BitchX — full-screen scrollback, inline system events, always-ready input line, slash commands for non-chat functions.

**Architecture:** The root model becomes a single scrollback buffer + input line + status bar. No more tab switching or split panes. Channels/DMs are background "windows" switched via `/join` or `Alt+N`. Non-chat features (nodes, stats, config, location) are accessed via `/slash` commands that either print inline or open temporary overlay panels. The existing `store.go`, `bridge.go`, `msgdb.go`, `names.go`, and `tabs/resolver.go` are preserved. Everything in `tabs/` is deleted or absorbed.

**Tech Stack:** Go, Bubble Tea v2, Lip Gloss v2, Bubbles v2 (textarea, viewport)

---

## File Map

**Keep as-is:**
- `internal/tui/store.go` — state container (minor additions for event log)
- `internal/tui/bridge.go` — SDK push notification wiring
- `internal/tui/msgdb.go` — SQLite persistence
- `internal/tui/names.go` — peer name resolution
- `internal/tui/tabs/resolver.go` — PeerResolver interface

**Rewrite:**
- `internal/tui/tui.go` → single-buffer root model (replaces tab shell)
- `internal/tui/theme.go` → simplified IRC-style theme

**New:**
- `internal/tui/input.go` — input line model (replaces chat_compose.go)
- `internal/tui/commands.go` — slash command parser + dispatch
- `internal/tui/scrollback.go` — scrollback buffer with styled line types
- `internal/tui/statusbar.go` — IRC-style status bar(s)
- `internal/tui/panels.go` — overlay panels for /nodes, /stats, /config, /location

**Delete:**
- `internal/tui/tabs/chat.go`
- `internal/tui/tabs/chat_compose.go`
- `internal/tui/tabs/nodes.go`
- `internal/tui/tabs/stats.go`
- `internal/tui/tabs/config.go`
- `internal/tui/tabs/location.go`
- `internal/tui/widgets/statusline.go`

## Line Types in Scrollback

Every line in the scrollback is a `ScrollLine` with a `Kind`:

```go
type LineKind int
const (
    LineChat       LineKind = iota // < [12:42] NodeB: hey everyone
    LineChatOut                    // > [12:42] hello *
    LineSystem                     // -- NodeC joined the mesh --
    LineDelivery                   // -- Delivery: NodeB ✓ NodeC ✓ --
    LineError                      // !! Connection lost
    LineInfo                       // :: 3 neighbors, 2 routes
    LineCommand                    // /nodes output rendered inline
)

type ScrollLine struct {
    Kind      LineKind
    Text      string      // pre-rendered with ANSI styles
    Timestamp time.Time
    Raw       interface{} // original data for re-rendering on resize
}
```

## Slash Commands

| Command | Action |
|---------|--------|
| `/b` or `/broadcast` | Switch to broadcast buffer |
| `/dm <addr-or-name>` | Switch to / create DM buffer |
| `/ch <N>` | Switch to channel N buffer |
| `/join <name>` | Alias for /ch by name |
| `/w` or `/windows` | List open buffers with unread counts |
| `/close` | Close current buffer (not broadcast) |
| `/nodes` | Print neighbor + route table inline |
| `/stats` | Print stats summary inline |
| `/config` | Print current config inline |
| `/config set <key> <val>` | Set config value (name, radio fields) |
| `/location` | Print GPS + peer locations inline |
| `/alias <addr> <name>` | Set peer alias |
| `/probe` | Send network probe |
| `/ping` | Ping connected node |
| `/reboot` | Reboot node (with confirm) |
| `/clear` | Clear scrollback |
| `/help` | Print command list |
| `/quit` or `/q` | Exit |

Buffer switching: `Alt+1-9` for first 9 buffers, `Ctrl+N`/`Ctrl+P` next/prev.

## Layout

```
┌──────────────────────────────────────────────────────────┐
│                                                          │ ← scrollback
│ -- Connected to 82E346A8 via ws://192.0.2.0/ws --   │    (viewport)
│ < [12:42] NodeB: hey everyone                            │
│ > [12:42] Hi all *                                       │
│ -- A1B2C3D4 (NodeC) joined [RSSI -68, SNR 9.5] --      │
│ < [12:43] NodeC: what's up                               │
│ > [12:44] not much *                                     │
│ -- Delivery: NodeB ✓ NodeC ✓ --                          │
│                                                          │
│                                                          │
├──────────────────────────────────────────────────────────┤
│ [●] broadcast | 82E346A8 (Node 1) | 3 peers | 12:44 PM │ ← status bar
├──────────────────────────────────────────────────────────┤
│ [broadcast] hello everyone█                              │ ← input line
└──────────────────────────────────────────────────────────┘
```

Status bar shows: connection dot, active buffer name, node address/name, peer count, unread indicator for other buffers (like `[2:DM*]`), clock.

---

## Task 1: Scrollback Buffer Model

**Files:**
- Create: `internal/tui/scrollback.go`

**Step 1: Write the scrollback model**

```go
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
    LineChat    LineKind = iota // incoming message
    LineChatOut                // outgoing message
    LineSystem                 // system event (join/part/connect/disconnect)
    LineDelivery               // delivery receipt summary
    LineError                  // error message
    LineInfo                   // informational (command output)
    LineCommand                // slash command echo
)

type ScrollLine struct {
    Kind      LineKind
    Timestamp time.Time
    Text      string // pre-rendered ANSI string
}

type Scrollback struct {
    lines    []ScrollLine
    viewport viewport.Model
    width    int
    height   int
    theme    ScrollTheme
    autoscroll bool // track if user is at bottom
}

type ScrollTheme struct {
    Timestamp  lipgloss.Style
    Incoming   lipgloss.Style
    Outgoing   lipgloss.Style
    System     lipgloss.Style
    Delivery   lipgloss.Style
    Error      lipgloss.Style
    Info       lipgloss.Style
    Command    lipgloss.Style
    Sender     lipgloss.Style
    SelfBadge  lipgloss.Style
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
    // Track whether user was at bottom before update
    atBottom := s.viewport.AtBottom()
    s.viewport, _ = s.viewport.Update(msg)
    s.autoscroll = s.viewport.AtBottom()
    // If user scrolled to bottom, re-enable autoscroll
    if !atBottom && s.viewport.AtBottom() {
        s.autoscroll = true
    }
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
```

**Step 2: Verify it compiles**

Run: `cd ~/src/bramble-cli && go build ./internal/tui/...`
Expected: no errors (file is self-contained)

**Step 3: Commit**

```bash
git add internal/tui/scrollback.go
git commit -m "feat(tui): add IRC-style scrollback buffer model"
```

---

## Task 2: Input Line Model

**Files:**
- Create: `internal/tui/input.go`

**Step 1: Write the input line model**

The input line is always visible, always focused (like IRC). It shows the active buffer name as a prompt. Pressing Enter sends. Lines starting with `/` are commands.

```go
package tui

import (
    "strings"

    "charm.land/bubbles/v2/textarea"
    tea "charm.land/bubbletea/v2"
    "charm.land/lipgloss/v2"
)

// InputMsg is sent when the user presses Enter with non-empty text.
type InputMsg struct {
    Text      string
    IsCommand bool   // starts with /
}

// InputLine is the always-visible input line at the bottom.
type InputLine struct {
    textarea textarea.Model
    prompt   string // e.g. "[broadcast]" or "[dm:NodeB]"
    width    int
    style    InputStyle
}

type InputStyle struct {
    Prompt lipgloss.Style
    Border lipgloss.Style
}

func NewInputLine() InputLine {
    ta := textarea.New()
    ta.Placeholder = "Type a message or /command..."
    ta.ShowLineNumbers = false
    ta.SetHeight(1)
    ta.CharLimit = 0

    // Enter sends; Ctrl+Enter / Alt+Enter for newline
    km := textarea.DefaultKeyMap()
    km.InsertNewline.SetKeys("ctrl+enter", "alt+enter")
    ta.KeyMap = km

    return InputLine{
        textarea: ta,
        prompt:   "[broadcast]",
        style: InputStyle{
            Prompt: lipgloss.NewStyle().
                Foreground(lipgloss.Color("#00FF87")).
                Bold(true),
            Border: lipgloss.NewStyle().
                BorderTop(true).
                BorderStyle(lipgloss.NormalBorder()).
                BorderForeground(lipgloss.Color("#555588")),
        },
    }
}

func (il *InputLine) SetPrompt(p string) {
    il.prompt = p
}

func (il *InputLine) SetWidth(w int) {
    il.width = w
    // Prompt takes some space; rest is textarea
    promptW := len(il.prompt) + 2 // prompt + space + cursor margin
    taW := w - promptW
    if taW < 20 {
        taW = 20
    }
    il.textarea.SetWidth(taW)
}

func (il *InputLine) Focus() tea.Cmd {
    return il.textarea.Focus()
}

func (il *InputLine) Blur() {
    il.textarea.Blur()
}

func (il InputLine) Update(msg tea.Msg) (InputLine, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyPressMsg:
        switch msg.String() {
        case "enter":
            text := strings.TrimSpace(il.textarea.Value())
            if text == "" {
                return il, nil
            }
            il.textarea.SetValue("")
            isCmd := strings.HasPrefix(text, "/")
            return il, func() tea.Msg {
                return InputMsg{Text: text, IsCommand: isCmd}
            }
        case "esc":
            il.textarea.SetValue("")
            return il, nil
        }
    }

    var cmd tea.Cmd
    il.textarea, cmd = il.textarea.Update(msg)
    return il, cmd
}

func (il InputLine) View() string {
    prompt := il.style.Prompt.Render(il.prompt)
    ta := il.textarea.View()
    line := prompt + " " + ta
    return il.style.Border.Width(il.width).Render(line)
}
```

**Step 2: Verify it compiles**

Run: `cd ~/src/bramble-cli && go build ./internal/tui/...`

**Step 3: Commit**

```bash
git add internal/tui/input.go
git commit -m "feat(tui): add IRC-style input line model"
```

---

## Task 3: Status Bar

**Files:**
- Create: `internal/tui/statusbar.go`

**Step 1: Write the status bar**

```go
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
    style     StatusBarStyle
}

type StatusBarStyle struct {
    Bar       lipgloss.Style
    ConnOK    lipgloss.Style
    ConnFail  lipgloss.Style
    Active    lipgloss.Style
    Inactive  lipgloss.Style
    Unread    lipgloss.Style
    Info      lipgloss.Style
    Clock     lipgloss.Style
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

func (sb StatusBar) View() string {
    var parts []string

    // Connection status
    if sb.connected {
        parts = append(parts, sb.style.ConnOK.Render("●"))
    } else {
        parts = append(parts, sb.style.ConnFail.Render("○"))
    }

    // Buffer indicators: [1:broadcast] [2:DM*]
    for i, buf := range sb.buffers {
        label := fmt.Sprintf("%d:%s", i+1, buf.Label)
        if buf.Active {
            parts = append(parts, sb.style.Active.Render("["+label+"]"))
        } else if buf.Unread > 0 {
            parts = append(parts, sb.style.Unread.Render(fmt.Sprintf("[%s(%d)]", label, buf.Unread)))
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
```

**Step 2: Verify compile**

Run: `cd ~/src/bramble-cli && go build ./internal/tui/...`

**Step 3: Commit**

```bash
git add internal/tui/statusbar.go
git commit -m "feat(tui): add IRC-style status bar"
```

---

## Task 4: Slash Command Parser

**Files:**
- Create: `internal/tui/commands.go`

**Step 1: Write the command parser and dispatch stubs**

```go
package tui

import (
    "context"
    "fmt"
    "strconv"
    "strings"
    "time"

    bramble "github.com/justinlindh/bramble-go"
)

// Command represents a parsed slash command.
type Command struct {
    Name string
    Args []string
    Raw  string
}

// ParseCommand parses a slash command string.
// Returns nil if the input is not a command.
func ParseCommand(input string) *Command {
    if !strings.HasPrefix(input, "/") {
        return nil
    }
    parts := strings.Fields(input[1:]) // strip leading /
    if len(parts) == 0 {
        return nil
    }
    return &Command{
        Name: strings.ToLower(parts[0]),
        Args: parts[1:],
        Raw:  input,
    }
}

// CommandHandler executes commands and writes output to the scrollback.
type CommandHandler struct {
    client   *bramble.Client
    store    *Store
    scroll   *Scrollback
    resolver PeerResolver
    // Callbacks into the root model
    onSwitchBuffer func(convID string)
    onQuit         func()
    onConfirm      func(prompt string, action func())
}

func NewCommandHandler(client *bramble.Client, store *Store, scroll *Scrollback, resolver PeerResolver) *CommandHandler {
    return &CommandHandler{
        client:   client,
        store:    store,
        scroll:   scroll,
        resolver: resolver,
    }
}

// Execute runs a parsed command. Returns true if the command was recognized.
func (h *CommandHandler) Execute(cmd *Command) bool {
    switch cmd.Name {
    case "b", "broadcast":
        h.cmdSwitchBuffer("broadcast")
    case "dm":
        h.cmdDM(cmd.Args)
    case "ch":
        h.cmdChannel(cmd.Args)
    case "w", "windows":
        h.cmdWindows()
    case "close":
        h.cmdClose()
    case "nodes":
        h.cmdNodes()
    case "stats":
        h.cmdStats()
    case "config":
        h.cmdConfig(cmd.Args)
    case "location", "loc":
        h.cmdLocation()
    case "alias":
        h.cmdAlias(cmd.Args)
    case "probe":
        h.cmdProbe()
    case "ping":
        h.cmdPing()
    case "reboot":
        h.cmdReboot()
    case "clear":
        h.scroll.Clear()
    case "help", "h":
        h.cmdHelp()
    case "quit", "q":
        if h.onQuit != nil {
            h.onQuit()
        }
    default:
        h.scroll.AddError(fmt.Sprintf("Unknown command: /%s (try /help)", cmd.Name))
        return false
    }
    return true
}

func (h *CommandHandler) cmdSwitchBuffer(convID string) {
    if h.onSwitchBuffer != nil {
        h.onSwitchBuffer(convID)
    }
}

func (h *CommandHandler) cmdDM(args []string) {
    if len(args) < 1 {
        h.scroll.AddError("Usage: /dm <address-or-name>")
        return
    }
    // Try to resolve name to address
    target := args[0]
    addr := h.resolveTarget(target)
    if addr == "" {
        h.scroll.AddError(fmt.Sprintf("Unknown peer: %s", target))
        return
    }
    convID := "dm:" + addr
    h.cmdSwitchBuffer(convID)
}

func (h *CommandHandler) cmdChannel(args []string) {
    if len(args) < 1 {
        h.scroll.AddError("Usage: /ch <number>")
        return
    }
    n, err := strconv.Atoi(args[0])
    if err != nil {
        h.scroll.AddError(fmt.Sprintf("Invalid channel number: %s", args[0]))
        return
    }
    convID := fmt.Sprintf("ch:%d", n)
    h.cmdSwitchBuffer(convID)
}

func (h *CommandHandler) cmdWindows() {
    convs := h.store.GetConversations()
    if len(convs) == 0 {
        h.scroll.AddInfo("  No open buffers")
        return
    }
    h.scroll.AddInfo("  Open buffers:")
    active := h.store.ActiveConvID
    for i, c := range convs {
        marker := "  "
        if c.ID == active {
            marker = "> "
        }
        unread := ""
        if c.Unread > 0 {
            unread = fmt.Sprintf(" (%d unread)", c.Unread)
        }
        h.scroll.AddInfo(fmt.Sprintf("  %s%d: %s%s", marker, i+1, c.Label, unread))
    }
}

func (h *CommandHandler) cmdClose() {
    active := h.store.ActiveConvID
    if active == "broadcast" {
        h.scroll.AddError("Can't close broadcast buffer")
        return
    }
    // Switch back to broadcast
    h.cmdSwitchBuffer("broadcast")
    h.scroll.AddSystem(fmt.Sprintf("Closed buffer: %s", active))
}

func (h *CommandHandler) cmdNodes() {
    h.store.mu.RLock()
    neighbors := h.store.Neighbors
    routes := h.store.Routes
    h.store.mu.RUnlock()

    h.scroll.AddInfo(fmt.Sprintf("  Neighbors (%d):", len(neighbors)))
    if len(neighbors) == 0 {
        h.scroll.AddInfo("    (none)")
    }
    for _, n := range neighbors {
        name := n.Address
        if h.resolver != nil {
            name = h.resolver.Resolve(n.Address)
        }
        ago := fmtDurationShort(time.Duration(n.LastSeenAgoMs) * time.Millisecond)
        h.scroll.AddInfo(fmt.Sprintf("    %-12s  %-12s  %4ddBm  SNR %.1f  %s",
            n.Address, name, n.RSSI, n.SNR, ago))
    }

    h.scroll.AddInfo(fmt.Sprintf("  Routes (%d):", len(routes)))
    if len(routes) == 0 {
        h.scroll.AddInfo("    (none)")
    }
    for _, r := range routes {
        h.scroll.AddInfo(fmt.Sprintf("    %-12s  via %-12s  %d hops  metric %d  %s",
            r.Dest, r.NextHop, r.HopCount, r.Metric, r.State))
    }
}

func (h *CommandHandler) cmdStats() {
    h.store.mu.RLock()
    status := h.store.Status
    airtime := h.store.Airtime
    h.store.mu.RUnlock()

    if status == nil {
        h.scroll.AddError("No status data available yet")
        return
    }

    h.scroll.AddInfo("  Node Status:")
    h.scroll.AddInfo(fmt.Sprintf("    Uptime: %s", fmtDurationShort(time.Duration(status.UptimeMs)*time.Millisecond)))
    h.scroll.AddInfo(fmt.Sprintf("    Messages: TX %d  RX %d  Fwd %d",
        status.Counters.TxMessages, status.Counters.RxMessages, status.Counters.FwdMessages))
    h.scroll.AddInfo(fmt.Sprintf("    Beacons:  TX %d  RX %d",
        status.Counters.TxBeacons, status.Counters.RxBeacons))

    if airtime != nil && len(airtime.Tiers) > 0 {
        h.scroll.AddInfo("  Airtime:")
        for _, t := range airtime.Tiers {
            pct := 0
            if t.MaxMs > 0 {
                pct = t.RemainingMs * 100 / t.MaxMs
            }
            h.scroll.AddInfo(fmt.Sprintf("    %-12s  %d%% remaining", t.Name, pct))
        }
    }
}

func (h *CommandHandler) cmdConfig(args []string) {
    if len(args) >= 3 && args[0] == "set" {
        h.cmdConfigSet(args[1], strings.Join(args[2:], " "))
        return
    }

    // Read-only config dump
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    cfg, err := h.client.Config(ctx)
    if err != nil {
        h.scroll.AddError(fmt.Sprintf("Config fetch error: %v", err))
        return
    }

    h.scroll.AddInfo("  Identity:")
    h.scroll.AddInfo(fmt.Sprintf("    Address:    %s", cfg.Address))
    h.scroll.AddInfo(fmt.Sprintf("    Name:       %s", cfg.NodeName))

    h.scroll.AddInfo("  Radio:")
    h.scroll.AddInfo(fmt.Sprintf("    Frequency:  %.3f MHz", cfg.Radio.FrequencyMhz))
    h.scroll.AddInfo(fmt.Sprintf("    SF:         %d", cfg.Radio.SF))
    h.scroll.AddInfo(fmt.Sprintf("    Bandwidth:  %d Hz", cfg.Radio.BwHz))
    h.scroll.AddInfo(fmt.Sprintf("    TX Power:   %d dBm", cfg.Radio.TxPowerDbm))

    if len(cfg.Channels) > 0 {
        h.scroll.AddInfo("  Channels:")
        for i, ch := range cfg.Channels {
            def := ""
            if ch.IsDefault {
                def = " ★"
            }
            h.scroll.AddInfo(fmt.Sprintf("    [%d] %s%s", i, ch.Name, def))
        }
    }
}

func (h *CommandHandler) cmdConfigSet(key, value string) {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    switch strings.ToLower(key) {
    case "name":
        if len(value) > 8 {
            h.scroll.AddError("Name max 8 characters")
            return
        }
        err := h.client.SetNodeName(ctx, value)
        if err != nil {
            h.scroll.AddError(fmt.Sprintf("Error: %v", err))
        } else {
            h.scroll.AddSystem(fmt.Sprintf("Node name set to %q", value))
        }
    default:
        h.scroll.AddError(fmt.Sprintf("Unknown config key: %s (settable: name)", key))
    }
}

func (h *CommandHandler) cmdLocation() {
    h.store.mu.RLock()
    gps := h.store.OwnGPS
    peers := h.store.PeerLocations
    h.store.mu.RUnlock()

    if gps != nil {
        h.scroll.AddInfo(fmt.Sprintf("  My GPS: %.6f, %.6f  Alt %.0fm  Sats %d",
            gps.Lat, gps.Lon, gps.Alt, gps.Sats))
    } else {
        h.scroll.AddInfo("  My GPS: no fix")
    }

    if len(peers) == 0 {
        h.scroll.AddInfo("  No peer locations")
        return
    }
    h.scroll.AddInfo(fmt.Sprintf("  Peer Locations (%d):", len(peers)))
    for _, p := range peers {
        name := p.Address
        if h.resolver != nil {
            name = h.resolver.Resolve(p.Address)
        }
        h.scroll.AddInfo(fmt.Sprintf("    %-12s  %.6f, %.6f", name, p.Lat, p.Lon))
    }
}

func (h *CommandHandler) cmdAlias(args []string) {
    if len(args) < 2 {
        h.scroll.AddError("Usage: /alias <address> <name>")
        return
    }
    addr := args[0]
    name := strings.Join(args[1:], " ")
    if h.resolver != nil {
        err := h.resolver.SetAlias(addr, name)
        if err != nil {
            h.scroll.AddError(fmt.Sprintf("Error: %v", err))
            return
        }
    }
    h.scroll.AddSystem(fmt.Sprintf("Alias set: %s → %s", addr, name))
}

func (h *CommandHandler) cmdProbe() {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    _, err := h.client.SendProbe(ctx)
    if err != nil {
        h.scroll.AddError(fmt.Sprintf("Probe error: %v", err))
        return
    }
    h.scroll.AddSystem("Network probe sent — results will appear as they arrive")
}

func (h *CommandHandler) cmdPing() {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    start := time.Now()
    err := h.client.Ping(ctx)
    if err != nil {
        h.scroll.AddError(fmt.Sprintf("Ping failed: %v", err))
        return
    }
    h.scroll.AddInfo(fmt.Sprintf("  Pong: %dms", time.Since(start).Milliseconds()))
}

func (h *CommandHandler) cmdReboot() {
    if h.onConfirm != nil {
        h.onConfirm("Reboot node? Type /reboot-confirm to proceed", func() {
            ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
            defer cancel()
            err := h.client.Reboot(ctx)
            if err != nil {
                h.scroll.AddError(fmt.Sprintf("Reboot error: %v", err))
            } else {
                h.scroll.AddSystem("Rebooting node...")
            }
        })
        return
    }
}

func (h *CommandHandler) cmdHelp() {
    h.scroll.AddInfo("  Commands:")
    h.scroll.AddInfo("    /b, /broadcast        Switch to broadcast")
    h.scroll.AddInfo("    /dm <addr|name>       Open/switch to DM")
    h.scroll.AddInfo("    /ch <number>          Switch to channel")
    h.scroll.AddInfo("    /w, /windows          List open buffers")
    h.scroll.AddInfo("    /close                Close current buffer")
    h.scroll.AddInfo("    /nodes                Show neighbors & routes")
    h.scroll.AddInfo("    /stats                Show node statistics")
    h.scroll.AddInfo("    /config               Show configuration")
    h.scroll.AddInfo("    /config set <k> <v>   Set config value")
    h.scroll.AddInfo("    /location             Show GPS & peer locations")
    h.scroll.AddInfo("    /alias <addr> <name>  Set peer alias")
    h.scroll.AddInfo("    /probe                Send network probe")
    h.scroll.AddInfo("    /ping                 Ping node")
    h.scroll.AddInfo("    /reboot               Reboot node")
    h.scroll.AddInfo("    /clear                Clear scrollback")
    h.scroll.AddInfo("    /help                 This help")
    h.scroll.AddInfo("    /quit                 Exit")
    h.scroll.AddInfo("")
    h.scroll.AddInfo("  Navigation:")
    h.scroll.AddInfo("    Alt+1-9               Switch buffer by number")
    h.scroll.AddInfo("    Ctrl+N / Ctrl+P       Next / prev buffer")
    h.scroll.AddInfo("    PgUp / PgDn           Scroll history")
    h.scroll.AddInfo("    Home / End            Top / bottom of history")
}

func (h *CommandHandler) resolveTarget(target string) string {
    // If it looks like a hex address, use directly
    if len(target) == 8 {
        allHex := true
        for _, c := range target {
            if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
                allHex = false
                break
            }
        }
        if allHex {
            return strings.ToUpper(target)
        }
    }
    // Try reverse-resolve from name
    if h.resolver != nil {
        if addr, ok := h.resolver.ReverseLookup(target); ok {
            return addr
        }
    }
    return ""
}

// fmtDurationShort formats a duration as a compact human string.
func fmtDurationShort(d time.Duration) string {
    switch {
    case d < time.Second:
        return "now"
    case d < time.Minute:
        return fmt.Sprintf("%ds", int(d.Seconds()))
    case d < time.Hour:
        return fmt.Sprintf("%dm", int(d.Minutes()))
    case d < 24*time.Hour:
        return fmt.Sprintf("%dh", int(d.Hours()))
    default:
        return fmt.Sprintf("%dd", int(d.Hours()/24))
    }
}
```

**Step 2: Add `ReverseLookup` to the PeerResolver interface**

Modify `internal/tui/tabs/resolver.go`:

```go
type PeerResolver interface {
    Resolve(addr string) string
    SetAlias(addr, name string) error
    GetAlias(addr string) (string, error)
    ReverseLookup(name string) (string, bool) // NEW
}
```

And implement it in `internal/tui/names.go`:

```go
// ReverseLookup finds an address by name (alias or firmware name).
func (r *NameResolver) ReverseLookup(name string) (string, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    lower := strings.ToLower(name)
    // Check aliases first
    for addr, alias := range r.aliases {
        if strings.ToLower(alias) == lower {
            return addr, true
        }
    }
    // Check firmware names
    for addr, fw := range r.fwNames {
        if strings.ToLower(fw) == lower {
            return addr, true
        }
    }
    return "", false
}
```

**Step 3: Verify compile**

Run: `cd ~/src/bramble-cli && go build ./internal/tui/...`

**Step 4: Commit**

```bash
git add internal/tui/commands.go internal/tui/tabs/resolver.go internal/tui/names.go
git commit -m "feat(tui): add slash command parser + handler"
```

---

## Task 5: Rewrite Root Model (tui.go)

**Files:**
- Rewrite: `internal/tui/tui.go`
- Rewrite: `internal/tui/theme.go`

This is the core task — replaces the entire tab-based shell with the IRC layout.

**Step 1: Rewrite theme.go**

```go
package tui

import "charm.land/lipgloss/v2"

// Theme holds styles for the IRC-style TUI.
type Theme struct {
    ScrollTheme  ScrollTheme
    StatusBar    StatusBarStyle
    Input        InputStyle
}

func DefaultTheme() Theme {
    return Theme{
        ScrollTheme: NewScrollTheme(),
    }
}

// PeerResolver re-export to avoid import cycle in commands.go
// (PeerResolver interface is in tabs/resolver.go but used here)
var _ = lipgloss.NewStyle() // keep import
```

**Step 2: Rewrite tui.go**

The new root model:
- Scrollback viewport fills all space above status bar + input
- Input line is always focused
- Bridge events flow into scrollback as styled lines
- Poll tick refreshes store data silently
- No tabs, no split panes

```go
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

// ── Fetch result Msgs (reused from old code) ─────────────────────────────────

type fetchStatusResult struct{ status *bramble.StatusResponse; err error }
type fetchNeighborsResult struct{ neighbors []bramble.Neighbor; err error }
type fetchRoutesResult struct{ routes []bramble.Route; err error }
type fetchAirtimeResult struct{ airtime *bramble.AirtimeStats; err error }
type fetchPeerLocsResult struct{ peers []bramble.LocationPeer; err error }

// ── Reconnect Msgs ───────────────────────────────────────────────────────────

type reconnectMsg struct{}
type reconnectResult struct {
    client *bramble.Client
    node   NodeInfo
    err    error
}

// ── Send result (from input) ──────────────────────────────────────────────────

type sendResultMsg struct {
    convID string
    text   string
    msgID  string
    err    error
}

// ── Model ────────────────────────────────────────────────────────────────────

type Model struct {
    client    *bramble.Client
    connectFn ConnectFn
    bridge    *Bridge
    store     *Store

    scroll    Scrollback
    statusBar StatusBar
    input     InputLine
    cmdHandler *CommandHandler

    width     int
    height    int
    node      NodeInfo
    ready     bool
    connected bool

    activeConv string // current buffer: "broadcast", "ch:N", "dm:ADDR"

    pollCount  int
    backoffSec int
    retryIn    int

    // Confirm state for destructive commands
    pendingConfirm func()
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

    scroll := NewScrollback()
    statusBar := NewStatusBar()
    input := NewInputLine()

    var resolver tabs.PeerResolver
    if store.Resolver != nil {
        resolver = store.Resolver
    }
    cmdHandler := NewCommandHandler(client, store, &scroll, resolver)

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

    // Wire command callbacks
    m.cmdHandler.onSwitchBuffer = func(convID string) {
        m.switchBuffer(convID)
    }
    m.cmdHandler.onQuit = nil // handled in Update via InputMsg

    return m
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
        // Only show messages for the active buffer
        convID := ClassifyMessageConvID(msg, m.node.Address)
        if convID == m.activeConv {
            outgoing := sm.Direction == "out"
            sender := msg.From
            if m.store.Resolver != nil && !outgoing {
                sender = m.store.Resolver.Resolve(sender)
            }
            badge := ""
            if outgoing {
                badge = badgeFor(sm.Status)
            }
            m.scroll.AddChat(sender, msg.Text, badge, outgoing)
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

func (m *Model) switchBuffer(convID string) {
    m.activeConv = convID
    m.store.SetActiveConv(convID)
    // Update input prompt
    label := convID
    if convID == "broadcast" {
        label = "broadcast"
    } else if strings.HasPrefix(convID, "dm:") {
        addr := convID[3:]
        if m.store.Resolver != nil {
            label = "dm:" + m.store.Resolver.Resolve(addr)
        }
    }
    m.input.SetPrompt("[" + label + "]")
    // Reload scrollback for this buffer
    m.reloadScrollback()
    m.scroll.AddSystem(fmt.Sprintf("Switched to %s", label))
}

func (m *Model) reloadScrollback() {
    m.scroll.Clear()
    conv := m.store.Conversations[m.activeConv]
    if conv == nil {
        return
    }
    for _, msg := range conv.Messages {
        outgoing := msg.From == m.node.Address || msg.From == ""
        sender := msg.From
        if m.store.Resolver != nil && !outgoing {
            sender = m.store.Resolver.Resolve(sender)
        }
        badge := ""
        if outgoing {
            badge = "*"
        }
        m.scroll.AddChat(sender, msg.Text, badge, outgoing)
    }
}

func (m *Model) updateStatusBar() {
    m.statusBar.SetConnection(m.connected, m.node.Address, m.node.Name)
    m.store.mu.RLock()
    m.statusBar.SetPeerCount(len(m.store.Neighbors))
    convs := m.store.GetConversations()
    m.store.mu.RUnlock()

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

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
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
        // Layout: scrollback gets all height minus statusbar(1) + input(3)
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

        // Reboot confirm
        if m.pendingConfirm != nil && key == "y" {
            fn := m.pendingConfirm
            m.pendingConfirm = nil
            fn()
            return m, nil
        } else if m.pendingConfirm != nil {
            m.pendingConfirm = nil
            m.scroll.AddSystem("Cancelled")
            return m, nil
        }

        // Global keys
        switch key {
        case "ctrl+c":
            return m, tea.Quit

        // Buffer switching: Alt+1-9
        case "alt+1", "alt+2", "alt+3", "alt+4", "alt+5",
             "alt+6", "alt+7", "alt+8", "alt+9":
            idx := int(key[4] - '1')
            convs := m.store.GetConversations()
            if idx < len(convs) {
                m.switchBuffer(convs[idx].ID)
            }
            return m, nil

        // Buffer cycling
        case "ctrl+n":
            m.cycleBuffer(1)
            return m, nil
        case "ctrl+p":
            m.cycleBuffer(-1)
            return m, nil

        // Scroll keys → viewport
        case "pgup", "pgdown", "home", "end":
            m.scroll.Update(msg)
            return m, nil
        }

        // Everything else → input line
        var inputCmd tea.Cmd
        m.input, inputCmd = m.input.Update(msg)
        cmds = append(cmds, inputCmd)

    // ── Input submission ──────────────────────────────────────────────────
    case InputMsg:
        if msg.IsCommand {
            cmd := ParseCommand(msg.Text)
            if cmd != nil {
                if cmd.Name == "quit" || cmd.Name == "q" {
                    return m, tea.Quit
                }
                if cmd.Name == "reboot-confirm" && m.pendingConfirm != nil {
                    fn := m.pendingConfirm
                    m.pendingConfirm = nil
                    fn()
                    return m, nil
                }
                m.cmdHandler.Execute(cmd)
            }
        } else {
            // Send message
            return m, m.sendMessage(msg.Text)
        }

    case sendResultMsg:
        if msg.err != nil {
            m.scroll.AddError(fmt.Sprintf("Send failed: %v", msg.err))
        } else {
            m.scroll.AddChat("me", msg.text, "*", true)
            // Store the outgoing message
            raw := bramble.Message{
                From:      m.node.Address,
                To:        convIDToAddr(m.activeConv),
                Text:      msg.text,
                MsgID:     msg.msgID,
                Timestamp: time.Now().Unix(),
            }
            m.store.AddMessage(raw)
        }

    // ── Poll tick ───────────────────────────────────────────────────────────
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

    // ── Fetch results ────────────────────────────────────────────────────────
    case fetchStatusResult:
        if msg.err != nil && m.connected {
            m.connected = false
            m.scroll.AddError("Connection lost")
            return m, m.scheduleReconnect()
        }
        if msg.err == nil {
            m.store.UpdateStatus(msg.status)
            m.updateStatusBar()
        }
    case fetchNeighborsResult:
        if msg.err == nil {
            // Detect new neighbors for system messages
            oldAddrs := make(map[string]bool)
            m.store.mu.RLock()
            for _, n := range m.store.Neighbors {
                oldAddrs[n.Address] = true
            }
            m.store.mu.RUnlock()

            m.store.UpdateNeighbors(msg.neighbors)
            m.updateStatusBar()

            for _, n := range msg.neighbors {
                if !oldAddrs[n.Address] {
                    name := n.Address
                    if m.store.Resolver != nil {
                        name = m.store.Resolver.Resolve(n.Address)
                    }
                    m.scroll.AddSystem(fmt.Sprintf("%s joined [RSSI %d, SNR %.1f]", name, n.RSSI, n.SNR))
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

    // ── Bridge Msgs ──────────────────────────────────────────────────────────
    case MsgReceived:
        m.store.AddMessage(msg.Msg)
        convID := ClassifyMessageConvID(msg.Msg, m.node.Address)
        if convID == m.activeConv {
            sender := msg.Msg.From
            if m.store.Resolver != nil {
                sender = m.store.Resolver.Resolve(sender)
            }
            m.scroll.AddChat(sender, msg.Msg.Text, "", false)
        }
        m.updateStatusBar()

    case AckReceived:
        m.store.UpdateAck(msg.Ack)

    case BroadcastDeliveryReceived:
        if m.store.msgdb != nil {
            d := msg.Delivery
            go func() { _ = m.store.msgdb.UpdateStatus(d.BroadcastID, d.Status) }()
        }
        // Show delivery in scrollback
        d := msg.Delivery
        status := "✓"
        if d.Status == "failed" {
            status = "✗"
        }
        peer := d.Peer
        if m.store.Resolver != nil {
            peer = m.store.Resolver.Resolve(peer)
        }
        if m.activeConv == "broadcast" {
            m.scroll.AddDelivery(fmt.Sprintf("%s %s", peer, status))
        }

    case NeighborChanged:
        return m, m.fetchNeighbors()

    case GpsEventReceived:
        m.store.UpdateOwnGPS(msg.Event)

    case TrafficEventReceived:
        // Could show inline if traffic debug is on

    case ProbeResultReceived:
        r := msg.Result
        name := r.Address
        if m.store.Resolver != nil {
            name = m.store.Resolver.Resolve(r.Address)
        }
        m.scroll.AddInfo(fmt.Sprintf("  Probe: %s  %dms  %d hops  RSSI %d",
            name, r.RttMs, r.Hops, r.RSSI))

    case ProbeCompleteReceived:
        m.scroll.AddSystem("Probe complete")

    // ── Reconnect ────────────────────────────────────────────────────────────
    case reconnectMsg:
        return m, m.doReconnect()

    case reconnectResult:
        if msg.err != nil {
            m.scroll.AddError(fmt.Sprintf("Reconnect failed: %v", msg.err))
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
        m.scroll.AddSystem("Reconnected")
        if m.bridge != nil {
            m.bridge.Start(m.client)
        }
        m.updateStatusBar()
        return m, m.fetchInitialData()
    }

    return m, tea.Batch(cmds...)
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

// View implements tea.Model.
func (m Model) View() tea.View {
    if !m.ready {
        v := tea.NewView("Connecting...")
        v.AltScreen = true
        return v
    }

    var sb strings.Builder

    // Scrollback (fills available space)
    scrollView := m.scroll.View()
    // Pad scrollback to full size
    scrollView = padToSize(scrollView, m.width, m.height-4)
    sb.WriteString(scrollView)
    sb.WriteString("\n")

    // Status bar
    m.updateStatusBar()
    sb.WriteString(m.statusBar.View())
    sb.WriteString("\n")

    // Input line
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
    m.scroll.AddSystem(fmt.Sprintf("Reconnecting in %ds...", m.retryIn))
    return tea.Tick(delay, func(t time.Time) tea.Msg { return reconnectMsg{} })
}

// ── Send command ─────────────────────────────────────────────────────────────

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
```

**Step 3: Update cmd/bramble entry point if it references old tab types**

Check `cmd/bramble/tui.go` (or wherever `bramble tui` is wired) and ensure it still calls `tui.New(...)` with the same signature. The `New` function signature is unchanged: `New(client, node, connectFn, msgdb)`.

**Step 4: Delete old tab files**

```bash
rm internal/tui/tabs/chat.go
rm internal/tui/tabs/chat_compose.go
rm internal/tui/tabs/nodes.go
rm internal/tui/tabs/stats.go
rm internal/tui/tabs/config.go
rm internal/tui/tabs/location.go
rm internal/tui/widgets/statusline.go
```

**Step 5: Build and fix any compilation issues**

Run: `cd ~/src/bramble-cli && go build -o /tmp/bramble ./cmd/bramble/`

This will likely have a few issues to resolve:
- Import paths referencing deleted tab types
- The `cmd/bramble` entry point may reference `tabs.StatsModel` etc — remove those
- The `PeerResolver` interface lives in `tabs/resolver.go` which we kept — verify imports

**Step 6: Verify it builds clean**

Run: `cd ~/src/bramble-cli && go build -o /tmp/bramble ./cmd/bramble/`
Expected: clean build

**Step 7: Commit**

```bash
git add -A
git commit -m "feat(tui): IRC-style rewrite — single buffer + slash commands

Replace tab-based split-pane layout with BitchX-inspired design:
- Full-screen scrollback buffer for messages and events
- Always-ready input line with buffer prompt
- Slash commands for all non-chat features (/nodes, /stats, /config, etc.)
- Buffer switching via /dm, /ch, /broadcast, Alt+1-9, Ctrl+N/P
- Inline system events for neighbor joins, delivery receipts
- IRC-style status bar with buffer list and unread counts

Deletes: chat.go, chat_compose.go, nodes.go, stats.go, config.go,
location.go, statusline.go (all absorbed into new architecture)"
```

---

## Task 6: Wire Entry Point + Welcome Message

**Files:**
- Modify: `cmd/bramble/tui.go` (or wherever the TUI command is defined)

**Step 1: Verify entry point compiles with new model**

The entry point should already work since `New()` has the same signature. Just ensure no references to old tab types remain.

**Step 2: Add welcome/connect message**

After `New()` returns and before the program starts, add an initial system message to the scrollback:

In `tui.go` Init(), add at the start:

```go
func (m Model) Init() tea.Cmd {
    // Welcome message
    m.scroll.AddSystem(fmt.Sprintf("Connected to %s via %s", m.node.Address, m.node.Transport))
    if m.node.Name != "" {
        m.scroll.AddSystem(fmt.Sprintf("Node: %s", m.node.Name))
    }
    m.scroll.AddSystem("Type /help for commands")

    return tea.Batch(
        tickCmd(),
        clockTickCmd(),
        m.input.Focus(),
        m.fetchInitialData(),
    )
}
```

**Step 3: Build + test manually**

Run: `cd ~/src/bramble-cli && go build -o /tmp/bramble ./cmd/bramble/`
Then: `/tmp/bramble tui --transport ws://192.0.2.0/ws`

Expected: single-buffer IRC layout, broadcast buffer active, messages flowing, /help works.

**Step 4: Commit**

```bash
git add -A
git commit -m "feat(tui): wire entry point + welcome message"
```

---

## Task 7: Polish + Edge Cases

**Files:**
- Modify: `internal/tui/tui.go`
- Modify: `internal/tui/scrollback.go`
- Modify: `internal/tui/commands.go`

**Step 1: Handle message-too-long warning**

In the input line or scrollback, show a byte counter warning when text > 150 bytes (like the old compose bar). Add this to `input.go` View():

```go
// After textarea, show byte count if > 100
text := il.textarea.Value()
byteCount := len([]byte(text))
if byteCount > 100 {
    // render counter beside prompt
}
```

**Step 2: Handle /reboot confirm flow properly**

Wire `onConfirm` in the command handler to set `m.pendingConfirm`:

```go
m.cmdHandler.onConfirm = func(prompt string, action func()) {
    m.scroll.AddSystem(prompt)
    m.pendingConfirm = action
}
```

**Step 3: Ensure DM messages from peers auto-create buffers**

When `MsgReceived` arrives for a DM not in the active buffer, ensure the store creates the buffer and the status bar shows the unread count. Already handled by `store.AddMessage` but verify the status bar updates.

**Step 4: Build + test**

Run: `cd ~/src/bramble-cli && go build -o /tmp/bramble ./cmd/bramble/`

**Step 5: Commit**

```bash
git add -A
git commit -m "fix(tui): polish edge cases — byte counter, confirm flow, auto-buffers"
```

---

## Execution Notes

- **Tasks 1-3** are independent and can be parallelized (scrollback, input, statusbar are standalone files)
- **Task 4** depends on Tasks 1+3 (commands write to scrollback, need resolver)
- **Task 5** depends on all previous tasks (brings everything together)
- **Tasks 6-7** are sequential polish

The old `tabs/*.go` files only get deleted in Task 5. Until then, the code has both old and new files; it compiles because Go only complains about unused imports, not unused files.

**Key risk:** The Bubbles v2 `viewport` and `textarea` APIs. If they've changed from what's in the plan, the agent will need to check `go doc` for the exact method signatures. Key things to verify:
- `viewport.New()` vs `viewport.New(width, height)` — v2 may use setters
- `viewport.AtBottom()` exists
- `textarea.SetHeight()` / `textarea.SetWidth()` exist
- `tea.Every` and `tea.Tick` signatures
