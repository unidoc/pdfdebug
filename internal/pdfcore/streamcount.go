package pdfcore

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/ascii85"
	"io"
	"math"

	"github.com/hhrutter/lzw"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// pipelineMeasurement reports whether countStages covered the runnable pipeline
// end to end. measured is true only when every runnable filter was counted;
// firstUnmeasured names the first runnable filter that could not be, and is ""
// when measured. It is a coverage fact kept separate from the ceiling verdict
// the error return carries: a pipeline can be measured and over the ceiling, or
// unmeasured and within it.
type pipelineMeasurement struct {
	measured        bool
	firstUnmeasured string
}

// countStages streams sd.Raw through the filters pdfcpu will run and reports
// ErrUnsupportedPDF when any one stage produces more than limit bytes. Nothing
// is held: each stage is counted through io.Discard, so a payload is measured
// without being allocated.
//
// It also reports measurement COVERAGE. A runnable filter the counter cannot
// size leaves the stages from it onward uncounted, and the returned
// pipelineMeasurement names that filter so the caller can act on the gap rather
// than read an absent count as "measured and in bounds". Coverage is a separate
// axis from the ceiling verdict, and the two are independent: a pipeline may be
// unmeasured yet still convicted on a stage ahead of the gap (a bomb feeding an
// unmodelled filter), or measured and within the ceiling.
//
// Every measured stage is measured in full, not just the terminal one. pdfcpu
// materialises the full output of each filter into a buffer before handing it to
// the next, so an intermediate stage is an allocation of its own and a pipeline
// led by a bomb exceeds the ceiling however small its final output is.
//
// The filters the counter cannot size are JBIG2Decode and 4-component DCTDecode,
// both of which need a real decode. A pipeline led by one of them is measured up
// to it - nothing, when it leads - and reported unmeasured from that filter. A
// predictor FlateDecode or a CCITTFaxDecode stage ends the chain but is itself
// measured; anything after it is reported unmeasured rather than counted at zero.
func countStages(sd *pdfcpu_types.StreamDict, limit int64) (pipelineMeasurement, error) {
	counters, firstUnmeasured := stageCounters(sd, limit)
	m := pipelineMeasurement{measured: firstUnmeasured == "", firstUnmeasured: firstUnmeasured}
	if len(counters) == 0 {
		return m, nil
	}

	// Draining the last stage pulls every stage below it. The rest are then
	// drained in reverse to finish any stage its successor stopped reading
	// early - an ASCIIHexDecode cutting at its '>' or a zlib stream ending
	// before its input does leaves bytes behind that pdfcpu still allocates.
	//
	// Read errors are ignored. The counts are the verdict, and a stream that
	// cannot be decoded reaches the same error in the decode that follows.
	// Ignoring them is also what matches pdfcpu's tolerance for a truncated or
	// checksum-broken zlib stream without having to enumerate it: the count
	// stops where pdfcpu's own output stops.
	for i := len(counters) - 1; i >= 0; i-- {
		_, _ = io.Copy(io.Discard, counters[i])
	}

	for _, c := range counters {
		if c.n > limit {
			return m, decodedCeilingExceeded(limit)
		}
		// A stage that ran out of input mid-run ends the count cleanly, but
		// pdfcpu indexes past its own buffer on the same bytes and the
		// runtime.Error that follows is re-panicked by safeCall. The count has
		// already parsed the stream, so it refuses the shape here rather than
		// handing it to a decoder that crashes on it.
		if f, ok := c.src.(faultingStream); ok && f.faults() {
			return m, errTruncatedRun
		}
	}
	return m, nil
}

// faultingStream is implemented by a stage reader that can tell its input would
// fault pdfcpu's decoder rather than merely end early.
type faultingStream interface {
	faults() bool
}

