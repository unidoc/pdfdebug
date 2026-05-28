package pdfcore

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// objectSourceByteCap caps the reserialized output of GetObjectSource. Past
// this many bytes the serializer truncates at the next top-level boundary
// (between dict pairs / between array elements) and always emits the closing
// bracket plus the `endobj` envelope, so output stays well-formed.
const objectSourceByteCap = 256 * 1024

// shortArrayElemThreshold controls when arrays render on one line vs one
// element per line. Arrays whose length is <= this AND whose elements are all
// simple (scalars / refs / names / literals) render on a single line.
const shortArrayElemThreshold = 4

// streamPlaceholderMarker prefixes lines that the frontend's indirect-ref
// scanner MUST skip. The byte-count placeholder for stream objects is the
// only line carrying this marker; it ships in the rendered output so the
// frontend's regex doesn't turn "12,345 bytes" into a fake `N G R` click
// target. The marker is a zero-width space (​) followed by ASCII '!';
// both are stripped by the frontend before display.
//
// NOTE: keeping the marker out-of-band would require a structured response
// type and ripple through bindings. A line-level marker is the simplest
// solution that meets the contract.
const streamPlaceholderMarker = "\u200b!"

// GetObjectSource returns a reserialized PDF-syntax representation of the
// indirect object identified by nodeID. Inline-node IDs (dict:* / arr:*)
// return ("", nil) -- the frontend renders the AC3 empty state. Stream
// objects render the dict plus a stream/endstream envelope with a byte-count
// placeholder; the placeholder line is marked so the frontend's ref scanner
// skips it.
func (ins *Inspector) GetObjectSource(tabID, nodeID string) (string, error) {
	if nodeID == "" {
		return "", fmt.Errorf("%w: empty node ID", ErrDocumentNotFound)
	}
	if strings.HasPrefix(nodeID, "error:") {
		return "", nil
	}

	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return "", err
	}
	// AC1: serialize pdfcpu access. resolveNodeObject + the writeDict/writeArray
	// walk dereference indirect refs through pdfcpu.
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	// The catalog tree node uses sentinel ID "root" but IS a real indirect
	// object. Map "root" to the catalog's indirect identity (via the trailer's
	// /Root pointer) so its Object Source renders instead of falling through
	// to the AC3 inline empty state. Per AC10 / Dev Notes: the catalog is a
	// real indirect object in the graph; only the trailer->catalog edge is
	// excluded from reverse-refs, not the catalog itself from Object Source.
	var (
		num, gen     int
		resolveID    = nodeID
		isCatalogID  = nodeID == "root"
	)
	if isCatalogID {
		if doc.PDFContext == nil || doc.PDFContext.XRefTable == nil || doc.PDFContext.XRefTable.Root == nil {
			return "", nil
		}
		rootRef := doc.PDFContext.XRefTable.Root
		num = rootRef.ObjectNumber.Value()
		gen = rootRef.GenerationNumber.Value()
		resolveID = fmt.Sprintf("obj:%d:%d", gen, num)
	} else {
		// Object Source is only defined for indirect objects.
		if !strings.HasPrefix(nodeID, "obj:") {
			return "", nil
		}
		kind, parentID, lastPart := parseNodeID(nodeID)
		if kind != "obj" {
			return "", nil
		}
		gen, err = strconv.Atoi(parentID)
		if err != nil {
			return "", fmt.Errorf("invalid gen number in node ID %q: %w", nodeID, err)
		}
		num, err = strconv.Atoi(lastPart)
		if err != nil {
			return "", fmt.Errorf("invalid obj number in node ID %q: %w", nodeID, err)
		}
	}

	var obj pdfcpu_types.Object
	err = safeCall(func() error {
		var e error
		obj, e = resolveNodeObject(doc, resolveID)
		return e
	})
	if err != nil {
		return "", wrapPDFError(err)
	}
	if obj == nil {
		// Indirect ref that did not resolve -- emit the envelope with `null`.
		return fmt.Sprintf("%d %d obj\nnull\nendobj", num, gen), nil
	}

	w := &sourceWriter{cap: objectSourceByteCap}
	w.WriteString(fmt.Sprintf("%d %d obj\n", num, gen))

	switch v := obj.(type) {
	case pdfcpu_types.StreamDict:
		writeStreamObject(w, v.Dict, streamLengthForSource(v))
	case pdfcpu_types.ObjectStreamDict:
		writeStreamObject(w, v.StreamDict.Dict, streamLengthForSource(v.StreamDict))
	case pdfcpu_types.XRefStreamDict:
		writeStreamObject(w, v.StreamDict.Dict, streamLengthForSource(v.StreamDict))
	case pdfcpu_types.Dict:
		writeDict(w, v, 0)
	case pdfcpu_types.Array:
		writeArray(w, v, 0)
	default:
		writeScalar(w, obj)
	}

	// WriteAlways: the cap contract guarantees the endobj envelope ships even
	// when content was truncated mid-serialization.
	w.WriteAlways("\nendobj")
	return w.String(), nil
}

