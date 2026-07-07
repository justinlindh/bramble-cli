package tui

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
)

// DeviceChoice is one row in the device picker. It never carries the token.
type DeviceChoice struct {
	Alias string
	Name  string
	Host  string
}

// pickerItem adapts a DeviceChoice to the bubbles list.DefaultItem interface.
type pickerItem struct{ c DeviceChoice }

func (i pickerItem) Title() string { return i.c.Alias }

func (i pickerItem) Description() string {
	if i.c.Name != "" {
		return i.c.Name + "  " + i.c.Host
	}
	return i.c.Host
}

func (i pickerItem) FilterValue() string {
	return i.c.Alias + " " + i.c.Name + " " + i.c.Host
}

// PickerModel is a small Bubble Tea program that lets the user pick a saved
// device before the main TUI connects. It reports the chosen alias (Choice) or
// that the user quit without choosing (Quit).
type PickerModel struct {
	list   list.Model
	choice string
	quit   bool
}

// NewPicker builds a device picker over the given choices.
func NewPicker(choices []DeviceChoice) PickerModel {
	items := make([]list.Item, len(choices))
	for i, c := range choices {
		items[i] = pickerItem{c: c}
	}
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Select a device (enter to connect, q to quit)"
	l.SetShowStatusBar(false)
	return PickerModel{list: l}
}

// Init implements tea.Model.
func (m PickerModel) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m PickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyPressMsg:
		// While the user is typing a filter, let the list consume keys so that
		// "q"/enter edit the filter rather than quitting or selecting.
		if m.list.FilterState() != list.Filtering {
			switch msg.String() {
			case "ctrl+c", "q", "esc":
				m.quit = true
				return m, tea.Quit
			case "enter":
				if it, ok := m.list.SelectedItem().(pickerItem); ok {
					m.choice = it.c.Alias
					return m, tea.Quit
				}
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View implements tea.Model.
func (m PickerModel) View() tea.View {
	v := tea.NewView(m.list.View())
	v.AltScreen = true
	return v
}

// Choice returns the selected alias, or "" if none was chosen.
func (m PickerModel) Choice() string { return m.choice }

// Quit reports whether the user dismissed the picker without choosing.
func (m PickerModel) Quit() bool { return m.quit }
