// Package tabs contains Bubble Tea tab submodels for the Bramble TUI.
package tabs

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	bramble "github.com/justinlindh/bramble-go"
)

// ── Messages ─────────────────────────────────────────────────────────────────

// ConfigDataMsg is sent when config data has been fetched.
type ConfigDataMsg struct {
	Config *bramble.ConfigResponse
	Err    error
}

// configAddChannelMsg is sent after AddChannel completes.
type configAddChannelMsg struct {
	result *bramble.AddChannelResult
	err    error
}

// configRemoveChannelMsg is sent after RemoveChannel completes.
type configRemoveChannelMsg struct{ err error }

// configSetDefaultChannelMsg is sent after SetDefaultChannel completes.
type configSetDefaultChannelMsg struct{ err error }

// configSaveNameMsg is sent after SetNodeName completes.
type configSaveNameMsg struct{ err error }

// configSaveMailboxMsg is sent after SetMailbox completes.
type configSaveMailboxMsg struct{ err error }

// configSaveRadioMsg is sent after SetRadio completes.
type configSaveRadioMsg struct{ err error }

// configRebootMsg is sent after Reboot completes.
type configRebootMsg struct{ err error }

// configTrafficDebugMsg is sent after GetTrafficDebug completes.
type configTrafficDebugMsg struct {
	resp *bramble.GetTrafficDebugResponse
	err  error
}

// ── Sections / Fields ─────────────────────────────────────────────────────────

const (
	configSectionIdentity = 0
	configSectionRadio    = 1
	configSectionActions  = 2
	configSectionChannels = 3
	configSectionCount    = 4
)

// Field indices within Identity section
const (
	identFieldAddress  = 0
	identFieldPubkey   = 1
	identFieldName     = 2
	identFieldMailbox  = 3
	identFieldCount    = 4
)

// Field indices within Radio section
const (
	radioFieldFreq    = 0
	radioFieldSF      = 1
	radioFieldBW      = 2
	radioFieldTxPower = 3
	radioFieldSave    = 4
	radioFieldCount   = 5
)

// Field indices within Actions section
const (
	actionFieldReboot        = 0
	actionFieldClearHistory  = 1
	actionFieldTrafficDebug  = 2
	actionFieldCount         = 3
)

// ── Theme ─────────────────────────────────────────────────────────────────────

type configTheme struct {
	sectionHeader lipgloss.Style
	label         lipgloss.Style
	value         lipgloss.Style
	editValue     lipgloss.Style
	selectedRow   lipgloss.Style
	mutedValue    lipgloss.Style
	successMsg    lipgloss.Style
	errorMsg      lipgloss.Style
	confirmMsg    lipgloss.Style
	toggleOn      lipgloss.Style
	toggleOff     lipgloss.Style
	saveButton    lipgloss.Style
}

func newConfigTheme() configTheme {
	return configTheme{
		sectionHeader: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00FF87")).
			MarginTop(1),
		label: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Width(16),
		value: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ccccdd")),
		editValue: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFCC00")).
			Underline(true),
		selectedRow: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ffffff")).
			Background(lipgloss.Color("#1a1a3a")),
		mutedValue: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#555566")),
		successMsg: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF87")),
		errorMsg: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555")),
		confirmMsg: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFAA00")).
			Bold(true),
		toggleOn: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF87")).
			Bold(true),
		toggleOff: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#555566")),
		saveButton: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#0d0d1a")).
			Background(lipgloss.Color("#00FF87")).
			Bold(true).
			Padding(0, 2),
	}
}

// ── ConfigModel ───────────────────────────────────────────────────────────────

// ConfigModel is the Config tab submodel.
type ConfigModel struct {
	client *bramble.Client
	theme  configTheme

	// Current data
	config  *bramble.ConfigResponse
	mailbox bool // current mailbox state

	// Navigation
	activeSection int
	sectionCursor [configSectionCount]int

	// Edit state
	editing     bool
	editBuffer  string

	// Confirm dialog
	confirmPending string // "reboot" | "clearhistory" | "radio"
	confirmInput   string

	// Radio edit fields (strings while editing)
	radioEdits [radioFieldCount]string
	radioValid [radioFieldCount]bool

	// Status message
	statusMsg     string
	statusIsError bool
	statusExpiry  time.Time

	// Traffic debug state
	trafficDebug bool

	// Channel add dialog state
	// addChannelStep: 0=idle, 1=entering name, 2=entering psk
	addChannelStep int
	addChannelName string
	addChannelPsk  string

	width  int
	height int
}

