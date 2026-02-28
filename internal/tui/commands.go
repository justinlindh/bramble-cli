// Package tui — commands.go
// Slash command parser and dispatcher for the IRC-style TUI.

package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	bramble "github.com/justinlindh/bramble-go"

	"github.com/justinlindh/bramble-cli/internal/tui/tabs"
)

// Command represents a parsed slash command.
type Command struct {
	Name string
	Args []string
	Raw  string
}

// ParseCommand parses a slash command string.
// Returns nil if the input is not a slash command or is empty.
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

// CmdAction represents a side-effect the root model should apply after a command.
type CmdAction struct {
	SwitchBuffer string // non-empty = switch to this buffer
	SendText     string // non-empty = send this text as a message
	SendTo       string // optional DM target address for direct send (without switching)
	SendCritical bool   // send as critical-priority message
	Quit         bool
	Reboot       bool // request reboot confirmation
}

// CommandHandler executes commands and writes output to the scrollback.
type CommandHandler struct {
	client   *bramble.Client
	store    *Store
	scroll   *Scrollback
	resolver tabs.PeerResolver
}

// NewCommandHandler creates a CommandHandler. Callbacks may be set after construction.
func NewCommandHandler(client *bramble.Client, store *Store, scroll *Scrollback, resolver tabs.PeerResolver) *CommandHandler {
	return &CommandHandler{
		client:   client,
		store:    store,
		scroll:   scroll,
		resolver: resolver,
	}
}

func (h *CommandHandler) activeConvID() string {
	h.store.mu.RLock()
	defer h.store.mu.RUnlock()
	if h.store.ActiveConvID == "" {
		return "broadcast"
	}
	return h.store.ActiveConvID
}

func (h *CommandHandler) persistLine(kind LineKind, rendered string) {
	h.store.AddConversationLine(h.activeConvID(), ScrollLine{Kind: kind, Timestamp: time.Now(), Text: rendered})
}

func (h *CommandHandler) addError(text string) {
	h.persistLine(LineError, h.scroll.theme.Error.Render("!! "+text))
	h.scroll.AddError(text)
}

func (h *CommandHandler) addInfo(text string) {
	h.persistLine(LineInfo, h.scroll.theme.Info.Render(text))
	h.scroll.AddInfo(text)
}

func (h *CommandHandler) addSystem(text string) {
	h.persistLine(LineSystem, h.scroll.theme.System.Render("-- "+text+" --"))
	h.scroll.AddSystem(text)
}

// Execute runs a parsed command. Returns a CmdAction for the root model to apply.
func (h *CommandHandler) Execute(cmd *Command) CmdAction {
	switch cmd.Name {
	case "b", "broadcast":
		return CmdAction{SwitchBuffer: "broadcast"}
	case "dm":
		return h.cmdDM(cmd.Args)
	case "msg":
		return h.cmdMsg(cmd.Args)
	case "critical":
		return h.cmdCritical(cmd.Args)
	case "ch":
		return h.cmdChannel(cmd.Args)
	case "w", "windows":
		h.cmdWindows()
	case "close":
		return h.cmdClose()
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
	case "nick":
		h.cmdNick(cmd.Args)
	case "me":
		return h.cmdMe(cmd.Args)
	case "slap":
		return h.cmdSlap(cmd.Args)
	case "probe":
		h.cmdProbe()
	case "ping":
		h.cmdPing()
	case "reboot":
		return CmdAction{Reboot: true}
	case "clear":
		h.scroll.Clear()
	case "help", "h":
		h.cmdHelp()
	case "quit", "q":
		return CmdAction{Quit: true}
	default:
		h.addError(fmt.Sprintf("Unknown command: /%s (try /help)", cmd.Name))
	}
	return CmdAction{}
}

func (h *CommandHandler) cmdDM(args []string) CmdAction {
	if len(args) < 1 {
		h.addError("Usage: /dm <address-or-name>")
		return CmdAction{}
	}
	target := args[0]
	addr := h.resolveTarget(target)
	if addr == "" {
		h.addError(fmt.Sprintf("Unknown peer: %s", target))
		return CmdAction{}
	}
	return CmdAction{SwitchBuffer: "dm:" + addr}
}

func (h *CommandHandler) cmdMsg(args []string) CmdAction {
	if len(args) < 2 {
		h.addError("Usage: /msg <addr|name> <text>")
		return CmdAction{}
	}
	target := args[0]
	addr := h.resolveTarget(target)
	if addr == "" {
		h.addError(fmt.Sprintf("Unknown peer: %s", target))
		return CmdAction{}
	}
	text := strings.TrimSpace(strings.Join(args[1:], " "))
	if text == "" {
		h.addError("Usage: /msg <addr|name> <text>")
		return CmdAction{}
	}
	return CmdAction{SendTo: addr, SendText: text}
}

