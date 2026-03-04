package tui

import (
	"testing"

	bramble "github.com/justinlindh/bramble-go"
)

func TestClassifyMessageConvID_EmptyToIncomingRoutesToDM(t *testing.T) {
	msg := bramble.Message{From: "A1B2C3D4", To: "", Text: "hi"}
	if got := ClassifyMessageConvID(msg, "DEADBEEF"); got != "dm:A1B2C3D4" {
		t.Fatalf("convID: got %q want %q", got, "dm:A1B2C3D4")
	}
}

func TestUpdateMsgReceived_EmptyToIncomingStoredInDMNotBroadcast(t *testing.T) {
	m := New(nil, NodeInfo{Address: "DEADBEEF", Name: "me"}, nil, nil)
	m.activeConv = "broadcast"
	m.store.SetActiveConv("broadcast")

	updated, _ := m.Update(MsgReceived{Msg: bramble.Message{From: "A1B2C3D4", Text: "hello"}})
	m2 := updated.(Model)

	dm := m2.store.Conversations["dm:A1B2C3D4"]
	if dm == nil || len(dm.Messages) != 1 {
		t.Fatalf("expected DM conversation to contain message")
	}
	if got := dm.Messages[0].Text; got != "hello" {
		t.Fatalf("dm text: got %q want hello", got)
	}

	if b := m2.store.Conversations["broadcast"]; b != nil && len(b.Messages) != 0 {
		t.Fatalf("expected broadcast messages unchanged, got %d", len(b.Messages))
	}
}

func TestClassifyMessageConvID_ExplicitBroadcastStillBroadcast(t *testing.T) {
	msg := bramble.Message{From: "A1B2C3D4", To: "FFFFFFFF", Text: "all"}
	if got := ClassifyMessageConvID(msg, "DEADBEEF"); got != "broadcast" {
		t.Fatalf("convID: got %q want broadcast", got)
	}
}
