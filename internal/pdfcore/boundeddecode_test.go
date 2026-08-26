// Co-located unit tests for the bounded stream-decode helper:
//
//	decodeBounded(sd *pdfcpu_types.StreamDict, limit int64) ([]byte, error)
//
// The helper caps how much a compressed stream may inflate to, so a small
// highly-compressible payload cannot allocate its way to an OOM before the
// extraction ceiling is applied. Fixtures are StreamDict values built in
// memory with compress/zlib; no PDF file is involved.
package pdfcore

import (
	"bytes"
	"compress/zlib"
	"encoding/ascii85"
	"errors"
	"runtime"
	"testing"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// zlibBytes returns payload compressed with the zlib wrapper pdfcpu's
// FlateDecode filter expects.
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

// zlibZeros returns the zlib encoding of n zero bytes without ever holding n
// bytes at once, so a multi-megabyte fixture costs a few KB to build.
func zlibZeros(t *testing.T, n int) []byte {
	t.Helper()
	const chunk = 64 * 1024
	block := make([]byte, chunk)
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	for written := 0; written < n; {
		size := min(chunk, n-written)
		if _, err := w.Write(block[:size]); err != nil {
			t.Fatalf("zlib write: %v", err)
		}
		written += size
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}
	return buf.Bytes()
}

// flateStream returns a StreamDict whose sole filter is FlateDecode over raw.
func flateStream(raw []byte) *pdfcpu_types.StreamDict {
	return &pdfcpu_types.StreamDict{
		Dict:           pdfcpu_types.Dict{},
		FilterPipeline: []pdfcpu_types.PDFFilter{{Name: "FlateDecode"}},
		Raw:            raw,
	}
}

// A stream that inflates past the limit is rejected, and the rejection is the
// extraction-ceiling sentinel rather than a decode failure.
func TestDecodeBounded_OverLimitReturnsUnsupported(t *testing.T) {
	const limit = int64(64 * 1024)
	sd := flateStream(zlibZeros(t, 8*1024*1024))

	out, err := decodeBounded(sd, limit)
	if !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("expected ErrUnsupportedPDF, got err=%v (out %d bytes)", err, len(out))
	}
	if out != nil {
		t.Errorf("expected no payload on a ceiling rejection, got %d bytes", len(out))
	}
}

// The rejection happens during inflation, not after it: an 8 MiB inflation
// stopped at a 64 KiB limit must not allocate the whole decompressed payload.
//
// TotalAlloc is cumulative and never decremented, so a GC landing mid-call
// cannot mask the allocation the way a live-heap reading would. It is
// process-wide, so this test runs serially and takes both readings with
// nothing but the guarded call between them.
func TestDecodeBounded_OverLimitDoesNotFullyInflate(t *testing.T) {
	const limit = int64(64 * 1024)
	const inflatedSize = 8 * 1024 * 1024
	const allocCeiling = uint64(4 * 1024 * 1024)

	sd := flateStream(zlibZeros(t, inflatedSize))

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	out, err := decodeBounded(sd, limit)
	runtime.ReadMemStats(&after)

	if !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("expected ErrUnsupportedPDF, got err=%v (out %d bytes)", err, len(out))
	}
	delta := after.TotalAlloc - before.TotalAlloc
	if delta >= allocCeiling {
		t.Errorf("allocated %d bytes rejecting a %d-byte inflation at a %d-byte limit; want under %d",
			delta, inflatedSize, limit, allocCeiling)
	}
}

// A compressible stream that stays inside the limit still yields its exact
// decoded bytes.
func TestDecodeBounded_InBoundsStreamReturnsDecodedBytes(t *testing.T) {
	const limit = int64(64 * 1024)
	payload := bytes.Repeat([]byte("compressible-"), 2000)
	sd := flateStream(zlibBytes(t, payload))

	out, err := decodeBounded(sd, limit)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Errorf("decoded bytes mismatch: got %d bytes, want %d", len(out), len(payload))
	}
}

