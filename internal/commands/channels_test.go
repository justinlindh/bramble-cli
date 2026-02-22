package commands

import (
	"strings"
	"testing"
)

func TestNewChannelsCmd_HasExpectedSubcommands(t *testing.T) {
	t.Parallel()

	cmd := newChannelsCmd()
	want := []string{"list", "add", "remove", "set-default"}

	for _, name := range want {
		if got, _, err := cmd.Find([]string{name}); err != nil || got == nil || got.Name() != name {
			t.Fatalf("expected subcommand %q to exist (got=%v, err=%v)", name, got, err)
		}
	}
}

func TestNewChannelsAddCmd_ArgValidation(t *testing.T) {
	t.Parallel()

	cmd := newChannelsAddCmd()
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when no name is provided")
	}
	if !strings.Contains(err.Error(), "accepts between 1 and 2 arg(s), received 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunChannelsRemove_InvalidIndex(t *testing.T) {
	t.Parallel()

	err := runChannelsRemove(newChannelsRemoveCmd(), []string{"abc"})
	if err == nil {
		t.Fatal("expected invalid index error")
	}
	if !strings.Contains(err.Error(), `invalid channel index "abc": must be an integer`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunChannelsSetDefault_InvalidIndex(t *testing.T) {
	t.Parallel()

	err := runChannelsSetDefault(newChannelsSetDefaultCmd(), []string{"x"})
	if err == nil {
		t.Fatal("expected invalid index error")
	}
	if !strings.Contains(err.Error(), `invalid channel index "x": must be an integer`) {
		t.Fatalf("unexpected error: %v", err)
	}
}
