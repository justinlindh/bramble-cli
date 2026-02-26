// Package tabs contains Bubble Tea tab submodels for the Bramble TUI.
package tabs

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	bramble "github.com/justinlindh/bramble-go"
)

// StatsDataMsg is sent when new Stats data has been fetched.
type StatsDataMsg struct {
	Status   bramble.StatusResponse
	Identity bramble.IdentityResponse
	Airtime  bramble.AirtimeStats
	FetchErr error
}

// StatsTrafficEventMsg is forwarded from the bridge for live traffic events.
type StatsTrafficEventMsg struct{ Event bramble.TrafficEvent }

// StatsProbeResultMsg carries a single probe response.
type StatsProbeResultMsg struct{ Result bramble.ProbeResult }

// StatsProbeCompleteMsg signals the probe window has closed.
type StatsProbeCompleteMsg struct{}

// sendProbeCmd fires a probe via the client.
func sendProbeCmd(client *bramble.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := client.SendProbe(ctx)
		if err != nil {
			return StatsDataMsg{FetchErr: fmt.Errorf("sendProbe: %w", err)}
		}
		return nil
	}
}

// setTrafficDebugCmd enables or disables traffic debug.
func setTrafficDebugCmd(client *bramble.Client, enable bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = client.SetTrafficDebug(ctx, bramble.SetTrafficDebugParams{Enabled: &enable})
		return nil
	}
}

// statsTheme holds the styles used by the Stats tab.
type statsTheme struct {
	panel        lipgloss.Style
	panelTitle   lipgloss.Style
	label        lipgloss.Style
	value        lipgloss.Style
	deltaUp      lipgloss.Style
	deltaDown    lipgloss.Style
	barGreen     lipgloss.Style
	barYellow    lipgloss.Style
	barRed       lipgloss.Style
	barEmpty     lipgloss.Style
	sectionTitle lipgloss.Style
	errStyle     lipgloss.Style
	trafficBeacon  lipgloss.Style
	trafficMessage lipgloss.Style
	trafficAck     lipgloss.Style
	trafficControl lipgloss.Style
	probeHeader    lipgloss.Style
}

func newStatsTheme() statsTheme {
	return statsTheme{
		panel: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#333355")).
			Padding(0, 1),
		panelTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00FF87")),
		label: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")),
		value: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ccccdd")),
		deltaUp: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF87")),
		deltaDown: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555")),
		barGreen: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF87")),
		barYellow: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFCC00")),
		barRed: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555")),
		barEmpty: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#333355")),
		sectionTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#aaaacc")),
		errStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555")),
		trafficBeacon: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#5599FF")),
		trafficMessage: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF87")),
		trafficAck: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFCC00")),
		trafficControl: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")),
		probeHeader: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#aaddff")),
	}
}

// trafficEntry holds a single traffic event for display.
type trafficEntry struct {
	ts       time.Time
	category string
	isTx     bool
	size     int
	peer     string
}

// probeEntry holds a single probe result for display.
type probeEntry struct {
	address   string
	hops      int
	latencyMs int64
	rssi      int
}

const maxTrafficEntries = 200
const trafficViewHeight = 8

// StatsModel is the Bubble Tea model for the Stats tab.
type StatsModel struct {
	client   *bramble.Client
	theme    statsTheme
	width    int
	height   int
	scrollY  int
	loading  bool
	fetchErr error

	// current data
	status   bramble.StatusResponse
	identity bramble.IdentityResponse
	airtime  bramble.AirtimeStats

	// previous values for delta calculation
	prevPacketsTx int
	prevPacketsRx int
	hasPrev       bool

	// probe state
	probing      bool
	probeResults []probeEntry

	// traffic monitor
	trafficLog    []trafficEntry
	trafficScroll int
	trafficDebug  bool

	// which section has focus for scroll: "main" or "traffic"
	focus string
}

