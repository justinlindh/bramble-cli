package commands

import (
	"encoding/json"
	"testing"
	"time"

	bramble "github.com/justinlindh/bramble-go"
)

func TestNewProbeCmd_Config(t *testing.T) {
	t.Parallel()

	cmd := newProbeCmd()
	if cmd.Use != "probe" {
		t.Fatalf("unexpected use: %q", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Fatal("expected RunE handler")
	}
}

func TestNewProbeCmd_HasTimeoutFlag(t *testing.T) {
	t.Parallel()

	cmd := newProbeCmd()
	f := cmd.Flags().Lookup("timeout")
	if f == nil {
		t.Fatal("--timeout flag not registered")
	}
	if f.DefValue != "0s" {
		t.Fatalf("expected default timeout 0s, got %q", f.DefValue)
	}
}

func TestProbeWaitDuration_UsesOverrideWhenSet(t *testing.T) {
	t.Parallel()

	got := probeWaitDuration(7*time.Second, 30)
	if got != 7*time.Second {
		t.Fatalf("expected 7s, got %s", got)
	}
}

func TestProbeWaitDuration_UsesAckWindowWhenNoOverride(t *testing.T) {
	t.Parallel()

	got := probeWaitDuration(0, 20)
	if got != 20*time.Second {
		t.Fatalf("expected 20s, got %s", got)
	}
}

func TestProbeWaitDuration_FallbackWhenBothZero(t *testing.T) {
	t.Parallel()

	got := probeWaitDuration(0, 0)
	if got != 10*time.Second {
		t.Fatalf("expected 10s fallback, got %s", got)
	}
}

func TestProbeWaitDuration_OverrideTakesPrecedenceOverAckWindow(t *testing.T) {
	t.Parallel()

	// Override should win even when ack_window is larger.
	got := probeWaitDuration(5*time.Second, 60)
	if got != 5*time.Second {
		t.Fatalf("expected 5s override, got %s", got)
	}
}

func TestProbeCommandResult_JSONIncludesResponses(t *testing.T) {
	t.Parallel()

	result := ProbeCommandResult{
		ProbeID:   42,
		AckWindow: 15,
		Responses: []bramble.ProbeResult{
			{
				Address:   "DEADBEEF",
				Hops:      2,
				RSSI:      -85,
				SNR:       7.5,
				LatencyMs: 320,
			},
		},
	}

	b, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if got["probe_id"] != float64(42) {
		t.Fatalf("probe_id mismatch: %v", got["probe_id"])
	}
	if got["ack_window"] != float64(15) {
		t.Fatalf("ack_window mismatch: %v", got["ack_window"])
	}

	responses, ok := got["responses"].([]any)
	if !ok {
		t.Fatalf("expected responses array, got: %T %v", got["responses"], got["responses"])
	}
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	resp := responses[0].(map[string]any)
	if resp["address"] != "DEADBEEF" {
		t.Fatalf("address mismatch: %v", resp["address"])
	}
	if resp["hops"] != float64(2) {
		t.Fatalf("hops mismatch: %v", resp["hops"])
	}
}

func TestProbeCommandResult_JSONOmitsResponsesWhenEmpty(t *testing.T) {
	t.Parallel()

	result := ProbeCommandResult{
		ProbeID:   1,
		AckWindow: 5,
	}

	b, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if _, ok := got["responses"]; ok {
		t.Fatal("expected responses to be omitted when empty")
	}
}
