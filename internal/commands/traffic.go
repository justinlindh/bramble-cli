package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	bramble "github.com/justinlindh/bramble-go"
	"github.com/spf13/cobra"
)

func newTrafficCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "traffic",
		Short: "Traffic debug telemetry commands",
		Long: `Manage traffic debug telemetry and export event data.

Subcommands:
  monitor   Live stream of traffic events
  export    Export traffic events to JSONL`,
	}
	cmd.AddCommand(
		newTrafficMonitorCmd(),
		newTrafficExportCmd(),
	)
	return cmd
}

func newTrafficMonitorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "monitor",
		Short: "Live stream of traffic debug events",
		Long: `Subscribe to real-time traffic debug events from the connected node.
Shows TX/RX packet metadata including category, airtime tier, packet length, and RSSI.
Press Ctrl+C to stop.

Flags:
  --tx-only     Only show TX events
  --rx-only     Only show RX events
  --category    Filter by category (beacon, timesync, routing, ack, chat, maintenance, other)`,
		RunE: runTrafficMonitor,
	}
	cmd.Flags().Bool("tx-only", false, "only show TX events")
	cmd.Flags().Bool("rx-only", false, "only show RX events")
	cmd.Flags().String("category", "", "filter by category")
	return cmd
}

func newTrafficExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export traffic events to JSONL",
		Long: `Fetch traffic events from the node's ring buffer and export to JSONL format.
Use --since to request only new events (incremental export).

Examples:
  bramble traffic export
  bramble traffic export --since 12345 --limit 100
  bramble traffic export --format jsonl > events.jsonl`,
		RunE: runTrafficExport,
	}
	cmd.Flags().Uint32("since", 0, "only fetch events with seq > since")
	cmd.Flags().Int("limit", 100, "maximum events to fetch (1-512)")
	cmd.Flags().String("format", "jsonl", "output format (jsonl)")
	return cmd
}

func runTrafficMonitor(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	txOnly, _ := cmd.Flags().GetBool("tx-only")
	rxOnly, _ := cmd.Flags().GetBool("rx-only")
	categoryFilter, _ := cmd.Flags().GetString("category")

	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	// Check if traffic debug is enabled
	status, err := client.GetTrafficDebug(ctx)
	if err != nil {
		return fmt.Errorf("bramble-cli: get traffic debug status: %w", err)
	}

	if !status.Enabled {
		fmt.Fprintln(os.Stderr, "Warning: Traffic debug is disabled. Enable it with: bramble config set-traffic-debug --enabled")
		fmt.Fprintln(os.Stderr, "Attempting to enable now...")
		enabled := true
		_, err := client.SetTrafficDebug(ctx, bramble.SetTrafficDebugParams{
			Enabled: &enabled,
		})
		if err != nil {
			return fmt.Errorf("bramble-cli: failed to enable traffic debug: %w", err)
		}
		fmt.Fprintln(os.Stderr, "Traffic debug enabled.")
	}

	fmt.Fprintln(os.Stderr, "Monitoring traffic events... (Ctrl+C to stop)")
	fmt.Fprintf(os.Stderr, "Buffer: %d/%d events, dropped: %d\n",
		status.BufferCount, status.BufferCapacity, status.DroppedCount)

	client.OnTrafficEvent(func(evt bramble.TrafficEvent) {
		// Apply filters
		if txOnly && !evt.IsTx {
			return
		}
		if rxOnly && evt.IsTx {
			return
		}
		if categoryFilter != "" && evt.Category != categoryFilter {
			return
		}

		if flagJSON {
			b, _ := json.Marshal(evt)
			fmt.Fprintln(os.Stdout, string(b))
		} else {
			direction := "RX"
			if evt.IsTx {
				direction = "TX"
			}
			rssiStr := ""
			if !evt.IsTx && evt.RSSI != 0 {
				rssiStr = fmt.Sprintf(" RSSI=%ddBm", evt.RSSI)
			}
			fmt.Fprintf(os.Stdout, "[%10d] %-2s %-12s %-10s %4db%s (tier=%s)\n",
				evt.Seq, direction, evt.Category, fmt.Sprintf("pkt=%d", evt.PktType),
				evt.PacketLen, rssiStr, evt.AirtimeTier)
		}
	})

	// Wait for Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Fprintln(os.Stderr, "\nStopping traffic monitor.")
	return nil
}

func runTrafficExport(cmd *cobra.Command, args []string) error {
	ctx, cancel := commandContext()
	defer cancel()

	sinceSeq, _ := cmd.Flags().GetUint32("since")
	limit, _ := cmd.Flags().GetInt("limit")
	format, _ := cmd.Flags().GetString("format")

	if format != "jsonl" {
		return fmt.Errorf("bramble-cli: unsupported format %q (only jsonl is supported)", format)
	}
	if limit < 1 || limit > 512 {
		return fmt.Errorf("bramble-cli: limit must be between 1 and 512")
	}

	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	params := bramble.GetTrafficEventsParams{
		Limit: &limit,
	}
	if sinceSeq > 0 {
		params.SinceSeq = &sinceSeq
	}

	resp, err := client.GetTrafficEvents(ctx, params)
	if err != nil {
		return fmt.Errorf("bramble-cli: get traffic events: %w", err)
	}

	if !flagJSON {
		fmt.Fprintf(os.Stderr, "Fetched %d/%d events\n", resp.Returned, resp.TotalAvailable)
	}

	// Write events as JSONL to stdout
	for _, evt := range resp.Events {
		b, err := json.Marshal(evt)
		if err != nil {
			return fmt.Errorf("bramble-cli: marshal event: %w", err)
		}
		fmt.Fprintln(os.Stdout, string(b))
	}

	return nil
}
