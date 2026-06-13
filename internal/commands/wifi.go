package commands

import (
	"fmt"
	"os"
	"strings"

	bramble "github.com/justinlindh/bramble-go"
	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/output"
)

func newWifiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wifi",
		Short: "Show WiFi status",
		Long:  "Show current WiFi mode and link information.",
		RunE:  runWifiStatus,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show WiFi status",
		RunE:  runWifiStatus,
	})
	return cmd
}

func runWifiStatus(cmd *cobra.Command, args []string) error {
	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	status, err := client.WifiStatus(ctx)
	if err != nil {
		return fmt.Errorf("bramble-cli: get wifi status: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, status)
	}

	fmt.Fprintln(os.Stdout, formatWifiStatus(status))
	return nil
}

func formatWifiStatus(status *bramble.WifiStatus) string {
	parts := []string{
		fmt.Sprintf("Mode: %s", strings.ToUpper(status.Mode)),
		fmt.Sprintf("SSID: %s", status.SSID),
		fmt.Sprintf("IP: %s", status.IP),
	}

	switch strings.ToLower(status.Mode) {
	case "station":
		parts = append(parts, fmt.Sprintf("RSSI: %d dBm", status.RSSI))
	case "ap":
		parts = append(parts, fmt.Sprintf("Clients: %d", status.Clients))
	}

	return strings.Join(parts, " | ")
}
