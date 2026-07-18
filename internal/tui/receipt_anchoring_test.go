package tui

import (
	"strings"
	"testing"
	"time"

	bramble "github.com/justinlindh/bramble-go"
)

// newBroadcastModel returns a Model whose active conversation is the broadcast
// buffer, ready to drive send/delivery events through Update.
func newBroadcastModel(t *testing.T) Model {
	t.Helper()
	m := New(nil, NodeInfo{Address: "AAAA0001", Name: "alice"}, nil, nil)
	m.activeConv = "broadcast"
	m.store.SetActiveConv("broadcast")
	return m
}

// sendBroadcast simulates the completion of a local broadcast send: it echoes
// the outgoing message and registers its broadcast correlation id.
func sendBroadcast(t *testing.T, m Model, text, msgID, broadcastID string) Model {
	t.Helper()
	updated, _ := m.Update(sendResultMsg{
		convID:      "broadcast",
		text:        text,
		msgID:       msgID,
		broadcastID: broadcastID,
	})
	return updated.(Model)
}

func deliver(t *testing.T, m Model, broadcastID, recipient string) Model {
	t.Helper()
	updated, _ := m.Update(BroadcastDeliveryReceived{Delivery: bramble.BroadcastDelivery{
		BroadcastID: broadcastID,
		Recipient:   recipient,
		Status:      "delivered",
	}})
	return updated.(Model)
}

// deliveryLineIndex returns the index of the single delivery line containing
// substr, or -1.
func deliveryLineIndex(m Model, substr string) int {
	for i, l := range m.scroll.lines {
		if l.Kind == LineDelivery && strings.Contains(l.Text, substr) {
			return i
		}
	}
	return -1
}

func countDeliveryLines(m Model) int {
	n := 0
	for _, l := range m.scroll.lines {
		if l.Kind == LineDelivery {
			n++
		}
	}
	return n
}

// Symptom 1: a receipt that arrives after an unrelated later message was
// printed must anchor beneath the broadcast it confirms, not beneath the later
// message.
func TestReceiptAnchorsToSentMessageNotLaterMessage(t *testing.T) {
	m := newBroadcastModel(t)
	m = sendBroadcast(t, m, "Evening check-in", "m1", "B1")

	// A later, unrelated broadcast from Bob arrives before his receipt does.
	later := time.Now().Unix() + 100
	updated, _ := m.Update(MsgReceived{Msg: bramble.Message{
		From: "BBBB0002", Broadcast: true, Text: "loud and clear", Timestamp: later,
	}})
	m = updated.(Model)

	// Now Bob's delivery receipt for our earlier broadcast lands.
	m = deliver(t, m, "B1", "BBBB0002")

	di := deliveryLineIndex(m, "BBBB0002")
	if di < 0 {
		t.Fatalf("expected a delivery line for the receipt, lines: %v", plainLines(m))
	}
	if di == 0 {
		t.Fatalf("delivery line has no message above it to anchor to")
	}
	prev := m.scroll.lines[di-1]
	if prev.Kind != LineChatOut || !strings.Contains(prev.Text, "Evening check-in") {
		t.Fatalf("receipt not anchored to our sent broadcast; line above receipt = %q", stripAnsi(prev.Text))
	}
	if strings.Contains(prev.Text, "loud and clear") {
		t.Fatalf("receipt mis-anchored to the later unrelated message")
	}
}

// Symptom 2 (aggregation): multiple recipients confirming one broadcast fold
// into a single receipt line that updates in place.
func TestReceiptsAggregateOntoOneLine(t *testing.T) {
	m := newBroadcastModel(t)
	m = sendBroadcast(t, m, "Evening check-in", "m1", "B1")

	m = deliver(t, m, "B1", "BBBB0002") // Bob
	if got := countDeliveryLines(m); got != 1 {
		t.Fatalf("after first receipt expected 1 delivery line, got %d", got)
	}
	m = deliver(t, m, "B1", "CCCC0003") // Lily, later

	if got := countDeliveryLines(m); got != 1 {
		t.Fatalf("expected receipts to aggregate onto one line, got %d lines", got)
	}
	di := deliveryLineIndex(m, "BBBB0002")
	if di < 0 {
		t.Fatalf("no delivery line found")
	}
	line := m.scroll.lines[di].Text
	if !strings.Contains(line, "BBBB0002") || !strings.Contains(line, "CCCC0003") {
		t.Fatalf("expected both recipients on one line, got %q", stripAnsi(line))
	}
}

