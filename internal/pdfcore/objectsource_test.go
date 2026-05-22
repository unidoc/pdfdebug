package pdfcore

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// helper to construct an Inspector + open a fixture PDF and return the doc.
func openFixture(t *testing.T, name string) (*Inspector, string) {
	t.Helper()
	ins := NewInspector()
	tabID := "tab-os-" + name
	_, err := ins.Open(tabID, filepath.Join(testdataDir(t), name))
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	return ins, tabID
}

func TestObjectSourceMinimalCatalog(t *testing.T) {
	ins, tabID := openFixture(t, "minimal.pdf")
	src, err := ins.GetObjectSource(tabID, "obj:0:1")
	if err != nil {
		t.Fatalf("GetObjectSource: %v", err)
	}
	if !strings.HasPrefix(src, "1 0 obj\n") {
		t.Errorf("expected envelope to start with '1 0 obj', got %q", src)
	}
	if !strings.HasSuffix(src, "\nendobj") {
		t.Errorf("expected envelope to end with 'endobj', got %q", src)
	}
	if !strings.Contains(src, "/Type /Catalog") {
		t.Errorf("expected /Type /Catalog, got %q", src)
	}
	if !strings.Contains(src, "/Pages 2 0 R") {
		t.Errorf("expected /Pages 2 0 R, got %q", src)
	}
}

func TestObjectSourceDictPagesNode(t *testing.T) {
	ins, tabID := openFixture(t, "multipage.pdf")
	src, err := ins.GetObjectSource(tabID, "obj:0:2")
	if err != nil {
		t.Fatalf("GetObjectSource: %v", err)
	}
	// Pages node has /Kids with 3 elements -- short-array threshold is 4 so
	// the array renders on one line.
	if !strings.Contains(src, "/Type /Pages") {
		t.Errorf("expected /Type /Pages, got %q", src)
	}
	if !strings.Contains(src, "/Count 3") {
		t.Errorf("expected /Count 3, got %q", src)
	}
	if !strings.Contains(src, "[ 3 0 R 4 0 R 5 0 R ]") {
		t.Errorf("expected short-form Kids array, got %q", src)
	}
}

// TestObjectSourceCycleSafe verifies a page tree /Parent cycle does not
// cause stack overflow during serialization: refs emit as `N G R` literals
// rather than being followed. Name pinned by integration test
// 9.10-INTG-004 (TestObjectSourceCycleSafe).
func TestObjectSourceCycleSafe(t *testing.T) {
	ins, tabID := openFixture(t, "multipage.pdf")
	src, err := ins.GetObjectSource(tabID, "obj:0:3")
	if err != nil {
		t.Fatalf("GetObjectSource: %v", err)
	}
	if !strings.Contains(src, "/Parent 2 0 R") {
		t.Errorf("expected /Parent emitted as 2 0 R (not dereferenced), got %q", src)
	}
}

func TestObjectSourceStreamPlaceholder(t *testing.T) {
	// content-stream.pdf has stream object 4 0 R with /Length and the encoded
	// bytes; the serializer must render dict + envelope + byte placeholder.
	ins, tabID := openFixture(t, "content-stream.pdf")
	src, err := ins.GetObjectSource(tabID, "obj:0:4")
	if err != nil {
		t.Fatalf("GetObjectSource: %v", err)
	}
	if !strings.Contains(src, "stream\n") {
		t.Errorf("expected stream marker, got %q", src)
	}
	if !strings.Contains(src, "endstream") {
		t.Errorf("expected endstream marker, got %q", src)
	}
	if !strings.Contains(src, "bytes -- see Content Stream tab for decoded view") {
		t.Errorf("expected stream byte-count placeholder copy, got %q", src)
	}
	// Placeholder line must carry the suppression marker for the frontend
	// scanner. The marker is a zero-width space + '!'.
	if !strings.Contains(src, streamPlaceholderMarker) {
		t.Errorf("expected stream placeholder marker, got %q", src)
	}
}

// TestObjectSourceInlineNodeIDReturnsEmptyString verifies inline-node IDs
// (dict:* / arr:*) return ("", nil) so the frontend renders the AC3 empty
// state. Name pinned by integration test 9.10-INTG-006.
func TestObjectSourceInlineNodeIDReturnsEmptyString(t *testing.T) {
	ins, tabID := openFixture(t, "minimal.pdf")
	src, err := ins.GetObjectSource(tabID, "dict:obj:0:1:Type")
	if err != nil {
		t.Fatalf("GetObjectSource (inline): %v", err)
	}
	if src != "" {
		t.Errorf("expected empty string for inline node, got %q", src)
	}
}