// stageCounters builds one counting reader per filter stage pdfcpu will run,
// each reading the output of the one before it. The chain stops at the first
// filter the counter cannot size, or after a stage that cannot feed the next
// one. It returns the counters and the name of the first runnable filter left
// unmeasured, "" when the whole runnable pipeline was covered.
func stageCounters(sd *pdfcpu_types.StreamDict, limit int64) ([]*stageCounter, string) {
	var counters []*stageCounter
	var r io.Reader = bytes.NewReader(sd.Raw)

	// /Height is a StreamDict dict entry, not a decode-param, so filterReader
	// cannot reach it. CCITTFaxDecode's /Rows falls back to it, matching pdfcpu's
	// fixParms, so it is read once here and handed down.
	height := 0
	if ip := sd.IntEntry("Height"); ip != nil {
		height = *ip
	}

	runs := runnableFilters(sd)
	for i, f := range runs {
		out, chainContinues := filterReader(r, f, i == len(runs)-1, height)
		if out == nil {
			return counters, f.Name
		}
		// The cap is what keeps the count bounded: a stage stops producing at
		// limit+1 bytes, which is one more than the ceiling allows and so
		// enough to convict it.
		c := &stageCounter{r: io.LimitReader(out, limit+1), src: out}
		// A stage whose output size is known from geometry (CCITTFaxDecode)
		// records its count directly rather than draining a synthetic payload,
		// which on the image path would be an O(limit) pass under the document
		// lock. The count is capped at limit+1, the same bound the drain applies.
		if ks, ok := out.(knownSize); ok {
			c.n = min(ks.size(), limit+1)
		}
		counters = append(counters, c)
		r = c
		// A stage that is counted but cannot feed the next one ends the chain
		// AFTER being added, so its own output is still measured. Anything past
		// it is unmeasured, so it is named rather than counted at zero.
		if !chainContinues {
			if i < len(runs)-1 {
				return counters, runs[i+1].Name
			}
			return counters, ""
		}
	}
	return counters, ""
}

// stageCounter counts the bytes one filter stage produces. src is the filter
// reader itself, kept so the stage can be asked whether its input would fault
// pdfcpu's decoder.
type stageCounter struct {
	r   io.Reader
	src io.Reader
	n   int64
}

func (s *stageCounter) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	s.n += int64(n)
	return n, err
}

// filterModelled classifies every filter name pdfcpu defines. A name maps to
// true when the counter sizes that stage's own output without a real decode, and
// false when it cannot and the stage must be treated as unmeasured. filterReader
// is driven from this map, so a filter pdfcpu adds - or one wired into
// filterReader - that is not classified here cannot reach a nil-returning
// default and silently turn the bound off; the classification test refuses it.
var filterModelled = map[string]bool{
	"ASCII85Decode":   true,
	"ASCIIHexDecode":  true,
	"RunLengthDecode": true,
	"LZWDecode":       true,
	"FlateDecode":     true,
	"CCITTFaxDecode":  true,
	"JBIG2Decode":     false,
	"DCTDecode":       false,
	"JPXDecode":       false,
}

// filterReader returns a reader over the output of f applied to r, and whether
// the chain may continue past this stage. A nil reader means a filter the
// counter does not model, or a CCITTFaxDecode stage it cannot size from its
// parameters; the stage it leads is then reported unmeasured.
//
// Only the byte COUNT has to match what pdfcpu produces, so a stage may yield
// the right number of wrong bytes. That is why a predictor FlateDecode and a
// CCITTFaxDecode stage both end the chain: they hand back a count, not the bytes
// a following filter would read. A predictor FlateDecode as the LAST stage is
// counted at the reconstructed width; anywhere else its bare inflate is counted
// instead, which over-states the rows by 1+1/rowSize and so stays an upper bound
// on both its own output and what it feeds downstream.
//
// A stage that cannot start reads as empty rather than as unmodelled: pdfcpu
// fails on the same input, so the decode below reports the real error instead of
// the counter guessing at one.
func filterReader(r io.Reader, f pdfcpu_types.PDFFilter, last bool, height int) (io.Reader, bool) {
	if !filterModelled[f.Name] {
		return nil, false
	}
	switch f.Name {
	case "FlateDecode":
		zr, err := zlib.NewReader(r)
		if err != nil {
			return bytes.NewReader(nil), true
		}
		if !hasPredictor(f.DecodeParms) {
			return zr, true
		}
		if !last {
			return zr, false
		}
		return predictorRows(zr, f.DecodeParms), true

	case "LZWDecode":
		// pdfcpu refuses a predictor on this filter outright rather than
		// applying one, so the stream produces nothing.
		if predictorValue(f.DecodeParms) > 1 {
			return bytes.NewReader(nil), true
		}
		return lzw.NewReader(r, earlyChange(f.DecodeParms)), true

	case "ASCII85Decode":
		return ascii85.NewDecoder(&ascii85Body{r: r}), true

	case "ASCIIHexDecode":
		return &asciiHexReader{r: bufio.NewReader(r)}, true

	case "RunLengthDecode":
		return &runLengthReader{r: bufio.NewReader(r)}, true

	case "CCITTFaxDecode":
		// The decoded bitmap is sized from geometry, not decoded. The chain ends
		// here because the size stands in for the bytes a following filter would
		// read - measuring that filter over a synthetic payload would under-count
		// it. A stream missing both /Rows and /Height cannot be sized and is left
		// unmeasured.
		n, ok := ccittDecodedSize(f.DecodeParms, height)
		if !ok {
			return nil, false
		}
		return sizedStage{n: n}, false
	}

	// Unreachable: filterModelled admits only the cases above. Kept so the
	// switch is total.
	return nil, false
}

