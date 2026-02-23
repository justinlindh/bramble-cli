package commands

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/justinlindh/bramble-cli/internal/output"
	bramble "github.com/justinlindh/bramble-go"
)

func TestBroadcastSendStatus_IncludesBroadcastID(t *testing.T) {
	t.Parallel()

	line := output.FormatBroadcastSendStatus(-1, "queued", "BCAST-123")
	if !strings.Contains(line, "broadcast_id=BCAST-123") {
		t.Fatalf("expected broadcast_id in output, got: %q", line)
	}
}

func TestMonitorBroadcastDeliveryLine_ShowsRecipientStatus(t *testing.T) {
	t.Parallel()

	evt := bramble.BroadcastDelivery{
		BroadcastID: "BCAST-123",
		Recipient:   "A1B2C3D4",
		Status:      "delivered",
	}
	line := monitorBroadcastDeliveryLine(time.Unix(1730000000, 0), evt)

	if !strings.Contains(line, "recipient=A1B2C3D4") || !strings.Contains(line, "status=delivered") {
		t.Fatalf("expected recipient+status in line, got: %q", line)
	}
}

func TestMonitorBroadcastDeliveryJSON_IncludesPayload(t *testing.T) {
	t.Parallel()

	evt := bramble.BroadcastDelivery{
		BroadcastID: "BCAST-123",
		Recipient:   "A1B2C3D4",
		Status:      "delivered",
		TimestampMs: 1730000000123,
	}
	b, err := monitorBroadcastDeliveryJSON(evt)
	if err != nil {
		t.Fatalf("monitorBroadcastDeliveryJSON returned error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}

	payload, ok := got["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected payload object, got: %#v", got["payload"])
	}
	if payload["broadcast_id"] != "BCAST-123" {
		t.Fatalf("broadcast_id mismatch: %#v", payload["broadcast_id"])
	}
	if payload["recipient"] != "A1B2C3D4" {
		t.Fatalf("recipient mismatch: %#v", payload["recipient"])
	}
	if payload["status"] != "delivered" {
		t.Fatalf("status mismatch: %#v", payload["status"])
	}
}
