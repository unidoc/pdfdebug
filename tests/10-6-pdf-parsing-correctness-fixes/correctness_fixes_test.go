// Package story_10_6_test holds the per-story acceptance test suite for
// Story 10.6: PDF Parsing and Data-Correctness Fixes.
//
// This is the structural-assertion suite -- it source-greps the production
// tree for the contracts pinned in the story's ACs, plus checks the fixture
// corpus on disk. Behavioural / business-logic tests (e.g.
// TestBuildReachableSetDeepNesting, TestParseBfrangeCarry, TestLatin1Decode
// FullRange, TestExtractStreamInfoIndirectLength, TestParseDifferencesOut
// OfRange, TestGetPlainTextSizeAfterRemove) live alongside the production
// code in internal/pdfcore/*_test.go per the story's Task 8 -- this suite
// pins their NAMES so a Dev cannot land the production change without the
// named test.
//
// TDD red-phase contract: every Test_10_6_* in this file FAILS today
// against the pre-implementation tree. Dev's job is to land the changes
// that turn each red test green.
//
// Run: cd tests/10-6-pdf-parsing-correctness-fixes && go test -v -count=1 ./...
package story_10_6_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// projectRoot walks up from cwd until it finds the project go.mod (module
// unidoc-pdf-debugger). Mirrors the pattern from the 10-5 sibling suite.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			if strings.Contains(string(data), "module unidoc-pdf-debugger") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root (no go.mod with module unidoc-pdf-debugger)")
		}
		dir = parent
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

// extractFunctionBody returns the substring of src starting at
// `func (ins *Inspector) <name>(` and continuing to the next top-level
// `func ` at column 0. Approximation; sufficient for "does the body contain
// X" substring checks.
func extractFunctionBody(t *testing.T, src, name string) string {
	t.Helper()
	needle := "func (ins *Inspector) " + name + "("
	startIdx := strings.Index(src, needle)
	if startIdx == -1 {
		return ""
	}
	tail := src[startIdx:]
	endIdx := strings.Index(tail[1:], "\nfunc ")
	if endIdx == -1 {
		return tail
	}
	return tail[:endIdx+1]
}

// extractTopLevelFuncBody locates `func <name>(` at column 0 (no receiver)
// and returns the body up to the next column-0 `func `.
func extractTopLevelFuncBody(t *testing.T, src, name string) string {
	t.Helper()
	needle := "func " + name + "("
	idx := strings.Index(src, needle)
	if idx == -1 {
		return ""
	}
	tail := src[idx:]
	end := strings.Index(tail[1:], "\nfunc ")
	if end == -1 {
		return tail
	}
	return tail[:end+1]
}

// ---------------------------------------------------------------------------
// AC#1 -- fixture corpus at testdata/correctness/ + README
// ---------------------------------------------------------------------------

// correctnessFixtures is the AC1-pinned list of fixture filenames. Each
// fixture demonstrates exactly one failure mode and is committed as a
// hand-authored uncompressed PDF (not generated at test time).
var correctnessFixtures = []string{
	"deep-nesting.pdf",
	"stream-length-indirect.pdf",
	"latin1-c1.pdf",
	"diff-out-of-range.pdf",
	"cjk-cmap-carry.pdf",
}

// Test_10_6_AC1_FixtureCorpusExists [P0] AC#1: the five named PDF fixtures
// MUST live under testdata/correctness/. Each is a hand-authored uncompressed
// PDF demonstrating one failure mode.
func Test_10_6_AC1_FixtureCorpusExists(t *testing.T) {
	root := projectRoot(t)
	for _, name := range correctnessFixtures {
		path := filepath.Join(root, "testdata", "correctness", name)
		t.Run(name, func(t *testing.T) {
			info, err := os.Stat(path)
			if err != nil {
				t.Errorf("[P0] 10-6-AC1: testdata/correctness/%s must exist (hand-authored uncompressed PDF demonstrating one failure mode)", name)
				return
			}
			if info.IsDir() {
				t.Errorf("[P0] 10-6-AC1: testdata/correctness/%s must be a file, not a directory", name)
				return
			}
			if info.Size() == 0 {
				t.Errorf("[P0] 10-6-AC1: testdata/correctness/%s must not be empty (must be a valid minimal PDF)", name)
			}
		})
	}
}

// Test_10_6_AC1_FixtureCorpusReadme [P0] AC#1: a README.md MUST sit alongside
// the fixtures, describing each fixture's purpose and byte-level structure
// sufficient for a future engineer to regenerate by hand.
func Test_10_6_AC1_FixtureCorpusReadme(t *testing.T) {
	root := projectRoot(t)
	readmePath := filepath.Join(root, "testdata", "correctness", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("[P0] 10-6-AC1: testdata/correctness/README.md must exist and describe each fixture's purpose and byte-level structure: %v", err)
	}
	body := string(data)
	for _, name := range correctnessFixtures {
		if !strings.Contains(body, name) {
			t.Errorf("[P0] 10-6-AC1: testdata/correctness/README.md must mention %q (describe its purpose and byte-level structure)", name)
		}
	}
}

// ---------------------------------------------------------------------------
// AC#2 -- drop depth cap in buildReachableSet AND findPathToObject
// ---------------------------------------------------------------------------

