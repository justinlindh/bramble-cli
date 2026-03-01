// Package commands provides all CLI subcommands for the bramble tool.
package commands

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/justinlindh/bramble-cli/internal/discovery"
	bramble "github.com/justinlindh/bramble-go"
	"github.com/justinlindh/bramble-go/transport"
	"github.com/spf13/cobra"
)

var (
	flagPort      string
	flagTransport string
	flagBLE       string
	flagJSON      bool
)

const (
	connectTimeout = 30 * time.Second
	requestTimeout = 10 * time.Second
)

func commandContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), requestTimeout)
}

// rootCmd is the top-level command.
// version is the current bramble-cli version.
const version = "0.1.0"

// rootCmd is the top-level command.
var rootCmd = &cobra.Command{
	Use:     "bramble",
	Version: version,
	Short:   "CLI for Bramble mesh nodes",
	Long: `bramble — command-line interface for Bramble LoRa mesh nodes.

Connects via USB serial (auto-detected or --port), WebSocket (--transport),
or Bluetooth Low Energy (--ble).

Examples:
  bramble status
  bramble --port /dev/ttyUSB0 peers
  bramble --transport ws://192.168.4.1/ws status
  bramble --ble Bramble status
  bramble send DEADBEEF "hello world"
  bramble monitor`,
	SilenceUsage: true,
}

// Execute runs the root command. Call this from main.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&flagPort, "port", "p", "", "serial port path (e.g. /dev/ttyUSB0)")
	rootCmd.PersistentFlags().StringVarP(&flagTransport, "transport", "t", "", "WebSocket transport URL (e.g. ws://192.168.4.1/ws)")
	rootCmd.PersistentFlags().StringVarP(&flagBLE, "ble", "b", "", "BLE device name to scan for (e.g. Bramble)")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "output results as JSON")

	rootCmd.AddCommand(
		newStatusCmd(),
		newDiagnosticsCmd(),
		newWifiCmd(),
		newPeersCmd(),
		newRoutesCmd(),
		newPingCmd(),
		newSendCmd(),
		newBroadcastCmd(),
		newMonitorCmd(),
		newConfigCmd(),
		newChannelsCmd(),
		newProbeCmd(),
		newRebootCmd(),
		newOTACmd(),
		newLocationCmd(),
		newDiscoverCmd(),
		newCompletionCmd(),
		newTrafficCmd(),
		newTUICmd(),
	)
}

// getClient resolves the transport and returns a connected Bramble client.
// Priority: --ble > --transport > --port > auto-detect USB.
func getClient(ctx context.Context) (*bramble.Client, error) {
	var t transport.Transport

	switch {
	case flagBLE != "":
		t = transport.NewBLE(transport.BLEConfig{DeviceName: flagBLE})

	case flagTransport != "":
		t = transport.NewWebSocket(flagTransport)

	case flagPort != "":
		t = transport.NewSerial(flagPort)

	default:
		port, err := discovery.Detect()
		if err != nil {
			return nil, err
		}
		if !flagJSON {
			fmt.Fprintf(os.Stderr, "Auto-detected device: %s\n", port)
		}
		t = transport.NewSerial(port)
	}

	client := bramble.NewClient(t)
	connectCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		connectCtx, cancel = context.WithTimeout(ctx, connectTimeout)
		defer cancel()
	}
	if err := client.Connect(connectCtx); err != nil {
		return nil, fmt.Errorf("bramble-cli: connect: %w", err)
	}
	return client, nil
}