// streamLengthForSource returns the integer byte length to display in the
// stream placeholder. Falls back to 0 when the dict has no /Length or it
// cannot be coerced to an int.
func streamLengthForSource(sd pdfcpu_types.StreamDict) int {
	if sd.StreamLength != nil {
		return int(*sd.StreamLength)
	}
	if v, ok := sd.Dict["Length"]; ok {
		if i, ok := v.(pdfcpu_types.Integer); ok {
			return int(i)
		}
	}
	return 0
}

// sourceWriter accumulates output up to a byte cap. Once truncated, further
// content writes are dropped; closing brackets and envelope text are still
// allowed via WriteAlways so the output stays well-formed. Backed by
// bytes.Buffer (not strings.Builder) so Truncate is available for the
// snapshot/rollback pattern that keeps cap-truncation between entries only.
type sourceWriter struct {
	buf       bytes.Buffer
	cap       int
	truncated bool
}

// WriteString appends s if the buffer is below the cap; once over, it flips
// the truncated flag and drops further content.
func (w *sourceWriter) WriteString(s string) {
	if w.truncated {
		return
	}
	if w.buf.Len()+len(s) > w.cap {
		w.truncated = true
		return
	}
	w.buf.WriteString(s)
}

// WriteAlways appends s regardless of the cap. Used for closing brackets and
// the `endobj` envelope so capped output is still well-formed PDF syntax.
func (w *sourceWriter) WriteAlways(s string) {
	w.buf.WriteString(s)
}

// snapshot returns the current buffer length so a caller can roll back to it
// via rollback. Used to keep cap-truncation strictly between top-level entries
// (Task 1.7 contract: "Truncation never happens mid-entry").
func (w *sourceWriter) snapshot() int {
	return w.buf.Len()
}

// rollback truncates the buffer back to len, discarding any partial entry that
// was written after the snapshot. Used in pair with snapshot when a write
// flips truncated mid-entry.
func (w *sourceWriter) rollback(len int) {
	w.buf.Truncate(len)
}

func (w *sourceWriter) String() string {
	return w.buf.String()
}

// writeStreamObject emits a stream object's dict + stream/endstream envelope
// with a byte-count placeholder. The placeholder line is marked so the
// frontend's indirect-ref scanner skips it. The stream/endstream envelope
// keywords use WriteAlways so a cap-truncated dict still produces well-formed
// output (matches the objectSourceByteCap contract).
func writeStreamObject(w *sourceWriter, d pdfcpu_types.Dict, length int) {
	writeDict(w, d, 0)
	w.WriteAlways("\nstream\n")
	w.WriteAlways(streamPlaceholderMarker)
	w.WriteAlways(fmt.Sprintf("[%s bytes -- see Content Stream tab for decoded view]\n", formatIntWithCommas(length)))
	w.WriteAlways("endstream")
}

// formatIntWithCommas renders an int with thousands separators (12345 ->
// "12,345"). Used in the stream byte-count placeholder. Operates on the
// absolute-value digit string so math.MinInt does not recurse-and-overflow
// (where -n wraps back to MinInt).
func formatIntWithCommas(n int) string {
	s := strconv.Itoa(n)
	negative := strings.HasPrefix(s, "-")
	digits := s
	if negative {
		digits = s[1:]
	}
	if len(digits) <= 3 {
		return s
	}
	// Insert commas every three digits from the right of the digit run.
	var b strings.Builder
	if negative {
		b.WriteString("-")
	}
	first := len(digits) % 3
	if first > 0 {
		b.WriteString(digits[:first])
		if len(digits) > first {
			b.WriteString(",")
		}
	}
	for i := first; i < len(digits); i += 3 {
		b.WriteString(digits[i : i+3])
		if i+3 < len(digits) {
			b.WriteString(",")
		}
	}
	return b.String()
}

// writeDict serializes a dict at the given indent level. Entries iterate in
// sorted key order for determinism. Indirect refs are NEVER followed -- they
// emit as `N G R` literals (this is what prevents the page tree's /Parent
// cycle from stack-overflowing).
func writeDict(w *sourceWriter, d pdfcpu_types.Dict, depth int) {
	indent := strings.Repeat("    ", depth)
	inner := strings.Repeat("    ", depth+1)
	// WriteAlways: pair with the closing `>>` so a cap-truncated parent never
	// strands a `>>` without its matching `<<`.
	w.WriteAlways("<<\n")

	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	emittedCount := 0
	truncatedAt := -1
	for i, k := range keys {
		if w.truncated {
			truncatedAt = i
			break
		}
		// Snapshot before the entry so we can roll back a partially-written
		// entry if the cap is hit mid-write. Task 1.7: "Truncation never
		// happens mid-entry."
		mark := w.snapshot()
		w.WriteString(inner)
		w.WriteString("/")
		w.WriteString(k)
		w.WriteString(" ")
		writeValue(w, d[k], depth+1)
		w.WriteString("\n")
		if w.truncated {
			w.rollback(mark)
			truncatedAt = i
			break
		}
		emittedCount++
	}
	// Emit the truncation marker whenever at least one entry was dropped,
	// including the truncatedAt == 0 case (cap exceeded before the first
	// entry could be written).
	if truncatedAt >= 0 && truncatedAt < len(keys) {
		w.WriteAlways(inner)
		w.WriteAlways(fmt.Sprintf("... [truncated, %d more entries]\n", len(keys)-emittedCount))
	}
	w.WriteAlways(indent)
	w.WriteAlways(">>")
}

