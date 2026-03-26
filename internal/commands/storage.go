package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/output"
)

func newStorageCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "storage",
		Short: "Show storage info",
		Long:  "Display board storage status including SD card presence and mount point.",
		RunE:  runStorage,
	}
}

func runStorage(cmd *cobra.Command, args []string) error {
	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	info, err := client.GetStorageInfo(ctx)
	if err != nil {
		return fmt.Errorf("bramble-cli: get storage info: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, info)
	}

	w := os.Stdout
	fmt.Fprintf(w, "SD present:   %s\n", boolStr(info.SDPresent, "yes", "no"))
	if info.MountPoint != "" {
		fmt.Fprintf(w, "Mount point:  %s\n", info.MountPoint)
	}
	return nil
}
