package commands

import (
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	bramble "github.com/justinlindh/bramble-go"
	"github.com/spf13/cobra"

	"github.com/justinlindh/bramble-cli/internal/framebuffer"
	"github.com/justinlindh/bramble-cli/internal/output"
)

// screenshotResult is the --json shape. The pixels are deliberately not in it:
// a framebuffer is megabytes of base64 and belongs in the file, not on stdout.
type screenshotResult struct {
	Path   string `json:"path"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Format string `json:"format"`
	Bytes  int    `json:"bytes"`
}

func newScreenshotCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "screenshot",
		Short: "Capture the device display to a PNG",
		Long: `Capture the connected node's display and write it as a PNG.

The framebuffer does not fit in one RPC response, so it arrives in chunks; the
SDK issues the capture and pages the whole frame back before it is decoded, so
the result is one consistent image rather than a composite.

Only boards with a graphical UI can answer this. A headless build reports
"no graphical ui".

Examples:
  bramble screenshot --out shot.png
  bramble --port /dev/ttyUSB0 screenshot --out /tmp/tdeck.png
  bramble screenshot --out - > shot.png`,
		RunE: runScreenshot,
	}
	cmd.Flags().StringP("out", "o", "screenshot.png", `output PNG path, or "-" for stdout`)
	return cmd
}

func runScreenshot(cmd *cobra.Command, args []string) error {
	out, _ := cmd.Flags().GetString("out")

	ctx, cancel := commandContext()
	defer cancel()
	client, err := getClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	shot, err := client.Screenshot(ctx)
	if err != nil {
		return fmt.Errorf("bramble-cli: screenshot: %w", err)
	}

	img, err := decodeScreenshot(shot)
	if err != nil {
		return fmt.Errorf("bramble-cli: %w", err)
	}

	if err := writeScreenshotPNG(cmd, out, img); err != nil {
		return err
	}

	if flagJSON {
		return output.PrintJSON(cmd.OutOrStdout(), screenshotResult{
			Path:   out,
			Width:  shot.Width,
			Height: shot.Height,
			Format: shot.Format,
			Bytes:  len(shot.Pixels),
		})
	}
	if out == "-" {
		return nil
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Wrote %s (%dx%d, %s)\n", out, shot.Width, shot.Height, shot.Format)
	return nil
}

// decodeScreenshot turns a captured frame into an image. Split out so the
// format dispatch is testable without a device.
func decodeScreenshot(shot *bramble.Screenshot) (image.Image, error) {
	switch strings.ToLower(shot.Format) {
	case "rgb565", "":
		// An empty format is what older firmware sends, and rgb565 is the only
		// format the screenshot RPC has ever produced.
		return framebuffer.DecodeRGB565(shot.Pixels, shot.Width, shot.Height)
	default:
		return nil, fmt.Errorf("unsupported framebuffer format %q", shot.Format)
	}
}

// writeScreenshotPNG writes to a path, or to stdout when the path is "-".
// The file is created before encoding so a bad path fails immediately rather
// than after a full frame has been re-encoded.
func writeScreenshotPNG(cmd *cobra.Command, path string, img image.Image) error {
	if path == "-" {
		return encodePNG(cmd.OutOrStdout(), img)
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("bramble-cli: create %s: %w", dir, err)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("bramble-cli: create %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	if err := encodePNG(f, img); err != nil {
		return err
	}
	return f.Close()
}

func encodePNG(w io.Writer, img image.Image) error {
	if err := png.Encode(w, img); err != nil {
		return fmt.Errorf("bramble-cli: encode PNG: %w", err)
	}
	return nil
}
