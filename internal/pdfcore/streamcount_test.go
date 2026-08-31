// Co-located unit tests for the per-stage stream counter:
//
//	countStages(sd *pdfcpu_types.StreamDict, limit int64) error
//
// The counter measures what every filter of a pipeline produces before any of it
// is allocated. These tests cover the multi-filter shapes: a stage that inflates
// behind a filter that shrinks or abandons it, and the tolerated zlib corruption
// that makes a stream's real size the only safe thing to measure.
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

// zlibRepeat returns the zlib encoding of prefix followed by as many whole
// copies of unit as fit in n bytes, written in blocks so a multi-megabyte
// fixture never holds n bytes at once.
func zlibRepeat(t *testing.T, prefix, unit []byte, n int) []byte {
	t.Helper()
	perBlock := max(1, 64*1024/len(unit))
	block := bytes.Repeat(unit, perBlock)
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(prefix); err != nil {
		t.Fatalf("zlib write: %v", err)
	}
	for units := n / len(unit); units > 0; {
		k := min(perBlock, units)
		if _, err := w.Write(block[:k*len(unit)]); err != nil {
			t.Fatalf("zlib write: %v", err)
		}
		units -= k
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}
	return buf.Bytes()
}

// ascii85Wrap returns the ASCII85 encoding of payload with the "~>" terminator
// pdfcpu requires.
func ascii85Wrap(payload []byte) []byte {
	enc := make([]byte, ascii85.MaxEncodedLen(len(payload)))
	n := ascii85.Encode(enc, payload)
	return append(enc[:n], '~', '>')
}

// pipeline returns a StreamDict over raw with the named filters, applied in the
// order pdfcpu applies them.
func pipeline(raw []byte, names ...string) *pdfcpu_types.StreamDict {
	filters := make([]pdfcpu_types.PDFFilter, len(names))
	for i, n := range names {
		filters[i] = pdfcpu_types.PDFFilter{Name: n}
	}
	return &pdfcpu_types.StreamDict{Dict: pdfcpu_types.Dict{}, FilterPipeline: filters, Raw: raw}
}

// allocatedBy reports the bytes allocated by fn. TotalAlloc is cumulative and
// never decremented, so a GC landing mid-call cannot mask an allocation.
func allocatedBy(fn func()) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	fn()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// pdfcpu tolerates a broken Adler-32 and hands back the bytes it inflated. With
// more than one filter in the pipeline the decoded size is the only thing that
// can decide the ceiling, and the count reads it exactly, so an in-bounds
// attachment carrying that corruption still extracts.
func TestCountStages_MultiFilterBrokenChecksumIsNotRejected(t *testing.T) {
	const limit = int64(50 * 1024 * 1024)
	const size = 40 * 1024 * 1024
	inner := zlibRepeat(t, nil, []byte{0}, size)
	broken := append([]byte(nil), inner...)
	broken[len(broken)-1] ^= 0xff

	sd := pipeline(ascii85Wrap(broken), "ASCII85Decode", "FlateDecode")
	if err := countStages(sd, limit); err != nil {
		t.Fatalf("a %d-byte in-bounds stream must count as in bounds under a %d limit, got %v",
			size, limit, err)
	}

	out, err := decodeBounded(sd, limit)
	if err != nil {
		t.Fatalf("expected the stream to decode, got %v", err)
	}
	if int64(len(out)) != size {
		t.Errorf("got %d bytes, want %d", len(out), size)
	}
}

// A filter that shrinks its input does not hide a bomb behind it. ASCIIHexDecode
// halves what it reads, so the Flate stage ahead of it has to be measured on its
// own: pdfcpu buffers that stage in full before the hex decode ever runs.
func TestCountStages_InflationBehindAShrinkingFilterIsRejected(t *testing.T) {
	const limit = int64(4 * 1024 * 1024)
	// 64 MiB of hex digits, which halve to 32 MiB of payload. Both stages are
	// over the ceiling; the point is that neither is allocated to find out.
	sd := pipeline(zlibRepeat(t, nil, []byte("00"), 64*1024*1024),
		"FlateDecode", "ASCIIHexDecode")

	if int64(len(sd.Raw)) > limit {
		t.Fatalf("fixture defeats the test: %d raw bytes exceed the %d limit, so the pre-guard rejects first",
			len(sd.Raw), limit)
	}

	var err error
	alloc := allocatedBy(func() { _, err = decodeBounded(sd, limit) })
	if !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("expected ErrUnsupportedPDF, got %v", err)
	}
	if alloc >= uint64(limit) {
		t.Errorf("allocated %d bytes rejecting a 64 MiB inflation at a %d-byte limit", alloc, limit)
	}
}

