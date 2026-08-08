package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/output"
)

func newRoutesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "routes",
		Short: "Show the routing table",
		Long:  "Display the current mesh routing table with destinations, next hops, and hop counts.",
		RunE:  runRoutes,
	}
}

func runRoutes(cmd *cobra.Command, args []string) error {
	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	routes, err := client.Routes(ctx)
	if err != nil {
		return fmt.Errorf("bramble-cli: get routes: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, routes)
	}

	if len(routes) == 0 {
		fmt.Fprintln(os.Stdout, "Routing table is empty.")
		return nil
	}

	// The columns are exactly the fields a route carries on the wire. There is
	// no last-used time among them: the node reports how often a route was
	// used, not when, and a column filled from a field the node never sends
	// would read as an observation rather than as a zero.
	headers := []string{"DEST", "NEXT HOP", "HOPS", "METRIC", "STATE", "USES"}
	rows := make([][]string, len(routes))
	for i, r := range routes {
		rows[i] = []string{
			r.Dest,
			r.NextHop,
			fmt.Sprintf("%d", r.HopCount),
			fmt.Sprintf("%d", r.Metric),
			r.State,
			fmt.Sprintf("%d", r.UseCount),
		}
	}
	output.Table(os.Stdout, headers, rows)
	return nil
}