// A stream far below the limit is the case a bounded decode most easily
// breaks: the underlying pdfcpu probe reports a short read as io.EOF with no
// payload, which must be read as "fully decoded, in bounds" and not as a
// failure.
func TestDecodeBounded_StreamFarBelowLimitReturnsDecodedBytes(t *testing.T) {
	const limit = int64(50 * 1024 * 1024)
	payload := []byte("a tiny attachment")
	sd := flateStream(zlibBytes(t, payload))

	out, err := decodeBounded(sd, limit)
	if err != nil {
		t.Fatalf("expected success for a stream far below the limit, got %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Errorf("decoded bytes mismatch: got %q, want %q", out, payload)
	}
}

// The full decode must also populate sd.Content, because the image path hands
// the same StreamDict to pdfcpu's renderer afterwards.
func TestDecodeBounded_InBoundsStreamPopulatesContent(t *testing.T) {
	const limit = int64(64 * 1024)
	payload := bytes.Repeat([]byte{0x41}, 4096)
	sd := flateStream(zlibBytes(t, payload))

	if _, err := decodeBounded(sd, limit); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !bytes.Equal(sd.Content, payload) {
		t.Errorf("sd.Content not populated with the decoded payload: got %d bytes, want %d",
			len(sd.Content), len(payload))
	}
}

// The raw payload is checked before any filter runs, because only the terminal
// filter of a pipeline is bounded.
func TestDecodeBounded_RawPayloadOverLimitRejected(t *testing.T) {
	const limit = int64(1024)
	sd := flateStream(bytes.Repeat([]byte{0x7a}, 4096))

	if _, err := decodeBounded(sd, limit); !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("expected ErrUnsupportedPDF for a raw payload over the limit, got %v", err)
	}
}

// An unfiltered stream carries its payload in Raw and must be answered with a
// plain length comparison, never a bounded probe.
func TestDecodeBounded_UnfilteredStreamReturnsRawBytes(t *testing.T) {
	const limit = int64(64 * 1024)
	payload := []byte("unfiltered attachment body")
	sd := &pdfcpu_types.StreamDict{Dict: pdfcpu_types.Dict{}, Raw: payload}

	out, err := decodeBounded(sd, limit)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Errorf("unfiltered payload mismatch: got %q, want %q", out, payload)
	}
}

// An empty-but-non-nil filter pipeline is the same shape as an unfiltered
// stream and must not reach the decoder, which would read from a nil reader.
func TestDecodeBounded_EmptyFilterPipelineReturnsRawBytes(t *testing.T) {
	const limit = int64(64 * 1024)
	payload := []byte("empty pipeline body")
	sd := &pdfcpu_types.StreamDict{
		Dict:           pdfcpu_types.Dict{},
		FilterPipeline: []pdfcpu_types.PDFFilter{},
		Raw:            payload,
	}

	out, err := decodeBounded(sd, limit)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Errorf("empty-pipeline payload mismatch: got %q, want %q", out, payload)
	}
}

// A stream the loader already decoded is answered from Content.
func TestDecodeBounded_AlreadyDecodedStreamReturnsContent(t *testing.T) {
	const limit = int64(64 * 1024)
	payload := []byte("pre-decoded content")
	sd := flateStream([]byte("ignored raw"))
	sd.Content = payload

	out, err := decodeBounded(sd, limit)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Errorf("pre-decoded payload mismatch: got %q, want %q", out, payload)
	}
}

// Pre-decoded content over the limit is still rejected.
func TestDecodeBounded_AlreadyDecodedStreamOverLimitRejected(t *testing.T) {
	const limit = int64(1024)
	sd := flateStream([]byte("ignored raw"))
	sd.Content = bytes.Repeat([]byte{0x42}, 4096)

	if _, err := decodeBounded(sd, limit); !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("expected ErrUnsupportedPDF for pre-decoded content over the limit, got %v", err)
	}
}

// A sole DCTDecode stream is never inflated by pdfcpu; it is handed back
// verbatim for the renderer. Probing that shape slices an undersized buffer,
// so the helper must answer it with a length comparison instead.
func TestDecodeBounded_SoleDCTStreamReturnsRawBytes(t *testing.T) {
	const limit = int64(64 * 1024)
	payload := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46}
	sd := &pdfcpu_types.StreamDict{
		Dict:           pdfcpu_types.Dict{},
		FilterPipeline: []pdfcpu_types.PDFFilter{{Name: "DCTDecode"}},
		Raw:            payload,
	}

	out, err := decodeBounded(sd, limit)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Errorf("DCT payload mismatch: got %v, want %v", out, payload)
	}
}