func TestObjectSourceUnknownNodeReturnsEmpty(t *testing.T) {
	ins, tabID := openFixture(t, "minimal.pdf")
	src, err := ins.GetObjectSource(tabID, "error:something")
	if err != nil {
		t.Fatalf("GetObjectSource (error node): %v", err)
	}
	if src != "" {
		t.Errorf("expected empty string for error node, got %q", src)
	}
}

// TestObjectSourceCatalogRootNodeID verifies the catalog tree node (sentinel
// nodeID "root") resolves to the catalog's real indirect object and emits its
// Object Source. Without this mapping the catalog selection would fall through
// to the AC3 inline empty state, hiding the source for a node that is in fact
// an indirect object (AC10 / Dev Notes: catalog is a real indirect object).
func TestObjectSourceCatalogRootNodeID(t *testing.T) {
	ins, tabID := openFixture(t, "minimal.pdf")
	src, err := ins.GetObjectSource(tabID, "root")
	if err != nil {
		t.Fatalf("GetObjectSource(root): %v", err)
	}
	if src == "" {
		t.Fatalf("expected catalog source for root nodeID, got empty string")
	}
	// minimal.pdf's catalog is 1 0 obj. The envelope must use its real
	// indirect identity, not the sentinel.
	if !strings.HasPrefix(src, "1 0 obj\n") {
		t.Errorf("expected envelope 1 0 obj, got %q", src)
	}
	if !strings.HasSuffix(src, "\nendobj") {
		t.Errorf("expected envelope to end with endobj, got %q", src)
	}
	if !strings.Contains(src, "/Type /Catalog") {
		t.Errorf("expected /Type /Catalog in catalog source, got %q", src)
	}
}

// TestObjectSourceTruncationNoMidEntry verifies the snapshot/rollback
// contract: when the cap is exhausted mid-entry, the partial entry is rolled
// back rather than left half-written. Task 1.7: "Truncation never happens
// mid-entry. This guarantees golden-file determinism and well-formed output
// even when capped."
func TestObjectSourceTruncationNoMidEntry(t *testing.T) {
	// Long-form array of names that are each 8 chars (e.g. "/NameAAA"). Pick
	// a cap that lands inside writing the third element.
	arr := pdfcpu_types.Array{
		pdfcpu_types.Name("AAAAAAAA"),
		pdfcpu_types.Name("BBBBBBBB"),
		pdfcpu_types.Name("CCCCCCCC"),
		pdfcpu_types.Name("DDDDDDDD"),
		pdfcpu_types.Name("EEEEEEEE"),
	}
	// Force long form by overriding the short-array threshold via length > 4.
	w := &sourceWriter{cap: 25} // small enough to trigger mid-entry on element 2+
	writeArray(w, arr, 0)
	got := w.String()
	// Must close the bracket regardless of cap.
	if !strings.HasSuffix(got, "]") {
		t.Errorf("array must close with ], got tail %q", got)
	}
	// Must NOT contain a half-written name (e.g. "/CCCC" without the rest).
	// Each fully-emitted name is 9 bytes: "/" + 8 alpha chars. Scan for any
	// "/X*" sequence and confirm it has length 9.
	for _, line := range strings.Split(got, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "/") && trimmed != "/" {
			if len(trimmed) != 9 {
				t.Errorf("found mid-entry partial name %q (expected len 9)", trimmed)
			}
		}
	}
}

func TestObjectSourceShortArrayInline(t *testing.T) {
	// MediaBox on page 3 is [0 0 612 792] -- 4 simple elements, single line.
	ins, tabID := openFixture(t, "minimal.pdf")
	src, err := ins.GetObjectSource(tabID, "obj:0:3")
	if err != nil {
		t.Fatalf("GetObjectSource: %v", err)
	}
	if !strings.Contains(src, "/MediaBox [ 0 0 612 792 ]") {
		t.Errorf("expected MediaBox short form, got %q", src)
	}
}

