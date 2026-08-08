package commands

import (
	"bytes"
	"strings"
	"testing"

	bramble "github.com/justinlindh/bramble-go"
)

func TestNewDiagnosticsCmd_Config(t *testing.T) {
	t.Parallel()

	cmd := newDiagnosticsCmd()
	if cmd.Use != "diagnostics" {
		t.Fatalf("unexpected use: %q", cmd.Use)
	}
	if len(cmd.Aliases) == 0 || cmd.Aliases[0] != "diag" {
		t.Fatalf("expected alias diag, got %v", cmd.Aliases)
	}
	if cmd.RunE == nil {
		t.Fatal("expected RunE handler")
	}
	if cmd.Flags().Lookup("heap-dump") == nil {
		t.Fatal("expected --heap-dump flag")
	}
}

func TestPrintDiagnosticsPretty(t *testing.T) {
	t.Parallel()

	d := &bramble.DiagnosticsResponse{
		UptimeS:  1234,
		FreeHeap: 56789,
		Heap: bramble.DiagnosticsHeap{
			InternalFree:             1000,
			InternalMinEverFree:      900,
			InternalLargestFreeBlock: 800,
			DMAFree:                  700,
			DMALargestFreeBlock:      600,
			PSRAMFree:                500,
			PSRAMMinEverFree:         400,
		},
		TaskStackHWM: []bramble.TaskStackHWM{{Task: "main", HWMWords: 128, HWMBytes: 512}},
	}

	var buf bytes.Buffer
	printDiagnosticsPretty(&buf, d)
	out := buf.String()

	checks := []string{"Summary", "Heap regions", "Task stack HWM", "main", "Uptime (s)", "Internal", "DMA", "PSRAM"}
	for _, needle := range checks {
		if !strings.Contains(out, needle) {
			t.Fatalf("expected output to contain %q, got:\n%s", needle, out)
		}
	}
}

// baseDiagnostics is the minimum a node always reports: no radio health, no
// backpressure, no GPS counters.
func baseDiagnostics() *bramble.DiagnosticsResponse {
	return &bramble.DiagnosticsResponse{
		UptimeS:      1234,
		FreeHeap:     56789,
		TaskStackHWM: []bramble.TaskStackHWM{{Task: "main", HWMWords: 128, HWMBytes: 512}},
	}
}

func ptr[T any](v T) *T { return &v }

// healthyRadio is a supported radio reporting every verdict clean.
func healthyRadio() *bramble.DiagnosticsRadioHealth {
	return &bramble.DiagnosticsRadioHealth{
		Supported:        true,
		TxPowerDBm:       22,
		Chip:             ptr("SX1262"),
		PAFault:          ptr(false),
		PLLFault:         ptr(false),
		OscillatorFault:  ptr(false),
		CalibrationFault: ptr(false),
		ConfigVerified:   ptr(true),
		Detail:           ptr("errors [none], mode STBY_RC, cmd tx-done, OCP 0x38, PA duty 0x04 hpMax 0x07 rated 22 dBm"),
	}
}

func renderDiagnostics(d *bramble.DiagnosticsResponse) string {
	var buf bytes.Buffer
	printDiagnosticsPretty(&buf, d)
	return buf.String()
}

func mustContain(t *testing.T, out string, needles ...string) {
	t.Helper()
	for _, needle := range needles {
		if !strings.Contains(out, needle) {
			t.Fatalf("expected output to contain %q, got:\n%s", needle, out)
		}
	}
}

func mustNotContain(t *testing.T, out string, needles ...string) {
	t.Helper()
	for _, needle := range needles {
		if strings.Contains(out, needle) {
			t.Fatalf("expected output not to contain %q, got:\n%s", needle, out)
		}
	}
}

