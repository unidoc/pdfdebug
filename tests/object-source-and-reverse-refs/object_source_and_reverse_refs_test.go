// Package object_source_and_reverse_refs_test provides acceptance tests for
// Story 9.10: Object Source View + Reverse References.
//
// TDD RED PHASE: these tests MUST fail until Story 9-10 is implemented.
//
// Test pyramid for this story:
//   - Backend AC1, AC3, AC5 (Object Source serialization) -> pdfcore Go unit
//     tests delegated via runPdfcoreTest. Source-format determinism is best
//     verified in-process where we can hand-craft fixtures and assert exact
//     output bytes.
//   - Backend AC6, AC9, AC11 (Reverse-ref index lifecycle and failure mode)
//     -> pdfcore Go unit + integration tests delegated via runPdfcoreTest.
//   - Backend AC7 (ReverseRef shape) -> structural assertions on model.go.
//   - Wails plumbing -> structural assertions on service.go.
//   - Frontend AC2, AC4, AC7, AC8, AC10, AC12 -> Vitest. Delegated here only
//     via structural checks that the right files/exports/data-testids exist,
//     because behavior is best asserted in the component tests themselves.
//
// No Playwright/E2E layer: every acceptance criterion in this story is fully
// observable at the API level (Object Source string, ReverseRef list) or the
// component level (state + dispatched actions). Adding a browser layer would
// repeat what the component tests already cover and contradict the test
// pyramid for this scope.
//
// Run: cd tests/object-source-and-reverse-refs && go test -v -count=1 ./...
package object_source_and_reverse_refs_test

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

// testdataDir returns the absolute path to the testdata/ directory at project root.
func testdataDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(projectRoot(t), "testdata")
}

// runPdfcoreTest runs a named test pattern in internal/pdfcore/... and fails
// if the test does not pass or does not exist. Identical pattern to
// tests/reference-navigation/reference_navigation_test.go so dev removes
// t.Skip in the underlying pdfcore tests in lockstep with implementation.
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
// AC#1 -- Object Source reserializes indirect objects as PDF syntax
// ---------------------------------------------------------------------------

// 9.10-INTG-001 [P0]: objectsource.go exists and defines GetObjectSource on Inspector.
func TestObjectSourceFileExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "internal", "pdfcore", "objectsource.go")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("[P0] 9.10-INTG-001: internal/pdfcore/objectsource.go does not exist")
	}
	src := readSource(t, "internal/pdfcore/objectsource.go")
	if !strings.Contains(src, "GetObjectSource") {
		t.Fatalf("[P0] 9.10-INTG-001: objectsource.go must declare GetObjectSource method on Inspector")
	}
}

// 9.10-INTG-002 [P0]: pdfcore unit test covers dict/array short/long forms (AC1).
func TestObjectSourceDictAndArrayForms(t *testing.T) {
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("[P0] 9.10-INTG-002: testdata/minimal.pdf missing")
	}
	runPdfcoreTest(t, "TestObjectSourceSerializeForms")
}

// 9.10-INTG-003 [P0]: indirect refs in serialized output are emitted as
// `N G R` literals, NOT dereferenced (so cyclic refs cannot stack-overflow).
func TestObjectSourceIndirectRefsNotDereferenced(t *testing.T) {
	runPdfcoreTest(t, "TestObjectSourceRefsEmittedNotDereferenced")
}

// 9.10-INTG-004 [P0]: cycle protection -- a page tree with /Parent self-loop
// must not stack-overflow during serialization.
func TestObjectSourceCycleSafe(t *testing.T) {
	runPdfcoreTest(t, "TestObjectSourceCycleSafe")
}

// 9.10-INTG-005 [P1]: deterministic dict key ordering (sorted ascending).
// Without this, golden tests would be flaky on Go map iteration.
func TestObjectSourceDeterministicKeyOrder(t *testing.T) {
	runPdfcoreTest(t, "TestObjectSourceDictKeysSorted")
}

// ---------------------------------------------------------------------------
// AC#3 -- inline-node selection returns ("", nil)
// ---------------------------------------------------------------------------

// 9.10-INTG-006 [P0]: GetObjectSource on a dict-entry / array-element node ID
// returns the empty string with no error, so the frontend can render the AC3
// empty-state copy.
func TestObjectSourceInlineNodeReturnsEmpty(t *testing.T) {
	runPdfcoreTest(t, "TestObjectSourceInlineNodeIDReturnsEmptyString")
}

// ---------------------------------------------------------------------------
// AC#5 -- stream objects render dict + placeholder + envelope
// ---------------------------------------------------------------------------

