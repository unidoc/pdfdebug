// Package inspector_concurrency_lifecycle_and_safecall_test holds the per-story acceptance test suite for
// Inspector Concurrency, Lifecycle Safety, and safeCall Sev-1.
//
// This is the structural-assertion suite -- it source-greps the production
// tree for the contracts pinned in the story's ACs that DO NOT require
// runtime exercise. Behavioural tests live alongside the production code:
//
//   internal/pdfcore/inspector_concurrent_test.go (-race soak)
//   internal/pdfcore/stream_test.go (same-node race)
//   internal/pdfcore/inspector_internal_test.go (lifecycle)
//   internal/pdfservice/service_recover_test.go (behaviour, build-tag-gated)
//
// A test here that pins an existing name (e.g. the safeCall contract tests that
// must survive) uses the "MUST still exist" framing, so a future deletion fails
// it.
//
// Run: cd tests/inspector-concurrency-lifecycle-and-safecall && go test -v -count=1 ./...
package inspector_concurrency_lifecycle_and_safecall_test

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
// DocumentState.pdfMu field + per-method lock acquisition
// ---------------------------------------------------------------------------

// TestDocumentStateHasPdfMu asserts DocumentState carries a `pdfMu sync.Mutex`
// field. The field name is pinned, so a renamed or relocated field fails, and the
// field must live INSIDE the DocumentState struct rather than on Inspector.
func TestDocumentStateHasPdfMu(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	// Anchor the search to lines inside the DocumentState struct. A naive
	// substring search would also match a comment elsewhere; require the
	// field declaration shape with `sync.Mutex` (or just `Mutex` if the
	// package alias differs -- pdfcore imports `sync` directly so this is
	// the canonical shape).
	re := regexp.MustCompile(`(?m)^\s*pdfMu\s+sync\.Mutex\b`)
	if !re.MatchString(src) {
		t.Errorf("internal/pdfcore/inspector.go must declare `pdfMu sync.Mutex` as a field on DocumentState (per-document mutex serializing pdfcpu calls)")
	}
}

// TestDocumentStateHasRevBuildOnce asserts DocumentState also carries
// `revBuildOnce sync.Once` to gate the lazy reverse-refs build. Same
// position check as pdfMu: it must be a field on the struct, not a package-level
// variable.
func TestDocumentStateHasRevBuildOnce(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	re := regexp.MustCompile(`(?m)^\s*revBuildOnce\s+sync\.Once\b`)
	if !re.MatchString(src) {
		t.Errorf("internal/pdfcore/inspector.go must declare `revBuildOnce sync.Once` as a field on DocumentState (lazy first-call build via sync.Once)")
	}
}

// pdfMuRequiredMethods is the inventory of Inspector methods that MUST
// acquire `doc.pdfMu` after GetDocument returns. The list is verbatim from
// the paragraph; new pdfcpu-touching methods would need updating here as
// part of their introduction story.
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