// Test_10_6_AC2_BuildReachableSetNoDepthCap [P0] AC#2: the depth guard
// `if head.depth >= maxRefDepth { continue }` at internal/pdfcore/object
// index.go:123 MUST be REMOVED. The visited-set (entries map) already
// prevents cycles; the depth cap mislabels legitimate-but-deep PDFs as
// orphan trees.
func Test_10_6_AC2_BuildReachableSetNoDepthCap(t *testing.T) {
	src := readSource(t, "internal/pdfcore/objectindex.go")
	body := extractTopLevelFuncBody(t, src, "buildReachableSet")
	if body == "" {
		t.Fatalf("[P0] 10-6-AC2: could not locate buildReachableSet in objectindex.go")
	}
	if strings.Contains(body, "head.depth >= maxRefDepth") {
		t.Errorf("[P0] 10-6-AC2: buildReachableSet must NOT contain `head.depth >= maxRefDepth` -- drop the depth cap; the visited-set already prevents cycles (AC2)")
	}
	// The reach-entry frame need no longer carry depth, but the spec leaves
	// that as a refactor preference. We do NOT pin the struct shape; only
	// the depth-test removal.
}

// Test_10_6_AC2_FindPathToObjectNoDepthCap [P0] AC#2: the matching depth
// guard `if entry.depth >= maxRefDepth { continue }` at internal/pdfcore/
// inspector.go:512 inside findPathToObject MUST be REMOVED for consistency
// (same package, same orphan-detection semantics).
func Test_10_6_AC2_FindPathToObjectNoDepthCap(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	body := extractTopLevelFuncBody(t, src, "findPathToObject")
	if body == "" {
		t.Fatalf("[P0] 10-6-AC2: could not locate findPathToObject in inspector.go")
	}
	if strings.Contains(body, "entry.depth >= maxRefDepth") {
		t.Errorf("[P0] 10-6-AC2: findPathToObject must NOT contain `entry.depth >= maxRefDepth` -- drop the depth cap (AC2: visited-set already prevents cycles)")
	}
}

// Test_10_6_AC2_TreeMaxRefDepthRetained [P0] AC#2: the maxRefDepth constant
// at internal/pdfcore/tree.go MUST be retained for the page-tree caller at
// tree.go (different cycle-tolerance semantics that this story does NOT
// change). A one-line comment noting the retention MUST be present.
func Test_10_6_AC2_TreeMaxRefDepthRetained(t *testing.T) {
	src := readSource(t, "internal/pdfcore/tree.go")
	if !strings.Contains(src, "maxRefDepth = 32") {
		t.Errorf("[P0] 10-6-AC2: internal/pdfcore/tree.go must retain `maxRefDepth = 32` for the page-tree caller (AC2: tree.go semantics unchanged by this story)")
	}
	// Look for an explanatory comment immediately above the constant. Loose
	// match: any of these phrases anchor the retention rationale.
	retentionMarkers := []string{
		"retained",
		"page-tree",
		"page tree",
		"buildChildren",
		"different cycle",
	}
	body := src
	hit := false
	for _, marker := range retentionMarkers {
		if strings.Contains(body, marker) {
			hit = true
			break
		}
	}
	if !hit {
		t.Errorf("[P0] 10-6-AC2: tree.go must carry a one-line comment near `maxRefDepth = 32` noting its retention rationale (e.g. \"retained for buildChildren / page-tree\", AC2 task 2)")
	}
}

// Test_10_6_AC2_BuildReachableSetTestExists [P0] AC#2: the new test
// TestBuildReachableSetDeepNesting MUST be declared in
// internal/pdfcore/objectindex_test.go. The story spec is explicit: the
// test exercises boundary at depth 32 AND well past it (e.g. depth 50).
func Test_10_6_AC2_BuildReachableSetTestExists(t *testing.T) {
	src := readSource(t, "internal/pdfcore/objectindex_test.go")
	needle := "func TestBuildReachableSetDeepNesting(t *testing.T)"
	if !strings.Contains(src, needle) {
		t.Errorf("[P0] 10-6-AC2: internal/pdfcore/objectindex_test.go must declare %q (AC2: boundary-at-32 AND well-past-32, e.g. 50)", needle)
	}
}

// Test_10_6_AC2_FindPathToObjectTestExists [P0] AC#2: the new test
// TestFindPathToObjectDeepNesting MUST be declared in
// internal/pdfcore/inspector_test.go.
func Test_10_6_AC2_FindPathToObjectTestExists(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector_test.go")
	needle := "func TestFindPathToObjectDeepNesting(t *testing.T)"
	if !strings.Contains(src, needle) {
		t.Errorf("[P0] 10-6-AC2: internal/pdfcore/inspector_test.go must declare %q (AC2: same depth-cap removal, behavioural pin)", needle)
	}
}

// ---------------------------------------------------------------------------
// AC#3 -- extractStreamInfo gains doc *DocumentState; IndirectRef length resolved
// ---------------------------------------------------------------------------

