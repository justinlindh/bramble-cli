package commands

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/output"
)

func generateHexToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage device WebSocket authentication",
		Long: `View and control the WebSocket auth token on a connected device.

When auth is enabled, WebSocket clients must provide the token to connect.
Serial connections are always unauthenticated (physical access = trusted).

Subcommands: status, disable, enable`,
	}
	cmd.AddCommand(
		newAuthStatusCmd(),
		newAuthDisableCmd(),
		newAuthEnableCmd(),
	)
	return cmd
}

func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether WebSocket auth is enabled",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := commandContext()
			defer cancel()
			client, err := getClient(ctx)
			if err != nil {
				return err
			}
			defer client.Close()

			token, err := client.GetAuthToken(ctx)
			if err != nil {
				return fmt.Errorf("bramble auth: %w", err)
			}

			if flagJSON {
				return output.PrintJSON(os.Stdout, map[string]any{
					"enabled": token != "",
					"token":   token,
				})
			}

			if token == "" {
				fmt.Fprintln(cmd.OutOrStdout(), "Auth: disabled (open access)")
				fmt.Fprintln(cmd.OutOrStdout(), "\nAny client can connect via WebSocket without a token.")
				fmt.Fprintln(cmd.OutOrStdout(), "Enable with: bramble auth enable")
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Auth: enabled")
				fmt.Fprintf(cmd.OutOrStdout(), "Token: %s\n", token)
				fmt.Fprintln(cmd.OutOrStdout(), "\nDisable for debugging: bramble auth disable")
			}
			return nil
		},
	}
}

func newAuthDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "Disable WebSocket auth (open access — use for debugging)",
		Long: `Clears the device's auth token, allowing any WebSocket client to connect
without authentication. Useful for debugging and development.

Re-enable with: bramble auth enable`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := commandContext()
			defer cancel()
			client, err := getClient(ctx)
			if err != nil {
				return err
			}
			defer client.Close()

			// SetAuthToken with empty string clears the token
			if err := client.SetAuthToken(ctx, ""); err != nil {
				return fmt.Errorf("bramble auth disable: %w", err)
			}

			if flagJSON {
				return output.PrintJSON(os.Stdout, map[string]any{"enabled": false})
			}

			fmt.Fprintln(cmd.OutOrStdout(), "✅ Auth disabled — WebSocket connections no longer require a token.")
			fmt.Fprintln(cmd.OutOrStdout(), "Re-enable with: bramble auth enable")
			return nil
		},
	}
}

func newAuthEnableCmd() *cobra.Command {
	var customToken string

	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable WebSocket auth (generates or sets a token)",
		Long: `Enables WebSocket authentication on the device. If no custom token is
provided, the device will generate a new random token on next boot.

To set a specific token: bramble auth enable --set <token>`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := commandContext()
			defer cancel()
			client, err := getClient(ctx)
			if err != nil {
				return err
			}
			defer client.Close()

			if customToken == "" {
				// Generate a random 32-char hex token
				customToken = generateHexToken()
			}

			if err := client.SetAuthToken(ctx, customToken); err != nil {
				return fmt.Errorf("bramble auth enable: %w", err)
			}

			// Verify the token was set
			token, err := client.GetAuthToken(ctx)
			if err != nil {
				return fmt.Errorf("bramble auth enable: verify: %w", err)
			}

			if flagJSON {
				return output.PrintJSON(os.Stdout, map[string]any{
					"enabled": true,
					"token":   token,
				})
			}

			fmt.Fprintln(cmd.OutOrStdout(), "✅ Auth enabled.")
			fmt.Fprintf(cmd.OutOrStdout(), "Token: %s\n", token)
			return nil
		},
	}

	cmd.Flags().StringVar(&customToken, "set", "", "set a specific auth token (default: device generates one)")
	return cmd
}
