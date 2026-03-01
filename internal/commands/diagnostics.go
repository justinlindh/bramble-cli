package commands

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/justinlindh/bramble-cli/internal/output"
	bramble "github.com/justinlindh/bramble-go"
	"github.com/spf13/cobra"
)

var diagnosticsHeapDump bool

func newDiagnosticsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "diagnostics",
		Aliases: []string{"diag"},
		Short:   "Show runtime diagnostics (heap and task stack HWM)",
		Long:    "Display runtime diagnostics including heap region stats and per-task stack high-water marks.",
		RunE:    runDiagnostics,
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

	d, err := client.GetDiagnostics(ctx, diagnosticsHeapDump)
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
}

func trimFloat(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}