// Test_10_6_AC3_ExtractStreamInfoSignature [P0] AC#3: the function signature
// at internal/pdfcore/inspector.go MUST be `func extractStreamInfo(doc
// *DocumentState, obj pdfcpu_types.Object) *StreamInfo`. The pre-fix
// signature `func extractStreamInfo(obj pdfcpu_types.Object) *StreamInfo`
// has no DocumentState handle and cannot dereference IndirectRef lengths.
func Test_10_6_AC3_ExtractStreamInfoSignature(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	want := "func extractStreamInfo(doc *DocumentState, obj pdfcpu_types.Object) *StreamInfo"
	if !strings.Contains(src, want) {
		t.Errorf("[P0] 10-6-AC3: internal/pdfcore/inspector.go must declare %q (AC3: doc handle threaded so IndirectRef /Length can be resolved via doc.PDFContext.Dereference)", want)
	}
	// And the pre-fix signature MUST be gone. Substring overlap would let
	// the post-fix line satisfy a contains check for the old signature,
	// so anchor with the closing paren of the old one-arg form.
	bad := "func extractStreamInfo(obj pdfcpu_types.Object) *StreamInfo"
	if strings.Contains(src, bad) {
		t.Errorf("[P0] 10-6-AC3: internal/pdfcore/inspector.go must NOT keep the pre-fix one-arg signature %q (AC3: the doc handle is mandatory)", bad)
	}
}

// Test_10_6_AC3_ExtractStreamInfoDereferencesIndirect [P0] AC#3: the post-fix
// extractStreamInfo body MUST handle the case where sd.StreamLength is nil
// by inspecting sd.Dict["Length"]; an Integer is used directly, an
// IndirectRef is resolved via doc.PDFContext.Dereference. Pin the load-
// bearing tokens.
func Test_10_6_AC3_ExtractStreamInfoDereferencesIndirect(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	body := extractTopLevelFuncBody(t, src, "extractStreamInfo")
	if body == "" {
		t.Fatalf("[P0] 10-6-AC3: could not locate extractStreamInfo in inspector.go")
	}
	// Pre-fix body uses `sd.StreamLength != nil`. Post-fix MUST additionally
	// inspect sd.Dict["Length"] when StreamLength is nil. Loose-but-anchored
	// substring checks; phrasing left to the Dev.
	if !strings.Contains(body, `sd.Dict["Length"]`) {
		t.Errorf("[P0] 10-6-AC3: extractStreamInfo must inspect `sd.Dict[\"Length\"]` when sd.StreamLength is nil (AC3 fallback path)")
	}
	if !strings.Contains(body, "doc.PDFContext.Dereference") {
		t.Errorf("[P0] 10-6-AC3: extractStreamInfo must call `doc.PDFContext.Dereference` to resolve an IndirectRef length (AC3)")
	}
	if !strings.Contains(body, "pdfcpu_types.IndirectRef") && !strings.Contains(body, "types.IndirectRef") {
		t.Errorf("[P0] 10-6-AC3: extractStreamInfo must type-switch on `pdfcpu_types.IndirectRef` to drive the dereference branch (AC3)")
	}
	if !strings.Contains(body, "pdfcpu_types.Integer") && !strings.Contains(body, "types.Integer") {
		t.Errorf("[P0] 10-6-AC3: extractStreamInfo must type-switch on `pdfcpu_types.Integer` for the direct-integer branch (AC3)")
	}
}

// Test_10_6_AC3_GetObjectDetailPassesDoc [P0] AC#3: the three extractStreamInfo
// call sites inside GetObjectDetail MUST pass `doc` as the first argument.
// Pre-fix call shape: `extractStreamInfo(obj)`. Post-fix: `extractStreamInfo(doc, obj)`.
func Test_10_6_AC3_GetObjectDetailPassesDoc(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	body := extractFunctionBody(t, src, "GetObjectDetail")
	if body == "" {
		t.Fatalf("[P0] 10-6-AC3: could not locate GetObjectDetail in inspector.go")
	}
	// Count post-fix call shape occurrences. Spec says 3 call sites (one per
	// StreamDict / ObjectStreamDict / XRefStreamDict branch).
	postFixCount := strings.Count(body, "extractStreamInfo(doc,")
	if postFixCount < 3 {
		t.Errorf("[P0] 10-6-AC3: GetObjectDetail must invoke `extractStreamInfo(doc, ...)` at all 3 stream-type branches (got %d occurrences of the new signature in the function body; AC3)", postFixCount)
	}
	// And the pre-fix single-arg call shape MUST be absent in this function.
	preFixRe := regexp.MustCompile(`extractStreamInfo\(obj\)`)
	if preFixRe.MatchString(body) {
		t.Errorf("[P0] 10-6-AC3: GetObjectDetail must NOT retain `extractStreamInfo(obj)` (the pre-fix call signature) -- AC3 threads doc through")
	}
}

// Test_10_6_AC3_IndirectLengthTestExists [P0] AC#3: TestExtractStreamInfo
// IndirectLength MUST be declared in internal/pdfcore/inspector_test.go.
// The spec also requires a pre-step that asserts sd.StreamLength == nil for
// the fixture's stream object -- the test owner is the Dev step.
func Test_10_6_AC3_IndirectLengthTestExists(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector_test.go")
	needle := "func TestExtractStreamInfoIndirectLength(t *testing.T)"
	if !strings.Contains(src, needle) {
		t.Errorf("[P0] 10-6-AC3: internal/pdfcore/inspector_test.go must declare %q (AC3: opens fixture, asserts non-zero length AND that StreamLength was nil pre-fix)", needle)
	}
}

// ---------------------------------------------------------------------------
// AC#4 -- latin1Decode doc comment rewrite + full-range pinning test
// ---------------------------------------------------------------------------

