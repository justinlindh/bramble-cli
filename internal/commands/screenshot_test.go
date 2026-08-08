package commands

import (
	"bytes"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	bramble "github.com/justinlindh/bramble-go"
	"github.com/spf13/cobra"
)

func redFrame(w, h int) []byte {
	// 0xF800 is rgb565 red; little endian on the wire is 00 F8.
	buf := make([]byte, w*h*2)
	for i := 0; i < len(buf); i += 2 {
		buf[i], buf[i+1] = 0x00, 0xF8
	}
	return buf
}

func TestDecodeScreenshotRGB565(t *testing.T) {
	img, err := decodeScreenshot(&bramble.Screenshot{
		Width: 2, Height: 2, Format: "rgb565", Pixels: redFrame(2, 2),
	})
	if err != nil {
		t.Fatalf("decodeScreenshot: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 2 || b.Dy() != 2 {
		t.Fatalf("bounds are %v, want 2x2", b)
	}
	r, g, b, _ := img.At(0, 0).RGBA()
	if want := (color.RGBA{255, 0, 0, 255}); uint8(r>>8) != want.R || uint8(g>>8) != want.G || uint8(b>>8) != want.B {
		t.Errorf("pixel decoded as r=%d g=%d b=%d, want red", r>>8, g>>8, b>>8)
	}
}

func TestDecodeScreenshotEmptyFormatIsTreatedAsRGB565(t *testing.T) {
	// Older firmware sends no format string, and rgb565 is the only format the
	// screenshot RPC has ever produced. Refusing it would break those boards.
	if _, err := decodeScreenshot(&bramble.Screenshot{
		Width: 2, Height: 2, Format: "", Pixels: redFrame(2, 2),
	}); err != nil {
		t.Fatalf("decodeScreenshot: %v", err)
	}
}

func TestDecodeScreenshotRejectsUnknownFormat(t *testing.T) {
	// Guessing at an unknown format would write a plausible looking PNG full of
	// garbage, which is worse than saying so.
	_, err := decodeScreenshot(&bramble.Screenshot{
		Width: 2, Height: 2, Format: "rgb888", Pixels: make([]byte, 12),
	})
	if err == nil || !strings.Contains(err.Error(), "rgb888") {
		t.Fatalf("error was %v, want it to name the unsupported format", err)
	}
}

func TestDecodeScreenshotRejectsShortFrame(t *testing.T) {
	_, err := decodeScreenshot(&bramble.Screenshot{
		Width: 320, Height: 240, Format: "rgb565", Pixels: make([]byte, 100),
	})
	if err == nil {
		t.Fatal("a short frame was accepted")
	}
}

func TestWriteScreenshotPNGToFile(t *testing.T) {
	img, err := decodeScreenshot(&bramble.Screenshot{
		Width: 4, Height: 4, Format: "rgb565", Pixels: redFrame(4, 4),
	})
	if err != nil {
		t.Fatalf("decodeScreenshot: %v", err)
	}

	path := filepath.Join(t.TempDir(), "nested", "shot.png")
	cmd := &cobra.Command{}
	if err := writeScreenshotPNG(cmd, path, img); err != nil {
		t.Fatalf("writeScreenshotPNG: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open written file: %v", err)
	}
	defer func() { _ = f.Close() }()

	decoded, err := png.Decode(f)
	if err != nil {
		t.Fatalf("the written file is not a valid PNG: %v", err)
	}
	if b := decoded.Bounds(); b.Dx() != 4 || b.Dy() != 4 {
		t.Errorf("written PNG is %v, want 4x4", b)
	}
}

func TestWriteScreenshotPNGToStdout(t *testing.T) {
	img, err := decodeScreenshot(&bramble.Screenshot{
		Width: 2, Height: 2, Format: "rgb565", Pixels: redFrame(2, 2),
	})
	if err != nil {
		t.Fatalf("decodeScreenshot: %v", err)
	}

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&out)
	if err := writeScreenshotPNG(cmd, "-", img); err != nil {
		t.Fatalf("writeScreenshotPNG: %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(out.Bytes())); err != nil {
		t.Fatalf("stdout did not receive a valid PNG: %v", err)
	}
}
