package commands

import (
	"fmt"
	"os"
	"time"

	bramble "github.com/justinlindh/bramble-go"
	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/output"
)

var broadcastChannel int
var broadcastWaitDeliverySec int
var broadcastCritical bool

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
	cmd.Flags().IntVar(&broadcastWaitDeliverySec, "wait-delivery", 0, "seconds to wait for broadcast delivery telemetry after send")
	cmd.Flags().BoolVar(&broadcastCritical, "critical", false, "send with critical priority")
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

	deliveries := make([]bramble.BroadcastDelivery, 0, 8)
	deliveryCh := make(chan bramble.BroadcastDelivery, 32)
	client.OnBroadcastDelivery(func(evt bramble.BroadcastDelivery) {
		select {
		case deliveryCh <- evt:
		default:
		}
	})

	var r *bramble.SendResult
	if broadcastCritical {
		r, err = client.SendBroadcastCritical(ctx, text)
		if err != nil {
			return fmt.Errorf("bramble-cli: broadcast critical: %w", err)
		}
	} else if broadcastChannel >= 0 {
		r, err = client.BroadcastOnChannel(ctx, broadcastChannel, text)
		if err != nil {
			return fmt.Errorf("bramble-cli: broadcast on channel %d: %w", broadcastChannel, err)
		}
	} else {
		r, err = client.SendBroadcast(ctx, text)
		if err != nil {
			return fmt.Errorf("bramble-cli: broadcast: %w", err)
		}
	}

	if broadcastWaitDeliverySec > 0 {
		timer := time.NewTimer(time.Duration(broadcastWaitDeliverySec) * time.Second)
		defer timer.Stop()
		for {
			select {
			case evt := <-deliveryCh:
				if r.BroadcastID == "" || evt.BroadcastID == "" || evt.BroadcastID == r.BroadcastID {
					deliveries = append(deliveries, evt)
				}
			case <-timer.C:
				goto done
			}
		}
	}

done:
	if flagJSON {
		result := BroadcastResult{Text: text, Status: r.Status}
		if broadcastChannel >= 0 {
			result.Channel = broadcastChannel
		}
		if r.BroadcastID != "" {
			result.BroadcastID = r.BroadcastID
		}
		if len(deliveries) > 0 {
			result.Deliveries = deliveries
		}
		if broadcastWaitDeliverySec > 0 {
			result.DeliveryWindowS = broadcastWaitDeliverySec
			result.DeliveryCount = len(deliveries)
		}
		return output.PrintJSON(os.Stdout, result)
	}

	fmt.Fprintln(os.Stdout, output.FormatBroadcastSendStatus(broadcastChannel, r.Status, r.BroadcastID))
	if broadcastWaitDeliverySec > 0 {
		fmt.Fprintf(os.Stdout, "Observed %d broadcast delivery events in %ds\n", len(deliveries), broadcastWaitDeliverySec)
		for _, evt := range deliveries {
			fmt.Fprintln(os.Stdout, output.FormatBroadcastDeliveryLine(time.Now(), evt))
		}
	}
	return nil
}