// A sole JPXDecode stream passes through the same way.
func TestDecodeBounded_SoleJPXStreamReturnsRawBytes(t *testing.T) {
	const limit = int64(64 * 1024)
	payload := []byte{0x00, 0x00, 0x00, 0x0c, 0x6a, 0x50, 0x20, 0x20}
	sd := &pdfcpu_types.StreamDict{
		Dict:           pdfcpu_types.Dict{},
		FilterPipeline: []pdfcpu_types.PDFFilter{{Name: "JPXDecode"}},
		Raw:            payload,
	}

	out, err := decodeBounded(sd, limit)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Errorf("JPX payload mismatch: got %v, want %v", out, payload)
	}
}

// predictorFlateStream returns a Flate stream carrying PNG-predictor rows
// (filter byte 0 = None) plus the matching /DecodeParms, and the bytes the
// predictor is expected to reconstruct.
func predictorFlateStream(t *testing.T, rows int) (*pdfcpu_types.StreamDict, []byte) {
	t.Helper()
	const columns = 4
	var encoded, expected bytes.Buffer
	for i := range rows {
		row := []byte{byte(i), 0x02, 0x03, 0x04}
		encoded.WriteByte(0)
		encoded.Write(row)
		expected.Write(row)
	}
	sd := &pdfcpu_types.StreamDict{
		Dict: pdfcpu_types.Dict{},
		FilterPipeline: []pdfcpu_types.PDFFilter{{
			Name: "FlateDecode",
			DecodeParms: pdfcpu_types.Dict{
				"Predictor":        pdfcpu_types.Integer(12),
				"Columns":          pdfcpu_types.Integer(columns),
				"Colors":           pdfcpu_types.Integer(1),
				"BitsPerComponent": pdfcpu_types.Integer(8),
			},
		}},
		Raw: zlibBytes(t, encoded.Bytes()),
	}
	return sd, expected.Bytes()
}

// Predictor Flate is outside what the helper can bound: pdfcpu's predictor
// path returns short output with a nil error and then slices it unchecked. The
// helper must keep the unbounded behaviour for that shape rather than probing
// it, so an ordinary in-bounds predictor stream decodes correctly and does not
// panic.
func TestDecodeBounded_PredictorFlateStreamDecodesWithoutPanic(t *testing.T) {
	const limit = int64(64 * 1024)
	sd, expected := predictorFlateStream(t, 10)

	out, err := decodeBounded(sd, limit)
	if err != nil {
		t.Fatalf("expected success for an in-bounds predictor stream, got %v", err)
	}
	if !bytes.Equal(out, expected) {
		t.Errorf("predictor payload mismatch: got %v, want %v", out, expected)
	}
}

// A predictor stream over the limit keeps the existing decode-then-measure
// rejection, which is still a ceiling rejection.
func TestDecodeBounded_PredictorFlateStreamOverLimitRejected(t *testing.T) {
	const limit = int64(1024)
	sd, _ := predictorFlateStream(t, 4096)

	if _, err := decodeBounded(sd, limit); !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("expected ErrUnsupportedPDF for an over-limit predictor stream, got %v", err)
	}
}

// A multi-filter pipeline is bounded only at its terminal filter; the ordinary
// ASCII85-then-Flate arrangement must still round-trip.
func TestDecodeBounded_ASCII85ThenFlatePipelineReturnsDecodedBytes(t *testing.T) {
	const limit = int64(64 * 1024)
	payload := bytes.Repeat([]byte("pipeline-"), 512)
	flated := zlibBytes(t, payload)

	encoded := make([]byte, ascii85.MaxEncodedLen(len(flated)))
	n := ascii85.Encode(encoded, flated)
	raw := append(encoded[:n], '~', '>')

	sd := &pdfcpu_types.StreamDict{
		Dict: pdfcpu_types.Dict{},
		FilterPipeline: []pdfcpu_types.PDFFilter{
			{Name: "ASCII85Decode"},
			{Name: "FlateDecode"},
		},
		Raw: raw,
	}

	out, err := decodeBounded(sd, limit)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Errorf("pipeline payload mismatch: got %d bytes, want %d", len(out), len(payload))
	}
}
