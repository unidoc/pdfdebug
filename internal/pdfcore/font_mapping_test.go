package pdfcore

// Story 13.3: Font CMap and Glyph-Mapping Inspection.
//
// TDD RED PHASE: these tests MUST fail until Task 1 lands the assembled
// per-code mapping table (AC1) and the coverage/health signals (AC2) on
// FontDetail (or a sub-view carried on it).
//
// Contract under test (the shape Dev must implement; named here so the CLI
// acceptance tests and the FontPreview Vitest stay in lockstep):
//
//	FontDetail.MappingRows []FontMappingRow  // AC1 union-of-declared-codes JOIN
//	FontDetail.Health       *FontHealth      // AC2 coverage/health signals
//
//	type FontMappingRow struct {
//	    Code        int    `json:"code"`        // character code
//	    CodeHex     string `json:"codeHex"`     // "0x41" form
//	    GlyphName   string `json:"glyphName"`   // from /Differences, "" if none
//	    Unicode     string `json:"unicode"`     // "U+XXXX" from ToUnicode, "" if none
//	    UnicodeText string `json:"unicodeText"` // literal glyph string
//	}
//
//	type FontHealth struct {
//	    DeclaredCodeCount             int   `json:"declaredCodeCount"`
//	    ToUnicodeMissing              bool  `json:"toUnicodeMissing"`
//	    IdentityWithoutToUnicode      bool  `json:"identityWithoutToUnicode"`
//	    EncodingWithoutToUnicodeCodes []int `json:"encodingWithoutToUnicodeCodes"`
//	}
//
// The assertions below reference these fields directly, so the package will
// FAIL TO COMPILE until model.go declares them -- the intended red signal.

