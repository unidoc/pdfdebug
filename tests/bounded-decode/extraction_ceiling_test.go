package bounded_decode_test

import (
	"bytes"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Embedded-file extraction path.
// ---------------------------------------------------------------------------

// A FlateDecode attachment that stays inside the extraction ceiling round-trips
// to its exact decompressed bytes. This is the case a size-bounded decode most
// easily breaks, because the bounded probe reports an in-bounds stream as a
// short read with no payload.
func TestEmbeddedExtraction_InBoundsFlateAttachmentReturnsDecodedBytes(t *testing.T) {
	bin := buildCLI(t)
	payload := bytes.Repeat([]byte("attachment-payload-"), 512)
	pdf := writeTempPDF(t, "attachment.pdf", flateEmbeddedPDF(zlibBytes(t, payload), len(payload)))

	stdout, stderr, ec := runCLIBytes(t, bin, "dump", "embedded", "--ref", "4 0 R", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, string(stderr))
	}
	if !bytes.Equal(stdout, payload) {
		t.Errorf("extracted bytes mismatch: got %d bytes, want %d", len(stdout), len(payload))
	}
}

// A tiny attachment, far below the ceiling, still extracts.
func TestEmbeddedExtraction_TinyFlateAttachmentReturnsDecodedBytes(t *testing.T) {
	bin := buildCLI(t)
	payload := []byte("small")
	pdf := writeTempPDF(t, "small.pdf", flateEmbeddedPDF(zlibBytes(t, payload), len(payload)))

	stdout, stderr, ec := runCLIBytes(t, bin, "dump", "embedded", "--ref", "4 0 R", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, string(stderr))
	}
	if !bytes.Equal(stdout, payload) {
		t.Errorf("extracted bytes mismatch: got %q, want %q", string(stdout), string(payload))
	}
}

// A few dozen KB of compressed zeros inflating past the ceiling is rejected and
// no payload reaches stdout.
func TestEmbeddedExtraction_OverCeilingFlateAttachmentRejected(t *testing.T) {
	bin := buildCLI(t)
	// The declared /Params /Size understates the payload, so only the decode
	// path can be the thing that rejects this.
	pdf := writeTempPDF(t, "bomb.pdf", flateEmbeddedPDF(zlibZeros(t, overCeilingSize), 1024))

	stdout, stderr, ec := runCLIBytes(t, bin, "dump", "embedded", "--ref", "4 0 R", pdf)
	if ec == 0 {
		t.Fatalf("expected a non-zero exit for a stream inflating past the ceiling, got 0 (%d bytes on stdout)", len(stdout))
	}
	if len(stdout) != 0 {
		t.Errorf("expected no payload on stdout, got %d bytes", len(stdout))
	}
	if !bytes.Contains(stderr, []byte("extraction ceiling")) {
		t.Errorf("expected the ceiling diagnostic on stderr, got %q", string(stderr))
	}
}

// ---------------------------------------------------------------------------
// Image extraction path. Both fixtures share the same 8x8 DeviceGray image
// dict; only the size of the compressed payload differs.
// ---------------------------------------------------------------------------

// An in-bounds FlateDecode image still decodes and renders.
func TestImageExtraction_InBoundsFlateImageRenders(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "image.pdf", flateImagePDF(zlibBytes(t, imagePixels())))

	stdout, stderr, ec := runCLIBytes(t, bin, "dump", "image", "--ref", "4 0 R", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, string(stderr))
	}
	img := decodeImageJSON(t, stdout)
	if img.Error != "" {
		t.Fatalf("expected no error for an in-bounds image, got %q", img.Error)
	}
	if img.Base64 == "" {
		t.Errorf("expected a rendered payload for an in-bounds image")
	}
}

// The same image dict with a payload that inflates past the ceiling is
// rejected: the image view reports the failure in its error field and emits no
// rendered payload.
func TestImageExtraction_OverCeilingFlateImageRejected(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "image-bomb.pdf", flateImagePDF(zlibZeros(t, overCeilingSize)))

	stdout, stderr, ec := runCLIBytes(t, bin, "dump", "image", "--ref", "4 0 R", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0 (the image view reports extraction failures in its payload), got %d (stderr: %s)", ec, string(stderr))
	}
	img := decodeImageJSON(t, stdout)
	if !strings.Contains(img.Error, "too large") {
		t.Fatalf("expected the ceiling error for an image stream inflating past the ceiling, got %q (base64 %d chars)", img.Error, len(img.Base64))
	}
	if img.Base64 != "" {
		t.Errorf("expected no rendered payload for a rejected image, got %d chars of base64", len(img.Base64))
	}
}
