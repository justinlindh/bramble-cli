package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/output"
)

func newBatteryCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "battery",
		Short: "Show battery status",
		Long:  "Display battery voltage (mV) and charge percentage.",
		RunE:  runBattery,
	}
}

func runBattery(cmd *cobra.Command, args []string) error {
	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	bat, err := client.Battery(ctx)
	if err != nil {
		return fmt.Errorf("bramble-cli: get battery: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, bat)
	}

	w := os.Stdout
	fmt.Fprintf(w, "Voltage:    %d mV\n", bat.VoltageMV)
	fmt.Fprintf(w, "Percentage: %d%%\n", bat.Percentage)
	return nil
}
