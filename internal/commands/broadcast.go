package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/justinlindh/bramble-cli/internal/output"
	"github.com/spf13/cobra"
)

func newBroadcastCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "broadcast <message>",
		Short: "Broadcast a message to all nodes",
		Long: `Send a broadcast text message to all reachable mesh nodes.

Example:
  bramble broadcast "hello everyone"`,
		Args: cobra.ExactArgs(1),
		RunE: runBroadcast,
	}
}

func runBroadcast(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	text := args[0]

	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.Broadcast(ctx, text)
	if err != nil {
		return fmt.Errorf("broadcast: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, map[string]any{
			"text":      text,
			"packet_id": result.PacketID,
		})
	}

	fmt.Fprintf(os.Stdout, "Broadcast: packet#%d\n", result.PacketID)
	return nil
}
