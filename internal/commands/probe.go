package commands

import (
	"fmt"
	"os"

	"github.com/justinlindh/bramble-cli/internal/output"
	"github.com/spf13/cobra"
)

func newProbeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "probe",
		Short: "Send a network probe",
		Long: `Broadcast a probe packet and print the probe ID.
Probe responses arrive asynchronously as notifications.
Use 'bramble monitor' to see responses as they arrive.`,
		RunE: runProbe,
	}
}

func runProbe(cmd *cobra.Command, args []string) error {
	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.SendProbe(ctx)
	if err != nil {
		return fmt.Errorf("bramble-cli: probe: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, map[string]any{
			"probe_id":   result.ProbeID,
			"ack_window": result.AckWindow,
		})
	}

	fmt.Fprintf(os.Stdout, "Probe sent: ID=%d  ack_window=%dms\n",
		result.ProbeID, result.AckWindow)
	fmt.Fprintln(os.Stdout, "Use 'bramble monitor' to see responses as they arrive.")
	return nil
}
