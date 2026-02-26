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
