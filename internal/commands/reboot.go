package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/output"
)

func newRebootCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reboot",
		Short: "Reboot the node",
		Long:  "Trigger a software reboot of the connected Bramble node.",
		RunE:  runReboot,
	}
}

func runReboot(cmd *cobra.Command, args []string) error {
	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.Reboot(ctx); err != nil {
		return fmt.Errorf("bramble-cli: reboot: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, StatusResult{Status: "rebooting"})
	}
	fmt.Fprintln(os.Stdout, "Node rebooting...")
	return nil
}
