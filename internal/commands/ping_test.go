package commands

import "testing"

func TestNewPingCmd_Config(t *testing.T) {
	t.Parallel()

	cmd := newPingCmd()
	if cmd.Use != "ping" {
		t.Fatalf("unexpected use: %q", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Fatal("expected RunE handler")
	}
}
