package commands

import (
	"testing"

	"github.com/justinlindh/bramble-go/transport"
)

func TestResolvedAuthToken_PrefersFlag(t *testing.T) {
	t.Setenv("BRAMBLE_TOKEN", "env-token")
	flagAuthToken = "flag-token"
	t.Cleanup(func() { flagAuthToken = "" })

	if got := resolvedAuthToken(); got != "flag-token" {
		t.Fatalf("expected flag token, got %q", got)
	}
}

func TestResolvedAuthToken_UsesEnvFallback(t *testing.T) {
	t.Setenv("BRAMBLE_TOKEN", "env-token")
	flagAuthToken = ""

	if got := resolvedAuthToken(); got != "env-token" {
		t.Fatalf("expected env token, got %q", got)
	}
}

func TestApplyAuthToken_SetsTransportField(t *testing.T) {
	flagAuthToken = ""
	t.Setenv("BRAMBLE_TOKEN", "env-token")

	w := transport.NewWebSocket("ws://example")
	applyAuthToken(w)
	if w.AuthToken() != "env-token" {
		t.Fatalf("expected env token on websocket, got %q", w.AuthToken())
	}

	s := transport.NewSerial("/dev/fake")
	applyAuthToken(s)
	if s.AuthToken() != "env-token" {
		t.Fatalf("expected env token on serial, got %q", s.AuthToken())
	}
}