// NewStats creates a new StatsModel.
func NewStats(client *bramble.Client) StatsModel {
	return StatsModel{
		client:  client,
		theme:   newStatsTheme(),
		loading: true,
		focus:   "main",
	}
}

// RefreshCmd returns a Bubble Tea command that fetches Status, Identity, and Airtime from the node.
func (m StatsModel) RefreshCmd() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		status, err := client.Status(ctx)
		if err != nil {
			return StatsDataMsg{FetchErr: fmt.Errorf("getStatus: %w", err)}
		}

		identity, err := client.Identity(ctx)
		if err != nil {
			return StatsDataMsg{FetchErr: fmt.Errorf("getIdentity: %w", err)}
		}

		airtime, err := client.Airtime(ctx)
		if err != nil {
			return StatsDataMsg{FetchErr: fmt.Errorf("getAirtime: %w", err)}
		}

		return StatsDataMsg{
			Status:   *status,
			Identity: *identity,
			Airtime:  *airtime,
		}
	}
}

// Init implements tea.Model.
func (m StatsModel) Init() tea.Cmd {
	return m.RefreshCmd()
}

// Update implements tea.Model.
func (m StatsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case StatsDataMsg:
		m.loading = false
		if msg.FetchErr != nil {
			m.fetchErr = msg.FetchErr
			return m, nil
		}
		m.fetchErr = nil
		// Save previous counters before updating
		if m.hasPrev {
			m.prevPacketsTx = m.status.PacketsTx
			m.prevPacketsRx = m.status.PacketsRx
		} else {
			m.prevPacketsTx = msg.Status.PacketsTx
			m.prevPacketsRx = msg.Status.PacketsRx
			m.hasPrev = true
		}
		m.status = msg.Status
		m.identity = msg.Identity
		m.airtime = msg.Airtime

	case StatsTrafficEventMsg:
		entry := trafficEntry{
			ts:       time.Now(),
			category: msg.Event.Category,
			isTx:     msg.Event.IsTx,
			size:     msg.Event.PacketLen,
			peer:     fmt.Sprintf("%04x", msg.Event.PktType), // best approximation; no peer addr in event
		}
		m.trafficLog = append(m.trafficLog, entry)
		if len(m.trafficLog) > maxTrafficEntries {
			m.trafficLog = m.trafficLog[len(m.trafficLog)-maxTrafficEntries:]
		}
		// Auto-scroll to bottom if not manually scrolled up
		maxScroll := len(m.trafficLog) - trafficViewHeight
		if maxScroll < 0 {
			maxScroll = 0
		}
		if m.trafficScroll >= maxScroll-1 {
			m.trafficScroll = maxScroll
		}

	case StatsProbeResultMsg:
		m.probeResults = append(m.probeResults, probeEntry{
			address:   msg.Result.Address,
			hops:      msg.Result.Hops,
			latencyMs: msg.Result.LatencyMs,
			rssi:      msg.Result.RSSI,
		})

	case StatsProbeCompleteMsg:
		m.probing = false

	case tea.KeyPressMsg:
		switch msg.String() {
		case "r":
			m.loading = true
			return m, m.RefreshCmd()
		case "p":
			// Send probe
			m.probing = true
			m.probeResults = nil
			return m, sendProbeCmd(m.client)
		case "t":
			// Toggle traffic debug
			m.trafficDebug = !m.trafficDebug
			return m, setTrafficDebugCmd(m.client, m.trafficDebug)
		case "tab":
			// Toggle focus between main and traffic
			if m.focus == "main" {
				m.focus = "traffic"
			} else {
				m.focus = "main"
			}
		case "up", "k":
			if m.focus == "traffic" {
				if m.trafficScroll > 0 {
					m.trafficScroll--
				}
			} else {
				if m.scrollY > 0 {
					m.scrollY--
				}
			}
		case "down", "j":
			if m.focus == "traffic" {
				maxScroll := len(m.trafficLog) - trafficViewHeight
				if maxScroll < 0 {
					maxScroll = 0
				}
				if m.trafficScroll < maxScroll {
					m.trafficScroll++
				}
			} else {
				m.scrollY++
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

// SetSize updates the terminal dimensions.
func (m *StatsModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// UpdateData applies new fetched data (called from root model on tick).
func (m *StatsModel) UpdateData(data StatsDataMsg) {
	if data.FetchErr != nil {
		m.fetchErr = data.FetchErr
		return
	}
	m.fetchErr = nil
	if m.hasPrev {
		m.prevPacketsTx = m.status.PacketsTx
		m.prevPacketsRx = m.status.PacketsRx
	} else {
		m.prevPacketsTx = data.Status.PacketsTx
		m.prevPacketsRx = data.Status.PacketsRx
		m.hasPrev = true
	}
	m.loading = false
	m.status = data.Status
	m.identity = data.Identity
	m.airtime = data.Airtime
}

// View implements tea.Model.
func (m StatsModel) View() tea.View {
	return tea.NewView(m.Render())
}

// Render returns the rendered string for the Stats tab content area.
func (m StatsModel) Render() string {
	if m.loading {
		return "\n  Loading stats..."
	}
	if m.fetchErr != nil {
		return "\n  " + m.theme.errStyle.Render("Error: "+m.fetchErr.Error())
	}

	var lines []string

	// ── Counter Grid ─────────────────────────────────────────────────
	lines = append(lines, m.theme.sectionTitle.Render("  Packet Counters"))
	lines = append(lines, m.renderCounterGrid())
	lines = append(lines, "")

	// ── Airtime Budget ────────────────────────────────────────────────
	if len(m.airtime.Tiers) > 0 {
		lines = append(lines, m.theme.sectionTitle.Render("  Airtime Budget"))
		lines = append(lines, m.renderAirtimeBars())
		lines = append(lines, "")
	}

	// ── Network Reach ─────────────────────────────────────────────────
	lines = append(lines, m.theme.sectionTitle.Render("  Network Reach"))
	lines = append(lines, m.renderNetworkReach())
	lines = append(lines, "")

	// ── System Info ──────────────────────────────────────────────────
	lines = append(lines, m.theme.sectionTitle.Render("  System Info"))
	lines = append(lines, m.renderSystemInfo())
	lines = append(lines, "")

	// ── Traffic Monitor ───────────────────────────────────────────────
	trafficFocus := ""
	if m.focus == "traffic" {
		trafficFocus = " [focused]"
	}
	debugState := ""
	if m.trafficDebug {
		debugState = " [debug ON]"
	}
	lines = append(lines, m.theme.sectionTitle.Render("  Traffic Monitor"+trafficFocus+debugState))
	lines = append(lines, m.renderTrafficLog())

	content := strings.Join(lines, "\n")

	// Apply scroll offset (main content only)
	contentLines := strings.Split(content, "\n")
	maxScroll := len(contentLines) - m.height + 4
	if maxScroll < 0 {
		maxScroll = 0
	}
	start := m.scrollY
	if start > maxScroll {
		start = maxScroll
	}
	if start > len(contentLines) {
		start = len(contentLines)
	}
	visible := contentLines[start:]
	maxVisible := m.height - 4
	if maxVisible < 1 {
		maxVisible = 1
	}
	if len(visible) > maxVisible {
		visible = visible[:maxVisible]
	}

	return strings.Join(visible, "\n")
}

// renderNetworkReach renders the network reach section with probe results.
func (m StatsModel) renderNetworkReach() string {
	t := m.theme
	var sb strings.Builder

	// Reach summary from status
	direct := m.status.Peers
	// We don't have routed count from status directly; show what we have
	sb.WriteString(fmt.Sprintf("  %s  %s\n",
		t.label.Render(fmt.Sprintf("%-12s", "Direct")),
		t.value.Render(fmt.Sprintf("%d neighbors", direct)),
	))

	// Probe state / results
	if m.probing {
		sb.WriteString("  " + t.probeHeader.Render("Probing...") + "\n")
		if len(m.probeResults) > 0 {
			sb.WriteString(m.renderProbeResults())
		}
	} else if len(m.probeResults) > 0 {
		sb.WriteString("  " + t.probeHeader.Render(fmt.Sprintf("Probe results (%d nodes):", len(m.probeResults))) + "\n")
		sb.WriteString(m.renderProbeResults())
	} else {
		sb.WriteString("  " + t.label.Render("[p] to send network probe") + "\n")
	}

	return sb.String()
}

// renderProbeResults renders the probe result table.
func (m StatsModel) renderProbeResults() string {
	t := m.theme
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("  %s\n", t.label.Render(fmt.Sprintf("  %-12s  %-5s  %-10s  %-6s", "Address", "Hops", "Latency", "RSSI"))))
	for _, r := range m.probeResults {
		sb.WriteString(fmt.Sprintf("  %s\n",
			t.value.Render(fmt.Sprintf("  %-12s  %-5d  %-10s  %d dBm",
				r.address,
				r.hops,
				fmt.Sprintf("%dms", r.latencyMs),
				r.rssi,
			)),
		))
	}
	return sb.String()
}

// renderTrafficLog renders the scrollable traffic event log.
func (m StatsModel) renderTrafficLog() string {
	t := m.theme
	if len(m.trafficLog) == 0 {
		return "  " + t.label.Render("No traffic events. [t] to enable debug.") + "\n"
	}

	// Determine visible slice
	start := m.trafficScroll
	end := start + trafficViewHeight
	if end > len(m.trafficLog) {
		end = len(m.trafficLog)
	}
	visible := m.trafficLog[start:end]

	var sb strings.Builder
	for _, e := range visible {
		dir := "RX"
		if e.isTx {
			dir = "TX"
		}
		ts := e.ts.Format("15:04:05")
		line := fmt.Sprintf("  %s  %-3s  %-12s  %4dB", ts, dir, e.category, e.size)
		colored := m.colorTrafficLine(e.category, line)
		sb.WriteString(colored + "\n")
	}

	// Scroll indicator
	total := len(m.trafficLog)
	if total > trafficViewHeight {
		sb.WriteString(fmt.Sprintf("  %s\n",
			t.label.Render(fmt.Sprintf("Showing %d–%d of %d events  [↑↓] scroll", start+1, end, total)),
		))
	}

	return sb.String()
}

// colorTrafficLine applies color based on traffic category.
func (m StatsModel) colorTrafficLine(category, line string) string {
	t := m.theme
	switch category {
	case "beacon", "timesync":
		return t.trafficBeacon.Render(line)
	case "chat":
		return t.trafficMessage.Render(line)
	case "ack":
		return t.trafficAck.Render(line)
	default:
		return t.trafficControl.Render(line)
	}
}

// renderCounterGrid renders TX and RX packet counters side by side.
func (m StatsModel) renderCounterGrid() string {
	t := m.theme

	txDelta := ""
	rxDelta := ""
	if m.hasPrev {
		dtx := m.status.PacketsTx - m.prevPacketsTx
		drx := m.status.PacketsRx - m.prevPacketsRx
		if dtx > 0 {
			txDelta = " " + t.deltaUp.Render(fmt.Sprintf("▲%d", dtx))
		} else if dtx < 0 {
			txDelta = " " + t.deltaDown.Render(fmt.Sprintf("▼%d", -dtx))
		}
		if drx > 0 {
			rxDelta = " " + t.deltaUp.Render(fmt.Sprintf("▲%d", drx))
		} else if drx < 0 {
			rxDelta = " " + t.deltaDown.Render(fmt.Sprintf("▼%d", -drx))
		}
	}

	txTitle := t.panelTitle.Render("TX Packets")
	txVal := t.value.Render(fmt.Sprintf("%d", m.status.PacketsTx)) + txDelta

	rxTitle := t.panelTitle.Render("RX Packets")
	rxVal := t.value.Render(fmt.Sprintf("%d", m.status.PacketsRx)) + rxDelta

	panelW := 22
	txPanel := t.panel.Width(panelW).Render(txTitle + "\n" + txVal)
	rxPanel := t.panel.Width(panelW).Render(rxTitle + "\n" + rxVal)

	// Join panels horizontally
	txLines := strings.Split(txPanel, "\n")
	rxLines := strings.Split(rxPanel, "\n")
	maxLen := len(txLines)
	if len(rxLines) > maxLen {
		maxLen = len(rxLines)
	}
	var sb strings.Builder
	for i := 0; i < maxLen; i++ {
		var tl, rl string
		if i < len(txLines) {
			tl = txLines[i]
		}
		if i < len(rxLines) {
			rl = rxLines[i]
		}
		sb.WriteString("  " + tl + "  " + rl + "\n")
	}
	return sb.String()
}

// renderAirtimeBars renders a horizontal progress bar for each airtime tier.
func (m StatsModel) renderAirtimeBars() string {
	t := m.theme
	barWidth := 30

	var sb strings.Builder
	for _, tier := range m.airtime.Tiers {
		usedMs := tier.MaxMs - tier.RemainingMs
		if usedMs < 0 {
			usedMs = 0
		}
		pct := 0
		if tier.MaxMs > 0 {
			pct = usedMs * 100 / tier.MaxMs
		}

		// Choose color
		var filledStyle lipgloss.Style
		switch {
		case pct >= 80:
			filledStyle = t.barRed
		case pct >= 50:
			filledStyle = t.barYellow
		default:
			filledStyle = t.barGreen
		}

		filled := barWidth * pct / 100
		if filled > barWidth {
			filled = barWidth
		}
		empty := barWidth - filled

		bar := filledStyle.Render(strings.Repeat("█", filled)) +
			t.barEmpty.Render(strings.Repeat("░", empty))

		name := tier.Name
		if utf8.RuneCountInString(name) > 10 {
			runes := []rune(name)
			name = string(runes[:10])
		}

		line := fmt.Sprintf("  %-10s %s %3d%%  %d/%d ms\n",
			name, bar, pct, usedMs, tier.MaxMs)
		sb.WriteString(line)
	}
	return sb.String()
}

// renderSystemInfo renders the system info key-value panel.
func (m StatsModel) renderSystemInfo() string {
	t := m.theme

	uptime := formatUptime(m.status.UptimeSec)

	addr := m.status.Address
	if addr == "" {
		addr = m.identity.Address
	}
	pubkey := m.identity.PubkeyHash
	if pubkey == "" {
		pubkey = "—"
	}

	rows := []struct{ label, value string }{
		{"Uptime", uptime},
		{"Firmware", m.status.FirmwareVersion},
		{"Protocol", m.status.ProtocolVersion},
		{"Hardware", m.status.Hardware},
		{"Address", addr},
		{"Pubkey Hash", pubkey},
		{"Radio", boolStatus(m.status.RadioOk)},
		{"Peers", fmt.Sprintf("%d", m.status.Peers)},
	}

	var sb strings.Builder
	for _, row := range rows {
		sb.WriteString(fmt.Sprintf("  %s  %s\n",
			t.label.Render(fmt.Sprintf("%-12s", row.label)),
			t.value.Render(row.value),
		))
	}
	return sb.String()
}

// formatUptime converts seconds into "Xd Xh Xm" format.
func formatUptime(sec int) string {
	if sec <= 0 {
		return "—"
	}
	d := sec / 86400
	sec %= 86400
	h := sec / 3600
	sec %= 3600
	m := sec / 60
	if d > 0 {
		return fmt.Sprintf("%dd %dh %dm", d, h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func boolStatus(ok bool) string {
	if ok {
		return "OK"
	}
	return "ERROR"
}
