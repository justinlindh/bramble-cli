package commands

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	bramble "github.com/justinlindh/bramble-go"

	"github.com/justinlindh/bramble-cli/internal/output"
)

// The control-plane network key. A node without one is INERT: it neither emits
// nor accepts authenticated control-plane traffic, so it does not mesh at all.
// Provisioning is therefore the first thing done to a new node.
//
// The key is a secret, so it is never accepted as a positional argument: a bare
// command-line value is visible to every other process via ps and /proc, and
// lands in shell history. It comes from --key-file, stdin, or a hidden prompt,
// exactly like `wifi set` handles a WPA passphrase.

var (
	netkeyFileFlag  string
	netkeyForceFlag bool
)

func newNetkeyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "netkey",
		Short: "Control-plane network key: found a network, join nodes to it, check convergence",
		Long: "Manage the fleet network key. A node with no key is INERT and will not\n" +
			"mesh. Found a network once with 'netkey generate', then join every other\n" +
			"node with 'netkey provision'. The key is never readable from a device:\n" +
			"only its one-way fingerprint is, which is how you confirm convergence.",
	}
	cmd.AddCommand(
		newNetkeyStatusCmd(),
		newNetkeyGenerateCmd(),
		newNetkeyProvisionCmd(),
		newNetkeyFingerprintCmd(),
	)
	return cmd
}

func newNetkeyStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether the connected node is provisioned, and its key fingerprint",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := commandContext()
			defer cancel()
			client, err := getClient(ctx)
			if err != nil {
				return err
			}
			defer client.Close()

			st, err := client.NetworkKeyStatus(ctx)
			if err != nil {
				return fmt.Errorf("bramble-cli: get network key status: %w", err)
			}
			if flagJSON {
				return output.PrintJSON(os.Stdout, st)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Provisioned: %s\n", boolStr(st.Provisioned, "yes", "no"))
			if st.Provisioned {
				fmt.Fprintf(w, "Fingerprint: %s\n", st.Fingerprint)
				fmt.Fprintf(w, "Every node on this network reports this same fingerprint.\n")
			} else {
				fmt.Fprintf(w, "This node is INERT: it has no network key, so it is not meshing.\n")
				fmt.Fprintf(w, "Found a network with 'bramble netkey generate', or join one with 'bramble netkey provision'.\n")
			}
			return nil
		},
	}
}

func newNetkeyGenerateCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "generate",
		Short: "Found a new network: mint a key ON the connected node and provision it",
		Long: "Mints an entropy-gated key on the device, provisions that node with it\n" +
			"atomically, and prints the key once so you can carry it to every other\n" +
			"node. This node becomes the fleet founder.\n\n" +
			"The printed key is the only copy that will ever exist: the device never\n" +
			"reads it back. Record it out of band before relying on it. Re-keying an\n" +
			"already-provisioned node cuts it off from every node still on the old\n" +
			"key, so that requires --force.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := commandContext()
			defer cancel()
			client, err := getClient(ctx)
			if err != nil {
				return err
			}
			defer client.Close()

			// Re-keying is destructive to an existing fleet: make it deliberate.
			if st, err := client.NetworkKeyStatus(ctx); err == nil && st.Provisioned && !netkeyForceFlag {
				return fmt.Errorf(
					"bramble-cli: this node is already provisioned (fingerprint %s); "+
						"generating a new key RE-KEYS it and cuts it off from every node still on the old key. "+
						"Pass --force if that is what you want", st.Fingerprint)
			}

			gen, err := client.GenerateNetworkKey(ctx)
			if err != nil {
				return fmt.Errorf("bramble-cli: generate network key: %w", err)
			}
			if flagJSON {
				return output.PrintJSON(os.Stdout, gen)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Network key:  %s\n", gen.Key)
			fmt.Fprintf(w, "Fingerprint:  %s\n\n", gen.Fingerprint)
			fmt.Fprintf(w, "This node founded the network and is provisioned with the key above.\n")
			fmt.Fprintf(w, "SAVE THE KEY NOW: it is never readable from a device again.\n")
			fmt.Fprintf(w, "Join other nodes with: bramble netkey provision --key-file <file>\n")
			return nil
		},
	}
	c.Flags().BoolVar(&netkeyForceFlag, "force", false, "re-key a node that is already provisioned (cuts it off from the old network)")
	return c
}

func newNetkeyProvisionCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "provision",
		Short: "Join the connected node to an existing network by its key",
		Long: "Provisions a key you already have, joining this node to that network.\n\n" +
			"The key is a secret, so it is never taken as a positional argument: pass\n" +
			"--key-file (use - for stdin), or omit it on an interactive terminal to be\n" +
			"prompted with input hidden. Accepts a bramble://net/v1?k=... share string\n" +
			"or a bare 64 hex characters.",
		Example: "  bramble netkey provision --key-file netkey.hex\n" +
			"  bramble netkey provision            # prompts, input hidden\n" +
			"  pass show mesh/netkey | bramble netkey provision --key-file -",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := readNetworkKeyInput(cmd)
			if err != nil {
				return err
			}
			key, err := bramble.ParseNetworkKeyShare(raw)
			if err != nil {
				return fmt.Errorf("bramble-cli: %w", err)
			}
			keyHex := hex.EncodeToString(key)
			want := bramble.NetworkKeyFingerprint(key)

			ctx, cancel := commandContext()
			defer cancel()
			client, err := getClient(ctx)
			if err != nil {
				return err
			}
			defer client.Close()

			// Re-keying is destructive: refuse to silently move a provisioned
			// node onto a different network.
			st, statusErr := client.NetworkKeyStatus(ctx)
			if statusErr == nil && st.Provisioned && st.Fingerprint != want && !netkeyForceFlag {
				return fmt.Errorf(
					"bramble-cli: this node is already on a different network (fingerprint %s, the key you gave is %s); "+
						"provisioning it RE-KEYS the node and cuts it off from its current network. "+
						"Pass --force if that is what you want", st.Fingerprint, want)
			}

			if err := client.SetNetworkKey(ctx, keyHex); err != nil {
				return fmt.Errorf("bramble-cli: set network key: %w", err)
			}

			// Read the fingerprint back from the node rather than trusting the
			// write: this is the same convergence check an operator would do.
			after, err := client.NetworkKeyStatus(ctx)
			if err != nil {
				return fmt.Errorf("bramble-cli: provisioned, but could not confirm status: %w", err)
			}
			if !after.Provisioned || after.Fingerprint != want {
				return fmt.Errorf(
					"bramble-cli: node did not converge (provisioned=%v, fingerprint %s, expected %s)",
					after.Provisioned, after.Fingerprint, want)
			}
			if flagJSON {
				return output.PrintJSON(os.Stdout, after)
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"Provisioned. This node joined the network (fingerprint %s).\n", after.Fingerprint)
			return nil
		},
	}
	c.Flags().StringVar(&netkeyFileFlag, "key-file", "", "file holding the network key (- for stdin); omit to be prompted")
	c.Flags().BoolVar(&netkeyForceFlag, "force", false, "re-key a node that is already on a different network")
	return c
}

func newNetkeyFingerprintCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "fingerprint",
		Short: "Derive a key's fingerprint locally, without touching a device",
		Long: "Prints SHA256(key)[0:4], the same value every provisioned node reports.\n" +
			"Use it to check a key you hold against what a node shows. Reads the key\n" +
			"the same way as 'provision': --key-file, stdin, or a hidden prompt.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := readNetworkKeyInput(cmd)
			if err != nil {
				return err
			}
			key, err := bramble.ParseNetworkKeyShare(raw)
			if err != nil {
				return fmt.Errorf("bramble-cli: %w", err)
			}
			fp := bramble.NetworkKeyFingerprint(key)
			if flagJSON {
				return output.PrintJSON(os.Stdout, map[string]string{"fingerprint": fp})
			}
			fmt.Fprintln(cmd.OutOrStdout(), fp)
			return nil
		},
	}
	c.Flags().StringVar(&netkeyFileFlag, "key-file", "", "file holding the network key (- for stdin); omit to be prompted")
	return c
}

// readNetworkKeyInput sources the key from --key-file, stdin, or a hidden
// prompt, and never from argv. Returns the raw text for the share-string
// parser to interpret.
func readNetworkKeyInput(cmd *cobra.Command) (string, error) {
	switch {
	case netkeyFileFlag == "-":
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("bramble-cli: read network key from stdin: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	case netkeyFileFlag != "":
		b, err := os.ReadFile(netkeyFileFlag)
		if err != nil {
			return "", fmt.Errorf("bramble-cli: read network key file: %w", err)
		}
		return strings.TrimSpace(string(b)), nil
	case isInteractive():
		return promptSecret(cmd, "Network key (bramble://net/v1?k=... or 64 hex chars): ")
	default:
		return "", fmt.Errorf(
			"bramble-cli: --key-file required when not running interactively " +
				"(use - to read the key from stdin; the key is never taken as an argument)")
	}
}