// NewConfig creates a new ConfigModel.
func NewConfig(client *bramble.Client) ConfigModel {
	m := ConfigModel{
		client: client,
		theme:  newConfigTheme(),
	}
	for i := range m.radioValid {
		m.radioValid[i] = true
	}
	return m
}

// Init fetches initial config data.
func (m ConfigModel) Init() tea.Cmd {
	return tea.Batch(m.fetchConfig(), m.fetchTrafficDebug())
}

// SetSize updates the display dimensions.
func (m *ConfigModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

// ── Fetch Commands ─────────────────────────────────────────────────────────────

func (m ConfigModel) fetchConfig() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cfg, err := client.Config(ctx)
		return ConfigDataMsg{Config: cfg, Err: err}
	}
}

func (m ConfigModel) fetchTrafficDebug() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		resp, err := client.GetTrafficDebug(ctx)
		return configTrafficDebugMsg{resp: resp, err: err}
	}
}

func (m ConfigModel) saveName(name string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := client.SetNodeName(ctx, name)
		return configSaveNameMsg{err: err}
	}
}

func (m ConfigModel) saveMailbox(enabled bool) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := client.SetMailbox(ctx, enabled)
		return configSaveMailboxMsg{err: err}
	}
}

func (m ConfigModel) saveRadio(cfg bramble.RadioConfig) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := client.SetRadio(ctx, cfg)
		return configSaveRadioMsg{err: err}
	}
}

func (m ConfigModel) doReboot() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := client.Reboot(ctx)
		return configRebootMsg{err: err}
	}
}

func (m ConfigModel) setTrafficDebug(enabled bool) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		params := bramble.SetTrafficDebugParams{Enabled: &enabled}
		_, err := client.SetTrafficDebug(ctx, params)
		return configTrafficDebugMsg{err: err}
	}
}

func (m ConfigModel) doAddChannel(name, psk string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result, err := client.AddChannel(ctx, name, psk)
		return configAddChannelMsg{result: result, err: err}
	}
}

func (m ConfigModel) doRemoveChannel(index int) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := client.RemoveChannel(ctx, index)
		return configRemoveChannelMsg{err: err}
	}
}

func (m ConfigModel) doSetDefaultChannel(index int) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := client.SetDefaultChannel(ctx, index)
		return configSetDefaultChannelMsg{err: err}
	}
}

// ── Update ─────────────────────────────────────────────────────────────────────

