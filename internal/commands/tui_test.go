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

func TestTUICmd_InheritsTimeoutFlag(t *testing.T) {
	t.Parallel()

	cmd, _, err := rootCmd.Find([]string{"tui"})
	if err != nil {
		t.Fatalf("find tui command: %v", err)
	}

	if f := cmd.InheritedFlags().Lookup("timeout"); f == nil {
		t.Fatal("expected tui command to inherit --timeout flag")
	}
}
