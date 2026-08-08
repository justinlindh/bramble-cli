package commands

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	bramble "github.com/justinlindh/bramble-go"
	"github.com/justinlindh/bramble-go/transport"
	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/discovery"
	"github.com/justinlindh/bramble-cli/internal/output"
)

// fleetNode is one row of the sweep. Every field after Port is best effort:
// a node that answers getStatus but not getBattery still belongs in the table,
// because the point of a sweep is seeing the whole bench at once, and dropping
// a node because one of its optional queries failed hides exactly the node
// most likely to be the problem.
type fleetNode struct {
	Port     string `json:"port"`
	Address  string `json:"address,omitempty"`
	Name     string `json:"name,omitempty"`
	Hardware string `json:"hardware,omitempty"`
	Firmware string `json:"firmware_version,omitempty"`
	UptimeS  int    `json:"uptime_s,omitempty"`
	Peers    int    `json:"peers,omitempty"`
	RadioOk  bool   `json:"radio_ok,omitempty"`
	GPS      string `json:"gps,omitempty"`
	Battery  string `json:"battery,omitempty"`
	Error    string `json:"error,omitempty"`
}

func newFleetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fleet",
		Short: "Status sweep across every attached node",
		Long: `Query every USB serial device at once and print one row per node.

Scans /dev/ttyUSB* and /dev/ttyACM*, connects to each in parallel, and reports
address, node name, hardware, firmware, uptime, peer count, GPS state and
battery. A port that does not answer gets a row with the error rather than
being dropped, since a node that stopped answering is usually the one you are
looking for.

Examples:
  bramble fleet
  bramble fleet --json
  bramble fleet --ports /dev/ttyUSB0,/dev/ttyUSB1`,
		RunE: runFleet,
	}
	cmd.Flags().StringSlice("ports", nil, "explicit port list (default: every /dev/ttyUSB* and /dev/ttyACM*)")
	cmd.Flags().Duration("per-node-timeout", 15*time.Second, "how long to give each node")
	return cmd
}

func runFleet(cmd *cobra.Command, args []string) error {
	ports, _ := cmd.Flags().GetStringSlice("ports")
	perNode, _ := cmd.Flags().GetDuration("per-node-timeout")

	// An explicitly empty --ports is a caller bug, not a request to sweep the
	// bench: cobra parses `--ports ""` to an empty slice, indistinguishable by
	// length alone from the flag being omitted. A script that builds the list
	// from an empty set would otherwise silently touch every attached node.
	if cmd.Flags().Changed("ports") && len(ports) == 0 {
		return fmt.Errorf("--ports was given an empty list; omit the flag to sweep every attached node")
	}

	if len(ports) == 0 {
		if flagPort != "" {
			ports = []string{flagPort}
		} else {
			detected, err := discovery.List()
			if err != nil {
				return err
			}
			ports = detected
		}
	}
	if len(ports) == 0 {
		if flagJSON {
			return output.PrintJSON(cmd.OutOrStdout(), []fleetNode{})
		}
		fmt.Fprintln(cmd.OutOrStdout(), "No USB serial devices found.")
		return nil
	}

	nodes := sweepFleet(context.Background(), ports, perNode, probeNodeOverSerial)

	if flagJSON {
		return output.PrintJSON(cmd.OutOrStdout(), nodes)
	}
	printFleetTable(cmd, nodes)
	return nil
}

// fleetProbe queries one port. Indirected so sweepFleet is testable without
// any serial hardware.
type fleetProbe func(ctx context.Context, port string) fleetNode

// sweepFleet probes every port concurrently and returns the rows in port order.
// Concurrency is the point: a serial connect plus a handful of RPCs takes long
// enough that a seven-node bench queried serially is a minute of waiting, and
// one wedged node would stall every node behind it.
func sweepFleet(ctx context.Context, ports []string, perNode time.Duration, probe fleetProbe) []fleetNode {
	results := make([]fleetNode, len(ports))
	var wg sync.WaitGroup

	for i, port := range ports {
		wg.Add(1)
		go func() {
			defer wg.Done()
			nodeCtx, cancel := context.WithTimeout(ctx, perNode)
			defer cancel()
			results[i] = probe(nodeCtx, port)
		}()
	}
	wg.Wait()

	sort.Slice(results, func(a, b int) bool { return results[a].Port < results[b].Port })
	return results
}

// probeNodeOverSerial connects to one port and collects what it can.
func probeNodeOverSerial(ctx context.Context, port string) fleetNode {
	node := fleetNode{Port: port}

	t := transport.NewSerial(port)
	applyAuthToken(t)
	client := bramble.NewClient(t)
	if err := client.Connect(ctx); err != nil {
		node.Error = err.Error()
		return node
	}
	defer func() { _ = client.Close() }()

	status, err := client.Status(ctx)
	if err != nil {
		node.Error = err.Error()
		return node
	}
	node.Address = status.Address
	node.Hardware = status.Hardware
	node.Firmware = status.FirmwareVersion
	node.UptimeS = status.UptimeSec
	node.Peers = status.Peers
	node.RadioOk = status.RadioOk

	// The rest is best effort. A board with no GPS or no fuel gauge answers
	// with an error, which is information about the board, not a failed sweep.
	if cfg, err := client.Config(ctx); err == nil {
		node.Name = cfg.NodeName
	}
	if pos, err := client.GPSPosition(ctx); err == nil {
		node.GPS = describeGPS(pos)
	}
	if bat, err := client.Battery(ctx); err == nil {
		node.Battery = describeBattery(bat)
	}
	return node
}

func describeGPS(pos *bramble.GPSPosition) string {
	if pos == nil {
		return ""
	}
	if !pos.Valid {
		return "no fix"
	}
	return "fix"
}

func describeBattery(bat *bramble.BatteryStatus) string {
	if bat == nil {
		return ""
	}
	return fmt.Sprintf("%d%% (%dmV)", bat.Percentage, bat.VoltageMV)
}

func printFleetTable(cmd *cobra.Command, nodes []fleetNode) {
	headers := []string{"PORT", "ADDRESS", "NAME", "HARDWARE", "FIRMWARE", "UPTIME", "PEERS", "RADIO", "GPS", "BATTERY"}
	rows := make([][]string, 0, len(nodes))
	failed := 0

	for _, n := range nodes {
		if n.Error != "" {
			failed++
			rows = append(rows, []string{n.Port, "-", "-", "-", "-", "-", "-", "-", "-", "-"})
			continue
		}
		rows = append(rows, []string{
			n.Port,
			n.Address,
			dashIfEmpty(n.Name),
			dashIfEmpty(n.Hardware),
			dashIfEmpty(n.Firmware),
			output.FormatUptime(n.UptimeS),
			strconv.Itoa(n.Peers),
			boolStr(n.RadioOk, "OK", "ERROR"),
			dashIfEmpty(n.GPS),
			dashIfEmpty(n.Battery),
		})
	}
	output.Table(cmd.OutOrStdout(), headers, rows)

	// The errors go under the table rather than into a column: they are long,
	// and truncating them into a cell would lose the part that says why.
	if failed > 0 {
		fmt.Fprintln(cmd.ErrOrStderr())
		for _, n := range nodes {
			if n.Error != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", n.Port, n.Error)
			}
		}
	}
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
