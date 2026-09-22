// Fixture builders and the CLI harness for the sample-interpretation cases in
// this suite.
//
// Black-box: the pdfdebug CLI binary is built once per package and run as a
// subprocess, so `dump image` is exercised the way a script would. Failures
// surface at RUNTIME, not at compile time, so the main unidoc-pdf-debugger
// module keeps building green while the feature is missing.
//
// Image dictionaries and JPEG marker chains are assembled from raw bytes into
// t.TempDir at run time rather than committed to testdata/: only these cases
// read these shapes, and the committed-fixture generator skips any file that
// already exists, so a fixture that changes shape would have to be deleted by
// hand first.
//
// Run: cd tests/image-extraction && go test -v -count=1 ./...
package image_extraction_test

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
)

// --- CLI harness ------------------------------------------------------------

var (
	cliBuildOnce sync.Once
	cliBinPath   string
	cliBuildErr  string
)

// buildCLI compiles the CLI binary once per test package and returns its path.
func buildCLI(t *testing.T) string {
	t.Helper()
	// Resolve the root OUTSIDE the Once: projectRoot fails via t.Fatalf, which is
	// runtime.Goexit, and sync.Once marks itself done in a defer - so failing in
	// there would consume the Once and leave every later test to fail at
	// exec.Command("") with an error that names nothing useful.
	root := projectRoot(t)
	cliBuildOnce.Do(func() {
		binName := "pdfdebug"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		tmpDir, err := os.MkdirTemp("", "pdfdebug-cli-")
		if err != nil {
			cliBuildErr = "failed to create temp dir: " + err.Error()
			return
		}
		binPath := filepath.Join(tmpDir, binName)
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/cli/")
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			cliBuildErr = "failed to build CLI binary: " + err.Error() + "\n" + string(output)
			return
		}
		cliBinPath = binPath
	})
	if cliBuildErr != "" {
		t.Fatalf("%s", cliBuildErr)
	}
	return cliBinPath
}

// runCLI executes the CLI binary with args and returns stdout, stderr and the
// exit code.
func runCLI(t *testing.T, args ...string) (stdout, stderr []byte, exitCode int) {
	t.Helper()
	cmd := exec.Command(buildCLI(t), args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run CLI: %v", err)
		}
	}
	return outBuf.Bytes(), errBuf.Bytes(), exitCode
}

// writeTempPDF writes content to a temp file and returns its path.
func writeTempPDF(t *testing.T, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write temp pdf: %v", err)
	}
	return path
}

// --- payload shape ----------------------------------------------------------

// imageJSON mirrors the `dump image --json` keys these cases assert on.
// Decoding into a typed struct keeps the assertions immune to field-order and
// sibling-key changes. The pointer fields carry the three-state distinctions:
// nil is "the key is absent from the PDF", a pointer to the zero value is
// "present with nothing to report".
type imageJSON struct {
	ObjectRef            string    `json:"objectRef"`
	ColorSpace           string    `json:"colorSpace"`
	Filter               string    `json:"filter"`
	Decode               []float64 `json:"decode"`
	ImageMask            bool      `json:"imageMask"`
	SMask                *string   `json:"smask"`
	AdobeMarker          string    `json:"adobeMarker"`
	AdobeTransform       *int      `json:"adobeTransform"`
	SampleInterpretation string    `json:"sampleInterpretation"`
	StoredBytes          int64     `json:"storedBytes"`
	DecodedBytes         int64     `json:"decodedBytes"`
	Warning              string    `json:"warning"`
	Error                string    `json:"error"`
}

// sampleInterpretationKeys are the keys the JSON contract carries for every
// image, present or absent. They are listed once here because two cases assert
// the whole set: one that every key is emitted, one that each is an explicit
// null rather than a missing key.
var sampleInterpretationKeys = []string{
	"decode", "imageMask", "smask", "adobeMarker", "adobeTransform", "sampleInterpretation",
}

