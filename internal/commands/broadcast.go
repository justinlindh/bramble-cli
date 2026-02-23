package commands

import (
	"fmt"
	"os"

	"github.com/justinlindh/bramble-cli/internal/output"
	bramble "github.com/justinlindh/bramble-go"
	"github.com/spf13/cobra"
)

var broadcastChannel int

func newBroadcastCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "broadcast <message>",
		Short: "Broadcast a message to all nodes",
		Long: `Send a mesh-wide text message.

By default this uses the public Broadcast channel.
Use --channel <index> to send on a specific channel instead.

Examples:
  bramble broadcast "hello everyone"
  bramble broadcast --channel 2 "hello channel 2"`,
		Args: cobra.ExactArgs(1),
		RunE: runBroadcast,
	}
	cmd.Flags().IntVar(&broadcastChannel, "channel", -1, "channel index for channel-scoped broadcast")
	return cmd
}

func runBroadcast(cmd *cobra.Command, args []string) error {
	ctx, cancel := commandContext()
	defer cancel()
	text := args[0]

	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	var r *bramble.SendResult
	if broadcastChannel >= 0 {
		r, err = client.BroadcastOnChannel(ctx, broadcastChannel, text)
		if err != nil {
			return fmt.Errorf("bramble-cli: broadcast on channel %d: %w", broadcastChannel, err)
		}
	} else {
		r, err = client.Broadcast(ctx, text)
		if err != nil {
			return fmt.Errorf("bramble-cli: broadcast: %w", err)
		}
	}
	if flagJSON {
		payload := map[string]any{"text": text, "status": r.Status}
		if broadcastChannel >= 0 {
			payload["channel"] = broadcastChannel
		}
		if r.BroadcastID != "" {
			payload["broadcast_id"] = r.BroadcastID
		}
		return output.PrintJSON(os.Stdout, payload)
	}
	fmt.Fprintln(os.Stdout, output.FormatBroadcastSendStatus(broadcastChannel, r.Status, r.BroadcastID))
	return nil
}
