// Package font_inspection_test provides acceptance tests for Font Inspection
// View.
//
// Test pyramid for this story:
//   - Backend (iconHint='font' contract),
//     (font dict extraction + encoding + ToUnicode CMap +
//     FontDescriptor + composite descendant) -> pdfcore Go unit tests
//     delegated via runPdfcoreTest. Pure-function font analysis is best
//     verified in-process with hand-crafted fixtures.
//   - Backend ErrNotAFont sentinel, (resolved-dict drives view) ->
//     pdfcore Go unit tests asserting the sentinel + resolveNodeObject reuse.
//   - Wails plumbing -> structural assertions on service.go.
//   - Frontend swap, badge.. section behavior, keyboard,
//     loading debounce, header label, a11y -> Vitest. Delegated
//     here only via structural checks that the FontPreview component file and
//     test file exist; full behavior contracts live in
//     frontend/src/components/FontPreview.test.tsx and
//     DetailPanel.fontPreview.test.tsx.
//
// No Playwright/E2E layer: every AC is fully observable at the API level
// (FontDetail struct, ErrNotAFont sentinel) or the component level (rendered
// DOM, dispatched NAVIGATE_TO_REF actions, debounced loading flag). Adding a
// browser layer would repeat what the component tests already cover and
// contradict the test pyramid for this scope.
//
// Run: cd tests/font-inspection && go test -v -count=1 ./...
package font_inspection_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// projectRoot walks up from the working directory until it finds the project
// go.mod (module unidoc-pdf-debugger), and returns its absolute path.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if content, err := os.ReadFile(goModPath); err == nil {
			if strings.Contains(string(content), "module unidoc-pdf-debugger") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root (no go.mod with module unidoc-pdf-debugger found)")
		}
		dir = parent
	}
}

// runPdfcoreTest runs a named test pattern in internal/pdfcore/... and fails
// if the test does not pass or does not exist. Identical pattern to
// tests/object-source-and-reverse-refs/object_source_and_reverse_refs_test.go
// so dev writes the actual assertion logic in internal/pdfcore/font_test.go
// while this integration suite pins the test-name strings.
func runPdfcoreTest(t *testing.T, runPattern string) {
	t.Helper()
	root := projectRoot(t)
	cmd := exec.Command("go", "test", "-v", "-run", runPattern, "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	outStr := string(output)
	if err != nil {
		t.Fatalf("pdfcore test failed:\n%s", outStr)
	}
	if strings.Contains(outStr, "no tests to run") {
		t.Fatalf("no matching test found for pattern %q -- unit test not implemented yet:\n%s", runPattern, outStr)
	}
	if !strings.Contains(outStr, "PASS") {
		t.Fatalf("expected PASS in output but got:\n%s", outStr)
	}
}

// readSource reads a file relative to the project root.
func readSource(t *testing.T, relPath string) string {
	t.Helper()
	root := projectRoot(t)
	content, err := os.ReadFile(filepath.Join(root, relPath))
	if err != nil {
		t.Fatalf("cannot read %s: %v", relPath, err)
	}
	return string(content)
}

// ---------------------------------------------------------------------------
// (backend half) + -- font.go exists, GetFontDetail declared,
// ErrNotAFont sentinel declared, resolveNodeObject reused.
// ---------------------------------------------------------------------------

// internal/pdfcore/font.go exists and declares GetFontDetail on
// Inspector.
func TestFontFileExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "internal", "pdfcore", "font.go")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("internal/pdfcore/font.go does not exist")
	}
	src := readSource(t, "internal/pdfcore/font.go")
	if !strings.Contains(src, "func (ins *Inspector) GetFontDetail(") {
		t.Fatalf("font.go must declare GetFontDetail method on Inspector with signature 'func (ins *Inspector) GetFontDetail(tabID, nodeID string) (*FontDetail, error)'")
	}
}

