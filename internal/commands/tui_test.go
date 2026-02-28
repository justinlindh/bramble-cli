package commands

import "testing"

func TestNewTUICmd_Config(t *testing.T) {
	t.Parallel()

	cmd := newTUICmd()
	if cmd.Use != "tui" {
		t.Fatalf("unexpected use: %q", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Fatal("expected RunE handler")
	}
}
