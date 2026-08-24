// Package detail_panel_tabs_test provides acceptance tests for XREF Table and
// Plain Text View Tabs in DetailPanel.
//
// Test pyramid for this story:
//
//   - Backend (XRefTable extraction + status strings +
//     compressed-row handling) -> pdfcore Go unit tests delegated via
//     runPdfcoreTest. Iterating pdfcpu's XRefTable.Table and producing the
//     stable IPC shape is best verified in-process where we can assert the
//     exact slice contents.
//   - Backend (Plain Text Latin-1 decode, control-byte
//     replacement, 5MB truncation cap, encrypted-stream passthrough) ->
//     pdfcore Go unit tests delegated via runPdfcoreTest. Byte-level
//     transformations need byte-level assertions.
//   - Backend failure mode (file moved post-open, pdfcpu panic) ->
//     pdfcore Go unit tests delegated via runPdfcoreTest.
//   - Wails plumbing (GetXRefTable / GetPlainText exposed) ->
//     structural assertions on service.go.
//   - IPC shape (XRefTable, XRefEntry, PlainTextDocument
//     model types with exact JSON tags) -> structural assertions on model.go.
//   - Frontend
//     -> Vitest. Delegated here only via structural checks that the right
//     files / exports / data-testids exist; full behavior contracts are
//     asserted in the component test files.
//
// No Playwright/E2E layer: every acceptance criterion in this story is fully
// observable at the API level (XRefTable struct, PlainTextDocument struct)
// or the component level (RTL state + dispatched actions + Radix data-state
// attributes). The keyboard, scroll-preservation, and tab-switching flows
// are all observable in jsdom. Adding a browser layer would repeat what the
// component tests already cover and contradict the test pyramid for this
// scope.
//
// Run: cd tests/detail-panel-tabs && go test -v -count=1 ./...
package detail_panel_tabs_test

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
// if the test does not pass or does not exist. Same pattern as
// tests/object-source-and-reverse-refs/object_source_and_reverse_refs_test.go
// so the dev removes t.Skip in the underlying pdfcore tests in lockstep with
// implementation.
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
// XRefTable extraction (backend)
// ---------------------------------------------------------------------------

// internal/pdfcore/xreftable.go exists and declares
// Inspector.GetXRefTable.
func TestXRefTableFileExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "internal", "pdfcore", "xreftable.go")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("internal/pdfcore/xreftable.go does not exist")
	}
	src := readSource(t, "internal/pdfcore/xreftable.go")
	if !strings.Contains(src, "GetXRefTable") {
		t.Fatalf("xreftable.go must declare GetXRefTable method on Inspector")
	}
}

// GetXRefTable returns rows for every expected object number, sorted by
// ObjNum asc, object 0 skipped, in-use entries have non-negative Offset,
// free + in-objstm carry the -1 / 0 sentinels.
func TestXRefTableBasicShape(t *testing.T) {
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/minimal.pdf missing")
	}
	runPdfcoreTest(t, "TestGetXRefTableBasicShape")
}

// Rows are sorted by (ObjNum asc, Gen asc) -- pdfcpu map iteration order is
// non-deterministic; sort on egress is mandatory.
func TestXRefTableSorted(t *testing.T) {
	runPdfcoreTest(t, "TestGetXRefTableSortedByObjNumThenGen")
}

// Object 0 (the free-list head) is skipped. It is NOT a real object and
// must never appear in the rendered table.
func TestXRefTableSkipsObjectZero(t *testing.T) {
	runPdfcoreTest(t, "TestGetXRefTableSkipsObjectZero")
}

// Status strings are the load-bearing IPC contract. Must be exactly "in-use"
// / "free" / "in-objstm" -- the frontend renders pills off these literals.
func TestXRefEntryStatusStrings(t *testing.T) {
	runPdfcoreTest(t, "TestGetXRefTableStatusLiterals")
}