// ErrNotAFont sentinel error is declared in errors.go.
// Fallback path: the frontend uses this sentinel to silently fall back to the
// generic DictView when the iconHint matches but the resolved dict is the
// /Resources /Font resource map (no /Type /Font). Without the sentinel, every
// resources map click would surface a generic error banner instead of the
// expected DictView.
func TestErrNotAFontSentinelDeclared(t *testing.T) {
	src := readSource(t, "internal/pdfcore/errors.go")
	if !strings.Contains(src, "ErrNotAFont") {
		t.Fatalf("errors.go must declare ErrNotAFont sentinel (Task 1.3). The frontend keys off this sentinel to fall back to DictView for the /Resources /Font resource-map false-positive case.")
	}
}

// font.go uses resolveNodeObject (NOT a hand-rolled resolver) so ObjStm
// and IndirectRef chains are handled transparently.
// Contract: resolved dict drives the view; no special-case code in
// GetFontDetail beyond existing infrastructure.
func TestFontUsesResolveNodeObject(t *testing.T) {
	src := readSource(t, "internal/pdfcore/font.go")
	if !strings.Contains(src, "resolveNodeObject") {
		t.Fatalf("font.go must use resolveNodeObject (existing helper) so ObjStm packaging and IndirectRef chains work transparently")
	}
}

// font.go wraps pdfcpu interactions in safeCall. Project invariant: any
// pdfcpu call site MUST recover panics; font dict parsing on a malformed font
// is a real risk surface.
func TestFontUsesSafeCall(t *testing.T) {
	src := readSource(t, "internal/pdfcore/font.go")
	if !strings.Contains(src, "safeCall") {
		t.Fatalf("font.go must wrap pdfcpu calls in safeCall (project-wide panic-recovery invariant; CMap parsing on malformed streams is a real panic surface)")
	}
}

// ---------------------------------------------------------------------------
// FontDetail / FontDescriptorInfo / EncodingDifference /
// ToUnicodeMapping model shapes
// ---------------------------------------------------------------------------

// model.go declares FontDetail with the exact JSON shape the frontend
// consumes. The frontend treats nil FontDescriptor and nil Descendant as
// "absent"; pointer fields are load-bearing for that.
func TestFontDetailStructShape(t *testing.T) {
	src := readSource(t, "internal/pdfcore/model.go")
	if !strings.Contains(src, "type FontDetail struct") {
		t.Fatalf("model.go must declare type FontDetail struct")
	}
	requiredFields := []string{
		"NodeID",
		"ObjectRef",
		"Subtype",
		"BaseFont",
		"FirstChar",
		"LastChar",
		"EncodingName",
		"BaseEncoding",
		"Differences",
		"ToUnicodeMappings",
		"ToUnicodeError",
		"Embedded",
		"FontDescriptor",
		"Descendant",
	}
	for _, f := range requiredFields {
		if !strings.Contains(src, f) {
			t.Errorf("FontDetail must declare field %q", f)
		}
	}
	requiredJSONTags := []string{
		`json:"nodeId"`,
		`json:"objectRef"`,
		`json:"subtype"`,
		`json:"baseFont"`,
		`json:"firstChar"`,
		`json:"lastChar"`,
		`json:"encodingName"`,
		`json:"baseEncoding"`,
		`json:"differences"`,
		`json:"toUnicodeMappings"`,
		`json:"toUnicodeError"`,
		`json:"embedded"`,
		`json:"fontDescriptor"`,
		`json:"descendant"`,
	}
	for _, tag := range requiredJSONTags {
		if !strings.Contains(src, tag) {
			t.Errorf("FontDetail must declare JSON tag %q (frontend serialization contract)", tag)
		}
	}
	// Descendant is *FontDetail (recursive pointer; nil for non-Type0 fonts).
	// FontDescriptor is *FontDescriptorInfo (nil when /FontDescriptor absent).
	// gofmt aligns struct fields with multiple spaces, so we normalize runs
	// of whitespace before substring matching to avoid false negatives.
	normalized := strings.Join(strings.Fields(src), " ")
	if !strings.Contains(normalized, "FontDescriptor *FontDescriptorInfo") {
		t.Errorf("FontDetail.FontDescriptor must be *FontDescriptorInfo (nil-able). A non-pointer would lose the absent-vs-empty distinction the FontPreview keys off")
	}
	if !strings.Contains(normalized, "Descendant *FontDetail") {
		t.Errorf("FontDetail.Descendant must be *FontDetail (nil for non-composite fonts)")
	}
}

