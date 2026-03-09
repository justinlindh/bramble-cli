package tui

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	bramble "github.com/justinlindh/bramble-go"
)

func newTestMsgDB(t *testing.T) *MsgDB {
	t.Helper()
	m, err := NewMsgDB(":memory:")
	if err != nil {
		t.Fatalf("NewMsgDB failed: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
	m.SetNodeAddr("NODE1")
	return m
}

func mustUpsert(t *testing.T, m *MsgDB, msg StoredMessage) {
	t.Helper()
	if err := m.UpsertMessage(msg); err != nil {
		t.Fatalf("UpsertMessage(%s) failed: %v", msg.ID, err)
	}
}

func TestMsgDB_DefaultDBPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := DefaultDBPath()
	if err != nil {
		t.Fatalf("DefaultDBPath failed: %v", err)
	}
	want := filepath.Join(home, ".local", "share", "bramble", "messages.db")
	if got != want {
		t.Fatalf("DefaultDBPath = %q, want %q", got, want)
	}
}

func TestMsgDB_NewAndMigrateIdempotent(t *testing.T) {
	m := newTestMsgDB(t)

	// Re-running migration should be safe.
	if err := m.migrate(); err != nil {
		t.Fatalf("migrate re-run failed: %v", err)
	}

	// Verify schema objects exist.
	wantNames := map[string]bool{
		"messages":      false,
		"peer_aliases":  false,
		"idx_conv":      false,
		"idx_timestamp": false,
	}
	rows, err := m.db.Query(`SELECT name FROM sqlite_master WHERE name IN ('messages','peer_aliases','idx_conv','idx_timestamp')`)
	if err != nil {
		t.Fatalf("schema query failed: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("rows.Scan failed: %v", err)
		}
		wantNames[name] = true
	}
	for name, found := range wantNames {
		if !found {
			t.Fatalf("expected schema object %q to exist", name)
		}
	}
}

func TestMsgDB_StoredMessageConversions(t *testing.T) {
	in := StoredMessage{
		ID:        "m1",
		NodeAddr:  "SELF",
		ConvID:    "dm:PEER",
		Direction: "out",
		Sender:    "SELF",
		Text:      "hello",
		Timestamp: 100,
	}
	bm := in.ToBramble()
	if bm.From != "SELF" || bm.To != "PEER" || bm.MsgID != "m1" {
		t.Fatalf("unexpected ToBramble(out): %+v", bm)
	}

	in.Direction = "in"
	in.Sender = "PEER"
	bm = in.ToBramble()
	if bm.From != "PEER" || bm.To != "PEER" {
		t.Fatalf("unexpected ToBramble(in): %+v", bm)
	}

	from := StoredMessageFromBramble(bramble.Message{From: "P", Text: "x", Timestamp: 0}, "SELF", "dm:P", "in")
	if from.ID == "" || from.Timestamp <= 0 || from.Status != "sent" || from.CreatedAt <= 0 {
		t.Fatalf("StoredMessageFromBramble did not populate defaults: %+v", from)
	}
}

func TestMsgDB_UpsertInsertUpdateStatusAndRelayPath(t *testing.T) {
	m := newTestMsgDB(t)

	base := StoredMessage{
		ID: "id-1", NodeAddr: "NODE1", ConvID: "dm:PEER", Direction: "out",
		Sender: "NODE1", Text: "first", Timestamp: 10, Status: "sent", Channel: 1, CreatedAt: 10,
	}
	mustUpsert(t, m, base)

	// Conflict update should only mutate status/relay_path; text should remain original.
	update := base
	update.Text = "new text should be ignored"
	update.Status = "delivered"
	update.RelayPath = []string{"A", "B"}
	mustUpsert(t, m, update)

	msgs, err := m.LoadConversation("dm:PEER", 10, 0)
	if err != nil {
		t.Fatalf("LoadConversation failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	got := msgs[0]
	if got.Text != "first" {
		t.Fatalf("text changed unexpectedly: got %q", got.Text)
	}
	if got.Status != "delivered" {
		t.Fatalf("status not updated: got %q", got.Status)
	}
	if len(got.RelayPath) != 2 || got.RelayPath[0] != "A" {
		t.Fatalf("relay path not updated: %+v", got.RelayPath)
	}

	if err := m.UpdateStatus("id-1", "read"); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}
	if err := m.UpdateRelayPath("id-1", []string{"X"}); err != nil {
		t.Fatalf("UpdateRelayPath failed: %v", err)
	}
	msgs, _ = m.LoadConversation("dm:PEER", 10, 0)
	if msgs[0].Status != "read" || len(msgs[0].RelayPath) != 1 || msgs[0].RelayPath[0] != "X" {
		t.Fatalf("update helpers not applied: %+v", msgs[0])
	}
}

func TestMsgDB_LoadConversationPaginationOrderingAndEmpty(t *testing.T) {
	m := newTestMsgDB(t)

	empty, err := m.LoadConversation("dm:none", 5, 0)
	if err != nil {
		t.Fatalf("LoadConversation empty failed: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty conversation, got %d", len(empty))
	}

	for i := 1; i <= 6; i++ {
		mustUpsert(t, m, StoredMessage{
			ID: fmt.Sprintf("m-%d", i), NodeAddr: "NODE1", ConvID: "dm:PEER", Direction: "in",
			Sender: "PEER", Text: fmt.Sprintf("t-%d", i), Timestamp: int64(i), Status: "sent", CreatedAt: int64(i),
		})
	}
	msgs, err := m.LoadConversation("dm:PEER", 3, 0)
	if err != nil {
		t.Fatalf("LoadConversation failed: %v", err)
	}
	if len(msgs) != 3 || msgs[0].Timestamp != 4 || msgs[2].Timestamp != 6 {
		t.Fatalf("unexpected recent page order: %+v", msgs)
	}

	older, err := m.LoadConversation("dm:PEER", 2, 4)
	if err != nil {
		t.Fatalf("LoadConversation beforeTimestamp failed: %v", err)
	}
	if len(older) != 2 || older[0].Timestamp != 2 || older[1].Timestamp != 3 {
		t.Fatalf("unexpected older page: %+v", older)
	}
}

func TestMsgDB_LoadConversationsAndRecent(t *testing.T) {
	m := newTestMsgDB(t)

	mustUpsert(t, m, StoredMessage{ID: "a1", NodeAddr: "NODE1", ConvID: "broadcast", Direction: "in", Sender: "P1", Text: "b", Timestamp: 1, Status: "sent", CreatedAt: 1})
	mustUpsert(t, m, StoredMessage{ID: "a2", NodeAddr: "NODE1", ConvID: "ch:dev", Direction: "in", Sender: "P2", Text: "c", Timestamp: 2, Status: "sent", CreatedAt: 2})
	mustUpsert(t, m, StoredMessage{ID: "a3", NodeAddr: "NODE1", ConvID: "dm:peer", Direction: "in", Sender: "P3", Text: "d", Timestamp: 3, Status: "sent", CreatedAt: 3})

	summaries, err := m.LoadConversations()
	if err != nil {
		t.Fatalf("LoadConversations failed: %v", err)
	}
	if len(summaries) != 3 {
		t.Fatalf("expected 3 conversation summaries, got %d", len(summaries))
	}
	if summaries[0].ID != "dm:peer" || summaries[0].LastTimestamp != 3 {
		t.Fatalf("unexpected summary ordering: %+v", summaries)
	}

	labels := map[string]string{}
	for _, s := range summaries {
		labels[s.ID] = s.Label
	}
	if labels["broadcast"] != "Broadcast" || labels["dm:peer"] != "peer" || labels["ch:dev"] != "ch:dev" {
		t.Fatalf("unexpected labels: %+v", labels)
	}

	recent, err := m.LoadRecent(2)
	if err != nil {
		t.Fatalf("LoadRecent failed: %v", err)
	}
	if len(recent) != 2 || recent[0].Timestamp != 2 || recent[1].Timestamp != 3 {
		t.Fatalf("unexpected recent ordering: %+v", recent)
	}
}

func TestMsgDB_AliasCRUDAndDuplicates(t *testing.T) {
	m := newTestMsgDB(t)

	if err := m.upsertAlias("NODE1", "P1", "alice"); err != nil {
		t.Fatalf("upsertAlias insert failed: %v", err)
	}
	if err := m.upsertAlias("NODE1", "P1", "alice2"); err != nil {
		t.Fatalf("upsertAlias update failed: %v", err)
	}
	if err := m.upsertAlias("NODE1", "P2", "bob"); err != nil {
		t.Fatalf("upsertAlias second peer failed: %v", err)
	}

	aliases, err := m.loadAliases("NODE1")
	if err != nil {
		t.Fatalf("loadAliases failed: %v", err)
	}
	if len(aliases) != 2 || aliases["P1"] != "alice2" {
		t.Fatalf("unexpected aliases: %+v", aliases)
	}

	if err := m.deleteAlias("NODE1", "P1"); err != nil {
		t.Fatalf("deleteAlias failed: %v", err)
	}
	aliases, _ = m.loadAliases("NODE1")
	if _, ok := aliases["P1"]; ok {
		t.Fatalf("expected alias P1 to be deleted, aliases=%+v", aliases)
	}
}

func TestMsgDB_ConcurrentAccess(t *testing.T) {
	m := newTestMsgDB(t)

	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("cid-%d", i%8) // intentional duplicates
			_ = m.UpsertMessage(StoredMessage{
				ID: id, NodeAddr: "NODE1", ConvID: "dm:C", Direction: "in",
				Sender: "C", Text: fmt.Sprintf("msg-%d", i), Timestamp: int64(i + 1),
				Status: "sent", RelayPath: []string{fmt.Sprintf("h-%d", i)}, CreatedAt: time.Now().Unix(),
			})
		}(i)
	}
	wg.Wait()

	msgs, err := m.LoadConversation("dm:C", 100, 0)
	if err != nil {
		t.Fatalf("LoadConversation after concurrency failed: %v", err)
	}
	if len(msgs) == 0 || len(msgs) > 8 {
		t.Fatalf("unexpected message count after concurrent upserts: %d", len(msgs))
	}
	// Ensure ordering is ascending.
	ts := make([]int64, len(msgs))
	for i := range msgs {
		ts[i] = msgs[i].Timestamp
	}
	if !sort.SliceIsSorted(ts, func(i, j int) bool { return ts[i] < ts[j] }) {
		t.Fatalf("timestamps not sorted ascending: %v", ts)
	}
}

