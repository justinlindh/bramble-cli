package commands

import (
	"fmt"
	"os"
	"time"

	"github.com/justinlindh/bramble-cli/internal/output"
	"github.com/spf13/cobra"
)

func newLocationCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "location",
		Short: "Location sharing management",
		Long:  "Subcommands: status, set-contact, remove-contact, share-once",
	}
	cmd.AddCommand(
		newLocationStatusCmd(),
		newLocationSetContactCmd(),
		newLocationRemoveContactCmd(),
		newLocationShareOnceCmd(),
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