// model.go declares the supporting types EncodingDifference,
// ToUnicodeMapping, FontDescriptorInfo. These cross the IPC boundary as
// nested arrays/objects inside FontDetail.
func TestFontSupportingTypesDeclared(t *testing.T) {
	src := readSource(t, "internal/pdfcore/model.go")

	if !strings.Contains(src, "type EncodingDifference struct") {
		t.Errorf("model.go must declare type EncodingDifference struct (Task 1.2)")
	}
	for _, tag := range []string{`json:"code"`, `json:"glyphName"`} {
		if !strings.Contains(src, tag) {
			t.Errorf("EncodingDifference must carry JSON tag %q", tag)
		}
	}

	if !strings.Contains(src, "type ToUnicodeMapping struct") {
		t.Errorf("model.go must declare type ToUnicodeMapping struct (Task 1.2)")
	}
	for _, tag := range []string{`json:"unicode"`, `json:"glyph"`} {
		if !strings.Contains(src, tag) {
			t.Errorf("ToUnicodeMapping must carry JSON tag %q", tag)
		}
	}

	if !strings.Contains(src, "type FontDescriptorInfo struct") {
		t.Errorf("model.go must declare type FontDescriptorInfo struct (Task 1.2)")
	}
	requiredFDFields := []string{
		"FontName",
		"Flags",
		"FlagNames",
		"ItalicAngle",
		"Ascent",
		"Descent",
		"CapHeight",
		"StemV",
		"FontBBox",
		"FontFileFormat",
		"FontFileSize",
	}
	for _, f := range requiredFDFields {
		if !strings.Contains(src, f) {
			t.Errorf("FontDescriptorInfo must declare field %q", f)
		}
	}
	for _, tag := range []string{
		`json:"fontName"`,
		`json:"flags"`,
		`json:"flagNames"`,
		`json:"italicAngle"`,
		`json:"ascent"`,
		`json:"descent"`,
		`json:"capHeight"`,
		`json:"stemV"`,
		`json:"fontBBox"`,
		`json:"fontFileFormat"`,
		`json:"fontFileSize"`,
	} {
		if !strings.Contains(src, tag) {
			t.Errorf("FontDescriptorInfo must carry JSON tag %q", tag)
		}
	}
}

// ---------------------------------------------------------------------------
// Backend half -- ErrNotAFont returned for the /Resources /Font
// resource-map false-positive case (Risk R3).
// ---------------------------------------------------------------------------

// (false-positive guard): when iconHint='font' is emitted for the
// /Resources /Font dict (which is a name->Font map, NOT a Font dict),
// GetFontDetail returns ErrNotAFont so the frontend renders the generic
// DictView instead of attempting a font-shaped extraction.
func TestGetFontDetailNotAFontSentinel(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_NotAFontSentinel")
}

// GetFontDetail with unknown tabID returns ErrDocumentNotFound (matches
// existing image/contentStream conventions).
func TestGetFontDetailUnknownTab(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_UnknownTab")
}

// GetFontDetail with empty nodeID returns a Go error (programmer error;
// matches GetImageData / GetContentStream conventions).
func TestGetFontDetailEmptyNodeID(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_EmptyNodeID")
}

// GetFontDetail on a malformed font dict does not panic (safeCall wrapping
// is asserted by behavior, not just by source grep).
func TestGetFontDetailPanicRecovery(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_PanicRecovery")
}

// ---------------------------------------------------------------------------
// Font metadata header: Subtype, BaseFont, FirstChar, LastChar,
// FontDescriptor presence, ToUnicode presence, Embedded badge wiring.
// ---------------------------------------------------------------------------

