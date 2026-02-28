package commands

import "testing"

func TestNewRebootCmd_Config(t *testing.T) {
	t.Parallel()

	cmd := newRebootCmd()
	if cmd.Use != "reboot" {
		t.Fatalf("unexpected use: %q", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Fatal("expected RunE handler")
	}
}
