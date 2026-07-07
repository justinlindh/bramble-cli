package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/justinlindh/bramble-cli/internal/devices"
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

// pickerMode is which sub-screen the picker is showing.
type pickerMode int

const (
	modeList pickerMode = iota
	modeConfirmDelete
	modeAdd
)

// add-form field indexes.
const (
	fieldAlias = iota
	fieldHost
	fieldToken
	numAddFields
)

// PickerModel is a Bubble Tea program that both selects a saved device and
// manages the address book (add / delete). It reports the chosen alias
// (Choice) or that the user quit without choosing (Quit). Book mutations are
// persisted immediately via the internal/devices package.
type PickerModel struct {
	list list.Model
	book *devices.Book
	path string

	mode        pickerMode
	deleteAlias string
	status      string // transient message shown under the list

	inputs []textinput.Model
	focus  int
	addErr string

	choice string
	quit   bool
}

// NewPicker builds a device picker over an address book. path is where book
// changes are saved.
func NewPicker(book *devices.Book, path string) PickerModel {
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Select a device"
	l.SetShowStatusBar(false)
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "connect")),
			key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add")),
			key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		}
	}

	m := PickerModel{list: l, book: book, path: path, inputs: newAddInputs()}
	m.refreshList("")
	return m
}

func newAddInputs() []textinput.Model {
	alias := textinput.New()
	alias.Placeholder = "alias (e.g. v4)"
	alias.CharLimit = 64

	host := textinput.New()
	host.Placeholder = "192.0.2.0 or ws://host/ws"
	host.CharLimit = 200

	token := textinput.New()
	token.Placeholder = "token (hidden, optional)"
	token.EchoMode = textinput.EchoPassword
	token.CharLimit = 200

	return []textinput.Model{alias, host, token}
}

// refreshList rebuilds the list items from the book, optionally selecting the
// row whose alias matches selectAlias.
func (m *PickerModel) refreshList(selectAlias string) {
	entries := m.book.List()
	items := make([]list.Item, 0, len(entries))
	sel := -1
	for i, ne := range entries {
		items = append(items, pickerItem{c: DeviceChoice{
			Alias: ne.Alias,
			Name:  ne.Entry.Name,
			Host:  ne.Entry.Host,
		}})
		if ne.Alias == selectAlias {
			sel = i
		}
	}
	m.list.SetItems(items)
	if sel >= 0 {
		m.list.Select(sel)
	}
}

// Init implements tea.Model.
func (m PickerModel) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m PickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Window size applies in every mode.
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.list.SetSize(ws.Width, ws.Height)
		return m, nil
	}
	switch m.mode {
	case modeConfirmDelete:
		return m.updateConfirm(msg)
	case modeAdd:
		return m.updateAdd(msg)
	default:
		return m.updateList(msg)
	}
}

func (m PickerModel) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyPressMsg); ok && m.list.FilterState() != list.Filtering {
		switch km.String() {
		case "ctrl+c", "q", "esc":
			m.quit = true
			return m, tea.Quit
		case "enter":
			if it, ok := m.list.SelectedItem().(pickerItem); ok {
				m.choice = it.c.Alias
				return m, tea.Quit
			}
			return m, nil
		case "a", "n":
			return m.enterAddMode()
		case "d", "delete":
			if it, ok := m.list.SelectedItem().(pickerItem); ok {
				m.mode = modeConfirmDelete
				m.deleteAlias = it.c.Alias
				m.status = ""
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m PickerModel) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch strings.ToLower(km.String()) {
	case "y":
		alias := m.deleteAlias
		m.book.Remove(alias)
		if err := m.book.Save(m.path); err != nil {
			m.status = "delete not saved: " + err.Error()
		} else {
			m.status = fmt.Sprintf("Deleted %q", alias)
		}
		m.refreshList("")
		m.mode = modeList
		return m, nil
	default: // n, esc, or anything else cancels
		m.mode = modeList
		return m, nil
	}
}

func (m PickerModel) updateAdd(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyPressMsg); ok {
		switch km.String() {
		case "esc":
			return m.exitAddMode(), nil
		case "tab", "down":
			return m.focusAdd(m.focus + 1), nil
		case "shift+tab", "up":
			return m.focusAdd(m.focus - 1), nil
		case "enter":
			if m.focus < numAddFields-1 {
				return m.focusAdd(m.focus + 1), nil
			}
			return m.submitAdd()
		}
	}

	var cmd tea.Cmd
	m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
	return m, cmd
}