// Simple Type1 font with named encoding -- BaseFont, Subtype,
// EncodingName populated; Differences empty; Embedded reflects
// FontDescriptor.FontFile* presence (or absence).
func TestFontDetailSimpleType1(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_SimpleType1WithNamedEncoding")
}

// Embedded badge truth source -- Embedded reflects FontDescriptor.FontFile
// / FontFile2 / FontFile3 presence for non-Type0 fonts; Embedded reflects
// Descendant.FontDescriptor for Type0 fonts.
func TestFontDetailEmbeddedFlagWiring(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_EmbeddedFlagReflectsFontFile")
}

// Unembedded font (e.g. Helvetica with no FontFile) reports
// Embedded=false AND FontDescriptor.FontFileFormat=="".
func TestFontDetailUnembeddedFont(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_UnembeddedFontReportsNoFile")
}

// ---------------------------------------------------------------------------
// Unknown / missing Subtype renders verbatim, no special handling
// ---------------------------------------------------------------------------

// A font dict with missing or rare Subtype (e.g. MMType1, Type3) does not
// panic. Subtype is rendered verbatim; no assumption is made about Encoding /
// FontDescriptor / DescendantFonts shape beyond "render if present".
func TestFontDetailUnknownSubtype(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_UnknownOrMissingSubtype")
}

// ---------------------------------------------------------------------------
// Encoding name vs differences vs built-in vs absent
// ---------------------------------------------------------------------------

// Named encoding (e.g. /WinAnsiEncoding,
// /MacRomanEncoding) populates EncodingName; Differences slice is empty;
// BaseEncoding is empty.
func TestFontDetailEncodingNamed(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_EncodingNameOnly")
}

// /Encoding dict with /Differences array parses PDF spec ordering
// correctly: each integer is a starting code, subsequent names increment
// the code by 1. Two integers in the same array reset the code base.
// BaseEncoding (if present) is captured verbatim.
func TestFontDetailEncodingDifferences(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_DifferencesArrayParsing")
}

// A font with no /Encoding entry yields EncodingName="" and BaseEncoding=""
// and an empty Differences slice. The frontend renders the "Built-in encoding"
// sentinel based on that combination.
func TestFontDetailEncodingMissing(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_NoEncodingEntry")
}

// ---------------------------------------------------------------------------
// /ToUnicode CMap parsing (bfchar + bfrange), unparseable handling,
// surrogate / PUA / control glyph blanking
// ---------------------------------------------------------------------------

// Bfchar block parses pairs of (src-hex, unicode-hex) into
// ToUnicodeMapping{Code, Unicode (U+XXXX form), Glyph (literal)}.
func TestToUnicodeBfcharParsing(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_BfcharBlockParsing")
}

// Bfrange block with triplet form `<low> <high> <unicode-base>` expands to
// one ToUnicodeMapping per code in the range, incrementing the unicode base
// by 1 each step. The list form `<low> <high> [<u1> <u2> ...]` expands to
// one ToUnicodeMapping per code, taking each ui in order.
func TestToUnicodeBfrangeParsing(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_BfrangeBlockParsing")
}

// multi-codepoint -- a bfchar entry whose unicode side is `<00660066006C>`
// (ligature ffl, two or more codepoints) renders the joined literal in the
// glyph cell and emits the codepoints as `U+0066 U+0066 U+006C` in the
// unicode field.
func TestToUnicodeMultiCodepointMapping(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_MultiCodepointLigature")
}

// Unpaired UTF-16 surrogate halves (D800-DFFF) in the unicode side decode to
// the codepoint without panic; the glyph cell is blank (or U+25CC dotted
// circle) because no valid character is present.
func TestToUnicodeSurrogateBlanksGlyph(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_SurrogateGlyphBlanked")
}

// Private Use Area codepoints (E000-F8FF, F0000-FFFFD, 100000-10FFFD) render
// with a blank glyph cell. Without the blank rule, embedded PUA characters
// render as Apple/Microsoft system glyphs that are misleading in a font-debug
// context.
func TestToUnicodePUABlanksGlyph(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_PrivateUseAreaGlyphBlanked")
}

