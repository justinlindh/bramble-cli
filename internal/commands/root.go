// Package commands provides all CLI subcommands for the bramble tool.
package commands

import (
	"context"
	"fmt"
	"os"
	"time"

	bramble "github.com/justinlindh/bramble-go"
	"github.com/justinlindh/bramble-go/transport"
	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/discovery"
)

var (
	flagPort      string
	flagTransport string
	flagBLE       string
	flagAuthToken string
	flagJSON      bool
	flagTimeout   time.Duration
)

const (
	connectTimeout = 30 * time.Second
	requestTimeout = 10 * time.Second
	// bleTimeout provides enough headroom for BLE scan (≤10s) + connect + RPC.
	bleTimeout = 45 * time.Second
)

// commandContext returns a context with an appropriate deadline for the active
// transport.  Resolution order:
//  1. --timeout flag (explicit user override)
//  2. BLE transport: bleTimeout (45s) — scan alone can take up to 10s
//  3. All other transports: requestTimeout (10s)
func commandContext() (context.Context, context.CancelFunc) {
	d := effectiveTimeout()
	return context.WithTimeout(context.Background(), d)
}

// effectiveTimeout returns the deadline duration that commandContext should use.
// It is a pure function of package-level flag state, making it straightforward
// to test without starting a real cobra command.
func effectiveTimeout() time.Duration {
	if flagTimeout > 0 {
		return flagTimeout
	}
	if flagBLE != "" {
		return bleTimeout
	}
	return requestTimeout
}

// version is the current bramble-cli version.
// It defaults to "dev" and is meant to be overridden at build time via:
//
//	-ldflags "-X github.com/justinlindh/bramble-cli/internal/commands.version=<version>"
var version = "dev"

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
	rootCmd.PersistentFlags().StringVar(&flagAuthToken, "token", "", "Auth token for node connection")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "output results as JSON")
	rootCmd.PersistentFlags().DurationVar(&flagTimeout, "timeout", 0, "override command timeout (e.g. 30s, 1m); default is 45s for BLE, 10s otherwise")

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
		newMeshTestCmd(),
		newCompletionCmd(),
		newTrafficCmd(),
		newTUICmd(),
		newPairCmd(),
		newAuthCmd(),
	)
}

func resolvedAuthToken() string {
	if flagAuthToken != "" {
		return flagAuthToken
	}
	return os.Getenv("BRAMBLE_TOKEN")
}

func applyAuthToken(t transport.Transport) {
	token := resolvedAuthToken()
	if token == "" {
		return
	}
	t.SetAuthToken(token)
}

// getClient resolves the transport and returns a connected Bramble client.
// Priority: --ble > --transport > --port > auto-detect USB.
func getClient(ctx context.Context) (*bramble.Client, error) {
	var t transport.Transport

	switch {
	case flagBLE != "":
		t = transport.NewBLE(flagBLE)

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

	applyAuthToken(t)
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
