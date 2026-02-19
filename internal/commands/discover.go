package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/justinlindh/bramble-cli/internal/discovery"
	"github.com/spf13/cobra"
)

func newDiscoverCmd() *cobra.Command {
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Scan for Bramble nodes on the local network via mDNS",
		Long: `Scans for Bramble mesh nodes advertising _bramble._tcp via mDNS.
Returns discovered nodes with their hostname, IP address, and WebSocket URL.

Examples:
  bramble discover
  bramble discover --timeout 5s
  bramble discover --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()

			if !flagJSON {
				fmt.Fprintf(cmd.ErrOrStderr(), "Scanning for Bramble nodes (%s)...\n", timeout)
			}

			nodes, err := discovery.DiscoverMDNS(ctx, timeout)
			if err != nil {
				return fmt.Errorf("discovery failed: %w", err)
			}

			if flagJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(nodes)
			}

			if len(nodes) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No Bramble nodes found.")
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Found %d Bramble node(s):\n", len(nodes))
			for _, n := range nodes {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s at %s\n", n.Hostname, n.Address)
				fmt.Fprintf(cmd.OutOrStdout(), "    WebSocket: %s\n", n.WSURL)
			}

			return nil
		},
	}

	cmd.Flags().DurationVar(&timeout, "timeout", 3*time.Second, "how long to scan for nodes")

	return cmd
}
