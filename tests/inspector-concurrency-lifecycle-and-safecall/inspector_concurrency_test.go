// Package story_10_5_test holds the per-story acceptance test suite for
// Story 10-5: Inspector Concurrency, Lifecycle Safety, and safeCall Sev-1.
//
// This is the structural-assertion suite -- it source-greps the production
// tree for the contracts pinned in the story's ACs that DO NOT require
// runtime exercise. Behavioural tests live alongside the production code:
//
//   internal/pdfcore/inspector_concurrent_test.go      (AC2 -race soak)
//   internal/pdfcore/stream_test.go                    (AC3 same-node race)
//   internal/pdfcore/inspector_internal_test.go        (AC4, AC7 lifecycle)
//   internal/pdfservice/service_recover_test.go        (AC5 behaviour, build-tag-gated)
//
// The TDD red-phase contract: every Test_10_5_* in this file FAILS today
// against the pre-implementation tree. Dev's job is to land the changes that
// turn each red test green. A test that already passes today is a contract
// pin (e.g. existing safeCall test names that must survive); those use the
// "MUST still exist" framing so a future deletion turns them red.
//
// Run: cd tests/10-5-inspector-concurrency-lifecycle-and-safecall-sev1 && go test -v -count=1 ./...
package story_10_5_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// projectRoot walks up from cwd until it finds the project go.mod (module
// unidoc-pdf-debugger). Mirrors the pattern from the 10-4 sibling suite.
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

// ---------------------------------------------------------------------------
// AC#1 -- DocumentState.pdfMu field + per-method lock acquisition
// ---------------------------------------------------------------------------

// Test_10_5_AC1_DocumentStateHasPdfMu [P0] AC#1: DocumentState carries a new
// `pdfMu sync.Mutex` field. The field name is pinned in the spec; a renamed
// or relocated field defeats the assertion. The field MUST live INSIDE the
// DocumentState struct, not on Inspector.
func Test_10_5_AC1_DocumentStateHasPdfMu(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	// Anchor the search to lines inside the DocumentState struct. A naive
	// substring search would also match a comment elsewhere; require the
	// field declaration shape with `sync.Mutex` (or just `Mutex` if the
	// package alias differs -- pdfcore imports `sync` directly so this is
	// the canonical shape).
	re := regexp.MustCompile(`(?m)^\s*pdfMu\s+sync\.Mutex\b`)
	if !re.MatchString(src) {
		t.Errorf("[P0] 10-5-AC1: internal/pdfcore/inspector.go must declare `pdfMu sync.Mutex` as a field on DocumentState (AC1: per-document mutex serializing pdfcpu calls)")
	}
}

// Test_10_5_AC1_DocumentStateHasRevBuildOnce [P0] AC#1/AC#7: DocumentState
// also carries `revBuildOnce sync.Once` to gate the lazy reverse-refs build.
// Same-position check as pdfMu -- it MUST be a field on the struct, not a
// package-level variable.
func Test_10_5_AC1_DocumentStateHasRevBuildOnce(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	re := regexp.MustCompile(`(?m)^\s*revBuildOnce\s+sync\.Once\b`)
	if !re.MatchString(src) {
		t.Errorf("[P0] 10-5-AC1/AC7: internal/pdfcore/inspector.go must declare `revBuildOnce sync.Once` as a field on DocumentState (AC7: lazy first-call build via sync.Once)")
	}
}

// pdfMuRequiredMethods is the AC1 inventory of Inspector methods that MUST
// acquire `doc.pdfMu` after GetDocument returns. The list is verbatim from
// the AC1 paragraph; new pdfcpu-touching methods would need updating here
// as part of their introduction story.
var pdfMuRequiredMethods = []string{
	"GetTreeRoot",
	"GetChildren",
	"GetObjectDetail",
	"GetAncestorPath",
	"GetContentStream",
	"GetPageContentStreamNodeID",
	"GetImageData",
	"GetFontDetail",
	"GetFontResourceMap",
	"GetFontView",
	"GetObjectSource",
	"GetReverseRefs",
	"GetObjectIndex",
	"GetXRefTable",
}

