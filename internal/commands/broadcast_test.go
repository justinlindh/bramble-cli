package commands

import (
	"testing"
)

func TestNewBroadcastCmd_Config(t *testing.T) {
	t.Parallel()

	cmd := newBroadcastCmd()
	if cmd.Use != "broadcast <message>" {
		t.Fatalf("unexpected use: %q", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Fatal("expected RunE handler")
	}
	if err := cmd.Args(cmd, []string{}); err == nil {
		t.Fatal("expected arg validation error for missing message")
	}
	if err := cmd.Args(cmd, []string{"hello"}); err != nil {
		t.Fatalf("expected single arg to pass, got %v", err)
	}

	ch, err := cmd.Flags().GetInt("channel")
	if err != nil {
		t.Fatalf("channel flag missing: %v", err)
	}
	if ch != -1 {
		t.Fatalf("channel default = %d, want -1", ch)
	}

	wait, err := cmd.Flags().GetInt("wait-delivery")
	if err != nil {
		t.Fatalf("wait-delivery flag missing: %v", err)
	}
	if wait != 0 {
		t.Fatalf("wait-delivery default = %d, want 0", wait)
	}
}
