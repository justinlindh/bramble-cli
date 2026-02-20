package commands

import (
	"fmt"
	"os"

	"github.com/justinlindh/bramble-cli/internal/output"
	"github.com/spf13/cobra"
)

func newPingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ping",
		Short: "Ping the node",
		Long:  "Send a ping to the connected node and verify it responds correctly.",
		RunE:  runPing,
	}
}

func runPing(cmd *cobra.Command, args []string) error {
	ctx, cancel := commandContext()
	defer cancel()

	// For ping, we need the full version info.
	// Connect first, then call Version + Ping.
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	ver, err := client.Version(ctx)
	if err != nil {
		return fmt.Errorf("bramble-cli: get version: %w", err)
	}

	identity, err := client.Identity(ctx)
	if err != nil {
		return fmt.Errorf("bramble-cli: get identity: %w", err)
	}

	if err := client.Ping(ctx); err != nil {
		return fmt.Errorf("bramble-cli: ping: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, map[string]string{
			"address":          identity.Address,
			"protocol_version": ver.ProtocolVersion,
			"status":           "pong",
		})
	}

	fmt.Fprintf(os.Stdout, "Pong from %s (protocol: %s)\n", identity.Address, ver.ProtocolVersion)
	return nil
}
