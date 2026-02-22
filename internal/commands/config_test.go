package commands

import (
	"strings"
	"testing"
)

func TestNewConfigCmd_HasExpectedSubcommands(t *testing.T) {
	t.Parallel()

	cmd := newConfigCmd()
	want := []string{"get", "set-name", "set-radio"}

	for _, name := range want {
		if got, _, err := cmd.Find([]string{name}); err != nil || got == nil || got.Name() != name {
			t.Fatalf("expected subcommand %q to exist (got=%v, err=%v)", name, got, err)
		}
	}
}

func TestRunConfigSetName_TooLong(t *testing.T) {
	t.Parallel()

	err := runConfigSetName(newConfigSetNameCmd(), []string{"toolongggg"})
	if err == nil {
		t.Fatal("expected too-long name error")
	}
	if !strings.Contains(err.Error(), `is too long (max 8 characters)`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunConfigSetRadio_NoFlagsChanged(t *testing.T) {
	t.Parallel()

	cmd := newConfigSetRadioCmd()
	err := runConfigSetRadio(cmd, nil)
	if err == nil {
		t.Fatal("expected error when no radio flags are provided")
	}
	if !strings.Contains(err.Error(), "specify at least one radio parameter") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewConfigSetNameCmd_ArgValidation(t *testing.T) {
	t.Parallel()

	cmd := newConfigSetNameCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected argument validation error")
	}
	if !strings.Contains(err.Error(), "accepts 1 arg(s), received 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}
