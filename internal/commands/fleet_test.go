package commands

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	bramble "github.com/justinlindh/bramble-go"
)

func TestSweepFleetReturnsRowsInPortOrder(t *testing.T) {
	// The probes finish in whatever order they finish, so the sweep has to
	// order the output itself or the table shuffles between runs and two runs
	// cannot be diffed.
	ports := []string{"/dev/ttyUSB2", "/dev/ttyACM0", "/dev/ttyUSB1"}
	probe := func(_ context.Context, port string) fleetNode {
		if port == "/dev/ttyACM0" {
			time.Sleep(20 * time.Millisecond)
		}
		return fleetNode{Port: port, Address: strings.ToUpper(port)}
	}

	got := sweepFleet(context.Background(), ports, time.Second, probe)
	want := []string{"/dev/ttyACM0", "/dev/ttyUSB1", "/dev/ttyUSB2"}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Port != want[i] {
			t.Errorf("row %d is %s, want %s", i, got[i].Port, want[i])
		}
	}
}

func TestSweepFleetProbesConcurrently(t *testing.T) {
	// A serial sweep of a seven node bench is a minute of waiting, and one
	// wedged node would stall every node behind it.
	const n = 5
	var mu sync.Mutex
	inFlight, peak := 0, 0

	probe := func(_ context.Context, port string) fleetNode {
		mu.Lock()
		inFlight++
		if inFlight > peak {
			peak = inFlight
		}
		mu.Unlock()

		time.Sleep(30 * time.Millisecond)

		mu.Lock()
		inFlight--
		mu.Unlock()
		return fleetNode{Port: port}
	}

	ports := make([]string, n)
	for i := range ports {
		ports[i] = string(rune('a'+i)) + "-port"
	}
	sweepFleet(context.Background(), ports, time.Second, probe)

	if peak < n {
		t.Errorf("peak concurrency was %d, want %d: the sweep is running serially", peak, n)
	}
}

func TestSweepFleetKeepsFailedNodesInTheResult(t *testing.T) {
	// Dropping a port that did not answer hides the node most likely to be the
	// problem, which is the opposite of what a status sweep is for.
	probe := func(_ context.Context, port string) fleetNode {
		if port == "/dev/ttyUSB1" {
			return fleetNode{Port: port, Error: "connect: permission denied"}
		}
		return fleetNode{Port: port, Address: "DEADBEEF"}
	}

	got := sweepFleet(context.Background(), []string{"/dev/ttyUSB0", "/dev/ttyUSB1"}, time.Second, probe)
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}
	if got[1].Error == "" {
		t.Error("the failing port lost its error")
	}
}

func TestSweepFleetGivesEachNodeItsOwnDeadline(t *testing.T) {
	// One slow node must not consume the budget of the others.
	var deadlines []time.Time
	var mu sync.Mutex

	probe := func(ctx context.Context, port string) fleetNode {
		dl, ok := ctx.Deadline()
		if !ok {
			t.Error("probe context had no deadline")
		}
		mu.Lock()
		deadlines = append(deadlines, dl)
		mu.Unlock()
		return fleetNode{Port: port}
	}

	sweepFleet(context.Background(), []string{"a", "b"}, 50*time.Millisecond, probe)
	if len(deadlines) != 2 {
		t.Fatalf("recorded %d deadlines, want 2", len(deadlines))
	}
}

func TestSweepFleetEmptyPortListIsEmptyResult(t *testing.T) {
	got := sweepFleet(context.Background(), nil, time.Second, func(context.Context, string) fleetNode {
		t.Fatal("probe was called with no ports")
		return fleetNode{}
	})
	if len(got) != 0 {
		t.Errorf("got %d rows, want 0", len(got))
	}
}

// An explicitly empty --ports must fail rather than fall through to the
// auto-detect path. cobra parses `--ports ""` to an empty slice, so length
// alone cannot tell it apart from the flag being omitted, and a scripted
// caller that built the list from an empty set would sweep the whole bench.
func TestFleetRejectsExplicitlyEmptyPorts(t *testing.T) {
	cmd := newFleetCmd()
	cmd.SetArgs([]string{"--ports", ""})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	err := cmd.Execute()
	if err == nil {
		t.Fatal("--ports \"\" was accepted; it must not fall through to auto-detect")
	}
	if !strings.Contains(err.Error(), "empty list") {
		t.Errorf("error %q does not explain that the list was empty", err)
	}
}

// The omitted flag keeps its meaning: no --ports at all is the auto-detect
// request, and must not trip the guard above.
func TestFleetOmittedPortsIsNotRejected(t *testing.T) {
	cmd := newFleetCmd()
	if cmd.Flags().Changed("ports") {
		t.Fatal("ports reported as changed before any parse")
	}
	if err := cmd.Flags().Parse(nil); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cmd.Flags().Changed("ports") {
		t.Error("ports reported as changed when the flag was never passed")
	}
}

func TestDescribeGPS(t *testing.T) {
	if got := describeGPS(&bramble.GPSPosition{Valid: true}); got != "fix" {
		t.Errorf("valid fix described as %q", got)
	}
	if got := describeGPS(&bramble.GPSPosition{Valid: false}); got != "no fix" {
		t.Errorf("invalid fix described as %q", got)
	}
	if got := describeGPS(nil); got != "" {
		t.Errorf("nil described as %q, want empty so the column shows a dash", got)
	}
}

func TestDescribeBattery(t *testing.T) {
	if got := describeBattery(&bramble.BatteryStatus{Percentage: 82, VoltageMV: 3950}); got != "82% (3950mV)" {
		t.Errorf("got %q", got)
	}
	if got := describeBattery(nil); got != "" {
		t.Errorf("nil described as %q, want empty so the column shows a dash", got)
	}
}