func TestObjectSourceLongArrayMultiline(t *testing.T) {
	// Build a synthetic long array using a serializer-only test: we exercise
	// writeArray directly because no fixture has a >4 element array. This is
	// a unit-level assertion on the serializer's branching, not on pdfcpu.
	arr := pdfcpu_types.Array{
		pdfcpu_types.Integer(1),
		pdfcpu_types.Integer(2),
		pdfcpu_types.Integer(3),
		pdfcpu_types.Integer(4),
		pdfcpu_types.Integer(5),
	}
	w := &sourceWriter{cap: objectSourceByteCap}
	writeArray(w, arr, 0)
	got := w.String()
	if !strings.Contains(got, "[\n") {
		t.Errorf("long array must open with [\\n, got %q", got)
	}
	if !strings.Contains(got, "\n]") {
		t.Errorf("long array must close with \\n], got %q", got)
	}
	for i := 1; i <= 5; i++ {
		if !strings.Contains(got, fmt.Sprintf("    %d\n", i)) {
			t.Errorf("expected element %d on its own indented line, got %q", i, got)
		}
	}
}

// TestObjectSourceTruncationRule verifies the truncation rule: output capped
// at the byte limit, only between top-level entries, always emits closing
// bracket AND the truncation marker. Name pinned by integration test
// 9.10-INTG-008.
func TestObjectSourceTruncationRule(t *testing.T) {
	w := &sourceWriter{cap: 200} // very low cap to force truncation quickly
	arr := make(pdfcpu_types.Array, 200)
	for i := range arr {
		arr[i] = pdfcpu_types.Integer(i)
	}
	writeArray(w, arr, 0)
	got := w.String()
	if !strings.Contains(got, "... [truncated, ") {
		t.Errorf("expected truncation marker, got %q", got)
	}
	if !strings.HasSuffix(got, "]") {
		t.Errorf("truncated array must still close with ], got tail %q", got[max(0, len(got)-40):])
	}
}

func TestObjectSourceScalarTypes(t *testing.T) {
	cases := []struct {
		val  pdfcpu_types.Object
		want string
	}{
		{pdfcpu_types.Name("Foo"), "/Foo"},
		{pdfcpu_types.StringLiteral("hello"), "(hello)"},
		{pdfcpu_types.HexLiteral("ABCD"), "<ABCD>"},
		{pdfcpu_types.Integer(42), "42"},
		{pdfcpu_types.Float(3.14), "3.14"},
		{pdfcpu_types.Boolean(true), "true"},
		{pdfcpu_types.Boolean(false), "false"},
		{nil, "null"},
	}
	for _, c := range cases {
		w := &sourceWriter{cap: 1024}
		writeScalar(w, c.val)
		got := w.String()
		if got != c.want {
			t.Errorf("writeScalar(%v) = %q, want %q", c.val, got, c.want)
		}
	}
}

// TestObjectSourceRefsEmittedNotDereferenced verifies indirect refs in
// serialized output emit as `N G R` literals (NOT followed). Name pinned by
// integration test 9.10-INTG-003.
func TestObjectSourceRefsEmittedNotDereferenced(t *testing.T) {
	w := &sourceWriter{cap: 1024}
	ref := pdfcpu_types.IndirectRef{
		ObjectNumber:     pdfcpu_types.Integer(5),
		GenerationNumber: pdfcpu_types.Integer(0),
	}
	writeValue(w, ref, 0)
	got := w.String()
	if got != "5 0 R" {
		t.Errorf("indirect ref must emit verbatim, got %q", got)
	}
}

// TestObjectSourceSerializeForms covers AC1: dict short/long forms, array
// short/long forms, and inline nesting. Aggregates the dict/array assertions
// from the multipage and minimal fixtures. Name pinned by integration test
// 9.10-INTG-002.
func TestObjectSourceSerializeForms(t *testing.T) {
	t.Run("short_array", func(t *testing.T) {
		ins, tabID := openFixture(t, "multipage.pdf")
		src, err := ins.GetObjectSource(tabID, "obj:0:2")
		if err != nil {
			t.Fatalf("GetObjectSource: %v", err)
		}
		if !strings.Contains(src, "[ 3 0 R 4 0 R 5 0 R ]") {
			t.Errorf("expected short-form Kids array, got %q", src)
		}
	})
	t.Run("dict_form", func(t *testing.T) {
		ins, tabID := openFixture(t, "minimal.pdf")
		src, err := ins.GetObjectSource(tabID, "obj:0:1")
		if err != nil {
			t.Fatalf("GetObjectSource: %v", err)
		}
		if !strings.HasPrefix(src, "1 0 obj\n") || !strings.HasSuffix(src, "\nendobj") {
			t.Errorf("envelope wrong, got %q", src)
		}
		if !strings.Contains(src, "<<\n") || !strings.Contains(src, "\n>>") {
			t.Errorf("dict markers missing, got %q", src)
		}
	})
	t.Run("long_array", func(t *testing.T) {
		arr := pdfcpu_types.Array{
			pdfcpu_types.Integer(1),
			pdfcpu_types.Integer(2),
			pdfcpu_types.Integer(3),
			pdfcpu_types.Integer(4),
			pdfcpu_types.Integer(5),
		}
		w := &sourceWriter{cap: objectSourceByteCap}
		writeArray(w, arr, 0)
		got := w.String()
		if !strings.Contains(got, "[\n") || !strings.Contains(got, "\n]") {
			t.Errorf("long array form missing markers, got %q", got)
		}
	})
}