func TestPrintDiagnosticsPretty_RadioHealthy(t *testing.T) {
	t.Parallel()

	d := baseDiagnostics()
	d.RadioHealth = healthyRadio()

	out := renderDiagnostics(d)
	mustContain(t, out,
		"Radio health",
		"SX1262",
		"Programmed TX power",
		"22 dBm (intent, not measurement)",
		"No transmit-path faults reported.",
		"Detail: errors [none], mode STBY_RC, cmd tx-done, OCP 0x38, PA duty 0x04 hpMax 0x07 rated 22 dBm",
	)
	mustNotContain(t, out, "PROBLEM", "WARNING")
}

func TestPrintDiagnosticsPretty_RadioPAFault(t *testing.T) {
	t.Parallel()

	rh := healthyRadio()
	rh.PAFault = ptr(true)
	d := baseDiagnostics()
	d.RadioHealth = rh

	out := renderDiagnostics(d)
	mustContain(t, out,
		"PROBLEM: the power amplifier did not ramp for a transmit, so nothing usable went on air.",
	)
	mustNotContain(t, out, "No transmit-path faults reported.")
	// The fault has to precede the values, not hide under them.
	if strings.Index(out, "PROBLEM") > strings.Index(out, "Programmed TX power") {
		t.Fatalf("expected the fault line before the field table, got:\n%s", out)
	}
}

func TestPrintDiagnosticsPretty_RadioConfigNotVerified(t *testing.T) {
	t.Parallel()

	rh := healthyRadio()
	rh.ConfigVerified = ptr(false)
	d := baseDiagnostics()
	d.RadioHealth = rh

	out := renderDiagnostics(d)
	mustContain(t, out,
		"PROBLEM: configuration writes are not reaching the chip, which caps output well below the commanded level.",
	)
	mustNotContain(t, out, "No transmit-path faults reported.")
}

// A synthesizer that never locked and an oscillator that never started stop
// transmission outright, exactly like a PA that never ramped, so they get the
// blocking label rather than a warning. Reporting a dead radio as a warning is
// the failure this guards against.
func TestPrintDiagnosticsPretty_RadioClockFaultsAreBlocking(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mutate  func(*bramble.DiagnosticsRadioHealth)
		message string
	}{
		{"pll", func(rh *bramble.DiagnosticsRadioHealth) { rh.PLLFault = ptr(true) }, "PROBLEM: the frequency synthesizer did not lock."},
		{"oscillator", func(rh *bramble.DiagnosticsRadioHealth) { rh.OscillatorFault = ptr(true) }, "PROBLEM: the reference oscillator did not start."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rh := healthyRadio()
			tc.mutate(rh)
			d := baseDiagnostics()
			d.RadioHealth = rh

			out := renderDiagnostics(d)
			mustContain(t, out, tc.message)
			mustNotContain(t, out, "WARNING", "No transmit-path faults reported.")
		})
	}
}

func TestPrintDiagnosticsPretty_RadioQuietFaultsWarnRatherThanAlarm(t *testing.T) {
	t.Parallel()

	// A failed calibration is the only fault the radio reports that genuinely
	// costs link budget without stopping a transmit, so it is the only one
	// that earns a warning rather than the blocking label.
	cases := []struct {
		name    string
		mutate  func(*bramble.DiagnosticsRadioHealth)
		message string
	}{
		{"calibration", func(rh *bramble.DiagnosticsRadioHealth) { rh.CalibrationFault = ptr(true) }, "WARNING: a calibration block failed, which costs link budget without failing a transmit outright."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rh := healthyRadio()
			tc.mutate(rh)
			d := baseDiagnostics()
			d.RadioHealth = rh

			out := renderDiagnostics(d)
			mustContain(t, out, tc.message)
			mustNotContain(t, out, "PROBLEM", "No transmit-path faults reported.")
		})
	}
}

func TestPrintDiagnosticsPretty_RadioBlockingAndQuietFaultsTogether(t *testing.T) {
	t.Parallel()

	rh := healthyRadio()
	rh.PAFault = ptr(true)
	rh.ConfigVerified = ptr(false)
	rh.CalibrationFault = ptr(true)
	d := baseDiagnostics()
	d.RadioHealth = rh

	out := renderDiagnostics(d)
	mustContain(t, out,
		"PROBLEM: the power amplifier did not ramp",
		"PROBLEM: configuration writes are not reaching the chip",
		"WARNING: a calibration block failed",
	)
	// Blocking faults are listed before the quieter ones.
	if strings.Index(out, "WARNING") < strings.Index(out, "PROBLEM") {
		t.Fatalf("expected blocking faults before warnings, got:\n%s", out)
	}
}

