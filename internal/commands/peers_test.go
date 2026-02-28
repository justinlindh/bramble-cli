package commands

import "testing"

func TestNewPeersCmd_Config(t *testing.T) {
	t.Parallel()

	cmd := newPeersCmd()
	if cmd.Use != "peers" {
		t.Fatalf("unexpected use: %q", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Fatal("expected RunE handler")
	}
}
