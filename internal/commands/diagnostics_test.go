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

func TestTrimFloat(t *testing.T) {
	t.Parallel()
	if got := trimFloat(42.0); got != "42" {
		t.Fatalf("trimFloat(42.0)=%q want 42", got)
	}
	if got := trimFloat(42.5); got != "42.5" {
		t.Fatalf("trimFloat(42.5)=%q want 42.5", got)
	}
}