func TestPrintDiagnosticsPretty_RadioUnsupported(t *testing.T) {
	t.Parallel()

	d := baseDiagnostics()
	d.RadioHealth = &bramble.DiagnosticsRadioHealth{Supported: false, TxPowerDBm: 17}

	out := renderDiagnostics(d)
	mustContain(t, out,
		"Radio health",
		"17 dBm (intent, not measurement)",
		"This target's radio driver cannot report transmit-path evidence",
	)
	// The message must not name a part, since an unsupported payload covers
	// both the emulator's virtual radio and a real part with no mapping yet.
	mustNotContain(t, out, "SX1262")
	// Nothing invented for verdicts that were never reported.
	mustNotContain(t, out,
		"PROBLEM",
		"WARNING",
		"No transmit-path faults reported.",
		"Detail:",
	)
}

func TestPrintDiagnosticsPretty_RadioVerdictsAbsentClaimsNoAllClear(t *testing.T) {
	t.Parallel()

	// Defensive: current firmware cannot produce this shape. All three radio
	// backends either report supported false (the virtual radio, and the
	// LR1110 whose mapping is not implemented) or fill every verdict (the
	// SX1262), and the single RPC emitter gates all the optional fields behind
	// supported, so today supported true implies every verdict is present.
	// This covers a backend that later answers only some of them, and any node
	// the CLI did not build. A supported radio reporting no verdicts must not
	// render as healthy: not checked is not the same as checked and clean.
	d := baseDiagnostics()
	d.RadioHealth = &bramble.DiagnosticsRadioHealth{
		Supported:  true,
		TxPowerDBm: 22,
		Chip:       ptr("SX1262"),
	}

	out := renderDiagnostics(d)
	mustContain(t, out, "Radio health", "SX1262", "22 dBm (intent, not measurement)")
	mustNotContain(t, out, "No transmit-path faults reported.", "PROBLEM", "WARNING", "Detail:")
}

func TestPrintDiagnosticsPretty_RadioPartialVerdicts(t *testing.T) {
	t.Parallel()

	// Also defensive, for the same reason as the case above: no current
	// backend answers partially. One verdict reported clean and the rest
	// absent is still an all-clear for what the node did answer, and must not
	// invent the checks it skipped.
	d := baseDiagnostics()
	d.RadioHealth = &bramble.DiagnosticsRadioHealth{
		Supported:  true,
		TxPowerDBm: 22,
		PAFault:    ptr(false),
	}

	out := renderDiagnostics(d)
	mustContain(t, out, "No transmit-path faults reported.")
	mustNotContain(t, out, "PROBLEM", "WARNING")
}

func TestPrintDiagnosticsPretty_RadioDetailPrintedVerbatim(t *testing.T) {
	t.Parallel()

	// The detail string belongs to the driver: printed as-is, never parsed
	// back into fields.
	detail := "errors [PA_RAMP PLL_LOCK], mode STBY_RC, cmd exec-failed, OCP 0x18, PA duty 0x04 hpMax 0x07 rated 22 dBm"
	rh := healthyRadio()
	rh.Detail = ptr(detail)
	d := baseDiagnostics()
	d.RadioHealth = rh

	out := renderDiagnostics(d)
	mustContain(t, out, "Detail: "+detail)
}

func TestPrintDiagnosticsPretty_RadioDetailAbsent(t *testing.T) {
	t.Parallel()

	rh := healthyRadio()
	rh.Detail = nil
	d := baseDiagnostics()
	d.RadioHealth = rh

	out := renderDiagnostics(d)
	mustContain(t, out, "Radio health")
	mustNotContain(t, out, "Detail:")
}