// 9.10-INTG-007 [P0]: stream serialization emits dict, then `stream` /
// `endstream` markers with the byte-count placeholder, NOT the raw bytes.
func TestObjectSourceStreamPlaceholder(t *testing.T) {
	runPdfcoreTest(t, "TestObjectSourceStreamPlaceholder")
}

// 9.10-INTG-008 [P1]: truncation rule -- output capped at the byte limit
// (default 256KB) only between top-level entries, always emits the closing
// bracket/brace and `endobj`, always emits the truncation marker.
func TestObjectSourceTruncation(t *testing.T) {
	runPdfcoreTest(t, "TestObjectSourceTruncationRule")
}

// ---------------------------------------------------------------------------
// AC#6 -- reverse-ref index built at document open, with failure mode
// ---------------------------------------------------------------------------

// 9.10-INTG-009 [P0]: reverserefs.go exists and declares GetReverseRefs on
// the Inspector.
func TestReverseRefsFileExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "internal", "pdfcore", "reverserefs.go")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("[P0] 9.10-INTG-009: internal/pdfcore/reverserefs.go does not exist")
	}
	src := readSource(t, "internal/pdfcore/reverserefs.go")
	if !strings.Contains(src, "GetReverseRefs") {
		t.Fatalf("[P0] 9.10-INTG-009: reverserefs.go must declare GetReverseRefs method on Inspector")
	}
}

// 9.10-INTG-010 [P0]: DocumentState carries the reverse-ref map AND the
// revRefsBuildFailed flag (distinct from "empty"). Without the flag, an
// index-build panic would silently mislabel every object as orphaned.
func TestDocumentStateCarriesReverseRefsFields(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	// The reverse-ref map and the failure flag both live on DocumentState
	// alongside streamCache. We do not pin the exact field name here, but we
	// do require BOTH a reverse-refs storage field and a build-failed flag.
	if !strings.Contains(src, "revRefsBuildFailed") {
		t.Fatalf("[P0] 9.10-INTG-010: inspector.go DocumentState must carry revRefsBuildFailed bool")
	}
	if !strings.Contains(src, "ReverseRef") {
		t.Fatalf("[P0] 9.10-INTG-010: inspector.go must reference ReverseRef (index storage)")
	}
}

// 9.10-INTG-011 [P0]: ErrReverseRefIndexUnavailable sentinel error is declared.
// The frontend uses it (per AC6 / Task 6.5 case 1) to render the unavailable
// banner. Empty list MUST mean "no incoming refs"; failure mode is signalled
// by this sentinel, not by an empty slice.
func TestReverseRefSentinelErrorDeclared(t *testing.T) {
	src := readSource(t, "internal/pdfcore/errors.go")
	if !strings.Contains(src, "ErrReverseRefIndexUnavailable") {
		t.Fatalf("[P0] 9.10-INTG-011: errors.go must declare ErrReverseRefIndexUnavailable sentinel")
	}
}

// 9.10-INTG-012 [P0]: index build happens at Open() inside safeCall. We can
// only verify this structurally from the integration layer: the source file
// must call into the build entry point inside Open, and the build must be
// wrapped by safeCall.
func TestReverseRefIndexBuiltAtOpen(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	if !strings.Contains(src, "buildReverseRefs") && !strings.Contains(src, "BuildReverseRefs") {
		t.Fatalf("[P0] 9.10-INTG-012: inspector.go Open must call a reverse-refs build entry point (buildReverseRefs / BuildReverseRefs)")
	}
	if !strings.Contains(src, "safeCall") {
		t.Fatalf("[P0] 9.10-INTG-012: inspector.go must use safeCall (already true today; check preserved)")
	}
}

// 9.10-INTG-013 [P0]: pdfcore unit test verifies the build walks the full
// graph from /Root using a visited set keyed by (num, gen) and emits a
// ReverseRef per outbound indirect ref encountered in dict/array/stream-dict
// containers (matching findPathToObject's container set).
func TestReverseRefIndexBuildFromCatalog(t *testing.T) {
	multipagePDF := filepath.Join(testdataDir(t), "multipage.pdf")
	if _, err := os.Stat(multipagePDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("[P0] 9.10-INTG-013: testdata/multipage.pdf missing")
	}
	runPdfcoreTest(t, "TestReverseRefIndexBuildPopulatesPagesAndKids")
}