// methodFileMap pins the file each Inspector method lives in. Anchoring the
// grep to the specific file avoids false positives from comments or test
// fixtures elsewhere in the package.
var methodFileMap = map[string]string{
	"GetTreeRoot":                "internal/pdfcore/tree.go",
	"GetChildren":                "internal/pdfcore/tree.go",
	"GetObjectDetail":            "internal/pdfcore/inspector.go",
	"GetAncestorPath":            "internal/pdfcore/inspector.go",
	"GetContentStream":           "internal/pdfcore/stream.go",
	"GetPageContentStreamNodeID": "internal/pdfcore/stream.go",
	"GetImageData":               "internal/pdfcore/image.go",
	"GetFontDetail":              "internal/pdfcore/font.go",
	"GetFontResourceMap":         "internal/pdfcore/font_roster.go",
	"GetFontView":                "internal/pdfcore/font_view.go",
	"GetObjectSource":            "internal/pdfcore/objectsource.go",
	"GetReverseRefs":             "internal/pdfcore/reverserefs.go",
	"GetObjectIndex":             "internal/pdfcore/objectindex.go",
	"GetXRefTable":               "internal/pdfcore/xreftable.go",
}

// pdfMuLockOwner maps a method that only DELEGATES to the helper that performs
// the locked work, so the grep looks where the mutex actually lives. The
// guarantee asserted is unchanged (the page-dict resolution happens under
// doc.pdfMu); it is just one call deeper. GetPageContentStreamNodeID resolves
// through pageContentStreamNodeIDs, which locks, so that both it and
// GetPageContentStream share one page-dict resolution.
var pdfMuLockOwner = map[string]string{
	"GetPageContentStreamNodeID": "pageContentStreamNodeIDs",
}

// Test_10_5_AC1_MethodsAcquirePdfMu [P0] AC#1: every method in
// pdfMuRequiredMethods MUST contain a `doc.pdfMu.Lock()` call and a
// `defer doc.pdfMu.Unlock()` immediately after the `GetDocument` call.
// We assert the two substrings appear in the function body via a
// per-method file read + regex scoped to the function range.
//
// A method listed in pdfMuLockOwner is checked in two steps instead: it must
// call its declared helper, and the HELPER must carry the lock pattern. A
// method that neither locks nor delegates still fails.
//
// The function boundary is approximated by `func (ins *Inspector) <Name>(`
// up to the next `func (` at the same depth -- adequate for grep purposes.
func Test_10_5_AC1_MethodsAcquirePdfMu(t *testing.T) {
	for _, method := range pdfMuRequiredMethods {
		t.Run(method, func(t *testing.T) {
			path, ok := methodFileMap[method]
			if !ok {
				t.Fatalf("internal test bug: methodFileMap missing %q", method)
			}
			src := readSource(t, path)
			target := method
			if owner, ok := pdfMuLockOwner[method]; ok {
				caller := extractFunctionBody(t, src, method)
				if caller == "" {
					t.Fatalf("[P0] 10-5-AC1: could not locate `func (ins *Inspector) %s(` in %s", method, path)
				}
				if !strings.Contains(caller, owner+"(") {
					t.Errorf("[P0] 10-5-AC1: %s in %s must either lock doc.pdfMu itself or delegate to %s, which does", method, path, owner)
				}
				target = owner
			}
			body := extractFunctionBody(t, src, target)
			if body == "" {
				t.Fatalf("[P0] 10-5-AC1: could not locate `func (ins *Inspector) %s(` in %s", target, path)
			}
			if !strings.Contains(body, "doc.pdfMu.Lock()") {
				t.Errorf("[P0] 10-5-AC1: %s in %s must call `doc.pdfMu.Lock()` (AC1: acquire per-document mutex immediately after GetDocument)", target, path)
			}
			if !strings.Contains(body, "defer doc.pdfMu.Unlock()") {
				t.Errorf("[P0] 10-5-AC1: %s in %s must call `defer doc.pdfMu.Unlock()` (AC1: deferred Unlock pattern)", target, path)
			}
		})
	}
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
	// Scan from startIdx forward for the next "\nfunc " (column-0 func)
	// which delimits the next top-level function.
	tail := src[startIdx:]
	endIdx := strings.Index(tail[1:], "\nfunc ")
	if endIdx == -1 {
		return tail
	}
	return tail[:endIdx+1]
}

// ---------------------------------------------------------------------------
// AC#3 -- stream.go cache contract: streamMu held for resolve+decode
// ---------------------------------------------------------------------------

