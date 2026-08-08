package commands

import (
	"bytes"
	"encoding/json"
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

func TestFormatTrafficEventLine_RXWithSourceAndRSSI(t *testing.T) {
	t.Parallel()

	line := formatTrafficEventLine(bramble.TrafficEvent{
		Seq:         42,
		PktType:     3,
		Category:    "chat",
		AirtimeTier: "normal",
		PacketLen:   64,
		RSSI:        -91,
		SrcAddr:     "1A2B3C4D",
	})

	for _, needle := range []string{"RX", "chat", "pkt=3", "64b", "src=1A2B3C4D", "RSSI=-91dBm", "tier=normal"} {
		if !strings.Contains(line, needle) {
			t.Fatalf("expected %q in %q", needle, line)
		}
	}
	// Attribution only works if the address sits next to the sample.
	if strings.Index(line, "src=") > strings.Index(line, "RSSI=") {
		t.Fatalf("expected src before RSSI in %q", line)
	}
}

// The firmware records an unknown origin as zero and omits the key (see
// traffic_event_add_json in main/util.c), so absence is the only "no origin"
// signal and must never render as a placeholder address.
func TestFormatTrafficEventLine_OmitsAbsentSource(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		evt  bramble.TrafficEvent
	}{
		{
			name: "tx event carries no origin",
			evt:  bramble.TrafficEvent{Seq: 7, Category: "beacon", AirtimeTier: "broadcast", PacketLen: 21, IsTx: true},
		},
		{
			name: "rx packet type with no origin field",
			evt:  bramble.TrafficEvent{Seq: 8, Category: "ack", AirtimeTier: "critical", PacketLen: 12, RSSI: -70},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			line := formatTrafficEventLine(tc.evt)
			if strings.Contains(line, "src=") {
				t.Fatalf("expected no src field in %q", line)
			}
			if strings.Contains(line, "00000000") {
				t.Fatalf("absent source rendered as a placeholder address in %q", line)
			}
		})
	}
}

func TestWriteTrafficEventsJSONL_SourceAddressPresence(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	err := writeTrafficEventsJSONL(&buf, []bramble.TrafficEvent{
		{Seq: 1, Category: "chat", AirtimeTier: "normal", PacketLen: 64, RSSI: -91, SrcAddr: "1A2B3C4D"},
		{Seq: 2, Category: "beacon", AirtimeTier: "broadcast", PacketLen: 21, IsTx: true},
	})
	if err != nil {
		t.Fatalf("writeTrafficEventsJSONL: %v", err)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 JSONL lines, got %d: %q", len(lines), buf.String())
	}

	var withSrc map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &withSrc); err != nil {
		t.Fatalf("unmarshal line 0: %v", err)
	}
	if withSrc["src_addr"] != "1A2B3C4D" {
		t.Fatalf("expected src_addr on the RX event, got %v", withSrc["src_addr"])
	}

	var withoutSrc map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &withoutSrc); err != nil {
		t.Fatalf("unmarshal line 1: %v", err)
	}
	if _, ok := withoutSrc["src_addr"]; ok {
		t.Fatalf("expected src_addr to be absent on the TX event, got %q", lines[1])
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

// A reported all-zero address renders like any other. The firmware guards the
// key on src_addr != 0 so it should never send one, but the schema's pattern
// admits it, and the renderer's job is to show what arrived rather than to
// invent a sentinel the wire does not define.
func TestFormatTrafficEventLine_RendersWhateverAddressArrives(t *testing.T) {
	t.Parallel()

	line := formatTrafficEventLine(bramble.TrafficEvent{
		Seq: 9, Category: "routing", AirtimeTier: "normal", PacketLen: 30, RSSI: -80, SrcAddr: "00000000",
	})
	if !strings.Contains(line, "src=00000000") {
		t.Fatalf("expected a reported all-zero address to render, got %q", line)
	}
}

// Decodes a whole getTrafficEvents response in the shape the firmware actually
// sends, then renders it. The other tests here build TrafficEvent values by
// hand, which proves the renderer works but cannot catch a field that never
// arrives: that gap is exactly how a src= rendering for a key no firmware sent
// survived review once already.
func TestTrafficEventDecodesWireShape(t *testing.T) {
	t.Parallel()

	const payload = `{
	  "events": [
	    {"seq":10241,"timestamp_ms":86400123,"pkt_type":3,"category":"chat",
	     "airtime_tier":"normal","packet_len":64,"rssi":-91,"is_tx":false,
	     "src_addr":"1A2B3C4D"},
	    {"seq":10242,"timestamp_ms":86400456,"pkt_type":3,"category":"chat",
	     "airtime_tier":"normal","packet_len":64,"rssi":0,"is_tx":true}
	  ],
	  "returned": 2,
	  "total_available": 2
	}`

	var resp bramble.TrafficEventsResponse
	if err := json.Unmarshal([]byte(payload), &resp); err != nil {
		t.Fatalf("unmarshal wire payload: %v", err)
	}
	if len(resp.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(resp.Events))
	}

	// The RX frame's origin has to survive decoding, or the src= rendering is
	// unreachable no matter how well it is unit-tested.
	if resp.Events[0].SrcAddr != "1A2B3C4D" {
		t.Fatalf("src_addr did not decode onto SrcAddr, got %q", resp.Events[0].SrcAddr)
	}
	if resp.Events[1].SrcAddr != "" {
		t.Fatalf("expected an omitted src_addr to decode as empty, got %q", resp.Events[1].SrcAddr)
	}

	if line := formatTrafficEventLine(resp.Events[0]); !strings.Contains(line, "src=1A2B3C4D") {
		t.Fatalf("expected the decoded origin to render, got %q", line)
	}
	if line := formatTrafficEventLine(resp.Events[1]); strings.Contains(line, "src=") {
		t.Fatalf("expected no src field for the TX event, got %q", line)
	}
}
