package pdfcore

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/ascii85"
	"io"

	"github.com/hhrutter/lzw"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// countStages streams sd.Raw through the filters pdfcpu will run and reports
// ErrUnsupportedPDF when any one stage produces more than limit bytes. Nothing
// is held: each stage is counted through io.Discard, so a payload is measured
// without being allocated.
//
// Every stage is measured, not just the terminal one. pdfcpu materialises the
// full output of each filter into a buffer before handing it to the next, so an
// intermediate stage is an allocation of its own and a pipeline led by a bomb
// exceeds the ceiling however small its final output is.
//
// A pipeline carrying a filter the counter does not model - CCITTFaxDecode,
// 4-component DCTDecode, JBIG2Decode - is measured up to that filter and no
// further, leaving that stage's own expansion to the post-decode check.
func countStages(sd *pdfcpu_types.StreamDict, limit int64) error {
	counters := stageCounters(sd, limit)
	if len(counters) == 0 {
		return nil
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
			return decodedCeilingExceeded(limit)
		}
	}
	return nil
}

// stageCounters builds one counting reader per filter stage pdfcpu will run,
// each reading the output of the one before it. The chain stops at the first
// filter the counter does not model.
func stageCounters(sd *pdfcpu_types.StreamDict, limit int64) []*stageCounter {
	var counters []*stageCounter
	var r io.Reader = bytes.NewReader(sd.Raw)

	runs := runnableFilters(sd)
	for i, f := range runs {
		out := filterReader(r, f, i == len(runs)-1)
		if out == nil {
			break
		}
		// The cap is what keeps the count bounded: a stage stops producing at
		// limit+1 bytes, which is one more than the ceiling allows and so
		// enough to convict it.
		c := &stageCounter{r: io.LimitReader(out, limit+1)}
		counters = append(counters, c)
		r = c
	}
	return counters
}

// stageCounter counts the bytes one filter stage produces.
type stageCounter struct {
	r io.Reader
	n int64
}

func (s *stageCounter) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	s.n += int64(n)
	return n, err
}

// filterReader returns a reader over the output of f applied to r, or nil for a
// filter the counter does not model.
//
// Only the byte COUNT has to match what pdfcpu produces. A stage may therefore
// yield the right number of wrong bytes, which is why the predictor path is
// modelled only as the last stage - the stages before it feed a filter that
// reads the values.
//
// A stage that cannot start reads as empty rather than as unmodelled: pdfcpu
// fails on the same input, so the decode below reports the real error instead of
// the counter guessing at one.
func filterReader(r io.Reader, f pdfcpu_types.PDFFilter, last bool) io.Reader {
	switch f.Name {
	case "FlateDecode":
		zr, err := zlib.NewReader(r)
		if err != nil {
			return bytes.NewReader(nil)
		}
		if !hasPredictor(f.DecodeParms) {
			return zr
		}
		if !last {
			return nil
		}
		return predictorRows(zr, f.DecodeParms)

	case "LZWDecode":
		// pdfcpu refuses a predictor on this filter outright rather than
		// applying one, so the stream produces nothing.
		if predictorValue(f.DecodeParms) > 1 {
			return bytes.NewReader(nil)
		}
		return lzw.NewReader(r, earlyChange(f.DecodeParms))

	case "ASCII85Decode":
		return ascii85.NewDecoder(&ascii85Body{r: r})

	case "ASCIIHexDecode":
		return &asciiHexReader{r: bufio.NewReader(r)}

	case "RunLengthDecode":
		return &runLengthReader{r: bufio.NewReader(r)}

	default:
		return nil
	}
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
// mid-run ends the stream, where pdfcpu indexes past its buffer and panics.
type runLengthReader struct {
	r       *bufio.Reader
	pending []byte
	done    bool
}

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
			rl.done = true
		}
		rl.pending = run[:n]
		return
	}
	v, err := rl.r.ReadByte()
	if err != nil {
		rl.done = true
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
