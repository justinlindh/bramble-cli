package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const testKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestNewNetkeyCmd_HasExpectedSubcommands(t *testing.T) {
	t.Parallel()

	cmd := newNetkeyCmd()
	want := []string{"status", "generate", "provision", "fingerprint"}

	for _, name := range want {
		if got, _, err := cmd.Find([]string{name}); err != nil || got == nil || got.Name() != name {
			t.Fatalf("expected subcommand %q to exist (got=%v, err=%v)", name, got, err)
		}
	}
}

// The key is a secret: accepting it as a positional argument would leak it to
// ps, /proc, and shell history. Both key-consuming commands must refuse args.
func TestNetkeyCmds_RejectPositionalKey(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		cmd  *cobra.Command
	}{
		{"provision", newNetkeyProvisionCmd()},
		{"fingerprint", newNetkeyFingerprintCmd()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cmd.Args(tc.cmd, []string{testKeyHex}); err == nil {
				t.Fatal("expected the command to reject a positional key argument")
			}
		})
	}
}

func TestReadNetworkKeyInput_FromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "netkey.hex")
	if err := os.WriteFile(path, []byte(testKeyHex+"\n"), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	// Construct the command first: binding the flag resets the package-level
	// var to its default, exactly as it does at startup before flag parsing.
	cmd := newNetkeyProvisionCmd()
	t.Cleanup(func() { netkeyFileFlag = "" })
	netkeyFileFlag = path

	got, err := readNetworkKeyInput(cmd)
	if err != nil {
		t.Fatalf("readNetworkKeyInput: %v", err)
	}
	if got != testKeyHex {
		t.Fatalf("key: got %q, want %q", got, testKeyHex)
	}
}

func TestReadNetworkKeyInput_FromShareStringFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "netkey.uri")
	share := "bramble://net/v1?k=" + testKeyHex
	if err := os.WriteFile(path, []byte(share+"\n"), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	cmd := newNetkeyProvisionCmd()
	t.Cleanup(func() { netkeyFileFlag = "" })
	netkeyFileFlag = path

	got, err := readNetworkKeyInput(cmd)
	if err != nil {
		t.Fatalf("readNetworkKeyInput: %v", err)
	}
	if got != share {
		t.Fatalf("share string: got %q, want %q", got, share)
	}
}

func TestReadNetworkKeyInput_MissingFile(t *testing.T) {
	cmd := newNetkeyProvisionCmd()
	t.Cleanup(func() { netkeyFileFlag = "" })
	netkeyFileFlag = filepath.Join(t.TempDir(), "absent.hex")

	if _, err := readNetworkKeyInput(cmd); err == nil {
		t.Fatal("expected an error for a missing key file")
	}
}

// Non-interactive with no --key-file must fail with actionable guidance rather
// than hanging on a prompt that nothing will answer.
func TestReadNetworkKeyInput_NonInteractiveWithoutFile(t *testing.T) {
	if isInteractive() {
		t.Skip("stdin is a terminal; this case covers scripted use")
	}
	cmd := newNetkeyProvisionCmd()
	t.Cleanup(func() { netkeyFileFlag = "" })
	netkeyFileFlag = ""

	_, err := readNetworkKeyInput(cmd)
	if err == nil {
		t.Fatal("expected an error when not interactive and no --key-file was given")
	}
	if !strings.Contains(err.Error(), "--key-file required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "never taken as an argument") {
		t.Fatalf("error should explain why the key is not a positional arg: %v", err)
	}
}

func TestNetkeyProvision_FlagsExposeKeyFileAndForce(t *testing.T) {
	t.Parallel()

	cmd := newNetkeyProvisionCmd()
	for _, name := range []string{"key-file", "force"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("expected --%s flag on netkey provision", name)
		}
	}
}

func TestNetkeyGenerate_HasForceFlag(t *testing.T) {
	t.Parallel()

	if newNetkeyGenerateCmd().Flags().Lookup("force") == nil {
		t.Fatal("expected --force on netkey generate: re-keying a provisioned node is destructive")
	}
}
