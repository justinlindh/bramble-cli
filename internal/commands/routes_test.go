package commands

import "testing"

func TestNewRoutesCmd_Config(t *testing.T) {
	t.Parallel()

	cmd := newRoutesCmd()
	if cmd.Use != "routes" {
		t.Fatalf("unexpected use: %q", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Fatal("expected RunE handler")
	}
}
