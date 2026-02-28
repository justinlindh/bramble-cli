package commands

import "testing"

func TestNewProbeCmd_Config(t *testing.T) {
	t.Parallel()

	cmd := newProbeCmd()
	if cmd.Use != "probe" {
		t.Fatalf("unexpected use: %q", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Fatal("expected RunE handler")
	}
}