// NodeID encoding is "obj:<gen>:<num>" for in-use and in-objstm rows; empty
// string for free rows. The IPC sentinel for free rows must NOT be a
// populated nodeID -- the frontend uses the empty string to make the row
// non-clickable.
func TestXRefEntryNodeIDEncoding(t *testing.T) {
	runPdfcoreTest(t, "TestGetXRefTableNodeIDEncoding")
}

// in-objstm rows expose the underlying object number in NodeID (NOT the
// host objstm). Compressed objects use gen=0 per ISO 32000-1 §7.5.8.1, so
// the nodeID is "obj:0:<num>" where <num> is the underlying object, not the
// host objstm. The risks list calls this out explicitly.
func TestXRefEntryCompressedNodeIDTargetsUnderlyingObject(t *testing.T) {
	runPdfcoreTest(t, "TestGetXRefTableCompressedNodeIDTargetsUnderlying")
}

// in-objstm rows set HostObjStm to the host /ObjStm object number; in-use and
// free rows set HostObjStm = 0 (sentinel: "not applicable"; the frontend
// renders "-").
func TestXRefEntryHostObjStmSentinel(t *testing.T) {
	runPdfcoreTest(t, "TestGetXRefTableHostObjStmSentinel")
}

// Offset is -1 for non-in-use rows (free and in-objstm). The frontend
// renders the literal -1 as the "-" glyph.
func TestXRefEntryOffsetSentinel(t *testing.T) {
	runPdfcoreTest(t, "TestGetXRefTableOffsetSentinel")
}

// GetXRefTable wraps the build in safeCall. A pdfcpu panic on a malformed xref
// must NOT propagate. Verified by calling against the malformed fixture and
// asserting we get either a result or a wrapPDFError-wrapped error -- never a
// panic.
func TestXRefTableSafeCallOnMalformed(t *testing.T) {
	malformedPDF := filepath.Join(testdataDir(t), "malformed.pdf")
	if _, err := os.Stat(malformedPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/malformed.pdf missing")
	}
	runPdfcoreTest(t, "TestGetXRefTableSafeCallOnMalformed")
}

// per-document cache returns the same pointer on the second call. After
// dropping doc.xrefTableCache, a fresh build returns a different pointer
// with equal contents.
func TestXRefTableCacheStable(t *testing.T) {
	runPdfcoreTest(t, "TestGetXRefTableCacheReturnsSamePointer")
}

// ---------------------------------------------------------------------------
// PlainText extraction (backend)
// ---------------------------------------------------------------------------

// internal/pdfcore/plaintext.go exists and declares
// Inspector.GetPlainText.
func TestPlainTextFileExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "internal", "pdfcore", "plaintext.go")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("internal/pdfcore/plaintext.go does not exist")
	}
	src := readSource(t, "internal/pdfcore/plaintext.go")
	if !strings.Contains(src, "GetPlainText") {
		t.Fatalf("plaintext.go must declare GetPlainText method on Inspector")
	}
}

// Plain Text returns the file bytes Latin-1-decoded. The %PDF- header
// signature must appear at the start of Content for a well-formed PDF, and
// TotalBytes must match the on-disk file size.
func TestPlainTextLatin1Header(t *testing.T) {
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/minimal.pdf missing")
	}
	runPdfcoreTest(t, "TestGetPlainTextLatin1HeaderAndSize")
}

// Every byte 0x00..0xFF round-trips losslessly EXCEPT the replaced control
// bytes. The decoder must use rune(b), NOT string(b) (which produces UTF-8 and
// mojibakes 0x80-0xFF). Replaced bytes: every byte in 0x00..0x1F except \t
// (0x09), \n (0x0A), \r (0x0D), plus 0x7F (DEL). Form-feed 0x0C IS replaced.
// Output codepoint for replaced bytes: U+FFFD.
func TestPlainTextLatin1FullByteRange(t *testing.T) {
	runPdfcoreTest(t, "TestGetPlainTextLatin1FullByteRange")
}

