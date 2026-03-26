package commands

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/output"
)

func newGpsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "gps",
		Short: "Show GPS position",
		Long:  "Display current GPS position including latitude, longitude, altitude, speed, and heading.",
		RunE:  runGps,
	}
}

func runGps(cmd *cobra.Command, args []string) error {
	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	pos, err := client.GetGpsPosition(ctx)
	if err != nil {
		return fmt.Errorf("bramble-cli: get gps position: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, pos)
	}

	w := os.Stdout
	fmt.Fprintf(w, "Valid:      %s\n", boolStr(pos.Valid, "yes", "no"))
	fmt.Fprintf(w, "Latitude:  %.6f\n", pos.Lat)
	fmt.Fprintf(w, "Longitude: %.6f\n", pos.Lon)
	fmt.Fprintf(w, "Altitude:  %.1f m\n", pos.Alt)
	fmt.Fprintf(w, "Speed:     %.1f km/h\n", pos.SpeedKmh)
	fmt.Fprintf(w, "Heading:   %.1f°\n", pos.HeadingDeg)
	fmt.Fprintf(w, "Accuracy:  %.1f m\n", pos.AccuracyM)
	if pos.Timestamp > 0 {
		t := time.Unix(pos.Timestamp, 0).UTC()
		fmt.Fprintf(w, "Fix time:  %s\n", t.Format(time.RFC3339))
	}
	return nil
}
