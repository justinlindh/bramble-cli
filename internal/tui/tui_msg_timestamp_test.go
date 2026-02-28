package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	bramble "github.com/justinlindh/bramble-go"
)

func TestMsgReceived_UsesMessageTimestampInRenderedLine(t *testing.T) {
	m := New(nil, NodeInfo{Address: "A1"}, nil, nil)
	m.activeConv = "broadcast"
	m.store.SetActiveConv("broadcast")

	ts := time.Now().Add(-2 * time.Hour).Unix()
	updated, _ := m.Update(MsgReceived{Msg: bramble.Message{
		From:      "B2",
		To:        "broadcast",
		Text:      "hello from replay",
		Timestamp: ts,
	}})
	m2 := updated.(Model)

	if got := m2.scroll.LineCount(); got != 1 {
		t.Fatalf("expected 1 rendered line, got %d", got)
	}

	wantTS := fmt.Sprintf("[%s]", time.Unix(ts, 0).Format("15:04"))
	if !strings.Contains(m2.scroll.lines[0].Text, wantTS) {
		t.Fatalf("expected rendered line to contain message timestamp %q, got %q", wantTS, m2.scroll.lines[0].Text)
	}
}