// Update handles messages for the config tab.
func (m ConfigModel) Update(msg tea.Msg) (ConfigModel, tea.Cmd) {
	switch msg := msg.(type) {

	case ConfigDataMsg:
		if msg.Err == nil && msg.Config != nil {
			m.config = msg.Config
			m.mailbox = false // TODO: expose mailbox state from API when available
			m.populateRadioEdits()
		}
		return m, nil

	case configTrafficDebugMsg:
		if msg.err == nil && msg.resp != nil {
			m.trafficDebug = msg.resp.Enabled
		}
		return m, nil

	case configAddChannelMsg:
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("Add channel error: %v", msg.err), true)
		} else {
			m.setStatus("Channel added", false)
			return m, m.fetchConfig()
		}
		return m, nil

	case configRemoveChannelMsg:
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("Remove channel error: %v", msg.err), true)
		} else {
			m.setStatus("Channel removed", false)
			// reset cursor safely
			if m.config != nil && m.sectionCursor[configSectionChannels] >= len(m.config.Channels)-1 {
				m.sectionCursor[configSectionChannels] = 0
			}
			return m, m.fetchConfig()
		}
		return m, nil

	case configSetDefaultChannelMsg:
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("Set default error: %v", msg.err), true)
		} else {
			m.setStatus("Default channel updated", false)
			return m, m.fetchConfig()
		}
		return m, nil

	case configSaveNameMsg:
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("Error: %v", msg.err), true)
		} else {
			m.setStatus("Name saved", false)
			return m, m.fetchConfig()
		}
		return m, nil

	case configSaveMailboxMsg:
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("Error: %v", msg.err), true)
		} else {
			m.setStatus("Mailbox updated", false)
		}
		return m, nil

	case configSaveRadioMsg:
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("Error: %v", msg.err), true)
		} else {
			m.setStatus("Radio saved", false)
			return m, m.fetchConfig()
		}
		return m, nil

	case configRebootMsg:
		if msg.err != nil {
			m.setStatus(fmt.Sprintf("Reboot error: %v", msg.err), true)
		} else {
			m.setStatus("Rebooting…", false)
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m ConfigModel) handleKey(msg tea.KeyPressMsg) (ConfigModel, tea.Cmd) {
	// Add channel dialog active
	if m.addChannelStep > 0 {
		return m.handleAddChannelKey(msg)
	}

	// Confirm dialog active
	if m.confirmPending != "" {
		switch msg.String() {
		case "y", "Y":
			pending := m.confirmPending
			m.confirmPending = ""
			m.confirmInput = ""
			return m.executePending(pending)
		case "n", "N", "esc":
			m.confirmPending = ""
			m.confirmInput = ""
			m.setStatus("Cancelled", false)
		}
		return m, nil
	}

	// Edit mode active
	if m.editing {
		return m.handleEditKey(msg)
	}

	// Normal navigation
	switch msg.String() {
	case "j", "down":
		m.moveCursor(1)
	case "k", "up":
		m.moveCursor(-1)
	case "tab":
		m.activeSection = (m.activeSection + 1) % configSectionCount
	case "shift+tab":
		m.activeSection = (m.activeSection - 1 + configSectionCount) % configSectionCount
	case "enter", " ":
		return m.activateField()
	case "ctrl+r":
		if m.activeSection == configSectionActions {
			m.sectionCursor[configSectionActions] = actionFieldReboot
		}
		m.confirmPending = "reboot"
		m.setStatus("", false)
		return m, nil
	case "a":
		if m.activeSection == configSectionChannels {
			m.addChannelStep = 1
			m.addChannelName = ""
			m.addChannelPsk = ""
			return m, nil
		}
	case "d":
		if m.activeSection == configSectionChannels {
			if m.config != nil && len(m.config.Channels) > 0 {
				m.confirmPending = "deletechannel"
				return m, nil
			}
		}
	case "s":
		if m.activeSection == configSectionChannels {
			if m.config != nil && len(m.config.Channels) > 0 {
				idx := m.sectionCursor[configSectionChannels]
				return m, m.doSetDefaultChannel(idx)
			}
		}
	}
	return m, nil
}

func (m ConfigModel) handleAddChannelKey(msg tea.KeyPressMsg) (ConfigModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.addChannelStep = 0
		m.addChannelName = ""
		m.addChannelPsk = ""
		m.setStatus("Cancelled", false)
		return m, nil
	case "enter":
		if m.addChannelStep == 1 {
			name := strings.TrimSpace(m.addChannelName)
			if name == "" {
				m.setStatus("Name is required", true)
				return m, nil
			}
			m.addChannelName = name
			m.addChannelStep = 2
			return m, nil
		}
		// step 2: submit
		name := m.addChannelName
		psk := m.addChannelPsk
		m.addChannelStep = 0
		m.addChannelName = ""
		m.addChannelPsk = ""
		return m, m.doAddChannel(name, psk)
	case "backspace":
		if m.addChannelStep == 1 && len(m.addChannelName) > 0 {
			m.addChannelName = m.addChannelName[:len(m.addChannelName)-1]
		} else if m.addChannelStep == 2 && len(m.addChannelPsk) > 0 {
			m.addChannelPsk = m.addChannelPsk[:len(m.addChannelPsk)-1]
		}
	default:
		ch := msg.String()
		if len(ch) == 1 {
			if m.addChannelStep == 1 {
				m.addChannelName += ch
			} else if m.addChannelStep == 2 {
				m.addChannelPsk += ch
			}
		}
	}
	return m, nil
}

