package tui

import (
	"strings"
	"testing"
)

func TestAddDeliveryGrouped_SameGroupStaysOnOneLine(t *testing.T) {
	sb := NewScrollback()

	sb.AddDeliveryGrouped("bcast-1", "alice ✓")
	sb.AddDeliveryGrouped("bcast-1", "bob ✓")

	if got := sb.LineCount(); got != 1 {
		t.Fatalf("expected 1 line for grouped deliveries, got %d", got)
	}

	line := sb.lines[0].Text
	if !strings.Contains(line, "alice ✓") || !strings.Contains(line, "bob ✓") {
		t.Fatalf("expected grouped line to contain both recipients, got %q", line)
	}
}

func TestAddDeliveryGrouped_DifferentGroupsCreateSeparateLines(t *testing.T) {
	sb := NewScrollback()

	sb.AddDeliveryGrouped("bcast-1", "alice ✓")
	sb.AddDeliveryGrouped("bcast-2", "carol ✓")

	if got := sb.LineCount(); got != 2 {
		t.Fatalf("expected 2 lines for different delivery groups, got %d", got)
	}
}
