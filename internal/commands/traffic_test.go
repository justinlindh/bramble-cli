package commands

import (
	"bytes"
	"strings"
	"testing"

	bramble "github.com/justinlindh/bramble-go"
)

func TestNewTrafficCmd_HasSubcommands(t *testing.T) {
	t.Parallel()

	cmd := newTrafficCmd()
	if cmd.Use != "traffic" {
		t.Fatalf("unexpected use: %q", cmd.Use)
	}
	if cmd.Commands() == nil || len(cmd.Commands()) != 2 {
		t.Fatalf("expected 2 subcommands, got %d", len(cmd.Commands()))
	}

	hasMonitor := false
	hasExport := false
	for _, sub := range cmd.Commands() {
		switch sub.Use {
		case "monitor":
			hasMonitor = true
		case "export":
			hasExport = true
		}
	}
	if !hasMonitor || !hasExport {
		t.Fatalf("expected monitor and export subcommands, got monitor=%v export=%v", hasMonitor, hasExport)
	}
}

func TestRunTrafficExport_RejectsUnsupportedFormat(t *testing.T) {
	t.Parallel()

	cmd := newTrafficExportCmd()
	if err := cmd.Flags().Set("format", "json"); err != nil {
		t.Fatalf("set format: %v", err)
	}

	err := runTrafficExport(cmd, nil)
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
	if !strings.Contains(err.Error(), "unsupported format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunTrafficExport_RejectsOutOfRangeLimit(t *testing.T) {
	t.Parallel()

	cmd := newTrafficExportCmd()
	if err := cmd.Flags().Set("limit", "0"); err != nil {
		t.Fatalf("set limit: %v", err)
	}

	err := runTrafficExport(cmd, nil)
	if err == nil {
		t.Fatal("expected error for out-of-range limit")
	}
	if !strings.Contains(err.Error(), "limit must be between 1 and 512") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFormatTrafficEventLine_RXWithRSSI(t *testing.T) {
	t.Parallel()

	line := formatTrafficEventLine(bramble.TrafficEvent{
		Seq:         42,
		PktType:     3,
		Category:    "chat",
		AirtimeTier: "normal",
		PacketLen:   64,
		RSSI:        -91,
	})

	for _, needle := range []string{"RX", "chat", "pkt=3", "64b", "RSSI=-91dBm", "tier=normal"} {
		if !strings.Contains(line, needle) {
			t.Fatalf("expected %q in %q", needle, line)
		}
	}
}

func TestWriteTrafficEventsJSONL_EmptyInput(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	if err := writeTrafficEventsJSONL(&buf, nil); err != nil {
		t.Fatalf("writeTrafficEventsJSONL: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no output for no events, got %q", buf.String())
	}
}
