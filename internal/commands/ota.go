package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

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

var runStatusCheck = func(ctx context.Context) error {
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()
	_, err = client.Status(ctx)
	return err
}

var otaSleep = time.Sleep

func newOTACmd() *cobra.Command {
	var firmwareURL string
	var waitForReboot bool
	var rebootTimeout time.Duration
	var pollInterval time.Duration

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

			if !resp.OK {
				return errors.New("bramble-cli: ota rejected by node")
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

			if !waitForReboot {
				return nil
			}

			fmt.Fprintf(os.Stdout, "Waiting up to %s for OTA reboot/reconnect...\n", rebootTimeout)
			deadline := time.Now().Add(rebootTimeout)
			sawDisconnect := false
			for time.Now().Before(deadline) {
				err := runStatusCheck(ctx)
				if err != nil {
					sawDisconnect = true
				} else if sawDisconnect {
					fmt.Fprintln(os.Stdout, "OTA outcome: success (node rebooted and reconnected)")
					return nil
				}
				otaSleep(pollInterval)
			}

			if sawDisconnect {
				return fmt.Errorf("bramble-cli: ota: node disconnected but did not reconnect within %s", rebootTimeout)
			}
			return fmt.Errorf("bramble-cli: ota: no reboot observed within %s (update may have failed or is still running)", rebootTimeout)
		},
	}

	cmd.Flags().StringVar(&firmwareURL, "url", "", "firmware URL (http(s)://.../bramble.bin)")
	cmd.Flags().BoolVar(&waitForReboot, "wait", true, "wait for node reboot/reconnect and report OTA outcome")
	cmd.Flags().DurationVar(&rebootTimeout, "wait-timeout", 2*time.Minute, "max time to wait for OTA reboot/reconnect")
	cmd.Flags().DurationVar(&pollInterval, "poll-interval", 2*time.Second, "status poll interval while waiting for OTA outcome")
	_ = cmd.MarkFlagRequired("url")

	return cmd
}