// C0/C1 control codepoints (<0x20, 0x7F-0xA0) render with a blank glyph
// cell. Same reason as PUA: literal control chars in a text cell are
// confusing.
func TestToUnicodeControlBlanksGlyph(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_ControlGlyphBlanked")
}

// Unparseable CMap -- when the ToUnicode stream decodes but the
// bfchar/bfrange scanner fails, ToUnicodeError is populated (non-empty
// string) and ToUnicodeMappings is empty. GetFontDetail still returns
// (*FontDetail, nil) -- partial-success semantics.
func TestToUnicodeUnparseable(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_UnparseableCMapPopulatesError")
}

// ---------------------------------------------------------------------------
// FontDescriptor decoding: name, flags (bit-decoded), metrics,
// FontFile / FontFile2 / FontFile3 presence and format string
// ---------------------------------------------------------------------------

// /FontDescriptor indirect ref is resolved;
// FontDescriptorInfo populates FontName, ItalicAngle, Ascent, Descent,
// CapHeight, StemV, FontBBox from the resolved dict.
func TestFontDescriptorMetrics(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_FontDescriptorMetricsPopulated")
}

// /Flags integer decodes per PDF 1.7 spec section 9.8.2 Table 123. Bit map
// (1-indexed): 1=FixedPitch, 2=Serif, 3=Symbolic, 4=Script, 6=Nonsymbolic,
// 7=Italic, 17=AllCap, 18=SmallCap, 19=ForceBold. FlagNames slice carries
// the human-readable names of every set bit; bits 5, 8-16 are reserved and
// MUST NOT appear in FlagNames.
func TestFontDescriptorFlagsDecoded(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_FlagsBitDecoded")
}

// FontFile / FontFile2 / FontFile3 detection. FontFile -> Format="Type1";
// FontFile2 -> Format="TrueType". FontFile3 reads its /Subtype for the format
// string (e.g. OpenType, Type1C, CIDFontType0C). FontFileSize is the byte
// length of the decoded stream content.
func TestFontDescriptorFontFileDetection(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_FontFileFormatAndSize")
}

// ---------------------------------------------------------------------------
// Composite font (Type0) descendant chain
// ---------------------------------------------------------------------------

// A /Subtype /Type0 font with /DescendantFonts[0] populates
// FontDetail.Descendant; Descendant.Subtype is "CIDFontType0" or
// "CIDFontType2"; Descendant.BaseFont matches the descendant's /BaseFont;
// Descendant.FontDescriptor is populated from the descendant's dict.
func TestFontDetailType0Descendant(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_Type0DescendantPopulated")
}

// Composite font Embedded badge wiring. For Type0 fonts the Embedded flag
// MUST reflect Descendant.FontDescriptor.FontFile*, NOT the outer dict's own
// FontDescriptor (which is typically absent on Type0 parent fonts).
func TestFontDetailType0EmbeddedFromDescendant(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_Type0EmbeddedReflectsDescendant")
}

// CIDSystemInfo fields (Registry, Ordering,
// Supplement), CIDToGIDMap mode ("Identity" or stream-length integer), and
// /DW (default width) all surface on the Descendant FontDetail. We pin these
// on the Descendant because the FontPreview's "Descendant Font" section
// reads them straight off.
func TestFontDetailType0CIDFields(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_Type0CIDSystemInfoFields")
}

// ---------------------------------------------------------------------------
// Wails service plumbing (Task 3)
// ---------------------------------------------------------------------------

// PDFService.GetFontDetail is exposed.
func TestServiceExposesGetFontDetail(t *testing.T) {
	src := readSource(t, "internal/pdfservice/service.go")
	if !strings.Contains(src, "GetFontDetail") {
		t.Fatalf("service.go must expose GetFontDetail (Task 3.1)")
	}
	// Return signature -- the frontend bindings depend on the exact element type.
	if !strings.Contains(src, "*pdfcore.FontDetail") {
		t.Fatalf("service.go GetFontDetail must return *pdfcore.FontDetail (Task 3.1)")
	}
}