// A stage its successor stops reading is still an allocation pdfcpu makes, so it
// is still measured. Here the Flate stage inflates to 64 MiB of which the
// second Flate reads only the first few bytes before its own stream ends.
func TestCountStages_InflationAbandonedByTheNextFilterIsRejected(t *testing.T) {
	const limit = int64(4 * 1024 * 1024)
	inner := zlibRepeat(t, nil, []byte("payload"), 4096)
	// The zeros after the inner stream are bytes the second Flate never reads.
	sd := pipeline(zlibRepeat(t, inner, []byte{0}, 64*1024*1024),
		"FlateDecode", "FlateDecode")

	if int64(len(sd.Raw)) > limit {
		t.Fatalf("fixture defeats the test: %d raw bytes exceed the %d limit", len(sd.Raw), limit)
	}

	var err error
	alloc := allocatedBy(func() { _, err = decodeBounded(sd, limit) })
	if !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("expected ErrUnsupportedPDF, got %v", err)
	}
	if alloc >= uint64(limit) {
		t.Errorf("allocated %d bytes rejecting a 64 MiB inflation at a %d-byte limit", alloc, limit)
	}
}

// The same shape with RunLengthDecode, whose 0x80 terminator lets it produce
// nothing at all from a stage that inflated hundreds of megabytes.
func TestCountStages_InflationBehindARunLengthTerminatorIsRejected(t *testing.T) {
	const limit = int64(4 * 1024 * 1024)
	sd := pipeline(zlibRepeat(t, []byte{0x80}, []byte{0}, 64*1024*1024),
		"FlateDecode", "RunLengthDecode")

	var err error
	alloc := allocatedBy(func() { _, err = decodeBounded(sd, limit) })
	if !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("expected ErrUnsupportedPDF, got %v", err)
	}
	if alloc >= uint64(limit) {
		t.Errorf("allocated %d bytes rejecting a 64 MiB inflation at a %d-byte limit", alloc, limit)
	}
}

// A predictor stream is counted rather than decoded and measured afterwards, so
// the row reconstruction is inside the bound like every other counted stage.
func TestCountStages_PredictorInflationIsRejectedBeforeAllocation(t *testing.T) {
	const limit = int64(4 * 1024 * 1024)
	// /Predictor 12 with /Columns 7 reads 8-byte rows and writes 7 of them, so
	// 64 MiB of rows is 56 MiB of output.
	sd := flateStream(zlibRepeat(t, nil, []byte{0}, 64*1024*1024))
	sd.FilterPipeline[0].DecodeParms = pdfcpu_types.Dict{
		"Predictor": pdfcpu_types.Integer(12),
		"Columns":   pdfcpu_types.Integer(7),
	}

	var err error
	alloc := allocatedBy(func() { _, err = decodeBounded(sd, limit) })
	if !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("expected ErrUnsupportedPDF, got %v", err)
	}
	if alloc >= uint64(limit) {
		t.Errorf("allocated %d bytes rejecting a 56 MiB predictor inflation at a %d-byte limit", alloc, limit)
	}
}

