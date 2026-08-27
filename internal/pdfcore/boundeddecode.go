package pdfcore

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// errProbeUnusable reports that pdfcpu's size-capped decode could not answer for
// a stream, so the caller falls back to the unbounded decode and the ceiling is
// applied after the allocation instead of during it. pdfcpu tails a capped decode
// with an unchecked data[:maxLen]; a filter returning fewer bytes than the cap
// with a nil error trips it, which is what the truncated and checksum-broken
// zlib streams pdfcpu deliberately tolerates do. See isSliceBoundsPanic for what
// the detection does and does not establish - it is a message match, so it
// cannot prove the panic came from that tail rather than from elsewhere in a
// filter, and the fallback is what makes it safe either way.
var errProbeUnusable = errors.New("bounded decode probe unusable")

// errStoppingFilterLeadsPipeline reports a pipeline whose first filter is one
// pdfcpu stops at. pdfcpu leaves its reader nil in that shape and copies from
// it, so the decode is refused instead of attempted.
var errStoppingFilterLeadsPipeline = errors.New("stopping filter leads the filter pipeline")

// errUnrunnablePredictor reports predictor parameters that make pdfcpu's row
// reconstruction non-terminating or divide by zero, so the decode is refused
// instead of attempted.
var errUnrunnablePredictor = errors.New("predictor parameters cannot be applied")

// decodeBounded returns the decoded bytes of sd and rejects a stream whose
// decoded size exceeds limit with ErrUnsupportedPDF. On success sd.Content
// holds the same bytes, so the caller may hand the StreamDict on to pdfcpu's
// renderer.
//
// What the limit bounds during inflation: when the last filter of the pipeline
// is FlateDecode without a /Predictor above 1, LZWDecode or ASCII85Decode, the
// stream is first probed with a size-capped decode that stops inflating at
// limit+1 bytes, so a highly compressible payload is rejected without its full
// decompressed size ever being allocated. An in-bounds stream is then decoded a
// second time: the capped decode discards its output, so the payload has to
// come from a full decode, which by then is proven to stay under the ceiling.
//
// Every other shape is decoded in full and measured afterwards, so it is
// rejected at the ceiling but only after the allocation. That covers
// ASCIIHexDecode, RunLengthDecode, DCTDecode, CCITTFaxDecode and predictor
// FlateDecode. The reason is not that pdfcpu ignores the cap for those filters:
// RunLengthDecode and predictor FlateDecode do honour it. It is that they
// report short output with a nil error, which reaches pdfcpu's unchecked
// data[:maxLen] tail, and whether that panics or silently returns a stale tail
// depends on the capacity of pdfcpu's buffer. A stale tail reads as exactly
// limit+1 bytes and would reject an ordinary in-bounds stream, so probing them
// would break extraction more often than it stops a bomb.
//
// Only the terminal filter of a multi-filter pipeline is capped; pdfcpu runs
// every earlier filter unbounded, so an intermediate stage can still inflate
// without limit. The len(sd.Raw) pre-guard bounds the input handed to the first
// filter, not what any stage expands it to.
//
// A pipeline that opens with a filter the decoder stops at (JPXDecode, or
// DCTDecode on a stream that is not 4-component) is refused, because pdfcpu
// copies from a nil reader in that shape.
func decodeBounded(sd *pdfcpu_types.StreamDict, limit int64) ([]byte, error) {
	// The loader pre-decodes a few stream kinds (XRef streams, object streams,
	// zero-length streams), which need no decode at all. Answered before the
	// raw guard, because an encoding may be larger than the payload it carries.
	if sd.Content != nil {
		if int64(len(sd.Content)) > limit {
			return nil, decodedCeilingExceeded(limit)
		}
		return sd.Content, nil
	}

	if int64(len(sd.Raw)) > limit {
		return nil, rawCeilingExceeded(limit)
	}

	if isPassthroughStream(sd) {
		sd.Content = sd.Raw
		return sd.Content, nil
	}

	// A stopping filter anywhere after the first leaves pdfcpu's reader valid
	// and yields the output of the filters ahead of it, which is preserved.
	// Only in first position is the reader nil.
	if first := sd.FilterPipeline[0]; stopsDecoding(first, sd.CSComponents) {
		return nil, fmt.Errorf("%w: %s", errStoppingFilterLeadsPipeline, first.Name)
	}

	// A zero row width makes pdfcpu's predictor loop read zero bytes forever, or
	// divide by zero on the /Predictor 12 path. The first of those spins a core
	// while the caller holds the document lock, and no recover can reach it, so
	// the shape is refused up front. pdfcpu guards /Colors but not /Columns.
	//
	// EVERY filter is checked, not just the terminal one: pdfcpu runs the earlier
	// stages of a pipeline unbounded, so a predictor sitting on a non-terminal
	// filter reaches the same loop. `[/FlateDecode /ASCII85Decode]` carrying
	// `/DecodeParms [<< /Predictor 2 /Columns 0 >> null]` hangs on stage one.
	for _, f := range sd.FilterPipeline {
		if !hasPredictor(f.DecodeParms) {
			continue
		}
		if err := checkPredictorParms(f.DecodeParms, limit); err != nil {
			return nil, err
		}
	}

	if isProbeableStream(sd) {
		if err := probeCeiling(sd, limit); err != nil {
			return nil, err
		}
	}

	if err := safeCall(func() error { return sd.Decode() }); err != nil {
		return nil, err
	}
	if int64(len(sd.Content)) > limit {
		return nil, decodedCeilingExceeded(limit)
	}
	return sd.Content, nil
}

