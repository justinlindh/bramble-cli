package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/output"
)

func newAudioCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audio",
		Short: "Audio controls",
		Long:  "Subcommands: status, set-volume, set-muted, play-tone",
	}
	cmd.AddCommand(
		newAudioStatusCmd(),
		newAudioSetVolumeCmd(),
		newAudioSetMutedCmd(),
		newAudioPlayToneCmd(),
	)
	return cmd
}

func newAudioStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show audio status",
		Long:  "Display audio availability, volume level, mute state, and playback status.",
		RunE:  runAudioStatus,
	}
}

func runAudioStatus(cmd *cobra.Command, args []string) error {
	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	status, err := client.AudioStatus(ctx)
	if err != nil {
		return fmt.Errorf("bramble-cli: get audio status: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, status)
	}

	w := os.Stdout
	fmt.Fprintf(w, "Available: %s\n", boolStr(status.Available, "yes", "no"))
	fmt.Fprintf(w, "Volume:    %d\n", status.Volume)
	fmt.Fprintf(w, "Muted:     %s\n", boolStr(status.Muted, "yes", "no"))
	fmt.Fprintf(w, "Playing:   %s\n", boolStr(status.Playing, "yes", "no"))
	return nil
}

func newAudioSetVolumeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-volume <level>",
		Short: "Set audio volume",
		Long:  "Set the audio volume level (0-100).",
		Args:  cobra.ExactArgs(1),
		RunE:  runAudioSetVolume,
	}
	return cmd
}

func runAudioSetVolume(cmd *cobra.Command, args []string) error {
	vol, err := parseIntArg(args[0], "volume")
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

	if err := client.SetVolume(ctx, vol); err != nil {
		return fmt.Errorf("bramble-cli: set volume: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, StatusResult{Status: "ok"})
	}
	fmt.Fprintf(os.Stdout, "Volume set to %d\n", vol)
	return nil
}

func newAudioSetMutedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set-muted <true|false>",
		Short: "Set audio mute state",
		Long:  "Mute or unmute audio output.",
		Args:  cobra.ExactArgs(1),
		RunE:  runAudioSetMuted,
	}
	return cmd
}

func runAudioSetMuted(cmd *cobra.Command, args []string) error {
	muted, err := parseBoolArg(args[0], "muted")
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

	if err := client.SetMuted(ctx, muted); err != nil {
		return fmt.Errorf("bramble-cli: set muted: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, StatusResult{Status: "ok"})
	}
	fmt.Fprintf(os.Stdout, "Muted: %s\n", boolStr(muted, "yes", "no"))
	return nil
}

func newAudioPlayToneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "play-tone <tone>",
		Short: "Play an audio tone",
		Long:  "Play a named tone on the node (e.g. alert, notification).",
		Args:  cobra.ExactArgs(1),
		RunE:  runAudioPlayTone,
	}
	return cmd
}

func runAudioPlayTone(cmd *cobra.Command, args []string) error {
	tone := args[0]

	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	if err := client.PlayTone(ctx, tone); err != nil {
		return fmt.Errorf("bramble-cli: play tone: %w", err)
	}

	if flagJSON {
		return output.PrintJSON(os.Stdout, StatusResult{Status: "ok"})
	}
	fmt.Fprintf(os.Stdout, "Playing tone: %s\n", tone)
	return nil
}
