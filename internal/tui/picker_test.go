package tui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/justinlindh/bramble-cli/internal/devices"
)

func seedPicker(t *testing.T) (PickerModel, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "devices.json")
	book := &devices.Book{Devices: map[string]devices.Entry{}}
	if err := book.Add("v3", devices.Entry{Host: "192.0.2.0", Token: "t3", Name: "V3"}); err != nil {
		t.Fatal(err)
	}
	if err := book.Add("v4", devices.Entry{Host: "192.0.2.0", Token: "t4", Name: "V4"}); err != nil {
		t.Fatal(err)
	}
	if err := book.Save(path); err != nil {
		t.Fatal(err)
	}
	m := NewPicker(book, path)
	m = apply(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	return m, path
}

func apply(m PickerModel, msg tea.Msg) PickerModel {
	updated, _ := m.Update(msg)
	return updated.(PickerModel)
}

func keyRune(r rune) tea.KeyPressMsg    { return tea.KeyPressMsg{Code: r, Text: string(r)} }
func keyCode(code rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code} }

func typeStr(m PickerModel, s string) PickerModel {
	for _, r := range s {
		m = apply(m, keyRune(r))
	}
	return m
}

func TestPicker_SelectConnects(t *testing.T) {
	m, _ := seedPicker(t)
	// First item is "v3" (sorted). Enter selects it.
	m = apply(m, keyCode(tea.KeyEnter))
	if m.Choice() != "v3" {
		t.Fatalf("Choice = %q, want v3", m.Choice())
	}
	if m.Quit() {
		t.Fatal("Quit should be false on a selection")
	}
}

func TestPicker_DeleteConfirmedPersists(t *testing.T) {
	m, path := seedPicker(t)
	m = apply(m, keyRune('d')) // arm delete on highlighted v3
	if m.mode != modeConfirmDelete || m.deleteAlias != "v3" {
		t.Fatalf("expected confirm-delete for v3, got mode=%d alias=%q", m.mode, m.deleteAlias)
	}
	m = apply(m, keyRune('y')) // confirm
	if m.mode != modeList {
		t.Fatalf("expected return to list, got mode=%d", m.mode)
	}
	// Disk must reflect the deletion.
	book, err := devices.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := book.Get("v3"); ok {
		t.Fatal("v3 should be deleted on disk")
	}
	if _, ok := book.Get("v4"); !ok {
		t.Fatal("v4 should remain")
	}
	if got := len(m.list.Items()); got != 1 {
		t.Fatalf("list should have 1 item, got %d", got)
	}
}

func TestPicker_DeleteCancelledKeeps(t *testing.T) {
	m, path := seedPicker(t)
	m = apply(m, keyRune('d'))
	m = apply(m, keyRune('n')) // cancel
	if m.mode != modeList {
		t.Fatalf("expected list mode after cancel, got %d", m.mode)
	}
	book, err := devices.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := book.Get("v3"); !ok {
		t.Fatal("v3 must still exist after cancelled delete")
	}
}

func TestPicker_AddNewPersistsAndMasks(t *testing.T) {
	m, path := seedPicker(t)
	m = apply(m, keyRune('a')) // enter add mode
	if m.mode != modeAdd {
		t.Fatalf("expected add mode, got %d", m.mode)
	}
	m = typeStr(m, "v9")                // alias field
	m = apply(m, keyCode(tea.KeyEnter)) // -> host
	m = typeStr(m, "10.0.0.9")          // host field (bare -> normalized)
	m = apply(m, keyCode(tea.KeyEnter)) // -> token
	m = typeStr(m, "supersecret")       // token field (hidden)

	// The add form must never render the raw token.
	if strings.Contains(m.addView(), "supersecret") {
		t.Fatal("add view leaked the token")
	}

	m = apply(m, keyCode(tea.KeyEnter)) // submit
	if m.addErr != "" {
		t.Fatalf("unexpected add error: %q", m.addErr)
	}
	if m.mode != modeList {
		t.Fatalf("expected list mode after add, got %d", m.mode)
	}

	book, err := devices.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	e, ok := book.Get("v9")
	if !ok {
		t.Fatal("v9 not saved")
	}
	if e.Host != "ws://10.0.0.9/ws" {
		t.Errorf("host = %q, want normalized", e.Host)
	}
	if e.Token != "supersecret" {
		t.Errorf("token = %q, want supersecret", e.Token)
	}
}

func TestPicker_AddRejectsDuplicate(t *testing.T) {
	m, path := seedPicker(t)
	m = apply(m, keyRune('a'))
	m = typeStr(m, "v4") // already exists
	m = apply(m, keyCode(tea.KeyEnter))
	m = typeStr(m, "1.2.3.4")
	m = apply(m, keyCode(tea.KeyEnter))
	// token left blank
	m = apply(m, keyCode(tea.KeyEnter)) // submit

	if m.mode != modeAdd {
		t.Fatalf("expected to stay in add mode on dup, got %d", m.mode)
	}
	if !strings.Contains(m.addErr, "already exists") {
		t.Fatalf("expected dup error, got %q", m.addErr)
	}
	// The existing v4 must be untouched on disk.
	book, err := devices.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if e, _ := book.Get("v4"); e.Host != "ws://192.0.2.0/ws" {
		t.Fatalf("v4 host mutated: %q", e.Host)
	}
}

func TestPicker_AddCancelWithEsc(t *testing.T) {
	m, _ := seedPicker(t)
	m = apply(m, keyRune('a'))
	m = typeStr(m, "typing")
	m = apply(m, keyCode(tea.KeyEsc))
	if m.mode != modeList {
		t.Fatalf("expected list mode after esc, got %d", m.mode)
	}
	if v := m.inputs[fieldAlias].Value(); v != "" {
		t.Fatalf("alias input should be cleared on cancel, got %q", v)
	}
}
