// Package pdf_parsing_correctness_fixes_test holds the per-story acceptance test suite for
// PDF Parsing and Data-Correctness Fixes.
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
// Run: cd tests/pdf-parsing-correctness-fixes && go test -v -count=1 ./...
package pdf_parsing_correctness_fixes_test

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
// Fixture corpus at testdata/correctness/ + README
// ---------------------------------------------------------------------------

// correctnessFixtures is the pinned list of fixture filenames. Each
// fixture demonstrates exactly one failure mode and is committed as a
// hand-authored uncompressed PDF (not generated at test time).
var correctnessFixtures = []string{
	"deep-nesting.pdf",
	"stream-length-indirect.pdf",
	"latin1-c1.pdf",
	"diff-out-of-range.pdf",
	"cjk-cmap-carry.pdf",
}

// TestFixtureCorpusExists asserts the five named PDF fixtures live under
// testdata/correctness/. Each is a hand-authored uncompressed PDF demonstrating one
// failure mode.
func TestFixtureCorpusExists(t *testing.T) {
	root := projectRoot(t)
	for _, name := range correctnessFixtures {
		path := filepath.Join(root, "testdata", "correctness", name)
		t.Run(name, func(t *testing.T) {
			info, err := os.Stat(path)
			if err != nil {
				t.Errorf("testdata/correctness/%s must exist (hand-authored uncompressed PDF demonstrating one failure mode)", name)
				return
			}
			if info.IsDir() {
				t.Errorf("testdata/correctness/%s must be a file, not a directory", name)
				return
			}
			if info.Size() == 0 {
				t.Errorf("testdata/correctness/%s must not be empty (must be a valid minimal PDF)", name)
			}
		})
	}
}

