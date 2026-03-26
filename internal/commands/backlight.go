package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/output"
)

func newBacklightCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "backlight <level>",
		Short: "Set display backlight level",
		Long:  "Set the display backlight brightness level (0-100).",
		Args:  cobra.ExactArgs(1),
		RunE:  runBacklight,
	}
}

func runBacklight(cmd *cobra.Command, args []string) error {
	level, err := parseIntArg(args[0], "level")
	if err != nil {
		return err
	}

	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	resp, err := client.SetBacklight(ctx, level)
	if err != nil {
		return fmt.Errorf("bramble-cli: set backlight: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, resp)
	}
	fmt.Fprintf(os.Stdout, "Backlight set to %d\n", resp.Level)
	return nil
}