func (m ConfigModel) handleEditKey(msg tea.KeyPressMsg) (ConfigModel, tea.Cmd) {
	isNameField := m.activeSection == configSectionIdentity &&
		m.sectionCursor[configSectionIdentity] == identFieldName
	switch msg.String() {
	case "enter":
		return m.commitEdit()
	case "esc":
		m.editing = false
		m.editBuffer = ""
		// Restore original value
		m.populateRadioEdits()
	case "backspace":
		if len(m.editBuffer) > 0 {
			m.editBuffer = m.editBuffer[:len(m.editBuffer)-1]
		}
		if !isNameField {
			m.syncEditToField()
		}
	default:
		ch := msg.String()
		if len(ch) == 1 {
			if isNameField {
				m.editBuffer += ch
			} else if ch[0] >= '0' && ch[0] <= '9' || ch[0] == '.' || ch[0] == '-' {
				m.editBuffer += ch
				m.syncEditToField()
			}
		}
	}
	return m, nil
}

func (m *ConfigModel) syncEditToField() {
	if m.activeSection == configSectionIdentity && m.sectionCursor[configSectionIdentity] == identFieldName {
		// Name edit: any character
		return
	}
	if m.activeSection == configSectionRadio {
		m.radioEdits[m.sectionCursor[configSectionRadio]] = m.editBuffer
	}
}

func (m ConfigModel) activateField() (ConfigModel, tea.Cmd) {
	switch m.activeSection {
	case configSectionIdentity:
		switch m.sectionCursor[configSectionIdentity] {
		case identFieldName:
			if m.config != nil {
				m.editBuffer = m.config.NodeName
			}
			m.editing = true
		case identFieldMailbox:
			m.mailbox = !m.mailbox
			return m, m.saveMailbox(m.mailbox)
		}
	case configSectionRadio:
		cursor := m.sectionCursor[configSectionRadio]
		if cursor == radioFieldSave {
			// Validate and confirm
			if !m.allRadioValid() {
				m.setStatus("Fix validation errors before saving", true)
				return m, nil
			}
			m.confirmPending = "radio"
			return m, nil
		}
		// Enter edit mode for field
		m.editBuffer = m.radioEdits[cursor]
		m.editing = true
	case configSectionActions:
		switch m.sectionCursor[configSectionActions] {
		case actionFieldReboot:
			m.confirmPending = "reboot"
		case actionFieldClearHistory:
			m.confirmPending = "clearhistory"
		case actionFieldTrafficDebug:
			m.trafficDebug = !m.trafficDebug
			return m, m.setTrafficDebug(m.trafficDebug)
		}
	}
	return m, nil
}

func (m ConfigModel) commitEdit() (ConfigModel, tea.Cmd) {
	m.editing = false
	switch m.activeSection {
	case configSectionIdentity:
		if m.sectionCursor[configSectionIdentity] == identFieldName {
			name := strings.TrimSpace(m.editBuffer)
			m.editBuffer = ""
			if name == "" {
				m.setStatus("Name cannot be empty", true)
				return m, nil
			}
			if len(name) > 8 {
				m.setStatus("Name max 8 characters", true)
				return m, nil
			}
			return m, m.saveName(name)
		}
	case configSectionRadio:
		cursor := m.sectionCursor[configSectionRadio]
		m.radioEdits[cursor] = m.editBuffer
		m.radioValid[cursor] = m.validateRadioField(cursor, m.editBuffer)
		m.editBuffer = ""
	}
	return m, nil
}

func (m ConfigModel) executePending(pending string) (ConfigModel, tea.Cmd) {
	switch pending {
	case "reboot":
		return m, m.doReboot()
	case "clearhistory":
		// No SDK method for clear history yet; show info
		m.setStatus("Clear history not yet available via API", true)
	case "radio":
		cfg := m.buildRadioConfig()
		return m, m.saveRadio(cfg)
	case "deletechannel":
		if m.config != nil && len(m.config.Channels) > 0 {
			idx := m.sectionCursor[configSectionChannels]
			if idx < len(m.config.Channels) {
				return m, m.doRemoveChannel(idx)
			}
		}
	}
	return m, nil
}

