package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/justinlindh/bramble-cli/internal/output"
	"github.com/spf13/cobra"
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
	ctx := context.Background()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.Reboot(ctx); err != nil {
		return fmt.Errorf("reboot: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, map[string]string{"status": "rebooting"})
	}
	fmt.Fprintln(os.Stdout, "Node rebooting...")
	return nil
}
