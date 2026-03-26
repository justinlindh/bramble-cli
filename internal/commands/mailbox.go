package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/output"
)

func newMailboxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mailbox",
		Short: "Mailbox controls",
		Long:  "Subcommands: set",
	}
	cmd.AddCommand(
		newMailboxSetCmd(),
	)
	return cmd
}

func newMailboxSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <true|false>",
		Short: "Enable or disable mailbox",
		Long:  "Enable or disable the node's mailbox for store-and-forward messaging.",
		Args:  cobra.ExactArgs(1),
		RunE:  runMailboxSet,
	}
}

func runMailboxSet(cmd *cobra.Command, args []string) error {
	enabled, err := parseBoolArg(args[0], "enabled")
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

	if err := client.SetMailbox(ctx, enabled); err != nil {
		return fmt.Errorf("bramble-cli: set mailbox: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, StatusResult{Status: "ok"})
	}
	fmt.Fprintf(os.Stdout, "Mailbox %s\n", boolStr(enabled, "enabled", "disabled"))
	return nil
}