// The predictor count is the reconstructed width, not the inflated width. A PNG
// predictor drops one filter byte per row, so a stream that inflates past the
// ceiling can still produce an in-bounds payload and must not be refused.
func TestCountStages_PredictorRowsAreCountedWithoutTheFilterByte(t *testing.T) {
	const rows = 10000
	const rowSize = 7
	// Output is exactly the limit; the inflated bytes are 8/7 of it and over.
	const limit = int64(rows * rowSize)

	raw := zlibRepeat(t, nil, []byte{0}, rows*(rowSize+1))
	sd := flateStream(raw)
	sd.FilterPipeline[0].DecodeParms = pdfcpu_types.Dict{
		"Predictor": pdfcpu_types.Integer(12),
		"Columns":   pdfcpu_types.Integer(rowSize),
	}

	if err := countStages(sd, limit); err != nil {
		t.Fatalf("a payload of exactly the ceiling must count as in bounds, got %v", err)
	}

	out, err := decodeBounded(sd, limit)
	if err != nil {
		t.Fatalf("expected the stream to decode, got %v", err)
	}
	if int64(len(out)) != limit {
		t.Errorf("decoded %d bytes, want %d", len(out), limit)
	}
}

// A TIFF predictor carries no row filter byte, so its rows are counted at their
// full width.
func TestCountStages_TIFFPredictorRowsCarryNoFilterByte(t *testing.T) {
	const rows = 1000
	const rowSize = 8
	payload := bytes.Repeat([]byte{0}, rows*rowSize)
	sd := flateStream(zlibBytes(t, payload))
	sd.FilterPipeline[0].DecodeParms = pdfcpu_types.Dict{
		"Predictor": pdfcpu_types.Integer(2),
		"Columns":   pdfcpu_types.Integer(rowSize),
	}

	if err := countStages(sd, int64(len(payload))); err != nil {
		t.Fatalf("a TIFF-predictor payload of exactly the ceiling must be in bounds, got %v", err)
	}
	if err := countStages(sd, int64(len(payload))-1); !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("one byte under the ceiling must be refused, got %v", err)
	}
}

// The counter decodes real bytes at every stage it models, not just the right
// number of them, so a filter reading the output of another still counts what
// pdfcpu would produce. ASCIIHexDecode feeding FlateDecode is the shape that
// catches a counter emitting filler.
func TestCountStages_HexFeedingFlateCountsTheRealPayload(t *testing.T) {
	payload := bytes.Repeat([]byte("hex-then-flate-"), 512)
	inner := zlibBytes(t, payload)

	hexed := make([]byte, 0, len(inner)*2+1)
	const digits = "0123456789abcdef"
	for _, b := range inner {
		hexed = append(hexed, digits[b>>4], digits[b&0x0f])
	}
	sd := pipeline(append(hexed, '>'), "ASCIIHexDecode", "FlateDecode")

	if err := countStages(sd, int64(len(payload))); err != nil {
		t.Fatalf("a payload of exactly the ceiling must be in bounds, got %v", err)
	}
	if err := countStages(sd, int64(len(payload))-1); !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("one byte under the ceiling must be refused, got %v", err)
	}

	out, err := decodeBounded(sd, int64(len(payload)))
	if err != nil {
		t.Fatalf("expected the stream to decode, got %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Errorf("payload mismatch: got %d bytes, want %d", len(out), len(payload))
	}
}

// A filter the counter does not model stops the chain, and the stages ahead of
// it are still measured. A Flate bomb feeding CCITTFaxDecode is refused on the
// Flate stage even though nothing can predict what CCITT would produce.
func TestCountStages_InflationAheadOfAnUnmodelledFilterIsRejected(t *testing.T) {
	const limit = int64(4 * 1024 * 1024)
	sd := pipeline(zlibRepeat(t, nil, []byte{0}, 64*1024*1024),
		"FlateDecode", "CCITTFaxDecode")

	if err := countStages(sd, limit); !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("expected the Flate stage to be refused, got %v", err)
	}
}

// A pipeline whose only filter is unmodelled is left to the post-decode check
// rather than refused, so an image the counter cannot measure still extracts.
func TestCountStages_SoleUnmodelledFilterIsNotRefused(t *testing.T) {
	sd := pipeline([]byte("ccitt bytes"), "CCITTFaxDecode")
	if err := countStages(sd, 1024); err != nil {
		t.Fatalf("an unmodelled filter must not be refused by the count, got %v", err)
	}
}

