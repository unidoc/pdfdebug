// Unit tests for the image decode ceiling, which sizes the bound handed to
// decodeBounded from the geometry an image dictionary declares.
package pdfcore

import (
	"math"
	"testing"
)

func TestImageDecodeCeiling_HonestLargeImageGetsRoomToDecode(t *testing.T) {
	// 8192 x 8192 8-bit grayscale is 64 MiB of samples: over the extraction
	// ceiling, but an ordinary scan rather than a bomb.
	const declared = int64(8192) * 8192 * 8 * 1 / 8
	want := declared*2 + imageDecodeHeadroom
	if got := imageDecodeCeiling(8192, 8192, 8, 1); got != want {
		t.Errorf("got %d, want %d (twice the %d declared bytes plus headroom)", got, want, declared)
	}
}

// The doubling and the headroom apply to the declared size, not to a rounded or
// truncated version of it, so a geometry whose byte size is not a whole multiple
// still lands exactly where the formula says.
func TestImageDecodeCeiling_TracksDeclaredSizeExactly(t *testing.T) {
	for _, c := range []struct {
		width, height, bitsPerComponent, comps int
	}{
		{4000, 4000, 8, 3},
		{6000, 4000, 8, 3},
		{5000, 5000, 16, 1},
		{3000, 3000, 4, 3},
	} {
		declared := int64(c.width) * int64(c.height) * int64(c.bitsPerComponent) * int64(c.comps) / 8
		want := declared*2 + imageDecodeHeadroom
		if want < maxImageBytes {
			want = maxImageBytes
		}
		got := imageDecodeCeiling(c.width, c.height, c.bitsPerComponent, c.comps)
		if got != want {
			t.Errorf("%dx%d bpc=%d comps=%d: got %d, want %d",
				c.width, c.height, c.bitsPerComponent, c.comps, got, want)
		}
	}
}

func TestImageDecodeCeiling_SmallImageKeepsTheExtractionCeiling(t *testing.T) {
	// A small picture must not be held to a tighter bound than before.
	if got := imageDecodeCeiling(8, 8, 8, 1); got != maxImageBytes {
		t.Errorf("got %d, want the maxImageBytes floor %d", got, maxImageBytes)
	}
}

func TestImageDecodeCeiling_UnusableGeometryFallsBackToExtractionCeiling(t *testing.T) {
	cases := []struct {
		name                                   string
		width, height, bitsPerComponent, comps int
	}{
		{"no width", 0, 100, 8, 1},
		{"no height", 100, 0, 8, 1},
		{"negative width", -1, 100, 8, 1},
		{"no bits per component", 100, 100, 0, 1},
		{"unknown color space", 100, 100, 8, 0},
		{"implausible bits per component", 100, 100, 1 << 20, 1},
		{"implausible component count", 100, 100, 8, 1 << 20},
	}
	for _, c := range cases {
		if got := imageDecodeCeiling(c.width, c.height, c.bitsPerComponent, c.comps); got != maxImageBytes {
			t.Errorf("%s: got %d, want the maxImageBytes fallback %d", c.name, got, maxImageBytes)
		}
	}
}

// A geometry that wraps the arithmetic falls back to the extraction ceiling
// rather than the wider absolute cap: an unusable declaration must not buy more
// room to inflate than an honest image gets.
func TestImageDecodeCeiling_WraparoundFallsBackTight(t *testing.T) {
	got := imageDecodeCeiling(math.MaxInt, math.MaxInt, 16, 4)
	if got != maxImageBytes {
		t.Errorf("got %d, want the maxImageBytes fallback %d", got, maxImageBytes)
	}
}

// Being large is not the same as being implausible. A scan past the render
// path's pixel cap still gets room for the samples its geometry declares, so a
// real large-format document is not refused for its size alone.
func TestImageDecodeCeiling_LargeScanPastThePixelCapStillGetsRoom(t *testing.T) {
	// 12000 x 9000 8-bit grayscale: 108 megapixels, 108 MB of samples.
	const width, height = 12000, 9000
	declared := int64(width) * height
	if declared <= maxImagePixels {
		t.Fatalf("fixture no longer exceeds the pixel cap: %d", declared)
	}
	got := imageDecodeCeiling(width, height, 8, 1)
	if got < declared {
		t.Errorf("ceiling %d is below the %d bytes the geometry declares", got, declared)
	}
	if got == maxImageBytes {
		t.Errorf("a large honest scan must not be pinned to the maxImageBytes fallback")
	}
}

// A geometry inside the pixel guard whose declared size still doubles past the
// absolute cap saturates at the cap rather than returning the larger figure.
func TestImageDecodeCeiling_LargeDeclaredSizeSaturatesAtAbsoluteCap(t *testing.T) {
	// 100M pixels is exactly the pixel guard, so this reaches the arithmetic;
	// 4 components at 8 bits declares 400 MB, and twice that is over the cap.
	const side = 10000
	declared := int64(side) * side * 8 * 4 / 8
	if declared*2+imageDecodeHeadroom <= maxImageDecodeBytes {
		t.Fatalf("fixture no longer exceeds the cap: declared %d", declared)
	}
	if got := imageDecodeCeiling(side, side, 8, 4); got != maxImageDecodeBytes {
		t.Errorf("got %d, want the absolute cap %d", got, maxImageDecodeBytes)
	}
}

// The ceiling never exceeds the absolute cap, whatever the geometry claims.
func TestImageDecodeCeiling_NeverExceedsAbsoluteCap(t *testing.T) {
	for _, comps := range []int{1, 3, 4, 32} {
		for _, bpc := range []int{1, 8, 16} {
			for _, side := range []int{100, 5000, 10000, 40000} {
				if got := imageDecodeCeiling(side, side, bpc, comps); got > maxImageDecodeBytes {
					t.Errorf("%dx%d bpc=%d comps=%d: ceiling %d exceeds the cap %d",
						side, side, bpc, comps, got, maxImageDecodeBytes)
				}
			}
		}
	}
}