// writeArray serializes an array. Short arrays (length <= threshold, all
// simple elements) render on one line: `[ V V V ]`. Long arrays render one
// element per line with hanging indentation, matching the AC1 example.
func writeArray(w *sourceWriter, arr pdfcpu_types.Array, depth int) {
	if isShortSimpleArray(arr) {
		// WriteAlways for the brackets; pair with the closing `]` so the
		// short form is well-formed even when cap-truncated mid-element.
		w.WriteAlways("[")
		for _, elem := range arr {
			// Snapshot-per-element keeps mid-element truncation from corrupting
			// the short form; if a write fails mid-element we drop that element
			// rather than emitting a half-written scalar.
			mark := w.snapshot()
			w.WriteString(" ")
			writeValue(w, elem, depth)
			if w.truncated {
				w.rollback(mark)
				break
			}
		}
		w.WriteAlways(" ]")
		return
	}
	indent := strings.Repeat("    ", depth)
	inner := strings.Repeat("    ", depth+1)
	// WriteAlways: pair with the closing `]` so a cap-truncated parent never
	// strands a `]` without its matching `[`.
	w.WriteAlways("[\n")
	emitted := 0
	truncatedAt := -1
	for i, elem := range arr {
		if w.truncated {
			truncatedAt = i
			break
		}
		// Snapshot before the element so we can roll back a partially-written
		// element if the cap is hit mid-write. Task 1.7: "Truncation never
		// happens mid-entry."
		mark := w.snapshot()
		w.WriteString(inner)
		writeValue(w, elem, depth+1)
		w.WriteString("\n")
		if w.truncated {
			w.rollback(mark)
			truncatedAt = i
			break
		}
		emitted++
	}
	// Emit the truncation marker whenever at least one element was dropped,
	// including the truncatedAt == 0 case (cap exceeded before the first
	// element could be written).
	if truncatedAt >= 0 && truncatedAt < len(arr) {
		w.WriteAlways(inner)
		w.WriteAlways(fmt.Sprintf("... [truncated, %d more entries]\n", len(arr)-emitted))
	}
	w.WriteAlways(indent)
	w.WriteAlways("]")
}

// isShortSimpleArray reports whether arr fits the short-array single-line
// form: length <= threshold AND every element is a scalar/ref/name (no
// nested dicts/arrays).
func isShortSimpleArray(arr pdfcpu_types.Array) bool {
	if len(arr) > shortArrayElemThreshold {
		return false
	}
	for _, elem := range arr {
		if isContainer(elem) {
			return false
		}
	}
	return true
}

// writeValue dispatches one value to the appropriate serializer. Containers
// recurse; scalars / refs render verbatim.
func writeValue(w *sourceWriter, val pdfcpu_types.Object, depth int) {
	switch v := val.(type) {
	case pdfcpu_types.Dict:
		writeDict(w, v, depth)
	case pdfcpu_types.StreamDict:
		// Inline stream encountered as a value -- emit the dict only; stream
		// markers belong at the top level (see GetObjectSource).
		writeDict(w, v.Dict, depth)
	case pdfcpu_types.ObjectStreamDict:
		writeDict(w, v.StreamDict.Dict, depth)
	case pdfcpu_types.XRefStreamDict:
		writeDict(w, v.StreamDict.Dict, depth)
	case pdfcpu_types.Array:
		writeArray(w, v, depth)
	case pdfcpu_types.IndirectRef:
		w.WriteString(fmt.Sprintf("%d %d R", v.ObjectNumber.Value(), v.GenerationNumber.Value()))
	default:
		writeScalar(w, val)
	}
}

// writeScalar renders a scalar value in canonical PDF syntax.
func writeScalar(w *sourceWriter, val pdfcpu_types.Object) {
	switch v := val.(type) {
	case pdfcpu_types.Name:
		w.WriteString("/")
		w.WriteString(string(v))
	case pdfcpu_types.StringLiteral:
		w.WriteString("(")
		w.WriteString(string(v))
		w.WriteString(")")
	case pdfcpu_types.HexLiteral:
		w.WriteString("<")
		w.WriteString(string(v))
		w.WriteString(">")
	case pdfcpu_types.Integer:
		w.WriteString(strconv.Itoa(int(v)))
	case pdfcpu_types.Float:
		w.WriteString(strconv.FormatFloat(float64(v), 'f', -1, 64))
	case pdfcpu_types.Boolean:
		if bool(v) {
			w.WriteString("true")
		} else {
			w.WriteString("false")
		}
	case nil:
		w.WriteString("null")
	default:
		w.WriteString("Unknown")
	}
}