// Test_10_5_AC3_GetContentStreamHoldsStreamMuForDecode [P0] AC#3:
// The post-fix GetContentStream body MUST NOT release streamMu between the
// cache check and the cache write. The simplest structural check: count the
// `doc.streamMu.Unlock()` calls inside the GetContentStream body. The
// post-fix shape has exactly one Unlock (paired with the single Lock that
// covers the whole path) plus the early-return Unlock for the cache hit.
// Today the body has 4 Unlocks (cache-hit return, post-cache-check, after
// "not a stream" early return, and the final write). The contract is "no
// drop-and-reacquire", which translates to: the Lock paired with the cache
// MISS path is the same Lock that wraps the cache WRITE.
//
// We assert the simpler form: the substring
// `doc.streamMu.Unlock()\n\tif _, ok := doc.streamCache[nodeID]` -- the
// current code shape between cache check and decode -- MUST NOT be present
// after the fix. That literal exists today; this test fails today.
func Test_10_5_AC3_GetContentStreamHoldsStreamMuForDecode(t *testing.T) {
	src := readSource(t, "internal/pdfcore/stream.go")
	// The current "drop the lock and reacquire" idiom uses the exact shape:
	//   doc.streamMu.Unlock()
	//   <work that calls into pdfcpu>
	//   doc.streamMu.Lock()
	// Count the Unlocks; post-fix the body should have AT MOST 2 (one for
	// the cache-hit early return, one for the final defer-style release).
	// The pre-fix body has 4. Use that ceiling as the contract.
	body := extractFunctionBody(t, src, "GetContentStream")
	if body == "" {
		t.Fatalf("[P0] 10-5-AC3: could not locate GetContentStream in stream.go")
	}
	unlockCount := strings.Count(body, "doc.streamMu.Unlock()")
	if unlockCount > 2 {
		t.Errorf("[P0] 10-5-AC3: GetContentStream contains %d `doc.streamMu.Unlock()` calls -- post-fix shape expects <= 2 (single critical section covering resolve+decode+write). Today's drop-and-reacquire pattern violates AC3.", unlockCount)
	}
}

// ---------------------------------------------------------------------------
// AC#5 -- pdfservice top-level recover helper + per-method defer
// ---------------------------------------------------------------------------

// Test_10_5_AC5_RecoverRuntimePanicHelperExists [P0] AC#5: service.go MUST
// declare the recoverRuntimePanic helper with the exact signature pinned by
// AC5: `func recoverRuntimePanic(methodName string, errOut *error)`.
func Test_10_5_AC5_RecoverRuntimePanicHelperExists(t *testing.T) {
	src := readSource(t, "internal/pdfservice/service.go")
	needle := "func recoverRuntimePanic(methodName string, errOut *error)"
	if !strings.Contains(src, needle) {
		t.Errorf("[P0] 10-5-AC5: internal/pdfservice/service.go must declare %q (AC5: pinned helper signature)", needle)
	}
}

// Test_10_5_AC5_RecoverHelperConvertsToMalformedPDF [P0] AC#5: the helper
// MUST construct the error via `fmt.Errorf(\"%w: internal error\",
// pdfcore.ErrMalformedPDF)` so errors.Is(err, pdfcore.ErrMalformedPDF)
// holds at the call site AND the frontend's /malformed/i regex matches.
func Test_10_5_AC5_RecoverHelperConvertsToMalformedPDF(t *testing.T) {
	src := readSource(t, "internal/pdfservice/service.go")
	needle := `fmt.Errorf("%w: internal error", pdfcore.ErrMalformedPDF)`
	if !strings.Contains(src, needle) {
		t.Errorf("[P0] 10-5-AC5: internal/pdfservice/service.go must construct the recovered error as %q so errors.Is(err, ErrMalformedPDF) holds AND the frontend regex /malformed/i matches (AC5)", needle)
	}
}

// Test_10_5_AC5_RecoverHelperLogsViaPrintf [P0] AC#5: the helper MUST emit
// a `log.Printf("pdfservice: runtime.Error in %s: %v\n%s", ...)` line with
// debug.Stack() so devs can diagnose the underlying Go bug from logs.
func Test_10_5_AC5_RecoverHelperLogsViaPrintf(t *testing.T) {
	src := readSource(t, "internal/pdfservice/service.go")
	needle := `log.Printf("pdfservice: runtime.Error in %s: %v\n%s"`
	if !strings.Contains(src, needle) {
		t.Errorf("[P0] 10-5-AC5: internal/pdfservice/service.go must call %q ... debug.Stack()) when recovering a runtime.Error (AC5)", needle)
	}
	if !strings.Contains(src, "debug.Stack()") {
		t.Errorf("[P0] 10-5-AC5: internal/pdfservice/service.go must include debug.Stack() in the recover-helper log line (AC5)")
	}
}