func (m PickerModel) enterAddMode() (tea.Model, tea.Cmd) {
	m.mode = modeAdd
	m.addErr = ""
	m.status = ""
	for i := range m.inputs {
		m.inputs[i].SetValue("")
		m.inputs[i].Blur()
	}
	m.focus = fieldAlias
	cmd := m.inputs[m.focus].Focus()
	return m, cmd
}

func (m PickerModel) exitAddMode() tea.Model {
	for i := range m.inputs {
		m.inputs[i].Blur()
		m.inputs[i].SetValue("")
	}
	m.mode = modeList
	m.addErr = ""
	m.focus = fieldAlias
	return m
}

// focusAdd moves focus to field index (wrapping) and returns the model.
func (m PickerModel) focusAdd(index int) tea.Model {
	if index < 0 {
		index = numAddFields - 1
	}
	index %= numAddFields
	for i := range m.inputs {
		if i == index {
			m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
	m.focus = index
	return m
}

func (m PickerModel) submitAdd() (tea.Model, tea.Cmd) {
	alias := strings.TrimSpace(m.inputs[fieldAlias].Value())
	host := strings.TrimSpace(m.inputs[fieldHost].Value())
	token := m.inputs[fieldToken].Value()

	if err := devices.ValidateAlias(alias); err != nil {
		m.addErr = err.Error()
		return m, nil
	}
	if _, exists := m.book.Get(alias); exists {
		m.addErr = fmt.Sprintf("alias %q already exists", alias)
		return m, nil
	}
	// Add normalizes the host and rejects an empty one.
	if err := m.book.Add(alias, devices.Entry{Host: host, Token: token}); err != nil {
		m.addErr = err.Error()
		return m, nil
	}
	if err := m.book.Save(m.path); err != nil {
		m.book.Remove(alias) // keep memory and disk consistent
		m.addErr = "save failed: " + err.Error()
		return m, nil
	}

	m.refreshList(alias) // highlight the new entry
	m.status = fmt.Sprintf("Added %q", alias)
	return m.exitAddMode(), nil
}

// View implements tea.Model.
func (m PickerModel) View() tea.View {
	var body string
	switch m.mode {
	case modeConfirmDelete:
		body = m.confirmView()
	case modeAdd:
		body = m.addView()
	default:
		body = m.listView()
	}
	v := tea.NewView(body)
	v.AltScreen = true
	return v
}

func (m PickerModel) listView() string {
	if m.status != "" {
		return m.list.View() + "\n  " + m.status
	}
	return m.list.View()
}

func (m PickerModel) confirmView() string {
	var b strings.Builder
	b.WriteString("\n  Delete alias \"" + m.deleteAlias + "\"?\n\n")
	b.WriteString("  y = delete      n / esc = cancel\n")
	return b.String()
}

func (m PickerModel) addView() string {
	labels := []string{"Alias", "Host ", "Token"}
	var b strings.Builder
	b.WriteString("\n  Add a device\n\n")
	for i := range m.inputs {
		marker := "  "
		if i == m.focus {
			marker = "> "
		}
		b.WriteString(marker + labels[i] + ": " + m.inputs[i].View() + "\n")
	}
	b.WriteString("\n  tab / up / down: move    enter: next or save    esc: cancel\n")
	if m.addErr != "" {
		b.WriteString("\n  error: " + m.addErr + "\n")
	}
	return b.String()
}

// Choice returns the selected alias, or "" if none was chosen.
func (m PickerModel) Choice() string { return m.choice }

// Quit reports whether the user dismissed the picker without choosing.
func (m PickerModel) Quit() bool { return m.quit }
