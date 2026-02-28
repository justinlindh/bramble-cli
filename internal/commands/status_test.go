package commands

import "testing"

func TestNewStatusCmd_Config(t *testing.T) {
	t.Parallel()

	cmd := newStatusCmd()
	if cmd.Use != "status" {
		t.Fatalf("unexpected use: %q", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Fatal("expected RunE handler")
	}
}

func TestBoolStr(t *testing.T) {
	t.Parallel()

	if got := boolStr(true, "yes", "no"); got != "yes" {
		t.Fatalf("boolStr(true) = %q, want yes", got)
	}
	if got := boolStr(false, "yes", "no"); got != "no" {
		t.Fatalf("boolStr(false) = %q, want no", got)
	}
}
