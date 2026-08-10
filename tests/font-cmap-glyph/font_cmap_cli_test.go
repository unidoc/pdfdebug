package font_cmap_glyph_test

// Story 13.3: Font CMap and Glyph-Mapping Inspection -- CLI acceptance suite.
//
// TDD RED PHASE: these tests MUST fail until Task 2 lands the fuller `dump
// font` mapping output (AC3) and Task 1/3 surface the mapping array + health
// signals on the FontView JSON (AC1/AC2 riding through the bindings).
//
// Contract under test (13-1: plain text is the DEFAULT; --json is the stable
// surface):
//   - `dump font --ref "N G R" <file>`            : plain summary (row count +
//                                                    health signals), NOT JSON.
//   - `dump font --glyphs --ref "N G R" <file>`   : full aligned per-code table
//                                                    (CODE GLYPH UNICODE TEXT).
//   - `dump font --json --ref "N G R" <file>`     : complete mapping array +
//                                                    health, regardless of --glyphs.
//
// Fixtures (testdata/fonts-mixed.pdf):
//   - obj "6 0 R": Type0 composite, /Identity-H, ToUnicode mapping 0x41->A,
//                  0x42->B (codes present in the joined mapping).
//   - obj "5 0 R": Type1 with /Differences [32 /space /exclam /quotedbl] and
//                  NO /ToUnicode (health: ToUnicode missing + encoding codes
//                  without ToUnicode).

import (
	"path/filepath"
	"strings"
	"testing"
)

// fontViewJSON mirrors the FontView JSON wire shape with the NEW Story 13.3
// fields. The struct tags pin the contract the bindings must carry through.
type fontViewJSON struct {
	Kind   string `json:"kind"`
	Detail *struct {
		ObjectRef   string `json:"objectRef"`
		Subtype     string `json:"subtype"`
		MappingRows []struct {
			Code        int    `json:"code"`
			CodeHex     string `json:"codeHex"`
			GlyphName   string `json:"glyphName"`
			Unicode     string `json:"unicode"`
			UnicodeText string `json:"unicodeText"`
		} `json:"mappingRows"`
		Health *struct {
			DeclaredCodeCount             int   `json:"declaredCodeCount"`
			ToUnicodeMissing              bool  `json:"toUnicodeMissing"`
			IdentityWithoutToUnicode      bool  `json:"identityWithoutToUnicode"`
			EncodingWithoutToUnicodeCodes []int `json:"encodingWithoutToUnicodeCodes"`
		} `json:"health"`
	} `json:"detail"`
}

// 13.3-INTG-001 (AC1, AC3): --json carries the complete mapping array on the
// detail payload. The ToUnicode codes 0x41/0x42 surface as joined rows with
// U+XXXX unicode and the literal glyph text.
func TestFontDump_JSON_CarriesMappingArray(t *testing.T) {
	bin := buildCLI(t)
	pdf := filepath.Join(testdataDir(t), "fonts-mixed.pdf")

	stdout, _, ec := runCLI(t, bin, "dump", "font", "--json", "--ref", "6 0 R", pdf)
	if ec != 0 {
		t.Fatalf("[13.3-INTG-001] expected exit 0, got %d", ec)
	}
	var view fontViewJSON
	mustParseJSON(t, stdout, &view)
	if view.Kind != "detail" || view.Detail == nil {
		t.Fatalf("[13.3-INTG-001] expected kind=detail with a detail payload, got kind=%q", view.Kind)
	}
	if len(view.Detail.MappingRows) == 0 {
		t.Fatalf("[13.3-INTG-001] detail.mappingRows is empty; want the joined per-code rows (AC1)")
	}
	var a *struct {
		Code        int    `json:"code"`
		CodeHex     string `json:"codeHex"`
		GlyphName   string `json:"glyphName"`
		Unicode     string `json:"unicode"`
		UnicodeText string `json:"unicodeText"`
	}
	for i := range view.Detail.MappingRows {
		if view.Detail.MappingRows[i].Code == 0x41 {
			a = &view.Detail.MappingRows[i]
			break
		}
	}
	if a == nil {
		t.Fatalf("[13.3-INTG-001] no mapping row for code 0x41")
	}
	if a.Unicode != "U+0041" {
		t.Errorf("[13.3-INTG-001] code 0x41 unicode = %q, want U+0041", a.Unicode)
	}
	if a.UnicodeText != "A" {
		t.Errorf("[13.3-INTG-001] code 0x41 unicodeText = %q, want A", a.UnicodeText)
	}
}

// 13.3-INTG-002 (AC2): --json carries the health signals on the detail payload.
// obj 5 has /Differences but no /ToUnicode -> toUnicodeMissing true and the
// Differences codes appear in encodingWithoutToUnicodeCodes.
func TestFontDump_JSON_CarriesHealthSignals(t *testing.T) {
	bin := buildCLI(t)
	pdf := filepath.Join(testdataDir(t), "fonts-mixed.pdf")

	stdout, _, ec := runCLI(t, bin, "dump", "font", "--json", "--ref", "5 0 R", pdf)
	if ec != 0 {
		t.Fatalf("[13.3-INTG-002] expected exit 0, got %d", ec)
	}
	var view fontViewJSON
	mustParseJSON(t, stdout, &view)
	if view.Detail == nil || view.Detail.Health == nil {
		t.Fatalf("[13.3-INTG-002] detail.health missing; want the coverage signals (AC2)")
	}
	if !view.Detail.Health.ToUnicodeMissing {
		t.Errorf("[13.3-INTG-002] health.toUnicodeMissing = false, want true (obj 5 has no /ToUnicode)")
	}
	codes := map[int]bool{}
	for _, c := range view.Detail.Health.EncodingWithoutToUnicodeCodes {
		codes[c] = true
	}
	for _, want := range []int{32, 33, 34} {
		if !codes[want] {
			t.Errorf("[13.3-INTG-002] code %d missing from health.encodingWithoutToUnicodeCodes", want)
		}
	}
}