func TestPrintDiagnosticsPretty_BackpressureAbsentAndPresent(t *testing.T) {
	t.Parallel()

	absent := renderDiagnostics(baseDiagnostics())
	mustNotContain(t, absent, "Backpressure", "Flood relay drops", "Probes accepted")

	d := baseDiagnostics()
	d.Backpressure = &bramble.DiagnosticsBackpressure{
		FloodRelayDrops: 4,
		ProbeIngress: bramble.DiagnosticsProbeIngress{
			Accepted:       12,
			DroppedReply:   3,
			DroppedForward: 7,
		},
	}

	out := renderDiagnostics(d)
	mustContain(t, out,
		"Backpressure",
		"Flood relay drops",
		"Probes accepted",
		"Probes dropped (unanswered)",
		"Probes dropped (not forwarded)",
		"12",
	)
}

func TestPrintDiagnosticsPretty_GPSAbsent(t *testing.T) {
	t.Parallel()

	out := renderDiagnostics(baseDiagnostics())
	mustNotContain(t, out, "GPS feed", "RX bytes", "Chip banner")
}

func TestPrintDiagnosticsPretty_GPSPresentWithZeroCounters(t *testing.T) {
	t.Parallel()

	// Zero rx bytes on a GPS board is a real reading (a dead UART link), so
	// it must render, unlike a board with no GPS at all.
	d := baseDiagnostics()
	d.GPSRxBytes = ptr(0.0)
	d.GPSRxLines = ptr(0.0)
	d.GPSChip = ptr("")
	d.GPSRxOverruns = ptr(0.0)
	d.GPSRxErrors = ptr(0.0)
	d.GPSRxDisabled = ptr(0.0)
	d.GPSRxRearmFail = ptr(0.0)

	out := renderDiagnostics(d)
	mustContain(t, out,
		"GPS feed",
		"RX bytes",
		"RX lines",
		"Chip banner",
		"(no banner seen)",
		"RX overruns",
		"RX errors",
		"RX disabled",
		"RX re-arm failures",
	)
}

func TestPrintDiagnosticsPretty_GPSChipBanner(t *testing.T) {
	t.Parallel()

	d := baseDiagnostics()
	d.GPSRxBytes = ptr(98765.0)
	d.GPSRxLines = ptr(4321.0)
	d.GPSChip = ptr("$PAIR021,AG3335M_V2.6.10")

	out := renderDiagnostics(d)
	mustContain(t, out, "GPS feed", "98765", "4321", "$PAIR021,AG3335M_V2.6.10")
	// Counters the firmware did not send stay off the table.
	mustNotContain(t, out, "RX overruns", "RX re-arm failures")
}

func TestRadioFaultTriggered(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		verdict   *bool
		faultWhen bool
		want      bool
	}{
		{name: "fault-on-true reported true", verdict: ptr(true), faultWhen: true, want: true},
		{name: "fault-on-true reported false", verdict: ptr(false), faultWhen: true, want: false},
		{name: "fault-on-false reported false", verdict: ptr(false), faultWhen: false, want: true},
		{name: "fault-on-false reported true", verdict: ptr(true), faultWhen: false, want: false},
		{name: "fault-on-true absent", verdict: nil, faultWhen: true, want: false},
		// An absent verdict must never fire a fault, which is the whole reason
		// these arrive as pointers: a missing key decoding to false would
		// otherwise report a config-verification failure the node never gave.
		{name: "fault-on-false absent", verdict: nil, faultWhen: false, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := radioFault{verdict: tc.verdict, faultWhen: tc.faultWhen}
			if got := f.triggered(); got != tc.want {
				t.Fatalf("triggered()=%v want %v", got, tc.want)
			}
		})
	}
}

func TestTrimFloat(t *testing.T) {
	t.Parallel()
	if got := trimFloat(42.0); got != "42" {
		t.Fatalf("trimFloat(42.0)=%q want 42", got)
	}
	if got := trimFloat(42.5); got != "42.5" {
		t.Fatalf("trimFloat(42.5)=%q want 42.5", got)
	}
}
