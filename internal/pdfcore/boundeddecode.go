package pdfcore

import (
	"errors"
	"fmt"
	"io"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// errProbeUnusable reports that pdfcpu's size-capped decode could not answer
// for a stream, so the caller falls back to the unbounded decode. pdfcpu tails
// a capped decode with an unchecked data[:maxLen]; a filter that returns fewer
// bytes than the cap with a nil error trips it, which happens on the truncated
// and checksum-broken zlib streams pdfcpu deliberately tolerates. Reaching that
// tail proves the stream decoded to less than the cap, so the fallback decode
// is under the ceiling anyway.
var errProbeUnusable = errors.New("bounded decode probe unusable")

// decodeBounded returns the decoded bytes of sd and rejects a stream whose
// decoded size exceeds limit with ErrUnsupportedPDF. On success sd.Content
// holds the same bytes, so the caller may hand the StreamDict on to pdfcpu's
// renderer.
//
// What the limit bounds: when the last filter of the pipeline is FlateDecode
// without a /Predictor above 1, LZWDecode or ASCII85Decode, the stream is first
// probed with a size-capped decode that stops inflating at limit+1 bytes, so a
// highly compressible payload is rejected without its full decompressed size
// ever being allocated. An in-bounds stream is then decoded a second time: the
// capped decode discards its output, so the payload has to come from a full
// decode, which by then is proven to stay under the ceiling.
//
// What it does not bound: ASCIIHexDecode, RunLengthDecode, DCTDecode,
// CCITTFaxDecode and predictor FlateDecode keep the unbounded decode-then-
// measure behaviour, because pdfcpu either ignores the cap for those filters or
// slices their output to the cap unchecked on ordinary in-bounds input. In a
// multi-filter pipeline only the last filter is capped, so len(sd.Raw) is
// checked against limit first as a second line of defense.
//
// A pipeline that puts a filter the decoder stops at (JPXDecode, or DCTDecode
// on a stream that is not 4-component) ahead of another filter is refused with
// a decode error, because pdfcpu never runs the rest of that pipeline.
func decodeBounded(sd *pdfcpu_types.StreamDict, limit int64) ([]byte, error) {
	if int64(len(sd.Raw)) > limit {
		return nil, ceilingExceeded(limit)
	}

	// The loader pre-decodes a few stream kinds (XRef streams, object streams,
	// zero-length streams), which need no decode at all.
	if sd.Content != nil {
		if int64(len(sd.Content)) > limit {
			return nil, ceilingExceeded(limit)
		}
		return sd.Content, nil
	}

	if isPassthroughStream(sd) {
		sd.Content = sd.Raw
		return sd.Content, nil
	}

	// A stopping filter ahead of another one leaves the rest of the pipeline
	// unrun, and pdfcpu reads from a nil reader when the stopping filter is
	// first, so the decode is refused rather than attempted.
	for _, f := range sd.FilterPipeline[:len(sd.FilterPipeline)-1] {
		if stopsDecoding(f, sd.CSComponents) {
			return nil, fmt.Errorf("filter %s stops the decoder before the rest of the pipeline runs", f.Name)
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
		return nil, ceilingExceeded(limit)
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
		return ceilingExceeded(limit)
	}
	return nil
}

// cappedDecode runs pdfcpu's size-capped decode, stopping inflation at maxLen
// bytes. It reports errProbeUnusable when the call panics on pdfcpu's unchecked
// data[:maxLen] tail, which safeCall re-panics because the panic is a
// runtime.Error.
func cappedDecode(sd *pdfcpu_types.StreamDict, maxLen int64) (data []byte, err error) {
	defer func() {
		if recover() != nil {
			data, err = nil, errProbeUnusable
		}
	}()
	err = safeCall(func() error {
		var e error
		data, e = sd.DecodeLength(maxLen)
		return e
	})
	return data, err
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
// the payload; ahead of another filter it means nothing downstream is decoded,
// and pdfcpu copies from a nil reader when f is the first filter.
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

// ceilingExceeded builds the rejection for a stream decoding past limit bytes.
func ceilingExceeded(limit int64) error {
	return fmt.Errorf("%w: decoded stream exceeds the %d-byte extraction ceiling", ErrUnsupportedPDF, limit)
}
