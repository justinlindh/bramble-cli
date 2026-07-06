package commands

import (
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	bramble "github.com/justinlindh/bramble-go"

	"github.com/justinlindh/bramble-cli/internal/output"
)

// anchorFileFlag overrides the default anchor keystore path.
var anchorFileFlag string

// anchorKeystorePath resolves where the operator's SECRET anchor seed lives.
// Default: ~/.config/bramble/anchor.seed (0600). This is the fleet root key;
// it never leaves this machine and is never sent to a node.
func anchorKeystorePath() (string, error) {
	if anchorFileFlag != "" {
		return anchorFileFlag, nil
	}
	if p := os.Getenv("BRAMBLE_ANCHOR_FILE"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("bramble-cli: resolve home dir: %w", err)
	}
	return filepath.Join(home, ".config", "bramble", "anchor.seed"), nil
}

func loadAnchorSeed() ([]byte, error) {
	path, err := anchorKeystorePath()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("bramble-cli: no anchor key at %s (run 'bramble anchor generate' or 'anchor import')", path)
		}
		return nil, fmt.Errorf("bramble-cli: read anchor key: %w", err)
	}
	seed, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != 32 {
		return nil, fmt.Errorf("bramble-cli: anchor key at %s is corrupt (want 64 hex chars)", path)
	}
	return seed, nil
}

func saveAnchorSeed(seed []byte, force bool) (string, error) {
	path, err := anchorKeystorePath()
	if err != nil {
		return "", err
	}
	if !force {
		if _, err := os.Stat(path); err == nil {
			return "", fmt.Errorf("bramble-cli: anchor key already exists at %s (use --force to overwrite; this DESTROYS the current fleet anchor)", path)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", fmt.Errorf("bramble-cli: create config dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(seed)+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("bramble-cli: write anchor key: %w", err)
	}
	return path, nil
}

func newAnchorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "anchor",
		Short: "Fleet trust-anchor: mint the anchor key, provision nodes, enroll members",
		Long: "Manage the fleet trust anchor. The anchor is one Ed25519 keypair held by\n" +
			"the operator; its private seed stays on this machine and is never sent to a\n" +
			"node. Anchored nodes pin only anchor-endorsed identities.",
	}
	cmd.PersistentFlags().StringVar(&anchorFileFlag, "anchor-file", "", "path to the anchor key seed (default ~/.config/bramble/anchor.seed)")
	cmd.AddCommand(
		newAnchorGenerateCmd(),
		newAnchorImportCmd(),
		newAnchorShowCmd(),
		newAnchorStatusCmd(),
		newAnchorProvisionCmd(),
		newAnchorEnrollCmd(),
		newAnchorSignCmd(),
	)
	return cmd
}

func newAnchorGenerateCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:   "generate",
		Short: "Mint a new fleet anchor keypair (offline; no device)",
		RunE: func(cmd *cobra.Command, args []string) error {
			seed, err := bramble.GenerateAnchorSeed()
			if err != nil {
				return err
			}
			path, err := saveAnchorSeed(seed, force)
			if err != nil {
				return err
			}
			pub, _ := bramble.AnchorPublicKey(seed)
			backup, _ := bramble.EncodeAnchorBackup(seed)
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "New fleet anchor minted and saved to %s\n\n", path)
			fmt.Fprintf(w, "Fingerprint: %s\n", bramble.AnchorFingerprint(pub))
			fmt.Fprintf(w, "Public key:  %s\n\n", hex.EncodeToString(pub))
			fmt.Fprintf(w, "BACK UP THIS SECRET NOW (it is the fleet root key):\n  %s\n\n", backup)
			fmt.Fprintln(w, "If you lose it you can never enroll new nodes (re-anchor flag day).")
			fmt.Fprintln(w, "If it leaks, anyone can enroll fleet members until you re-anchor.")
			return nil
		},
	}
	c.Flags().BoolVar(&force, "force", false, "overwrite an existing anchor key (destroys the current fleet anchor)")
	return c
}

func newAnchorImportCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:   "import <backup>",
		Short: "Import an anchor from a bramble://anchor/v1?sk= backup (or bare 64-hex seed)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			seed, err := bramble.ParseAnchorBackup(args[0])
			if err != nil {
				return err
			}
			path, err := saveAnchorSeed(seed, force)
			if err != nil {
				return err
			}
			pub, _ := bramble.AnchorPublicKey(seed)
			fmt.Fprintf(cmd.OutOrStdout(), "Anchor imported to %s\nFingerprint: %s\n", path, bramble.AnchorFingerprint(pub))
			return nil
		},
	}
	c.Flags().BoolVar(&force, "force", false, "overwrite an existing anchor key")
	return c
}

