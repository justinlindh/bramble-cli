package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/justinlindh/bramble-cli/internal/output"
	bramble "github.com/justinlindh/bramble-go"
	"github.com/spf13/cobra"
)

var runOTAUpdate = func(ctx context.Context, url string) (*bramble.OTAUpdateResponse, error) {
	client, err := getClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	return client.OTAUpdate(ctx, url)
}

func newOTACmd() *cobra.Command {
	var firmwareURL string

	cmd := &cobra.Command{
		Use:   "ota",
		Short: "Trigger OTA firmware update from URL",
		Long:  "Instruct the connected Bramble node to perform an OTA update using a firmware URL.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := commandContext()
			defer cancel()

			resp, err := runOTAUpdate(ctx, firmwareURL)
			if err != nil {
				return fmt.Errorf("bramble-cli: ota: %w", err)
			}

			if flagJSON {
				return output.PrintJSON(os.Stdout, resp)
			}

			fmt.Fprintf(os.Stdout, "OTA update: ok=%t\n", resp.OK)
			if resp.Note != "" {
				fmt.Fprintf(os.Stdout, "Note: %s\n", resp.Note)
			}
			if resp.Partition != "" {
				fmt.Fprintf(os.Stdout, "Partition: %s\n", resp.Partition)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&firmwareURL, "url", "", "firmware URL (http(s)://.../bramble.bin)")
	_ = cmd.MarkFlagRequired("url")

	return cmd
}