// Test_10_6_AC4_Latin1DecodeBodyUnchanged [P0] AC#4: the latin1Decode body
// at internal/pdfcore/plaintext.go is UNCHANGED. The story explicitly says
// the implementation does not change -- only the doc comment and a new
// pinning test. We pin the load-bearing branch (rune(b) cast inside the
// 0x09/0x0A/0x0D + 0x20<=c!=0x7F branches) so the Dev cannot accidentally
// "fix" the C1 behavior by extending the U+FFFD replacement range.
func Test_10_6_AC4_Latin1DecodeBodyUnchanged(t *testing.T) {
	src := readSource(t, "internal/pdfcore/plaintext.go")
	body := extractTopLevelFuncBody(t, src, "latin1Decode")
	if body == "" {
		t.Fatalf("[P0] 10-6-AC4: could not locate latin1Decode in plaintext.go")
	}
	// Pre-existing branches that AC4 says MUST stay:
	if !strings.Contains(body, "c == 0x09 || c == 0x0A || c == 0x0D") {
		t.Errorf("[P0] 10-6-AC4: latin1Decode must retain the `c == 0x09 || c == 0x0A || c == 0x0D` whitespace passthrough branch (AC4: implementation unchanged)")
	}
	if !strings.Contains(body, "c >= 0x20 && c != 0x7F") {
		t.Errorf("[P0] 10-6-AC4: latin1Decode must retain the `c >= 0x20 && c != 0x7F` Latin-1 passthrough branch (AC4: C1 and 0xA0-0xFF map verbatim via rune(c))")
	}
	if !strings.Contains(body, "sb.WriteRune(rune(c))") {
		t.Errorf("[P0] 10-6-AC4: latin1Decode must retain `sb.WriteRune(rune(c))` for the passthrough branches (AC4: lossless byte-for-codepoint mapping)")
	}
}

// Test_10_6_AC4_Latin1DecodeDocCommentRewritten [P0] AC#4: the doc comment
// at internal/pdfcore/plaintext.go MUST accurately describe behavior. The
// post-fix comment includes the phrase "C1 controls" (explicitly naming the
// 0x80-0x9F range) and "map verbatim" (the lossless Latin-1 contract).
// Pre-fix comment carries neither phrase.
func Test_10_6_AC4_Latin1DecodeDocCommentRewritten(t *testing.T) {
	src := readSource(t, "internal/pdfcore/plaintext.go")
	// Anchor on the comment immediately preceding `func latin1Decode`.
	idx := strings.Index(src, "func latin1Decode(")
	if idx == -1 {
		t.Fatalf("[P0] 10-6-AC4: could not locate latin1Decode in plaintext.go")
	}
	// Walk back ~30 lines to capture the leading // comment block.
	start := max(idx-2000, 0)
	preface := src[start:idx]
	// Trim to the start of the contiguous // block before the func.
	lines := strings.Split(preface, "\n")
	commentLines := []string{}
	// Walk from the bottom up collecting consecutive `//` lines.
	for i := len(lines) - 1; i >= 0; i-- {
		trim := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trim, "//") {
			commentLines = append([]string{trim}, commentLines...)
			continue
		}
		if trim == "" && len(commentLines) > 0 {
			continue
		}
		if len(commentLines) > 0 {
			break
		}
	}
	commentBlock := strings.Join(commentLines, "\n")
	requiredPhrases := []string{
		"C1 controls",
		"0x80-0x9F",
		"verbatim",
	}
	for _, p := range requiredPhrases {
		if !strings.Contains(commentBlock, p) {
			t.Errorf("[P0] 10-6-AC4: latin1Decode doc comment must mention %q (AC4: comment rewrite must accurately describe C1 passthrough). Got comment block:\n%s", p, commentBlock)
		}
	}
}

// Test_10_6_AC4_Latin1FullRangeUnitTestExists [P0] AC#4: TestLatin1DecodeFull
// Range MUST be declared in internal/pdfcore/plaintext_test.go. The spec
// says it pins the byte-for-codepoint contract for every byte 0x00-0xFF via
// a direct call (no fixture).
func Test_10_6_AC4_Latin1FullRangeUnitTestExists(t *testing.T) {
	src := readSource(t, "internal/pdfcore/plaintext_test.go")
	needle := "func TestLatin1DecodeFullRange(t *testing.T)"
	if !strings.Contains(src, needle) {
		t.Errorf("[P0] 10-6-AC4: internal/pdfcore/plaintext_test.go must declare %q (AC4: pins the byte-for-codepoint contract for all 256 bytes via direct latin1Decode call)", needle)
	}
}

// Test_10_6_AC4_Latin1C1IntegrationTestExists [P0] AC#4: TestGetPlainText
// Latin1C1 MUST be declared in plaintext_test.go (or inspector_test.go).
// This is the integration test that opens the fixture and asserts the C1
// region maps verbatim.
func Test_10_6_AC4_Latin1C1IntegrationTestExists(t *testing.T) {
	root := projectRoot(t)
	needle := "func TestGetPlainTextLatin1C1(t *testing.T)"
	// Test can live in either plaintext_test.go or inspector_test.go.
	candidates := []string{
		"internal/pdfcore/plaintext_test.go",
		"internal/pdfcore/inspector_test.go",
	}
	for _, c := range candidates {
		data, err := os.ReadFile(filepath.Join(root, c))
		if err != nil {
			continue
		}
		if strings.Contains(string(data), needle) {
			return
		}
	}
	t.Errorf("[P0] 10-6-AC4: %q must be declared in internal/pdfcore/plaintext_test.go (preferred) or inspector_test.go (AC4: integration test against testdata/correctness/latin1-c1.pdf)", needle)
}