// 13.3-INTG-003 (AC3): the plain-text default (no --glyphs, no --json) is a
// BOUNDED summary -- a declared-code count plus the health signals -- NOT the
// full per-code table and NOT JSON.
func TestFontDump_PlainSummary_BoundedNotFullTable(t *testing.T) {
	bin := buildCLI(t)
	pdf := filepath.Join(testdataDir(t), "fonts-mixed.pdf")

	stdout, _, ec := runCLI(t, bin, "dump", "font", "--ref", "5 0 R", pdf)
	if ec != 0 {
		t.Fatalf("[13.3-INTG-003] expected exit 0, got %d", ec)
	}
	if isJSONObject(stdout) {
		t.Fatalf("[13.3-INTG-003] default output must be plain text, not JSON:\n%.200s", stdout)
	}
	// The summary must report a declared-code count for the mapping table.
	if !strings.Contains(strings.ToLower(stdout), "code") {
		t.Errorf("[13.3-INTG-003] plain summary should mention the mapping code count\n%s", stdout)
	}
	// The summary must surface the missing-ToUnicode health signal.
	if !strings.Contains(strings.ToLower(stdout), "tounicode") {
		t.Errorf("[13.3-INTG-003] plain summary should surface the ToUnicode health signal\n%s", stdout)
	}
	// Without --glyphs the full per-code TABLE header must NOT appear -- the
	// summary is bounded. Anchor to the actual header line (four ordered
	// columns) rather than free-floating uppercase tokens, which could match a
	// health label or reformatted text anywhere in the output.
	if perCodeTableHeaderPresent(stdout) {
		t.Errorf("[13.3-INTG-003] plain summary leaked the full per-code table (use --glyphs for that)\n%s", stdout)
	}
}

// perCodeTableHeaderPresent reports whether stdout contains the --glyphs
// per-code table header as a single anchored line whose fields are exactly
// CODE GLYPH UNICODE TEXT in order. Anchoring to the whole header row (not
// independent whole-output greps of the generic tokens GLYPH/UNICODE/TEXT)
// keeps the assertion from false-matching stray uppercase text elsewhere.
func perCodeTableHeaderPresent(stdout string) bool {
	for _, line := range strings.Split(stdout, "\n") {
		f := strings.Fields(line)
		if len(f) == 4 && f[0] == "CODE" && f[1] == "GLYPH" && f[2] == "UNICODE" && f[3] == "TEXT" {
			return true
		}
	}
	return false
}

// 13.3-INTG-004 (AC3): with --glyphs the plain output is the FULL per-code
// table with aligned columns CODE GLYPH UNICODE TEXT. obj 6's ToUnicode codes
// surface their U+XXXX and literal text in the table body.
func TestFontDump_Glyphs_FullPerCodeTable(t *testing.T) {
	bin := buildCLI(t)
	pdf := filepath.Join(testdataDir(t), "fonts-mixed.pdf")

	stdout, _, ec := runCLI(t, bin, "dump", "font", "--glyphs", "--ref", "6 0 R", pdf)
	if ec != 0 {
		t.Fatalf("[13.3-INTG-004] expected exit 0, got %d", ec)
	}
	if isJSONObject(stdout) {
		t.Fatalf("[13.3-INTG-004] --glyphs output must be plain text, not JSON:\n%.200s", stdout)
	}
	// Assert the anchored header row (CODE GLYPH UNICODE TEXT as ordered
	// columns on one line), not four independent whole-output token greps.
	if !perCodeTableHeaderPresent(stdout) {
		t.Errorf("[13.3-INTG-004] --glyphs output missing the aligned per-code table header (CODE GLYPH UNICODE TEXT)\n%s", stdout)
	}
	if !strings.Contains(stdout, "U+0041") {
		t.Errorf("[13.3-INTG-004] --glyphs table missing the 0x41->U+0041 row\n%s", stdout)
	}
}

// 13.3-INTG-005 (AC3): --json includes the complete mapping array EVEN WITHOUT
// --glyphs. The --glyphs flag affects only the plain-text verbosity; the JSON
// surface is complete unconditionally.
func TestFontDump_JSON_CompleteRegardlessOfGlyphs(t *testing.T) {
	bin := buildCLI(t)
	pdf := filepath.Join(testdataDir(t), "fonts-mixed.pdf")

	stdout, _, ec := runCLI(t, bin, "dump", "font", "--json", "--ref", "6 0 R", pdf)
	if ec != 0 {
		t.Fatalf("[13.3-INTG-005] expected exit 0, got %d", ec)
	}
	var view fontViewJSON
	mustParseJSON(t, stdout, &view)
	if view.Detail == nil || len(view.Detail.MappingRows) == 0 {
		t.Fatalf("[13.3-INTG-005] --json (no --glyphs) must still carry the complete mapping array")
	}
}

// 13.3-INTG-006 (AC3 usage): the `dump font` usage/help text documents the new
// --glyphs flag so it is discoverable.
func TestFontDump_UsageDocumentsGlyphsFlag(t *testing.T) {
	bin := buildCLI(t)
	// Invoke `dump font` with no file to trigger the resource-specific usage.
	stdout, stderr, _ := runCLI(t, bin, "dump", "font")
	combined := stdout + stderr
	if !strings.Contains(combined, "--glyphs") {
		t.Errorf("[13.3-INTG-006] `dump font` usage does not mention --glyphs:\n%s", combined)
	}
}