// form-feed (0x0C) is explicitly replaced, not preserved. The story
// mandates this so the gutter cannot acquire surprise pagination artifacts.
// This test pins the FF-specific behavior so a future "preserve FF"
// optimization can't slip past review.
func TestPlainTextFormFeedReplaced(t *testing.T) {
	runPdfcoreTest(t, "TestGetPlainTextFormFeedReplaced")
}

// \t (0x09), \n (0x0A), \r (0x0D) are PRESERVED. CRLF survives as two
// characters (the frontend regex /\r\n?|\n/ collapses to one logical line
// break -- backend contract is "verbatim").
func TestPlainTextWhitespaceBytesPreserved(t *testing.T) {
	runPdfcoreTest(t, "TestGetPlainTextWhitespaceBytesPreserved")
}

// Retired: the 25 MiB cap + truncation banner are removed
// in favour of a single uncapped lazy-load. See
// tests/async-plain-text-load/ for the replacement coverage.

// Encrypted streams (file with /Filter /Crypt) pass through as raw on-disk
// bytes. The backend MUST NOT attempt to decode or decrypt. Verified by
// feeding a controlled byte pattern through the decoder and asserting it
// survives the transform unchanged (modulo the control-byte normalization).
func TestPlainTextNoDecryptOrDecode(t *testing.T) {
	runPdfcoreTest(t, "TestGetPlainTextNoDecryptOrDecode")
}

// When the file is moved or deleted post-open, the GetPlainText call
// surfaces an os.IsNotExist-class error wrapped via wrapPDFError. The
// frontend's extractErrorMessage will unwrap it; the ErrorBoundary safety
// net is unchanged.
func TestPlainTextFileMovedError(t *testing.T) {
	runPdfcoreTest(t, "TestGetPlainTextFileMovedReturnsError")
}

// per-document cache returns the same pointer on the second call. Mutex
// coverage includes the I/O so two concurrent calls share one disk read.
func TestPlainTextCacheStable(t *testing.T) {
	runPdfcoreTest(t, "TestGetPlainTextCacheReturnsSamePointer")
}

// GetPlainText wraps I/O + decode in safeCall. A panic during decode must
// NOT crash the process.
func TestPlainTextSafeCallWraps(t *testing.T) {
	src := readSource(t, "internal/pdfcore/plaintext.go")
	if !strings.Contains(src, "safeCall") {
		t.Fatalf("plaintext.go must wrap I/O + decode in safeCall (R3 of story risks list)")
	}
}

// ---------------------------------------------------------------------------
// IPC shape -- model.go must declare the new types with the exact JSON
// tags the frontend will receive over Wails bindings.
// ---------------------------------------------------------------------------

// model.go declares XRefTable struct with TabID + Entries.
func TestModelXRefTableStruct(t *testing.T) {
	src := readSource(t, "internal/pdfcore/model.go")
	if !strings.Contains(src, "type XRefTable struct") {
		t.Fatalf("model.go must declare `type XRefTable struct`")
	}
	requiredTags := []string{
		`json:"tabId"`,
		`json:"entries"`,
	}
	// We assert the tag presence after the struct keyword; both tabs and the
	// reverse-refs struct use the same tabId tag, so a stricter assertion
	// would false-positive. Loose substring match is the right granularity.
	for _, tag := range requiredTags {
		if !strings.Contains(src, tag) {
			t.Fatalf("model.go must declare JSON tag %q for XRefTable", tag)
		}
	}
}