// probeCeiling reports ErrUnsupportedPDF when sd inflates past limit. The
// capped decode discards its output, so a nil return means only "in bounds",
// never that the payload is in hand. A stream the cap cannot be applied to also
// returns nil, leaving the caller on the unbounded path.
func probeCeiling(sd *pdfcpu_types.StreamDict, limit int64) error {
	probe, err := cappedDecode(sd, limit+1)
	switch {
	// io.CopyN reports a short copy as io.EOF, which is how a stream that
	// decoded fully within the cap surfaces. It carries no payload.
	// ANY probe error means the cap could not be applied to this stream, not that
	// the stream is unextractable, so fall back to the unbounded decode and let
	// the post-decode check enforce the ceiling. Three shapes arrive here and all
	// three want the same answer: io.EOF, which is how io.CopyN reports a stream
	// that decoded fully within the cap and carries no payload; the unusable
	// probe; and a filter stopped mid-symbol at the cap, which errors where the
	// same stream decodes fine in full. A bomb does NOT reach here - an
	// over-ceiling stream fills the cap and returns no error at all.
	case err != nil:
		return nil
	case int64(len(probe)) > limit:
		// pdfcpu's stale-tail read is indistinguishable from a real over-ceiling
		// result here: both come back as exactly limit+1 bytes with a nil error.
		// pdfcpu tolerates a broken checksum or a truncated deflate stream, so an
		// in-bounds stream whose buffer capacity happens to reach the cap lands in
		// this branch too. Confirm with an exact count before refusing.
		if size, ok := flateDecodedSize(sd, limit); ok && size <= limit {
			return nil
		}
		return decodedCeilingExceeded(limit)
	}
	return nil
}

// flateDecodedSize counts what a sole no-predictor FlateDecode stream inflates
// to, stopping at limit+1 bytes, and reports whether the count is meaningful. It
// exists to settle the one case pdfcpu's capped decode cannot: counting through
// io.Discard holds nothing but the reader's window, so it is exact where the
// stale tail is ambiguous. A truncated or checksum-broken stream counts what it
// could read, matching the tolerance pdfcpu applies to the same input.
//
// Only a single-filter pipeline qualifies: with any filter ahead of it sd.Raw is
// not the deflate input, and the count would be meaningless.
func flateDecodedSize(sd *pdfcpu_types.StreamDict, limit int64) (int64, bool) {
	if len(sd.FilterPipeline) != 1 {
		return 0, false
	}
	f := sd.FilterPipeline[0]
	if f.Name != "FlateDecode" || hasPredictor(f.DecodeParms) {
		return 0, false
	}
	zr, err := zlib.NewReader(bytes.NewReader(sd.Raw))
	if err != nil {
		return 0, false
	}
	defer func() { _ = zr.Close() }()
	size, err := io.CopyN(io.Discard, zr, limit+1)
	switch {
	case err == nil, errors.Is(err, io.EOF),
		errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, zlib.ErrChecksum):
		return size, true
	default:
		return 0, false
	}
}

