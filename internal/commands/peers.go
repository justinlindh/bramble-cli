package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/justinlindh/bramble-cli/internal/output"
	"github.com/spf13/cobra"
)

func newPeersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "peers",
		Short: "List direct radio neighbors",
		Long:  "Show all nodes heard directly over the radio, with RSSI, SNR, and last-heard time.",
		RunE:  runPeers,
	}
}

func runPeers(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	neighbors, err := client.Neighbors(ctx)
	if err != nil {
		return fmt.Errorf("get neighbors: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, neighbors)
	}

	if len(neighbors) == 0 {
		fmt.Fprintln(os.Stdout, "No neighbors heard yet.")
		return nil
	}

	headers := []string{"ADDRESS", "RSSI", "SNR", "LAST SEEN"}
	rows := make([][]string, len(neighbors))
	for i, n := range neighbors {
		rows[i] = []string{
			n.Address,
			fmt.Sprintf("%d dBm", n.RSSI),
			fmt.Sprintf("%.1f dB", n.SNR),
			output.FormatMs(n.LastHeardMs),
		}
	}
	output.Table(os.Stdout, headers, rows)
	return nil
}
