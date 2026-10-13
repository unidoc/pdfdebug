package pdfcore

import (
	"bytes"
	"context"
	"image"
	"testing"
)

// largeRasterImagePDF builds a single-page document with one FlateDecode
// DeviceGray image XObject (object 4) declared side x side, so the source is far
// past maxThumbnailEdge and the preview and the full render diverge. The zlib
// body is built without ever holding side*side bytes at once.
func largeRasterImagePDF(t *testing.T, side int) []byte {
	t.Helper()
	raw := zlibZeros(t, side*side)
	dict := "/Type /XObject /Subtype /Image /Width " + itoa(side) +
		" /Height " + itoa(side) +
		" /ColorSpace /DeviceGray /BitsPerComponent 8 /Filter /FlateDecode"
	img := "4 0 obj\n<< " + dict + " /Length " + itoa(len(raw)) + " >>\nstream\n" +
		string(raw) + "\nendstream\nendobj\n\n"
	return assemblexref(
		"%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]"+
			" /Resources << /XObject << /Im0 4 0 R >> >> >>\nendobj\n\n",
		img,
	)
}

// imageDims decodes encoded image bytes and returns their pixel dimensions.
func imageDims(t *testing.T, encoded []byte) (width, height int) {
	t.Helper()
	cfg, _, err := image.DecodeConfig(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("failed to decode image bytes: %v", err)
	}
	return cfg.Width, cfg.Height
}

// The save path (GetImageBytes) returns the FULL-resolution render at the
// source geometry, while the preview path (GetImageData) returns a thumbnail
// bounded by maxThumbnailEdge. This pins that the saved bytes decode to the real
// dimensions and are strictly larger than the thumbnail, so the save writes the
// full image and not the downsampled preview.
func TestGetImageBytes_ReturnsFullResolutionNotThumbnail(t *testing.T) {
	const side = 8192
	ins, tabID := writeTempPDF(t, "large-raster.pdf", largeRasterImagePDF(t, side))

	nodeID := findImageNode(t, ins, tabID, "root", 0)
	if nodeID == "" {
		t.Fatal("no image node found in large-raster fixture")
	}

	full, _, err := ins.GetImageBytes(tabID, nodeID)
	if err != nil {
		t.Fatalf("GetImageBytes returned error: %v", err)
	}
	if len(full) == 0 {
		t.Fatal("GetImageBytes returned no bytes")
	}
	fw, fh := imageDims(t, full)
	if fw != side || fh != side {
		t.Errorf("saved image is not full resolution: decoded %dx%d, want the source %dx%d",
			fw, fh, side, side)
	}

	preview, err := ins.GetImageData(context.Background(), tabID, nodeID)
	if err != nil {
		t.Fatalf("GetImageData returned error: %v", err)
	}
	if preview.Error != "" {
		t.Fatalf("GetImageData reported error: %q", preview.Error)
	}
	if preview.ThumbWidth > maxThumbnailEdge || preview.ThumbHeight > maxThumbnailEdge {
		t.Fatalf("thumbnail exceeds the max edge %d: got %dx%d",
			maxThumbnailEdge, preview.ThumbWidth, preview.ThumbHeight)
	}
	if fw <= preview.ThumbWidth || fh <= preview.ThumbHeight {
		t.Errorf("saved bytes are not larger than the thumbnail: full %dx%d, thumbnail %dx%d",
			fw, fh, preview.ThumbWidth, preview.ThumbHeight)
	}
}
