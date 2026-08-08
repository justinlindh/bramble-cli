// Package framebuffer decodes raw device framebuffers into Go images.
package framebuffer

import (
	"fmt"
	"image"
	"image/color"
)

// DecodeRGB565 converts a raw rgb565 framebuffer into an RGBA image.
//
// Byte order is LITTLE endian: the low byte of each 16-bit pixel comes first,
// which is how the ESP32 display drivers hand the frame over. Reading the pair
// big endian instead produces an image with correct layout and wrong colours (a
// pink or red cast), which reads as a display or capture fault rather than as a
// decoding mistake, so it is worth being explicit about.
//
// Channel expansion replicates the high bits into the low ones rather than
// left-shifting and leaving zeros, so full-scale input maps to full-scale
// output: 5-bit 0x1F becomes 0xFF, not 0xF8. Without that, a white screen
// decodes as very slightly grey and every image is subtly dark.
func DecodeRGB565(data []byte, width, height int) (*image.RGBA, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("framebuffer: bad dimensions %dx%d", width, height)
	}
	want := width * height * 2
	if len(data) != want {
		return nil, fmt.Errorf("framebuffer: got %d bytes for a %dx%d rgb565 frame, want %d",
			len(data), width, height, want)
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			i := (y*width + x) * 2
			v := uint16(data[i]) | uint16(data[i+1])<<8
			img.SetRGBA(x, y, color.RGBA{
				R: expand5(uint8(v >> 11 & 0x1F)),
				G: expand6(uint8(v >> 5 & 0x3F)),
				B: expand5(uint8(v & 0x1F)),
				A: 0xFF,
			})
		}
	}
	return img, nil
}

// expand5 scales a 5-bit channel to 8 bits so 0x1F maps to 0xFF.
func expand5(v uint8) uint8 { return v<<3 | v>>2 }

// expand6 scales a 6-bit channel to 8 bits so 0x3F maps to 0xFF.
func expand6(v uint8) uint8 { return v<<2 | v>>4 }