// ---------------------------------------------------------------------------
// AC#5 -- parseDifferences out-of-range guard
// ---------------------------------------------------------------------------

// Test_10_6_AC5_ParseDifferencesBoundsGuard [P0] AC#5: at internal/pdfcore/
// font.go inside parseDifferences, after the `currentCode = int(v)` line,
// a guard `if currentCode < 0 || currentCode > 255 { continue }` MUST be
// present. The pre-fix code has no such guard; /Differences arrays carrying
// out-of-range integers (-1, 999, etc.) leak rows into the encoding table
// with garbage codes.
func Test_10_6_AC5_ParseDifferencesBoundsGuard(t *testing.T) {
	src := readSource(t, "internal/pdfcore/font.go")
	body := extractTopLevelFuncBody(t, src, "parseDifferences")
	if body == "" {
		t.Fatalf("[P0] 10-6-AC5: could not locate parseDifferences in font.go")
	}
	// Loose anchor: any of these shapes satisfies the spec. The Dev gets
	// some leeway on phrasing but must reject codes < 0 OR > 255.
	candidatePatterns := []string{
		"currentCode < 0 || currentCode > 255",
		"currentCode > 255 || currentCode < 0",
		"currentCode < 0 || 255 < currentCode",
	}
	hit := false
	for _, pat := range candidatePatterns {
		if strings.Contains(body, pat) {
			hit = true
			break
		}
	}
	if !hit {
		t.Errorf("[P0] 10-6-AC5: parseDifferences must contain a guard rejecting `currentCode < 0` and `currentCode > 255` after the `currentCode = int(v)` line (AC5: out-of-range codes skipped silently). Acceptable shapes: %v", candidatePatterns)
	}
}

// Test_10_6_AC5_ParseDifferencesOutOfRangeTestExists [P0] AC#5: TestParse
// DifferencesOutOfRange MUST be declared in internal/pdfcore/font_test.go.
// The spec says it calls parseDifferences directly with a synthesized array
// (Integer(-1), Integer(999), and a name).
func Test_10_6_AC5_ParseDifferencesOutOfRangeTestExists(t *testing.T) {
	src := readSource(t, "internal/pdfcore/font_test.go")
	needle := "func TestParseDifferencesOutOfRange(t *testing.T)"
	if !strings.Contains(src, needle) {
		t.Errorf("[P0] 10-6-AC5: internal/pdfcore/font_test.go must declare %q (AC5: synthesized array with Integer(-1), Integer(999), and a name)", needle)
	}
}

// ---------------------------------------------------------------------------
// AC#6 -- parseBfrange overflow break replaced with carry propagation
// ---------------------------------------------------------------------------

// Test_10_6_AC6_ParseBfrangeNoSilentBreak [P0] AC#6: the existing overflow
// `break` at internal/pdfcore/font.go (lines 780-783) MUST be REPLACED with
// a carry implementation. The structural marker for the pre-fix shape is
// the `if tail > 0xFFFF { break }` pattern; that pattern is silent data
// loss for ranges that legitimately cross UTF-16 unit boundaries.
//
// We assert the literal `if tail > 0xFFFF {\n\t\t\t\tbreak\n\t\t\t}` (or a
// loose form thereof) is gone, and a carry-propagation marker is present.
func Test_10_6_AC6_ParseBfrangeNoSilentBreak(t *testing.T) {
	src := readSource(t, "internal/pdfcore/font.go")
	body := extractTopLevelFuncBody(t, src, "parseBfrange")
	if body == "" {
		t.Fatalf("[P0] 10-6-AC6: could not locate parseBfrange in font.go")
	}
	// The pre-fix pattern is the *combination* of `tail > 0xFFFF` AND `break`
	// in a context with no carry propagation. Loose check: the post-fix
	// MUST NOT contain the standalone `if tail > 0xFFFF {\n\t\t\t\tbreak`.
	silentBreakRe := regexp.MustCompile(`if\s+tail\s*>\s*0xFFFF\s*{\s*break\s*}`)
	if silentBreakRe.MatchString(body) {
		t.Errorf("[P0] 10-6-AC6: parseBfrange must NOT keep the silent `if tail > 0xFFFF { break }` overflow handler -- AC6 replaces it with carry propagation across UTF-16 units")
	}
}

// Test_10_6_AC6_ParseBfrangeCarryShape [P0] AC#6: the post-fix parseBfrange
// MUST carry into higher UTF-16 units when the trailing unit overflows.
// Pin the load-bearing tokens of the carry implementation: setting
// units[last] to `tail & 0xFFFF` AND propagating `tail >> 16` (or
// equivalent) into the next-higher unit.
func Test_10_6_AC6_ParseBfrangeCarryShape(t *testing.T) {
	src := readSource(t, "internal/pdfcore/font.go")
	body := extractTopLevelFuncBody(t, src, "parseBfrange")
	if body == "" {
		t.Fatalf("[P0] 10-6-AC6: could not locate parseBfrange in font.go")
	}
	// Look for the bit-mask write into the trailing unit. Acceptable forms:
	//   advanced[last] = uint16(tail & 0xFFFF)
	//   advanced[len(advanced)-1] = uint16(tail & 0xFFFF)
	//   units[len(units)-1] = uint16(tail & 0xFFFF)
	maskRe := regexp.MustCompile(`uint16\(\s*tail\s*&\s*0xFFFF\s*\)`)
	if !maskRe.MatchString(body) {
		t.Errorf("[P0] 10-6-AC6: parseBfrange must write the trailing UTF-16 unit as `uint16(tail & 0xFFFF)` (AC6 carry implementation)")
	}
	// Look for the high-bit propagation. Acceptable forms:
	//   tail >> 16
	//   carry := tail >> 16
	shiftRe := regexp.MustCompile(`tail\s*>>\s*16`)
	if !shiftRe.MatchString(body) {
		t.Errorf("[P0] 10-6-AC6: parseBfrange must propagate the carry via `tail >> 16` into a higher UTF-16 unit (AC6 carry implementation)")
	}
}