// 9.10-INTG-014 [P0]: build panic sets revRefsBuildFailed=true and GetReverseRefs
// returns ErrReverseRefIndexUnavailable -- verifies the AC6 failure mode.
func TestReverseRefIndexFailureModeReturnsSentinel(t *testing.T) {
	runPdfcoreTest(t, "TestReverseRefIndexBuildPanicSurfacesSentinel")
}

// 9.10-INTG-015 [P1]: visited-set keyed by (num, gen) only, NO depth cap.
// maxRefDepth would silently exclude deeply nested objects and falsely mark
// them orphan. This test verifies the source does NOT reuse maxRefDepth in
// the new index build.
func TestReverseRefIndexUsesVisitedSetNotDepthCap(t *testing.T) {
	src := readSource(t, "internal/pdfcore/reverserefs.go")
	// The new file must NOT use maxRefDepth; cycles are caught by the
	// visited set alone. (We grep for the constant by name; if dev wants to
	// rename the constant later, they update this test alongside the rename.)
	if strings.Contains(src, "maxRefDepth") {
		t.Fatalf("[P1] 9.10-INTG-015: reverserefs.go MUST NOT reuse maxRefDepth -- cycle protection lives in the visited set keyed by (num, gen). Using a depth cap would falsely mark deeply nested objects as orphans.")
	}
	// And the source must reference a visited construct (set/map).
	if !strings.Contains(src, "visited") {
		t.Fatalf("[P1] 9.10-INTG-015: reverserefs.go must use a visited set/map for cycle protection")
	}
}

// ---------------------------------------------------------------------------
// AC#7 -- ReverseRef shape (ParentNodeID, ParentRef, ParentType, Path)
// ---------------------------------------------------------------------------

// 9.10-INTG-016 [P0]: model.go declares the ReverseRef struct with the exact
// JSON shape the frontend expects. ParentType MUST be *string so the
// frontend can distinguish "absent" from "empty value" -- this is load-bearing.
func TestReverseRefStructShape(t *testing.T) {
	src := readSource(t, "internal/pdfcore/model.go")
	if !strings.Contains(src, "ReverseRef") {
		t.Fatalf("[P0] 9.10-INTG-016: model.go must declare type ReverseRef")
	}
	requiredFields := []string{
		`ParentNodeID`,
		`ParentRef`,
		`ParentType`,
		`Path`,
	}
	for _, f := range requiredFields {
		if !strings.Contains(src, f) {
			t.Fatalf("[P0] 9.10-INTG-016: ReverseRef must declare field %q", f)
		}
	}
	requiredJSONTags := []string{
		`json:"parentNodeId"`,
		`json:"parentRef"`,
		`json:"parentType,omitempty"`,
		`json:"path"`,
	}
	for _, tag := range requiredJSONTags {
		if !strings.Contains(src, tag) {
			t.Fatalf("[P0] 9.10-INTG-016: ReverseRef must declare JSON tag %q -- frontend serialization contract", tag)
		}
	}
	// ParentType MUST be *string (omitempty pointer). A non-pointer type would
	// lose the "absent vs empty" distinction required by Task 6.4.
	if !strings.Contains(src, "ParentType *string") {
		t.Fatalf("[P0] 9.10-INTG-016: ParentType must be *string so the frontend can distinguish 'key absent' from 'value is empty name'")
	}
}

// 9.10-INTG-017 [P1]: returned entries are ordered by ParentRef ascending,
// with Path ascending as the secondary key. Two refs from the same parent
// into different /Kids slots MUST have deterministic display order so the
// section does not jitter between calls.
func TestReverseRefEntriesDeterministicOrdering(t *testing.T) {
	runPdfcoreTest(t, "TestReverseRefEntriesSortedByParentThenPath")
}

// 9.10-INTG-018 [P1]: catalog has zero reverse-refs by construction. The
// trailer's /Root pointer is NOT recorded; the catalog is the BFS source.
// AC10's "Document root" copy distinguishes catalog from orphan via tree
// iconHint, not via the index, so the index-level emptiness must be
// observable here.
func TestReverseRefIndexCatalogHasZeroEntries(t *testing.T) {
	multipagePDF := filepath.Join(testdataDir(t), "multipage.pdf")
	if _, err := os.Stat(multipagePDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("[P1] 9.10-INTG-018: testdata/multipage.pdf missing")
	}
	runPdfcoreTest(t, "TestReverseRefIndexCatalogIsEmpty")
}

// 9.10-INTG-019 [P1]: an orphan-injected fixture returns an empty list (NOT
// the sentinel error). This is the AC9 contract: empty list == no incoming
// dict-graph refs (possible orphan).
func TestReverseRefIndexOrphanReturnsEmptyList(t *testing.T) {
	runPdfcoreTest(t, "TestReverseRefIndexOrphanObjectHasEmptyList")
}

