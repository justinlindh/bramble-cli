package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	bramble "github.com/justinlindh/bramble-go"
)

func TestNewMeshTestCmd_HasFlagsAndRunE(t *testing.T) {
	cmd := newMeshTestCmd()
	if cmd.Use != "mesh-test" {
		t.Fatalf("unexpected use: %s", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Fatal("expected RunE")
	}
	for _, name := range []string{"config", "sender", "count", "spacing", "wait", "json", "verbose", "validate-primitives"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("missing flag --%s", name)
		}
	}
}

func TestLoadMeshTestConfig_ParsesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mesh-test.json")
	content := `{
  "sender":"ws://192.0.2.21/ws",
  "broadcasts":7,
  "spacing_seconds":3,
  "wait_seconds":9,
  "nodes":[
    {"name":"N1","transport":"/dev/ttyACM0"}
  ]
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadMeshTestConfig(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Sender != "ws://192.0.2.21/ws" || cfg.Broadcasts != 7 || len(cfg.Nodes) != 1 {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadMeshTestConfig_MissingIsAllowed(t *testing.T) {
	cfg, err := loadMeshTestConfig(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if cfg.Broadcasts != 0 || len(cfg.Nodes) != 0 {
		t.Fatalf("expected empty config, got %+v", cfg)
	}
}

func TestBuildMeshNodeList_IncludesSenderTransport(t *testing.T) {
	sender := "ws://192.0.2.21/ws"
	nodes := buildMeshNodeList(meshTestConfig{Sender: sender})
	found := false
	for _, n := range nodes {
		if n.Transport == sender {
			found = true
			if !n.FromConfig {
				t.Fatalf("sender node should be marked FromConfig")
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected sender transport %q in node list", sender)
	}
}

func TestHasTrafficEvent_MatchesTypeAndDirection(t *testing.T) {
	events := []bramble.TrafficEvent{
		{PktType: 0x0A, IsTx: true},
		{PktType: 0x07, IsTx: false},
	}
	if !hasTrafficEvent(events, 0x0A, true) {
		t.Fatalf("expected tx data event match")
	}
	if !hasTrafficEvent(events, 0x07, false) {
		t.Fatalf("expected rx receipt event match")
	}
	if hasTrafficEvent(events, 0x07, true) {
		t.Fatalf("did not expect tx receipt match")
	}
	if !hasTrafficEventAny(events, []int{0x11, 0x0A}, true) {
		t.Fatalf("expected any-match helper to find pkt type")
	}
}

func TestFormatMeshTestReport_IncludesPrimitiveLifecycleSection(t *testing.T) {
	report := meshTestReport{
		PrimitiveValidationEnabled: true,
		PrimitiveBroadcasts: []primitiveBroadcastResult{
			{
				Index:        1,
				SenderDataTx: true,
				Recipients: []primitiveRecipientResult{
					{Recipient: "AAAA0001", BroadcastRx: true, ReceiptTx: true, SenderReceiptRx: true},
					{Recipient: "BBBB0002", BroadcastRx: false, ReceiptTx: false, SenderReceiptRx: false},
				},
			},
		},
	}
	out := formatMeshTestReport(report)
	for _, want := range []string{
		"Primitive lifecycle validation:",
		"#1 sender_data_tx=ok",
		"AAAA0001 rx=ok receipt_tx=ok sender_rx_receipt=ok",
		"BBBB0002 rx=miss receipt_tx=miss sender_rx_receipt=miss",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q\nOutput:\n%s", want, out)
		}
	}
}

func TestFormatMeshTestReport(t *testing.T) {
	report := meshTestReport{
		Nodes: []meshNode{
			{Transport: "ws://a/ws", Reachable: true, Address: "AAAA0001", Hardware: "heltec_v3", Sender: true},
			{Transport: "/dev/ttyACM0", Reachable: false, Error: "timeout"},
		},
		BroadcastsSent: 2,
		Broadcasts: []broadcastResult{
			{Index: 1, Expected: 1, Received: 1, Recipients: []receiptResult{{Recipient: "BBBB0002", LatencyMs: 120}}},
			{Index: 2, Expected: 1, Received: 0, MissingRecipients: []string{"BBBB0002"}},
		},
		TotalExpected: 2,
		TotalReceived: 1,
		DeliveryRate:  50,
		NodeReliability: []nodeReliability{
			{Address: "BBBB0002", Received: 1, Expected: 2},
		},
	}
	out := formatMeshTestReport(report)
	for _, want := range []string{
		"=== Bramble Mesh Delivery Test Report ===",
		"Nodes: 2 (1 reachable, 1 unreachable)",
		"Delivery rate: 50% (1/2 receipts)",
		"#1 1/1 ✅",
		"#2 0/1 ⚠️",
		"missing: BBBB0002",
		"Per-node reliability:",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected output to contain %q\nOutput:\n%s", want, out)
		}
	}
}
