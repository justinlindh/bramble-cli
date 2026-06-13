package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	bramble "github.com/justinlindh/bramble-go"
	"github.com/justinlindh/bramble-go/transport"
	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/discovery"
)

func newPairCmd() *cobra.Command {
	var saveConfig bool

	cmd := &cobra.Command{
		Use:   "pair",
		Short: "Retrieve auth token from a serial-connected device",
		Long: `Connects to a Bramble device via serial and retrieves its WebSocket auth token.
The token is required for WebSocket (network) connections.

The device must be connected via USB serial. After pairing, use the token with:
  bramble --token <token> -t ws://<ip>/ws status
  export BRAMBLE_TOKEN=<token>

Examples:
  bramble pair                         # auto-detect serial device
  bramble pair -p /dev/ttyACM0         # specific port
  bramble pair --save                  # save token to ~/.config/bramble/tokens.json
  bramble pair --json                  # output as JSON`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := commandContext()
			defer cancel()

			// Pair only works over serial — ignore WS/BLE flags
			var port string
			if flagPort != "" {
				port = flagPort
			} else {
				detected, err := discovery.Detect()
				if err != nil {
					return fmt.Errorf("bramble pair: no serial device found: %w", err)
				}
				port = detected
				if !flagJSON {
					fmt.Fprintf(cmd.ErrOrStderr(), "Auto-detected device: %s\n", port)
				}
			}

			t := transport.NewSerial(port)
			client := bramble.NewClient(t)
			if err := client.Connect(ctx); err != nil {
				return fmt.Errorf("bramble pair: connect: %w", err)
			}
			defer client.Close()

			// Get device info for context
			ver, err := client.Version(ctx)
			if err != nil {
				return fmt.Errorf("bramble pair: get version: %w", err)
			}

			status, err := client.Status(ctx)
			if err != nil {
				return fmt.Errorf("bramble pair: get status: %w", err)
			}

			token, err := client.AuthToken(ctx)
			if err != nil {
				return fmt.Errorf("bramble pair: get auth token: %w", err)
			}

			if token == "" {
				if !flagJSON {
					fmt.Fprintln(cmd.OutOrStdout(), "No auth token configured on this device (open access).")
				}
				return nil
			}

			if flagJSON {
				result := map[string]string{
					"address":  status.Address,
					"hardware": ver.Hardware,
					"firmware": ver.FirmwareVersion,
					"token":    token,
					"port":     port,
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Device: %s (%s)\n", status.Address, ver.Hardware)
			fmt.Fprintf(cmd.OutOrStdout(), "Token:  %s\n", token)
			fmt.Fprintf(cmd.OutOrStdout(), "\nConnect via WebSocket:\n")
			fmt.Fprintf(cmd.OutOrStdout(), "  bramble -t ws://<device-ip>/ws --token %s status\n", token)
			fmt.Fprintf(cmd.OutOrStdout(), "\nOr set environment variable:\n")
			fmt.Fprintf(cmd.OutOrStdout(), "  export BRAMBLE_TOKEN=%s\n", token)

			if saveConfig {
				if err := saveTokenConfig(status.Address, token, ver.Hardware, port); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not save token config: %v\n", err)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "\nToken saved to %s\n", tokenConfigPath())
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&saveConfig, "save", false, "save token to ~/.config/bramble/tokens.json")
	return cmd
}

func tokenConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "bramble", "tokens.json")
}

func saveTokenConfig(address, token, hardware, port string) error {
	path := tokenConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	// Load existing config
	var tokens map[string]map[string]string
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &tokens)
	}
	if tokens == nil {
		tokens = make(map[string]map[string]string)
	}

	tokens[strings.ToUpper(address)] = map[string]string{
		"token":    token,
		"hardware": hardware,
		"port":     port,
	}

	out, err := json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o600)
}
