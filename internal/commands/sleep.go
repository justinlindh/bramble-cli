package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/output"
)

func newSleepCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sleep <seconds>",
		Short: "Put the node to sleep",
		Long:  "Put the node into deep sleep with an optional wake-after duration in seconds. Use 0 for indefinite sleep.",
		Args:  cobra.ExactArgs(1),
		RunE:  runSleep,
	}
}

func runSleep(cmd *cobra.Command, args []string) error {
	secs, err := parseIntArg(args[0], "seconds")
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

	resp, err := client.Sleep(ctx, secs)
	if err != nil {
		return fmt.Errorf("bramble-cli: sleep: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, resp)
	}

	w := os.Stdout
	if resp.OK {
		fmt.Fprintln(w, "Node entering sleep.")
		if resp.WakeAfterS > 0 {
			fmt.Fprintf(w, "Wake after: %ds\n", resp.WakeAfterS)
		}
	} else {
		fmt.Fprintln(w, "Sleep request failed.")
	}
	if resp.Note != "" {
		fmt.Fprintf(w, "Note: %s\n", resp.Note)
	}
	return nil
}
