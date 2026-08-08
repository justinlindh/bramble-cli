package commands

import (
	"fmt"
	"os"
	"strings"

	bramble "github.com/justinlindh/bramble-go"
	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/output"
)

// blePasskeyDigits is the passkey length BLE SMP defines and the node enforces.
const blePasskeyDigits = 6

func newBleSecurityCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ble-security",
		Short: "Show or configure BLE pairing security",
		Long: `Show how the node authenticates BLE pairing, and manage its static passkey.

A node with a display shows a fresh random 6-digit code on its own screen for
each pairing attempt; there is nothing to configure and it rejects a static
passkey. A node without a display uses a static 6-digit passkey you set here,
or pairs with no code at all until one is set.

Setting, changing, or clearing the passkey wipes the node's stored BLE bonds,
so every client that was paired must pair again with the current code.

Subcommands: status, set-passkey, clear-passkey`,
		RunE: runBleSecurityStatus,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show the BLE pairing mode",
		RunE:  runBleSecurityStatus,
	})
	cmd.AddCommand(newBleSetPasskeyCmd())
	cmd.AddCommand(newBleClearPasskeyCmd())
	return cmd
}

func newBleSetPasskeyCmd() *cobra.Command {
	var passkey string

	cmd := &cobra.Command{
		Use:   "set-passkey",
		Short: "Set the static BLE pairing passkey",
		Long: `Set the 6-digit passkey BLE clients must enter to pair with this node.

The passkey is write-only: the node stores it and never reports it back, so
there is no way to read a forgotten one. Set a new one instead.

Setting it wipes the node's stored BLE bonds, so every client that was paired
must pair again with the new code.

Nodes with a display reject this: they show a fresh random code per pairing
attempt instead.

The passkey is never accepted as a positional argument, only via --passkey or
an interactive prompt: a bare command-line argument is visible to every other
process on the machine (ps, /proc, shell history), while a flag or prompt is
not logged or echoed.

Examples:
  bramble ble-security set-passkey                    # prompts, input hidden
  bramble ble-security set-passkey --passkey 314159`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBleSetPasskey(cmd, passkey)
		},
	}
	cmd.Flags().StringVar(&passkey, "passkey", "", "6-digit pairing passkey (avoid in scripts/shell history; omit to be prompted)")
	return cmd
}

func newBleClearPasskeyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clear-passkey",
		Short: "Clear the static BLE pairing passkey",
		Long: `Clear the node's static BLE passkey, returning it to unauthenticated pairing.

With no passkey set, any client in range can pair without entering a code,
which leaves pairing open to a man-in-the-middle. Clearing also wipes the
node's stored BLE bonds, so every client that was paired must pair again.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return applyBlePasskey(cmd, "", "clear ble passkey")
		},
	}
}

// validateBlePasskey enforces the node's rule up front so a typo costs a
// message rather than a bond wipe and a round trip. The rejected value is not
// quoted back: a typo is usually a near miss of the real passkey, and the
// point of keeping it off the command line is that it stays out of terminals
// and logs.
func validateBlePasskey(passkey string) error {
	if len(passkey) != blePasskeyDigits {
		return fmt.Errorf("bramble-cli: passkey invalid: must be exactly %d digits", blePasskeyDigits)
	}
	for _, r := range passkey {
		if r < '0' || r > '9' {
			return fmt.Errorf("bramble-cli: passkey invalid: must be digits only")
		}
	}
	return nil
}

func runBleSetPasskey(cmd *cobra.Command, passkey string) error {
	if !cmd.Flags().Changed("passkey") {
		if !isInteractive() {
			return fmt.Errorf("bramble-cli: --passkey required when not running interactively")
		}
		entered, err := promptSecret(cmd, fmt.Sprintf("New %d-digit BLE pairing passkey: ", blePasskeyDigits))
		if err != nil {
			return fmt.Errorf("bramble-cli: %w", err)
		}
		passkey = entered
	}

	if err := validateBlePasskey(passkey); err != nil {
		return err
	}
	return applyBlePasskey(cmd, passkey, "set ble passkey")
}

// applyBlePasskey sends the change and turns the node's refusal into a failure.
// The node reports a refusal as ok:false inside a successful RPC, so a nil
// error from the SDK is not on its own a success.
func applyBlePasskey(cmd *cobra.Command, passkey, action string) error {
	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	resp, err := client.SetBlePasskey(ctx, passkey)
	if err != nil {
		return fmt.Errorf("bramble-cli: %s: %w", action, err)
	}
	if !resp.OK {
		reason := resp.Error
		if reason == "" {
			reason = "node refused the change without giving a reason"
		}
		return fmt.Errorf("bramble-cli: %s: %s", action, reason)
	}

	clearing := passkey == ""
	if flagJSON {
		return output.PrintJSON(os.Stdout, BlePasskeyResult{
			Mode:             string(resp.Mode),
			StaticPasskeySet: blePasskeySetAfter(resp.Mode, clearing),
			BondsWiped:       true,
		})
	}

	if clearing {
		fmt.Fprintln(cmd.OutOrStdout(), "BLE passkey cleared. Pairing no longer requires a code.")
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "BLE passkey set. Clients must enter it to pair.")
	}
	fmt.Fprintln(cmd.OutOrStdout(), "All stored BLE bonds were wiped: every previously paired client must pair again.")
	if resp.Mode != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Pairing mode: %s\n", describeBleMode(resp.Mode))
	}
	return nil
}

// blePasskeySetAfter reports whether a passkey is stored once a change has
// been accepted. The mode the node reports back is authoritative, so it wins
// over what was asked for; a node that omits it leaves the request as the only
// evidence.
func blePasskeySetAfter(mode bramble.BleSecurityMode, cleared bool) bool {
	if mode == "" {
		return !cleared
	}
	return mode == bramble.BleSecurityModeStaticPasskey
}

func runBleSecurityStatus(cmd *cobra.Command, args []string) error {
	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	sec, err := client.BleSecurity(ctx)
	if err != nil {
		return fmt.Errorf("bramble-cli: get ble security: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, BleSecurityResult{
			Mode:             string(sec.Mode),
			StaticPasskeySet: sec.StaticPasskeySet,
		})
	}

	fmt.Fprintln(cmd.OutOrStdout(), formatBleSecurity(sec))
	return nil
}

// describeBleMode names a pairing mode in a single readable phrase.
func describeBleMode(mode bramble.BleSecurityMode) string {
	switch mode {
	case bramble.BleSecurityModePasskeyDisplay:
		return "random code shown on the device screen"
	case bramble.BleSecurityModeStaticPasskey:
		return "static passkey"
	case bramble.BleSecurityModeJustWorks:
		return "just works (no code required)"
	default:
		return string(mode)
	}
}

func formatBleSecurity(sec *bramble.BleSecurity) string {
	lines := []string{fmt.Sprintf("BLE pairing: %s", describeBleMode(sec.Mode))}

	switch sec.Mode {
	case bramble.BleSecurityModePasskeyDisplay:
		lines = append(lines,
			"The node shows a fresh 6-digit code for each pairing attempt. Nothing to configure.")
		if sec.StaticPasskeySet {
			// The node resolves display before static passkey, and refuses
			// every passkey change while it has a display, so a stored
			// passkey here is inert and cannot be cleared from the CLI.
			lines = append(lines,
				"A static passkey is stored but unused: the displayed code wins, and the node refuses passkey changes while it has a display.")
		}
	case bramble.BleSecurityModeStaticPasskey:
		lines = append(lines,
			"Clients must enter the configured 6-digit code. The code is write-only and cannot be read back.",
			"Change it with: bramble ble-security set-passkey")
	case bramble.BleSecurityModeJustWorks:
		lines = append(lines,
			"Any client in range can pair without a code, which leaves pairing open to a man-in-the-middle.",
			"Set a code with: bramble ble-security set-passkey")
	}

	return strings.Join(lines, "\n")
}
