package commands

import (
	"strings"
	"testing"
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