// dumpImageJSON runs `dump image --ref "4 0 R" --json` over a fixture and
// returns the payload both typed and as a raw key map, so a case can assert on
// a parsed value or on the literal null a missing value emits.
func dumpImageJSON(t *testing.T, name string, pdf []byte, extraArgs ...string) (imageJSON, map[string]json.RawMessage) {
	t.Helper()
	path := writeTempPDF(t, name, pdf)
	args := append([]string{"dump", "image", "--ref", "4 0 R", "--json"}, extraArgs...)
	stdout, stderr, ec := runCLI(t, append(args, path)...)
	if ec != 0 {
		t.Fatalf("expected exit 0 (the image view reports per-image failures in its payload), got %d\nstderr: %s", ec, string(stderr))
	}
	var typed imageJSON
	if err := json.Unmarshal(stdout, &typed); err != nil {
		t.Fatalf("parse image JSON: %v\nraw: %s", err, string(stdout))
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(stdout, &raw); err != nil {
		t.Fatalf("parse image JSON as a key map: %v\nraw: %s", err, string(stdout))
	}
	return typed, raw
}

// unmarshalImage parses a `dump image --json` payload into the typed shape.
func unmarshalImage(data []byte, out *imageJSON) error {
	return json.Unmarshal(data, out)
}

// dumpImagePlain runs the plain-text `dump image` over a fixture at the given
// object reference and returns stdout.
func dumpImagePlain(t *testing.T, name string, pdf []byte, ref string) string {
	t.Helper()
	path := writeTempPDF(t, name, pdf)
	stdout, stderr, ec := runCLI(t, "dump", "image", "--ref", ref, path)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d\nstderr: %s", ec, string(stderr))
	}
	return string(stdout)
}

// --- PDF assembly -----------------------------------------------------------

func pad10(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 10 {
		s = "0" + s
	}
	return s
}

// assemblePDF stitches a header, object bodies (object i+1), an xref table and
// a trailer with /Root. Objects are []byte so binary stream payloads survive.
func assemblePDF(objs [][]byte, rootNum int) []byte {
	body := []byte("%PDF-1.7\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = len(body)
		body = append(body, o...)
	}
	xrefOff := len(body)
	size := len(objs) + 1
	xref := "xref\n0 " + strconv.Itoa(size) + "\n0000000000 65535 f \n"
	for _, off := range offsets {
		xref += pad10(off) + " 00000 n \n"
	}
	trailer := "trailer\n<< /Size " + strconv.Itoa(size) + " /Root " + strconv.Itoa(rootNum) + " 0 R >>\n" +
		"startxref\n" + strconv.Itoa(xrefOff) + "\n%%EOF\n"
	return append(body, append([]byte(xref), []byte(trailer)...)...)
}

// streamObj returns object num with the given dict entries and a raw stream
// body. /Length is appended automatically.
func streamObj(num int, dictEntries string, raw []byte) []byte {
	head := strconv.Itoa(num) + " 0 obj\n<< " + dictEntries +
		" /Length " + strconv.Itoa(len(raw)) + " >>\nstream\n"
	out := append([]byte(head), raw...)
	return append(out, []byte("\nendstream\nendobj\n")...)
}

// imagePDF builds a single-page document whose object 4 is an image XObject
// with the given dictionary entries and raw stream bytes. Extra objects follow
// it, so a fixture can add an /SMask target or an indirect /Decode array as
// object 5 and up.
func imagePDF(dictEntries string, raw []byte, extra ...[]byte) []byte {
	objs := [][]byte{
		[]byte("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"),
		[]byte("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"),
		[]byte("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]" +
			" /Resources << /XObject << /Im0 4 0 R >> >> >>\nendobj\n"),
		streamObj(4, dictEntries, raw),
	}
	return assemblePDF(append(objs, extra...), 1)
}

// rgbImagePDF builds a three-component DCTDecode image around the given JPEG
// bytes, with the given extra dictionary entries spliced in.
func rgbImagePDF(jpegBytes []byte, extraEntries string, extra ...[]byte) []byte {
	dict := "/Type /XObject /Subtype /Image /Width 8 /Height 8" +
		" /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode " + extraEntries
	return imagePDF(dict, jpegBytes, extra...)
}

