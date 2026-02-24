package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/justinlindh/bramble-cli/internal/output"
	bramble "github.com/justinlindh/bramble-go"
	"github.com/spf13/cobra"
)

func newLocationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "location",
		Short: "Location sharing management",
		Long:  "Subcommands: status, set-contact, remove-contact, share-once, get-config, set-config",
	}
	cmd.AddCommand(
		newLocationStatusCmd(),
		newLocationSetContactCmd(),
		newLocationRemoveContactCmd(),
		newLocationShareOnceCmd(),
		newLocationGetConfigCmd(),
		newLocationSetConfigCmd(),
	)
	return cmd
}

func newLocationStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show peer location data",
		RunE:  runLocationStatus,
	}
}

func runLocationStatus(cmd *cobra.Command, args []string) error {
	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	peers, err := client.PeerLocations(ctx)
	if err != nil {
		return fmt.Errorf("bramble-cli: get peer locations: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, peers)
	}

	if len(peers) == 0 {
		fmt.Fprintln(os.Stdout, "No peer location data available.")
		return nil
	}

	headers := []string{"ADDRESS", "NAME", "TIER", "LAT", "LON", "ONLINE", "UPDATED"}
	rows := make([][]string, len(peers))
	for i, p := range peers {
		lat, lon := "N/A", "N/A"
		if p.Position != nil {
			lat = fmt.Sprintf("%.5f", p.Position.Lat)
			lon = fmt.Sprintf("%.5f", p.Position.Lon)
		}
		online := "no"
		if p.Online {
			online = "yes"
		}
		updated := "unknown"
		if p.LastUpdatedMs > 0 {
			t := time.Unix(p.LastUpdatedMs/1000, 0)
			updated = t.Format("15:04:05")
		}
		rows[i] = []string{
			p.Addr,
			p.Name,
			p.Tier,
			lat,
			lon,
			online,
			updated,
		}
	}
	output.Table(os.Stdout, headers, rows)
	return nil
}

func newLocationSetContactCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-contact <address> <tier>",
		Short: "Add or update a location sharing contact",
		Long: `Configure location sharing with a specific peer.
Tier controls what data is shared: "exact", "city", or "region".

Example:
  bramble location set-contact DEADBEEF exact
  bramble location set-contact CAFEBABE city`,
		Args: cobra.ExactArgs(2),
		RunE: runLocationSetContact,
	}
	return cmd
}

func runLocationSetContact(cmd *cobra.Command, args []string) error {
	addr, err := ParseAddress(args[0])
	if err != nil {
		return fmt.Errorf("bramble-cli: invalid address %q: %w", args[0], err)
	}
	tier := args[1]

	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.SetLocationContact(ctx, addr, tier); err != nil {
		return fmt.Errorf("bramble-cli: set-contact: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, map[string]any{
			"addr":   output.Addr(addr),
			"tier":   tier,
			"status": "ok",
		})
	}
	fmt.Fprintf(os.Stdout, "Location contact %s set to tier %q\n", output.Addr(addr), tier)
	return nil
}

func newLocationRemoveContactCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove-contact <address>",
		Short: "Stop sharing location with a peer",
		Args:  cobra.ExactArgs(1),
		RunE:  runLocationRemoveContact,
	}
}

func runLocationRemoveContact(cmd *cobra.Command, args []string) error {
	addr, err := ParseAddress(args[0])
	if err != nil {
		return fmt.Errorf("bramble-cli: invalid address %q: %w", args[0], err)
	}

	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.RemoveLocationContact(ctx, addr); err != nil {
		return fmt.Errorf("bramble-cli: remove-contact: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, map[string]any{"addr": output.Addr(addr), "status": "removed"})
	}
	fmt.Fprintf(os.Stdout, "Removed location contact %s\n", output.Addr(addr))
	return nil
}

func newLocationShareOnceCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "share-once <address>",
		Short: "Send a one-time location update to a peer",
		Args:  cobra.ExactArgs(1),
		RunE:  runLocationShareOnce,
	}
}

