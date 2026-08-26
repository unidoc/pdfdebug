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
	"compress/lzw"
	"compress/zlib"
	"encoding/ascii85"
	"errors"
	"runtime"
	"testing"
	"time"

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

// The rejection happens during inflation, not after it: a 64 MiB inflation
// stopped at a 64 KiB limit must not allocate the whole decompressed payload.
//
// TotalAlloc is cumulative and never decremented, so a GC landing mid-call
// cannot mask the allocation the way a live-heap reading would. It is
// process-wide, so this test runs serially and takes both readings with
// nothing but the guarded call between them.
func TestDecodeBounded_OverLimitDoesNotFullyInflate(t *testing.T) {
	// The limit has to clear the COMPRESSED size, or the raw pre-guard refuses
	// the stream before any decode runs and this measures that guard instead of
	// the probe. 64 MiB of zeros deflates to ~130 KB, so 256 KiB leaves the
	// pre-guard satisfied while still being 256x smaller than the inflation.
	const limit = int64(256 * 1024)
	const inflatedSize = 64 * 1024 * 1024
	const allocCeiling = uint64(4 * 1024 * 1024)

	sd := flateStream(zlibZeros(t, inflatedSize))

	// Pin that precondition, so raising the fixture cannot silently move the
	// rejection back onto the pre-guard.
	if int64(len(sd.Raw)) > limit {
		t.Fatalf("fixture defeats the test: %d compressed bytes exceed the %d limit, so the raw pre-guard rejects before the probe",
			len(sd.Raw), limit)
	}

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

// The encoded payload is checked before any filter runs, so an oversized input
// is refused without handing it to the decoder at all.
func TestDecodeBounded_RawPayloadOverLimitRejected(t *testing.T) {
	const limit = int64(1024)
	sd := flateStream(bytes.Repeat([]byte{0x7a}, 4096))

	if _, err := decodeBounded(sd, limit); !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("expected ErrUnsupportedPDF for a raw payload over the limit, got %v", err)
	}
}

// An already-decoded payload is answered from Content even when the encoded
// bytes it came from are larger than the limit, so an expanding encoding does
// not reject a payload that fits.
func TestDecodeBounded_PreDecodedContentBeatsOversizedRaw(t *testing.T) {
	const limit = int64(1024)
	payload := bytes.Repeat([]byte{'x'}, 512)
	sd := &pdfcpu_types.StreamDict{
		Dict:    pdfcpu_types.Dict{},
		Raw:     bytes.Repeat([]byte{'0'}, 4096),
		Content: payload,
	}

	out, err := decodeBounded(sd, limit)
	if err != nil {
		t.Fatalf("expected the pre-decoded payload, got %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(out), len(payload))
	}
}

// pdfcpu tolerates a truncated zlib stream, which makes its capped decode
// return short output with a nil error and trip the unchecked tail. The helper
// falls back to the unbounded decode rather than crashing or rejecting.
func TestDecodeBounded_TruncatedFlateStreamStillDecodes(t *testing.T) {
	const limit = int64(64 * 1024)
	payload := bytes.Repeat([]byte{'a'}, 4096)
	raw := zlibBytes(t, payload)
	sd := flateStream(raw[:len(raw)-4])

	// Pin the branch this covers: the probe cannot answer for this stream.
	if _, probeErr := cappedDecode(flateStream(raw[:len(raw)-4]), limit+1); !errors.Is(probeErr, errProbeUnusable) {
		t.Fatalf("expected the probe to report itself unusable, got %v", probeErr)
	}

	out, err := decodeBounded(sd, limit)
	if err != nil {
		t.Fatalf("expected a truncated in-bounds stream to decode, got %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(out), len(payload))
	}
}

// Truncation does not buy a bomb a way past the ceiling: the capped decode
// still fills to the cap before the stream runs out.
func TestDecodeBounded_TruncatedOverLimitStreamRejected(t *testing.T) {
	const limit = int64(64 * 1024)
	raw := zlibZeros(t, 8*1024*1024)
	sd := flateStream(raw[:len(raw)-4])

	if _, err := decodeBounded(sd, limit); !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("expected ErrUnsupportedPDF for a truncated over-ceiling stream, got %v", err)
	}
}

// A panic that is not pdfcpu's unchecked tail carries no proof that the stream
// stayed under the cap, so it is re-panicked instead of becoming a fallback.
func TestCappedDecode_NonBoundsPanicIsNotSwallowed(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected the panic to propagate")
		}
	}()

	// A nil StreamDict makes pdfcpu dereference it, which is not a bounds error.
	_, _ = cappedDecode(nil, 1024)
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