// cappedDecode runs pdfcpu's size-capped decode, stopping inflation at maxLen
// bytes. It reports errProbeUnusable when the call panics on pdfcpu's unchecked
// data[:maxLen] tail. Any other panic is re-panicked, so a genuine bug still
// surfaces through safeCall's runtime.Error contract instead of being turned
// into a silent fallback.
func cappedDecode(sd *pdfcpu_types.StreamDict, maxLen int64) (data []byte, err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if !isSliceBoundsPanic(r) {
			panic(r)
		}
		data, err = nil, errProbeUnusable
	}()
	err = safeCall(func() error {
		var e error
		data, e = sd.DecodeLength(maxLen)
		return e
	})
	return data, err
}

// isSliceBoundsPanic reports whether r is a runtime slice-bounds error, which is
// what pdfcpu's unchecked data[:maxLen] tail raises. The match is on the message,
// so it cannot distinguish that tail from an out-of-range slice elsewhere in a
// filter. It does not have to: for the three probeable filters an over-ceiling
// stream always fills the cap and so cannot reach the tail, and any other
// bounds panic falls back to a decode whose result is still measured against the
// ceiling - the same guarantee the extraction path had before, one allocation
// later. Panics that are not bounds errors carry no such reasoning and re-panic.
func isSliceBoundsPanic(r any) bool {
	e, ok := r.(runtime.Error)
	return ok && strings.Contains(e.Error(), "slice bounds out of range")
}

// isPassthroughStream reports whether pdfcpu hands sd.Raw back unchanged
// instead of running a filter over it: no filters at all, or a sole stopping
// filter. A cap on those shapes slices the content unchecked, and an
// empty-but-non-nil pipeline reaches pdfcpu's decoder with a nil reader.
func isPassthroughStream(sd *pdfcpu_types.StreamDict) bool {
	if len(sd.FilterPipeline) == 0 {
		return true
	}
	return len(sd.FilterPipeline) == 1 && stopsDecoding(sd.FilterPipeline[0], sd.CSComponents)
}

// stopsDecoding reports whether pdfcpu breaks out of its filter loop at f
// rather than running it: JPXDecode always, DCTDecode unless the stream is
// 4-component. As the sole filter of a pipeline that means the raw bytes are
// the payload; as the first of several it means pdfcpu copies from a nil
// reader.
func stopsDecoding(f pdfcpu_types.PDFFilter, csComponents int) bool {
	return f.Name == "JPXDecode" || (f.Name == "DCTDecode" && csComponents != 4)
}

// isProbeableStream reports whether the terminal filter of sd's pipeline both
// honours a decode cap and reports short output as io.EOF. The second half is
// what keeps pdfcpu's unchecked data[:maxLen] tail out of reach on an in-bounds
// stream. Requires a non-empty pipeline.
func isProbeableStream(sd *pdfcpu_types.StreamDict) bool {
	f := sd.FilterPipeline[len(sd.FilterPipeline)-1]
	switch f.Name {
	case "LZWDecode", "ASCII85Decode":
		return true
	case "FlateDecode":
		return !hasPredictor(f.DecodeParms)
	default:
		return false
	}
}

// Bounds on predictor parameters. pdfcpu sizes TWO row buffers of
// (bpc*colors*columns+7)/8 (+1 off the TIFF path) from these before reading a
// single byte, so an unbounded value is an unbounded allocation that neither the
// raw pre-guard nor limit constrains - a 15-byte deflate stream declaring
// /Columns 1000000000 allocates gigabytes. PDF allows sample depths up to 16 and
// DeviceN carries at most 32 colorants; a row wider than 2^20 samples is not a
// real image. Together these cap the row size near 64 MB and keep the product
// clear of int64 overflow, which would otherwise wrap to a zero row size and
// divide by zero.
const (
	maxPredictorColumns          = 1 << 20
	maxPredictorColors           = 32
	maxPredictorBitsPerComponent = 16
)

