package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/justinlindh/bramble-cli/internal/output"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show node status",
		Long:  "Display address, firmware, radio, peer count, packet counters, and uptime.",
		RunE:  runStatus,
	}
}

func runStatus(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	// Gather data concurrently.
	status, err := client.Status(ctx)
	if err != nil {
		return fmt.Errorf("get status: %w", err)
	}
	identity, err := client.Identity(ctx)
	if err != nil {
		return fmt.Errorf("get identity: %w", err)
	}
	version, err := client.Version(ctx)
	if err != nil {
		return fmt.Errorf("get version: %w", err)
	}

	if flagJSON {
		type statusJSON struct {
			Address         string `json:"address"`
			Name            string `json:"name"`
			FirmwareVersion string `json:"firmware_version"`
			ProtocolVersion string `json:"protocol_version"`
			Hardware        string `json:"hardware"`
			PubkeyHash      string `json:"pubkey_hash"`
			UptimeSec       int    `json:"uptime_sec"`
			Neighbors       int    `json:"neighbors"`
			Routes          int    `json:"routes"`
			TxCount         int    `json:"tx_count"`
			RxCount         int    `json:"rx_count"`
			DroppedCount    int    `json:"dropped_count"`
			FreeHeapBytes   int    `json:"free_heap_bytes"`
			AirtimeUsedMs   int    `json:"airtime_used_ms"`
		}
		return output.PrintJSON(os.Stdout, statusJSON{
			Address:         identity.Address,
			PubkeyHash:      identity.PubkeyHash,
			FirmwareVersion: version.FirmwareVersion,
			ProtocolVersion: version.ProtocolVersion,
			Hardware:        version.Hardware,
			UptimeSec:       status.UptimeSec,
			Neighbors:       status.NeighborCount,
			Routes:          status.RouteCount,
			TxCount:         status.TxCount,
			RxCount:         status.RxCount,
			DroppedCount:    status.DroppedCount,
			FreeHeapBytes:   status.FreeHeapBytes,
			AirtimeUsedMs:   status.AirtimeUsedMs,
		})
	}

	w := os.Stdout
	fmt.Fprintf(w, "Address:   %s\n", identity.Address)
	fmt.Fprintf(w, "Pubkey:    %s\n", identity.PubkeyHash)
	fmt.Fprintf(w, "Firmware:  %s\n", version.FirmwareVersion)
	fmt.Fprintf(w, "Protocol:  %s\n", version.ProtocolVersion)
	fmt.Fprintf(w, "Hardware:  %s\n", version.Hardware)
	fmt.Fprintf(w, "Uptime:    %s\n", output.FormatUptime(status.UptimeSec))
	fmt.Fprintf(w, "Neighbors: %d\n", status.NeighborCount)
	fmt.Fprintf(w, "Routes:    %d\n", status.RouteCount)
	fmt.Fprintf(w, "TX/RX:     %d / %d  (dropped: %d)\n", status.TxCount, status.RxCount, status.DroppedCount)
	fmt.Fprintf(w, "Free heap: %d bytes\n", status.FreeHeapBytes)
	fmt.Fprintf(w, "Airtime:   %d ms used\n", status.AirtimeUsedMs)
	if status.Position != nil {
		p := status.Position
		fmt.Fprintf(w, "Position:  %.6f, %.6f (±%.0fm, alt %.0fm)\n",
			p.Lat, p.Lon, p.Accuracy, p.Alt)
	}
	return nil
}
