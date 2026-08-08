package commands

import (
	"fmt"
	"io"
	"os"
	"strconv"

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

// radioFault pairs a verdict from the node with the sentence a reader needs to
// act on it. The verdicts are generic by design, so the wording explains the
// consequence rather than naming any one part's registers.
type radioFault struct {
	// verdict is the node's answer, nil when it did not report this check.
	verdict *bool
	// faultWhen is the value that means something is wrong: PAFault reports a
	// fault as true, ConfigVerified reports one as false.
	faultWhen bool
	// blocking marks a fault that stops usable transmission outright, as
	// opposed to one that quietly costs link budget.
	blocking bool
	message  string
}

func (f radioFault) triggered() bool {
	return f.verdict != nil && *f.verdict == f.faultWhen
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

	rows := [][]string{}
	if rh.Chip != nil && *rh.Chip != "" {
		rows = append(rows, []string{"Radio", *rh.Chip})
	}
	rows = append(rows, []string{"Programmed TX power", fmt.Sprintf("%d dBm (intent, not measurement)", rh.TxPowerDBm)})

	if !rh.Supported {
		output.Table(w, []string{"Metric", "Value"}, rows)
		fmt.Fprintln(w, "This target's radio driver cannot report transmit-path evidence, so only the programmed power is available.")
		return
	}

	printRadioVerdicts(w, rh)
	output.Table(w, []string{"Metric", "Value"}, rows)

	// Chip-specific supporting values, printed verbatim on their own line. The
	// format belongs to the driver and differs per radio part, so it is never
	// parsed and never split back into fields.
	if rh.Detail != nil && *rh.Detail != "" {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Detail: %s\n", *rh.Detail)
	}
}

// printRadioVerdicts leads with what is wrong, so a fault is not buried under
// the values below it. Faults that stop transmission outright are separated
// from the quieter ones that only cost link budget.
func printRadioVerdicts(w io.Writer, rh *bramble.DiagnosticsRadioHealth) {
	checks := []radioFault{
		{rh.PAFault, true, true, "the power amplifier did not ramp for a transmit, so nothing usable went on air."},
		{rh.ConfigVerified, false, true, "configuration writes are not reaching the chip, which caps output well below the commanded level."},
		// A synthesizer that never locked and a reference oscillator that never
		// started are not degradations: the part has no usable clock or no
		// frequency to sit on, so nothing goes out at all. They are rarer than
		// a PA fault, not milder, and reporting them as warnings invites
		// exactly the wrong reaction from someone reading a dead node's output.
		{rh.PLLFault, true, true, "the frequency synthesizer did not lock."},
		{rh.OscillatorFault, true, true, "the reference oscillator did not start."},
		{rh.CalibrationFault, true, false, "a calibration block failed, which costs link budget without failing a transmit outright."},
	}

	reported, triggered := 0, 0
	for _, c := range checks {
		if c.verdict != nil {
			reported++
		}
		if !c.triggered() {
			continue
		}
		triggered++
		label := "WARNING"
		if c.blocking {
			label = "PROBLEM"
		}
		fmt.Fprintf(w, "%s: %s\n", label, c.message)
	}

	switch {
	case triggered > 0:
		fmt.Fprintln(w)
	case reported > 0:
		// Only claim an all-clear the node actually gave. With every verdict
		// absent, silence here means "not checked", not "checked and clean".
		fmt.Fprintln(w, "No transmit-path faults reported.")
		fmt.Fprintln(w)
	}
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
