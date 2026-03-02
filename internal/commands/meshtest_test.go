package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewMeshTestCmd_HasFlagsAndRunE(t *testing.T) {
	cmd := newMeshTestCmd()
	if cmd.Use != "mesh-test" {
		t.Fatalf("unexpected use: %s", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Fatal("expected RunE")
	}
	for _, name := range []string{"config", "sender", "count", "spacing", "wait", "json", "verbose"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("missing flag --%s", name)
		}
	}
}

func TestLoadMeshTestConfig_ParsesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mesh-test.json")
	content := `{
  "sender":"ws://192.0.2.0/ws",
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
	if cfg.Sender != "ws://192.0.2.0/ws" || cfg.Broadcasts != 7 || len(cfg.Nodes) != 1 {
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
