package framebuffer

import (
	"image/color"
	"testing"
)

// le encodes a 16-bit pixel little endian, the way the device sends it.
func le(v uint16) []byte { return []byte{byte(v), byte(v >> 8)} }

func rgb565(r, g, b uint16) uint16 { return r<<11 | g<<5 | b }

func TestDecodeRGB565PrimariesAreFullScale(t *testing.T) {
	// The rounding rule under test: a saturated 5- or 6-bit channel must decode
	// to 0xFF, not 0xF8/0xFC. A plain left shift makes every image subtly dark
	// and white pixels slightly grey.
	cases := []struct {
		name  string
		pixel uint16
		want  color.RGBA
	}{
		{"black", rgb565(0, 0, 0), color.RGBA{0, 0, 0, 255}},
		{"white", rgb565(31, 63, 31), color.RGBA{255, 255, 255, 255}},
		{"red", rgb565(31, 0, 0), color.RGBA{255, 0, 0, 255}},
		{"green", rgb565(0, 63, 0), color.RGBA{0, 255, 0, 255}},
		{"blue", rgb565(0, 0, 31), color.RGBA{0, 0, 255, 255}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img, err := DecodeRGB565(le(tc.pixel), 1, 1)
			if err != nil {
				t.Fatalf("DecodeRGB565: %v", err)
			}
			if got := img.RGBAAt(0, 0); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDecodeRGB565IsLittleEndian(t *testing.T) {
	// This is the one that bites: pure red is 0xF800, whose little endian bytes
	// are 00 F8. A big endian decoder reads those same bytes as 0x00F8, which is
	// a dark blue, and a whole frame decoded that way comes out with a pink or
	// red cast that looks like a hardware fault.
	img, err := DecodeRGB565([]byte{0x00, 0xF8}, 1, 1)
	if err != nil {
		t.Fatalf("DecodeRGB565: %v", err)
	}
	got := img.RGBAAt(0, 0)
	if want := (color.RGBA{255, 0, 0, 255}); got != want {
		t.Fatalf("got %v, want %v: bytes 00 F8 must read as 0xF800 (red), not 0x00F8", got, want)
	}
}

func TestDecodeRGB565IsRowMajor(t *testing.T) {
	// Four distinct pixels in a 2x2 frame pin the pixel ordering, so a
	// transposed or column-major read cannot pass.
	data := make([]byte, 0, 8)
	data = append(data, le(rgb565(31, 0, 0))...)   // (0,0) red
	data = append(data, le(rgb565(0, 63, 0))...)   // (1,0) green
	data = append(data, le(rgb565(0, 0, 31))...)   // (0,1) blue
	data = append(data, le(rgb565(31, 63, 31))...) // (1,1) white

	img, err := DecodeRGB565(data, 2, 2)
	if err != nil {
		t.Fatalf("DecodeRGB565: %v", err)
	}

	want := map[[2]int]color.RGBA{
		{0, 0}: {255, 0, 0, 255},
		{1, 0}: {0, 255, 0, 255},
		{0, 1}: {0, 0, 255, 255},
		{1, 1}: {255, 255, 255, 255},
	}
	for at, w := range want {
		if got := img.RGBAAt(at[0], at[1]); got != w {
			t.Errorf("pixel (%d,%d) = %v, want %v", at[0], at[1], got, w)
		}
	}
}

func TestDecodeRGB565BoundsMatchRequestedSize(t *testing.T) {
	img, err := DecodeRGB565(make([]byte, 320*240*2), 320, 240)
	if err != nil {
		t.Fatalf("DecodeRGB565: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 320 || b.Dy() != 240 {
		t.Errorf("bounds are %dx%d, want 320x240", b.Dx(), b.Dy())
	}
}

func TestDecodeRGB565RejectsWrongByteCount(t *testing.T) {
	// A short frame is what a broken pager loop produces, and decoding it
	// anyway would yield a half-drawn image that looks like a device fault.
	for _, n := range []int{0, 7, 9} {
		if _, err := DecodeRGB565(make([]byte, n), 2, 2); err == nil {
			t.Errorf("%d bytes for a 2x2 frame was accepted, want an error", n)
		}
	}
}

func TestDecodeRGB565RejectsBadDimensions(t *testing.T) {
	for _, d := range [][2]int{{0, 10}, {10, 0}, {-1, 10}} {
		if _, err := DecodeRGB565(nil, d[0], d[1]); err == nil {
			t.Errorf("dimensions %dx%d were accepted, want an error", d[0], d[1])
		}
	}
}