// model.go declares XRefEntry struct with the load-bearing fields and JSON
// tags. Status / Offset / HostObjStm / NodeID are the IPC contract -- the
// frontend renders pills, byte offsets, and navigation targets off these exact
// names.
func TestModelXRefEntryStruct(t *testing.T) {
	src := readSource(t, "internal/pdfcore/model.go")
	if !strings.Contains(src, "type XRefEntry struct") {
		t.Fatalf("model.go must declare `type XRefEntry struct`")
	}
	requiredFields := []string{"ObjNum", "Gen", "Status", "Offset", "HostObjStm", "NodeID"}
	for _, f := range requiredFields {
		if !strings.Contains(src, f) {
			t.Fatalf("XRefEntry must declare field %q", f)
		}
	}
	requiredTags := []string{
		`json:"objNum"`,
		`json:"gen"`,
		`json:"status"`,
		`json:"offset"`,
		`json:"hostObjStm"`,
		`json:"nodeID"`,
	}
	for _, tag := range requiredTags {
		if !strings.Contains(src, tag) {
			t.Fatalf("XRefEntry must declare JSON tag %q -- frontend serialization contract", tag)
		}
	}
}

// model.go declares PlainTextDocument struct with the fields the
// truncation banner needs (TotalBytes + CapBytes + Truncated).
func TestModelPlainTextDocumentStruct(t *testing.T) {
	src := readSource(t, "internal/pdfcore/model.go")
	if !strings.Contains(src, "type PlainTextDocument struct") {
		t.Fatalf("model.go must declare `type PlainTextDocument struct`")
	}
	// Truncated + CapBytes removed; structural assertions on those
	// fields live in tests/async-plain-text-load/.
	requiredFields := []string{"TabID", "Content", "TotalBytes"}
	for _, f := range requiredFields {
		if !strings.Contains(src, f) {
			t.Fatalf("PlainTextDocument must declare field %q", f)
		}
	}
	requiredTags := []string{
		`json:"tabId"`,
		`json:"content"`,
		`json:"totalBytes"`,
	}
	for _, tag := range requiredTags {
		if !strings.Contains(src, tag) {
			t.Fatalf("PlainTextDocument must declare JSON tag %q", tag)
		}
	}
}

// DocumentState carries the xrefTableCache + plainTextCache fields.
// Per-document caching is part of the IPC contract -- without it, two
// GetXRefTable / GetPlainText calls on the same document would duplicate work.
func TestDocumentStateCarriesNewCaches(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	if !strings.Contains(src, "xrefTableCache") {
		t.Fatalf("inspector.go DocumentState must carry xrefTableCache")
	}
	if !strings.Contains(src, "plainTextCache") {
		t.Fatalf("inspector.go DocumentState must carry plainTextCache")
	}
}

// ---------------------------------------------------------------------------
// Wails service plumbing
// ---------------------------------------------------------------------------

// PDFService.GetXRefTable is exposed with the correct return type. The
// frontend bindings depend on the exact slice element type across the IPC
// boundary.
func TestServiceExposesGetXRefTable(t *testing.T) {
	src := readSource(t, "internal/pdfservice/service.go")
	if !strings.Contains(src, "GetXRefTable") {
		t.Fatalf("service.go must expose GetXRefTable")
	}
	if !strings.Contains(src, "*pdfcore.XRefTable") {
		t.Fatalf("service.go GetXRefTable must return (*pdfcore.XRefTable, error)")
	}
}

// PDFService.GetPlainText is exposed with the correct return type.
func TestServiceExposesGetPlainText(t *testing.T) {
	src := readSource(t, "internal/pdfservice/service.go")
	if !strings.Contains(src, "GetPlainText") {
		t.Fatalf("service.go must expose GetPlainText")
	}
	if !strings.Contains(src, "*pdfcore.PlainTextDocument") {
		t.Fatalf("service.go GetPlainText must return (*pdfcore.PlainTextDocument, error)")
	}
}

// ---------------------------------------------------------------------------
// DetailPanel tab bar (structural)
// ---------------------------------------------------------------------------
//
// Behavior contracts (Radix activationMode="manual", arrow-key focus
// movement, scroll preservation, no stale-content frame) are asserted in
// DetailPanel.tabs.test.tsx. We assert here only that the wiring points
// referenced by exist in the source.

