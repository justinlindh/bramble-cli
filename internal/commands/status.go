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

	status, err := client.Status(ctx)
	if err != nil {
		return fmt.Errorf("get status: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, status)
	}

	w := os.Stdout
	fmt.Fprintf(w, "Address:   %s\n", status.Address)
	fmt.Fprintf(w, "Firmware:  %s\n", status.FirmwareVersion)
	fmt.Fprintf(w, "Protocol:  %s\n", status.ProtocolVersion)
	fmt.Fprintf(w, "Hardware:  %s\n", status.Hardware)
	fmt.Fprintf(w, "Radio:     %s\n", boolStr(status.RadioOk, "OK", "ERROR"))
	fmt.Fprintf(w, "Uptime:    %s\n", output.FormatUptime(status.UptimeSec))
	fmt.Fprintf(w, "Peers:     %d\n", status.Peers)
	fmt.Fprintf(w, "Beacons:   %d TX / %d RX\n", status.BeaconTx, status.BeaconRx)
	fmt.Fprintf(w, "Packets:   %d TX / %d RX\n", status.PacketsTx, status.PacketsRx)
	return nil
}

func boolStr(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}