// Test_10_6_AC6_ParseBfrangePreLoopSpanCheckUnchanged [P0] AC#6: the
// pre-loop span check at internal/pdfcore/font.go (`high-low+1 >
// maxBfrangeSpan`) MUST be UNCHANGED. AC6 carry concerns the per-iteration
// trailing-unit overflow only.
func Test_10_6_AC6_ParseBfrangePreLoopSpanCheckUnchanged(t *testing.T) {
	src := readSource(t, "internal/pdfcore/font.go")
	body := extractTopLevelFuncBody(t, src, "parseBfrange")
	if body == "" {
		t.Fatalf("[P0] 10-6-AC6: could not locate parseBfrange in font.go")
	}
	if !strings.Contains(body, "high-low+1 > maxBfrangeSpan") {
		t.Errorf("[P0] 10-6-AC6: parseBfrange must retain the pre-loop span check `if high-low+1 > maxBfrangeSpan` (AC6: pre-loop span check unchanged)")
	}
}

// Test_10_6_AC6_ParseBfrangeCarryTestExists [P0] AC#6: TestParseBfrangeCarry
// MUST be declared in internal/pdfcore/font_test.go. The spec covers three
// cases: (a) trailing-unit carry into higher unit; (b) leading-unit overflow
// stops the loop; (c) the existing pre-loop span-cap rejection still returns
// an error.
func Test_10_6_AC6_ParseBfrangeCarryTestExists(t *testing.T) {
	src := readSource(t, "internal/pdfcore/font_test.go")
	needle := "func TestParseBfrangeCarry(t *testing.T)"
	if !strings.Contains(src, needle) {
		t.Errorf("[P0] 10-6-AC6: internal/pdfcore/font_test.go must declare %q (AC6: covers trailing-unit carry, leading-unit overflow stop, AND the unchanged pre-loop span-cap rejection)", needle)
	}
}

// ---------------------------------------------------------------------------
// AC#7 -- DocumentState.FileSize captured at Open; redundant os.Stat removed
// ---------------------------------------------------------------------------

// Test_10_6_AC7_DocumentStateHasFileSize [P0] AC#7: DocumentState MUST
// declare a new `FileSize int64` field. The field is pinned by name -- a
// renamed or relocated field defeats the assertion.
func Test_10_6_AC7_DocumentStateHasFileSize(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	// Anchor: field declaration shape on its own line.
	re := regexp.MustCompile(`(?m)^\s*FileSize\s+int64\b`)
	if !re.MatchString(src) {
		t.Errorf("[P0] 10-6-AC7: internal/pdfcore/inspector.go must declare `FileSize int64` as a field on DocumentState (AC7: stat-at-Open value, surfaced by GetPlainTextSize and threaded into readPlainText)")
	}
}

// Test_10_6_AC7_OpenPopulatesFileSize [P0] AC#7: Inspector.Open MUST set
// the new FileSize field on the DocumentState literal -- the existing
// `fileSize := fi.Size()` is already captured and passed into DocumentInfo,
// so the change is literal: include `FileSize: fileSize` inside the
// `doc := &DocumentState{...}` literal (NOT the DocumentInfo literal,
// which already has it pre-fix).
func Test_10_6_AC7_OpenPopulatesFileSize(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	body := extractFunctionBody(t, src, "Open")
	if body == "" {
		t.Fatalf("[P0] 10-6-AC7: could not locate Inspector.Open in inspector.go")
	}
	// Anchor on the DocumentState struct literal specifically. Walk from
	// `doc := &DocumentState{` to the matching closing `}` (brace count of 1
	// is enough since the literal has no nested braces in the baseline).
	openStart := strings.Index(body, "doc := &DocumentState{")
	if openStart == -1 {
		t.Fatalf("[P0] 10-6-AC7: could not locate `doc := &DocumentState{` in Inspector.Open body")
	}
	// Find the matching closing brace.
	depth := 0
	end := -1
	for i := openStart; i < len(body); i++ {
		switch body[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i + 1
			}
		}
		if end != -1 {
			break
		}
	}
	if end == -1 {
		t.Fatalf("[P0] 10-6-AC7: could not find closing brace for DocumentState literal in Inspector.Open")
	}
	literal := body[openStart:end]
	if !strings.Contains(literal, "FileSize:") {
		t.Errorf("[P0] 10-6-AC7: Inspector.Open must include `FileSize: fileSize` inside the `doc := &DocumentState{...}` literal (AC7: stat-at-Open value cached on DocumentState). Got literal:\n%s", literal)
	}
	// And the value MUST be the local `fileSize` variable, not a fresh stat.
	if !strings.Contains(literal, "FileSize: fileSize") && !strings.Contains(literal, "FileSize:  fileSize") {
		t.Errorf("[P0] 10-6-AC7: Inspector.Open must populate FileSize from the local `fileSize` variable captured at the start of Open (AC7: reuse the existing stat result; do NOT re-stat). Got literal:\n%s", literal)
	}
}