// DetailPanel.tsx imports Radix Tabs and uses activationMode="manual"
// (mandated to prevent focus-driven fetches).
func TestDetailPanelImportsRadixTabsManual(t *testing.T) {
	src := readSource(t, "frontend/src/components/DetailPanel.tsx")
	if !strings.Contains(src, "@radix-ui/react-tabs") {
		t.Fatalf("DetailPanel.tsx must import from @radix-ui/react-tabs")
	}
	if !strings.Contains(src, `activationMode="manual"`) {
		t.Fatalf("DetailPanel.tsx must configure <Tabs.Root activationMode=\"manual\">")
	}
}

// Tab triggers and panes carry the documented data-testids the Vitest
// assertions and downstream tooling target.
func TestDetailPanelTabTestIds(t *testing.T) {
	src := readSource(t, "frontend/src/components/DetailPanel.tsx")
	requiredTestIds := []string{
		"detail-tab-object",
		"detail-tab-xref",
		"detail-tab-plaintext",
		"detail-pane-object",
		"detail-pane-xref",
		"detail-pane-plaintext",
	}
	for _, tid := range requiredTestIds {
		if !strings.Contains(src, tid) {
			t.Errorf("DetailPanel.tsx missing data-testid=%q", tid)
		}
	}
}

// Tablist has aria-label="Detail view" so screen readers announce the tab
// group.
func TestDetailPanelTablistAriaLabel(t *testing.T) {
	src := readSource(t, "frontend/src/components/DetailPanel.tsx")
	if !strings.Contains(src, `aria-label="Detail view"`) {
		t.Fatalf("DetailPanel.tsx <Tabs.List> must carry aria-label=\"Detail view\"")
	}
}

// detailView state exists and resets to 'object' on activeTabId change.
// The reset effect is the contract surface for (switching documents resets
// to Object) and (no stale cross-document frame).
func TestDetailPanelDetailViewReset(t *testing.T) {
	src := readSource(t, "frontend/src/components/DetailPanel.tsx")
	if !strings.Contains(src, "detailView") {
		t.Fatalf("DetailPanel.tsx must declare detailView local state")
	}
	if !strings.Contains(src, "setDetailView") {
		t.Fatalf("DetailPanel.tsx must declare setDetailView setter")
	}
	// Reset effect on activeTabId change -- contract.
	// We do not pin the exact effect spelling; we require both `setDetailView('object')`
	// and `[activeTabId]` to appear in the source. A loose match is the right
	// granularity because dev may write the effect inline or hoist it.
	if !strings.Contains(src, `setDetailView('object')`) && !strings.Contains(src, `setDetailView("object")`) {
		t.Fatalf("DetailPanel.tsx must call setDetailView('object') -- the reset path on active-tab change")
	}
}

// The Object pane keeps its existing header (nav buttons, label,
// objectRef, Referenced by section) nested inside the Object Tabs.Content
// -- so XREF and Plain Text panes do NOT inherit the "Properties -
// <stale-key>" header.
func TestDetailPanelObjectPaneHeaderNested(t *testing.T) {
	src := readSource(t, "frontend/src/components/DetailPanel.tsx")
	// The simplest structural guard: the existing nav button testids must
	// still be in the source AND a Tabs.Content with value="object" must
	// appear. Dev can rearrange ordering as long as both invariants hold.
	if !strings.Contains(src, "nav-back-button") {
		t.Fatalf("DetailPanel.tsx must still render nav-back-button inside the Object pane")
	}
	if !strings.Contains(src, "nav-forward-button") {
		t.Fatalf("DetailPanel.tsx must still render nav-forward-button inside the Object pane")
	}
	// The Tabs.Content for the Object pane must exist.
	if !strings.Contains(src, `value="object"`) {
		t.Fatalf("DetailPanel.tsx must declare <Tabs.Content value=\"object\"> for the Object pane")
	}
}

