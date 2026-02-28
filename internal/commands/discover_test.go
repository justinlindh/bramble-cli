package commands

import (
	"testing"
	"time"
)

func TestNewDiscoverCmd_Config(t *testing.T) {
	t.Parallel()

	cmd := newDiscoverCmd()
	if cmd.Use != "discover" {
		t.Fatalf("unexpected use: %q", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Fatal("expected RunE handler")
	}

	timeoutFlag := cmd.Flags().Lookup("timeout")
	if timeoutFlag == nil {
		t.Fatal("expected timeout flag")
	}
	if timeoutFlag.DefValue != (3 * time.Second).String() {
		t.Fatalf("timeout default = %q, want %q", timeoutFlag.DefValue, (3 * time.Second).String())
	}
}