func runLocationShareOnce(cmd *cobra.Command, args []string) error {
	addr, err := ParseAddress(args[0])
	if err != nil {
		return fmt.Errorf("bramble-cli: invalid address %q: %w", args[0], err)
	}

	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.ShareLocationOnce(ctx, addr); err != nil {
		return fmt.Errorf("bramble-cli: share-once: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, map[string]any{"addr": output.Addr(addr), "status": "sent"})
	}
	fmt.Fprintf(os.Stdout, "Location shared with %s\n", output.Addr(addr))
	return nil
}

func newLocationGetConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get-config",
		Short: "Fetch current location configuration",
		RunE:  runLocationGetConfig,
	}
}

func runLocationGetConfig(cmd *cobra.Command, args []string) error {
	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	cfg, err := client.Config(ctx)
	if err != nil {
		return fmt.Errorf("bramble-cli: get location config: %w", err)
	}

	return output.PrintJSON(os.Stdout, cfg.Location)
}

func newLocationSetConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-config",
		Short: "Update location sharing configuration",
		RunE:  runLocationSetConfig,
	}
	cmd.Flags().String("file", "", "path to JSON file containing canonical location config fields")
	cmd.Flags().Bool("enabled", false, "enable/disable location sharing")
	cmd.Flags().String("default-tier", "", "default sharing tier")
	cmd.Flags().Int("interval-s", 0, "default share interval in seconds")
	cmd.Flags().String("source", "", "location source")
	cmd.Flags().String("contact-rules", "", "JSON array for contact_rules")
	cmd.Flags().String("channel-targets", "", "JSON array for channel_targets")
	return cmd
}

func runLocationSetConfig(cmd *cobra.Command, args []string) error {
	locationCfg, changed, err := buildLocationConfigFromInput(cmd)
	if err != nil {
		return err
	}
	if !changed {
		return fmt.Errorf("bramble-cli: specify at least one location config field")
	}

	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.SetLocationConfig(ctx, locationCfg); err != nil {
		return fmt.Errorf("bramble-cli: set location config: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, map[string]any{"status": "ok", "location": locationCfg})
	}
	fmt.Fprintln(os.Stdout, "Location config updated.")
	return nil
}

func buildLocationConfigFromInput(cmd *cobra.Command) (bramble.LocationConfig, bool, error) {
	cfg := bramble.LocationConfig{}
	changed := false

	if cmd.Flags().Changed("file") {
		path, _ := cmd.Flags().GetString("file")
		body, err := os.ReadFile(path)
		if err != nil {
			return bramble.LocationConfig{}, false, fmt.Errorf("bramble-cli: read location config file: %w", err)
		}
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&cfg); err != nil {
			return bramble.LocationConfig{}, false, fmt.Errorf("bramble-cli: decode location config file: %w", err)
		}
		changed = true
	}

	if cmd.Flags().Changed("enabled") {
		v, _ := cmd.Flags().GetBool("enabled")
		cfg.Enabled = &v
		changed = true
	}
	if cmd.Flags().Changed("default-tier") {
		v, _ := cmd.Flags().GetString("default-tier")
		cfg.DefaultTier = &v
		changed = true
	}
	if cmd.Flags().Changed("interval-s") {
		v, _ := cmd.Flags().GetInt("interval-s")
		cfg.IntervalS = &v
		changed = true
	}
	if cmd.Flags().Changed("source") {
		v, _ := cmd.Flags().GetString("source")
		cfg.Source = &v
		changed = true
	}
	if cmd.Flags().Changed("contact-rules") {
		v, _ := cmd.Flags().GetString("contact-rules")
		if err := json.Unmarshal([]byte(v), &cfg.ContactRules); err != nil {
			return bramble.LocationConfig{}, false, fmt.Errorf("bramble-cli: parse contact_rules JSON: %w", err)
		}
		changed = true
	}
	if cmd.Flags().Changed("channel-targets") {
		v, _ := cmd.Flags().GetString("channel-targets")
		if err := json.Unmarshal([]byte(v), &cfg.ChannelTargets); err != nil {
			return bramble.LocationConfig{}, false, fmt.Errorf("bramble-cli: parse channel_targets JSON: %w", err)
		}
		changed = true
	}

	return cfg, changed, nil
}
