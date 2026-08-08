package commands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bramble "github.com/justinlindh/bramble-go"
	"github.com/spf13/cobra"
)

func sampleTopologyExport() *bramble.TopologyExport {
	return &bramble.TopologyExport{
		TwinSchema: 1,
		Node: bramble.TopologyNode{
			Address:         "3D4E5F60",
			Name:            "tower",
			FirmwareVersion: "0.9.3",
			ProtocolVersion: "0.5.0",
			Hardware:        "heltec_v3",
			UptimeS:         7200,
		},
		Radio: bramble.TopologyRadio{
			FrequencyMhz:    915.0,
			SF:              9,
			BwHz:            125000,
			CodingRate:      1,
			TxPowerDbm:      17,
			Region:          "US915",
			Regulatory:      "FCC Part 15.247",
			MaxDutyCyclePct: 100,
		},
		Neighbors: []bramble.Neighbor{
			{Address: "0A1B2C3D", Name: "basecamp", RSSI: -95, SNR: 8,
				LastSeenAgoMs: 4200, DeliveryRate: 250, AirtimeRemaining: 88},
		},
		Routes: []bramble.Route{
			{Dest: "0A1B2C3D", NextHop: "0A1B2C3D", HopCount: 1, Metric: 140, State: "active", UseCount: 9},
		},
	}
}

func TestNewTopologyCmd_HasExportSubcommand(t *testing.T) {
	t.Parallel()

	cmd := newTopologyCmd()
	got, _, err := cmd.Find([]string{"export"})
	if err != nil || got == nil || got.Name() != "export" {
		t.Fatalf("expected an export subcommand (got=%v, err=%v)", got, err)
	}
	if got.Flags().Lookup("out") == nil {
		t.Fatal("expected an --out flag")
	}
	if def := got.Flags().Lookup("out").DefValue; def != "-" {
		t.Errorf("--out defaults to %q, want stdout so the document pipes without a temp file", def)
	}
	if err := got.Args(got, []string{"tower.json"}); err == nil {
		t.Error("a positional path was accepted; the output path is --out")
	}
}

// The importer is deliberately strict, so the file this command writes has to
// carry the document's own top-level keys. A document re-encoded under Go field
// names would look fine here and be refused by the twin.
func TestWriteTopologyExport_FileCarriesTheDocumentKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exports", "tower.json")
	cmd := &cobra.Command{}

	if err := writeTopologyExport(cmd, path, sampleTopologyExport()); err != nil {
		t.Fatalf("writeTopologyExport: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("the file is not JSON: %v", err)
	}
	for _, key := range []string{"twin_schema", "node", "radio", "neighbors", "routes"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("the document has no %q key:\n%s", key, raw)
		}
	}
	if doc["twin_schema"] != float64(1) {
		t.Errorf("twin_schema written as %v", doc["twin_schema"])
	}

	neighbors, ok := doc["neighbors"].([]any)
	if !ok || len(neighbors) != 1 {
		t.Fatalf("neighbors written as %v", doc["neighbors"])
	}
	n, ok := neighbors[0].(map[string]any)
	if !ok {
		t.Fatalf("neighbor written as %v", neighbors[0])
	}
	// The link-quality keys are the ones the export contract spells in
	// camelCase; the twin reads a link's quality out of exactly these.
	for _, key := range []string{"address", "rssi", "snr", "deliveryRate", "airtimeRemaining"} {
		if _, ok := n[key]; !ok {
			t.Errorf("the neighbor has no %q key: %v", key, n)
		}
	}
}

func TestWriteTopologyExport_StdoutGetsTheDocumentAndNothingElse(t *testing.T) {
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)

	if err := writeTopologyExport(cmd, "-", sampleTopologyExport()); err != nil {
		t.Fatalf("writeTopologyExport: %v", err)
	}

	// One document, parseable on its own: this is what pipes into the twin.
	var doc map[string]any
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("stdout is not a single JSON document: %v\n%s", err, out.String())
	}
	node, ok := doc["node"].(map[string]any)
	if !ok || node["address"] != "3D4E5F60" {
		t.Errorf("the exporting node is missing from the document: %v", doc["node"])
	}
}

func TestWriteTopologyExport_UnwritablePathNamesThePath(t *testing.T) {
	// A directory where the file should be: the failure has to say which path
	// it was, since a fleet export runs one command per node.
	dir := t.TempDir()
	blocked := filepath.Join(dir, "tower.json")
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	err := writeTopologyExport(&cobra.Command{}, blocked, sampleTopologyExport())
	if err == nil {
		t.Fatal("writing over a directory succeeded")
	}
	if !strings.Contains(err.Error(), blocked) {
		t.Errorf("error %q does not name the path", err)
	}
}

func TestNewTopologyExportSummary(t *testing.T) {
	t.Parallel()

	got := newTopologyExportSummary("tower.json", sampleTopologyExport())
	want := topologyExportSummary{
		Path:       "tower.json",
		TwinSchema: 1,
		Address:    "3D4E5F60",
		Name:       "tower",
		Neighbors:  1,
		Routes:     1,
	}
	if got != want {
		t.Errorf("summary is %+v, want %+v", got, want)
	}
}

// A node that answered with an empty mesh view is the interesting case when
// collecting a fleet, so the counts have to survive an export with no links.
func TestNewTopologyExportSummary_EmptyMeshView(t *testing.T) {
	t.Parallel()

	doc := sampleTopologyExport()
	doc.Neighbors = nil
	doc.Routes = nil
	got := newTopologyExportSummary("tower.json", doc)
	if got.Neighbors != 0 || got.Routes != 0 {
		t.Errorf("summary is %+v, want zero neighbors and routes", got)
	}
	if got.Address != "3D4E5F60" {
		t.Errorf("summary lost the node address: %+v", got)
	}
}
