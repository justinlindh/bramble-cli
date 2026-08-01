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
		Short: "Show or configure WiFi",
		Long:  "Show current WiFi mode and link information, or provision station credentials.",
		RunE:  runWifiStatus,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show WiFi status",
		RunE:  runWifiStatus,
	})
	cmd.AddCommand(newWifiSetCmd())
	return cmd
}

const (
	wifiSSIDMaxLen     = 32
	wifiPasswordMaxLen = 64
)

func newWifiSetCmd() *cobra.Command {
	var password string
	var open bool
	var reboot bool

	cmd := &cobra.Command{
		Use:   "set <ssid>",
		Short: "Provision WiFi station credentials",
		Long: `Provision WiFi station credentials on the node.

Credentials are persisted to the node's NVS store but do not take effect
until the node reboots. Pass --reboot to reboot immediately, or run
"bramble reboot" yourself afterwards.

The password is never accepted as a positional argument, only via
--password or an interactive prompt: a bare command-line argument is
visible to every other process on the machine (ps, /proc, shell history),
while a flag or prompt is not logged or echoed. If the terminal is
interactive and --password is omitted, you are prompted with input hidden.
Use --open for a network with no password.

Examples:
  bramble wifi set "Home Network"                 # prompts for password
  bramble wifi set "Home Network" --password "hunter2"
  bramble wifi set "Guest Network" --open
  bramble wifi set "Home Network" --open --reboot # apply immediately`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWifiSet(cmd, args, password, open, reboot)
		},
	}
	cmd.Flags().StringVar(&password, "password", "", "WPA2-PSK passphrase (avoid in scripts/shell history; omit to be prompted)")
	cmd.Flags().BoolVar(&open, "open", false, "provision an open network (no password)")
	cmd.Flags().BoolVar(&reboot, "reboot", false, "reboot the node immediately to apply the new credentials")
	return cmd
}

func runWifiSet(cmd *cobra.Command, args []string, password string, open, reboot bool) error {
	ssid := args[0]
	if len(ssid) < 1 || len(ssid) > wifiSSIDMaxLen {
		return fmt.Errorf("bramble-cli: ssid %q invalid: must be 1-%d characters", ssid, wifiSSIDMaxLen)
	}

	passwordChanged := cmd.Flags().Changed("password")

	switch {
	case passwordChanged && open && password != "":
		return fmt.Errorf("bramble-cli: --open and --password are mutually exclusive")
	case passwordChanged && !open && password == "":
		return fmt.Errorf("bramble-cli: --password \"\" provisions an open network; pass --open to confirm")
	case !passwordChanged && !open:
		if !isInteractive() {
			return fmt.Errorf("bramble-cli: --password required (or --open for no password) when not running interactively")
		}
		entered, err := promptSecret(cmd, fmt.Sprintf("Password for %q (leave blank for open network): ", ssid))
		if err != nil {
			return fmt.Errorf("bramble-cli: %w", err)
		}
		password = entered
	}

	if len(password) > wifiPasswordMaxLen {
		return fmt.Errorf("bramble-cli: password too long: must be at most %d characters", wifiPasswordMaxLen)
	}

	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	resp, err := client.SetWifiConfig(ctx, ssid, password)
	if err != nil {
		return fmt.Errorf("bramble-cli: set wifi config: %w", err)
	}

	rebooted := false
	if resp.Applied == "reboot_required" && reboot {
		if err := client.Reboot(ctx); err != nil {
			return fmt.Errorf("bramble-cli: set wifi config: credentials saved but reboot failed: %w", err)
		}
		rebooted = true
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, WifiSetResult{SSID: ssid, Applied: resp.Applied, Rebooted: rebooted})
	}

	fmt.Fprintf(os.Stdout, "WiFi credentials set for %q.\n", ssid)
	switch {
	case rebooted:
		fmt.Fprintln(os.Stdout, "Node rebooting to apply new credentials...")
	case resp.Applied == "reboot_required":
		fmt.Fprintln(os.Stdout, "Reboot required to apply: run \"bramble reboot\".")
	case resp.Applied == "live":
		fmt.Fprintln(os.Stdout, "Applied immediately.")
	}
	return nil
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