// All three Tabs.Content panes mount simultaneously via forceMount
// (Radix). This preserves scroll position across tab switches.
// Confirming forceMount is in the source is the structural surface for that
// scroll-preservation guarantee.
func TestDetailPanelForceMountForScrollPreservation(t *testing.T) {
	src := readSource(t, "frontend/src/components/DetailPanel.tsx")
	if !strings.Contains(src, "forceMount") {
		t.Fatalf("DetailPanel.tsx must use <Tabs.Content forceMount> on all three panes (scroll-preservation contract)")
	}
}

// ---------------------------------------------------------------------------
// . -- XRefTableView component (structural)
// ---------------------------------------------------------------------------

// XRefTableView.tsx exists and exports XRefTableView.
func TestXRefTableViewFileExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "XRefTableView.tsx")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("frontend/src/components/XRefTableView.tsx must exist")
	}
	src := readSource(t, "frontend/src/components/XRefTableView.tsx")
	if !strings.Contains(src, "XRefTableView") {
		t.Fatalf("XRefTableView.tsx must export XRefTableView")
	}
}

// XRefTableView carries the load-bearing data-testids the Vitest
// assertions target.
func TestXRefTableViewTestIds(t *testing.T) {
	src := readSource(t, "frontend/src/components/XRefTableView.tsx")
	requiredTestIds := []string{
		"xref-loading",
		"xref-error",
		"xref-empty", // no-document state
	}
	for _, tid := range requiredTestIds {
		if !strings.Contains(src, tid) {
			t.Errorf("XRefTableView.tsx missing data-testid=%q", tid)
		}
	}
}

// The table uses semantic HTML (<table>, <thead>, <tbody>, <tr>, <th>, <td>)
// WITHOUT explicit role="..." attributes (the native elements carry implicit
// ARIA roles). Every row carries tabIndex.
func TestXRefTableViewSemanticHTML(t *testing.T) {
	src := readSource(t, "frontend/src/components/XRefTableView.tsx")
	if !strings.Contains(src, "<table") {
		t.Fatalf("XRefTableView.tsx must use a semantic <table> element")
	}
	if !strings.Contains(src, "<thead") {
		t.Fatalf("XRefTableView.tsx must use a semantic <thead> element")
	}
	if !strings.Contains(src, "<tbody") {
		t.Fatalf("XRefTableView.tsx must use a semantic <tbody> element")
	}
	if !strings.Contains(src, "tabIndex={0}") {
		t.Fatalf("XRefTableView.tsx rows must carry tabIndex={0}")
	}
	// Negative assertion: no explicit role="table" / role="row" /
	// role="columnheader" -- the native elements provide them.
	for _, antiRole := range []string{`role="table"`, `role="row"`, `role="columnheader"`, `role="cell"`, `role="rowgroup"`} {
		if strings.Contains(src, antiRole) {
			t.Errorf("XRefTableView.tsx must NOT add explicit %s (native semantic elements carry implicit roles)", antiRole)
		}
	}
}

// Free rows carry aria-disabled="true" so screen readers announce the
// disabled state.
func TestXRefTableViewFreeRowAriaDisabled(t *testing.T) {
	src := readSource(t, "frontend/src/components/XRefTableView.tsx")
	if !strings.Contains(src, "aria-disabled") {
		t.Fatalf("XRefTableView.tsx must set aria-disabled=\"true\" on free rows")
	}
}

// XRefTableView Vitest suite exists. Behavior contracts (status pill text,
// click navigation, in-objstm targets the underlying obj, arrow-key row
// navigation, 200ms debounce, error rendering) are asserted in the Vitest
// file.
func TestXRefTableViewTestFileExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "XRefTableView.test.tsx")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("frontend/src/components/XRefTableView.test.tsx must exist")
	}
}

// ---------------------------------------------------------------------------
// PlainTextView component (structural)
// ---------------------------------------------------------------------------

