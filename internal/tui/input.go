package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	singlePacketMaxBytes = 203
	fragmentPayloadBytes = 154
	fragmentedMaxBytes   = 616
)

// InputMsg is sent when the user presses Enter with non-empty text.
type InputMsg struct {
	Text      string
	IsCommand bool // starts with /
}

// InputLine is the always-visible input line at the bottom.
type InputLine struct {
	textarea textarea.Model
	prompt   string // e.g. "[broadcast]" or "[dm:NodeB]"
	width    int
	style    InputStyle
}

type InputStyle struct {
	Prompt       lipgloss.Style
	Border       lipgloss.Style
	Typeahead    lipgloss.Style
	ByteOK       lipgloss.Style
	ByteWarn     lipgloss.Style
	ByteHigh     lipgloss.Style
	ByteError    lipgloss.Style
	CounterLabel lipgloss.Style
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

	// Focus immediately — textarea must be focused to accept input
	ta.Focus()

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
			Typeahead: lipgloss.NewStyle().
				Foreground(lipgloss.Color("#666688")),
			ByteOK: lipgloss.NewStyle().
				Foreground(lipgloss.Color("#aaaacc")),
			ByteWarn: lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFAA00")).Bold(true),
			ByteHigh: lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF8800")).Bold(true),
			ByteError: lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF5555")).Bold(true),
			CounterLabel: lipgloss.NewStyle().
				Foreground(lipgloss.Color("#8888aa")),
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
		case "tab":
			if suffix := commandSuggestionSuffix(il.textarea.Value()); suffix != "" {
				il.textarea.SetValue(il.textarea.Value() + suffix)
				return il, nil
			}
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
	if suffix := commandSuggestionSuffix(il.textarea.Value()); suffix != "" {
		ta += il.style.Typeahead.Render(suffix)
	}

	meta := messageByteMeta(il.textarea.Value(), strings.HasPrefix(strings.TrimSpace(il.textarea.Value()), "/"))
	byteStyle := il.style.ByteOK
	switch {
	case meta.OverLimit:
		byteStyle = il.style.ByteError
	case meta.FragmentCount >= 3:
		byteStyle = il.style.ByteHigh
	case meta.FragmentCount == 2:
		byteStyle = il.style.ByteWarn
	}
	byteIndicator := " " + byteStyle.Render(fmt.Sprintf("[%d/%d bytes]", meta.ByteCount, meta.MaxBytes))
	if meta.Label != "" {
		byteIndicator += " " + il.style.CounterLabel.Render(meta.Label)
	}

	line := prompt + " " + ta + byteIndicator
	return il.style.Border.Width(il.width).Render(line)
}

type byteMeta struct {
	ByteCount     int
	MaxBytes      int
	FragmentCount int
	OverLimit     bool
	Label         string
}

func messageByteMeta(text string, isCommand bool) byteMeta {
	if isCommand {
		meta := byteMeta{ByteCount: len([]byte(text)), MaxBytes: fragmentedMaxBytes}
		if label, over := commandLimitLabel(text); label != "" {
			meta.Label = label
			meta.OverLimit = over
		}
		return meta
	}
	bytes := len([]byte(text))
	meta := byteMeta{ByteCount: bytes, MaxBytes: fragmentedMaxBytes}
	if bytes == 0 {
		return meta
	}
	if bytes <= singlePacketMaxBytes {
		meta.FragmentCount = 1
		return meta
	}
	meta.FragmentCount = (bytes + fragmentPayloadBytes - 1) / fragmentPayloadBytes
	if meta.FragmentCount > 4 {
		meta.OverLimit = true
		meta.Label = "too long"
		return meta
	}
	meta.Label = fmt.Sprintf("%d fragments", meta.FragmentCount)
	return meta
}

var knownCommands = []string{
	"/alias", "/b", "/broadcast", "/ch", "/clear", "/close", "/config", "/critical", "/dm",
	"/h", "/help", "/location", "/loc", "/me", "/mouse", "/msg", "/nick", "/nodes",
	"/ping", "/probe", "/q", "/quit", "/reboot", "/slap", "/stats", "/w", "/windows",
}

func commandSuggestionSuffix(input string) string {
	if !strings.HasPrefix(input, "/") || strings.ContainsAny(input, " \t") {
		return ""
	}
	for _, cmd := range knownCommands {
		if strings.HasPrefix(cmd, input) && cmd != input {
			return strings.TrimPrefix(cmd, input)
		}
	}
	return ""
}

func commandLimitLabel(text string) (label string, over bool) {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "/nick ") {
		name := strings.TrimSpace(strings.TrimPrefix(trimmed, "/nick "))
		if len([]rune(name)) > 32 {
			return "nick max 32 chars", true
		}
		return "nick max 32 chars", false
	}
	if strings.HasPrefix(trimmed, "/config set name ") {
		name := strings.TrimSpace(strings.TrimPrefix(trimmed, "/config set name "))
		if len([]rune(name)) > 32 {
			return "name max 32 chars", true
		}
		return "name max 32 chars", false
	}
	return "", false
}