// Test_10_5_AC5_RecoverHelperRePanicsNonRuntimeError [P0] AC#5: non-runtime
// panics MUST be re-panicked so genuine Go bugs outside pdfcpu's documented
// surface still crash the test binary loudly. The shape is `panic(r)` after
// the negative type assertion.
func Test_10_5_AC5_RecoverHelperRePanicsNonRuntimeError(t *testing.T) {
	src := readSource(t, "internal/pdfservice/service.go")
	body := extractFunctionBodyTopLevel(t, src, "recoverRuntimePanic")
	if body == "" {
		t.Skipf("[P0] 10-5-AC5: recoverRuntimePanic not declared yet (see Test_10_5_AC5_RecoverRuntimePanicHelperExists)")
	}
	if !strings.Contains(body, "panic(r)") {
		t.Errorf("[P0] 10-5-AC5: recoverRuntimePanic must re-panic non-runtime errors via `panic(r)` (AC5: preserves the test-binary-crash diagnostic for genuine bugs)")
	}
}

// extractFunctionBodyTopLevel locates `func <name>(` at column 0 (no
// receiver) and returns the body up to the next column-0 `func `.
func extractFunctionBodyTopLevel(t *testing.T, src, name string) string {
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

// pdfserviceWrappedMethods is the AC5-pinned list of 15 PDFService methods
// that MUST begin with `defer recoverRuntimePanic("<Name>", &err)`. Methods
// outside this list (OpenFileDialog, CloseDocument, GetPlainText,
// CancelPlainText, GetPlainTextSize) MUST NOT be wrapped.
var pdfserviceWrappedMethods = []string{
	"OpenFile",
	"GetTreeRoot",
	"GetChildren",
	"GetObjectDetail",
	"GetAncestorPath",
	"GetContentStream",
	"GetImageData",
	"GetFontDetail",
	"GetFontResourceMap",
	"GetFontView",
	"GetObjectSource",
	"GetReverseRefs",
	"GoToPage",
	"GetObjectIndex",
	"GetXRefTable",
}

// pdfserviceUnwrappedMethods is the AC5-pinned EXCLUSION list -- methods
// that MUST NOT have the recover wrapper because they do not call pdfcpu.
// A future Dev who adds the wrapper here would launder a Go bug as
// "malformed PDF" and mislead the user.
var pdfserviceUnwrappedMethods = []string{
	"OpenFileDialog",
	"CloseDocument",
	"GetPlainText",
	"CancelPlainText",
	"GetPlainTextSize",
}

// Test_10_5_AC5_WrappedMethodsHaveDeferRecover [P0] AC#5: every method in
// pdfserviceWrappedMethods MUST contain `defer recoverRuntimePanic(` in its
// body. The exact methodName argument is also checked to catch a copy-paste
// bug where every wrapper logs the same string.
func Test_10_5_AC5_WrappedMethodsHaveDeferRecover(t *testing.T) {
	src := readSource(t, "internal/pdfservice/service.go")
	for _, method := range pdfserviceWrappedMethods {
		t.Run(method, func(t *testing.T) {
			body := extractPDFServiceMethodBody(t, src, method)
			if body == "" {
				t.Fatalf("[P0] 10-5-AC5: could not locate `func (s *PDFService) %s(` in service.go", method)
			}
			needle := `defer recoverRuntimePanic("` + method + `", &err)`
			if !strings.Contains(body, needle) {
				t.Errorf("[P0] 10-5-AC5: PDFService.%s must contain %q (AC5: per-method wrapper with verbatim method name)", method, needle)
			}
		})
	}
}

// Test_10_5_AC5_UnwrappedMethodsHaveNoDeferRecover [P0] AC#5: methods in
// pdfserviceUnwrappedMethods MUST NOT contain `defer recoverRuntimePanic(`.
// Wrapping them is a contract violation (launders Go bugs in non-pdfcpu
// code as ErrMalformedPDF -- misleading user-facing message).
func Test_10_5_AC5_UnwrappedMethodsHaveNoDeferRecover(t *testing.T) {
	src := readSource(t, "internal/pdfservice/service.go")
	for _, method := range pdfserviceUnwrappedMethods {
		t.Run(method, func(t *testing.T) {
			body := extractPDFServiceMethodBody(t, src, method)
			if body == "" {
				// Acceptable -- the method may not exist (this is a
				// negative assertion). Skip silently.
				return
			}
			if strings.Contains(body, "defer recoverRuntimePanic(") {
				t.Errorf("[P0] 10-5-AC5: PDFService.%s MUST NOT contain `defer recoverRuntimePanic(` (AC5: non-pdfcpu methods must not launder Go bugs as ErrMalformedPDF)", method)
			}
		})
	}
}

// extractPDFServiceMethodBody locates `func (s *PDFService) <name>(` and
// returns the body up to the next top-level func.
func extractPDFServiceMethodBody(t *testing.T, src, name string) string {
	t.Helper()
	needle := "func (s *PDFService) " + name + "("
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
// AC#6 -- existing safeCall test surface MUST survive unchanged
// ---------------------------------------------------------------------------

// safeCallContractTests is the AC6 verbatim list of test names in
// errors_test.go that MUST continue to exist post-implementation. A rename
// or deletion is the contract violation AC6 forbids.
var safeCallContractTests = []string{
	"TestSafeCallSuccess",
	"TestSafeCallReturnsError",
	"TestSafeCallCatchesStringPanic",
	"TestSafeCallCatchesErrorPanic",
	"TestSafeCallPropagatesRuntimeError",
}

// Test_10_5_AC6_SafeCallContractTestsExist [P0] AC#6: each named test in
// errors_test.go MUST still be declared. This is a contract pin -- the test
// already passes today; a future Dev removal turns it red.
func Test_10_5_AC6_SafeCallContractTestsExist(t *testing.T) {
	src := readSource(t, "internal/pdfcore/errors_test.go")
	for _, name := range safeCallContractTests {
		needle := "func " + name + "(t *testing.T)"
		if !strings.Contains(src, needle) {
			t.Errorf("[P0] 10-5-AC6: internal/pdfcore/errors_test.go must still declare %s (AC6: the pdfcore-layer safeCall contract is unchanged by this story)", name)
		}
	}
}

// Test_10_5_AC6_SafeCallRePanicsRuntimeErrorPreserved [P0] AC#6:
// internal/pdfcore/errors.go's safeCall MUST retain the runtime.Error
// re-panic. AC6 says: "the Sev-1 fix lives at the pdfservice boundary
// (AC5), NOT inside safeCall." Same contract as 10-4 STRUCT_010.
func Test_10_5_AC6_SafeCallRePanicsRuntimeErrorPreserved(t *testing.T) {
	src := readSource(t, "internal/pdfcore/errors.go")
	re := regexp.MustCompile(`if\s+_,\s*ok\s*:=\s*r\.\(runtime\.Error\)`)
	if !re.MatchString(src) {
		t.Errorf("[P0] 10-5-AC6: internal/pdfcore/errors.go must retain the runtime.Error type assertion in safeCall's recover block (AC6: safeCall contract unchanged)")
	}
	if !strings.Contains(src, "panic(r)") {
		t.Errorf("[P0] 10-5-AC6: internal/pdfcore/errors.go must retain `panic(r)` in safeCall (AC6: the pdfcore-layer re-panic is preserved)")
	}
}

// ---------------------------------------------------------------------------
// AC#7 -- reverse-refs build moved out of Open
// ---------------------------------------------------------------------------

// Test_10_5_AC7_OpenNoLongerCallsBuildReverseRefs [P0] AC#7: the
// Inspector.Open body MUST NOT call buildReverseRefs. The build was at
// inspector.go lines 140-156 under the comment "Build the reverse-ref
// index inside safeCall."; AC7 moves this to buildReverseRefsOnce invoked
// by the first GetReverseRefs call.
func Test_10_5_AC7_OpenNoLongerCallsBuildReverseRefs(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	body := extractFunctionBody(t, src, "Open")
	if body == "" {
		t.Fatalf("[P0] 10-5-AC7: could not locate Inspector.Open in inspector.go")
	}
	if strings.Contains(body, "buildReverseRefs(doc") {
		t.Errorf("[P0] 10-5-AC7: Inspector.Open must NOT call `buildReverseRefs(doc, ...)` -- AC7 defers the build to first GetReverseRefs via buildReverseRefsOnce")
	}
}

// Test_10_5_AC7_BuildReverseRefsOnceHelperExists [P0] AC#7: a new helper
// `buildReverseRefsOnce(doc *DocumentState)` MUST be declared in pdfcore.
// It owns the lazy-build path: acquires doc.pdfMu, invokes the sync.Once
// inner safeCall-wrapped builder, sets revRefsBuildFailed on panic.
func Test_10_5_AC7_BuildReverseRefsOnceHelperExists(t *testing.T) {
	root := projectRoot(t)
	// The helper can live in inspector.go or reverserefs.go -- search both.
	candidates := []string{"internal/pdfcore/inspector.go", "internal/pdfcore/reverserefs.go"}
	needle := "func buildReverseRefsOnce(doc *DocumentState)"
	found := false
	for _, c := range candidates {
		data, err := os.ReadFile(filepath.Join(root, c))
		if err != nil {
			continue
		}
		if strings.Contains(string(data), needle) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("[P0] 10-5-AC7: a helper `func buildReverseRefsOnce(doc *DocumentState)` must be declared in either internal/pdfcore/inspector.go or internal/pdfcore/reverserefs.go (AC7: lazy first-call build path)")
	}
}

// Test_10_5_AC7_GetReverseRefsCallsBuildReverseRefsOnce [P0] AC#7:
// Inspector.GetReverseRefs MUST call buildReverseRefsOnce BEFORE the
// existing revRefsBuildFailed check, so the very first invocation triggers
// the build.
func Test_10_5_AC7_GetReverseRefsCallsBuildReverseRefsOnce(t *testing.T) {
	src := readSource(t, "internal/pdfcore/reverserefs.go")
	body := extractFunctionBody(t, src, "GetReverseRefs")
	if body == "" {
		t.Fatalf("[P0] 10-5-AC7: could not locate GetReverseRefs in reverserefs.go")
	}
	if !strings.Contains(body, "buildReverseRefsOnce(doc)") {
		t.Errorf("[P0] 10-5-AC7: GetReverseRefs must invoke `buildReverseRefsOnce(doc)` BEFORE the revRefsBuildFailed check (AC7: lazy build)")
	}
}

// ---------------------------------------------------------------------------
// AC#8 -- main.go openFileAndEmitWithWarning dispatches pdfcpu read to goroutine
// ---------------------------------------------------------------------------

// Test_10_5_AC8_OpenFileAndEmitDispatchesGoroutine [P0] AC#8: the
// openFileAndEmitWithWarning function MUST dispatch the pdfcpu read to a
// `go func(...)` so the Wails event-dispatch goroutine returns immediately.
// We anchor on (a) the function's signature gaining a *sync.WaitGroup
// parameter per AC9, and (b) a `go func(` appearing inside the body.
func Test_10_5_AC8_OpenFileAndEmitDispatchesGoroutine(t *testing.T) {
	src := readSource(t, "main.go")
	// AC9 task 7 gains an extra *sync.WaitGroup parameter.
	sigRe := regexp.MustCompile(`func openFileAndEmitWithWarning\([^)]*\*sync\.WaitGroup[^)]*\)`)
	if !sigRe.MatchString(src) {
		t.Errorf("[P0] 10-5-AC8: openFileAndEmitWithWarning signature must include a `*sync.WaitGroup` parameter (AC8 + AC9 Task 7)")
	}
	body := extractTopLevelFuncBody(t, src, "openFileAndEmitWithWarning")
	if body == "" {
		t.Fatalf("[P0] 10-5-AC8: could not locate openFileAndEmitWithWarning in main.go")
	}
	if !strings.Contains(body, "go func(") {
		t.Errorf("[P0] 10-5-AC8: openFileAndEmitWithWarning must dispatch the pdfcpu read to `go func(...)` so the Wails event-dispatch goroutine returns immediately (AC8)")
	}
	// The goroutine takes path/extraWarning/svc/app as explicit arguments
	// for lifetime documentation. Loose anchor: the closure has at least
	// one parameter (matches the AC8 spec shape).
	closureRe := regexp.MustCompile(`go func\([^)]*\w+\s+\w+`)
	if !closureRe.MatchString(body) {
		t.Errorf("[P0] 10-5-AC8: the `go func(...)` in openFileAndEmitWithWarning should take explicit parameters (path, extraWarning, svc, app, wg) per AC8 (Goroutine arguments are used for explicit lifetime documentation)")
	}
}

// extractTopLevelFuncBody locates `func <name>(` at column 0 (no receiver,
// no method receiver pattern) and returns the body up to the next column-0
// `func `. Differs from extractFunctionBodyTopLevel by being defined in
// this file's scope so the AC8 / AC9 main.go tests don't depend on the
// AC5 helper.
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
// AC#9 -- main.go openFilesBatch uses per-iteration WaitGroup.Wait()
// ---------------------------------------------------------------------------

// Test_10_5_AC9_OpenFilesBatchUsesWaitGroup [P0] AC#9: the batch open
// dispatcher MUST declare a local `sync.WaitGroup` and call `wg.Wait()`
// at the end of each iteration of the dispatch loop. The exact shape is
// pinned in AC9's code block.
func Test_10_5_AC9_OpenFilesBatchUsesWaitGroup(t *testing.T) {
	src := readSource(t, "main.go")
	// openFilesBatch is a closure assignment (`openFilesBatch := func(...)`),
	// not a top-level func -- search for the assignment + body.
	startNeedle := "openFilesBatch := func("
	idx := strings.Index(src, startNeedle)
	if idx == -1 {
		t.Fatalf("[P0] 10-5-AC9: could not locate `openFilesBatch := func(` in main.go")
	}
	// Bound the body by looking for the next top-level closure or func
	// declaration. Anchor on the `document:batch-complete` emit which is
	// the last line of the closure (line 244 baseline).
	tail := src[idx:]
	endIdx := strings.Index(tail, `app.Event.Emit("document:batch-complete"`)
	if endIdx == -1 {
		t.Fatalf("[P0] 10-5-AC9: could not locate `document:batch-complete` emit inside openFilesBatch")
	}
	body := tail[:endIdx]
	// AC9 contract markers:
	if !strings.Contains(body, "var wg sync.WaitGroup") {
		t.Errorf("[P0] 10-5-AC9: openFilesBatch must declare a local `var wg sync.WaitGroup` (AC9 code shape)")
	}
	if !strings.Contains(body, "wg.Add(1)") {
		t.Errorf("[P0] 10-5-AC9: openFilesBatch must call `wg.Add(1)` before each goroutine dispatch (AC9 code shape)")
	}
	if !strings.Contains(body, "wg.Wait()") {
		t.Errorf("[P0] 10-5-AC9: openFilesBatch must call `wg.Wait()` per iteration to serialize file dispatch (AC9 code shape)")
	}
}

// Test_10_5_AC9_OpenFilesBatchHasFinalWait [P0] AC#9: the
// `app.Event.Emit("document:batch-complete", nil)` line MUST be preceded
// by a final `wg.Wait()` (defensive -- by per-iteration Wait the wg is
// already drained, but the explicit Wait survives future refactors).
func Test_10_5_AC9_OpenFilesBatchHasFinalWait(t *testing.T) {
	src := readSource(t, "main.go")
	emitNeedle := `app.Event.Emit("document:batch-complete"`
	emitIdx := strings.Index(src, emitNeedle)
	if emitIdx == -1 {
		t.Fatalf("[P0] 10-5-AC9: could not locate `document:batch-complete` emit in main.go")
	}
	// Look back ~10 lines for wg.Wait()
	start := max(emitIdx-400, 0)
	pre := src[start:emitIdx]
	if !strings.Contains(pre, "wg.Wait()") {
		t.Errorf("[P0] 10-5-AC9: openFilesBatch must call `wg.Wait()` immediately before the `document:batch-complete` emit (AC9 defensive final Wait)")
	}
}

// Test_10_5_AC9_BatchCancelledCheckBetweenIterations [P0] AC#9: the
// `batchCancelled.Load()` check MUST sit between iterations so cancel
// skips remaining un-kicked files without preempting in-flight ones.
// Baseline already has this -- pin it so the AC9 refactor does not lose
// the contract.
func Test_10_5_AC9_BatchCancelledCheckBetweenIterations(t *testing.T) {
	src := readSource(t, "main.go")
	if !strings.Contains(src, "batchCancelled.Load()") {
		t.Errorf("[P0] 10-5-AC9: main.go must retain `batchCancelled.Load()` check between iterations (AC9: cancel skips un-kicked files)")
	}
}
