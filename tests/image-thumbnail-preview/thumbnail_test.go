package image_thumbnail_preview_test

import (
	"strings"
	"testing"
)

// A large raster is shipped for display as a downsampled preview while every
// metadata field keeps reporting the real image. The declared 8192x8192 is far
// above any plausible fixed max-edge, so an honest thumbnail is strictly smaller
// than the source in both the reported ThumbWidth/ThumbHeight and the pixels of
// the base64 payload; Width/Height still describe the PDF's image.
func TestImagePreview_LargeRasterDownsampledWhileMetadataReportsRealGeometry(t *testing.T) {
	bin := buildCLI(t)
	const side = 8192
	pdf := writeTempPDF(t, "large-raster.pdf",
		flateImagePDFSized(zlibZeros(t, side*side), side, side))

	stdout, stderr, ec := runCLIBytes(t, bin, "dump", "image", "--ref", "4 0 R", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, string(stderr))
	}
	img := decodeImageJSON(t, stdout)
	if img.Error != "" {
		t.Fatalf("expected a large honest raster to render, got error %q", img.Error)
	}
	if img.Base64 == "" {
		t.Fatalf("expected a rendered preview payload")
	}

	// Metadata reports the real image.
	if img.Width != side || img.Height != side {
		t.Errorf("metadata must report the real geometry: got %dx%d, want %dx%d",
			img.Width, img.Height, side, side)
	}

	// Reduced-preview dimensions are reported and are smaller than the source.
	if img.ThumbWidth <= 0 || img.ThumbHeight <= 0 {
		t.Errorf("expected reported thumbnail dimensions > 0, got %dx%d", img.ThumbWidth, img.ThumbHeight)
	}
	if img.ThumbWidth >= side || img.ThumbHeight >= side {
		t.Errorf("expected the thumbnail to be smaller than the %dx%d source, got %dx%d",
			side, side, img.ThumbWidth, img.ThumbHeight)
	}

	// The pixels actually shipped are reduced, not the full-resolution render.
	dw, dh := decodedPreviewDims(t, img.Base64)
	if dw >= side || dh >= side {
		t.Errorf("preview payload still carries full-resolution pixels: decoded %dx%d, source %dx%d",
			dw, dh, side, side)
	}
	if dw != img.ThumbWidth || dh != img.ThumbHeight {
		t.Errorf("reported thumbnail dimensions must match the payload: reported %dx%d, decoded %dx%d",
			img.ThumbWidth, img.ThumbHeight, dw, dh)
	}
}

// An image already smaller than the max edge is shipped unchanged: the reported
// thumbnail dimensions equal the real geometry and the payload carries the
// original pixels.
func TestImagePreview_SmallImagePassesThroughUnchanged(t *testing.T) {
	bin := buildCLI(t)
	px := make([]byte, 64) // 8x8 DeviceGray, one byte per pixel
	for i := range px {
		px[i] = byte(i * 4)
	}
	pdf := writeTempPDF(t, "small-image.pdf", flateImagePDFSized(zlibBytes(t, px), 8, 8))

	stdout, stderr, ec := runCLIBytes(t, bin, "dump", "image", "--ref", "4 0 R", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, string(stderr))
	}
	img := decodeImageJSON(t, stdout)
	if img.Error != "" {
		t.Fatalf("expected a small image to render, got error %q", img.Error)
	}
	if img.ThumbWidth != 8 || img.ThumbHeight != 8 {
		t.Errorf("a below-max-edge image must pass through unchanged: got thumbnail %dx%d, want 8x8",
			img.ThumbWidth, img.ThumbHeight)
	}
	if dw, dh := decodedPreviewDims(t, img.Base64); dw != 8 || dh != 8 {
		t.Errorf("preview payload must keep the original 8x8 pixels, got %dx%d", dw, dh)
	}
}

// The outcome discriminator separates the three cases the frontend must branch
// on without parsing the error text: an ordinary thumbnail success, the
// lying-stream ceiling refusal (declared small, actual enormous), and a generic
// per-image failure. Each case must carry a non-empty, mutually distinct kind.
func TestImagePreview_KindDiscriminatesThreeOutcomes(t *testing.T) {
	bin := buildCLI(t)

	// Success: a large but honest raster renders to a thumbnail.
	successPDF := writeTempPDF(t, "ok.pdf", flateImagePDFSized(zlibZeros(t, 8192*8192), 8192, 8192))
	success := runImageJSON(t, bin, successPDF)
	if success.Error != "" || success.Base64 == "" {
		t.Fatalf("success fixture did not render: error %q, base64 len %d", success.Error, len(success.Base64))
	}

	// Ceiling refusal: 8x8 declared, 60 MB of samples -- refused at the
	// geometry ceiling, no payload.
	lyingPDF := writeTempPDF(t, "lying.pdf", flateImagePDFSized(zlibZeros(t, lyingStreamPayloadSize), 8, 8))
	lying := runImageJSON(t, bin, lyingPDF)
	if !strings.Contains(lying.Error, "too large") || lying.Base64 != "" {
		t.Fatalf("lying-stream fixture is not on the ceiling path: error %q, base64 len %d",
			lying.Error, len(lying.Base64))
	}

	// Generic failure: an indirect /DeviceN colorant array faults the component
	// lookup -- a per-image error that is neither success nor the ceiling refusal.
	genericPDF := writeTempPDF(t, "generic.pdf", deviceNImagePDF())
	generic := runImageJSON(t, bin, genericPDF)
	if generic.Error == "" || generic.Base64 != "" {
		t.Fatalf("generic-error fixture did not fail as expected: error %q, base64 len %d",
			generic.Error, len(generic.Base64))
	}

	for name, kind := range map[string]string{"success": success.Kind, "ceiling": lying.Kind, "generic": generic.Kind} {
		if kind == "" {
			t.Errorf("%s outcome carries no discriminator kind", name)
		}
	}
	if success.Kind == lying.Kind || success.Kind == generic.Kind || lying.Kind == generic.Kind {
		t.Errorf("the three outcomes must carry distinct kinds: success=%q ceiling=%q generic=%q",
			success.Kind, lying.Kind, generic.Kind)
	}
}

func runImageJSON(t *testing.T, bin, pdf string) imageJSON {
	t.Helper()
	stdout, stderr, ec := runCLIBytes(t, bin, "dump", "image", "--ref", "4 0 R", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, string(stderr))
	}
	return decodeImageJSON(t, stdout)
}