// ---------------------------------------------------------------------------
// AC#11 -- per-document index across tab switches
// ---------------------------------------------------------------------------

// 9.10-INTG-020 [P1]: opening two documents in two tabs yields two
// independent indexes; queries against one are unaffected by the other.
func TestReverseRefIndexPerDocument(t *testing.T) {
	runPdfcoreTest(t, "TestReverseRefIndexPerDocumentIsolation")
}

// ---------------------------------------------------------------------------
// Wails service plumbing (Task 3)
// ---------------------------------------------------------------------------

// 9.10-INTG-021 [P0]: PDFService.GetObjectSource is exposed.
func TestServiceExposesGetObjectSource(t *testing.T) {
	src := readSource(t, "internal/pdfservice/service.go")
	if !strings.Contains(src, "GetObjectSource") {
		t.Fatalf("[P0] 9.10-INTG-021: service.go must expose GetObjectSource")
	}
}

// 9.10-INTG-022 [P0]: PDFService.GetReverseRefs is exposed and returns a slice
// of *pdfcore.ReverseRef (matching Task 3.1).
func TestServiceExposesGetReverseRefs(t *testing.T) {
	src := readSource(t, "internal/pdfservice/service.go")
	if !strings.Contains(src, "GetReverseRefs") {
		t.Fatalf("[P0] 9.10-INTG-022: service.go must expose GetReverseRefs")
	}
	// Return type signature -- the frontend bindings depend on the exact slice
	// element type. Loose match: must contain *pdfcore.ReverseRef in the
	// signature line.
	if !strings.Contains(src, "[]*pdfcore.ReverseRef") {
		t.Fatalf("[P0] 9.10-INTG-022: service.go GetReverseRefs must return []*pdfcore.ReverseRef")
	}
}

// ---------------------------------------------------------------------------
// AC#2, AC#3, AC#4, AC#7, AC#8, AC#10, AC#12 -- frontend wiring (structural)
// ---------------------------------------------------------------------------
//
// Behavior assertions for these criteria live in the Vitest files
// (ObjectInfoPanel.test.tsx, ReverseRefsSection.test.tsx, DetailPanel.test.tsx).
// We assert only that the required files/exports/testids exist so the Vitest
// suites can target them. The full behavior contract is asserted in Vitest.

// 9.10-STRUCT-001 [P0]: ObjectInfoPanel.tsx still exists (file is NOT renamed
// per Task 5.1) and now exports ObjectSourcePanel.
func TestObjectInfoPanelRenamedExport(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "ObjectInfoPanel.tsx")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("[P0] 9.10-STRUCT-001: ObjectInfoPanel.tsx must still exist (in-place rewrite, file NOT renamed)")
	}
	src := readSource(t, "frontend/src/components/ObjectInfoPanel.tsx")
	if !strings.Contains(src, "ObjectSourcePanel") {
		t.Fatalf("[P0] 9.10-STRUCT-001: ObjectInfoPanel.tsx must export ObjectSourcePanel (the renamed component per Task 5.1)")
	}
	// The new data-testid replaces the old one.
	if !strings.Contains(src, `"object-source-panel"`) {
		t.Fatalf("[P0] 9.10-STRUCT-001: ObjectInfoPanel.tsx must use data-testid \"object-source-panel\" (Task 5.1)")
	}
	if strings.Contains(src, `"object-info-panel"`) {
		t.Fatalf("[P0] 9.10-STRUCT-001: ObjectInfoPanel.tsx must NOT keep the old data-testid \"object-info-panel\" -- replace with \"object-source-panel\"")
	}
}

// 9.10-STRUCT-002 [P0]: MainLayout.tsx imports ObjectSourcePanel (not the old
// ObjectInfoPanel symbol).
func TestMainLayoutImportsObjectSourcePanel(t *testing.T) {
	src := readSource(t, "frontend/src/components/MainLayout.tsx")
	if !strings.Contains(src, "ObjectSourcePanel") {
		t.Fatalf("[P0] 9.10-STRUCT-002: MainLayout.tsx must import ObjectSourcePanel (Task 5.1)")
	}
}