// Test_10_6_AC7_GetPlainTextSizeUsesCachedField [P0] AC#7: Inspector.Get
// PlainTextSize at internal/pdfcore/plaintext.go MUST return `doc.FileSize`
// directly. The pre-fix body calls `os.Stat(doc.FilePath)` -- AC7 removes
// that call entirely.
func Test_10_6_AC7_GetPlainTextSizeUsesCachedField(t *testing.T) {
	src := readSource(t, "internal/pdfcore/plaintext.go")
	body := extractFunctionBody(t, src, "GetPlainTextSize")
	if body == "" {
		t.Fatalf("[P0] 10-6-AC7: could not locate GetPlainTextSize in plaintext.go")
	}
	if strings.Contains(body, "os.Stat(") {
		t.Errorf("[P0] 10-6-AC7: GetPlainTextSize must NOT call `os.Stat(...)` -- AC7 removes the redundant re-stat and returns the value captured at Open")
	}
	if !strings.Contains(body, "doc.FileSize") {
		t.Errorf("[P0] 10-6-AC7: GetPlainTextSize must return `doc.FileSize` (AC7: cached-at-Open value)")
	}
}

// Test_10_6_AC7_ReadPlainTextSignatureTakesSize [P0] AC#7: the helper
// readPlainText at internal/pdfcore/plaintext.go MUST gain a `size int64`
// parameter and remove the in-function `os.Stat(path)` call. The pre-fix
// signature is `func readPlainText(ctx context.Context, path, tabID string)`;
// post-fix is `func readPlainText(ctx context.Context, path string, size int64, tabID string)`
// (or an equivalent ordering -- pin only the presence of `size int64`).
func Test_10_6_AC7_ReadPlainTextSignatureTakesSize(t *testing.T) {
	src := readSource(t, "internal/pdfcore/plaintext.go")
	// Anchor on `func readPlainText(` and assert `size int64` appears in
	// the parameter list before the next `)`.
	idx := strings.Index(src, "func readPlainText(")
	if idx == -1 {
		t.Fatalf("[P0] 10-6-AC7: could not locate readPlainText in plaintext.go")
	}
	tail := src[idx:]
	closeIdx := strings.Index(tail, ")")
	if closeIdx == -1 {
		t.Fatalf("[P0] 10-6-AC7: malformed readPlainText signature in plaintext.go (no closing paren)")
	}
	signature := tail[:closeIdx+1]
	if !strings.Contains(signature, "size int64") {
		t.Errorf("[P0] 10-6-AC7: readPlainText signature must include `size int64` (AC7: caller threads doc.FileSize through). Got: %s", signature)
	}
}

// Test_10_6_AC7_ReadPlainTextNoStat [P0] AC#7: the readPlainText body MUST
// NOT call `os.Stat(path)`. The pre-fix body calls it at line 140 to get
// totalBytes; post-fix uses the passed-in `size` argument.
func Test_10_6_AC7_ReadPlainTextNoStat(t *testing.T) {
	src := readSource(t, "internal/pdfcore/plaintext.go")
	body := extractTopLevelFuncBody(t, src, "readPlainText")
	if body == "" {
		t.Fatalf("[P0] 10-6-AC7: could not locate readPlainText in plaintext.go")
	}
	if strings.Contains(body, "os.Stat(") {
		t.Errorf("[P0] 10-6-AC7: readPlainText must NOT call `os.Stat(...)` -- AC7 removes the redundant stat and uses the passed-in `size` argument")
	}
}

// Test_10_6_AC7_GetPlainTextSizeDocCommentUpdated [P0] AC#7: the doc comment
// on GetPlainTextSize MUST be updated. The pre-fix wording "surfaces the raw
// os.Stat error when the file moves post-Open" must be REMOVED; the post-fix
// wording mentions "captured at Open" (or equivalent), affirming that
// post-Open file moves do not affect the returned value.
func Test_10_6_AC7_GetPlainTextSizeDocCommentUpdated(t *testing.T) {
	src := readSource(t, "internal/pdfcore/plaintext.go")
	// Anchor on the doc-comment block immediately preceding GetPlainTextSize.
	idx := strings.Index(src, "func (ins *Inspector) GetPlainTextSize(")
	if idx == -1 {
		t.Fatalf("[P0] 10-6-AC7: could not locate Inspector.GetPlainTextSize in plaintext.go")
	}
	start := max(idx-2000, 0)
	preface := src[start:idx]
	lines := strings.Split(preface, "\n")
	commentLines := []string{}
	for i := len(lines) - 1; i >= 0; i-- {
		trim := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trim, "//") {
			commentLines = append([]string{trim}, commentLines...)
			continue
		}
		if trim == "" && len(commentLines) > 0 {
			continue
		}
		if len(commentLines) > 0 {
			break
		}
	}
	commentBlock := strings.Join(commentLines, "\n")
	// Pre-fix wording MUST be gone.
	if strings.Contains(commentBlock, "surfaces the raw os.Stat error") {
		t.Errorf("[P0] 10-6-AC7: GetPlainTextSize doc comment must NOT retain \"surfaces the raw os.Stat error when the file moves post-Open\" -- AC7 removes the re-stat and the contract changes")
	}
	// Post-fix wording: any of these phrases anchor the new contract.
	postFixMarkers := []string{
		"captured at Open",
		"size captured at Open",
		"stat-at-Open",
	}
	hit := false
	for _, m := range postFixMarkers {
		if strings.Contains(commentBlock, m) {
			hit = true
			break
		}
	}
	if !hit {
		t.Errorf("[P0] 10-6-AC7: GetPlainTextSize doc comment must affirm the value was \"captured at Open\" (AC7: post-Open moves/deletions do not affect this value). Got:\n%s", commentBlock)
	}
}

