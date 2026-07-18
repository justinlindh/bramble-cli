package tui

import (
	"strings"
	"testing"
)

func TestAddDelivery_RendersFaintSummaryLine(t *testing.T) {
	sb := NewScrollback()

	sb.AddDelivery("alice ✓  bob ✓")

	if got := sb.LineCount(); got != 1 {
		t.Fatalf("expected 1 delivery line, got %d", got)
	}
	if sb.lines[0].Kind != LineDelivery {
		t.Fatalf("expected LineDelivery kind, got %v", sb.lines[0].Kind)
	}
	line := sb.lines[0].Text
	if !strings.Contains(line, "alice ✓") || !strings.Contains(line, "bob ✓") {
		t.Fatalf("expected delivery line to contain both recipients, got %q", line)
	}
	if !strings.Contains(line, "--") {
		t.Fatalf("expected delivery line to be wrapped in dashes, got %q", line)
	}
}