// stopperLedStream returns a StreamDict whose pipeline leads with a filter
// pdfcpu's decoder stops at, followed by the named terminal filter over a
// zlib-compressed payload.
func stopperLedStream(t *testing.T, leading, terminal string) *pdfcpu_types.StreamDict {
	t.Helper()
	return &pdfcpu_types.StreamDict{
		Dict: pdfcpu_types.Dict{},
		FilterPipeline: []pdfcpu_types.PDFFilter{
			{Name: leading},
			{Name: terminal},
		},
		Raw: zlibBytes(t, []byte("payload behind a stopping filter")),
	}
}

// pdfcpu breaks out of its filter loop at a non-4-component DCTDecode and then
// copies from a nil reader, so a pipeline led by one must be refused instead of
// decoded. The rejection is a decode failure, not the extraction-ceiling
// sentinel, or both call sites would report the stream as too large.
func TestDecodeBounded_PipelineLedByDCTReturnsDecodeError(t *testing.T) {
	const limit = int64(64 * 1024)

	for _, terminal := range []string{"FlateDecode", "RunLengthDecode"} {
		sd := stopperLedStream(t, "DCTDecode", terminal)

		out, err := decodeBounded(sd, limit)
		if !errors.Is(err, errStoppingFilterLeadsPipeline) {
			t.Fatalf("terminal %s: expected the leading-stopper refusal, got err=%v (out %d bytes)", terminal, err, len(out))
		}
		if errors.Is(err, ErrUnsupportedPDF) {
			t.Errorf("terminal %s: refusal must not carry the ceiling sentinel: %v", terminal, err)
		}
	}
}

// The same holds for JPXDecode, which the decoder stops at regardless of the
// component count.
func TestDecodeBounded_PipelineLedByJPXReturnsDecodeError(t *testing.T) {
	const limit = int64(64 * 1024)
	sd := stopperLedStream(t, "JPXDecode", "FlateDecode")

	out, err := decodeBounded(sd, limit)
	if !errors.Is(err, errStoppingFilterLeadsPipeline) {
		t.Fatalf("expected the leading-stopper refusal, got err=%v (out %d bytes)", err, len(out))
	}
	if errors.Is(err, ErrUnsupportedPDF) {
		t.Errorf("refusal must not carry the ceiling sentinel: %v", err)
	}
}

// A 4-component DCTDecode does not stop the decoder, so it stays on the
// ordinary decode path rather than the refusal above.
func TestDecodeBounded_PipelineLedByFourComponentDCTIsNotRefused(t *testing.T) {
	const limit = int64(64 * 1024)
	sd := stopperLedStream(t, "DCTDecode", "FlateDecode")
	sd.CSComponents = 4

	_, err := decodeBounded(sd, limit)
	if err == nil {
		t.Fatal("expected the JPEG decode of a non-JPEG payload to fail")
	}
	if errors.Is(err, errStoppingFilterLeadsPipeline) {
		t.Errorf("4-component DCTDecode must not be treated as a stopping filter: %v", err)
	}
}

// A stopping filter that is neither first nor last leaves pdfcpu's reader
// valid, so the filters ahead of it still yield their output and the pipeline
// is not refused.
func TestDecodeBounded_MidPipelineStopperDecodesEarlierFilters(t *testing.T) {
	const limit = int64(64 * 1024)
	payload := []byte("flate output ahead of a mid-pipeline stopper")
	sd := &pdfcpu_types.StreamDict{
		Dict: pdfcpu_types.Dict{},
		FilterPipeline: []pdfcpu_types.PDFFilter{
			{Name: "FlateDecode"},
			{Name: "DCTDecode"},
			{Name: "ASCII85Decode"},
		},
		Raw: zlibBytes(t, payload),
	}

	out, err := decodeBounded(sd, limit)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Errorf("payload mismatch: got %q, want %q", out, payload)
	}
}

// A stopping filter in terminal position leaves the pipeline decodable up to
// that point, which is what pdfcpu does with it.
func TestDecodeBounded_TrailingDCTInPipelineDecodesEarlierFilters(t *testing.T) {
	const limit = int64(64 * 1024)
	payload := []byte("flate output behind a trailing DCTDecode")
	sd := &pdfcpu_types.StreamDict{
		Dict: pdfcpu_types.Dict{},
		FilterPipeline: []pdfcpu_types.PDFFilter{
			{Name: "FlateDecode"},
			{Name: "DCTDecode"},
		},
		Raw: zlibBytes(t, payload),
	}

	out, err := decodeBounded(sd, limit)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Errorf("payload mismatch: got %q, want %q", out, payload)
	}
}