// runnableFilters returns the filters pdfcpu will actually run: the pipeline up
// to the first filter it stops at. Filters at and after that index never run.
func runnableFilters(sd *pdfcpu_types.StreamDict) []pdfcpu_types.PDFFilter {
	if i := stopIndex(sd); i >= 0 {
		return sd.FilterPipeline[:i]
	}
	return sd.FilterPipeline
}

// ascii85Body yields an ASCII85 stream up to its "~>" terminator. pdfcpu strips
// that terminator before decoding and Go's decoder rejects it as illegal input,
// so the cut has to happen here. Bytes after the terminator are dropped where
// pdfcpu rejects the whole stream; the count only has to avoid understating what
// pdfcpu produces, and a stream pdfcpu rejects produces nothing.
type ascii85Body struct {
	r    io.Reader
	done bool
}

func (a *ascii85Body) Read(p []byte) (int, error) {
	if a.done {
		return 0, io.EOF
	}
	n, err := a.r.Read(p)
	if i := bytes.IndexByte(p[:n], '~'); i >= 0 {
		a.done = true
		return i, io.EOF
	}
	return n, err
}

// asciiHexReader decodes an ASCIIHexDecode stream the way pdfcpu does: the
// whitespace pdfcpu skips is skipped, the '>' terminator ends the stream, and a
// trailing half byte is padded with '0'.
type asciiHexReader struct {
	r    *bufio.Reader
	done bool
}

func (a *asciiHexReader) Read(p []byte) (int, error) {
	if len(p) == 0 || a.done {
		return 0, io.EOF
	}
	for i := range p {
		b, ok := a.hexByte()
		if !ok {
			return i, io.EOF
		}
		p[i] = b
	}
	return len(p), nil
}

// hexByte assembles one decoded byte from the next two hex digits, padding a
// lone trailing digit with '0'.
func (a *asciiHexReader) hexByte() (byte, bool) {
	hi, ok := a.hexDigit()
	if !ok {
		return 0, false
	}
	lo, ok := a.hexDigit()
	if !ok {
		lo = 0
	}
	return hi<<4 | lo, true
}

// hexDigit returns the value of the next hex digit, skipping the whitespace
// pdfcpu ignores and stopping at the '>' terminator, at end of input, or at a
// character pdfcpu's decoder would reject.
func (a *asciiHexReader) hexDigit() (byte, bool) {
	for {
		c, err := a.r.ReadByte()
		if err != nil {
			a.done = true
			return 0, false
		}
		switch {
		case c == '>':
			a.done = true
			return 0, false
		case c == 0x09, c == 0x0a, c == 0x0c, c == 0x0d, c == 0x20:
			continue
		case c >= '0' && c <= '9':
			return c - '0', true
		case c >= 'a' && c <= 'f':
			return c - 'a' + 10, true
		case c >= 'A' && c <= 'F':
			return c - 'A' + 10, true
		default:
			a.done = true
			return 0, false
		}
	}
}