func (h *CommandHandler) cmdCritical(args []string) CmdAction {
	if len(args) < 1 {
		h.addError("Usage: /critical <text>")
		return CmdAction{}
	}
	text := strings.TrimSpace(strings.Join(args, " "))
	if text == "" {
		h.addError("Usage: /critical <text>")
		return CmdAction{}
	}
	return CmdAction{SendText: text, SendCritical: true}
}

func (h *CommandHandler) cmdChannel(args []string) CmdAction {
	if len(args) < 1 {
		h.addError("Usage: /ch <buffer#|all|mesh:N>")
		return CmdAction{}
	}
	arg := strings.TrimSpace(strings.ToLower(args[0]))
	if arg == "all" || arg == "broadcast" || arg == "b" {
		return CmdAction{SwitchBuffer: "broadcast"}
	}

	// Human-friendly: /ch 2 means second visible buffer in statusline.
	if n, err := strconv.Atoi(arg); err == nil {
		convs := h.store.GetConversations()
		if n < 1 || n > len(convs) {
			h.addError(fmt.Sprintf("Buffer %d not found. Use /w to list buffers.", n))
			return CmdAction{}
		}
		return CmdAction{SwitchBuffer: convs[n-1].ID}
	}

	// Explicit mesh channel selector: /ch mesh:1 or /ch ch:1
	if strings.HasPrefix(arg, "mesh:") || strings.HasPrefix(arg, "ch:") {
		idxStr := strings.TrimPrefix(strings.TrimPrefix(arg, "mesh:"), "ch:")
		n, err := strconv.Atoi(idxStr)
		if err != nil {
			h.addError(fmt.Sprintf("Invalid mesh channel: %s", args[0]))
			return CmdAction{}
		}
		return CmdAction{SwitchBuffer: fmt.Sprintf("ch:%d", n)}
	}

	h.addError(fmt.Sprintf("Unknown channel selector %q. Use /w, /ch <buffer#>, /ch all, or /ch mesh:N", args[0]))
	return CmdAction{}
}

func (h *CommandHandler) cmdWindows() {
	convs := h.store.GetConversations()
	if len(convs) == 0 {
		h.addInfo("  No open buffers")
		return
	}
	h.addInfo("  Open buffers:")
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
		h.addInfo(fmt.Sprintf("  %s%d: %s%s", marker, i+1, c.Label, unread))
	}
}

func (h *CommandHandler) cmdClose() CmdAction {
	active := h.store.ActiveConvID
	if active == "broadcast" {
		h.addError("Can't close broadcast buffer")
		return CmdAction{}
	}
	h.addSystem(fmt.Sprintf("Closed buffer: %s", active))
	return CmdAction{SwitchBuffer: "broadcast"}
}

func (h *CommandHandler) cmdNodes() {
	h.store.mu.RLock()
	neighbors := h.store.Neighbors
	routes := h.store.Routes
	h.store.mu.RUnlock()

	h.addInfo(fmt.Sprintf("  Neighbors (%d):", len(neighbors)))
	if len(neighbors) == 0 {
		h.addInfo("    (none)")
	}
	for _, n := range neighbors {
		name := n.Address
		if h.resolver != nil {
			name = h.resolver.Resolve(n.Address)
		}
		ago := fmtDurationShort(time.Duration(n.LastSeenAgoMs) * time.Millisecond)
		h.addInfo(fmt.Sprintf("    %-12s  %-12s  %4ddBm  SNR %.1f  %s",
			n.Address, name, n.RSSI, n.SNR, ago))
	}

	h.addInfo(fmt.Sprintf("  Routes (%d):", len(routes)))
	if len(routes) == 0 {
		h.addInfo("    (none)")
	}
	for _, r := range routes {
		h.addInfo(fmt.Sprintf("    %-12s  via %-12s  %d hops  metric %d  %s",
			r.Dest, r.NextHop, r.HopCount, r.Metric, r.State))
	}
}