// An LZW stage carrying a predictor produces nothing, because pdfcpu rejects the
// combination outright rather than reconstructing rows. Counting it as an
// inflation would refuse the stream with the wrong reason.
func TestCountStages_PredictorOnLZWCountsAsEmpty(t *testing.T) {
	sd := pipeline([]byte("whatever"), "LZWDecode")
	sd.FilterPipeline[0].DecodeParms = pdfcpu_types.Dict{"Predictor": pdfcpu_types.Integer(12)}

	if err := countStages(sd, 16); err != nil {
		t.Fatalf("a predictor pdfcpu refuses must not be counted as an inflation, got %v", err)
	}
}

// The RunLengthDecode counter mirrors pdfcpu's run encoding: a control byte
// under 0x80 introduces that many plus one literal bytes, one above it repeats
// the next byte, and 0x80 ends the stream.
func TestCountStages_RunLengthRunsAreCountedLikePDFCPU(t *testing.T) {
	// Two literals, then the longest repeat the encoding allows: 0x81 stands for
	// 257-129 copies. 0x80 is the terminator, so 128 copies is the maximum.
	raw := []byte{0x01, 'a', 'b', 0x81, 'c', 0x80}
	const want = int64(2 + 128)

	sd := pipeline(raw, "RunLengthDecode")
	if err := countStages(sd, want); err != nil {
		t.Fatalf("a payload of exactly the ceiling must be in bounds, got %v", err)
	}
	if err := countStages(sd, want-1); !errors.Is(err, ErrUnsupportedPDF) {
		t.Fatalf("one byte under the ceiling must be refused, got %v", err)
	}
}

// A run promising more bytes than remain is refused before the decode. pdfcpu
// indexes past its own buffer on that input, and safeCall re-panics the
// runtime.Error, so letting it through crashes both extraction paths on a
// handful of bytes. The count parses the stream anyway, so it catches the shape.
func TestDecodeBounded_TruncatedRunLengthIsRefusedNotFaulted(t *testing.T) {
	for _, c := range []struct {
		name string
		raw  []byte
	}{
		{"literal run promising four bytes with two present", []byte{0x03, 'a', 'b'}},
		{"literal control byte is the last byte", []byte{0x03}},
		{"repeat control byte with no value after it", []byte{0x81}},
	} {
		t.Run(c.name, func(t *testing.T) {
			sd := pipeline(c.raw, "RunLengthDecode")
			if err := countStages(sd, 1024); !errors.Is(err, errTruncatedRun) {
				t.Errorf("countStages: expected the truncated-run refusal, got %v", err)
			}
			// decodeBounded is what the extraction paths call, and it is the
			// call that used to reach the fault, so assert it and not only the
			// count.
			if _, err := decodeBounded(sd, 1024); !errors.Is(err, errTruncatedRun) {
				t.Errorf("decodeBounded: expected the truncated-run refusal, got %v", err)
			}
		})
	}
}

// A stream ending cleanly BETWEEN runs is not truncated. pdfcpu's loop ends on
// its own there, so refusing it would reject a stream that decodes.
func TestDecodeBounded_RunLengthEndingBetweenRunsIsNotRefused(t *testing.T) {
	// Two literals and a complete repeat, with no 0x80 terminator.
	sd := pipeline([]byte{0x01, 'a', 'b', 0x81, 'c'}, "RunLengthDecode")

	out, err := decodeBounded(sd, 1024)
	if err != nil {
		t.Fatalf("a stream ending between runs must decode, got %v", err)
	}
	if want := 2 + 128; len(out) != want {
		t.Errorf("decoded %d bytes, want %d", len(out), want)
	}
}

// The same refusal applies to a run inside a pipeline, not just a sole filter.
func TestDecodeBounded_TruncatedRunLengthBehindAnotherFilterIsRefused(t *testing.T) {
	sd := pipeline(zlibBytes(t, []byte{0x03, 'a', 'b'}), "FlateDecode", "RunLengthDecode")
	if _, err := decodeBounded(sd, 1024); !errors.Is(err, errTruncatedRun) {
		t.Fatalf("expected the truncated-run refusal, got %v", err)
	}
}