// runLengthReader decodes a RunLengthDecode stream: a control byte under 0x80
// introduces that many plus one literal bytes, one above it repeats the next
// byte 257 minus its value times, and 0x80 ends the stream. Input that runs out
// mid-run ends the count and sets truncated, because pdfcpu indexes past its
// own buffer on those bytes.
type runLengthReader struct {
	r         *bufio.Reader
	pending   []byte
	done      bool
	truncated bool
}

// faults reports that pdfcpu's decoder would index past its buffer on this
// input. Running out of input BETWEEN runs is a clean end and does not count;
// only a run whose payload is short does.
func (rl *runLengthReader) faults() bool { return rl.truncated }

func (rl *runLengthReader) Read(p []byte) (int, error) {
	for len(rl.pending) == 0 {
		if rl.done {
			return 0, io.EOF
		}
		rl.fill()
	}
	n := copy(p, rl.pending)
	rl.pending = rl.pending[n:]
	return n, nil
}

// fill decodes the next run into pending, marking the stream done at the 0x80
// terminator or on truncated input.
func (rl *runLengthReader) fill() {
	b, err := rl.r.ReadByte()
	if err != nil || b == 0x80 {
		rl.done = true
		return
	}
	if b < 0x80 {
		run := make([]byte, int(b)+1)
		n, err := io.ReadFull(rl.r, run)
		if err != nil {
			rl.done, rl.truncated = true, true
		}
		rl.pending = run[:n]
		return
	}
	v, err := rl.r.ReadByte()
	if err != nil {
		rl.done, rl.truncated = true, true
		return
	}
	rl.pending = bytes.Repeat([]byte{v}, 257-int(b))
}

// predictorRows reports what pdfcpu's row reconstruction produces from r without
// reconstructing anything. pdfcpu reads whole rows of rowSize bytes, plus one
// row-filter byte on the PNG predictors, and writes rowSize bytes per row, so
// the count is the row width times the number of whole rows. The bytes handed
// back are the raw row rather than the reconstructed one, which is why this may
// only be the last stage of a chain.
func predictorRows(r io.Reader, parms pdfcpu_types.Dict) io.Reader {
	values := map[string]int{"Columns": 1, "Colors": 1, "BitsPerComponent": 8}
	for key := range values {
		if v, found := parms[key]; found {
			if n, ok := predictorParmValue(v); ok {
				values[key] = n
			}
		}
	}

	rowSize := (values["BitsPerComponent"]*values["Colors"]*values["Columns"] + 7) / 8
	// A zero row width would make the reader hand back nothing forever. Callers
	// reject those parameters before they get here; this keeps a direct call from
	// spinning.
	if rowSize < 1 {
		return bytes.NewReader(nil)
	}
	read := rowSize
	// Every predictor but TIFF prefixes each row with a filter byte, which
	// pdfcpu reads and does not write.
	if predictorValue(parms) != predictorTIFF {
		read++
	}
	return &predictorRowReader{r: r, row: make([]byte, read), rowSize: rowSize}
}

// predictorTIFF is the one predictor whose rows carry no leading filter byte.
const predictorTIFF = 2

type predictorRowReader struct {
	r       io.Reader
	row     []byte
	pending []byte
	rowSize int
}

func (p *predictorRowReader) Read(b []byte) (int, error) {
	if len(p.pending) == 0 {
		// A partial row makes pdfcpu fail the whole decode, so it contributes
		// nothing to what the stream produces.
		if _, err := io.ReadFull(p.r, p.row); err != nil {
			return 0, io.EOF
		}
		p.pending = p.row[:p.rowSize]
	}
	n := copy(b, p.pending)
	p.pending = p.pending[n:]
	return n, nil
}

// predictorValue returns the /Predictor of parms, or 1 when the entry is absent
// or a type pdfcpu's parameter flattening discards. 1 is the PDF default and
// means no prediction.
func predictorValue(parms pdfcpu_types.Dict) int {
	v, found := parms["Predictor"]
	if !found {
		return 1
	}
	n, ok := predictorParmValue(v)
	if !ok {
		return 1
	}
	return n
}

