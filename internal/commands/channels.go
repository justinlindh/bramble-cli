package commands

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/justinlindh/bramble-cli/internal/output"
	"github.com/spf13/cobra"
)

func newChannelsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "channels",
		Short: "Manage mesh channels",
		Long:  "Subcommands: list, add, remove, set-default",
	}
	cmd.AddCommand(
		newChannelsListCmd(),
		newChannelsAddCmd(),
		newChannelsRemoveCmd(),
		newChannelsSetDefaultCmd(),
	)
	return cmd
}

func newChannelsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured channels",
		RunE:  runChannelsList,
	}
}

func runChannelsList(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	cfg, err := client.Config(ctx)
	if err != nil {
		return fmt.Errorf("get config: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, cfg.Channels)
	}

	if len(cfg.Channels) == 0 {
		fmt.Fprintln(os.Stdout, "No channels configured.")
		return nil
	}

	headers := []string{"ID", "NAME", "DEFAULT"}
	rows := make([][]string, len(cfg.Channels))
	for i, ch := range cfg.Channels {
		def := ""
		if ch.IsDefault {
			def = "✓"
		}
		rows[i] = []string{
			strconv.Itoa(ch.ID),
			ch.Name,
			def,
		}
	}
	output.Table(os.Stdout, headers, rows)
	return nil
}

func newChannelsAddCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add <name> [psk]",
		Short: "Add a new channel",
		Long: `Add a new channel with the given name and optional pre-shared key.

Example:
  bramble channels add "team" "s3cr3t"
  bramble channels add "public"`,
		Args: cobra.RangeArgs(1, 2),
		RunE: runChannelsAdd,
	}
	return cmd
}

func runChannelsAdd(cmd *cobra.Command, args []string) error {
	name := args[0]
	psk := ""
	if len(args) > 1 {
		psk = args[1]
	}

	ctx := context.Background()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	result, err := client.AddChannel(ctx, name, psk)
	if err != nil {
		return fmt.Errorf("add channel: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, map[string]any{
			"index": result.Index,
			"name":  name,
		})
	}
	fmt.Fprintf(os.Stdout, "Channel %q added at index %d\n", name, result.Index)
	return nil
}

func newChannelsRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <index>",
		Short: "Remove a channel by index",
		Args:  cobra.ExactArgs(1),
		RunE:  runChannelsRemove,
	}
}

func runChannelsRemove(cmd *cobra.Command, args []string) error {
	idx, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid channel index %q: must be an integer", args[0])
	}

	ctx := context.Background()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.RemoveChannel(ctx, idx); err != nil {
		return fmt.Errorf("remove channel: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, map[string]any{"index": idx, "status": "removed"})
	}
	fmt.Fprintf(os.Stdout, "Channel %d removed.\n", idx)
	return nil
}

func newChannelsSetDefaultCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-default <index>",
		Short: "Set the default channel for outgoing messages",
		Args:  cobra.ExactArgs(1),
		RunE:  runChannelsSetDefault,
	}
}

func runChannelsSetDefault(cmd *cobra.Command, args []string) error {
	idx, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid channel index %q: must be an integer", args[0])
	}

	ctx := context.Background()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.SetDefaultChannel(ctx, idx); err != nil {
		return fmt.Errorf("set-default: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, map[string]any{"index": idx, "status": "ok"})
	}
	fmt.Fprintf(os.Stdout, "Default channel set to %d.\n", idx)
	return nil
}