func (h *CommandHandler) cmdStats() {
	h.store.mu.RLock()
	status := h.store.Status
	airtime := h.store.Airtime
	h.store.mu.RUnlock()

	if status == nil {
		h.addError("No status data available yet")
		return
	}

	h.addInfo("  Node Status:")
	h.addInfo(fmt.Sprintf("    Uptime: %s", fmtDurationShort(time.Duration(status.UptimeSec)*time.Second)))
	h.addInfo(fmt.Sprintf("    Messages: TX %d  RX %d",
		status.PacketsTx, status.PacketsRx))
	h.addInfo(fmt.Sprintf("    Beacons:  TX %d  RX %d",
		status.BeaconTx, status.BeaconRx))

	if airtime != nil && len(airtime.Tiers) > 0 {
		h.addInfo("  Airtime:")
		for _, t := range airtime.Tiers {
			pct := 0
			if t.MaxMs > 0 {
				pct = t.RemainingMs * 100 / t.MaxMs
			}
			h.addInfo(fmt.Sprintf("    %-12s  %d%% remaining", t.Name, pct))
		}
	}
}

func (h *CommandHandler) cmdConfig(args []string) {
	if len(args) >= 3 && args[0] == "set" {
		h.cmdConfigSet(args[1], strings.Join(args[2:], " "))
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg, err := h.client.Config(ctx)
	if err != nil {
		h.addError(fmt.Sprintf("Config fetch error: %v", err))
		return
	}

	h.addInfo("  Identity:")
	h.addInfo(fmt.Sprintf("    Address:    %s", cfg.Address))
	h.addInfo(fmt.Sprintf("    Name:       %s", cfg.NodeName))

	h.addInfo("  Radio:")
	h.addInfo(fmt.Sprintf("    Frequency:  %d MHz", cfg.Radio.FrequencyMhz))
	h.addInfo(fmt.Sprintf("    SF:         %d", cfg.Radio.SF))
	h.addInfo(fmt.Sprintf("    Bandwidth:  %d Hz", cfg.Radio.BwHz))
	h.addInfo(fmt.Sprintf("    TX Power:   %d dBm", cfg.Radio.TxPowerDbm))

	if len(cfg.Channels) > 0 {
		h.addInfo("  Channels:")
		for i, ch := range cfg.Channels {
			def := ""
			if ch.IsDefault {
				def = " ★"
			}
			h.addInfo(fmt.Sprintf("    [%d] %s%s", i, ch.Name, def))
		}
	}
}

func (h *CommandHandler) cmdConfigSet(key, value string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	switch strings.ToLower(key) {
	case "name":
		if len(value) > 8 {
			h.addError("Name max 8 characters")
			return
		}
		err := h.client.SetNodeName(ctx, value)
		if err != nil {
			h.addError(fmt.Sprintf("Error: %v", err))
		} else {
			h.addSystem(fmt.Sprintf("Node name set to %q", value))
		}
	default:
		h.addError(fmt.Sprintf("Unknown config key: %s (settable: name)", key))
	}
}

func (h *CommandHandler) cmdLocation() {
	h.store.mu.RLock()
	gps := h.store.OwnGPS
	peers := h.store.PeerLocations
	h.store.mu.RUnlock()

	if gps != nil && gps.Valid {
		h.addInfo(fmt.Sprintf("  My GPS: %.6f, %.6f  Alt %dm  Sats %d",
			gps.Lat, gps.Lon, gps.AltM, gps.Sats))
		h.addInfo("  " + openStreetMapURL(gps.Lat, gps.Lon))
	} else {
		h.addInfo("  My GPS: no fix")
	}

	if len(peers) == 0 {
		h.addInfo("  No peer locations")
		return
	}
	h.addInfo(fmt.Sprintf("  Peer Locations (%d):", len(peers)))
	for _, p := range peers {
		name := p.Addr
		if h.resolver != nil {
			name = h.resolver.Resolve(p.Addr)
		}
		pos := ""
		if p.Position != nil {
			pos = fmt.Sprintf("%.6f, %.6f", p.Position.Lat, p.Position.Lon)
		}
		h.addInfo(fmt.Sprintf("    %-12s  %s", name, pos))
		if p.Position != nil {
			h.addInfo("    " + openStreetMapURL(p.Position.Lat, p.Position.Lon))
		}
	}
}

func openStreetMapURL(lat, lon float64) string {
	return fmt.Sprintf("https://www.openstreetmap.org/?mlat=%.6f&mlon=%.6f#map=17/%.6f/%.6f", lat, lon, lat, lon)
}

func (h *CommandHandler) cmdAlias(args []string) {
	if len(args) < 2 {
		h.addError("Usage: /alias <address> <name>")
		return
	}
	addr := args[0]
	name := strings.Join(args[1:], " ")
	if h.resolver != nil {
		err := h.resolver.SetAlias(addr, name)
		if err != nil {
			h.addError(fmt.Sprintf("Error: %v", err))
			return
		}
	}
	h.addSystem(fmt.Sprintf("Alias set: %s → %s", addr, name))
}

func (h *CommandHandler) cmdNick(args []string) {
	if len(args) < 1 {
		h.addError("Usage: /nick <name> (max 8 chars)")
		return
	}
	name := strings.Join(args, " ")
	if len(name) > 8 {
		h.addError("Nick max 8 characters")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := h.client.SetNodeName(ctx, name)
	if err != nil {
		h.addError(fmt.Sprintf("Nick error: %v", err))
		return
	}
	h.addSystem(fmt.Sprintf("Nick changed to %q", name))
}

func (h *CommandHandler) cmdMe(args []string) CmdAction {
	if len(args) < 1 {
		h.addError("Usage: /me <action> (e.g. /me waves hello)")
		return CmdAction{}
	}
	return h.actionCmd(strings.Join(args, " "))
}

func (h *CommandHandler) cmdSlap(args []string) CmdAction {
	if len(args) < 1 {
		h.addError("Usage: /slap <target>")
		return CmdAction{}
	}
	return h.actionCmd(fmt.Sprintf("slaps %s around a bit with a large trout", strings.Join(args, " ")))
}

func (h *CommandHandler) actionCmd(text string) CmdAction {
	// Wrap in CTCP ACTION format: ACTION text
	actionText := "ACTION " + text + ""
	return CmdAction{SendText: actionText}
}

func (h *CommandHandler) cmdProbe() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := h.client.SendProbe(ctx)
	if err != nil {
		h.addError(fmt.Sprintf("Probe error: %v", err))
		return
	}
	h.addSystem("Network probe sent — results will appear as they arrive")
}

func (h *CommandHandler) cmdPing() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	err := h.client.Ping(ctx)
	if err != nil {
		h.addError(fmt.Sprintf("Ping failed: %v", err))
		return
	}
	h.addInfo(fmt.Sprintf("  Pong: %dms", time.Since(start).Milliseconds()))
}