func (m *ConfigModel) moveCursor(delta int) {
	max := m.sectionFieldCount() - 1
	cur := m.sectionCursor[m.activeSection] + delta
	if cur < 0 {
		// Move to previous section
		prev := m.activeSection - 1
		if prev < 0 {
			prev = configSectionCount - 1
		}
		m.activeSection = prev
		m.sectionCursor[m.activeSection] = m.sectionFieldCountFor(prev) - 1
		return
	}
	if cur > max {
		// Move to next section
		next := m.activeSection + 1
		if next >= configSectionCount {
			next = 0
		}
		m.activeSection = next
		m.sectionCursor[m.activeSection] = 0
		return
	}
	m.sectionCursor[m.activeSection] = cur
}

func (m ConfigModel) sectionFieldCountFor(section int) int {
	switch section {
	case configSectionIdentity:
		return identFieldCount
	case configSectionRadio:
		return radioFieldCount
	case configSectionActions:
		return actionFieldCount
	case configSectionChannels:
		if m.config != nil && len(m.config.Channels) > 0 {
			return len(m.config.Channels)
		}
		return 1
	}
	return 1
}

func (m ConfigModel) sectionFieldCount() int {
	switch m.activeSection {
	case configSectionIdentity:
		return identFieldCount
	case configSectionRadio:
		return radioFieldCount
	case configSectionActions:
		return actionFieldCount
	case configSectionChannels:
		if m.config != nil && len(m.config.Channels) > 0 {
			return len(m.config.Channels)
		}
		return 1
	}
	return 1
}

func (m *ConfigModel) populateRadioEdits() {
	if m.config == nil {
		return
	}
	r := m.config.Radio
	m.radioEdits[radioFieldFreq] = strconv.FormatFloat(float64(r.FrequencyMhz), 'f', 3, 64)
	m.radioEdits[radioFieldSF] = strconv.Itoa(r.SF)
	m.radioEdits[radioFieldBW] = strconv.Itoa(r.BwHz)
	m.radioEdits[radioFieldTxPower] = strconv.Itoa(r.TxPowerDbm)
	for i := range m.radioValid {
		m.radioValid[i] = true
	}
}

func (m ConfigModel) validateRadioField(field int, val string) bool {
	switch field {
	case radioFieldFreq:
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return false
		}
		return f >= 400.0 && f <= 928.0
	case radioFieldSF:
		n, err := strconv.Atoi(val)
		if err != nil {
			return false
		}
		return n >= 7 && n <= 12
	case radioFieldBW:
		n, err := strconv.Atoi(val)
		if err != nil {
			return false
		}
		return n == 125000 || n == 250000 || n == 500000
	case radioFieldTxPower:
		n, err := strconv.Atoi(val)
		if err != nil {
			return false
		}
		return n >= 0 && n <= 22
	}
	return true
}

func (m ConfigModel) allRadioValid() bool {
	for i := 0; i < radioFieldCount-1; i++ { // exclude Save button
		if !m.validateRadioField(i, m.radioEdits[i]) {
			return false
		}
	}
	return true
}

func (m ConfigModel) buildRadioConfig() bramble.RadioConfig {
	cfg := bramble.RadioConfig{}
	if f, err := strconv.ParseFloat(m.radioEdits[radioFieldFreq], 64); err == nil {
		cfg.FreqMhz = &f
	}
	if n, err := strconv.Atoi(m.radioEdits[radioFieldSF]); err == nil {
		cfg.SF = &n
	}
	if n, err := strconv.Atoi(m.radioEdits[radioFieldBW]); err == nil {
		bwKhz := n / 1000
		cfg.BwKhz = &bwKhz
	}
	if n, err := strconv.Atoi(m.radioEdits[radioFieldTxPower]); err == nil {
		cfg.TxPowerDbm = &n
	}
	return cfg
}

func (m *ConfigModel) setStatus(msg string, isErr bool) {
	m.statusMsg = msg
	m.statusIsError = isErr
	if msg != "" {
		m.statusExpiry = time.Now().Add(4 * time.Second)
	}
}

// ── View ──────────────────────────────────────────────────────────────────────