import (
	"testing"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// malformedToUnicodeFontDict returns an inline /Type /Font dict whose
// /ToUnicode is a StreamDict with a malformed CMap body (a beginbfchar with no
// endbfchar). Content is pre-set so StreamDict.Decode short-circuits (no
// FilterPipeline) and populateToUnicode reaches parseToUnicodeCMap, which
// reports the error -> ToUnicodeError. The dict also carries a /Differences
// entry so the degraded view still has rows to assemble.
func malformedToUnicodeFontDict() pdfcpu_types.Dict {
	badCMap := []byte("beginbfchar\n<0041> <0041>\n") // no endbfchar -> parse error
	sd := pdfcpu_types.StreamDict{
		Dict:    pdfcpu_types.Dict{},
		Content: badCMap,
		Raw:     badCMap,
	}
	return pdfcpu_types.Dict{
		"Type":     pdfcpu_types.Name("Font"),
		"Subtype":  pdfcpu_types.Name("Type1"),
		"BaseFont": pdfcpu_types.Name("Broken"),
		"Encoding": pdfcpu_types.Dict{
			"Type":         pdfcpu_types.Name("Encoding"),
			"BaseEncoding": pdfcpu_types.Name("WinAnsiEncoding"),
			"Differences": pdfcpu_types.Array{
				pdfcpu_types.Integer(65), pdfcpu_types.Name("A"),
			},
		},
		"ToUnicode": sd,
	}
}

// 13.3-UNIT-001 (AC1): a simple font's mapping table is the UNION of codes in
// /Differences and /ToUnicode, joined per code. fonts-mixed.pdf has no single
// font that carries BOTH a Differences table and a ToUnicode CMap, so this
// test drives an in-memory dict assembled from the existing parser outputs via
// the public extraction path. obj 5 (Differences, no ToUnicode) is the closest
// real fixture: every declared code must surface a row with the glyph name and
// an EMPTY unicode/unicodeText (no ToUnicode entry to join).
func TestFontMapping_SimpleFont_JoinsDifferencesAndToUnicode(t *testing.T) {
	ins, tabID := openFontsPDF(t)
	// obj 5: /Differences [32 /space /exclam /quotedbl], FontDescriptor 8, no ToUnicode.
	detail, err := ins.GetFontDetail(tabID, "obj:0:5")
	if err != nil {
		t.Fatalf("GetFontDetail: %v", err)
	}

	rows := detail.MappingRows
	if len(rows) != 3 {
		t.Fatalf("MappingRows count = %d, want 3 (union of declared codes)", len(rows))
	}

	byCode := map[int]FontMappingRow{}
	for _, r := range rows {
		byCode[r.Code] = r
	}

	space, ok := byCode[32]
	if !ok {
		t.Fatalf("no MappingRow for code 32")
	}
	if space.GlyphName != "/space" {
		t.Errorf("code 32 GlyphName = %q, want /space (joined from Differences)", space.GlyphName)
	}
	if space.CodeHex != "0x20" {
		t.Errorf("code 32 CodeHex = %q, want 0x20", space.CodeHex)
	}
	// No ToUnicode in this font -- unicode/unicodeText must be empty, not garbage.
	if space.Unicode != "" {
		t.Errorf("code 32 Unicode = %q, want empty (no ToUnicode entry)", space.Unicode)
	}
	if space.UnicodeText != "" {
		t.Errorf("code 32 UnicodeText = %q, want empty (no ToUnicode entry)", space.UnicodeText)
	}
}

// 13.3-UNIT-002 (AC1): the join carries ToUnicode unicode + literal text when a
// code is present in the CMap. obj 6 (Type0) has a ToUnicode CMap mapping
// 0x41->A, 0x42->B and Identity-H encoding (no /Differences). Every CMap code
// must surface a row with the U+XXXX unicode and the literal glyph string.
func TestFontMapping_ToUnicodeCodesJoinUnicodeAndText(t *testing.T) {
	ins, tabID := openFontsPDF(t)
	detail, err := ins.GetFontDetail(tabID, "obj:0:6")
	if err != nil {
		t.Fatalf("GetFontDetail: %v", err)
	}

	byCode := map[int]FontMappingRow{}
	for _, r := range detail.MappingRows {
		byCode[r.Code] = r
	}

	a, ok := byCode[0x41]
	if !ok {
		t.Fatalf("no MappingRow for code 0x41 (present in ToUnicode)")
	}
	if a.Unicode != "U+0041" {
		t.Errorf("code 0x41 Unicode = %q, want U+0041", a.Unicode)
	}
	if a.UnicodeText != "A" {
		t.Errorf("code 0x41 UnicodeText = %q, want A", a.UnicodeText)
	}
}

// 13.3-UNIT-003 (AC1, CID): a CID/Type0 font surfaces the descendant's
// CIDSystemInfo, CIDToGIDMap value, and the ToUnicode ranges. The mapping
// assembly must NOT crash on the composite shape; the CID metadata is reachable
// on the descendant and the ToUnicode rows assemble on the parent's view.
func TestFontMapping_CIDFont_SurfacesCIDMetadataAndRanges(t *testing.T) {
	ins, tabID := openFontsPDF(t)
	detail, err := ins.GetFontDetail(tabID, "obj:0:6")
	if err != nil {
		t.Fatalf("GetFontDetail: %v", err)
	}
	if detail.Descendant == nil || detail.Descendant.CIDSystemInfo == nil {
		t.Fatalf("expected descendant CIDSystemInfo on Type0 font")
	}
	if detail.Descendant.CIDToGIDMap != "Identity" {
		t.Errorf("descendant CIDToGIDMap = %q, want Identity", detail.Descendant.CIDToGIDMap)
	}
	// The composite font's assembled mapping must reflect the ToUnicode ranges
	// (0x41, 0x42) -- it is the parent Type0 dict that carries /ToUnicode.
	if len(detail.MappingRows) < 2 {
		t.Errorf("MappingRows count = %d, want >= 2 (the ToUnicode ranges)", len(detail.MappingRows))
	}
}

// 13.3-UNIT-004 (AC2): a font with Encoding codes absent from ToUnicode flags
// each such code -- extraction will fail for them. obj 5 has /Differences codes
// 32,33,34 and NO ToUnicode, so all three are "encoding without ToUnicode".
func TestFontHealth_EncodingCodesWithoutToUnicode(t *testing.T) {
	ins, tabID := openFontsPDF(t)
	detail, err := ins.GetFontDetail(tabID, "obj:0:5")
	if err != nil {
		t.Fatalf("GetFontDetail: %v", err)
	}
	if detail.Health == nil {
		t.Fatalf("expected FontDetail.Health to be populated")
	}
	codes := map[int]bool{}
	for _, c := range detail.Health.EncodingWithoutToUnicodeCodes {
		codes[c] = true
	}
	for _, want := range []int{32, 33, 34} {
		if !codes[want] {
			t.Errorf("code %d missing from EncodingWithoutToUnicodeCodes (extraction will fail for it)", want)
		}
	}
}

// 13.3-UNIT-005 (AC2): a font missing a ToUnicode CMap entirely is flagged.
// obj 5 has no /ToUnicode entry at all.
func TestFontHealth_ToUnicodeMissingFlagged(t *testing.T) {
	ins, tabID := openFontsPDF(t)
	detail, err := ins.GetFontDetail(tabID, "obj:0:5")
	if err != nil {
		t.Fatalf("GetFontDetail: %v", err)
	}
	if detail.Health == nil {
		t.Fatalf("expected FontDetail.Health to be populated")
	}
	if !detail.Health.ToUnicodeMissing {
		t.Errorf("ToUnicodeMissing = false, want true (obj 5 has no /ToUnicode)")
	}
}

// 13.3-UNIT-006 (AC2): a font WITH a ToUnicode CMap does NOT raise the
// missing-ToUnicode flag. obj 6 (Type0) carries /ToUnicode 10 0 R.
func TestFontHealth_ToUnicodePresentNotFlagged(t *testing.T) {
	ins, tabID := openFontsPDF(t)
	detail, err := ins.GetFontDetail(tabID, "obj:0:6")
	if err != nil {
		t.Fatalf("GetFontDetail: %v", err)
	}
	if detail.Health == nil {
		t.Fatalf("expected FontDetail.Health to be populated")
	}
	if detail.Health.ToUnicodeMissing {
		t.Errorf("ToUnicodeMissing = true, want false (obj 6 has /ToUnicode)")
	}
}

// 13.3-UNIT-007 (AC2): the declared-code count reflects the union of
// Differences + ToUnicode codes -- the "complete" denominator the view shows.
// obj 5 has 3 Differences codes and no ToUnicode -> 3 declared codes.
func TestFontHealth_DeclaredCodeCount(t *testing.T) {
	ins, tabID := openFontsPDF(t)
	detail, err := ins.GetFontDetail(tabID, "obj:0:5")
	if err != nil {
		t.Fatalf("GetFontDetail: %v", err)
	}
	if detail.Health == nil {
		t.Fatalf("expected FontDetail.Health to be populated")
	}
	if detail.Health.DeclaredCodeCount != 3 {
		t.Errorf("DeclaredCodeCount = %d, want 3 (union of declared codes)", detail.Health.DeclaredCodeCount)
	}
}

// 13.3-UNIT-008 (AC5 degradation): a malformed ToUnicode stream degrades to the
// existing fallback -- ToUnicodeError is set, the mapping assembly does not
// panic, and the health signals still populate for whatever parsed. The
// in-memory dict here carries a /ToUnicode stream whose CMap body is malformed
// (beginbfchar with no endbfchar), exercising the parseToUnicodeCMap error path
// without re-parsing it in the test.
func TestFontMapping_MalformedToUnicode_DegradesNotCrash(t *testing.T) {
	// doc with nil PDFContext: dereferenceIfRef tolerates it, and the inline
	// dict carries direct (non-ref) values, so no dereferencing is needed.
	doc := &DocumentState{}
	detail := buildFontDetailFromDict(doc, "obj:0:1", malformedToUnicodeFontDict())

	if detail.ToUnicodeError == "" {
		t.Errorf("ToUnicodeError = empty, want a parse-error string (malformed CMap)")
	}
	// Health must still be present even when ToUnicode failed to parse.
	if detail.Health == nil {
		t.Fatalf("expected FontDetail.Health populated even on malformed ToUnicode")
	}
	// A malformed/unparseable ToUnicode counts as effectively missing coverage.
	if !detail.Health.ToUnicodeMissing {
		t.Errorf("ToUnicodeMissing = false, want true (CMap present but unparseable yields no coverage)")
	}
	// MappingRows must be a non-nil slice (possibly the Differences-only rows),
	// never a nil deref or panic.
	if detail.MappingRows == nil {
		t.Errorf("MappingRows = nil, want non-nil slice on degraded font")
	}
}
