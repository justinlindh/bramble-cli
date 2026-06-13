package commands

import (
	"fmt"
	"os"

	bramble "github.com/justinlindh/bramble-go"
	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/output"
)

func newBeaconPolicyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "beacon-policy",
		Short: "Manage beacon policy",
		Long:  "Subcommands: get, set",
	}
	cmd.AddCommand(
		newBeaconPolicyGetCmd(),
		newBeaconPolicySetCmd(),
	)
	return cmd
}

func newBeaconPolicyGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Show beacon policy",
		Long:  "Display current beacon policy configuration and status.",
		RunE:  runBeaconPolicyGet,
	}
}

func runBeaconPolicyGet(cmd *cobra.Command, args []string) error {
	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	policy, err := client.BeaconPolicy(ctx)
	if err != nil {
		return fmt.Errorf("bramble-cli: get beacon policy: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, policy)
	}

	w := os.Stdout
	fmt.Fprintln(w, "Configuration:")
	fmt.Fprintf(w, "  Enabled:          %s\n", boolStr(policy.Config.Enabled, "yes", "no"))
	fmt.Fprintf(w, "  Mode:             %s\n", policy.Config.Mode)
	fmt.Fprintf(w, "  Base interval:    %s\n", output.FormatMs(int64(policy.Config.BaseIntervalMs)))
	fmt.Fprintf(w, "  Min interval:     %s\n", output.FormatMs(int64(policy.Config.MinIntervalMs)))
	fmt.Fprintf(w, "  Max interval:     %s\n", output.FormatMs(int64(policy.Config.MaxIntervalMs)))
	fmt.Fprintf(w, "  Dense threshold:  %d\n", policy.Config.DenseThreshold)
	fmt.Fprintf(w, "  Churn threshold:  %d\n", policy.Config.ChurnThreshold)
	fmt.Fprintf(w, "  Churn window:     %s\n", output.FormatMs(int64(policy.Config.ChurnWindowMs)))
	fmt.Fprintln(w, "Status:")
	fmt.Fprintf(w, "  Active mode:      %s\n", policy.Status.ActiveMode)
	fmt.Fprintf(w, "  Current interval: %s\n", output.FormatMs(int64(policy.Status.CurrentIntervalMs)))
	fmt.Fprintf(w, "  Neighbor count:   %d\n", policy.Status.NeighborCount)
	fmt.Fprintf(w, "  Churn events:     %d\n", policy.Status.ChurnEvents)
	fmt.Fprintf(w, "  In backoff:       %s\n", boolStr(policy.Status.InBackoff, "yes", "no"))
	return nil
}

var (
	bpEnabled        string
	bpMode           string
	bpBaseIntervalMs int
	bpMinIntervalMs  int
	bpMaxIntervalMs  int
	bpDenseThreshold int
	bpChurnThreshold int
	bpChurnWindowMs  int
)

func newBeaconPolicySetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Update beacon policy",
		Long: `Update beacon policy configuration. All flags are optional; only provided
values are sent to the node.

Examples:
  bramble beacon-policy set --enabled true --mode adaptive
  bramble beacon-policy set --base-interval-ms 5000`,
		RunE: runBeaconPolicySet,
	}
	cmd.Flags().StringVar(&bpEnabled, "enabled", "", "enable or disable beacons (true/false)")
	cmd.Flags().StringVar(&bpMode, "mode", "", "beacon mode (e.g. fixed, adaptive)")
	cmd.Flags().IntVar(&bpBaseIntervalMs, "base-interval-ms", 0, "base beacon interval in milliseconds")
	cmd.Flags().IntVar(&bpMinIntervalMs, "min-interval-ms", 0, "minimum beacon interval in milliseconds")
	cmd.Flags().IntVar(&bpMaxIntervalMs, "max-interval-ms", 0, "maximum beacon interval in milliseconds")
	cmd.Flags().IntVar(&bpDenseThreshold, "dense-threshold", 0, "dense network threshold")
	cmd.Flags().IntVar(&bpChurnThreshold, "churn-threshold", 0, "churn event threshold")
	cmd.Flags().IntVar(&bpChurnWindowMs, "churn-window-ms", 0, "churn detection window in milliseconds")
	return cmd
}

func runBeaconPolicySet(cmd *cobra.Command, args []string) error {
	params := bramble.SetBeaconPolicyParams{}

	if cmd.Flags().Changed("enabled") {
		v, err := parseBoolArg(bpEnabled, "enabled")
		if err != nil {
			return err
		}
		params.Enabled = &v
	}
	if cmd.Flags().Changed("mode") {
		params.Mode = bpMode
	}
	if cmd.Flags().Changed("base-interval-ms") {
		params.BaseIntervalMs = &bpBaseIntervalMs
	}
	if cmd.Flags().Changed("min-interval-ms") {
		params.MinIntervalMs = &bpMinIntervalMs
	}
	if cmd.Flags().Changed("max-interval-ms") {
		params.MaxIntervalMs = &bpMaxIntervalMs
	}
	if cmd.Flags().Changed("dense-threshold") {
		params.DenseThreshold = &bpDenseThreshold
	}
	if cmd.Flags().Changed("churn-threshold") {
		params.ChurnThreshold = &bpChurnThreshold
	}
	if cmd.Flags().Changed("churn-window-ms") {
		params.ChurnWindowMs = &bpChurnWindowMs
	}

	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.SetBeaconPolicy(ctx, params); err != nil {
		return fmt.Errorf("bramble-cli: set beacon policy: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, StatusResult{Status: "ok"})
	}
	fmt.Fprintln(os.Stdout, "Beacon policy updated.")
	return nil
}