// cmykImagePDF builds a four-component DCTDecode image around the given bytes.
// Nothing here decodes: image/jpeg refuses a four-component stream outright
// unless it carries an Adobe APP14, and a hand-rolled four-component entropy
// stream is not worth the cost. That is the point of these fixtures - the
// four-component rows are exactly the ones whose reported fields have to
// survive a failed decode.
func cmykImagePDF(raw []byte, extraEntries string, extra ...[]byte) []byte {
	dict := "/Type /XObject /Subtype /Image /Width 8 /Height 8" +
		" /ColorSpace /DeviceCMYK /BitsPerComponent 8 /Filter /DCTDecode " + extraEntries
	return imagePDF(dict, raw, extra...)
}

// deviceNImagePDF builds a DCTDecode image whose /DeviceN colour space names
// its colorants through an INDIRECT reference. Every object exists, so the
// document opens; pdfcpu's component lookup then asserts that entry is an Array
// without dereferencing it and faults, so the component count for this image
// cannot be resolved and the read carries a per-image error.
// Objects 5 and 6 are the colorant array and the tint transform, so extra
// objects a case adds start at 7.
func deviceNImagePDF(raw []byte, extraEntries string, extra ...[]byte) []byte {
	dict := "/Type /XObject /Subtype /Image /Width 8 /Height 8 /BitsPerComponent 8" +
		" /ColorSpace [/DeviceN 5 0 R /DeviceCMYK 6 0 R] /Filter /DCTDecode " + extraEntries
	objs := [][]byte{
		[]byte("5 0 obj\n[/Ink1 /Ink2 /Ink3 /Ink4]\nendobj\n"),
		[]byte("6 0 obj\n<< /FunctionType 2 /Domain [0 1] /C0 [0 0 0 0]" +
			" /C1 [1 1 1 1] /N 1 >>\nendobj\n"),
	}
	return imagePDF(dict, raw, append(objs, extra...)...)
}

// flateImagePDF builds a single-component FlateDecode image: a stream with no
// JPEG in it at all.
func flateImagePDF(t *testing.T, extraEntries string, extra ...[]byte) []byte {
	dict := "/Type /XObject /Subtype /Image /Width 8 /Height 8" +
		" /ColorSpace /DeviceGray /BitsPerComponent 8 /Filter /FlateDecode " + extraEntries
	return imagePDF(dict, zlibBytes(t, grayPixels()), extra...)
}

// stencilMaskPDF builds a stencil mask: /ImageMask true, no /ColorSpace, one
// 1-bit sample per pixel, with the given extra dictionary entries.
//
// The payload is deliberately not valid zlib. A mask that decodes reaches a
// renderer that faults on it, which replaces the whole payload with an internal
// error and hides the fields under test; a stream that fails to decode stops
// short of that, which is also the ordering these fields have to survive.
func stencilMaskPDF(extraEntries string) []byte {
	dict := "/Type /XObject /Subtype /Image /Width 8 /Height 8" +
		" /ImageMask true /Filter /FlateDecode " + extraEntries
	return imagePDF(dict, []byte("not a flate stream"))
}

