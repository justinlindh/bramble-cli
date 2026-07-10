package tui

import (
	"testing"

	bramble "github.com/justinlindh/bramble-go"
)

const selfAddr = "CAFEBABE"

// The live bramble.onMessage notification carries Broadcast/Channel but no
// "to". These cases pin the regression: an incoming DM (empty To, Broadcast
// false) must classify as a DM, not Broadcast.
func TestClassifyMessageConvID(t *testing.T) {
	cases := []struct {
		name string
		msg  bramble.Message
		want string
	}{
		{"live DM (no to, broadcast false)", bramble.Message{From: "DEADBEEF", Broadcast: false}, "dm:DEADBEEF"},
		{"live broadcast", bramble.Message{From: "DEADBEEF", Broadcast: true}, "broadcast"},
		{"live channel", bramble.Message{From: "DEADBEEF", Channel: 3}, "ch:3"},
		{"outgoing DM (from self, to peer)", bramble.Message{From: selfAddr, To: "DEADBEEF"}, "dm:DEADBEEF"},
		{"echoed broadcast (to=broadcast)", bramble.Message{From: selfAddr, To: "broadcast"}, "broadcast"},
		{"legacy broadcast (to=FFFFFFFF)", bramble.Message{From: "DEADBEEF", To: "FFFFFFFF"}, "broadcast"},
		{"echoed/history channel (to=ch:2)", bramble.Message{From: selfAddr, To: "ch:2"}, "ch:2"},
		{"history DM (to=peer)", bramble.Message{From: "DEADBEEF", To: selfAddr}, "dm:DEADBEEF"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyMessageConvID(tc.msg, selfAddr); got != tc.want {
				t.Fatalf("ClassifyMessageConvID = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConvIDForMessage(t *testing.T) {
	s := NewStore()
	s.Identity = &bramble.IdentityResponse{Address: selfAddr}

	cases := []struct {
		name string
		msg  bramble.Message
		want string
	}{
		{"live DM (no to)", bramble.Message{From: "DEADBEEF", Broadcast: false}, "dm:DEADBEEF"},
		{"live broadcast", bramble.Message{From: "DEADBEEF", Broadcast: true}, "broadcast"},
		{"live channel", bramble.Message{From: "DEADBEEF", Channel: 5}, "ch:5"},
		{"outgoing DM", bramble.Message{From: selfAddr, To: "DEADBEEF"}, "dm:DEADBEEF"},
		{"echoed broadcast", bramble.Message{From: selfAddr, To: "broadcast"}, "broadcast"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.convIDForMessage(tc.msg); got != tc.want {
				t.Fatalf("convIDForMessage = %q, want %q", got, tc.want)
			}
		})
	}
}
