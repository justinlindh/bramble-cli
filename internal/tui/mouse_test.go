package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestMouseModeDefaultOnInView(t *testing.T) {
	m := New(nil, NodeInfo{Address: "A1"}, nil, nil)
	m.ready = true
	m.width = 80
	m.height = 24
	m.scroll.SetSize(80, 20)
	m.statusBar.SetWidth(80)
	m.input.SetWidth(80)

	v := m.View()
	if v.MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("expected default mouse mode cell motion, got %v", v.MouseMode)
	}
}

func TestMouseCommandTogglesAndReportsState(t *testing.T) {
	m := New(nil, NodeInfo{Address: "A1"}, nil, nil)

	updated, _ := m.Update(InputMsg{Text: "/mouse", IsCommand: true})
	m2 := updated.(Model)
	if m2.mouseEnabled {
		t.Fatalf("expected /mouse to toggle off")
	}

	conv := m2.store.GetActiveConversation()
	if len(conv.Events) == 0 || !strings.Contains(conv.Events[len(conv.Events)-1].Text, "Mouse mode: off") {
		t.Fatalf("expected state line with Mouse mode: off")
	}

	updated, _ = m2.Update(InputMsg{Text: "/mouse on", IsCommand: true})
	m3 := updated.(Model)
	if !m3.mouseEnabled {
		t.Fatalf("expected /mouse on to set on")
	}
}

func TestMouseDisabledSkipsMouseClickHandling(t *testing.T) {
	m := New(nil, NodeInfo{Address: "A1"}, nil, nil)
	m.ready = true
	m.width = 80
	m.height = 24
	m.scroll.SetSize(80, 20)
	m.statusBar.SetWidth(80)
	m.input.SetWidth(80)
	m.store.AddConversationLine("dm:BEEF1234", ScrollLine{Kind: LineSystem, Text: "x"})
	m.updateStatusBar()
	m.mouseEnabled = false

	updated, _ := m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 2, Y: 20})
	m2 := updated.(Model)
	if m2.activeConv != "broadcast" {
		t.Fatalf("expected active conversation unchanged when mouse disabled, got %q", m2.activeConv)
	}
}