// ascii85Stream returns a StreamDict whose sole filter is ASCII85Decode over
// the encoding of payload.
func ascii85Stream(payload []byte) *pdfcpu_types.StreamDict {
	buf := make([]byte, ascii85.MaxEncodedLen(len(payload)))
	n := ascii85.Encode(buf, payload)
	return &pdfcpu_types.StreamDict{
		Dict:           pdfcpu_types.Dict{},
		FilterPipeline: []pdfcpu_types.PDFFilter{{Name: "ASCII85Decode"}},
		Raw:            append(buf[:n], '~', '>'),
	}
}

// ASCII85Decode is one of the three filters the probe is applied to, so its
// in-bounds path has to come back with the exact payload rather than the empty
// result the capped decode hands back.
func TestDecodeBounded_SoleASCII85InBoundsReturnsDecodedBytes(t *testing.T) {
	const limit = int64(64 * 1024)
	payload := bytes.Repeat([]byte("ascii85-payload-"), 256)

	out, err := decodeBounded(ascii85Stream(payload), limit)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(out), len(payload))
	}
}

// The same filter over a payload past the limit is rejected at the ceiling.
func TestDecodeBounded_SoleASCII85OverLimitRejected(t *testing.T) {
	const limit = int64(4 * 1024)
	payload := bytes.Repeat([]byte{'x'}, 32*1024)

	if _, err := decodeBounded(ascii85Stream(payload), limit); !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("expected ErrUnsupportedPDF, got %v", err)
	}
}

// A stream far below the limit exercises the same short-output path as the
// Flate case, which is where a naive probe implementation breaks.
func TestDecodeBounded_SoleASCII85FarBelowLimitReturnsDecodedBytes(t *testing.T) {
	const limit = int64(50 * 1024 * 1024)
	payload := []byte("tiny")

	out, err := decodeBounded(ascii85Stream(payload), limit)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Errorf("payload mismatch: got %q, want %q", out, payload)
	}
}

// A stream with a broken checksum whose decoded size lands where pdfcpu's buffer
// capacity reaches the cap comes back from the capped decode as exactly limit+1
// bytes with a nil error - indistinguishable from a real over-ceiling result.
// It must still decode, because it is in bounds.
func TestDecodeBounded_BrokenChecksumInBoundsStreamIsNotRejected(t *testing.T) {
	// 40 MiB decoded against a 50 MiB limit: past the point where the buffer has
	// doubled to 64 MiB, so the stale tail fills the cap without panicking.
	const limit = int64(50 * 1024 * 1024)
	const size = 40 * 1024 * 1024
	raw := zlibZeros(t, size)
	// Corrupt the trailing Adler-32 rather than truncating, so the deflate data
	// is intact and the decoded length is exactly `size`.
	broken := append([]byte(nil), raw...)
	broken[len(broken)-1] ^= 0xff

	sd := flateStream(broken)
	out, err := decodeBounded(sd, limit)
	if err != nil {
		t.Fatalf("expected a %d-byte in-bounds stream to decode under a %d limit, got %v", size, limit, err)
	}
	if int64(len(out)) != size {
		t.Errorf("got %d bytes, want %d", len(out), size)
	}
}

// The exact-count confirmation must not become a way past the ceiling: a genuine
// bomb still counts over the limit and is still refused.
func TestDecodeBounded_ExactCountDoesNotAdmitABomb(t *testing.T) {
	const limit = int64(64 * 1024)
	sd := flateStream(zlibZeros(t, 8*1024*1024))

	if _, err := decodeBounded(sd, limit); !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("expected ErrUnsupportedPDF, got %v", err)
	}
}

