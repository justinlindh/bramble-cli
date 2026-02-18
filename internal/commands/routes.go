package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/justinlindh/bramble-cli/internal/output"
	"github.com/spf13/cobra"
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
	ctx := context.Background()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	routes, err := client.Routes(ctx)
	if err != nil {
		return fmt.Errorf("get routes: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, routes)
	}

	if len(routes) == 0 {
		fmt.Fprintln(os.Stdout, "Routing table is empty.")
		return nil
	}

	headers := []string{"DEST", "NEXT HOP", "HOPS", "METRIC", "STATE", "LAST USED"}
	rows := make([][]string, len(routes))
	for i, r := range routes {
		rows[i] = []string{
			r.Dest,
			r.NextHop,
			fmt.Sprintf("%d", r.HopCount),
			fmt.Sprintf("%d", r.Metric),
			r.State,
			output.FormatMs(r.LastUsedMs),
		}
	}
	output.Table(os.Stdout, headers, rows)
	return nil
}
