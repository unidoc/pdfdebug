// Unit tests for the image decode ceiling, which sizes the bound handed to
// decodeBounded from the geometry an image dictionary declares.
package pdfcore

import "testing"

func TestImageDecodeCeiling_HonestLargeImageGetsRoomToDecode(t *testing.T) {
	// 8192 x 8192 8-bit grayscale is 64 MiB of samples: over the extraction
	// ceiling, but an ordinary scan rather than a bomb.
	got := imageDecodeCeiling(8192, 8192, 8, 1)
	const declared = int64(8192) * 8192
	if got <= declared {
		t.Fatalf("ceiling %d must exceed the %d bytes the geometry declares", got, declared)
	}
	if got <= maxImageBytes {
		t.Errorf("ceiling %d should be raised above maxImageBytes for a large declared image", got)
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

// An absurd geometry must not wrap into a small positive ceiling, which would
// reject the image instead of bounding it.
func TestImageDecodeCeiling_AbsurdGeometryIsCappedNotWrapped(t *testing.T) {
	for _, c := range []struct {
		name          string
		width, height int
	}{
		{"overflowing product", 1 << 40, 1 << 40},
		{"past the pixel cap", 40000, 40000},
	} {
		got := imageDecodeCeiling(c.width, c.height, 16, 4)
		if got != maxImageDecodeBytes {
			t.Errorf("%s: got %d, want the absolute cap %d", c.name, got, maxImageDecodeBytes)
		}
	}
}

// The ceiling never exceeds the absolute cap, whatever the geometry claims.
func TestImageDecodeCeiling_NeverExceedsAbsoluteCap(t *testing.T) {
	for _, comps := range []int{1, 3, 4, 32} {
		for _, bpc := range []int{1, 8, 16, 64} {
			if got := imageDecodeCeiling(20000, 20000, bpc, comps); got > maxImageDecodeBytes {
				t.Errorf("bpc=%d comps=%d: ceiling %d exceeds the cap %d",
					bpc, comps, got, maxImageDecodeBytes)
			}
		}
	}
}