// Symptom 2 (late receipt for an earlier interleaved broadcast): with two
// broadcasts on screen, a late receipt for the first must update the first
// broadcast's line, not append onto the newer one.
func TestInterleavedBroadcastsAnchorIndependently(t *testing.T) {
	m := newBroadcastModel(t)
	m = sendBroadcast(t, m, "first broadcast", "m1", "B1")
	m = sendBroadcast(t, m, "second broadcast", "m2", "B2")

	// Bob confirms the SECOND broadcast first.
	m = deliver(t, m, "B2", "BBBB0002")
	// Then a late receipt for the FIRST broadcast arrives from Lily.
	m = deliver(t, m, "B1", "CCCC0003")

	if got := countDeliveryLines(m); got != 2 {
		t.Fatalf("expected 2 independent delivery lines, got %d", got)
	}

	firstReceipt := deliveryLineIndex(m, "CCCC0003")  // Lily, for B1
	secondReceipt := deliveryLineIndex(m, "BBBB0002") // Bob, for B2
	if firstReceipt < 0 || secondReceipt < 0 {
		t.Fatalf("missing a delivery line; lines: %v", plainLines(m))
	}

	// Each receipt must sit directly beneath its own broadcast.
	if p := m.scroll.lines[firstReceipt-1]; !strings.Contains(p.Text, "first broadcast") {
		t.Fatalf("late receipt for first broadcast anchored to wrong message: %q", stripAnsi(p.Text))
	}
	if p := m.scroll.lines[secondReceipt-1]; !strings.Contains(p.Text, "second broadcast") {
		t.Fatalf("receipt for second broadcast anchored to wrong message: %q", stripAnsi(p.Text))
	}
	// The late (first-broadcast) receipt must render above the second broadcast.
	if firstReceipt > secondReceipt {
		t.Fatalf("late receipt for earlier broadcast rendered below the newer broadcast")
	}
}

func plainLines(m Model) []string {
	out := make([]string, 0, len(m.scroll.lines))
	for _, l := range m.scroll.lines {
		out = append(out, stripAnsi(l.Text))
	}
	return out
}

// Live regression: the firmware's sendBroadcast response carries broadcast_id
// but no message_id. Receipts must still anchor via the broadcast id fallback.
func TestReceiptAnchorsWhenSendResultHasNoMessageID(t *testing.T) {
	m := newBroadcastModel(t)
	m = sendBroadcast(t, m, "Evening check-in", "", "B1") // msgID empty, live shape

	m = deliver(t, m, "B1", "BBBB0002")

	di := deliveryLineIndex(m, "BBBB0002")
	if di < 1 {
		t.Fatalf("expected an anchored delivery line, lines: %v", plainLines(m))
	}
	prev := m.scroll.lines[di-1]
	if prev.Kind != LineChatOut || !strings.Contains(prev.Text, "Evening check-in") {
		t.Fatalf("receipt not anchored beneath the broadcast, line above = %q", stripAnsi(prev.Text))
	}
}

// Live regression: a receipt that arrives before the send result is processed
// must render once the send result registers the correlation.
func TestReceiptArrivingBeforeSendResultStillRenders(t *testing.T) {
	m := newBroadcastModel(t)

	// Delivery event races ahead of the send RPC result.
	m = deliver(t, m, "B1", "BBBB0002")
	m = sendBroadcast(t, m, "Evening check-in", "", "B1")

	di := deliveryLineIndex(m, "BBBB0002")
	if di < 1 {
		t.Fatalf("expected the early receipt to render after registration, lines: %v", plainLines(m))
	}
	if prev := m.scroll.lines[di-1]; !strings.Contains(prev.Text, "Evening check-in") {
		t.Fatalf("early receipt anchored to wrong line: %q", stripAnsi(prev.Text))
	}
}

// Live regression: /clear must survive transcript rebuilds. A delivery repaint
// after /clear must not resurrect the cleared history.
func TestClearSurvivesDeliveryRepaint(t *testing.T) {
	m := newBroadcastModel(t)

	// Old session history in the store.
	old := time.Now().Unix() - 3600
	updated, _ := m.Update(MsgReceived{Msg: bramble.Message{From: "BBBB0002", Broadcast: true, Text: "stale history line", Timestamp: old}})
	m = updated.(Model)
	m = sendBroadcast(t, m, "old broadcast", "", "B0")

	// /clear through the real command path.
	m.cmdHandler.Execute(ParseCommand("/clear"))
	if got := m.scroll.LineCount(); got != 0 {
		t.Fatalf("expected empty scrollback after /clear, got %d lines", got)
	}

	// New broadcast after the clear, then a delivery event triggers a rebuild.
	m = sendBroadcast(t, m, "fresh broadcast", "", "B1")
	m = deliver(t, m, "B1", "BBBB0002")

	for _, l := range plainLines(m) {
		if strings.Contains(l, "stale history line") || strings.Contains(l, "old broadcast") {
			t.Fatalf("delivery repaint resurrected cleared history: %v", plainLines(m))
		}
	}
	di := deliveryLineIndex(m, "BBBB0002")
	if di < 1 {
		t.Fatalf("expected receipt beneath the fresh broadcast, lines: %v", plainLines(m))
	}
	if prev := m.scroll.lines[di-1]; !strings.Contains(prev.Text, "fresh broadcast") {
		t.Fatalf("receipt anchored to wrong line after clear: %q", stripAnsi(prev.Text))
	}
}

// /clear must also survive a conversation switch away and back.
func TestClearSurvivesConversationSwitch(t *testing.T) {
	m := newBroadcastModel(t)
	m = sendBroadcast(t, m, "old broadcast", "", "B0")

	m.cmdHandler.Execute(ParseCommand("/clear"))

	m.switchBuffer("dm:BBBB0002")
	m.switchBuffer("broadcast")

	for _, l := range plainLines(m) {
		if strings.Contains(l, "old broadcast") {
			t.Fatalf("conversation switch resurrected cleared history: %v", plainLines(m))
		}
	}
}
