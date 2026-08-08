package commands

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	bramble "github.com/justinlindh/bramble-go"
	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/output"
)

var diagnosticsHeapDump bool

func newDiagnosticsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "diagnostics",
		Aliases: []string{"diag"},
		Short:   "Show runtime diagnostics (heap, task stacks, radio health)",
		Long: `Display runtime diagnostics from the connected node.

Covers heap region stats, per-task stack high-water marks, the radio's
self-reported transmit-path health, airtime backpressure counters and the
GNSS raw-feed counters. Sections the firmware does not report are omitted.`,
		RunE: runDiagnostics,
	}
	cmd.Flags().BoolVar(&diagnosticsHeapDump, "heap-dump", false, "request firmware heap_caps_dump() to serial log before returning diagnostics")
	return cmd
}

func runDiagnostics(cmd *cobra.Command, args []string) error {
	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	d, err := client.Diagnostics(ctx, diagnosticsHeapDump)
	if err != nil {
		return fmt.Errorf("bramble-cli: get diagnostics: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, d)
	}

	printDiagnosticsPretty(os.Stdout, d)
	return nil
}

func printDiagnosticsPretty(w io.Writer, d *bramble.DiagnosticsResponse) {
	fmt.Fprintln(w, "Summary")
	output.Table(w,
		[]string{"Metric", "Value"},
		[][]string{
			{"Uptime (s)", fmt.Sprintf("%.0f", d.UptimeS)},
			{"Free heap", fmt.Sprintf("%.0f", d.FreeHeap)},
		},
	)
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Heap regions")
	output.Table(w,
		[]string{"Region", "Free", "Min ever free", "Largest free block"},
		[][]string{
			{"Internal", fmt.Sprintf("%.0f", d.Heap.InternalFree), fmt.Sprintf("%.0f", d.Heap.InternalMinEverFree), fmt.Sprintf("%.0f", d.Heap.InternalLargestFreeBlock)},
			{"DMA", fmt.Sprintf("%.0f", d.Heap.DMAFree), "-", fmt.Sprintf("%.0f", d.Heap.DMALargestFreeBlock)},
			{"PSRAM", fmt.Sprintf("%.0f", d.Heap.PSRAMFree), fmt.Sprintf("%.0f", d.Heap.PSRAMMinEverFree), "-"},
		},
	)
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Task stack HWM")
	rows := make([][]string, 0, len(d.TaskStackHWM))
	for _, h := range d.TaskStackHWM {
		rows = append(rows, []string{h.Task, trimFloat(h.HWMWords), trimFloat(h.HWMBytes)})
	}
	if len(rows) == 0 {
		rows = append(rows, []string{"(none)", "-", "-"})
	}
	output.Table(w, []string{"Task", "HWM (words)", "HWM (bytes)"}, rows)

	printRadioHealth(w, d.RadioHealth)
	printBackpressure(w, d.Backpressure)
	printGPSFeed(w, d)
}

// printRadioHealth renders the radio's self-reported transmit-path evidence.
// The section is skipped entirely when the firmware omitted it, because an
// absent section and a healthy one must not look the same.
func printRadioHealth(w io.Writer, rh *bramble.DiagnosticsRadioHealth) {
	if rh == nil {
		return
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Radio health")

	// Lead with the two readings that mean something is actually broken, so
	// they are not buried in the field dump below.
	faults := 0
	if rh.PARampError != nil && *rh.PARampError {
		fmt.Fprintln(w, "PROBLEM: PA_RAMP is latched. The power amplifier did not ramp for a transmit, so nothing usable went on air.")
		faults++
	}
	if rh.OCPOK != nil && !*rh.OCPOK {
		fmt.Fprintln(w, "PROBLEM: the OCP register does not read back what the driver programmed. PA configuration writes are not reaching the chip, which caps output well below the commanded level.")
		faults++
	}
	if faults > 0 {
		fmt.Fprintln(w)
	}

	rows := [][]string{
		{"Programmed TX power", fmt.Sprintf("%d dBm (intent, not measurement)", rh.TxPowerDBm)},
	}

	if !rh.Supported {
		output.Table(w, []string{"Metric", "Value"}, rows)
		fmt.Fprintln(w, "No SX1262 to interrogate on this target, so chip-level transmit evidence is unavailable.")
		return
	}

	if rh.DeviceErrors != nil {
		rows = append(rows, []string{"Device errors", fmt.Sprintf("0x%04X (%s)", uint16(*rh.DeviceErrors), deviceErrorNames(*rh.DeviceErrors, rh.DeviceErrorsStr))})
	}
	if rh.Status != nil {
		rows = append(rows, []string{"Status byte", fmt.Sprintf("0x%02X", uint8(*rh.Status))})
	}
	if rh.ChipMode != nil {
		rows = append(rows, []string{"Chip mode", *rh.ChipMode})
	}
	if rh.CmdStatus != nil {
		rows = append(rows, []string{"Last command status", *rh.CmdStatus})
	}
	if rh.OCP != nil {
		rows = append(rows, []string{"OCP readback", formatOCP(*rh.OCP, rh.OCPExpected, rh.OCPOK)})
	}
	if paPoint := formatPAOperatingPoint(rh); paPoint != "" {
		rows = append(rows, []string{"PA operating point", paPoint})
	}

	output.Table(w, []string{"Metric", "Value"}, rows)
}

// sx1262DeviceErrorFlags maps the GetDeviceErrors bitmask to flag names, in
// the same order the firmware renders them.
var sx1262DeviceErrorFlags = []struct {
	bit  int
	name string
}{
	{1 << 8, "PA_RAMP"},
	{1 << 6, "PLL_LOCK"},
	{1 << 5, "XOSC_START"},
	{1 << 4, "IMG_CALIB"},
	{1 << 3, "ADC_CALIB"},
	{1 << 2, "PLL_CALIB"},
	{1 << 1, "RC13M_CALIB"},
	{1 << 0, "RC64K_CALIB"},
}

// deviceErrorNames names the set bits of a GetDeviceErrors mask. The firmware
// already decodes them, so its string wins when present; decoding locally
// keeps a raw mask readable when it is not.
func deviceErrorNames(errors int, firmwareStr *string) string {
	if firmwareStr != nil && *firmwareStr != "" {
		return *firmwareStr
	}
	names := make([]string, 0, len(sx1262DeviceErrorFlags))
	for _, f := range sx1262DeviceErrorFlags {
		if errors&f.bit != 0 {
			names = append(names, f.name)
		}
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, " ")
}

func formatOCP(ocp int, expected *int, ok *bool) string {
	s := fmt.Sprintf("0x%02X", uint8(ocp))
	if expected != nil {
		s += fmt.Sprintf(", expected 0x%02X", uint8(*expected))
	}
	switch {
	case ok == nil:
	case *ok:
		s += " (ok)"
	default:
		s += " (MISMATCH)"
	}
	return s
}

func formatPAOperatingPoint(rh *bramble.DiagnosticsRadioHealth) string {
	parts := make([]string, 0, 3)
	if rh.PADutyCycle != nil {
		parts = append(parts, fmt.Sprintf("duty cycle %d", *rh.PADutyCycle))
	}
	if rh.PAHPMax != nil {
		parts = append(parts, fmt.Sprintf("hp max %d", *rh.PAHPMax))
	}
	if rh.PARatedDBm != nil {
		parts = append(parts, fmt.Sprintf("rated %d dBm", *rh.PARatedDBm))
	}
	return strings.Join(parts, ", ")
}

// printBackpressure renders the airtime backpressure counters, which record
// load the node shed rather than transmitting.
func printBackpressure(w io.Writer, bp *bramble.DiagnosticsBackpressure) {
	if bp == nil {
		return
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Backpressure")
	output.Table(w,
		[]string{"Counter", "Value"},
		[][]string{
			{"Flood relay drops", trimFloat(bp.FloodRelayDrops)},
			{"Probes accepted", trimFloat(bp.ProbeIngress.Accepted)},
			{"Probes dropped (unanswered)", trimFloat(bp.ProbeIngress.DroppedReply)},
			{"Probes dropped (not forwarded)", trimFloat(bp.ProbeIngress.DroppedForward)},
		},
	)
}

// printGPSFeed renders the GNSS raw-feed counters. Boards without GPS omit
// every one of them, and the section is skipped rather than shown as zeros.
func printGPSFeed(w io.Writer, d *bramble.DiagnosticsResponse) {
	rows := make([][]string, 0, 7)
	appendCount := func(label string, v *float64) {
		if v != nil {
			rows = append(rows, []string{label, trimFloat(*v)})
		}
	}

	appendCount("RX bytes", d.GPSRxBytes)
	appendCount("RX lines", d.GPSRxLines)
	if d.GPSChip != nil {
		chip := *d.GPSChip
		if chip == "" {
			chip = "(no banner seen)"
		}
		rows = append(rows, []string{"Chip banner", chip})
	}
	appendCount("RX overruns", d.GPSRxOverruns)
	appendCount("RX errors", d.GPSRxErrors)
	appendCount("RX disabled", d.GPSRxDisabled)
	appendCount("RX re-arm failures", d.GPSRxRearmFail)

	if len(rows) == 0 {
		return
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "GPS feed")
	output.Table(w, []string{"Counter", "Value"}, rows)
}

func trimFloat(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}
