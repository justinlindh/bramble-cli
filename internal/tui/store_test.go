package tui

import (
	"path/filepath"
	"testing"
	"time"

	bramble "github.com/justinlindh/bramble-go"
)

func newTestDB(t *testing.T) *MsgDB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "messages.db")
	db, err := NewMsgDB(dbPath)
	if err != nil {
		t.Fatalf("NewMsgDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetNodeAddr("SELF0001")
	return db
}

func waitFor(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met before timeout")
}

func TestStore_NewAndBasicUpdates(t *testing.T) {
	s := NewStore()
	if s.ActiveConvID != "broadcast" {
		t.Fatalf("expected default active conv broadcast, got %q", s.ActiveConvID)
	}
	if s.IsNewConversation("broadcast") {
		t.Fatalf("broadcast should exist by default")
	}
	if !s.IsNewConversation("dm:ABCD") {
		t.Fatalf("dm conversation should be new")
	}

	st := &bramble.StatusResponse{Address: "SELF0001", RadioOk: true}
	s.UpdateStatus(st)
	if s.Status != st {
		t.Fatalf("status not updated")
	}

	id := &bramble.IdentityResponse{Address: "SELF0001"}
	s.UpdateIdentity(id)
	if s.Identity != id {
		t.Fatalf("identity not updated")
	}

	routes := []bramble.Route{{Dest: "A", NextHop: "B", HopCount: 1}}
	s.UpdateRoutes(routes)
	if len(s.Routes) != 1 || s.Routes[0].Dest != "A" {
		t.Fatalf("routes not updated: %#v", s.Routes)
	}

	air := &bramble.AirtimeStats{}
	s.UpdateAirtime(air)
	if s.Airtime != air {
		t.Fatalf("airtime not updated")
	}

	peers := []bramble.LocationPeer{{Addr: "BEEF", Name: "peer-1"}}
	s.UpdatePeerLocations(peers)
	gotPeers := s.GetPeerLocations()
	if len(gotPeers) != 1 || gotPeers[0].Addr != "BEEF" {
		t.Fatalf("peer locations mismatch: %#v", gotPeers)
	}
	gotPeers[0].Addr = "MUTATED"
	if s.GetPeerLocations()[0].Addr != "BEEF" {
		t.Fatalf("GetPeerLocations should return copy")
	}

	if s.GetOwnGPS() != nil {
		t.Fatalf("expected nil GPS initially")
	}
	gps := bramble.GpsEvent{Event: "gps", Valid: true, Lat: 1.23, Lon: 4.56}
	s.UpdateOwnGPS(gps)
	gotGPS := s.GetOwnGPS()
	if gotGPS == nil || gotGPS.Lat != 1.23 || gotGPS.Lon != 4.56 {
		t.Fatalf("gps mismatch: %#v", gotGPS)
	}
	gotGPS.Lat = 99
	if s.GetOwnGPS().Lat != 1.23 {
		t.Fatalf("GetOwnGPS should return copy")
	}
}

func TestStore_UpdateNeighborsUpdatesResolver(t *testing.T) {
	s := NewStore()
	s.Resolver = NewNameResolver(nil, "SELF0001")

	neighbors := []bramble.Neighbor{{Address: "A1B2C3D4", Name: "Lily"}}
	s.UpdateNeighbors(neighbors)

	if len(s.Neighbors) != 1 || s.Neighbors[0].Address != "A1B2C3D4" {
		t.Fatalf("neighbors not updated: %#v", s.Neighbors)
	}
	if got := s.Resolver.Resolve("A1B2C3D4"); got != "Lily" {
		t.Fatalf("resolver not updated, got %q", got)
	}
}

func TestStore_AddMessageConversationsAndSnapshots(t *testing.T) {
	s := NewStore()
	s.UpdateIdentity(&bramble.IdentityResponse{Address: "SELF0001"})
	s.Resolver = NewNameResolver(nil, "SELF0001")
	s.Resolver.UpdateFirmwareNames([]bramble.Neighbor{{Address: "PEER0001", Name: "Buddy"}})

	// incoming DM creates dm:peer conv and increments unread when inactive
	s.AddMessage(bramble.Message{From: "PEER0001", To: "SELF0001", Text: "hi", Timestamp: 111})
	if s.IsNewConversation("dm:PEER0001") {
		t.Fatalf("dm conversation should exist")
	}

	// outgoing DM routes by recipient when from self
	s.AddMessage(bramble.Message{From: "SELF0001", To: "PEER0002", Text: "yo", Timestamp: 112})
	// channel and broadcast routes
	s.AddMessage(bramble.Message{From: "PEER0003", To: "ch:7", Text: "chan", Timestamp: 113})
	s.AddMessage(bramble.Message{From: "PEER0004", To: "broadcast", Text: "all", Timestamp: 114})

	// zero timestamp normalized
	s.AddMessage(bramble.Message{From: "PEER0005", To: "broadcast", Text: "now"})
	active := s.GetActiveConversation()
	if active == nil || len(active.Messages) < 2 {
		t.Fatalf("expected broadcast messages, got %#v", active)
	}
	if active.Messages[len(active.Messages)-1].Timestamp <= 0 {
		t.Fatalf("expected normalized timestamp")
	}

	s.AddConversationLine("dm:PEER0001", ScrollLine{Text: "event"})
	convs := s.GetConversations()
	if len(convs) < 4 {
		t.Fatalf("expected multiple conversations, got %d", len(convs))
	}

	var dm *Conversation
	for _, c := range convs {
		if c.ID == "dm:PEER0001" {
			dm = c
			break
		}
	}
	if dm == nil {
		t.Fatalf("dm conversation missing")
	}
	if dm.Label != "@Buddy(0001)" {
		t.Fatalf("expected resolved DM label, got %q", dm.Label)
	}
	if dm.Unread == 0 {
		t.Fatalf("expected unread increment for inactive DM conv")
	}
	if len(dm.Events) != 1 || dm.Events[0].Text != "event" {
		t.Fatalf("expected stored conversation line")
	}

	// snapshots are copies
	dm.Messages[0].Text = "mutated"
	dm.Events[0].Text = "mutated"
	for _, c := range s.GetConversations() {
		if c.ID == "dm:PEER0001" {
			if c.Messages[0].Text == "mutated" || c.Events[0].Text == "mutated" {
				t.Fatalf("GetConversations should return deep-ish copies")
			}
		}
	}

	s.SetActiveConv("dm:PEER0001")
	if got := s.GetActiveConversation(); got == nil || got.ID != "dm:PEER0001" || got.Unread != 0 {
		t.Fatalf("active conversation not switched/cleared: %#v", got)
	}
}

func TestStore_LoadOlderMessagesAndAck(t *testing.T) {
	db := newTestDB(t)
	s := NewStore()
	s.SetMsgDB(db)
	s.UpdateIdentity(&bramble.IdentityResponse{Address: "SELF0001"})

	seed := []StoredMessage{
		{ID: "m1", NodeAddr: "SELF0001", ConvID: "dm:PEER", Direction: "in", Sender: "PEER", Text: "1", Timestamp: 100, Status: "sent", CreatedAt: 100},
		{ID: "m2", NodeAddr: "SELF0001", ConvID: "dm:PEER", Direction: "in", Sender: "PEER", Text: "2", Timestamp: 200, Status: "sent", CreatedAt: 200},
		{ID: "m3", NodeAddr: "SELF0001", ConvID: "dm:PEER", Direction: "in", Sender: "PEER", Text: "3", Timestamp: 300, Status: "sent", CreatedAt: 300},
	}
	for _, m := range seed {
		if err := db.UpsertMessage(m); err != nil {
			t.Fatalf("seed UpsertMessage: %v", err)
		}
	}

	loaded := s.LoadOlderMessages("dm:PEER", 250, 10)
	if len(loaded) != 2 || loaded[0].MsgID != "m1" || loaded[1].MsgID != "m2" {
		t.Fatalf("unexpected paged load: %#v", loaded)
	}

	latestTwo := s.LoadOlderMessages("dm:PEER", 0, 2)
	if len(latestTwo) != 2 || latestTwo[0].MsgID != "m2" || latestTwo[1].MsgID != "m3" {
		t.Fatalf("unexpected latest load: %#v", latestTwo)
	}

	if got := s.LoadOlderMessages("missing", 0, 5); len(got) != 0 {
		t.Fatalf("expected empty for missing conv, got %#v", got)
	}

	// UpdateAck persists status asynchronously when packet id is present.
	s.UpdateAck(bramble.Ack{PacketID: "m2", Status: "delivered"})
	waitFor(t, func() bool {
		msgs, err := db.LoadConversation("dm:PEER", 10, 0)
		if err != nil {
			return false
		}
		for _, m := range msgs {
			if m.ID == "m2" {
				return m.Status == "delivered"
			}
		}
		return false
	})

	// no panic/no-op paths
	s.UpdateAck(bramble.Ack{PacketID: "", Status: "ignored"})
	s2 := NewStore()
	s2.UpdateAck(bramble.Ack{PacketID: "m2", Status: "ignored"})

	// nil DB path for load
	s3 := NewStore()
	if got := s3.LoadOlderMessages("dm:PEER", 0, 10); got != nil {
		t.Fatalf("expected nil when db missing, got %#v", got)
	}
}