// Test_10_6_AC7_GetPlainTextThreadsSizeToReadPlainText [P0] AC#7: GetPlainText
// MUST pass `doc.FileSize` (or equivalent) to readPlainText. The pre-fix call
// is `readPlainText(ctx, doc.FilePath, tabID)`; post-fix MUST include the
// size argument.
func Test_10_6_AC7_GetPlainTextThreadsSizeToReadPlainText(t *testing.T) {
	src := readSource(t, "internal/pdfcore/plaintext.go")
	body := extractFunctionBody(t, src, "GetPlainText")
	if body == "" {
		t.Fatalf("[P0] 10-6-AC7: could not locate GetPlainText in plaintext.go")
	}
	if !strings.Contains(body, "doc.FileSize") {
		t.Errorf("[P0] 10-6-AC7: GetPlainText must pass `doc.FileSize` into readPlainText (AC7: caller threads cached size through)")
	}
	// Pre-fix call shape MUST be gone.
	preFix := "readPlainText(ctx, doc.FilePath, tabID)"
	if strings.Contains(body, preFix) {
		t.Errorf("[P0] 10-6-AC7: GetPlainText must NOT keep the pre-fix call `%s` -- AC7 inserts the size argument", preFix)
	}
}

// Test_10_6_AC7_GetPlainTextSizeAfterRemoveTestExists [P0] AC#7:
// TestGetPlainTextSizeAfterRemove MUST be declared in either inspector_test.go
// or plaintext_test.go. The spec says it opens a temp PDF, removes the file,
// and asserts GetPlainTextSize returns the original size without error.
func Test_10_6_AC7_GetPlainTextSizeAfterRemoveTestExists(t *testing.T) {
	root := projectRoot(t)
	needle := "func TestGetPlainTextSizeAfterRemove(t *testing.T)"
	candidates := []string{
		"internal/pdfcore/plaintext_test.go",
		"internal/pdfcore/inspector_test.go",
	}
	for _, c := range candidates {
		data, err := os.ReadFile(filepath.Join(root, c))
		if err != nil {
			continue
		}
		if strings.Contains(string(data), needle) {
			return
		}
	}
	t.Errorf("[P0] 10-6-AC7: %q must be declared in internal/pdfcore/plaintext_test.go (preferred) or inspector_test.go (AC7: opens temp PDF, removes file, asserts size returned without error)", needle)
}

// ---------------------------------------------------------------------------
// AC#8 -- baseline regression invariant: existing Story-10-1 tests survive
// ---------------------------------------------------------------------------

// Test_10_6_AC8_PlainTextAsyncTestsExist [P0] AC#8: the Story 10-1 async
// plaintext test surface MUST continue to exist after AC7's refactor. The
// test file is internal/pdfcore/plaintext_async_test.go.
//
// This is a baseline-invariant pin: passes pre-fix, MUST keep passing
// post-fix. AC7's audit step ("Audit any Story-10-1 test that asserts the
// post-Open-stat error path") may update assertions inside these tests,
// but the test functions themselves must remain.
func Test_10_6_AC8_PlainTextAsyncTestsExist(t *testing.T) {
	src := readSource(t, "internal/pdfcore/plaintext_async_test.go")
	// At least one Test function MUST be declared in this file.
	re := regexp.MustCompile(`(?m)^func Test\w+\(t \*testing\.T\)`)
	if !re.MatchString(src) {
		t.Errorf("[P0] 10-6-AC8: internal/pdfcore/plaintext_async_test.go must continue to declare at least one Test* function (AC8 baseline: Story 10-1 async test surface preserved)")
	}
}

// Test_10_6_AC8_SafeCallContractTestsPreserved [P0] AC#8: the safeCall
// contract tests pinned by Story 10-5 AC6 (and originally 10-4 STRUCT_010)
// MUST continue to exist. This story does not touch safeCall, but its
// fixture-corpus tests will run alongside the rest of the pdfcore suite,
// so the baseline invariant remains.
func Test_10_6_AC8_SafeCallContractTestsPreserved(t *testing.T) {
	src := readSource(t, "internal/pdfcore/errors_test.go")
	required := []string{
		"TestSafeCallSuccess",
		"TestSafeCallReturnsError",
		"TestSafeCallCatchesStringPanic",
		"TestSafeCallCatchesErrorPanic",
		"TestSafeCallPropagatesRuntimeError",
	}
	for _, name := range required {
		needle := "func " + name + "(t *testing.T)"
		if !strings.Contains(src, needle) {
			t.Errorf("[P0] 10-6-AC8: internal/pdfcore/errors_test.go must still declare %s (baseline invariant: safeCall contract unchanged by this story)", name)
		}
	}
}
