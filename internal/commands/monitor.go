package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/justinlindh/bramble-cli/internal/output"
	bramble "github.com/justinlindh/bramble-go"
	"github.com/spf13/cobra"
)

func newMonitorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "Live event stream from the node",
		Long: `Subscribe to real-time events from the connected node.
Prints incoming messages, delivery acks, and neighbor changes.
Press Ctrl+C to stop.

Flags:
  --messages    Only show message events
  --neighbors   Only show neighbor change events
  --events      Comma-separated event filter (message, ack, neighbor, broadcast-delivery)`,
		RunE: runMonitor,
	}
	cmd.Flags().Bool("messages", false, "only show message events")
	cmd.Flags().Bool("neighbors", false, "only show neighbor-change events")
	cmd.Flags().StringSlice("events", nil, "event filter (message, ack, neighbor, broadcast-delivery)")
	return cmd
}

func runMonitor(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	onlyMessages, _ := cmd.Flags().GetBool("messages")
	onlyNeighbors, _ := cmd.Flags().GetBool("neighbors")
	eventFilters, _ := cmd.Flags().GetStringSlice("events")

	// Default: show everything.
	showMessages := !onlyNeighbors || onlyMessages
	showNeighbors := !onlyMessages || onlyNeighbors
	showAcks := !onlyMessages && !onlyNeighbors
	showBroadcastDeliveries := !onlyMessages && !onlyNeighbors

	if len(eventFilters) > 0 {
		showMessages = false
		showNeighbors = false
		showAcks = false
		showBroadcastDeliveries = false
		for _, raw := range eventFilters {
			switch strings.TrimSpace(strings.ToLower(raw)) {
			case "message", "messages":
				showMessages = true
			case "ack", "acks":
				showAcks = true
			case "neighbor", "neighbors", "neighbor-change":
				showNeighbors = true
			case "broadcast-delivery", "broadcast_delivery", "broadcastdelivery":
				showBroadcastDeliveries = true
			}
		}
	}

	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	fmt.Fprintln(os.Stderr, "Monitoring node events... (Ctrl+C to stop)")

	if showMessages {
		client.OnMessage(func(msg bramble.Message) {
			ts := time.Unix(msg.Timestamp, 0).Format("15:04:05")
			if flagJSON {
				b, _ := json.Marshal(map[string]any{
					"event":     "message",
					"from":      fmt.Sprintf("%08X", msg.From),
					"to":        fmt.Sprintf("%08X", msg.To),
					"text":      msg.Text,
					"tier":      msg.Tier,
					"timestamp": msg.Timestamp,
					"msg_id":    msg.MsgID,
				})
				fmt.Fprintln(os.Stdout, string(b))
			} else {
				fmt.Fprintf(os.Stdout, "[%s] MSG %08X→%08X  %q\n",
					ts, msg.From, msg.To, msg.Text)
			}
		})
	}

	if showAcks {
		client.OnAck(func(ack bramble.Ack) {
			if flagJSON {
				b, _ := json.Marshal(map[string]any{
					"event":     "ack",
					"packet_id": ack.PacketID,
					"status":    ack.Status,
				})
				fmt.Fprintln(os.Stdout, string(b))
			} else {
				fmt.Fprintf(os.Stdout, "[%s] ACK  packet#%d  status=%s\n",
					time.Now().Format("15:04:05"), ack.PacketID, ack.Status)
			}
		})
	}

	if showNeighbors {
		client.OnNeighborChange(func() {
			if flagJSON {
				b, _ := json.Marshal(map[string]string{
					"event": "neighbor_change",
					"time":  time.Now().Format(time.RFC3339),
				})
				fmt.Fprintln(os.Stdout, string(b))
			} else {
				fmt.Fprintf(os.Stdout, "[%s] NEIGHBOR  table updated\n",
					time.Now().Format("15:04:05"))
			}
		})
	}

	if showBroadcastDeliveries {
		client.OnBroadcastDelivery(func(evt bramble.BroadcastDelivery) {
			if flagJSON {
				b, _ := monitorBroadcastDeliveryJSON(evt)
				fmt.Fprintln(os.Stdout, string(b))
			} else {
				fmt.Fprintln(os.Stdout, monitorBroadcastDeliveryLine(time.Now(), evt))
			}
		})
	}

	// Wait for Ctrl+C.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Fprintln(os.Stderr, "\nStopping monitor.")
	return nil
}

func monitorBroadcastDeliveryLine(now time.Time, evt bramble.BroadcastDelivery) string {
	return output.FormatBroadcastDeliveryLine(now, evt)
}

func monitorBroadcastDeliveryJSON(evt bramble.BroadcastDelivery) ([]byte, error) {
	return json.Marshal(map[string]any{
		"event":   "broadcast_delivery",
		"payload": evt,
	})
}