func newAnchorShowCmd() *cobra.Command {
	var reveal bool
	c := &cobra.Command{
		Use:   "show",
		Short: "Show the local anchor fingerprint (and backup with --reveal)",
		RunE: func(cmd *cobra.Command, args []string) error {
			seed, err := loadAnchorSeed()
			if err != nil {
				return err
			}
			pub, _ := bramble.AnchorPublicKey(seed)
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Fingerprint: %s\nPublic key:  %s\n", bramble.AnchorFingerprint(pub), hex.EncodeToString(pub))
			if reveal {
				backup, _ := bramble.EncodeAnchorBackup(seed)
				fmt.Fprintf(w, "Backup (SECRET): %s\n", backup)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&reveal, "reveal", false, "also print the SECRET anchor backup seed")
	return c
}

func newAnchorStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the connected node's anchor + endorsement state",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := commandContext()
			defer cancel()
			client, err := getClient(ctx)
			if err != nil {
				return err
			}
			defer client.Close()
			st, err := client.AnchorStatus(ctx)
			if err != nil {
				return fmt.Errorf("bramble-cli: get anchor status: %w", err)
			}
			if flagJSON {
				return output.PrintJSON(os.Stdout, st)
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Anchored: %s\n", boolStr(st.Anchored, "yes", "no"))
			if st.Anchored {
				fmt.Fprintf(w, "Anchor fingerprint: %s\n", st.AnchorFingerprint)
			}
			fmt.Fprintf(w, "Endorsed: %s\n", boolStr(st.Endorsed, "yes", "no"))
			return nil
		},
	}
}

func newAnchorProvisionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "provision",
		Short: "Provision the local anchor's PUBLIC key onto the connected node",
		RunE: func(cmd *cobra.Command, args []string) error {
			seed, err := loadAnchorSeed()
			if err != nil {
				return err
			}
			pub, err := bramble.AnchorPublicKey(seed)
			if err != nil {
				return err
			}
			ctx, cancel := commandContext()
			defer cancel()
			client, err := getClient(ctx)
			if err != nil {
				return err
			}
			defer client.Close()
			if err := client.SetAnchor(ctx, hex.EncodeToString(pub)); err != nil {
				return fmt.Errorf("bramble-cli: set anchor: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Provisioned anchor %s onto the node.\n", bramble.AnchorFingerprint(pub))
			return nil
		},
	}
}

func newAnchorEnrollCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enroll",
		Short: "Enroll the connected node: sign a permanent cert for its identity and apply it",
		RunE: func(cmd *cobra.Command, args []string) error {
			seed, err := loadAnchorSeed()
			if err != nil {
				return err
			}
			ctx, cancel := commandContext()
			defer cancel()
			client, err := getClient(ctx)
			if err != nil {
				return err
			}
			defer client.Close()

			id, err := client.Identity(ctx)
			if err != nil {
				return fmt.Errorf("bramble-cli: get identity: %w", err)
			}
			if id.Ed25519Pub == "" {
				return fmt.Errorf("bramble-cli: node did not report ed25519_pub (firmware too old for trust anchor)")
			}
			naHex, sigHex, err := bramble.SignEndorsementHex(hex.EncodeToString(seed), id.Ed25519Pub, bramble.PermanentNotAfter)
			if err != nil {
				return err
			}
			if err := client.SetEndorsement(ctx, naHex, sigHex); err != nil {
				return fmt.Errorf("bramble-cli: set endorsement (node rejected the cert? check the node's anchor matches): %w", err)
			}
			st, _ := client.AnchorStatus(ctx)
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Enrolled node %s (identity %s...).\n", id.Address, id.Ed25519Pub[:16])
			if st != nil {
				fmt.Fprintf(w, "Endorsed: %s\n", boolStr(st.Endorsed, "yes", "no"))
			}
			return nil
		},
	}
}

func newAnchorSignCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sign <node-ed25519-pubkey-hex>",
		Short: "Sign a permanent cert for a node's identity key (offline; for remote enrollment)",
		Long: "Produce an endorsement cert for a node you are NOT directly connected to.\n" +
			"Give the node's ed25519 pubkey (from 'bramble anchor status' JSON or the\n" +
			"node's webapp), and send the printed cert back to be applied via setEndorsement.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			seed, err := loadAnchorSeed()
			if err != nil {
				return err
			}
			naHex, sigHex, err := bramble.SignEndorsementHex(hex.EncodeToString(seed), strings.TrimSpace(args[0]), bramble.PermanentNotAfter)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "Cert for %s (permanent):\n", args[0])
			fmt.Fprintf(w, "  not_after:       %s\n", naHex)
			fmt.Fprintf(w, "  endorsement_sig: %s\n", sigHex)
			return nil
		},
	}
}
