package commands

import (
	"encoding/json"
	"strings"
	"testing"

	bramble "github.com/justinlindh/bramble-go"
)

func TestNewBleSecurityCmd_Config(t *testing.T) {
	t.Parallel()

	cmd := newBleSecurityCmd()
	if cmd.Use != "ble-security" {
		t.Fatalf("unexpected use: %q", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Fatal("expected RunE handler for default ble-security status")
	}

	for _, name := range []string{"status", "set-passkey", "clear-passkey"} {
		sub, _, err := cmd.Find([]string{name})
		if err != nil {
			t.Fatalf("find %s subcommand: %v", name, err)
		}
		if sub == nil || sub.Name() != name {
			t.Fatalf("expected %s subcommand, got %#v", name, sub)
		}
		if sub.RunE == nil {
			t.Fatalf("expected %s subcommand RunE handler", name)
		}
	}

	setCmd, _, err := cmd.Find([]string{"set-passkey"})
	if err != nil {
		t.Fatalf("find set-passkey subcommand: %v", err)
	}
	if setCmd.Flags().Lookup("passkey") == nil {
		t.Fatal("expected --passkey flag on ble-security set-passkey")
	}
}

func TestNewBleSetPasskeyCmd_PasskeyNeverPositional(t *testing.T) {
	t.Parallel()

	cmd := newBleSetPasskeyCmd()
	if cmd.Use != "set-passkey" {
		t.Fatalf("unexpected use string: %q (the passkey must not be a positional argument)", cmd.Use)
	}
	if err := cmd.Args(cmd, []string{"314159"}); err == nil {
		t.Fatal("expected a positional passkey to be rejected")
	}
}

func TestValidateBlePasskey(t *testing.T) {
	t.Parallel()

	valid := []string{"314159", "000000", "999999"}
	for _, passkey := range valid {
		if err := validateBlePasskey(passkey); err != nil {
			t.Fatalf("validateBlePasskey(%q) = %v, want nil", passkey, err)
		}
	}

	invalid := []string{"", "12345", "1234567", "12345a", "31 415", "-31415", "３１４１５９"}
	for _, passkey := range invalid {
		if err := validateBlePasskey(passkey); err == nil {
			t.Fatalf("validateBlePasskey(%q) = nil, want an error", passkey)
		}
	}
}

// A malformed passkey must fail before the node is contacted: the node wipes
// every bond as the first step of a passkey change, so a typo that reached it
// would cost a re-pair of every client for nothing.
func TestRunBleSetPasskey_RejectsMalformedBeforeConnecting(t *testing.T) {
	t.Parallel()

	cmd := newBleSetPasskeyCmd()
	if err := cmd.Flags().Set("passkey", "12345"); err != nil {
		t.Fatalf("set --passkey: %v", err)
	}
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error for a 5-digit passkey")
	}
	if !strings.Contains(err.Error(), "exactly 6 digits") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunBleSetPasskey_RequiresFlagWhenNotInteractive(t *testing.T) {
	t.Parallel()

	cmd := newBleSetPasskeyCmd()
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when neither --passkey nor a terminal is available")
	}
	if !strings.Contains(err.Error(), "--passkey required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBlePasskeySetAfter(t *testing.T) {
	t.Parallel()

	cases := []struct {
		mode    bramble.BleSecurityMode
		cleared bool
		want    bool
	}{
		{bramble.BleSecurityModeStaticPasskey, false, true},
		{bramble.BleSecurityModeJustWorks, true, false},
		// The node's mode outranks the request: a set that the node answered
		// as just-works did not leave a passkey stored.
		{bramble.BleSecurityModeJustWorks, false, false},
		// A node that omits the mode leaves the request as the only evidence.
		{"", false, true},
		{"", true, false},
	}
	for _, tc := range cases {
		if got := blePasskeySetAfter(tc.mode, tc.cleared); got != tc.want {
			t.Fatalf("blePasskeySetAfter(%q, %v) = %v, want %v", tc.mode, tc.cleared, got, tc.want)
		}
	}
}

func TestFormatBleSecurity_PasskeyDisplay(t *testing.T) {
	t.Parallel()

	got := formatBleSecurity(&bramble.BleSecurity{Mode: bramble.BleSecurityModePasskeyDisplay})
	for _, want := range []string{"random code shown on the device screen", "Nothing to configure"} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatBleSecurity(display) = %q, missing %q", got, want)
		}
	}
	if strings.Contains(got, "set-passkey") {
		t.Fatalf("display nodes reject a static passkey; do not advertise it: %q", got)
	}
}

