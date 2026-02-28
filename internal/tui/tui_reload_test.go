package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	bramble "github.com/justinlindh/bramble-go"
)

func TestReloadScrollback_ReplaysConversationEvents(t *testing.T) {
	m := New(nil, NodeInfo{Address: "a1"}, nil, nil)
	m.activeConv = "broadcast"
	m.store.SetActiveConv("broadcast")

	m.store.AddMessage(bramble.Message{From: "b2", To: "broadcast", Text: "hello", Timestamp: 200})
	m.store.AddConversationLine("broadcast", ScrollLine{Kind: LineSystem, Timestamp: time.Unix(100, 0), Text: "-- joined --"})
	m.store.AddConversationLine("broadcast", ScrollLine{Kind: LineInfo, Timestamp: time.Unix(300, 0), Text: "info line"})

	m.reloadScrollback()

	if got := m.scroll.LineCount(); got != 3 {
		t.Fatalf("expected 3 lines, got %d", got)
	}
	if m.scroll.lines[0].Kind != LineSystem {
		t.Fatalf("expected first line to be system, got %v", m.scroll.lines[0].Kind)
	}
	if m.scroll.lines[1].Kind != LineChat {
		t.Fatalf("expected second line to be chat, got %v", m.scroll.lines[1].Kind)
	}
	if m.scroll.lines[2].Kind != LineInfo {
		t.Fatalf("expected third line to be info, got %v", m.scroll.lines[2].Kind)
	}

	wantChatTS := fmt.Sprintf("[%s]", time.Unix(200, 0).Format("15:04"))
	if !strings.Contains(m.scroll.lines[1].Text, wantChatTS) {
		t.Fatalf("expected chat line to render original timestamp %q, got %q", wantChatTS, m.scroll.lines[1].Text)
	}
}