// 9.10-STRUCT-003 [P0]: the new ReverseRefsSection component exists.
func TestReverseRefsSectionExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "ReverseRefsSection.tsx")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("[P0] 9.10-STRUCT-003: frontend/src/components/ReverseRefsSection.tsx must exist (Task 6.1)")
	}
	src := readSource(t, "frontend/src/components/ReverseRefsSection.tsx")
	if !strings.Contains(src, "export") || !strings.Contains(src, "ReverseRefsSection") {
		t.Fatalf("[P0] 9.10-STRUCT-003: ReverseRefsSection.tsx must export ReverseRefsSection")
	}
}

// 9.10-STRUCT-004 [P0]: DetailPanel.tsx mounts ReverseRefsSection AFTER the
// existing parsed view, with a per-selection key (Task 6.3/7.1).
func TestDetailPanelMountsReverseRefsSection(t *testing.T) {
	src := readSource(t, "frontend/src/components/DetailPanel.tsx")
	if !strings.Contains(src, "ReverseRefsSection") {
		t.Fatalf("[P0] 9.10-STRUCT-004: DetailPanel.tsx must mount <ReverseRefsSection> (Task 7.1)")
	}
	if !strings.Contains(src, "GetReverseRefs") {
		t.Fatalf("[P0] 9.10-STRUCT-004: DetailPanel.tsx must fetch GetReverseRefs on selection change (Task 7.2)")
	}
	// The remount-on-selection-change contract uses key={selectedNodeId} on
	// the section element itself (Task 6.3). We do NOT pin the exact text
	// because dev may write `key={...}` with any expression; we only check
	// that selectedNodeId flows into the section's key prop.
	// A loose substring match catches the obvious failure modes (key on a
	// wrapper, or key omitted entirely).
	if !strings.Contains(src, "key={selectedNodeId}") {
		t.Fatalf("[P0] 9.10-STRUCT-004: DetailPanel.tsx must render <ReverseRefsSection key={selectedNodeId} ...> so the section unmounts/remounts on selection change (Task 6.3 reset rule)")
	}
}

// 9.10-STRUCT-005 [P0]: Vitest suite for ReverseRefsSection exists.
func TestReverseRefsSectionTestFileExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "ReverseRefsSection.test.tsx")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("[P0] 9.10-STRUCT-005: frontend/src/components/ReverseRefsSection.test.tsx must exist")
	}
}

// 9.10-STRUCT-006 [P0]: ObjectInfoPanel.test.tsx has been rewritten in place
// to cover Story 9-10 (Task 8.1). We assert that the new test file references
// the renamed component, the new testid, and the AC4 obj:gen:num mapping.
func TestObjectInfoPanelTestFileRewritten(t *testing.T) {
	src := readSource(t, "frontend/src/components/ObjectInfoPanel.test.tsx")
	if !strings.Contains(src, "ObjectSourcePanel") {
		t.Fatalf("[P0] 9.10-STRUCT-006: ObjectInfoPanel.test.tsx must reference the renamed ObjectSourcePanel")
	}
	if !strings.Contains(src, "GetObjectSource") {
		t.Fatalf("[P0] 9.10-STRUCT-006: ObjectInfoPanel.test.tsx must mock GetObjectSource (the new fetcher)")
	}
	// The load-bearing mapping test: 5 0 R -> obj:0:5 (NOT obj:5:0).
	// Either form below proves the assertion exists.
	hasMappingAssertion := strings.Contains(src, "obj:0:5") && strings.Contains(src, "5 0 R")
	if !hasMappingAssertion {
		t.Fatalf("[P0] 9.10-STRUCT-006: ObjectInfoPanel.test.tsx must assert the AC4 mapping `5 0 R` -> `obj:0:5` (capture1=num, capture2=gen). Without this, a swap would dispatch the wrong nodeID silently.")
	}
}

// 9.10-STRUCT-007 [P1]: ReferenceNavigation.test.tsx has been updated to
// reflect the panel rewrite (rename references, new clickable-span detection).
func TestReferenceNavigationTestUpdated(t *testing.T) {
	src := readSource(t, "frontend/src/components/ReferenceNavigation.test.tsx")
	if !strings.Contains(src, "ObjectSourcePanel") {
		t.Fatalf("[P1] 9.10-STRUCT-007: ReferenceNavigation.test.tsx must reference ObjectSourcePanel (Task 8.4 rename)")
	}
}

// ---------------------------------------------------------------------------
// Manual verification placeholders (Task 9) -- documented, not asserted.
// ---------------------------------------------------------------------------
//
// Task 9 requires manual verification against representative PDFs. Those
// criteria (the Postscript Language Reference's redundancy bug being gone,
// image preview coexisting with the section, etc.) are not automatable from
// the integration test layer because they require visual confirmation. They
// are covered by Task 9.1-9.5 in the implementation checklist.