// A stored passkey on a node that displays its own code is inert: the node
// resolves display first and refuses passkey changes, so status says so rather
// than implying the stored code is in use.
func TestFormatBleSecurity_PasskeyDisplayWithStoredPasskey(t *testing.T) {
	t.Parallel()

	got := formatBleSecurity(&bramble.BleSecurity{
		Mode:             bramble.BleSecurityModePasskeyDisplay,
		StaticPasskeySet: true,
	})
	if !strings.Contains(got, "stored but unused") {
		t.Fatalf("formatBleSecurity(display+stored) = %q, missing the inert-passkey note", got)
	}
}

func TestFormatBleSecurity_StaticPasskey(t *testing.T) {
	t.Parallel()

	got := formatBleSecurity(&bramble.BleSecurity{
		Mode:             bramble.BleSecurityModeStaticPasskey,
		StaticPasskeySet: true,
	})
	for _, want := range []string{"static passkey", "cannot be read back", "bramble ble-security set-passkey"} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatBleSecurity(static) = %q, missing %q", got, want)
		}
	}
}

func TestFormatBleSecurity_JustWorks(t *testing.T) {
	t.Parallel()

	got := formatBleSecurity(&bramble.BleSecurity{Mode: bramble.BleSecurityModeJustWorks})
	for _, want := range []string{"just works", "man-in-the-middle", "bramble ble-security set-passkey"} {
		if !strings.Contains(got, want) {
			t.Fatalf("formatBleSecurity(just-works) = %q, missing %q", got, want)
		}
	}
}

func TestDescribeBleMode_UnknownModePassesThrough(t *testing.T) {
	t.Parallel()

	if got := describeBleMode("some-future-mode"); got != "some-future-mode" {
		t.Fatalf("describeBleMode(unknown) = %q, want the raw mode", got)
	}
}

// The passkey is write-only on the node, and the JSON output must not
// reintroduce it from the client side.
func TestBlePasskeyResult_OmitsThePasskey(t *testing.T) {
	t.Parallel()

	encoded, err := json.Marshal(BlePasskeyResult{Mode: "static-passkey", StaticPasskeySet: true, BondsWiped: true})
	if err != nil {
		t.Fatalf("marshal BlePasskeyResult: %v", err)
	}
	if strings.Contains(string(encoded), "passkey\":\"") {
		t.Fatalf("BlePasskeyResult must not carry a passkey value: %s", encoded)
	}
	for _, want := range []string{`"mode":"static-passkey"`, `"static_passkey_set":true`, `"bonds_wiped":true`} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("BlePasskeyResult JSON = %s, missing %s", encoded, want)
		}
	}
}

// Both JSON shapes report the posture under the same keys, so a script does
// not have to special-case which ble-security subcommand produced the output.
func TestBleSecurityResult_SharesKeysWithBlePasskeyResult(t *testing.T) {
	t.Parallel()

	status, err := json.Marshal(BleSecurityResult{Mode: "just-works"})
	if err != nil {
		t.Fatalf("marshal BleSecurityResult: %v", err)
	}
	for _, want := range []string{`"mode":"just-works"`, `"static_passkey_set":false`} {
		if !strings.Contains(string(status), want) {
			t.Fatalf("BleSecurityResult JSON = %s, missing %s", status, want)
		}
	}
}
