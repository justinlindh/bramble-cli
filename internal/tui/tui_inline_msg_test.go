package tui

import "testing"

func TestInlineDMResultEchoesInCurrentBufferAndStoresInDM(t *testing.T) {
	m := New(nil, NodeInfo{Address: "DEADBEEF", Name: "me"}, nil, nil)
	m.activeConv = "broadcast"
	m.store.SetActiveConv("broadcast")

	updated, _ := m.Update(sendResultMsg{
		convID:     "dm:A1B2C3D4",
		echoConvID: "broadcast",
		toAddr:     "A1B2C3D4",
		text:       "hello",
		display:    "-> [@A1B2C3D4] hello",
		msgID:      "mid-1",
		inlineDM:   true,
	})
	m2 := updated.(Model)

	if m2.activeConv != "broadcast" {
		t.Fatalf("expected active conv to remain broadcast, got %q", m2.activeConv)
	}

	b := m2.store.Conversations["broadcast"]
	if b == nil || len(b.Events) == 0 {
		t.Fatalf("expected broadcast to contain echoed info event")
	}

	dm := m2.store.Conversations["dm:A1B2C3D4"]
	if dm == nil || len(dm.Messages) != 1 {
		t.Fatalf("expected dm buffer to contain one stored message")
	}
	if got := dm.Messages[0].Text; got != "hello" {
		t.Fatalf("expected dm text %q, got %q", "hello", got)
	}
}
