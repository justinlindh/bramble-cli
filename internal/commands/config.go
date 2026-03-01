package commands

import (
	"fmt"
	"os"

	"github.com/justinlindh/bramble-cli/internal/output"
	bramble "github.com/justinlindh/bramble-go"
	"github.com/spf13/cobra"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Read or update node configuration",
		Long:  "Subcommands: get, set-name, set-radio",
	}
	cmd.AddCommand(
		newConfigGetCmd(),
		newConfigSetNameCmd(),
		newConfigSetRadioCmd(),
	)
	return cmd
}

func newConfigGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Print full node configuration",
		RunE:  runConfigGet,
	}
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	cfg, err := client.Config(ctx)
	if err != nil {
		return fmt.Errorf("bramble-cli: get config: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, cfg)
	}

	w := os.Stdout
	fmt.Fprintf(w, "Node:       %s\n", cfg.NodeName)
	fmt.Fprintf(w, "Address:    %s\n", cfg.Address)
	fmt.Fprintf(w, "Radio:\n")
	fmt.Fprintf(w, "  Freq:     %d MHz\n", cfg.Radio.FrequencyMhz)
	fmt.Fprintf(w, "  SF:       %d\n", cfg.Radio.SF)
	fmt.Fprintf(w, "  BW:       %d Hz\n", cfg.Radio.BwHz)
	fmt.Fprintf(w, "  TXPower:  %d dBm\n", cfg.Radio.TxPowerDbm)
	fmt.Fprintf(w, "  Profile:  %s\n", cfg.Radio.Profile)
	fmt.Fprintf(w, "Channels:\n")
	for _, ch := range cfg.Channels {
		def := ""
		if ch.IsDefault {
			def = " (default)"
		}
		lock := ""
		if ch.HasPsk {
			lock = " 🔒"
		}
		fmt.Fprintf(w, "  [%d] %s%s%s\n", ch.ID, ch.Name, lock, def)
	}
	return nil
}

func newConfigSetNameCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-name <name>",
		Short: "Set the node display name (max 32 chars)",
		Args:  cobra.ExactArgs(1),
		RunE:  runConfigSetName,
	}
}

func runConfigSetName(cmd *cobra.Command, args []string) error {
	name := args[0]
	if len(name) > 32 {
		return fmt.Errorf("bramble-cli: name %q is too long (max 32 characters)", name)
	}

	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.SetNodeName(ctx, name); err != nil {
		return fmt.Errorf("bramble-cli: set-name: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, SetNameResult{Name: name, Status: "ok"})
	}
	fmt.Fprintf(os.Stdout, "Node name set to %q\n", name)
	return nil
}

func newConfigSetRadioCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-radio",
		Short: "Update radio parameters",
		Long: `Update one or more radio parameters.

Example:
  bramble config set-radio --freq 915.0 --sf 10 --bw 125 --cr 5 --txpower 20`,
		RunE: runConfigSetRadio,
	}
	cmd.Flags().Float64("freq", 0, "frequency in MHz (e.g. 915.0)")
	cmd.Flags().Int("sf", 0, "spreading factor (7-12)")
	cmd.Flags().Int("bw", 0, "bandwidth in kHz (e.g. 125, 250, 500)")
	cmd.Flags().Int("cr", 0, "coding rate (5-8, meaning 4/5..4/8)")
	cmd.Flags().Int("txpower", 0, "TX power in dBm")
	return cmd
}

func runConfigSetRadio(cmd *cobra.Command, args []string) error {
	config := bramble.RadioConfig{}
	changed := false

	if cmd.Flags().Changed("freq") {
		v, _ := cmd.Flags().GetFloat64("freq")
		config.FreqMhz = &v
		changed = true
	}
	if cmd.Flags().Changed("sf") {
		v, _ := cmd.Flags().GetInt("sf")
		config.SF = &v
		changed = true
	}
	if cmd.Flags().Changed("bw") {
		v, _ := cmd.Flags().GetInt("bw")
		config.BwKhz = &v
		changed = true
	}
	if cmd.Flags().Changed("cr") {
		v, _ := cmd.Flags().GetInt("cr")
		config.CR = &v
		changed = true
	}
	if cmd.Flags().Changed("txpower") {
		v, _ := cmd.Flags().GetInt("txpower")
		config.TxPowerDbm = &v
		changed = true
	}

	if !changed {
		return fmt.Errorf("bramble-cli: specify at least one radio parameter (--freq, --sf, --bw, --cr, --txpower)")
	}

	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.SetRadio(ctx, config); err != nil {
		return fmt.Errorf("bramble-cli: set-radio: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, StatusResult{Status: "ok"})
	}
	fmt.Fprintln(os.Stdout, "Radio parameters updated.")
	return nil
}