// View renders the config tab.
func (m ConfigModel) View() string {
	t := m.theme
	var sb strings.Builder

	sb.WriteString("\n")

	// ── Identity Section ──────────────────────────────────────────────────────
	sb.WriteString(t.sectionHeader.Render("  ┤ IDENTITY ├"))
	sb.WriteString("\n")
	m.renderIdentitySection(&sb)

	// ── Radio Section ─────────────────────────────────────────────────────────
	sb.WriteString(t.sectionHeader.Render("  ┤ RADIO SETTINGS ├"))
	sb.WriteString("\n")
	m.renderRadioSection(&sb)

	// ── Actions Section ───────────────────────────────────────────────────────
	sb.WriteString(t.sectionHeader.Render("  ┤ ACTIONS ├"))
	sb.WriteString("\n")
	m.renderActionsSection(&sb)

	// ── Channels Section ──────────────────────────────────────────────────────
	sb.WriteString(t.sectionHeader.Render("  ┤ CHANNELS ├"))
	sb.WriteString("\n")
	m.renderChannelsSection(&sb)

	// ── Confirm dialog ────────────────────────────────────────────────────────
	if m.confirmPending != "" {
		sb.WriteString("\n")
		var msg string
		switch m.confirmPending {
		case "reboot":
			msg = "Reboot node?"
		case "clearhistory":
			msg = "Clear message history?"
		case "radio":
			msg = "Save radio settings?"
		case "deletechannel":
			chName := ""
			if m.config != nil {
				idx := m.sectionCursor[configSectionChannels]
				if idx < len(m.config.Channels) {
					chName = m.config.Channels[idx].Name
				}
			}
			msg = fmt.Sprintf("Delete channel %q?", chName)
		}
		sb.WriteString(t.confirmMsg.Render(fmt.Sprintf("  ⚠  %s  [y/n]", msg)))
		sb.WriteString("\n")
	}

	// ── Add Channel dialog ────────────────────────────────────────────────────
	if m.addChannelStep > 0 {
		sb.WriteString("\n")
		if m.addChannelStep == 1 {
			sb.WriteString(t.confirmMsg.Render("  ┤ ADD CHANNEL ├  Name (required):"))
			sb.WriteString("\n")
			sb.WriteString(fmt.Sprintf("  %s\n", t.editValue.Render(m.addChannelName+"█")))
			sb.WriteString(t.mutedValue.Render("  Enter to continue · Esc to cancel"))
			sb.WriteString("\n")
		} else {
			sb.WriteString(t.confirmMsg.Render(fmt.Sprintf("  ┤ ADD CHANNEL ├  Name: %s  PSK (optional):", m.addChannelName)))
			sb.WriteString("\n")
			pskDisplay := strings.Repeat("*", len(m.addChannelPsk))
			sb.WriteString(fmt.Sprintf("  %s\n", t.editValue.Render(pskDisplay+"█")))
			sb.WriteString(t.mutedValue.Render("  Enter to add · Esc to cancel"))
			sb.WriteString("\n")
		}
	}

	// ── Status message ────────────────────────────────────────────────────────
	if m.statusMsg != "" && time.Now().Before(m.statusExpiry) {
		sb.WriteString("\n")
		style := t.successMsg
		if m.statusIsError {
			style = t.errorMsg
		}
		sb.WriteString(style.Render(fmt.Sprintf("  %s", m.statusMsg)))
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m ConfigModel) renderIdentitySection(sb *strings.Builder) {
	t := m.theme
	isActive := m.activeSection == configSectionIdentity
	cursor := m.sectionCursor[configSectionIdentity]

	addr := "—"
	pubkey := "—"
	name := "—"
	if m.config != nil {
		addr = m.config.Address
		name = m.config.NodeName
	}

	rows := []struct {
		field int
		label string
		value string
		ro    bool
	}{
		{identFieldAddress, "Address", addr, true},
		{identFieldPubkey, "Pubkey Hash", pubkey, true},
		{identFieldName, "Node Name", name, false},
		{identFieldMailbox, "Mailbox", "", false},
	}

	for _, row := range rows {
		selected := isActive && cursor == row.field
		label := t.label.Render(row.label)
		var val string

		if row.field == identFieldMailbox {
			if m.mailbox {
				val = t.toggleOn.Render("[●] Enabled")
			} else {
				val = t.toggleOff.Render("[○] Disabled")
			}
		} else if row.ro {
			val = t.mutedValue.Render(row.value)
		} else if selected && m.editing && row.field == identFieldName {
			val = t.editValue.Render(m.editBuffer + "█")
		} else {
			val = t.value.Render(row.value)
		}

		line := fmt.Sprintf("  %s  %s", label, val)
		if selected && !m.editing {
			line = t.selectedRow.Render(line)
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
}

func (m ConfigModel) renderRadioSection(sb *strings.Builder) {
	t := m.theme
	isActive := m.activeSection == configSectionRadio
	cursor := m.sectionCursor[configSectionRadio]

	radioLabels := [radioFieldCount]string{"Frequency (MHz)", "Spreading Factor", "Bandwidth (Hz)", "TX Power (dBm)", ""}
	radioHints := [radioFieldCount]string{"400–928 MHz", "7–12", "125000/250000/500000", "0–22 dBm", ""}

	for i := 0; i < radioFieldSave; i++ {
		selected := isActive && cursor == i
		label := t.label.Render(radioLabels[i])
		val := m.radioEdits[i]
		hint := t.mutedValue.Render(radioHints[i])

		var valStr string
		if selected && m.editing {
			valStr = t.editValue.Render(m.editBuffer + "█")
		} else if !m.radioValid[i] {
			valStr = t.errorMsg.Render(val + "  ← invalid")
		} else {
			valStr = t.value.Render(val)
		}

		line := fmt.Sprintf("  %s  %s  %s", label, valStr, hint)
		if selected && !m.editing {
			line = t.selectedRow.Render(line)
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	// Save button
	selected := isActive && cursor == radioFieldSave
	saveStyle := t.saveButton
	if selected {
		saveStyle = saveStyle.Background(lipgloss.Color("#00CC6A"))
	}
	btn := saveStyle.Render("[ Save Radio ]")
	sb.WriteString(fmt.Sprintf("  %s\n", btn))
}

func (m ConfigModel) renderChannelsSection(sb *strings.Builder) {
	t := m.theme
	isActive := m.activeSection == configSectionChannels
	cursor := m.sectionCursor[configSectionChannels]

	if m.config == nil || len(m.config.Channels) == 0 {
		sb.WriteString(t.mutedValue.Render("  No channels configured"))
		sb.WriteString("\n")
		sb.WriteString(t.mutedValue.Render("  a  Add channel"))
		sb.WriteString("\n")
		return
	}

	for i, ch := range m.config.Channels {
		selected := isActive && cursor == i

		defaultMark := "  "
		if ch.IsDefault {
			defaultMark = "★ "
		}

		pskStatus := t.mutedValue.Render("no-psk")
		if ch.HasPsk {
			pskStatus = t.toggleOn.Render("psk")
		}

		name := t.value.Render(ch.Name)
		line := fmt.Sprintf("  %s[%d] %-20s %s", defaultMark, i, name, pskStatus)
		if selected {
			line = t.selectedRow.Render(line)
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	if isActive {
		sb.WriteString(t.mutedValue.Render("  a Add · d Delete · s Set default"))
		sb.WriteString("\n")
	}
}

func (m ConfigModel) renderActionsSection(sb *strings.Builder) {
	t := m.theme
	isActive := m.activeSection == configSectionActions
	cursor := m.sectionCursor[configSectionActions]

	type actionRow struct {
		label string
		value string
	}

	var debugVal string
	if m.trafficDebug {
		debugVal = t.toggleOn.Render("[●] On")
	} else {
		debugVal = t.toggleOff.Render("[○] Off")
	}

	rows := []actionRow{
		{"Reboot Node", t.errorMsg.Render("[ Reboot ]")},
		{"Clear History", t.value.Render("[ Clear ]")},
		{"Traffic Debug", debugVal},
	}

	for i, row := range rows {
		selected := isActive && cursor == i
		label := t.label.Render(row.label)
		line := fmt.Sprintf("  %s  %s", label, row.value)
		if selected {
			line = t.selectedRow.Render(line)
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}
}
