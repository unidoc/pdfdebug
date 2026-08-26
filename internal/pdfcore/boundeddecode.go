package pdfcore

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// errProbeUnusable reports that pdfcpu's size-capped decode could not answer
// for a stream, so the caller falls back to the unbounded decode. pdfcpu tails
// a capped decode with an unchecked data[:maxLen]; a filter that returns fewer
// bytes than the cap with a nil error trips it, which happens on the truncated
// and checksum-broken zlib streams pdfcpu deliberately tolerates. Reaching that
// tail proves the stream decoded to fewer bytes than the cap, so the fallback
// decode stays under the ceiling.
var errProbeUnusable = errors.New("bounded decode probe unusable")

// errStoppingFilterLeadsPipeline reports a pipeline whose first filter is one
// pdfcpu stops at. pdfcpu leaves its reader nil in that shape and copies from
// it, so the decode is refused instead of attempted.
var errStoppingFilterLeadsPipeline = errors.New("stopping filter leads the filter pipeline")

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
	case errors.Is(err, io.EOF), errors.Is(err, errProbeUnusable):
		return nil
	case err != nil:
		return err
	case int64(len(probe)) > limit:
		return decodedCeilingExceeded(limit)
	}
	return nil
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

// isSliceBoundsPanic reports whether r is the runtime slice-bounds error raised
// by pdfcpu's unchecked data[:maxLen] tail. Only that panic proves the stream
// decoded to fewer bytes than the cap, which is what makes the unbounded
// fallback safe.
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
