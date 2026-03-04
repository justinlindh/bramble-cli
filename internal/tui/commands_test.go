package tui

import (
	"strings"
	"testing"

	bramble "github.com/justinlindh/bramble-go"

	"github.com/justinlindh/bramble-cli/internal/tui/tabs"
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

func TestCommandHandlerCriticalReturnsSendAction(t *testing.T) {
	store := NewStore()
	sb := NewScrollback()
	h := NewCommandHandler(nil, store, &sb, testResolver{})

	action := h.Execute(&Command{Name: "critical", Args: []string{"priority", "message"}})

	if action.SendText != "priority message" {
		t.Fatalf("expected SendText=%q, got %q", "priority message", action.SendText)
	}
	if !action.SendCritical {
		t.Fatalf("expected SendCritical=true")
	}
}

func TestCommandHandlerHelpIncludesCritical(t *testing.T) {
	store := NewStore()
	sb := NewScrollback()
	h := NewCommandHandler(nil, store, &sb, testResolver{})

	h.Execute(&Command{Name: "help"})

	conv := store.GetActiveConversation()
	found := false
	for _, line := range conv.Events {
		if strings.Contains(line.Text, "/critical <text>") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected /help output to include /critical usage")
	}
}

func TestCommandHandlerSlapReturnsActionMessage(t *testing.T) {
	store := NewStore()
	sb := NewScrollback()
	h := NewCommandHandler(nil, store, &sb, testResolver{})

	action := h.Execute(&Command{Name: "slap", Args: []string{"NodeName"}})

	if action.SendText != "\x01ACTION slaps NodeName around a bit with a large trout\x01" {
		t.Fatalf("expected slap action payload, got %q", action.SendText)
	}
}

func TestCommandHandlerHelpIncludesSlap(t *testing.T) {
	store := NewStore()
	sb := NewScrollback()
	h := NewCommandHandler(nil, store, &sb, testResolver{})

	h.Execute(&Command{Name: "help"})

	conv := store.GetActiveConversation()
	found := false
	for _, line := range conv.Events {
		if strings.Contains(line.Text, "/slap <target>") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected /help output to include /slap usage")
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

	ownURL := "https://www.openstreetmap.org/?mlat=12.345678&mlon=-98.765432#map=17/12.345678/-98.765432"
	peerURL := "https://www.openstreetmap.org/?mlat=12.345670&mlon=-98.765440#map=17/12.345670/-98.765440"

	if !strings.Contains(joined, termLink(ownURL, ownURL)) {
		t.Fatalf("expected own GPS OSC8 OSM link in /location output, got:\n%s", joined)
	}
	if !strings.Contains(joined, termLink(peerURL, peerURL)) {
		t.Fatalf("expected peer OSC8 OSM link in /location output, got:\n%s", joined)
	}
}

func TestCommandHandlerMouseToggleActions(t *testing.T) {
	store := NewStore()
	sb := NewScrollback()
	h := NewCommandHandler(nil, store, &sb, testResolver{})

	action := h.Execute(&Command{Name: "mouse"})
	if action.SetMouseEnabled == nil || *action.SetMouseEnabled {
		t.Fatalf("expected /mouse to toggle from on to off")
	}

	action = h.Execute(&Command{Name: "mouse", Args: []string{"on"}})
	if action.SetMouseEnabled == nil || !*action.SetMouseEnabled {
		t.Fatalf("expected /mouse on to set enabled")
	}

	action = h.Execute(&Command{Name: "mouse", Args: []string{"off"}})
	if action.SetMouseEnabled == nil || *action.SetMouseEnabled {
		t.Fatalf("expected /mouse off to set disabled")
	}
}

func TestCommandHandlerHelpIncludesMouse(t *testing.T) {
	store := NewStore()
	sb := NewScrollback()
	h := NewCommandHandler(nil, store, &sb, testResolver{})

	h.Execute(&Command{Name: "help"})

	conv := store.GetActiveConversation()
	joined := ""
	for _, line := range conv.Events {
		joined += line.Text + "\n"
	}
	if !strings.Contains(joined, "/mouse [on|off]") {
		t.Fatalf("expected /help output to include /mouse usage")
	}
	if !strings.Contains(joined, "Shift+click/drag") {
		t.Fatalf("expected /help output to mention Shift+click/drag bypass")
	}
}
