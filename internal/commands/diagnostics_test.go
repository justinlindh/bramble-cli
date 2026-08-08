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

// healthyRadio is a supported SX1262 with nothing latched.
func healthyRadio() *bramble.DiagnosticsRadioHealth {
	return &bramble.DiagnosticsRadioHealth{
		Supported:       true,
		TxPowerDBm:      22,
		DeviceErrors:    ptr(0),
		DeviceErrorsStr: ptr("none"),
		PARampError:     ptr(false),
		Status:          ptr(0x2C),
		ChipMode:        ptr("STBY_RC"),
		CmdStatus:       ptr("tx-done"),
		OCP:             ptr(0x38),
		OCPExpected:     ptr(0x38),
		OCPOK:           ptr(true),
		PADutyCycle:     ptr(4),
		PAHPMax:         ptr(7),
		PARatedDBm:      ptr(22),
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
		"Programmed TX power",
		"22 dBm (intent, not measurement)",
		"0x0000 (none)",
		"STBY_RC",
		"tx-done",
		"0x38, expected 0x38 (ok)",
		"duty cycle 4, hp max 7, rated 22 dBm",
	)
	mustNotContain(t, out, "PROBLEM")
}

func TestPrintDiagnosticsPretty_RadioPARampError(t *testing.T) {
	t.Parallel()

	rh := healthyRadio()
	rh.DeviceErrors = ptr(0x0100)
	rh.DeviceErrorsStr = ptr("PA_RAMP")
	rh.PARampError = ptr(true)
	d := baseDiagnostics()
	d.RadioHealth = rh

	out := renderDiagnostics(d)
	mustContain(t, out,
		"PROBLEM: PA_RAMP is latched.",
		"nothing usable went on air",
		"0x0100 (PA_RAMP)",
	)
	// The fault has to precede the field dump, not hide inside it.
	if strings.Index(out, "PROBLEM") > strings.Index(out, "Programmed TX power") {
		t.Fatalf("expected the fault line before the field table, got:\n%s", out)
	}
}

func TestPrintDiagnosticsPretty_RadioOCPMismatch(t *testing.T) {
	t.Parallel()

	rh := healthyRadio()
	rh.OCP = ptr(0x18)
	rh.OCPOK = ptr(false)
	d := baseDiagnostics()
	d.RadioHealth = rh

	out := renderDiagnostics(d)
	mustContain(t, out,
		"PROBLEM: the OCP register does not read back what the driver programmed.",
		"PA configuration writes are not reaching the chip",
		"0x18, expected 0x38 (MISMATCH)",
	)
}

func TestPrintDiagnosticsPretty_RadioUnsupported(t *testing.T) {
	t.Parallel()

	d := baseDiagnostics()
	d.RadioHealth = &bramble.DiagnosticsRadioHealth{Supported: false, TxPowerDBm: 17}

	out := renderDiagnostics(d)
	mustContain(t, out,
		"Radio health",
		"17 dBm (intent, not measurement)",
		"No SX1262 to interrogate on this target",
	)
	// No wall of zeros for registers that were never read.
	mustNotContain(t, out,
		"Device errors",
		"Status byte",
		"Chip mode",
		"Last command status",
		"OCP readback",
		"PA operating point",
		"PROBLEM",
	)
}

func TestPrintDiagnosticsPretty_RadioPartialFields(t *testing.T) {
	t.Parallel()

	// A supported radio whose optional readbacks are missing must show the
	// rows it has and drop the rest, never substitute zeros.
	d := baseDiagnostics()
	d.RadioHealth = &bramble.DiagnosticsRadioHealth{
		Supported:  true,
		TxPowerDBm: 22,
		ChipMode:   ptr("RX"),
	}

	out := renderDiagnostics(d)
	mustContain(t, out, "Chip mode", "RX")
	mustNotContain(t, out, "Device errors", "OCP readback", "PA operating point", "Status byte")
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

func TestDeviceErrorNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		errors      int
		firmwareStr *string
		want        string
	}{
		{name: "firmware string wins", errors: 0x0100, firmwareStr: ptr("PA_RAMP"), want: "PA_RAMP"},
		{name: "empty firmware string falls back to local decode", errors: 0x0100, firmwareStr: ptr(""), want: "PA_RAMP"},
		{name: "absent firmware string falls back to local decode", errors: 0x0041, want: "PLL_LOCK RC64K_CALIB"},
		{name: "clear mask", errors: 0, want: "none"},
		{name: "all flags", errors: 0x017F, want: "PA_RAMP PLL_LOCK XOSC_START IMG_CALIB ADC_CALIB PLL_CALIB RC13M_CALIB RC64K_CALIB"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := deviceErrorNames(tc.errors, tc.firmwareStr); got != tc.want {
				t.Fatalf("deviceErrorNames(%#x)=%q want %q", tc.errors, got, tc.want)
			}
		})
	}
}

func TestFormatOCP(t *testing.T) {
	t.Parallel()

	if got := formatOCP(0x38, ptr(0x38), ptr(true)); got != "0x38, expected 0x38 (ok)" {
		t.Fatalf("unexpected ok rendering: %q", got)
	}
	if got := formatOCP(0x18, ptr(0x38), ptr(false)); got != "0x18, expected 0x38 (MISMATCH)" {
		t.Fatalf("unexpected mismatch rendering: %q", got)
	}
	// An absent expected value or verdict must not invent one.
	if got := formatOCP(0x38, nil, nil); got != "0x38" {
		t.Fatalf("unexpected bare rendering: %q", got)
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