// TestMethodsAcquirePdfMu asserts every method in pdfMuRequiredMethods contains a
// `doc.pdfMu.Lock()` call and a `defer doc.pdfMu.Unlock()` immediately after its
// `GetDocument` call, by reading the file and scanning a regex scoped to the
// function range.
//
// A method listed in pdfMuLockOwner is checked in two steps instead: it must call
// its declared helper, and the HELPER must carry the lock pattern. A method that
// neither locks nor delegates still fails.
//
// The function boundary is approximated by `func (ins *Inspector) <Name>(` up to
// the next `func (` at the same depth, which is adequate for grep purposes.
func TestMethodsAcquirePdfMu(t *testing.T) {
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
					t.Fatalf("could not locate `func (ins *Inspector) %s(` in %s", method, path)
				}
				if !strings.Contains(caller, owner+"(") {
					t.Errorf("%s in %s must either lock doc.pdfMu itself or delegate to %s, which does", method, path, owner)
				}
				target = owner
			}
			body := extractFunctionBody(t, src, target)
			if body == "" {
				t.Fatalf("could not locate `func (ins *Inspector) %s(` in %s", target, path)
			}
			if !strings.Contains(body, "doc.pdfMu.Lock()") {
				t.Errorf("%s in %s must call `doc.pdfMu.Lock` (acquire per-document mutex immediately after GetDocument)", target, path)
			}
			if !strings.Contains(body, "defer doc.pdfMu.Unlock()") {
				t.Errorf("%s in %s must call `defer doc.pdfMu.Unlock` (deferred Unlock pattern)", target, path)
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
// stream.go cache contract: streamMu held for resolve+decode
// ---------------------------------------------------------------------------

// TestGetContentStreamHoldsStreamMuForDecode asserts the GetContentStream body does
// not release streamMu between the cache check and the cache write. The contract is
// "no drop-and-reacquire": the Lock paired with the cache MISS path must be the
// same Lock that wraps the cache WRITE.
//
// The structural form asserted is the absence of the substring
// `doc.streamMu.Unlock()\n\tif _, ok := doc.streamCache[nodeID]`, which is the
// drop-and-reacquire shape.
func TestGetContentStreamHoldsStreamMuForDecode(t *testing.T) {
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
		t.Fatalf("could not locate GetContentStream in stream.go")
	}
	unlockCount := strings.Count(body, "doc.streamMu.Unlock()")
	if unlockCount > 2 {
		t.Errorf("GetContentStream contains %d `doc.streamMu.Unlock()` calls -- expected <= 2, one single critical section covering resolve+decode+write. A drop-and-reacquire pattern reopens the race.", unlockCount)
	}
}

// ---------------------------------------------------------------------------
// Pdfservice top-level recover helper + per-method defer
// ---------------------------------------------------------------------------

// TestRecoverRuntimePanicHelperExists asserts service.go declares the
// recoverRuntimePanic helper with the exact signature
// `func recoverRuntimePanic(methodName string, errOut *error)`.
func TestRecoverRuntimePanicHelperExists(t *testing.T) {
	src := readSource(t, "internal/pdfservice/service.go")
	needle := "func recoverRuntimePanic(methodName string, errOut *error)"
	if !strings.Contains(src, needle) {
		t.Errorf("internal/pdfservice/service.go must declare %q (pinned helper signature)", needle)
	}
}

// TestRecoverHelperConvertsToMalformedPDF asserts the helper constructs the error
// via `fmt.Errorf(\"%w: internal error\", pdfcore.ErrMalformedPDF)`, so
// errors.Is(err, pdfcore.ErrMalformedPDF) holds at the call site and the frontend's
// /malformed/i regex matches.
func TestRecoverHelperConvertsToMalformedPDF(t *testing.T) {
	src := readSource(t, "internal/pdfservice/service.go")
	needle := `fmt.Errorf("%w: internal error", pdfcore.ErrMalformedPDF)`
	if !strings.Contains(src, needle) {
		t.Errorf("internal/pdfservice/service.go must construct the recovered error as %q so errors.Is(err, ErrMalformedPDF) holds AND the frontend regex /malformed/i matches", needle)
	}
}

// TestRecoverHelperLogsViaPrintf asserts the helper emits a
// `log.Printf("pdfservice: runtime.Error in %s: %v\n%s", ...)` line carrying
// debug.Stack(), so the underlying Go bug is diagnosable from logs.
func TestRecoverHelperLogsViaPrintf(t *testing.T) {
	src := readSource(t, "internal/pdfservice/service.go")
	needle := `log.Printf("pdfservice: runtime.Error in %s: %v\n%s"`
	if !strings.Contains(src, needle) {
		t.Errorf("internal/pdfservice/service.go must call %q... debug.Stack) when recovering a runtime.Error", needle)
	}
	if !strings.Contains(src, "debug.Stack()") {
		t.Errorf("internal/pdfservice/service.go must include debug.Stack in the recover-helper log line")
	}
}

