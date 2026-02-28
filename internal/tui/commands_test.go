package tui

import (
	"strings"
	"testing"

	"github.com/justinlindh/bramble-cli/internal/tui/tabs"
	bramble "github.com/justinlindh/bramble-go"
)

type testResolver struct {
	reverse map[string]string
}

func (r testResolver) Resolve(addr string) string          { return addr }
func (r testResolver) GetAlias(addr string) (string, bool) { return "", false }
func (r testResolver) SetAlias(addr, alias string) error   { return nil }
func (r testResolver) ReverseLookup(name string) (string, bool) {
	v, ok := r.reverse[name]
	return v, ok
}

var _ tabs.PeerResolver = testResolver{}

func TestCommandHandlerMsgReturnsDirectSendAction(t *testing.T) {
	store := NewStore()
	sb := NewScrollback()
	h := NewCommandHandler(nil, store, &sb, testResolver{reverse: map[string]string{"alice": "A1B2C3D4"}})

	action := h.Execute(&Command{Name: "msg", Args: []string{"alice", "hello", "there"}})

	if action.SwitchBuffer != "" {
		t.Fatalf("expected no buffer switch, got %q", action.SwitchBuffer)
	}
	if action.SendTo != "A1B2C3D4" {
		t.Fatalf("expected SendTo=A1B2C3D4, got %q", action.SendTo)
	}
	if action.SendText != "hello there" {
		t.Fatalf("expected SendText=%q, got %q", "hello there", action.SendText)
	}
}

func TestCommandHandlerHelpIncludesMsg(t *testing.T) {
	store := NewStore()
	sb := NewScrollback()
	h := NewCommandHandler(nil, store, &sb, testResolver{})

	h.Execute(&Command{Name: "help"})

	conv := store.GetActiveConversation()
	found := false
	for _, line := range conv.Events {
		if strings.Contains(line.Text, "/msg <addr|name> <text>") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected /help output to include /msg usage")
	}
}

func TestCommandHandlerLocationIncludesOpenStreetMapLinks(t *testing.T) {
	store := NewStore()
	sb := NewScrollback()
	h := NewCommandHandler(nil, store, &sb, testResolver{})

	store.UpdateOwnGPS(bramble.GpsEvent{Valid: true, Lat: 12.345678, Lon: -98.765432, AltM: 836, Sats: 12})
	store.UpdatePeerLocations([]bramble.LocationPeer{{
		Addr:     "ABCD1234",
		Position: &bramble.Position{Lat: 12.345670, Lon: -98.765440},
	}})

	h.Execute(&Command{Name: "location"})

	conv := store.GetActiveConversation()
	var lines []string
	for _, line := range conv.Events {
		lines = append(lines, line.Text)
	}
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "https://www.openstreetmap.org/?mlat=12.345678&mlon=-98.765432#map=17/12.345678/-98.765432") {
		t.Fatalf("expected own GPS OSM link in /location output, got:\n%s", joined)
	}
	if !strings.Contains(joined, "https://www.openstreetmap.org/?mlat=12.345670&mlon=-98.765440#map=17/12.345670/-98.765440") {
		t.Fatalf("expected peer OSM link in /location output, got:\n%s", joined)
	}
}