// ---------------------------------------------------------------------------
// IndirectRef / ObjStm packaging transparency (no special-case code in
// GetFontDetail; existing resolveNodeObject handles both)
// ---------------------------------------------------------------------------

// A font dict reached through an indirect-ref chain is fully populated.
// The same FontDetail comes back regardless of whether the caller passes
// the direct dict's nodeID or an indirect-ref node ID, because
// resolveNodeObject dereferences transparently.
func TestFontDetailIndirectRefChain(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_IndirectRefChainResolved")
}

// A font dict packaged in /ObjStm extracts the same FontDetail as the same
// dict outside ObjStm. resolveNodeObject's ObjStm path is shared; this
// asserts the behavior continues to work.
func TestFontDetailObjStmPackaging(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_ObjStmPackagedFont")
}

// ---------------------------------------------------------------------------
// Defensive -- Type3 / unknown subtypes do not panic
// ---------------------------------------------------------------------------

// Defensive coverage -- Type3 font dict (procedural
// glyphs, /CharProcs sub-dict) does not panic; FontDetail.Subtype="Type3";
// metadata fields populate where present; ToUnicode / Encoding remain valid
// for the cases that apply.
func TestFontDetailType3DoesNotPanic(t *testing.T) {
	runPdfcoreTest(t, "TestGetFontDetail_Type3FontNoPanic")
}

// ---------------------------------------------------------------------------
// Architecture compliance
// ---------------------------------------------------------------------------

// pdfcore/font.go has zero Wails imports. pdfcore must not depend on the
// desktop framework.
func TestFontFileZeroWailsImports(t *testing.T) {
	src := readSource(t, "internal/pdfcore/font.go")
	if strings.Contains(src, "wailsapp") {
		t.Errorf("internal/pdfcore/font.go imports Wails (contains 'wailsapp') -- pdfcore must have zero Wails dependencies")
	}
}

// go vet passes on pdfcore.
func TestPdfcoreGoVet(t *testing.T) {
	root := projectRoot(t)
	cmd := exec.Command("go", "vet", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go vet failed on pdfcore:\n%s", string(output))
	}
}

// All pdfcore tests still pass (no regression after font work lands). This
// catches accidental shared-state corruption -- e.g. if FontDetail
// construction reuses a slice across documents.
func TestPdfcoreNoRegression(t *testing.T) {
	root := projectRoot(t)
	cmd := exec.Command("go", "test", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pdfcore regression -- tests failed:\n%s", string(output))
	}
	if !strings.Contains(string(output), "ok") && !strings.Contains(string(output), "PASS") {
		t.Fatalf("expected PASS or ok in output but got:\n%s", string(output))
	}
}

// All pdfservice tests still pass (no regression after GetFontDetail
// binding lands).
func TestPdfserviceNoRegression(t *testing.T) {
	root := projectRoot(t)
	cmd := exec.Command("go", "test", "-count=1", "./internal/pdfservice/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pdfservice regression -- tests failed:\n%s", string(output))
	}
	if !strings.Contains(string(output), "ok") && !strings.Contains(string(output), "PASS") {
		t.Fatalf("expected PASS or ok in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// Frontend structural (Vitest behavior contracts live in the .test.tsx files;
// this asserts only file/symbol existence so the unit suites can target them)
// ---------------------------------------------------------------------------

// FontPreview.tsx component file exists with the expected
// default-or-named export.
func TestFontPreviewComponentFileExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "FontPreview.tsx")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("frontend/src/components/FontPreview.tsx must exist (Task 4.1)")
	}
	src := readSource(t, "frontend/src/components/FontPreview.tsx")
	if !strings.Contains(src, "FontPreview") {
		t.Fatalf("FontPreview.tsx must export a FontPreview component")
	}
	// Per the story Task 4.1: FontPreview is a pure presentational component
	// that takes a `detail` prop AND an `onReferenceClick` handler. We pin
	// both in the source so the frontend tests can drive the click path
	// through the handler (matching the ImagePreview / ContentStreamViewer
	// presentational pattern).
	if !strings.Contains(src, "onReferenceClick") {
		t.Fatalf("FontPreview.tsx must accept an onReferenceClick prop so DetailPanel threads its handleReferenceClick handler through")
	}
	if !strings.Contains(src, "detail") {
		t.Fatalf("FontPreview.tsx must accept a `detail` prop carrying the FontDetail data")
	}
}

