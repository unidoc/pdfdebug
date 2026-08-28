// Unit tests for the image decode ceiling, which sizes the bound handed to
// decodeBounded from the geometry an image dictionary declares.
package pdfcore

import (
	"math"
	"testing"

	pdfcpu_render "github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
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

// Hand-computed expectations, so a wrong formula cannot be transcribed into both
// the code and the test. Each is ceil(width*bitsPerPixel/8)*height, doubled,
// plus the 1 MiB headroom.
func TestImageDecodeCeiling_MatchesHandComputedValues(t *testing.T) {
	for _, c := range []struct {
		width, height, bitsPerComponent, comps int
		want                                   int64
	}{
		// 12000 bytes per row x 4000 rows = 48,000,000 declared.
		{4000, 4000, 8, 3, 97_048_576},
		// 18000 x 4000 = 72,000,000.
		{6000, 4000, 8, 3, 145_048_576},
		// 10000 x 5000 = 50,000,000.
		{5000, 5000, 16, 1, 101_048_576},
		// 201 bits per row pads to 26 bytes, x 1048576 rows = 27,262,976.
		// The unpadded formula gives 26,345,472 and a different ceiling, so this
		// case distinguishes the two.
		{201, 1 << 20, 1, 1, 55_574_528},
	} {
		got := imageDecodeCeiling(c.width, c.height, c.bitsPerComponent, c.comps)
		if got != c.want {
			t.Errorf("%dx%d bpc=%d comps=%d: got %d, want %d",
				c.width, c.height, c.bitsPerComponent, c.comps, got, c.want)
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

// A side too long to be a picture falls back to the extraction ceiling rather
// than the wider absolute cap: a dimension that cannot be trusted must not buy
// more room to inflate than an honest image gets.
func TestImageDecodeCeiling_UntrustworthyDimensionFallsBackTight(t *testing.T) {
	for _, c := range []struct {
		name          string
		width, height int
	}{
		{"both sides absurd", math.MaxInt, math.MaxInt},
		{"width absurd", math.MaxInt, 100},
		{"height absurd", 100, math.MaxInt},
		{"just past the dimension bound", (1 << 20) + 1, 1},
	} {
		if got := imageDecodeCeiling(c.width, c.height, 16, 4); got != maxImageBytes {
			t.Errorf("%s: got %d, want the maxImageBytes fallback %d", c.name, got, maxImageBytes)
		}
	}
}

// Row padding is what the real stored size uses, so a sub-byte image is measured
// by its padded rows rather than by a fraction of a byte per pixel. The geometry
// here is chosen so both formulas clear the maxImageBytes floor, which is what
// makes the two answers distinguishable at all - below the floor they collapse
// to the same value and the difference is invisible.
func TestImageDecodeCeiling_AccountsForRowPadding(t *testing.T) {
	const width, height = 201, 1 << 20
	const paddedDeclared = int64(26) * height // 201 bits -> 26 bytes per row
	const unpaddedDeclared = int64(width) * height / 8

	if paddedDeclared*2+imageDecodeHeadroom <= maxImageBytes ||
		unpaddedDeclared*2+imageDecodeHeadroom <= maxImageBytes {
		t.Fatal("fixture no longer clears the floor on both formulas; the case cannot discriminate")
	}
	got := imageDecodeCeiling(width, height, 1, 1)
	if want := paddedDeclared*2 + imageDecodeHeadroom; got != want {
		t.Errorf("got %d, want %d from the padded row size", got, want)
	}
	if got == unpaddedDeclared*2+imageDecodeHeadroom {
		t.Error("ceiling matches the unpadded formula, so row padding is not applied")
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
	// 4 components at 8 bits over 100M pixels declares 400 MB, and twice that
	// is past the cap, so the cap is what comes back.
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

// The resolved component count is reused when the DCT path already set it, so no
// second lookup happens for that stream. The value is deliberately 3 rather than
// 4: 4 is what the unresolvable fallback returns, so a reused count and a
// fallback would be indistinguishable and deleting the reuse would not fail.
func TestDeclaredComponents_ReusesResolvedCount(t *testing.T) {
	sd := &pdfcpu_types.StreamDict{Dict: pdfcpu_types.Dict{}, CSComponents: 3}
	if got := declaredComponents(nil, sd); got != 3 {
		t.Errorf("got %d, want the already-resolved 3", got)
	}
}

// An image with no /ColorSpace at all - an ImageMask, say - has no count to
// derive, so the estimate widens instead of collapsing to zero. Zero would pin
// the ceiling to its tightest value and refuse a large extraction.
func TestDeclaredComponents_MissingColorSpaceWidensTheEstimate(t *testing.T) {
	sd := &pdfcpu_types.StreamDict{Dict: pdfcpu_types.Dict{}}
	got := declaredComponents(nil, sd)
	if got <= 0 {
		t.Fatalf("got %d, want a positive fallback", got)
	}
	if imageDecodeCeiling(8192, 8192, 8, got) <= maxImageBytes {
		t.Errorf("fallback %d leaves a large image pinned to the floor", got)
	}
}

// pdfcpu's component lookup dereferences and asserts types unchecked, so a
// colour space it cannot resolve faults it, and safeCall re-panics a runtime
// error by design. This pins the absorption.
//
// Which shapes reach the lookup varies: a dangling reference makes the document
// fail to open, while a well-formed /DeviceN with an indirect colorant array
// opens and reaches it. The nil table below forces the fault directly, which is
// the cheapest way to exercise the fallback without a document.
func TestDeclaredComponents_UnresolvableColorSpaceDoesNotPanic(t *testing.T) {
	sd := &pdfcpu_types.StreamDict{
		Dict: pdfcpu_types.Dict{
			"ColorSpace": pdfcpu_types.NewNameArray("ICCBased"),
		},
	}
	// Pin that this fixture really does fault the underlying lookup. Without
	// this the assertion below passes whether the panic was absorbed or never
	// happened, since both answer with the same fallback.
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("fixture no longer faults pdfcpu's lookup; the absorption is untested")
			}
		}()
		_, _ = pdfcpu_render.ColorSpaceComponents(nil, sd)
	}()

	got := declaredComponents(nil, sd)
	if got <= 0 {
		t.Errorf("got %d, want the positive fallback after an absorbed failure", got)
	}
}

// Several malformed /ColorSpace shapes fault pdfcpu's component lookup, and
// declaredComponents answers with its fallback for each rather than letting the
// fault escape. The DCT pre-decode branch in GetImageData has its own absorbing
// wrapper around the same lookup; it is not covered here, and no document
// reaches it either, since pdfcpu refuses to open a file carrying these shapes.
func TestDeclaredComponents_MalformedColorSpaceShapesAllFallBack(t *testing.T) {
	for _, cs := range []pdfcpu_types.Object{
		pdfcpu_types.Array{},
		pdfcpu_types.NewNameArray("Indexed"),
		pdfcpu_types.Array{pdfcpu_types.Integer(5)},
	} {
		sd := &pdfcpu_types.StreamDict{Dict: pdfcpu_types.Dict{"ColorSpace": cs}}

		// Pin that the shape really does fault. Without this the assertion below
		// passes on the plain-error path too, which returns the same fallback and
		// would leave the absorption itself unexercised.
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%v: fixture no longer faults the lookup; absorption untested", cs)
				}
			}()
			_, _ = pdfcpu_render.ColorSpaceComponents(nil, sd)
		}()

		if got := declaredComponents(nil, sd); got <= 0 {
			t.Errorf("%v: got %d, want a positive fallback", cs, got)
		}
	}
}

// A stencil mask has no /ColorSpace, so the component count cannot be derived
// from one. Sizing it by the unresolved fallback measures a 1-bit mask as 32
// components of 8 bits and hands it a ceiling 256 times its declared samples,
// which is room a compressed mask could inflate into. The real geometry is one
// bit and one component.
func TestImageDecodeCeiling_StencilMaskIsSizedByItsRealGeometry(t *testing.T) {
	const side = 4096
	maskBytes := int64(side) * side / 8 // 1 bit per pixel

	asMask := imageDecodeCeiling(side, side, 1, 1)
	asUnresolved := imageDecodeCeiling(side, side, 8, maxComponents)

	if asMask >= asUnresolved {
		t.Fatalf("fixture cannot discriminate: mask ceiling %d, unresolved-fallback ceiling %d", asMask, asUnresolved)
	}
	if asMask < maskBytes {
		t.Errorf("ceiling %d is below the %d bytes a 1-bit mask declares", asMask, maskBytes)
	}
	if asUnresolved != maxImageDecodeBytes {
		t.Errorf("the fallback sizing should saturate the cap, got %d", asUnresolved)
	}
}
