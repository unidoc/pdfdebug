package pdfcore

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// makeTestPNG builds a w x h PNG filled with an opaque color. When opaque is
// false the entire first column is fully transparent, so the transparency
// survives downsampling and the scaled result is not opaque.
func makeTestPNG(t *testing.T, w, h int, opaque bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.NRGBA{R: 100, G: 150, B: 200, A: 255})
		}
	}
	if !opaque {
		for y := range h {
			img.Set(0, y, color.NRGBA{})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	return buf.Bytes()
}

func TestMakeThumbnail_OpaqueDownsampleEncodesJPEG(t *testing.T) {
	src := makeTestPNG(t, maxThumbnailEdge+500, 8, true)
	b64, mime, tw, th, err := makeThumbnail(src, "png")
	if err != nil {
		t.Fatalf("makeThumbnail: %v", err)
	}
	if mime != "image/jpeg" {
		t.Errorf("mime = %q, want image/jpeg for an opaque downsampled source", mime)
	}
	if b64 == "" || tw > maxThumbnailEdge || th > maxThumbnailEdge {
		t.Errorf("thumb %dx%d b64empty=%v, want bounded non-empty", tw, th, b64 == "")
	}
	if _, err := base64.StdEncoding.DecodeString(b64); err != nil {
		t.Errorf("preview is not valid base64: %v", err)
	}
}

func TestMakeThumbnail_TransparentDownsampleEncodesPNG(t *testing.T) {
	src := makeTestPNG(t, maxThumbnailEdge+500, 8, false)
	_, mime, _, _, err := makeThumbnail(src, "png")
	if err != nil {
		t.Fatalf("makeThumbnail: %v", err)
	}
	if mime != "image/png" {
		t.Errorf("mime = %q, want image/png for a source with transparency", mime)
	}
}

func TestMakeThumbnail_SmallShipsOriginal(t *testing.T) {
	src := makeTestPNG(t, 32, 32, true)
	b64, mime, tw, th, err := makeThumbnail(src, "png")
	if err != nil {
		t.Fatalf("makeThumbnail: %v", err)
	}
	if mime != "image/png" || tw != 32 || th != 32 {
		t.Errorf("small image: mime=%q %dx%d, want image/png 32x32 unchanged", mime, tw, th)
	}
	if b64 != base64.StdEncoding.EncodeToString(src) {
		t.Error("small image should ship its original bytes unchanged")
	}
}