// DoReboot performs the actual reboot (called by root model after confirmation).
func (h *CommandHandler) DoReboot() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.client.Reboot(ctx); err != nil {
		h.addError(fmt.Sprintf("Reboot error: %v", err))
	} else {
		h.addSystem("Rebooting node...")
	}
}

func (h *CommandHandler) cmdHelp() {
	h.addInfo("  Commands:")
	h.addInfo("    /b, /broadcast        Switch to broadcast")
	h.addInfo("    /dm <addr|name>       Open/switch to DM")
	h.addInfo("    /msg <addr|name> <text> Send DM inline (no switch)")
	h.addInfo("    /critical <text>      Send as critical priority (more retries; emergency airtime tier)")
	h.addInfo("    /ch <sel>             Switch buffer (/ch 2, /ch all, /ch mesh:1)")
	h.addInfo("    /w, /windows          List open buffers")
	h.addInfo("    /close                Close current buffer")
	h.addInfo("    /nodes                Show neighbors & routes")
	h.addInfo("    /stats                Show node statistics")
	h.addInfo("    /config               Show configuration")
	h.addInfo("    /config set <k> <v>   Set config value")
	h.addInfo("    /location             Show GPS & peer locations")
	h.addInfo("    /alias <addr> <name>  Set peer alias")
	h.addInfo("    /nick <name>          Change node name (max 8)")
	h.addInfo("    /me <action>          Send action (* Nick does something)")
	h.addInfo("    /slap <target>        mIRC trout slap action")
	h.addInfo("    /probe                Send network probe")
	h.addInfo("    /ping                 Ping node")
	h.addInfo("    /reboot               Reboot node")
	h.addInfo("    /clear                Clear scrollback")
	h.addInfo("    /help                 This help")
	h.addInfo("    /quit                 Exit")
	h.addInfo("")
	h.addInfo("  Navigation:")
	h.addInfo("    Alt+1-9               Switch buffer by number")
	h.addInfo("    Ctrl+N / Ctrl+P       Next / prev buffer")
	h.addInfo("    PgUp / PgDn           Scroll history")
	h.addInfo("    Home / End            Top / bottom of history")
}

// resolveTarget resolves a name or hex address to a canonical address string.
func (h *CommandHandler) resolveTarget(target string) string {
	// If it looks like a hex address (8 hex chars), use directly.
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
	// Try reverse-resolve from name.
	if h.resolver != nil {
		if addr, ok := h.resolver.ReverseLookup(target); ok {
			return addr
		}
	}
	return ""
}

// fmtDurationShort formats a duration as a compact human-readable string.
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