// zlibBytes returns payload compressed with the zlib wrapper /FlateDecode
// expects.
func zlibBytes(t *testing.T, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("zlib write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}
	return buf.Bytes()
}

// grayPixels returns the 64 bytes of an 8x8 DeviceGray bitmap.
func grayPixels() []byte {
	px := make([]byte, 64)
	for i := range px {
		px[i] = byte(i * 4)
	}
	return px
}

// --- JPEG marker chains -----------------------------------------------------

// rgbJPEG encodes an 8x8 RGB image. The encoder writes SOI, a JFIF APP0, the
// tables, SOF0, SOS and the entropy data, and no APP14 - splicing one in after
// the APP0 leaves the result decodable whatever transform byte is used, because
// the JFIF marker already decided the colour transform.
func rgbJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.Set(x, y, color.RGBA{R: uint8(x * 30), G: uint8(y * 30), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode jpeg fixture: %v", err)
	}
	return buf.Bytes()
}

// adobeAPP14 returns the 16-byte Adobe APP14 segment carrying the given
// transform: the marker, a length of 14 covering itself plus the 12-byte
// record, the five-byte "Adobe" identifier, version 100, two flag words and the
// transform byte.
func adobeAPP14(transform byte) []byte {
	return []byte{
		0xFF, 0xEE, 0x00, 0x0E,
		'A', 'd', 'o', 'b', 'e',
		0x00, 0x64,
		0x00, 0x00,
		0x00, 0x00,
		transform,
	}
}

// foreignAPP14 returns an APP14 segment of the same length whose identifier is
// not "Adobe". A reader that branches on the marker and not the identifier
// reports its last payload byte as a transform.
func foreignAPP14() []byte {
	return []byte{
		0xFF, 0xEE, 0x00, 0x0E,
		'O', 't', 'h', 'e', 'r',
		0x00, 0x64,
		0x00, 0x00,
		0x00, 0x00,
		0x07,
	}
}

// shortAPP14 returns an APP14 whose payload is four bytes: too short to hold
// the Adobe record, so it is skipped rather than read past.
func shortAPP14() []byte {
	return []byte{0xFF, 0xEE, 0x00, 0x06, 'A', 'd', 'o', 'b'}
}

// spliceAfterAPP0 inserts a segment directly after the JFIF APP0 of an encoded
// JPEG, which is where a writer would put its own application segment.
func spliceAfterAPP0(t *testing.T, src, seg []byte) []byte {
	t.Helper()
	at := 2
	if len(src) >= 6 && src[0] == 0xFF && src[1] == 0xD8 && src[2] == 0xFF && src[3] == 0xE0 {
		at = 4 + int(binary.BigEndian.Uint16(src[4:6]))
	}
	if at > len(src) {
		t.Fatalf("APP0 length runs past the encoded JPEG (%d > %d)", at, len(src))
	}
	out := make([]byte, 0, len(src)+len(seg))
	out = append(out, src[:at]...)
	out = append(out, seg...)
	return append(out, src[at:]...)
}

// markerChain returns a chain of SOI, the given segments, SOS and a few bytes
// of entropy data. Nothing after SOS is a marker, so a walk that reads past it
// is reading compressed samples.
func markerChain(segments ...[]byte) []byte {
	out := []byte{0xFF, 0xD8}
	for _, s := range segments {
		out = append(out, s...)
	}
	out = append(out, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3F, 0x00)
	out = append(out, 0x12, 0x34, 0x56, 0x78)
	return append(out, 0xFF, 0xD9)
}

// fillBytes returns a run of 0xFF padding, which is legal between segments and
// is not a marker.
func fillBytes(n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = 0xFF
	}
	return out
}

// restartMarkers returns the standalone markers D0 through D7, which carry no
// length field. A walk that reads two length bytes after one of them consumes
// whatever follows, so a segment placed after them disappears.
func restartMarkers() []byte {
	out := make([]byte, 0, 16)
	for m := byte(0xD0); m <= 0xD7; m++ {
		out = append(out, 0xFF, m)
	}
	return out
}

// truncatedChain declares a segment length that runs past the end of the
// buffer.
func truncatedChain() []byte {
	return []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x40, 0x01, 0x02, 0x03}
}

// chainWithoutSOS walks off the end of the buffer without ever reaching SOS.
func chainWithoutSOS() []byte {
	out := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}
	return append(out, make([]byte, 14)...)
}

// undersizedLengthChain declares a segment length below the two bytes the
// length field itself occupies, which cannot be advanced past.
func undersizedLengthChain() []byte {
	return []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x01, 0x41, 0x42, 0xFF, 0xDA, 0x00, 0x02}
}

// --- assertion helpers ------------------------------------------------------

// sameFloats reports whether two decode arrays match.
func sameFloats(got, want []float64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// isJSONNull reports whether the key is present in the payload and carries a
// literal null.
func isJSONNull(raw map[string]json.RawMessage, key string) bool {
	v, ok := raw[key]
	return ok && string(bytes.TrimSpace(v)) == "null"
}
