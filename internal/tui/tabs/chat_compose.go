// Package tabs provides per-tab Bubble Tea sub-models for the Bramble TUI.
package tabs

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	bramble "github.com/justinlindh/bramble-go"
)

const (
	maxBytes         = 200
	warnBytes        = 150
	fragmentWarning  = " (will fragment)"
)

// SendResultMsg is returned after an async send attempt.
type SendResultMsg struct {
	ConvID string
	Text   string
	MsgID  string
	Err    error
}

// ComposeModel is the compose bar sub-model for the Chat tab.
type ComposeModel struct {
	client       *bramble.Client
	activeConvID string
	textarea     textarea.Model
	width        int
	focused      bool
	// styles
	counterNormal lipgloss.Style
	counterWarn   lipgloss.Style
	counterErr    lipgloss.Style
	barStyle      lipgloss.Style
}

// NewCompose creates a new ComposeModel.
func NewCompose(client *bramble.Client) ComposeModel {
	ta := textarea.New()
	ta.Placeholder = "Type a message..."
	ta.ShowLineNumbers = false
	ta.SetHeight(3)
	ta.CharLimit = 0 // no hard limit; we show soft warning

	// Remap Enter to NOT insert newline by default — we'll intercept it.
	// We use Ctrl+Enter / Alt+Enter for newlines in the textarea.
	km := textarea.DefaultKeyMap()
	km.InsertNewline.SetKeys("ctrl+enter", "alt+enter")
	ta.KeyMap = km

	return ComposeModel{
		client:   client,
		textarea: ta,
		counterNormal: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")),
		counterWarn: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFAA00")).
			Bold(true),
		counterErr: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5555")).
			Bold(true),
		barStyle: lipgloss.NewStyle().
			BorderTop(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("#555588")),
	}
}

// SetSize updates the compose bar dimensions.
func (m *ComposeModel) SetSize(width int) {
	m.width = width
	// Reserve space for byte counter "NNN/200 (will fragment)" ~25 chars
	taWidth := width - 27
	if taWidth < 20 {
		taWidth = 20
	}
	m.textarea.SetWidth(taWidth)
}

// SetConvID sets the active conversation ID.
func (m *ComposeModel) SetConvID(id string) {
	m.activeConvID = id
}

// Focus focuses the compose textarea.
func (m *ComposeModel) Focus() tea.Cmd {
	m.focused = true
	return m.textarea.Focus()
}

// Blur removes focus from the compose textarea.
func (m *ComposeModel) Blur() {
	m.focused = false
	m.textarea.Blur()
}

// Focused reports whether the compose bar is focused.
func (m ComposeModel) Focused() bool {
	return m.focused
}

// Value returns the current text in the compose field.
func (m ComposeModel) Value() string {
	return m.textarea.Value()
}

// Init implements tea.Model.
func (m ComposeModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m ComposeModel) Update(msg tea.Msg) (ComposeModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			m.textarea.SetValue("")
			m.textarea.Blur()
			m.focused = false
			return m, nil

		case "enter":
			text := strings.TrimSpace(m.textarea.Value())
			if text == "" {
				return m, nil
			}
			convID := m.activeConvID
			client := m.client
			// Clear immediately on send attempt.
			m.textarea.SetValue("")
			return m, sendCmd(client, convID, text)
		}
	}

	if m.focused {
		var taCmd tea.Cmd
		m.textarea, taCmd = m.textarea.Update(msg)
		cmds = append(cmds, taCmd)
	}

	return m, tea.Batch(cmds...)
}

// View renders the compose bar.
func (m ComposeModel) View() string {
	text := m.textarea.Value()
	byteCount := len([]byte(text))

	// Build counter string.
	counterStr := fmt.Sprintf("%d/%d", byteCount, maxBytes)
	var counter string
	switch {
	case byteCount > maxBytes:
		counter = m.counterErr.Render(counterStr + fragmentWarning)
	case byteCount > warnBytes:
		counter = m.counterWarn.Render(counterStr)
	default:
		counter = m.counterNormal.Render(counterStr)
	}

	taView := m.textarea.View()
	// Combine textarea + counter on same line.
	row := lipgloss.JoinHorizontal(lipgloss.Bottom, taView, "  ", counter)
	return m.barStyle.Width(m.width).Render(row)
}

// sendCmd returns a tea.Cmd that sends a message async.
func sendCmd(client *bramble.Client, convID, text string) tea.Cmd {
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
			ch, _ := strconv.Atoi(chStr)
			var res *bramble.SendResult
			res, err = client.BroadcastOnChannel(ctx, ch, text)
			if err == nil {
				msgID = res.MessageID
			}

		case strings.HasPrefix(convID, "dm:"):
			addrStr := strings.TrimPrefix(convID, "dm:")
			// Parse hex address.
			var addr uint64
			_, parseErr := fmt.Sscanf(addrStr, "%x", &addr)
			if parseErr != nil {
				err = fmt.Errorf("invalid dm address %q: %w", addrStr, parseErr)
			} else {
				var res *bramble.SendResult
				res, err = client.Send(ctx, uint32(addr), text)
				if err == nil {
					msgID = res.MessageID
				}
			}

		default:
			err = fmt.Errorf("unknown conversation id %q", convID)
		}

		return SendResultMsg{
			ConvID: convID,
			Text:   text,
			MsgID:  msgID,
			Err:    err,
		}
	}
}