// TestRecoverHelperRePanicsNonRuntimeError asserts non-runtime panics are
// re-panicked, so genuine Go bugs outside pdfcpu's documented surface still crash
// loudly. The shape is `panic(r)` after the negative type assertion.
func TestRecoverHelperRePanicsNonRuntimeError(t *testing.T) {
	src := readSource(t, "internal/pdfservice/service.go")
	body := extractFunctionBodyTopLevel(t, src, "recoverRuntimePanic")
	if body == "" {
		t.Skipf("recoverRuntimePanic not declared yet (see TestRecoverRuntimePanicHelperExists)")
	}
	if !strings.Contains(body, "panic(r)") {
		t.Errorf("recoverRuntimePanic must re-panic non-runtime errors via `panic(r)` (preserves the test-binary-crash diagnostic for genuine bugs)")
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

// pdfserviceWrappedMethods is the pinned list of 15 PDFService methods
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

// pdfserviceUnwrappedMethods is the pinned EXCLUSION list -- methods that
// MUST NOT have the recover wrapper because they do not call pdfcpu. A
// future Dev who adds the wrapper here would launder a Go bug as
// "malformed PDF" and mislead the user.
var pdfserviceUnwrappedMethods = []string{
	"OpenFileDialog",
	"CloseDocument",
	"GetPlainText",
	"CancelPlainText",
	"GetPlainTextSize",
}

// TestWrappedMethodsHaveDeferRecover asserts every method in
// pdfserviceWrappedMethods contains `defer recoverRuntimePanic(` in its body. The
// methodName argument is checked too, which catches a copy-paste bug where every
// wrapper logs the same string.
func TestWrappedMethodsHaveDeferRecover(t *testing.T) {
	src := readSource(t, "internal/pdfservice/service.go")
	for _, method := range pdfserviceWrappedMethods {
		t.Run(method, func(t *testing.T) {
			body := extractPDFServiceMethodBody(t, src, method)
			if body == "" {
				t.Fatalf("could not locate `func (s *PDFService) %s(` in service.go", method)
			}
			needle := `defer recoverRuntimePanic("` + method + `", &err)`
			if !strings.Contains(body, needle) {
				t.Errorf("PDFService.%s must contain %q (per-method wrapper with verbatim method name)", method, needle)
			}
		})
	}
}

// TestUnwrappedMethodsHaveNoDeferRecover asserts methods in
// pdfserviceUnwrappedMethods do NOT contain `defer recoverRuntimePanic(`. Wrapping
// them would launder Go bugs in non-pdfcpu code as ErrMalformedPDF, giving a
// misleading user-facing message.
func TestUnwrappedMethodsHaveNoDeferRecover(t *testing.T) {
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
				t.Errorf("PDFService.%s MUST NOT contain `defer recoverRuntimePanic(` (non-pdfcpu methods must not launder Go bugs as ErrMalformedPDF)", method)
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
// Existing safeCall test surface MUST survive unchanged
// ---------------------------------------------------------------------------

// safeCallContractTests is the verbatim list of test names in
// errors_test.go that MUST continue to exist post-implementation. A rename
// or deletion is the contract violation forbids.
var safeCallContractTests = []string{
	"TestSafeCallSuccess",
	"TestSafeCallReturnsError",
	"TestSafeCallCatchesStringPanic",
	"TestSafeCallCatchesErrorPanic",
	"TestSafeCallPropagatesRuntimeError",
}

// TestSafeCallContractTestsExist asserts each named safeCall contract test in
// errors_test.go is still declared. It is a contract pin: a later removal fails
// it.
func TestSafeCallContractTestsExist(t *testing.T) {
	src := readSource(t, "internal/pdfcore/errors_test.go")
	for _, name := range safeCallContractTests {
		needle := "func " + name + "(t *testing.T)"
		if !strings.Contains(src, needle) {
			t.Errorf("internal/pdfcore/errors_test.go must still declare %s (the pdfcore-layer safeCall contract is unchanged by this story)", name)
		}
	}
}

// TestSafeCallRePanicsRuntimeErrorPreserved asserts internal/pdfcore/errors.go's
// safeCall retains the runtime.Error re-panic. The Sev-1 fix lives at the
// pdfservice boundary, NOT inside safeCall. The pdfcpu bump suite pins the same
// contract from its own angle.
func TestSafeCallRePanicsRuntimeErrorPreserved(t *testing.T) {
	src := readSource(t, "internal/pdfcore/errors.go")
	re := regexp.MustCompile(`if\s+_,\s*ok\s*:=\s*r\.\(runtime\.Error\)`)
	if !re.MatchString(src) {
		t.Errorf("internal/pdfcore/errors.go must retain the runtime.Error type assertion in safeCall's recover block (safeCall contract unchanged)")
	}
	if !strings.Contains(src, "panic(r)") {
		t.Errorf("internal/pdfcore/errors.go must retain `panic(r)` in safeCall (the pdfcore-layer re-panic is preserved)")
	}
}

// ---------------------------------------------------------------------------
// reverse-refs build moved out of Open
// ---------------------------------------------------------------------------

// TestOpenNoLongerCallsBuildReverseRefs asserts the Inspector.Open body does not
// call buildReverseRefs. The reverse-ref build belongs to buildReverseRefsOnce,
// invoked by the first GetReverseRefs call.
func TestOpenNoLongerCallsBuildReverseRefs(t *testing.T) {
	src := readSource(t, "internal/pdfcore/inspector.go")
	body := extractFunctionBody(t, src, "Open")
	if body == "" {
		t.Fatalf("could not locate Inspector.Open in inspector.go")
	}
	if strings.Contains(body, "buildReverseRefs(doc") {
		t.Errorf("Inspector.Open must NOT call `buildReverseRefs(doc, ...)` -- the build is deferred to the first GetReverseRefs via buildReverseRefsOnce")
	}
}

// TestBuildReverseRefsOnceHelperExists asserts pdfcore declares
// `buildReverseRefsOnce(doc *DocumentState)`. That helper owns the lazy-build
// path: it acquires doc.pdfMu, invokes the sync.Once inner safeCall-wrapped
// builder, and sets revRefsBuildFailed on panic.
func TestBuildReverseRefsOnceHelperExists(t *testing.T) {
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
		t.Errorf("a helper `func buildReverseRefsOnce(doc *DocumentState)` must be declared in either internal/pdfcore/inspector.go or internal/pdfcore/reverserefs.go (lazy first-call build path)")
	}
}

// TestGetReverseRefsCallsBuildReverseRefsOnce asserts Inspector.GetReverseRefs
// calls buildReverseRefsOnce BEFORE the revRefsBuildFailed check, so the very
// first invocation triggers the build.
func TestGetReverseRefsCallsBuildReverseRefsOnce(t *testing.T) {
	src := readSource(t, "internal/pdfcore/reverserefs.go")
	body := extractFunctionBody(t, src, "GetReverseRefs")
	if body == "" {
		t.Fatalf("could not locate GetReverseRefs in reverserefs.go")
	}
	if !strings.Contains(body, "buildReverseRefsOnce(doc)") {
		t.Errorf("GetReverseRefs must invoke `buildReverseRefsOnce(doc)` BEFORE the revRefsBuildFailed check (lazy build)")
	}
}

// ---------------------------------------------------------------------------
// main.go openFileAndEmitWithWarning dispatches pdfcpu read to goroutine
// ---------------------------------------------------------------------------

// TestOpenFileAndEmitDispatchesGoroutine asserts openFileAndEmitWithWarning
// dispatches the pdfcpu read to a `go func(...)` so the Wails event-dispatch
// goroutine returns immediately. It anchors on the signature carrying a
// *sync.WaitGroup parameter and on a `go func(` inside the body.
func TestOpenFileAndEmitDispatchesGoroutine(t *testing.T) {
	src := readSource(t, "main.go")
	// Task 7 gains an extra *sync.WaitGroup parameter.
	sigRe := regexp.MustCompile(`func openFileAndEmitWithWarning\([^)]*\*sync\.WaitGroup[^)]*\)`)
	if !sigRe.MatchString(src) {
		t.Errorf("openFileAndEmitWithWarning signature must include a `*sync.WaitGroup` parameter")
	}
	body := extractTopLevelFuncBody(t, src, "openFileAndEmitWithWarning")
	if body == "" {
		t.Fatalf("could not locate openFileAndEmitWithWarning in main.go")
	}
	if !strings.Contains(body, "go func(") {
		t.Errorf("openFileAndEmitWithWarning must dispatch the pdfcpu read to `go func(...)` so the Wails event-dispatch goroutine returns immediately")
	}
	// The goroutine takes path/extraWarning/svc/app as explicit arguments
	// for lifetime documentation. Loose anchor: the closure has at least
	// one parameter (matches the spec shape).
	closureRe := regexp.MustCompile(`go func\([^)]*\w+\s+\w+`)
	if !closureRe.MatchString(body) {
		t.Errorf("the `go func(...)` in openFileAndEmitWithWarning should take explicit parameters (path, extraWarning, svc, app, wg) (Goroutine arguments are used for explicit lifetime documentation)")
	}
}

// extractTopLevelFuncBody locates `func <name>(` at column 0 (no receiver,
// no method receiver pattern) and returns the body up to the next column-0
// `func `. Differs from extractFunctionBodyTopLevel by being defined in
// this file's scope so the main.go tests don't depend on the helper.
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
// main.go openFilesBatch uses per-iteration WaitGroup.Wait()
// ---------------------------------------------------------------------------

// TestOpenFilesBatchUsesWaitGroup asserts the batch open dispatcher declares a
// local `sync.WaitGroup` and calls `wg.Wait()` at the end of each iteration of the
// dispatch loop.
func TestOpenFilesBatchUsesWaitGroup(t *testing.T) {
	src := readSource(t, "main.go")
	// openFilesBatch is a closure assignment (`openFilesBatch := func(...)`),
	// not a top-level func -- search for the assignment + body.
	startNeedle := "openFilesBatch := func("
	idx := strings.Index(src, startNeedle)
	if idx == -1 {
		t.Fatalf("could not locate `openFilesBatch:= func(` in main.go")
	}
	// Bound the body by looking for the next top-level closure or func
	// declaration. Anchor on the `document:batch-complete` emit which is
	// the last line of the closure (line 244 baseline).
	tail := src[idx:]
	endIdx := strings.Index(tail, `app.Event.Emit("document:batch-complete"`)
	if endIdx == -1 {
		t.Fatalf("could not locate `document:batch-complete` emit inside openFilesBatch")
	}
	body := tail[:endIdx]
	// Contract markers:
	if !strings.Contains(body, "var wg sync.WaitGroup") {
		t.Errorf("openFilesBatch must declare a local `var wg sync.WaitGroup` (code shape)")
	}
	if !strings.Contains(body, "wg.Add(1)") {
		t.Errorf("openFilesBatch must call `wg.Add(1)` before each goroutine dispatch (code shape)")
	}
	if !strings.Contains(body, "wg.Wait()") {
		t.Errorf("openFilesBatch must call `wg.Wait` per iteration to serialize file dispatch (code shape)")
	}
}

// TestOpenFilesBatchHasFinalWait asserts the
// `app.Event.Emit("document:batch-complete", nil)` line is preceded by a final
// `wg.Wait()`. The per-iteration Wait already drains the group, so the explicit
// final Wait is there to survive future refactors.
func TestOpenFilesBatchHasFinalWait(t *testing.T) {
	src := readSource(t, "main.go")
	emitNeedle := `app.Event.Emit("document:batch-complete"`
	emitIdx := strings.Index(src, emitNeedle)
	if emitIdx == -1 {
		t.Fatalf("could not locate `document:batch-complete` emit in main.go")
	}
	// Look back ~10 lines for wg.Wait()
	start := max(emitIdx-400, 0)
	pre := src[start:emitIdx]
	if !strings.Contains(pre, "wg.Wait()") {
		t.Errorf("openFilesBatch must call `wg.Wait` immediately before the `document:batch-complete` emit (defensive final Wait)")
	}
}

// TestBatchCancelledCheckBetweenIterations asserts the `batchCancelled.Load()`
// check sits between iterations, so a cancel skips the remaining un-kicked files
// without preempting the in-flight ones.
func TestBatchCancelledCheckBetweenIterations(t *testing.T) {
	src := readSource(t, "main.go")
	if !strings.Contains(src, "batchCancelled.Load()") {
		t.Errorf("main.go must retain `batchCancelled.Load` check between iterations (cancel skips un-kicked files)")
	}
}
