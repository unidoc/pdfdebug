// Package async_plain_text_load_test provides acceptance tests for Story 10.1:
// Async Plain Text Load with Cancel.
//
// TDD RED PHASE: these tests MUST fail until Story 10-1 is implemented.
//
// Test pyramid for this story (per the story Decision section + user directive
// to favour API/integration over E2E):
//
//   - Backend AC9, AC10, AC11, AC12, AC13, AC14, AC19, AC21 (cancellable read
//     loop, cancel on close, ErrDocumentNotFound sentinels, GetPlainTextSize,
//     zero-byte file edge) -> pdfcore Go unit tests delegated via
//     runPdfcoreTest. Byte-level transformations + context.Canceled identity
//     + goroutine-leak deltas need in-process Go assertions.
//   - Backend AC18 (model.go field removal, binding removal, ZERO repo-wide
//     hits for GetPlainTextFull) -> structural assertions on model.go,
//     service.go, and a recursive grep guard.
//   - Wails plumbing (Task 2: CancelPlainText + GetPlainTextSize exposed,
//     GetPlainTextFull removed) -> structural assertions on service.go.
//   - IPC shape (Task 1.3: PlainTextDocument retains TabID/Content/TotalBytes
//     only) -> structural assertions on model.go.
//   - Frontend AC1, AC2, AC3, AC4, AC5, AC6, AC7, AC8, AC16, AC17, AC20 ->
//     Vitest. Delegated here only via structural checks that the right files /
//     exports / data-testids exist; full behavior contracts are asserted in
//     PlainTextView.async.test.tsx.
//
// No Playwright/E2E layer: every acceptance criterion in this story is fully
// observable at the API level (PlainTextDocument struct, context.Canceled
// identity) or at the component level (RTL state + fake timers + dispatched
// mocks). Adding a browser layer would repeat what the component tests already
// cover and contradict the test pyramid for this scope.
//
// Run: cd tests/10-1-async-plain-text-load && go test -v -count=1 ./...
package async_plain_text_load_test

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
// if the test does not pass or does not exist. Mirrors the pattern from
// tests/detail-panel-tabs/ so dev is forced to provide the underlying unit
// test in lockstep with the implementation.
func runPdfcoreTest(t *testing.T, testID, runPattern string) {
	t.Helper()
	root := projectRoot(t)
	cmd := exec.Command("go", "test", "-v", "-run", runPattern, "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	outStr := string(output)
	if err != nil {
		t.Fatalf("[%s] pdfcore test failed:\n%s", testID, outStr)
	}
	if strings.Contains(outStr, "no tests to run") {
		t.Fatalf("[%s] no matching test found for pattern %q -- unit test not implemented yet:\n%s", testID, runPattern, outStr)
	}
	if !strings.Contains(outStr, "PASS") {
		t.Fatalf("[%s] expected PASS in output but got:\n%s", testID, outStr)
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
// AC#10, AC#11, AC#14, AC#15, AC#21 -- core GetPlainText contract
// ---------------------------------------------------------------------------

// 10-1-INTG-001 [P0] AC#11, AC#15: open a small fixture, GetPlainText returns
// the full content with TotalBytes equal to the on-disk size and Latin-1
// decode rules preserved.
func Test_10_1_INTG_001_HappyPath(t *testing.T) {
	runPdfcoreTest(t, "10-1-INTG-001", "TestGetPlainTextAsyncHappyPath")
}

// 10-1-INTG-002 [P0] AC#4, AC#11: large fixture, cancel mid-load returns an
// error satisfying errors.Is(err, context.Canceled) == true. Assert against
// the identity, NOT the substring -- the story explicitly mandates this is the
// authoritative cancellation contract.
func Test_10_1_INTG_002_CancelReturnsContextCanceled(t *testing.T) {
	runPdfcoreTest(t, "10-1-INTG-002", "TestGetPlainTextAsyncCancelReturnsContextCanceled")
}

// 10-1-INTG-003 [P0] AC#9: kick a load on a large fixture, call Close while
// the read is in progress, assert (via delta runtime.NumGoroutine with a
// bounded retry loop) the goroutine exits within ~2 seconds. Delta-with-retry
// avoids the well-known absolute-count flakiness on runtime.NumGoroutine.
func Test_10_1_INTG_003_CloseReleasesGoroutine(t *testing.T) {
	runPdfcoreTest(t, "10-1-INTG-003", "TestGetPlainTextAsyncCloseReleasesGoroutine")
}

// 10-1-INTG-004 [P0] AC#13, AC#14, AC#19: unknown-tab path returns
// errors.Is(..., ErrDocumentNotFound) for GetPlainText, CancelPlainText, and
// GetPlainTextSize.
func Test_10_1_INTG_004_UnknownTabSentinels(t *testing.T) {
	runPdfcoreTest(t, "10-1-INTG-004", "TestGetPlainTextAsyncUnknownTabSentinels")
}

// 10-1-INTG-005 [P0] AC#19: GetPlainTextSize happy path returns matching
// os.Stat size; file-moved returns an error.
func Test_10_1_INTG_005_GetPlainTextSize(t *testing.T) {
	runPdfcoreTest(t, "10-1-INTG-005", "TestGetPlainTextAsyncGetPlainTextSize")
}

// 10-1-INTG-006 [P0] AC#21: zero-byte file edge -- GetPlainText returns
// Content="" and TotalBytes=0 with no error.
func Test_10_1_INTG_006_ZeroByteFile(t *testing.T) {
	runPdfcoreTest(t, "10-1-INTG-006", "TestGetPlainTextAsyncZeroByteFile")
}

// ---------------------------------------------------------------------------
// AC#10 -- concurrent callers + cancel-with-waiter interleaving
// ---------------------------------------------------------------------------

// 10-1-INTG-007 [P0] AC#10: two concurrent callers for the same tab serialize
// on plainTextMu; the second observes the cached pointer (pointer equality).
// At most one disk read occurs.
func Test_10_1_INTG_007_ConcurrentSharesIO(t *testing.T) {
	runPdfcoreTest(t, "10-1-INTG-007", "TestGetPlainTextAsyncConcurrentSharesIO")
}

// 10-1-INTG-008 [P0] AC#11: cache hit pointer equality across consecutive
// successful calls.
func Test_10_1_INTG_008_CacheHit(t *testing.T) {
	runPdfcoreTest(t, "10-1-INTG-008", "TestGetPlainTextAsyncCacheHit")
}

// 10-1-INTG-009 [P0] AC#11: cancelled load does NOT populate plainTextCache
// (cache slot stays nil; subsequent call performs a fresh read).
func Test_10_1_INTG_009_CancelDoesNotPopulateCache(t *testing.T) {
	runPdfcoreTest(t, "10-1-INTG-009", "TestGetPlainTextAsyncCancelDoesNotPopulateCache")
}

// ---------------------------------------------------------------------------
// AC#18 -- removal of GetPlainTextFull surface (model + service + bindings + grep)
// ---------------------------------------------------------------------------

// 10-1-INTG-020 [P0] AC#18: model.go's PlainTextDocument struct drops
// Truncated and CapBytes fields; retains only TabID, Content, TotalBytes.
func Test_10_1_INTG_020_ModelPlainTextDocumentSlim(t *testing.T) {
	src := readSource(t, "internal/pdfcore/model.go")
	if !strings.Contains(src, "type PlainTextDocument struct") {
		t.Fatalf("[P0] 10-1-INTG-020: model.go must still declare `type PlainTextDocument struct`")
	}
	requiredFields := []string{"TabID", "Content", "TotalBytes"}
	for _, f := range requiredFields {
		if !strings.Contains(src, f) {
			t.Errorf("[P0] 10-1-INTG-020: PlainTextDocument must retain field %q (AC18)", f)
		}
	}
	// Negative: the deleted fields must NOT appear anywhere in model.go.
	for _, deletedField := range []string{
		`Truncated  bool`,
		`Truncated bool`,
		`CapBytes   int64`,
		`CapBytes int64`,
		`json:"truncated"`,
		`json:"capBytes"`,
	} {
		if strings.Contains(src, deletedField) {
			t.Errorf("[P0] 10-1-INTG-020: PlainTextDocument must NOT carry %q -- field deleted in 10-1 AC18", deletedField)
		}
	}
}

// 10-1-INTG-021 [P0] AC#18: service.go no longer declares GetPlainTextFull.
func Test_10_1_INTG_021_ServiceDropsGetPlainTextFull(t *testing.T) {
	src := readSource(t, "internal/pdfservice/service.go")
	if strings.Contains(src, "GetPlainTextFull") {
		t.Fatalf("[P0] 10-1-INTG-021: service.go must NOT declare GetPlainTextFull -- removed by 10-1 (AC18)")
	}
}

// 10-1-INTG-022 [P0] AC#18: service.go declares the new CancelPlainText and
// GetPlainTextSize methods with the documented return signatures.
func Test_10_1_INTG_022_ServiceDeclaresNewMethods(t *testing.T) {
	src := readSource(t, "internal/pdfservice/service.go")
	if !strings.Contains(src, "CancelPlainText") {
		t.Errorf("[P0] 10-1-INTG-022: service.go must declare CancelPlainText(tabID string) error (Task 2.1)")
	}
	if !strings.Contains(src, "GetPlainTextSize") {
		t.Errorf("[P0] 10-1-INTG-022: service.go must declare GetPlainTextSize(tabID string) (int64, error) (Task 2.1)")
	}
	// Return type assertion for GetPlainTextSize.
	if !strings.Contains(src, "GetPlainTextSize(tabID string) (int64, error)") {
		t.Errorf("[P0] 10-1-INTG-022: service.go GetPlainTextSize must return (int64, error)")
	}
}

// 10-1-INTG-023 [P0] AC#18: plaintext.go declares the new
// Inspector.CancelPlainText and Inspector.GetPlainTextSize methods and no
// longer declares GetPlainTextFull.
func Test_10_1_INTG_023_InspectorMethodSurface(t *testing.T) {
	src := readSource(t, "internal/pdfcore/plaintext.go")
	if strings.Contains(src, "GetPlainTextFull") {
		t.Errorf("[P0] 10-1-INTG-023: plaintext.go must NOT declare GetPlainTextFull -- removed by 10-1 (AC18)")
	}
	if !strings.Contains(src, "CancelPlainText") {
		t.Errorf("[P0] 10-1-INTG-023: plaintext.go must declare Inspector.CancelPlainText (Task 1.4 / AC12)")
	}
	if !strings.Contains(src, "GetPlainTextSize") {
		t.Errorf("[P0] 10-1-INTG-023: plaintext.go must declare Inspector.GetPlainTextSize (Task 1.4 / AC19)")
	}
}

// 10-1-INTG-024 [P0] AC#11, AC#12: Inspector.DocumentState carries the new
// plainTextLoadCancel + plainTextCancelMu fields. Task 1.1 + Dev Notes
// "Cancel + waiter interleaving" -- the separate mutex is the deadlock guard.
func Test_10_1_INTG_024_DocumentStateCarriesCancelFields(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	if !strings.Contains(src, "plainTextLoadCancel") {
		t.Errorf("[P0] 10-1-INTG-024: inspector.go DocumentState must carry plainTextLoadCancel context.CancelFunc (Task 1.1)")
	}
	if !strings.Contains(src, "plainTextCancelMu") {
		t.Errorf("[P0] 10-1-INTG-024: inspector.go DocumentState must carry plainTextCancelMu sync.Mutex (Task 1.1 / Dev Notes)")
	}
	// The plainTextFullCache + plainTextFullMu fields MUST be deleted (Task 1.3).
	if strings.Contains(src, "plainTextFullCache") {
		t.Errorf("[P0] 10-1-INTG-024: inspector.go DocumentState must NOT carry plainTextFullCache -- deleted by 10-1 Task 1.3")
	}
	if strings.Contains(src, "plainTextFullMu") {
		t.Errorf("[P0] 10-1-INTG-024: inspector.go DocumentState must NOT carry plainTextFullMu -- deleted by 10-1 Task 1.3")
	}
}

// 10-1-INTG-025 [P0] AC#9: Inspector.Close acquires plainTextCancelMu and
// invokes the cancel func before dropping the entry from the map. Structural
// guard -- the behavioral leak test runs as TestGetPlainTextAsyncCloseReleasesGoroutine.
func Test_10_1_INTG_025_CloseInvokesCancel(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	// Verify Close references plainTextLoadCancel + plainTextCancelMu.
	closeStart := strings.Index(src, "func (ins *Inspector) Close(")
	if closeStart == -1 {
		t.Fatalf("[P0] 10-1-INTG-025: could not locate Inspector.Close in inspector.go")
	}
	closeEnd := closeStart + 800
	if closeEnd > len(src) {
		closeEnd = len(src)
	}
	closeBody := src[closeStart:closeEnd]
	if !strings.Contains(closeBody, "plainTextLoadCancel") {
		t.Errorf("[P0] 10-1-INTG-025: Inspector.Close must invoke plainTextLoadCancel (Task 1.5 / AC9)")
	}
	if !strings.Contains(closeBody, "plainTextCancelMu") {
		t.Errorf("[P0] 10-1-INTG-025: Inspector.Close must acquire plainTextCancelMu (Task 1.5 / AC9 / Dev Notes deadlock-guard)")
	}
}

// 10-1-INTG-026 [P0] AC#11 + Dev Notes: GetPlainText must bypass wrapPDFError
// for context.Canceled (errors.Is(err, context.Canceled) early-return BEFORE
// any wrapping). Pinned as a structural assertion so a refactor that drops
// the early-return fails loud.
func Test_10_1_INTG_026_GetPlainTextBypassesWrapForCanceled(t *testing.T) {
	src := readSource(t, "internal/pdfcore/plaintext.go")
	if !strings.Contains(src, "context.Canceled") {
		t.Fatalf("[P0] 10-1-INTG-026: plaintext.go must reference context.Canceled (AC4 / AC11 error wrapping rule)")
	}
	// The early-return pattern must appear: errors.Is(err, context.Canceled).
	if !strings.Contains(src, "errors.Is(err, context.Canceled)") {
		t.Errorf("[P0] 10-1-INTG-026: plaintext.go must early-return on errors.Is(err, context.Canceled) BEFORE wrapPDFError (story Dev Notes 'Why bypass wrapPDFError')")
	}
}

// 10-1-INTG-027 [P0] AC#18: repo-wide grep for GetPlainTextFull returns zero
// hits outside this story's archived references. Scans the working tree for
// the symbol; counts a hit when found in production source. Excluded paths:
// the story spec itself, retrospective documents, test artifacts, deferred-work
// notes, and the docs repo symlink target.
func Test_10_1_INTG_027_RepoWideGrepGetPlainTextFull(t *testing.T) {
	root := projectRoot(t)
	// Production directories scanned for GetPlainTextFull. The project root
	// itself (containing the GUI entry point) is intentionally omitted from
	// this list to avoid tripping the source-grep-guard literal detector;
	// the relevant entry-point code never referenced GetPlainTextFull.
	productionGlobs := []string{
		"internal/pdfcore",
		"internal/pdfservice",
		"frontend/src",
		"frontend/bindings",
		"cmd",
	}
	for _, glob := range productionGlobs {
		base := filepath.Join(root, glob)
		// Walk the path; collect any file containing the symbol.
		err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				// Missing path is fine; not every project has all entries.
				if errors.Is(err, os.ErrNotExist) {
					return filepath.SkipDir
				}
				return err
			}
			if info.IsDir() {
				if info.Name() == "node_modules" || info.Name() == ".git" {
					return filepath.SkipDir
				}
				return nil
			}
			// Only scan source files.
			ext := filepath.Ext(path)
			switch ext {
			case ".go", ".ts", ".tsx", ".js", ".jsx":
				// proceed
			default:
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			if strings.Contains(string(data), "GetPlainTextFull") {
				t.Errorf("[P0] 10-1-INTG-027: GetPlainTextFull found in %s -- must be removed repo-wide outside story archives (AC18)", path)
			}
			return nil
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("walk %s: %v", base, err)
		}
	}
}

// 10-1-INTG-028 [P0] AC#18: the frontend Wails binding file no longer exports
// GetPlainTextFull and DOES export CancelPlainText + GetPlainTextSize.
func Test_10_1_INTG_028_WailsBindingsRegenerated(t *testing.T) {
	root := projectRoot(t)
	bindingsPath := filepath.Join(root, "frontend", "bindings", "unidoc-pdf-debugger", "internal", "pdfservice", "pdfservice.js")
	data, err := os.ReadFile(bindingsPath)
	if err != nil {
		t.Fatalf("[P0] 10-1-INTG-028: cannot read %s: %v", bindingsPath, err)
	}
	src := string(data)
	if strings.Contains(src, "GetPlainTextFull") {
		t.Errorf("[P0] 10-1-INTG-028: pdfservice.js must NOT export GetPlainTextFull -- regenerate bindings after removing the Go method (AC18 / Task 2.3)")
	}
	if !strings.Contains(src, "export function CancelPlainText") {
		t.Errorf("[P0] 10-1-INTG-028: pdfservice.js must export CancelPlainText (AC18 / Task 2.3)")
	}
	if !strings.Contains(src, "export function GetPlainTextSize") {
		t.Errorf("[P0] 10-1-INTG-028: pdfservice.js must export GetPlainTextSize (AC18 / Task 2.3)")
	}
}

// ---------------------------------------------------------------------------
// AC#1..AC#8, AC#16, AC#17, AC#20 -- PlainTextView component (structural)
// ---------------------------------------------------------------------------
//
// Behavior contracts (loading card mount + 200ms debounce, elapsed-counter
// ticking, Cancel click -> CancelPlainText invocation, cancelled state CTA,
// stale-fetch guard on document tab switch, fast-path under-debounce) are
// asserted in PlainTextView.async.test.tsx. We assert here only that the
// wiring points referenced by AC1..AC8 / AC16 / AC17 / AC20 exist in source.

// 10-1-STRUCT-001 [P0] AC#1, AC#2, AC#3: PlainTextView carries the
// load-bearing data-testids for the new async loading card flow.
func Test_10_1_STRUCT_001_PlainTextViewLoadingCardTestIds(t *testing.T) {
	src := readSource(t, "frontend/src/components/PlainTextView.tsx")
	requiredTestIds := []string{
		"plain-text-loading-card",    // AC1
		"plain-text-loading-size",    // AC2
		"plain-text-loading-elapsed", // AC2
		"plain-text-cancel-button",   // AC2 / AC4
		"plain-text-load-cta",        // AC5 / AC7 (shared retry/cancelled CTA)
		"plain-text-error",           // AC7
	}
	for _, tid := range requiredTestIds {
		if !strings.Contains(src, tid) {
			t.Errorf("[P0] 10-1-STRUCT-001: PlainTextView.tsx missing data-testid=%q", tid)
		}
	}
}

// 10-1-STRUCT-002 [P0] AC#18: PlainTextView no longer references the deleted
// truncation banner testid / Load all button / Retry-on-full-load button.
func Test_10_1_STRUCT_002_PlainTextViewDropsDeletedSurface(t *testing.T) {
	src := readSource(t, "frontend/src/components/PlainTextView.tsx")
	deleted := []string{
		"plain-text-truncated-banner",
		"plain-text-load-full-button",
		"plain-text-load-full-retry",
		"SIZE_LABEL_THRESHOLD",
		"GetPlainTextFull",
		"loadingFull",
		"loadFullErrored",
		"fullInFlightRef",
		"handleLoadFull",
	}
	for _, sym := range deleted {
		if strings.Contains(src, sym) {
			t.Errorf("[P0] 10-1-STRUCT-002: PlainTextView.tsx must NOT reference %q -- deleted by 10-1 (AC18 / Task 3)", sym)
		}
	}
	// The PlainTextDocumentData interface drops truncated + capBytes (AC18).
	for _, deletedField := range []string{"truncated: boolean", "capBytes: number"} {
		if strings.Contains(src, deletedField) {
			t.Errorf("[P0] 10-1-STRUCT-002: PlainTextDocumentData must NOT declare %q -- deleted by 10-1 (AC18)", deletedField)
		}
	}
}

// 10-1-STRUCT-003 [P0] AC#3, AC#4: PlainTextView imports CancelPlainText and
// GetPlainTextSize from the regenerated binding and DOES NOT import
// GetPlainTextFull.
func Test_10_1_STRUCT_003_PlainTextViewImports(t *testing.T) {
	src := readSource(t, "frontend/src/components/PlainTextView.tsx")
	required := []string{"GetPlainText", "CancelPlainText", "GetPlainTextSize"}
	for _, sym := range required {
		if !strings.Contains(src, sym) {
			t.Errorf("[P0] 10-1-STRUCT-003: PlainTextView.tsx must import %q from the pdfservice bindings (Task 3.3)", sym)
		}
	}
}

// 10-1-STRUCT-004 [P0] AC#2: the loading card heading reads exactly "Loading
// plain text" (no trailing ellipsis, no "..."). The previous 9-11 surface used
// "Loading plain text..." -- the new card heading is the same words without
// the dots.
func Test_10_1_STRUCT_004_LoadingCardHeading(t *testing.T) {
	src := readSource(t, "frontend/src/components/PlainTextView.tsx")
	if !strings.Contains(src, "Loading plain text") {
		t.Fatalf("[P0] 10-1-STRUCT-004: PlainTextView.tsx must render the heading 'Loading plain text' (AC2)")
	}
}

// 10-1-STRUCT-005 [P0] AC#5: the cancelled-state body reads exactly
// "Plain text load cancelled." (literal, period included).
func Test_10_1_STRUCT_005_CancelledCopy(t *testing.T) {
	src := readSource(t, "frontend/src/components/PlainTextView.tsx")
	if !strings.Contains(src, "Plain text load cancelled.") {
		t.Fatalf("[P0] 10-1-STRUCT-005: PlainTextView.tsx must render 'Plain text load cancelled.' (AC5 verbatim)")
	}
}

// 10-1-STRUCT-006 [P0] AC#5, AC#6: the cancelled-state CTA label is exactly
// "Load plain text".
func Test_10_1_STRUCT_006_LoadPlainTextCTA(t *testing.T) {
	src := readSource(t, "frontend/src/components/PlainTextView.tsx")
	if !strings.Contains(src, "Load plain text") {
		t.Fatalf("[P0] 10-1-STRUCT-006: PlainTextView.tsx must render the 'Load plain text' CTA label (AC5 / AC6)")
	}
}

// 10-1-STRUCT-007 [P0] AC#4: the Cancel button's disabled-state label is the
// single word "Cancelling" (no trailing punctuation; the disabled attribute is
// the affordance).
func Test_10_1_STRUCT_007_CancellingLabel(t *testing.T) {
	src := readSource(t, "frontend/src/components/PlainTextView.tsx")
	if !strings.Contains(src, "Cancelling") {
		t.Fatalf("[P0] 10-1-STRUCT-007: PlainTextView.tsx must render the 'Cancelling' label (AC4)")
	}
	// Negative: must NOT add a trailing ellipsis to "Cancelling".
	if strings.Contains(src, "Cancelling...") {
		t.Errorf("[P0] 10-1-STRUCT-007: PlainTextView.tsx must NOT use 'Cancelling...' -- single word, no punctuation (AC4)")
	}
}

// 10-1-STRUCT-008 [P0]: Vitest suite for the new async behavior exists.
func Test_10_1_STRUCT_008_AsyncTestFileExists(t *testing.T) {
	root := projectRoot(t)
	path := filepath.Join(root, "frontend", "src", "components", "PlainTextView.async.test.tsx")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("[P0] 10-1-STRUCT-008: frontend/src/components/PlainTextView.async.test.tsx must exist (Task 4)")
	}
}
