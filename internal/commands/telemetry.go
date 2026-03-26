package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/output"
)

func newTelemetryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "telemetry",
		Short: "Telemetry controls",
		Long:  "Subcommands: set-mode",
	}
	cmd.AddCommand(
		newTelemetrySetModeCmd(),
	)
	return cmd
}

func newTelemetrySetModeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-mode <mode>",
		Short: "Set broadcast telemetry mode",
		Long:  "Set the broadcast telemetry mode (e.g. off, basic, full).",
		Args:  cobra.ExactArgs(1),
		RunE:  runTelemetrySetMode,
	}
}

func runTelemetrySetMode(cmd *cobra.Command, args []string) error {
	mode := args[0]

	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	resp, err := client.SetBroadcastTelemetryMode(ctx, mode)
	if err != nil {
		return fmt.Errorf("bramble-cli: set broadcast telemetry mode: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, resp)
	}
	fmt.Fprintf(os.Stdout, "Broadcast telemetry mode set to: %s\n", resp.BroadcastTelemetryMode)
	return nil
}
