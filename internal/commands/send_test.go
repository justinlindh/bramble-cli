package commands

import (
	"strings"
	"testing"
)

func TestNewSendCmd_ArgValidation(t *testing.T) {
	t.Parallel()

	cmd := newSendCmd()
	cmd.SetArgs([]string{"only-one-arg"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing message argument")
	}
	if !strings.Contains(err.Error(), "accepts 2 arg(s), received 1") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunSend_InvalidAddress(t *testing.T) {
	t.Parallel()

	err := runSend(newSendCmd(), []string{"not-an-address", "hello"})
	if err == nil {
		t.Fatal("expected error for invalid address")
	}
	if !strings.Contains(err.Error(), `invalid address "not-an-address"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}
