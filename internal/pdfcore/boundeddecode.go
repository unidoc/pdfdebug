package pdfcore

import (
	"errors"
	"fmt"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

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
// The size is measured before it is allocated. countStages streams sd.Raw
// through the filters pdfcpu will run and counts what each one produces through
// io.Discard, so a highly compressible payload is rejected without ever being
// held. An in-bounds stream is then decoded for real, which by then is proven to
// stay under the ceiling. Every stage is measured, not only the terminal one:
// pdfcpu buffers the full output of each filter before running the next, so an
// intermediate stage is an allocation of its own.
//
// The one shape still measured after the fact is a pipeline carrying a filter
// the counter does not model - CCITTFaxDecode, 4-component DCTDecode,
// JBIG2Decode. Those are counted up to that filter, so what feeds it is
// bounded, and its own expansion is left to the post-decode check.
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
	// The check runs before the count because it also bounds an allocation the
	// count cannot see: pdfcpu sizes two row buffers from these parameters
	// before reading a single byte.
	//
	// Only the filters pdfcpu actually runs are checked, and only FlateDecode
	// among them: it is the sole filter calling decodePostProcess, which is the
	// row reconstruction these parameters drive. LZWDecode reads /Predictor only
	// to reject it outright, and every other filter ignores the entry, so
	// refusing on their parameters rejects a decode that runs.
	for _, f := range runnableFilters(sd) {
		if f.Name != "FlateDecode" || !hasPredictor(f.DecodeParms) {
			continue
		}
		if err := checkPredictorParms(f.DecodeParms, limit); err != nil {
			return nil, err
		}
	}

	if err := countStages(sd, limit); err != nil {
		return nil, err
	}

	if err := safeCall(func() error { return sd.Decode() }); err != nil {
		return nil, err
	}
	if int64(len(sd.Content)) > limit {
		return nil, decodedCeilingExceeded(limit)
	}
	return sd.Content, nil
}

// isPassthroughStream reports whether pdfcpu hands sd.Raw back unchanged
// instead of running a filter over it: no filters at all, or a sole stopping
// filter. An empty-but-non-nil pipeline reaches pdfcpu's decoder with a nil
// reader.
func isPassthroughStream(sd *pdfcpu_types.StreamDict) bool {
	if len(sd.FilterPipeline) == 0 {
		return true
	}
	return len(sd.FilterPipeline) == 1 && stopsDecoding(sd.FilterPipeline[0], sd.CSComponents)
}

// stopIndex returns the index of the first filter pdfcpu stops at, or -1 when
// there is none. Filters at and after that index never run.
func stopIndex(sd *pdfcpu_types.StreamDict) int {
	for i, f := range sd.FilterPipeline {
		if stopsDecoding(f, sd.CSComponents) {
			return i
		}
	}
	return -1
}

// stopsDecoding reports whether pdfcpu breaks out of its filter loop at f
// rather than running it: JPXDecode always, DCTDecode unless the stream is
// 4-component. As the sole filter of a pipeline that means the raw bytes are
// the payload; as the first of several it means pdfcpu copies from a nil
// reader.
func stopsDecoding(f pdfcpu_types.PDFFilter, csComponents int) bool {
	return f.Name == "JPXDecode" || (f.Name == "DCTDecode" && csComponents != 4)
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
	// caller asked for. Both rows are what has to fit, not one: checking a single
	// row still lets a 29-byte stream allocate 100 MB against a 50 MB ceiling.
	rowBytes := int64(values["BitsPerComponent"]*values["Colors"]*values["Columns"]+7) / 8
	if 2*rowBytes > limit {
		return fmt.Errorf("%w: two %d-byte predictor rows exceed the %d-byte ceiling",
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
	return predictorValue(parms) > 1
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