// earlyChange reports the /EarlyChange flag pdfcpu passes to the LZW reader,
// which defaults to 1.
func earlyChange(parms pdfcpu_types.Dict) bool {
	v, found := parms["EarlyChange"]
	if !found {
		return true
	}
	n, ok := predictorParmValue(v)
	if !ok {
		return true
	}
	return n == 1
}

// knownSize is implemented by a stage reader whose decoded byte count is known
// without reading it. stageCounters records that count directly instead of
// draining the stage, so a large geometry does not drive an O(limit) pass under
// the document lock.
type knownSize interface{ size() int64 }

// sizedStage stands in for a stage whose decoded size is computed from geometry
// rather than by decoding (CCITTFaxDecode). It yields no bytes; only its size is
// read, so the bitmap is never materialised.
type sizedStage struct{ n int64 }

func (sizedStage) Read([]byte) (int, error) { return 0, io.EOF }

func (s sizedStage) size() int64 { return s.n }

// ccittDecodedSize reports the byte size of the bilevel bitmap pdfcpu's
// CCITTFaxDecode allocates, computed from the decode parameters without decoding.
// The output is one bit per pixel packed to whole bytes per row:
// ceil(Columns/8) * Rows. /Columns defaults to 1728, and /Rows falls back to the
// image /Height when absent, matching pdfcpu's fixParms. /K, /BlackIs1 and
// /EncodedByteAlign change the bits or the encoded input, not the decoded count,
// so they are ignored. pdfcpu errors on /K > 0 rather than decoding, so the count
// for such a stream over-estimates a decode that never runs, which is safe.
//
// ok is false when the geometry cannot be sized: no /Rows and no /Height, zero
// rows, or a non-positive /Columns, all of which leave the stage unmeasured. A
// negative row count instead returns the maximum size so the cap refuses the
// stream before pdfcpu's unbounded auto-detect decode can allocate.
func ccittDecodedSize(parms pdfcpu_types.Dict, height int) (int64, bool) {
	columns := 1728
	if v, ok := parmInt(parms, "Columns"); ok {
		columns = v
	}
	rows, ok := parmInt(parms, "Rows")
	if !ok {
		rows = height
	}
	// A negative /Rows (or negative /Height fallback) is malformed. pdfcpu
	// forwards it to x/image, whose negative-height path auto-detects the height
	// and decodes rows until the data ends - an unbounded decode this geometry
	// cannot size. Report it at the maximum so the cap refuses it before that
	// allocation, not after. Checked ahead of /Columns so a zero or negative
	// /Columns cannot route a negative row count to the unmeasured path.
	if rows < 0 {
		return math.MaxInt64, true
	}
	if columns <= 0 {
		return 0, false
	}
	// Zero rows stays unmeasured: pdfcpu yields nothing for it, so there is no
	// size to convict on.
	if rows == 0 {
		return 0, false
	}
	// A crafted /Columns or /Rows could overflow the int64 product and wrap to a
	// small or negative count that reads as in-bounds - a fail-open. Any bilevel
	// image this large is over every real ceiling, so it is reported at the
	// maximum and left for the cap to refuse. The bound keeps the arithmetic below
	// it clear of overflow: (2^31/8) * 2^31 = 2^59 < 2^63.
	const maxDim int64 = 1 << 31
	if int64(columns) > maxDim || int64(rows) > maxDim {
		return math.MaxInt64, true
	}
	// The row-width and product arithmetic runs in int64 so a large /Columns
	// cannot wrap where int is 32-bit: columns+7 would overflow a 32-bit int
	// before the widening conversion, giving a small or negative count that
	// reads as in-bounds.
	return (int64(columns) + 7) / 8 * int64(rows), true
}

// parmInt reads a decode parameter the way pdfcpu's parameter flattening does:
// an Integer, or a Boolean coerced to 0 or 1. ok is false when the entry is
// absent or a type pdfcpu discards.
func parmInt(parms pdfcpu_types.Dict, key string) (int, bool) {
	v, found := parms[key]
	if !found {
		return 0, false
	}
	return predictorParmValue(v)
}
