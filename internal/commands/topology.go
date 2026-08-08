package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	bramble "github.com/justinlindh/bramble-go"

	"github.com/justinlindh/bramble-cli/internal/output"
)

// topologyExportSummary is the --json shape of a successful export to a file.
// The exported document itself is not in it: the document is the deliverable
// and it went to the file, so repeating it here would give a caller two copies
// that can disagree about which one the simulator reads.
type topologyExportSummary struct {
	Path       string `json:"path"`
	TwinSchema int    `json:"twin_schema"`
	Address    string `json:"address"`
	Name       string `json:"name,omitempty"`
	Neighbors  int    `json:"neighbors"`
	Routes     int    `json:"routes"`
}

func newTopologyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "topology",
		Short: "Export this node's view of the mesh for the simulator's digital twin",
		Long: "Read a node's own view of the mesh as one document: who it is, the\n" +
			"neighbors it hears and at what link quality, its routing table, and the\n" +
			"PHY and frequency plan that price its time-on-air.\n\n" +
			"Collect one document per node and the simulator can rebuild the deployment\n" +
			"as a runnable scenario.",
	}
	cmd.AddCommand(newTopologyExportCmd())
	return cmd
}

func newTopologyExportCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "export",
		Short: "Write the connected node's topology export document",
		Long: `Write one node's topology export as JSON, to a file or to stdout.

The document is the input to the digital-twin importer in the bramble repo:
export from every node you can reach, then hand the files to the simulator's
twin subcommand, which merges them into a link graph and runs the firmware's
own protocol code over it to report capacity and single points of failure.

  bramble --port /dev/ttyUSB0 topology export --out tower.json
  bramble --port /dev/ttyUSB1 topology export --out ridge.json
  bramble-gosim twin tower.json ridge.json

Export from as many nodes as you can: for a node that never exports, only the
direction its neighbors heard is known, and the twin's report names every
direction it had to assume.

This is observation, not prediction. Every link is a snapshot taken when the
call was made, and the document carries the fields the export contract defines,
re-encoded: a field a newer firmware adds outside that contract is not carried
through. The importer refuses a schema version it does not know rather than
guessing at fields, so a document it accepts is one it fully understands.`,
		Example: "  bramble topology export --out tower.json\n" +
			"  bramble topology export | jq .neighbors\n" +
			"  bramble --json topology export --out tower.json",
		Args: cobra.NoArgs,
		RunE: runTopologyExport,
	}
	c.Flags().StringP("out", "o", "-", `output path for the export document, or "-" for stdout`)
	return c
}

func runTopologyExport(cmd *cobra.Command, args []string) error {
	out, _ := cmd.Flags().GetString("out")

	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	doc, err := client.ExportTopology(ctx)
	if err != nil {
		return fmt.Errorf("bramble-cli: export topology: %w", err)
	}

	if err := writeTopologyExport(cmd, out, doc); err != nil {
		return err
	}

	// Writing to stdout, the document is the output: a summary printed after it
	// would land in the same stream and break the pipe into the twin importer,
	// and the document already carries everything the summary would say.
	if out == "-" {
		return nil
	}

	summary := newTopologyExportSummary(out, doc)
	if flagJSON {
		return output.PrintJSON(cmd.OutOrStdout(), summary)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Wrote %s: node %s, %d neighbor(s), %d route(s), twin schema %d\n",
		summary.Path, summary.Address, summary.Neighbors, summary.Routes, summary.TwinSchema)
	fmt.Fprintf(cmd.ErrOrStderr(),
		"Export the rest of the fleet, then run: bramble-gosim twin %s ...\n", summary.Path)
	return nil
}

// newTopologyExportSummary describes what was written. Counts rather than
// contents: an operator collecting a fleet's exports is checking that each node
// answered with a mesh view that is not empty, and the file holds the rest.
func newTopologyExportSummary(path string, doc *bramble.TopologyExport) topologyExportSummary {
	return topologyExportSummary{
		Path:       path,
		TwinSchema: doc.TwinSchema,
		Address:    doc.Node.Address,
		Name:       doc.Node.Name,
		Neighbors:  len(doc.Neighbors),
		Routes:     len(doc.Routes),
	}
}

// writeTopologyExport writes the document to a path, or to stdout when the path
// is "-". The file is created before the document is encoded so a bad path
// fails immediately, and it is closed explicitly so a write that failed on
// flush is reported rather than discarded by a deferred close.
func writeTopologyExport(cmd *cobra.Command, path string, doc *bramble.TopologyExport) error {
	if path == "-" {
		return output.PrintJSON(cmd.OutOrStdout(), doc)
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("bramble-cli: create %s: %w", dir, err)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("bramble-cli: create %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	if err := output.PrintJSON(f, doc); err != nil {
		return fmt.Errorf("bramble-cli: write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("bramble-cli: write %s: %w", path, err)
	}
	return nil
}