// TestFixtureCorpusReadme asserts a README.md sits alongside the fixtures,
// describing each fixture's purpose and byte-level structure well enough for a
// future engineer to regenerate it by hand.
func TestFixtureCorpusReadme(t *testing.T) {
	root := projectRoot(t)
	readmePath := filepath.Join(root, "testdata", "correctness", "README.md")
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("testdata/correctness/README.md must exist and describe each fixture's purpose and byte-level structure: %v", err)
	}
	body := string(data)
	for _, name := range correctnessFixtures {
		if !strings.Contains(body, name) {
			t.Errorf("testdata/correctness/README.md must mention %q (describe its purpose and byte-level structure)", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Drop depth cap in buildReachableSet AND findPathToObject
// ---------------------------------------------------------------------------

// TestBuildReachableSetNoDepthCap asserts buildReachableSet in
// internal/pdfcore/objectindex.go carries no `head.depth >= maxRefDepth` guard.
// The visited-set (entries map) already prevents cycles, and a depth cap mislabels
// legitimate-but-deep PDFs as orphan trees.
func TestBuildReachableSetNoDepthCap(t *testing.T) {
	src := readSource(t, "internal/pdfcore/objectindex.go")
	body := extractTopLevelFuncBody(t, src, "buildReachableSet")
	if body == "" {
		t.Fatalf("could not locate buildReachableSet in objectindex.go")
	}
	if strings.Contains(body, "head.depth >= maxRefDepth") {
		t.Errorf("buildReachableSet must NOT contain `head.depth >= maxRefDepth` -- drop the depth cap; the visited-set already prevents cycles")
	}
	// The reach-entry frame need no longer carry depth, but the spec leaves
	// that as a refactor preference. We do NOT pin the struct shape; only
	// the depth-test removal.
}

// TestFindPathToObjectNoDepthCap asserts findPathToObject in
// internal/pdfcore/inspector.go carries no `entry.depth >= maxRefDepth` guard
// either -- same package, same orphan-detection semantics.
func TestFindPathToObjectNoDepthCap(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	body := extractTopLevelFuncBody(t, src, "findPathToObject")
	if body == "" {
		t.Fatalf("could not locate findPathToObject in inspector.go")
	}
	if strings.Contains(body, "entry.depth >= maxRefDepth") {
		t.Errorf("findPathToObject must NOT contain `entry.depth >= maxRefDepth` -- drop the depth cap (visited-set already prevents cycles)")
	}
}

// TestTreeMaxRefDepthRetained asserts the maxRefDepth constant in
// internal/pdfcore/tree.go is retained for the page-tree caller, whose
// cycle-tolerance semantics differ.
func TestTreeMaxRefDepthRetained(t *testing.T) {
	src := readSource(t, "internal/pdfcore/tree.go")
	if !strings.Contains(src, "maxRefDepth = 32") {
		t.Errorf("internal/pdfcore/tree.go must retain `maxRefDepth = 32` for the page-tree caller (tree.go semantics unchanged by this story)")
	}
}

// TestBuildReachableSetTestExists asserts TestBuildReachableSetDeepNesting is
// declared in internal/pdfcore/objectindex_test.go, covering the boundary at depth
// 32 and well past it.
func TestBuildReachableSetTestExists(t *testing.T) {
	src := readSource(t, "internal/pdfcore/objectindex_test.go")
	needle := "func TestBuildReachableSetDeepNesting(t *testing.T)"
	if !strings.Contains(src, needle) {
		t.Errorf("internal/pdfcore/objectindex_test.go must declare %q (boundary-at-32 AND well-past-32, e.g. 50)", needle)
	}
}

// TestFindPathToObjectTestExists asserts TestFindPathToObjectDeepNesting is
// declared in internal/pdfcore/inspector_test.go.
func TestFindPathToObjectTestExists(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector_test.go")
	needle := "func TestFindPathToObjectDeepNesting(t *testing.T)"
	if !strings.Contains(src, needle) {
		t.Errorf("internal/pdfcore/inspector_test.go must declare %q (same depth-cap removal, behavioural pin)", needle)
	}
}

// ---------------------------------------------------------------------------
// extractStreamInfo gains doc *DocumentState; IndirectRef length resolved
// ---------------------------------------------------------------------------

// TestExtractStreamInfoSignature asserts the signature in
// internal/pdfcore/inspector.go is `func extractStreamInfo(doc *DocumentState, obj
// pdfcpu_types.Object) *StreamInfo`. Without the DocumentState handle the function
// cannot dereference IndirectRef lengths.
func TestExtractStreamInfoSignature(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	want := "func extractStreamInfo(doc *DocumentState, obj pdfcpu_types.Object) *StreamInfo"
	if !strings.Contains(src, want) {
		t.Errorf("internal/pdfcore/inspector.go must declare %q (doc handle threaded so IndirectRef /Length can be resolved via doc.PDFContext.Dereference)", want)
	}
	// And the pre-fix signature MUST be gone. Substring overlap would let
	// the post-fix line satisfy a contains check for the old signature,
	// so anchor with the closing paren of the old one-arg form.
	bad := "func extractStreamInfo(obj pdfcpu_types.Object) *StreamInfo"
	if strings.Contains(src, bad) {
		t.Errorf("internal/pdfcore/inspector.go must NOT keep the pre-fix one-arg signature %q (the doc handle is mandatory)", bad)
	}
}

// TestExtractStreamInfoDereferencesIndirect asserts the extractStreamInfo body
// handles a nil sd.StreamLength by inspecting sd.Dict["Length"]: an Integer is used
// directly and an IndirectRef is resolved via doc.PDFContext.Dereference. The
// load-bearing tokens are pinned.
func TestExtractStreamInfoDereferencesIndirect(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	body := extractTopLevelFuncBody(t, src, "extractStreamInfo")
	if body == "" {
		t.Fatalf("could not locate extractStreamInfo in inspector.go")
	}
	// Pre-fix body uses `sd.StreamLength != nil`. Post-fix MUST additionally
	// inspect sd.Dict["Length"] when StreamLength is nil. Loose-but-anchored
	// substring checks; phrasing left to the Dev.
	if !strings.Contains(body, `sd.Dict["Length"]`) {
		t.Errorf("extractStreamInfo must inspect `sd.Dict[\"Length\"]` when sd.StreamLength is nil (fallback path)")
	}
	if !strings.Contains(body, "doc.PDFContext.Dereference") {
		t.Errorf("extractStreamInfo must call `doc.PDFContext.Dereference` to resolve an IndirectRef length")
	}
	if !strings.Contains(body, "pdfcpu_types.IndirectRef") && !strings.Contains(body, "types.IndirectRef") {
		t.Errorf("extractStreamInfo must type-switch on `pdfcpu_types.IndirectRef` to drive the dereference branch")
	}
	if !strings.Contains(body, "pdfcpu_types.Integer") && !strings.Contains(body, "types.Integer") {
		t.Errorf("extractStreamInfo must type-switch on `pdfcpu_types.Integer` for the direct-integer branch")
	}
}

// TestGetObjectDetailPassesDoc asserts the three extractStreamInfo call sites
// inside GetObjectDetail pass `doc` as the first argument, i.e.
// `extractStreamInfo(doc, obj)`.
func TestGetObjectDetailPassesDoc(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	body := extractFunctionBody(t, src, "GetObjectDetail")
	if body == "" {
		t.Fatalf("could not locate GetObjectDetail in inspector.go")
	}
	// Count post-fix call shape occurrences. Spec says 3 call sites (one per
	// StreamDict / ObjectStreamDict / XRefStreamDict branch).
	postFixCount := strings.Count(body, "extractStreamInfo(doc,")
	if postFixCount < 3 {
		t.Errorf("GetObjectDetail must invoke `extractStreamInfo(doc, ...)` at all 3 stream-type branches (got %d occurrences of that signature in the function body)", postFixCount)
	}
	// And the pre-fix single-arg call shape MUST be absent in this function.
	preFixRe := regexp.MustCompile(`extractStreamInfo\(obj\)`)
	if preFixRe.MatchString(body) {
		t.Errorf("GetObjectDetail must NOT retain the single-argument `extractStreamInfo(obj)` call -- doc is threaded through")
	}
}

// TestIndirectLengthTestExists asserts TestExtractStreamInfoIndirectLength is
// declared in internal/pdfcore/inspector_test.go. That test also needs a pre-step
// asserting sd.StreamLength == nil for the fixture's stream object.
func TestIndirectLengthTestExists(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector_test.go")
	needle := "func TestExtractStreamInfoIndirectLength(t *testing.T)"
	if !strings.Contains(src, needle) {
		t.Errorf("internal/pdfcore/inspector_test.go must declare %q (opens fixture, asserts non-zero length AND that StreamLength was nil pre-fix)", needle)
	}
}

// ---------------------------------------------------------------------------
// latin1Decode doc comment rewrite + full-range pinning test
// ---------------------------------------------------------------------------

// TestLatin1DecodeBodyUnchanged pins the load-bearing branch of latin1Decode in
// internal/pdfcore/plaintext.go: the rune(b) cast inside the 0x09/0x0A/0x0D and
// 0x20 <= c != 0x7F branches. The implementation must not change, so nobody can
// "fix" the C1 behavior by extending the U+FFFD replacement range.
func TestLatin1DecodeBodyUnchanged(t *testing.T) {
	src := readSource(t, "internal/pdfcore/plaintext.go")
	body := extractTopLevelFuncBody(t, src, "latin1Decode")
	if body == "" {
		t.Fatalf("could not locate latin1Decode in plaintext.go")
	}
	// Pre-existing branches that says MUST stay:
	if !strings.Contains(body, "c == 0x09 || c == 0x0A || c == 0x0D") {
		t.Errorf("latin1Decode must retain the `c == 0x09 || c == 0x0A || c == 0x0D` whitespace passthrough branch (implementation unchanged)")
	}
	if !strings.Contains(body, "c >= 0x20 && c != 0x7F") {
		t.Errorf("latin1Decode must retain the `c >= 0x20 && c != 0x7F` Latin-1 passthrough branch (C1 and 0xA0-0xFF map verbatim via rune(c))")
	}
	if !strings.Contains(body, "sb.WriteRune(rune(c))") {
		t.Errorf("latin1Decode must retain `sb.WriteRune(rune(c))` for the passthrough branches (lossless byte-for-codepoint mapping)")
	}
}

// TestLatin1FullRangeUnitTestExists asserts TestLatin1DecodeFullRange is declared
// in internal/pdfcore/plaintext_test.go, pinning the byte-for-codepoint contract
// for every byte 0x00-0xFF via a direct call with no fixture.
func TestLatin1FullRangeUnitTestExists(t *testing.T) {
	src := readSource(t, "internal/pdfcore/plaintext_test.go")
	needle := "func TestLatin1DecodeFullRange(t *testing.T)"
	if !strings.Contains(src, needle) {
		t.Errorf("internal/pdfcore/plaintext_test.go must declare %q (pins the byte-for-codepoint contract for all 256 bytes via direct latin1Decode call)", needle)
	}
}

// TestLatin1C1IntegrationTestExists asserts TestGetPlainTextLatin1C1 is declared in
// plaintext_test.go or inspector_test.go. That is the integration test which opens
// the fixture and asserts the C1 region maps verbatim.
func TestLatin1C1IntegrationTestExists(t *testing.T) {
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
	t.Errorf("%q must be declared in internal/pdfcore/plaintext_test.go (preferred) or inspector_test.go (integration test against testdata/correctness/latin1-c1.pdf)", needle)
}

// ---------------------------------------------------------------------------
// parseDifferences out-of-range guard
// ---------------------------------------------------------------------------

// TestParseDifferencesBoundsGuard asserts parseDifferences in
// internal/pdfcore/font.go carries `if currentCode < 0 || currentCode > 255 {
// continue }` after the `currentCode = int(v)` line. Without it, /Differences
// arrays holding out-of-range integers (-1, 999) leak rows into the encoding table
// with garbage codes.
func TestParseDifferencesBoundsGuard(t *testing.T) {
	src := readSource(t, "internal/pdfcore/font.go")
	body := extractTopLevelFuncBody(t, src, "parseDifferences")
	if body == "" {
		t.Fatalf("could not locate parseDifferences in font.go")
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
		t.Errorf("parseDifferences must contain a guard rejecting `currentCode < 0` and `currentCode > 255` after the `currentCode = int(v)` line (out-of-range codes skipped silently). Acceptable shapes: %v", candidatePatterns)
	}
}

// TestParseDifferencesOutOfRangeTestExists asserts
// TestParseDifferencesOutOfRange is declared in internal/pdfcore/font_test.go,
// calling parseDifferences directly with a synthesized array (Integer(-1),
// Integer(999), and a name).
func TestParseDifferencesOutOfRangeTestExists(t *testing.T) {
	src := readSource(t, "internal/pdfcore/font_test.go")
	needle := "func TestParseDifferencesOutOfRange(t *testing.T)"
	if !strings.Contains(src, needle) {
		t.Errorf("internal/pdfcore/font_test.go must declare %q (synthesized array with Integer(-1), Integer(999), and a name)", needle)
	}
}

// ---------------------------------------------------------------------------
// parseBfrange overflow break replaced with carry propagation
// ---------------------------------------------------------------------------

// TestParseBfrangeNoSilentBreak asserts parseBfrange in internal/pdfcore/font.go
// carries no `if tail > 0xFFFF { break }` overflow bail-out, which is silent data
// loss for ranges that legitimately cross UTF-16 unit boundaries, and that a
// carry-propagation marker is present instead.
func TestParseBfrangeNoSilentBreak(t *testing.T) {
	src := readSource(t, "internal/pdfcore/font.go")
	body := extractTopLevelFuncBody(t, src, "parseBfrange")
	if body == "" {
		t.Fatalf("could not locate parseBfrange in font.go")
	}
	// The pre-fix pattern is the *combination* of `tail > 0xFFFF` AND `break`
	// in a context with no carry propagation. Loose check: the post-fix
	// MUST NOT contain the standalone `if tail > 0xFFFF {\n\t\t\t\tbreak`.
	silentBreakRe := regexp.MustCompile(`if\s+tail\s*>\s*0xFFFF\s*{\s*break\s*}`)
	if silentBreakRe.MatchString(body) {
		t.Errorf("parseBfrange must NOT keep the silent `if tail > 0xFFFF { break }` overflow handler -- carry propagation across UTF-16 units replaces it")
	}
}

// TestParseBfrangeCarryShape asserts parseBfrange carries into higher UTF-16 units
// when the trailing unit overflows, pinning the load-bearing tokens: units[last]
// set to `tail & 0xFFFF` and `tail >> 16` propagated into the next-higher unit.
func TestParseBfrangeCarryShape(t *testing.T) {
	src := readSource(t, "internal/pdfcore/font.go")
	body := extractTopLevelFuncBody(t, src, "parseBfrange")
	if body == "" {
		t.Fatalf("could not locate parseBfrange in font.go")
	}
	// Look for the bit-mask write into the trailing unit. Acceptable forms:
	//   advanced[last] = uint16(tail & 0xFFFF)
	//   advanced[len(advanced)-1] = uint16(tail & 0xFFFF)
	//   units[len(units)-1] = uint16(tail & 0xFFFF)
	maskRe := regexp.MustCompile(`uint16\(\s*tail\s*&\s*0xFFFF\s*\)`)
	if !maskRe.MatchString(body) {
		t.Errorf("parseBfrange must write the trailing UTF-16 unit as `uint16(tail & 0xFFFF)` (carry implementation)")
	}
	// Look for the high-bit propagation. Acceptable forms:
	//   tail >> 16
	//   carry := tail >> 16
	shiftRe := regexp.MustCompile(`tail\s*>>\s*16`)
	if !shiftRe.MatchString(body) {
		t.Errorf("parseBfrange must propagate the carry via `tail >> 16` into a higher UTF-16 unit (carry implementation)")
	}
}

// TestParseBfrangePreLoopSpanCheckUnchanged asserts the pre-loop span check in
// internal/pdfcore/font.go (`high-low+1 > maxBfrangeSpan`) is untouched. The carry
// handling concerns the per-iteration trailing-unit overflow only.
func TestParseBfrangePreLoopSpanCheckUnchanged(t *testing.T) {
	src := readSource(t, "internal/pdfcore/font.go")
	body := extractTopLevelFuncBody(t, src, "parseBfrange")
	if body == "" {
		t.Fatalf("could not locate parseBfrange in font.go")
	}
	if !strings.Contains(body, "high-low+1 > maxBfrangeSpan") {
		t.Errorf("parseBfrange must retain the pre-loop span check `if high-low+1 > maxBfrangeSpan` (pre-loop span check unchanged)")
	}
}

// TestParseBfrangeCarryTestExists asserts TestParseBfrangeCarry is declared in
// internal/pdfcore/font_test.go, covering three cases: trailing-unit carry into a
// higher unit, leading-unit overflow stopping the loop, and the pre-loop span-cap
// rejection still returning an error.
func TestParseBfrangeCarryTestExists(t *testing.T) {
	src := readSource(t, "internal/pdfcore/font_test.go")
	needle := "func TestParseBfrangeCarry(t *testing.T)"
	if !strings.Contains(src, needle) {
		t.Errorf("internal/pdfcore/font_test.go must declare %q (covers trailing-unit carry, leading-unit overflow stop, AND the unchanged pre-loop span-cap rejection)", needle)
	}
}

// ---------------------------------------------------------------------------
// DocumentState.FileSize captured at Open; redundant os.Stat removed
// ---------------------------------------------------------------------------

// TestDocumentStateHasFileSize asserts DocumentState declares a `FileSize int64`
// field. The field is pinned by name, so a rename or relocation fails.
func TestDocumentStateHasFileSize(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	// Anchor: field declaration shape on its own line.
	re := regexp.MustCompile(`(?m)^\s*FileSize\s+int64\b`)
	if !re.MatchString(src) {
		t.Errorf("internal/pdfcore/inspector.go must declare `FileSize int64` as a field on DocumentState (stat-at-Open value, surfaced by GetPlainTextSize and threaded into readPlainText)")
	}
}

// TestOpenPopulatesFileSize asserts Inspector.Open sets FileSize inside the `doc
// := &DocumentState{...}` literal, not only inside the DocumentInfo literal. The
// value is already captured as `fileSize := fi.Size()`.
func TestOpenPopulatesFileSize(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	body := extractFunctionBody(t, src, "Open")
	if body == "" {
		t.Fatalf("could not locate Inspector.Open in inspector.go")
	}
	// Anchor on the DocumentState struct literal specifically. Walk from
	// `doc := &DocumentState{` to the matching closing `}` (brace count of 1
	// is enough since the literal has no nested braces in the baseline).
	openStart := strings.Index(body, "doc := &DocumentState{")
	if openStart == -1 {
		t.Fatalf("could not locate `doc:= &DocumentState{` in Inspector.Open body")
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
		t.Fatalf("could not find closing brace for DocumentState literal in Inspector.Open")
	}
	literal := body[openStart:end]
	if !strings.Contains(literal, "FileSize:") {
		t.Errorf("Inspector.Open must include `FileSize: fileSize` inside the `doc:= &DocumentState{...}` literal (stat-at-Open value cached on DocumentState). Got literal:\n%s", literal)
	}
	// And the value MUST be the local `fileSize` variable, not a fresh stat.
	if !strings.Contains(literal, "FileSize: fileSize") && !strings.Contains(literal, "FileSize:  fileSize") {
		t.Errorf("Inspector.Open must populate FileSize from the local `fileSize` variable captured at the start of Open (reuse the existing stat result; do NOT re-stat). Got literal:\n%s", literal)
	}
}

// TestGetPlainTextSizeUsesCachedField asserts Inspector.GetPlainTextSize in
// internal/pdfcore/plaintext.go returns `doc.FileSize` directly and makes no
// `os.Stat(doc.FilePath)` call.
func TestGetPlainTextSizeUsesCachedField(t *testing.T) {
	src := readSource(t, "internal/pdfcore/plaintext.go")
	body := extractFunctionBody(t, src, "GetPlainTextSize")
	if body == "" {
		t.Fatalf("could not locate GetPlainTextSize in plaintext.go")
	}
	if strings.Contains(body, "os.Stat(") {
		t.Errorf("GetPlainTextSize must NOT call `os.Stat(...)` -- it returns the size captured at Open")
	}
	if !strings.Contains(body, "doc.FileSize") {
		t.Errorf("GetPlainTextSize must return `doc.FileSize` (cached-at-Open value)")
	}
}

// TestReadPlainTextSignatureTakesSize asserts the readPlainText helper in
// internal/pdfcore/plaintext.go takes a `size int64` parameter, so it needs no
// in-function `os.Stat(path)` call. Only the presence of `size int64` is pinned,
// not the parameter ordering.
func TestReadPlainTextSignatureTakesSize(t *testing.T) {
	src := readSource(t, "internal/pdfcore/plaintext.go")
	// Anchor on `func readPlainText(` and assert `size int64` appears in
	// the parameter list before the next `)`.
	idx := strings.Index(src, "func readPlainText(")
	if idx == -1 {
		t.Fatalf("could not locate readPlainText in plaintext.go")
	}
	tail := src[idx:]
	closeIdx := strings.Index(tail, ")")
	if closeIdx == -1 {
		t.Fatalf("malformed readPlainText signature in plaintext.go (no closing paren)")
	}
	signature := tail[:closeIdx+1]
	if !strings.Contains(signature, "size int64") {
		t.Errorf("readPlainText signature must include `size int64` (caller threads doc.FileSize through). Got: %s", signature)
	}
}

// TestReadPlainTextNoStat asserts the readPlainText body does not call
// `os.Stat(path)`: totalBytes comes from the passed-in `size` argument.
func TestReadPlainTextNoStat(t *testing.T) {
	src := readSource(t, "internal/pdfcore/plaintext.go")
	body := extractTopLevelFuncBody(t, src, "readPlainText")
	if body == "" {
		t.Fatalf("could not locate readPlainText in plaintext.go")
	}
	if strings.Contains(body, "os.Stat(") {
		t.Errorf("readPlainText must NOT call `os.Stat(...)` -- it uses the passed-in `size` argument")
	}
}

// TestGetPlainTextThreadsSizeToReadPlainText asserts GetPlainText passes
// `doc.FileSize` (or equivalent) into readPlainText rather than calling it with
// path and tabID alone.
func TestGetPlainTextThreadsSizeToReadPlainText(t *testing.T) {
	src := readSource(t, "internal/pdfcore/plaintext.go")
	body := extractFunctionBody(t, src, "GetPlainText")
	if body == "" {
		t.Fatalf("could not locate GetPlainText in plaintext.go")
	}
	if !strings.Contains(body, "doc.FileSize") {
		t.Errorf("GetPlainText must pass `doc.FileSize` into readPlainText (caller threads cached size through)")
	}
	// Pre-fix call shape MUST be gone.
	preFix := "readPlainText(ctx, doc.FilePath, tabID)"
	if strings.Contains(body, preFix) {
		t.Errorf("GetPlainText must NOT keep the size-less call `%s` -- the cached size is passed in", preFix)
	}
}

// TestGetPlainTextSizeAfterRemoveTestExists asserts
// TestGetPlainTextSizeAfterRemove is declared in inspector_test.go or
// plaintext_test.go: it opens a temp PDF, removes the file, and asserts
// GetPlainTextSize returns the original size without error.
func TestGetPlainTextSizeAfterRemoveTestExists(t *testing.T) {
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
	t.Errorf("%q must be declared in internal/pdfcore/plaintext_test.go (preferred) or inspector_test.go (opens temp PDF, removes file, asserts size returned without error)", needle)
}

// ---------------------------------------------------------------------------
// Baseline regression invariant: the existing async plain-text tests survive
// ---------------------------------------------------------------------------

// TestPlainTextAsyncTestsExist asserts the async plaintext test surface in
// internal/pdfcore/plaintext_async_test.go still exists. It is a baseline
// invariant: assertions inside those tests may be updated as the post-Open-stat
// error path changes, but the test functions themselves must remain.
func TestPlainTextAsyncTestsExist(t *testing.T) {
	src := readSource(t, "internal/pdfcore/plaintext_async_test.go")
	// At least one Test function MUST be declared in this file.
	re := regexp.MustCompile(`(?m)^func Test\w+\(t \*testing\.T\)`)
	if !re.MatchString(src) {
		t.Errorf("internal/pdfcore/plaintext_async_test.go must continue to declare at least one Test* function (the async plain-text test surface must be preserved)")
	}
}

// TestSafeCallContractTestsPreserved asserts the safeCall contract tests still
// exist. Nothing here touches safeCall, but the fixture-corpus tests run alongside
// the rest of the pdfcore suite, so the baseline invariant is worth pinning.
func TestSafeCallContractTestsPreserved(t *testing.T) {
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
			t.Errorf("internal/pdfcore/errors_test.go must still declare %s (baseline invariant: safeCall contract unchanged by this story)", name)
		}
	}
}
