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

// CommandHandler executes commands and writes output to the scrollback.
type CommandHandler struct {
	client   *bramble.Client
	store    *Store
	scroll   *Scrollback
	resolver tabs.PeerResolver
	// Callbacks into the root model.
	onSwitchBuffer func(convID string)
	onQuit         func()
	onConfirm      func(prompt string, action func())
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
	target := args[0]
	addr := h.resolveTarget(target)
	if addr == "" {
		h.scroll.AddError(fmt.Sprintf("Unknown peer: %s", target))
		return
	}
	h.cmdSwitchBuffer("dm:" + addr)
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
	h.cmdSwitchBuffer(fmt.Sprintf("ch:%d", n))
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
	h.scroll.AddInfo(fmt.Sprintf("    Uptime: %s", fmtDurationShort(time.Duration(status.UptimeSec)*time.Second)))
	h.scroll.AddInfo(fmt.Sprintf("    Messages: TX %d  RX %d",
		status.PacketsTx, status.PacketsRx))
	h.scroll.AddInfo(fmt.Sprintf("    Beacons:  TX %d  RX %d",
		status.BeaconTx, status.BeaconRx))

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
	h.scroll.AddInfo(fmt.Sprintf("    Frequency:  %d MHz", cfg.Radio.FrequencyMhz))
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

	if gps != nil && gps.Valid {
		h.scroll.AddInfo(fmt.Sprintf("  My GPS: %.6f, %.6f  Alt %dm  Sats %d",
			gps.Lat, gps.Lon, gps.AltM, gps.Sats))
	} else {
		h.scroll.AddInfo("  My GPS: no fix")
	}

	if len(peers) == 0 {
		h.scroll.AddInfo("  No peer locations")
		return
	}
	h.scroll.AddInfo(fmt.Sprintf("  Peer Locations (%d):", len(peers)))
	for _, p := range peers {
		name := p.Addr
		if h.resolver != nil {
			name = h.resolver.Resolve(p.Addr)
		}
		pos := ""
		if p.Position != nil {
			pos = fmt.Sprintf("%.6f, %.6f", p.Position.Lat, p.Position.Lon)
		}
		h.scroll.AddInfo(fmt.Sprintf("    %-12s  %s", name, pos))
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
		h.onConfirm("Reboot node? Confirm with /reboot-confirm", func() {
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
	// No confirm callback; execute directly.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := h.client.Reboot(ctx); err != nil {
		h.scroll.AddError(fmt.Sprintf("Reboot error: %v", err))
	} else {
		h.scroll.AddSystem("Rebooting node...")
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