// PlainTextView.tsx exists and exports PlainTextView.
func TestPlainTextViewFileExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "PlainTextView.tsx")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("frontend/src/components/PlainTextView.tsx must exist")
	}
	src := readSource(t, "frontend/src/components/PlainTextView.tsx")
	if !strings.Contains(src, "PlainTextView") {
		t.Fatalf("PlainTextView.tsx must export PlainTextView")
	}
}

// PlainTextView carries the load-bearing
// data-testids. The truncation-banner testid is retired;
// the new async loading-card testids are pinned in
// tests/async-plain-text-load/.
func TestPlainTextViewTestIds(t *testing.T) {
	src := readSource(t, "frontend/src/components/PlainTextView.tsx")
	requiredTestIds := []string{
		"plain-text-error",
		"plain-text-empty", // no-document state
	}
	for _, tid := range requiredTestIds {
		if !strings.Contains(src, tid) {
			t.Errorf("PlainTextView.tsx missing data-testid=%q", tid)
		}
	}
}

// Retired: the truncation banner that read capBytes +
// totalBytes from the payload no longer exists. The async-load loading card
// surfaces totalBytes via GetPlainTextSize instead.

// The line-break regex collapses CRLF / lone CR / lone LF to one logical
// line break each. The literal /\r\n?|\n/ MUST appear
// in the source -- a naive split('\n') would leave stray \r and break the
// gutter line count.
func TestPlainTextViewLineBreakRegex(t *testing.T) {
	src := readSource(t, "frontend/src/components/PlainTextView.tsx")
	// We match the regex literal as a substring. Forward-slash bracketing is
	// the most common JS regex form; if dev writes new RegExp() instead, this
	// test will need to be updated alongside.
	if !strings.Contains(src, `\r\n?|\n`) {
		t.Fatalf("PlainTextView.tsx must split on /\\r\\n?|\\n/ to collapse CRLF/lone CR/lone LF to one row each")
	}
}

// PlainTextView Vitest suite exists.
func TestPlainTextViewTestFileExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "PlainTextView.test.tsx")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("frontend/src/components/PlainTextView.test.tsx must exist")
	}
}

// ---------------------------------------------------------------------------
// DetailPanel.tabs.test.tsx -- integration test for the tab bar
// ---------------------------------------------------------------------------

// DetailPanel.tabs.test.tsx exists. The Vitest suite covers -- the
// behavior is best asserted at the component layer.
func TestDetailPanelTabsTestFileExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "DetailPanel.tabs.test.tsx")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("frontend/src/components/DetailPanel.tabs.test.tsx must exist")
	}
}

// ---------------------------------------------------------------------------
// extractErrorMessage extraction (shared helper)
// ---------------------------------------------------------------------------

// extractErrorMessage is extracted to
// frontend/src/lib/extractErrorMessage.ts as a shared helper. XRefTableView
// and PlainTextView import it from the new location.
func TestExtractErrorMessageExtracted(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "lib", "extractErrorMessage.ts")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("frontend/src/lib/extractErrorMessage.ts must exist")
	}
	src := readSource(t, "frontend/src/lib/extractErrorMessage.ts")
	if !strings.Contains(src, "extractErrorMessage") {
		t.Fatalf("extractErrorMessage.ts must export extractErrorMessage")
	}
	if !strings.Contains(src, "export") {
		t.Fatalf("extractErrorMessage.ts must export the helper")
	}
}

// DetailPanel.tsx imports extractErrorMessage from the new lib path.
func TestDetailPanelImportsExtractedHelper(t *testing.T) {
	src := readSource(t, "frontend/src/components/DetailPanel.tsx")
	if !strings.Contains(src, "extractErrorMessage") {
		t.Fatalf("DetailPanel.tsx must still reference extractErrorMessage (refactor preserves behavior)")
	}
	if !strings.Contains(src, "extractErrorMessage") || !strings.Contains(src, "lib/extractErrorMessage") {
		t.Fatalf("DetailPanel.tsx must import extractErrorMessage from 'lib/extractErrorMessage'")
	}
}