func TestMsgDB_CloseAndReopenFileDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "messages.db")
	m, err := NewMsgDB(path)
	if err != nil {
		t.Fatalf("NewMsgDB(file) failed: %v", err)
	}
	m.SetNodeAddr("NODE1")
	mustUpsert(t, m, StoredMessage{ID: "x", NodeAddr: "NODE1", ConvID: "dm:z", Direction: "in", Sender: "z", Text: "hi", Timestamp: 1, Status: "sent", CreatedAt: 1})
	if err := m.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	m2, err := NewMsgDB(path)
	if err != nil {
		t.Fatalf("reopen failed: %v", err)
	}
	defer m2.Close()
	m2.SetNodeAddr("NODE1")
	msgs, err := m2.LoadConversation("dm:z", 10, 0)
	if err != nil {
		t.Fatalf("LoadConversation after reopen failed: %v", err)
	}
	if len(msgs) != 1 || msgs[0].ID != "x" {
		t.Fatalf("expected persisted message after reopen, got %+v", msgs)
	}
}

func TestMsgDB_ConvIDHelpers(t *testing.T) {
	cases := []struct{ in, label, to string }{
		{"broadcast", "Broadcast", "broadcast"},
		{"", "", "broadcast"},
		{"ch:ops", "ch:ops", "ch:ops"},
		{"dm:abc", "abc", "abc"},
		{"raw", "raw", "raw"},
	}
	for _, tc := range cases {
		if got := convIDToLabel(tc.in); got != tc.label {
			t.Fatalf("convIDToLabel(%q)=%q want %q", tc.in, got, tc.label)
		}
		if got := convIDToTo(tc.in); got != tc.to {
			t.Fatalf("convIDToTo(%q)=%q want %q", tc.in, got, tc.to)
		}
	}
}

func TestMsgDB_NewMsgDB_InvalidPath(t *testing.T) {
	file := filepath.Join(t.TempDir(), "parent-file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file failed: %v", err)
	}
	_, err := NewMsgDB(filepath.Join(file, "messages.db"))
	if err == nil {
		t.Fatalf("expected NewMsgDB to fail for invalid parent path")
	}
}

// Keep compile-time checks for raw db access helpers used in tests.
var _ *sql.DB