// checkPredictorParms rejects predictor parameters that would make pdfcpu's row
// reconstruction non-terminating, divide by zero, or allocate without regard to
// the ceiling. A zero row width is the non-terminating case: /Predictor 2 leaves
// io.ReadFull reading nothing forever, and the PNG predictors divide by it.
//
// Absent entries need no check - PDF defines 1 for /Columns and /Colors and 8 for
// /BitsPerComponent.
func checkPredictorParms(parms pdfcpu_types.Dict, limit int64) error {
	// PDF defaults when the entry is absent.
	values := map[string]int{"Columns": 1, "Colors": 1, "BitsPerComponent": 8}
	for _, bound := range []struct {
		key      string
		min, max int
	}{
		{"Columns", 1, maxPredictorColumns},
		{"Colors", 1, maxPredictorColors},
		{"BitsPerComponent", 1, maxPredictorBitsPerComponent},
	} {
		v, found := parms[bound.key]
		if !found {
			continue
		}
		n, ok := predictorParmValue(v)
		if !ok {
			continue
		}
		if n < bound.min || n > bound.max {
			return fmt.Errorf("%w: /%s is %d, outside %d..%d",
				errUnrunnablePredictor, bound.key, n, bound.min, bound.max)
		}
		values[bound.key] = n
	}

	// The per-parameter bounds above only keep the arithmetic safe. On their own
	// they still admit a 64 MB row, and pdfcpu allocates TWO of those before
	// reading anything - 128 MB from a handful of bytes, whatever ceiling the
	// caller asked for. A row wider than the whole ceiling cannot belong to a
	// stream that fits under it, so measure the row against the ceiling.
	rowBytes := int64(values["BitsPerComponent"]*values["Colors"]*values["Columns"]+7) / 8
	if rowBytes > limit {
		return fmt.Errorf("%w: a %d-byte predictor row exceeds the %d-byte ceiling",
			errUnrunnablePredictor, rowBytes, limit)
	}
	return nil
}

// predictorParmValue reads a decode parameter the way pdfcpu's parameter
// flattening does: an Integer, or a Boolean as 0 or 1. Reading Booleans matters -
// pdfcpu coerces them, so /Columns false arrives at the predictor as a zero row
// width exactly as /Columns 0 would. Any other type pdfcpu discards, leaving the
// PDF default, so it needs no check here.
func predictorParmValue(v pdfcpu_types.Object) (int, bool) {
	switch t := v.(type) {
	case pdfcpu_types.Integer:
		return t.Value(), true
	case pdfcpu_types.Boolean:
		if t.Value() {
			return 1, true
		}
		return 0, true
	}
	return 0, false
}

// hasPredictor reports whether parms carries a /Predictor above 1, the value
// that puts pdfcpu's FlateDecode on its row-reconstruction path. Non-integer
// values are ignored, matching pdfcpu's own parameter flattening.
func hasPredictor(parms pdfcpu_types.Dict) bool {
	v, found := parms["Predictor"]
	if !found {
		return false
	}
	i, ok := v.(pdfcpu_types.Integer)
	return ok && i.Value() > 1
}

// decodedCeilingExceeded builds the rejection for a stream decoding past limit
// bytes.
func decodedCeilingExceeded(limit int64) error {
	return fmt.Errorf("%w: decoded stream exceeds the %d-byte extraction ceiling", ErrUnsupportedPDF, limit)
}

// rawCeilingExceeded builds the rejection for a stream whose encoded bytes
// already exceed limit, before any decode is attempted.
func rawCeilingExceeded(limit int64) error {
	return fmt.Errorf("%w: encoded stream exceeds the %d-byte extraction ceiling", ErrUnsupportedPDF, limit)
}