// TestObjectSourceDictKeysSorted verifies dict entries are emitted in sorted
// key order so the output is deterministic (Go map iteration is random). Name
// pinned by integration test 9.10-INTG-005.
func TestObjectSourceDictKeysSorted(t *testing.T) {
	d := pdfcpu_types.Dict{
		"Zebra": pdfcpu_types.Integer(1),
		"Alpha": pdfcpu_types.Integer(2),
		"Middle": pdfcpu_types.Integer(3),
	}
	w := &sourceWriter{cap: objectSourceByteCap}
	writeDict(w, d, 0)
	got := w.String()
	aIdx := strings.Index(got, "/Alpha")
	mIdx := strings.Index(got, "/Middle")
	zIdx := strings.Index(got, "/Zebra")
	if aIdx < 0 || mIdx < 0 || zIdx < 0 {
		t.Fatalf("expected all three keys, got %q", got)
	}
	if aIdx >= mIdx || mIdx >= zIdx {
		t.Errorf("expected keys in ascending order Alpha < Middle < Zebra, got positions a=%d m=%d z=%d in %q", aIdx, mIdx, zIdx, got)
	}
}

// TestFormatIntWithCommas covers the comma-grouping helper used by the stream
// byte-count placeholder. Includes the math.MinInt regression case called out
// in Review-1 ("formatIntWithCommas(math.MinInt) infinite-recursed because
// -MinInt wraps to MinInt on int64") plus the boundary widths the production
// callers actually hit (0, 1-3 digits, 4 digits, 7 digits, signed forms).
func TestFormatIntWithCommas(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1,000"},
		{1234, "1,234"},
		{12345, "12,345"},
		{123456, "123,456"},
		{1234567, "1,234,567"},
		{-1, "-1"},
		{-1000, "-1,000"},
		{-123456, "-123,456"},
		// math.MinInt must not recurse-and-overflow (Review-1 regression).
		{math.MinInt, strconv.Itoa(math.MinInt)[:1] + insertCommasForTest(strconv.Itoa(math.MinInt)[1:])},
	}
	for _, c := range cases {
		got := formatIntWithCommas(c.in)
		if got != c.want {
			t.Errorf("formatIntWithCommas(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

// insertCommasForTest mirrors the helper's positive-path grouping so the
// MinInt assertion above stays platform-portable (math.MinInt differs between
// 32-bit and 64-bit builds).
func insertCommasForTest(digits string) string {
	if len(digits) <= 3 {
		return digits
	}
	first := len(digits) % 3
	var b strings.Builder
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

// TestStreamLengthForSource covers the three precedence paths of the byte
// length resolver: explicit StreamLength pointer, dict /Length Integer
// fallback, and the zero default when neither is set.
func TestStreamLengthForSource(t *testing.T) {
	t.Run("uses StreamLength pointer when set", func(t *testing.T) {
		var n int64 = 4242
		sd := pdfcpu_types.StreamDict{
			Dict:         pdfcpu_types.Dict{"Length": pdfcpu_types.Integer(7)},
			StreamLength: &n,
		}
		if got := streamLengthForSource(sd); got != 4242 {
			t.Errorf("StreamLength path: got %d, want 4242", got)
		}
	})
	t.Run("falls back to /Length Integer when StreamLength nil", func(t *testing.T) {
		sd := pdfcpu_types.StreamDict{
			Dict: pdfcpu_types.Dict{"Length": pdfcpu_types.Integer(99)},
		}
		if got := streamLengthForSource(sd); got != 99 {
			t.Errorf("/Length fallback: got %d, want 99", got)
		}
	})
	t.Run("returns 0 when neither is available", func(t *testing.T) {
		sd := pdfcpu_types.StreamDict{Dict: pdfcpu_types.Dict{}}
		if got := streamLengthForSource(sd); got != 0 {
			t.Errorf("zero default: got %d, want 0", got)
		}
	})
	t.Run("returns 0 when /Length is non-Integer", func(t *testing.T) {
		sd := pdfcpu_types.StreamDict{
			Dict: pdfcpu_types.Dict{"Length": pdfcpu_types.Name("NotANumber")},
		}
		if got := streamLengthForSource(sd); got != 0 {
			t.Errorf("non-Integer /Length: got %d, want 0", got)
		}
	})
}

// TestGetObjectSourceEmptyNodeIDReturnsError verifies the entry-guard at the
// top of GetObjectSource: an empty nodeID rejects with ErrDocumentNotFound
// rather than silently returning "". This protects callers from accidentally
// rendering a blank panel when the selection state is misconfigured.
func TestGetObjectSourceEmptyNodeIDReturnsError(t *testing.T) {
	ins, tabID := openFixture(t, "minimal.pdf")
	_, err := ins.GetObjectSource(tabID, "")
	if err == nil {
		t.Fatalf("expected error for empty nodeID, got nil")
	}
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

// TestGetObjectSourceUnknownTab verifies that an unknown tab ID produces an
// error rather than a partial result; the catalog-resolution branch and the
// indirect-object branch both depend on a valid document handle.
func TestGetObjectSourceUnknownTab(t *testing.T) {
	ins := NewInspector()
	if _, err := ins.GetObjectSource("no-such-tab", "obj:0:1"); err == nil {
		t.Errorf("expected error for unknown tab, got nil")
	}
	if _, err := ins.GetObjectSource("no-such-tab", "root"); err == nil {
		t.Errorf("expected error for unknown tab on root nodeID, got nil")
	}
}

// TestObjectSourceMalformedNodeIDNumbers verifies the strconv guards on the
// gen/num parsing path: non-numeric components return a wrapped error so the
// frontend sees a meaningful rejection rather than a successful empty result.
func TestObjectSourceMalformedNodeIDNumbers(t *testing.T) {
	ins, tabID := openFixture(t, "minimal.pdf")
	if _, err := ins.GetObjectSource(tabID, "obj:abc:1"); err == nil {
		t.Errorf("expected error for non-numeric gen, got nil")
	}
	if _, err := ins.GetObjectSource(tabID, "obj:0:xyz"); err == nil {
		t.Errorf("expected error for non-numeric num, got nil")
	}
}

// TestWriteStreamObjectWithCapTruncatedDict verifies the WriteAlways contract:
// even when the dict body is cap-truncated, the stream/endstream envelope
// markers ship unconditionally so the output stays well-formed. The story's
// Risk R6 and the writeStreamObject comment depend on this invariant.
func TestWriteStreamObjectWithCapTruncatedDict(t *testing.T) {
	d := pdfcpu_types.Dict{
		"AAAAAAAA": pdfcpu_types.Integer(1),
		"BBBBBBBB": pdfcpu_types.Integer(2),
		"CCCCCCCC": pdfcpu_types.Integer(3),
		"DDDDDDDD": pdfcpu_types.Integer(4),
	}
	// Cap is small enough that the dict will truncate.
	w := &sourceWriter{cap: 30}
	writeStreamObject(w, d, 12345)
	got := w.String()
	if !strings.Contains(got, "stream\n") {
		t.Errorf("envelope must ship `stream` marker even when capped, got %q", got)
	}
	if !strings.Contains(got, "endstream") {
		t.Errorf("envelope must ship `endstream` marker even when capped, got %q", got)
	}
	if !strings.Contains(got, "12,345 bytes") {
		t.Errorf("byte-count placeholder must ship even when capped, got %q", got)
	}
	if !strings.Contains(got, streamPlaceholderMarker) {
		t.Errorf("placeholder line must carry the suppression marker, got %q", got)
	}
}

// TestSourceWriterWriteAlwaysIgnoresCap verifies the close-bracket contract.
// WriteAlways must append even when truncated == true; otherwise capped output
// could ship without its matching `>>` / `]` / `endobj`.
func TestSourceWriterWriteAlwaysIgnoresCap(t *testing.T) {
	w := &sourceWriter{cap: 5}
	w.WriteString("abcde")            // fills the cap exactly
	w.WriteString("more content here") // flips truncated
	if !w.truncated {
		t.Fatalf("expected writer to be flagged truncated")
	}
	w.WriteAlways("]")
	if !strings.HasSuffix(w.String(), "]") {
		t.Errorf("WriteAlways must append even after truncation, got %q", w.String())
	}
}