// FontPreview.test.tsx exists. The behavior contract for every.. rendering
// rule lives there.
func TestFontPreviewTestFileExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "FontPreview.test.tsx")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("frontend/src/components/FontPreview.test.tsx must exist (Task 6.1)")
	}
}

// DetailPanel.tsx wires the FontPreview branch. The branch fires when
// detail.type==='dict' AND selectedNodeIconHint==='font' AND the GetFontView
// fetch resolves with Kind=='detail'. We assert structurally that the imports
// + fetch call + render path exist.
//
// Updated post-refactor: the dev unified GetFontDetail + GetFontResourceMap
// into a single GetFontView endpoint so the Wails binding layer no longer
// logs ERR on the /Resources /Font false-positive path. The frontend now
// calls GetFontView exactly once per click.
func TestDetailPanelMountsFontPreview(t *testing.T) {
	src := readSource(t, "frontend/src/components/DetailPanel.tsx")
	if !strings.Contains(src, "FontPreview") {
		t.Fatalf("DetailPanel.tsx must import and render FontPreview (Task 5.1)")
	}
	if !strings.Contains(src, "GetFontView") {
		t.Fatalf("DetailPanel.tsx must call GetFontView when iconHint==='font' (unified endpoint)")
	}
	// 200ms debounce parallel to imageLoading/showImageLoading. The loading
	// state lives in the showFontLoading flag; the debounce behavior itself
	// is covered by in DetailPanel.fontPreview.test.tsx (asserts the
	// font-loading indicator at both sides of the 200ms edge).
	if !strings.Contains(src, "showFontLoading") {
		t.Fatalf("DetailPanel.tsx must declare a `showFontLoading` state for the 200ms-debounced indicator")
	}
}

// DetailPanel.fontPreview.test.tsx exists. Mirrors the
// DetailPanel.reverseRefs.test.tsx pattern used by Story 9-10 -- standalone
// file to avoid splicing into the existing 1678-line DetailPanel.test.tsx.
func TestDetailPanelFontPreviewTestFileExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "DetailPanel.fontPreview.test.tsx")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("frontend/src/components/DetailPanel.fontPreview.test.tsx must exist (Task 6.2 integration test)")
	}
}

// DetailPanel header label uses "Font" prefix when FontPreview is the
// active view. The exact string format is "Font - <BaseFont>" or "Font"
// when BaseFont is missing. We pin the prefix presence in the source so a
// refactor that drops it fails immediately.
func TestDetailPanelFontHeaderLabel(t *testing.T) {
	src := readSource(t, "frontend/src/components/DetailPanel.tsx")
	// Loose check: the header label path or TYPE_LABEL_MAP-equivalent must
	// reference 'Font' so the active-view branch can stamp the header.
	// We avoid pinning the exact string to leave dev free to restructure
	// the label-building logic (e.g. inline vs. lookup-map extension), but
	// SOMETHING in DetailPanel.tsx must mention 'Font' in label-building
	// context post-implementation.
	if !strings.Contains(src, "'Font'") && !strings.Contains(src, `"Font"`) {
		t.Fatalf("DetailPanel.tsx must surface a 'Font' label string when FontPreview is active")
	}
}

// ---------------------------------------------------------------------------
// Manual verification placeholders (Task 6 manual smoke) -- documented, not
// asserted. The dev step opens a real PDF with a Type0 composite font and
// confirms the descendant section renders, the embedded badge is green when
// FontFile* is present, the ToUnicode table renders without literal control
// chars. These checks require visual confirmation and are not automatable
// from the integration layer.
// ---------------------------------------------------------------------------