// A zero row width makes pdfcpu's predictor reconstruction loop forever on the
// /Predictor 2 path and divide by zero on /Predictor 12. Both are refused before
// the decode starts, because a hang holding the document lock cannot be
// recovered from.
func TestDecodeBounded_NonPositivePredictorParmsRefused(t *testing.T) {
	for _, c := range []struct {
		name      string
		predictor int
		key       string
		value     int
	}{
		{"predictor 2 with zero columns", 2, "Columns", 0},
		{"predictor 12 with zero columns", 12, "Columns", 0},
		{"zero colors", 12, "Colors", 0},
		{"zero bits per component", 12, "BitsPerComponent", 0},
		{"negative columns", 12, "Columns", -1},
	} {
		sd := flateStream(zlibZeros(t, 4096))
		sd.FilterPipeline[0].DecodeParms = pdfcpu_types.Dict{
			"Predictor": pdfcpu_types.Integer(c.predictor),
			c.key:       pdfcpu_types.Integer(c.value),
		}

		done := make(chan error, 1)
		go func() {
			_, err := decodeBounded(sd, 64*1024)
			done <- err
		}()
		select {
		case err := <-done:
			if !errors.Is(err, errUnrunnablePredictor) {
				t.Errorf("%s: expected the unrunnable-predictor refusal, got %v", c.name, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: decode did not return; the predictor guard is not holding", c.name)
		}
	}
}

// Predictor parameters that are merely absent are legal and must not be refused:
// PDF defines defaults for all three.
func TestDecodeBounded_AbsentPredictorParmsAreNotRefused(t *testing.T) {
	sd := flateStream(zlibZeros(t, 4096))
	sd.FilterPipeline[0].DecodeParms = pdfcpu_types.Dict{"Predictor": pdfcpu_types.Integer(12)}

	if _, err := decodeBounded(sd, 64*1024); errors.Is(err, errUnrunnablePredictor) {
		t.Errorf("absent predictor parms must not be refused: %v", err)
	}
}

// lzwStream returns a StreamDict whose sole filter is LZWDecode over payload.
// /EarlyChange 0 selects the code-width behaviour Go's encoder produces; pdfcpu
// defaults the entry to 1, which would desynchronise the decoder.
func lzwStream(t *testing.T, payload []byte) *pdfcpu_types.StreamDict {
	t.Helper()
	var buf bytes.Buffer
	w := lzw.NewWriter(&buf, lzw.MSB, 8)
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("lzw write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("lzw close: %v", err)
	}
	return &pdfcpu_types.StreamDict{
		Dict: pdfcpu_types.Dict{},
		FilterPipeline: []pdfcpu_types.PDFFilter{{
			Name:        "LZWDecode",
			DecodeParms: pdfcpu_types.Dict{"EarlyChange": pdfcpu_types.Integer(0)},
		}},
		Raw: buf.Bytes(),
	}
}

// LZWDecode is the third allowlisted terminal filter, and the one the exact-count
// confirmation does not cover, so its in-bounds path needs its own pin.
func TestDecodeBounded_SoleLZWInBoundsReturnsDecodedBytes(t *testing.T) {
	const limit = int64(64 * 1024)
	payload := bytes.Repeat([]byte("lzw-payload-"), 512)

	out, err := decodeBounded(lzwStream(t, payload), limit)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(out), len(payload))
	}
}

// A stream far below the limit is the H1 case for this filter: the capped decode
// reports a short read with no payload, which must read as in-bounds.
func TestDecodeBounded_SoleLZWFarBelowLimitReturnsDecodedBytes(t *testing.T) {
	const limit = int64(50 * 1024 * 1024)
	payload := []byte("tiny lzw")

	out, err := decodeBounded(lzwStream(t, payload), limit)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Errorf("payload mismatch: got %q, want %q", out, payload)
	}
}

// An LZW stream inflating past the limit is rejected at the ceiling. The limit
// clears the compressed size so the raw pre-guard is not what refuses it.
func TestDecodeBounded_SoleLZWOverLimitRejected(t *testing.T) {
	const limit = int64(256 * 1024)
	sd := lzwStream(t, make([]byte, 8*1024*1024))

	if int64(len(sd.Raw)) > limit {
		t.Fatalf("fixture defeats the test: %d compressed bytes exceed the %d limit", len(sd.Raw), limit)
	}

	// Assert the allocation too. Output alone cannot tell the probe from the
	// post-decode check: both reject this stream, one of them after inflating it.
	const allocCeiling = uint64(4 * 1024 * 1024)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := decodeBounded(sd, limit)
	runtime.ReadMemStats(&after)

	if !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("expected ErrUnsupportedPDF, got %v", err)
	}
	if delta := after.TotalAlloc - before.TotalAlloc; delta >= allocCeiling {
		t.Errorf("allocated %d bytes rejecting an 8 MiB inflation at a %d-byte limit; want under %d",
			delta, limit, allocCeiling)
	}
}
