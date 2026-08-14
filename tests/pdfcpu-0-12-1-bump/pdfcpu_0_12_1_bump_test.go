// Package pdfcpu_0_12_1_bump_test provides acceptance tests for pdfcpu patch
// bump (v0.12.0 -> v0.12.1).
//
// The suite covers two things:
//
//  1. The bump deltas:
//     - go.mod's pdfcpu require literal at v0.12.1
//     - go.sum carrying v0.12.1 hashes
//     - _bmad-output/project-context.md's Technology Stack line for pdfcpu in
//       the verbatim phrasing
//
//  2. Baseline invariants, which hold on either side of the bump. They pin the
//     contract surface the patch must not regress:
//     - safeCall's runtime.Error re-panic guarantee
//     - the six named errors_test.go tests still exist
//     - image.go's three memory-guard literals are unchanged
//     - image.go still calls the two pdfcpu_render functions
//     - stream.go still wraps sd.Decode() in safeCall
//     - stream_test.go still carries TestTokenizeInlineImagePayloadOpaque
//     - pdfcore/doc.go retains the blank import that pins pdfcpu (Dev Notes:
//       "pdfcpu dependency is pinned via blank import")
//
// Test pyramid for this story (per the user directive to favour API/integration
// over E2E and to keep unit tests for business logic only):
//
//   - Pure structural / source-grep assertions only. Patch bumps introduce
//     ZERO new business logic, ZERO new component, ZERO new hook. Adding
//     unit/component/E2E tests would be speculative coverage.
//   - The behavioral ACs (vet/lint baseline diff, parent test pass,
//     per-suite tests pass, CLI smoke) are EXPLICITLY delegated by
//     the story spec to "run the existing test surface" via Task 2 (parent
//     `go test ./...`, per-suite `cd tests/<name> && go test -count=1 ./...`)
//     and Task 3 (CLI binary smoke). This story does not author parallel
//     behavioral tests for them.
//   - (rollback policy) is a conditional flow; nothing to assert.
//
// Run: cd tests/pdfcpu-0-12-1-bump && go test -v -count=1 ./...
package pdfcpu_0_12_1_bump_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// preBumpVersion is the current pinned pdfcpu version literal. The post-bump
// tree MUST replace this with targetVersion on the require line in go.mod.
const preBumpVersion = "v0.12.0"

// targetVersion is the patch bump destination.
const targetVersion = "v0.12.1"

// pdfcpuModulePath is the Go module path for pdfcpu, used to anchor regex /
// substring matches against go.mod and go.sum.
const pdfcpuModulePath = "github.com/pdfcpu/pdfcpu"

// projectRoot walks up from the working directory until it finds the project
// go.mod (module unidoc-pdf-debugger), and returns its absolute path.
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

// findRequireLine returns the line in go.mod that declares the pdfcpu require,
// or "" if absent.
func findRequireLine(src string) string {
	for line := range strings.SplitSeq(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, pdfcpuModulePath+" ") {
			return trimmed
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// go.mod version literal change + go.sum settled
// ---------------------------------------------------------------------------

// TestGoModPdfcpuAtTargetVersion asserts go.mod declares pdfcpu at the target
// version.
func TestGoModPdfcpuAtTargetVersion(t *testing.T) {
	src := readSource(t, "go.mod")
	line := findRequireLine(src)
	if line == "" {
		t.Fatalf("go.mod must declare a `%s` require", pdfcpuModulePath)
	}
	want := pdfcpuModulePath + " " + targetVersion
	if !strings.Contains(line, want) {
		t.Errorf("go.mod pdfcpu require line is %q; expected to contain %q (bump v0.12.0 -> v0.12.1)", line, want)
	}
}

// TestGoModDoesNotCarryPreBumpVersion asserts go.mod no longer carries the
// pre-bump literal on the pdfcpu require line. A stale `v0.12.0` means the edit
// was missed or landed on the wrong line.
func TestGoModDoesNotCarryPreBumpVersion(t *testing.T) {
	src := readSource(t, "go.mod")
	line := findRequireLine(src)
	if line == "" {
		t.Fatalf("go.mod must declare a `%s` require", pdfcpuModulePath)
	}
	stale := pdfcpuModulePath + " " + preBumpVersion
	if strings.Contains(line, stale) {
		t.Errorf("go.mod pdfcpu require line still carries the pre-bump pin %q -- it must read v0.12.1", stale)
	}
}

// TestGoSumCarriesTargetVersion asserts go.sum carries the v0.12.1 hash and go.mod
// hash for pdfcpu, catching a missed `go mod tidy` that would leave a bumped
// go.mod with a stale go.sum and a broken build. It skips while
// TestGoModPdfcpuAtTargetVersion is unsatisfied, which is the gate for "go mod
// tidy has been run".
func TestGoSumCarriesTargetVersion(t *testing.T) {
	gomod := readSource(t, "go.mod")
	line := findRequireLine(gomod)
	if !strings.Contains(line, pdfcpuModulePath+" "+targetVersion) {
		t.Skipf("skipped -- go.mod is not bumped to the target pdfcpu version yet")
	}
	gosum := readSource(t, "go.sum")
	hashLine := pdfcpuModulePath + " " + targetVersion + " h1:"
	modLine := pdfcpuModulePath + " " + targetVersion + "/go.mod h1:"
	if !strings.Contains(gosum, hashLine) {
		t.Errorf("go.sum must contain a line starting with %q -- run `go mod tidy` after editing go.mod", hashLine)
	}
	if !strings.Contains(gosum, modLine) {
		t.Errorf("go.sum must contain a line starting with %q -- run `go mod tidy` after editing go.mod", modLine)
	}
	// Negative: pre-bump hashes for v0.12.0 must be evicted by `go mod tidy`.
	// If both old and new are present, `go mod tidy` was not run cleanly.
	staleHash := pdfcpuModulePath + " " + preBumpVersion + " h1:"
	if strings.Contains(gosum, staleHash) {
		t.Errorf("go.sum still carries pre-bump hash line %q -- run `go mod tidy` to evict (the diff to go.sum is exactly the version literal change plus what `go mod tidy` writes)", staleHash)
	}
}

// ---------------------------------------------------------------------------
// safeCall re-panic guarantee + named errors_test.go suite intact
// ---------------------------------------------------------------------------

// TestSafeCallRePanicsRuntimeError asserts internal/pdfcore/errors.go's safeCall
// body retains the runtime.Error re-panic: when recover() returns a runtime.Error
// (nil deref, slice out of range, bad type assertion) it is re-panicked rather
// than laundered into ErrMalformedPDF. That holds even if pdfcpu's internal panic
// surface changes.
//
// The substring matches the exact source shape and is whitespace tolerant by
// virtue of a non-greedy regex.
func TestSafeCallRePanicsRuntimeError(t *testing.T) {
	src := readSource(t, "internal/pdfcore/errors.go")
	// The contract is two adjacent lines:
	//   if _, ok := r.(runtime.Error); ok {
	//       panic(r)
	// Anchor on the type assertion + the bare `panic(r)`.
	rePanic := regexp.MustCompile(`if\s+_,\s*ok\s*:=\s*r\.\(runtime\.Error\)`)
	if !rePanic.MatchString(src) {
		t.Errorf("internal/pdfcore/errors.go must retain the runtime.Error type assertion in safeCall's recover block (the re-panic guarantee)")
	}
	if !strings.Contains(src, "panic(r)") {
		t.Errorf("internal/pdfcore/errors.go must retain the `panic(r)` re-panic call in safeCall (runtime.Error must surface, not be laundered into ErrMalformedPDF)")
	}
}

// safeCallNamedTests is the mandated test name list. Each must exist in
// errors_test.go post-bump. A renamed test that drifts from this list is a
// silent contract loss; says these specific tests MUST pass.
var safeCallNamedTests = []string{
	"TestSafeCallPropagatesRuntimeError",
	"TestSafeCallSuccess",
	"TestSafeCallReturnsError",
	"TestSafeCallCatchesStringPanic",
	"TestSafeCallCatchesErrorPanic",
}

// wrapPDFErrorNamedTests is the "TestWrapPDFError* suite" set. names the
// suite by glob; this list pins the current members. A bump must not delete
// any.
var wrapPDFErrorNamedTests = []string{
	"TestWrapPDFErrorPasswordBecomesEncrypted",
	"TestWrapPDFErrorOwnerPasswordBecomesEncrypted",
	"TestWrapPDFErrorPanicPrefixBecomesMalformed",
	"TestWrapPDFErrorGenericBecomesMalformed",
	"TestWrapPDFErrorPreservesOriginal",
}

// TestSafeCallNamedTestsExist asserts every test named in the safeCall contract
// still exists in errors_test.go. A `func TestX(t *testing.T)` substring match is
// stable against doc-comment edits but catches a renamed or deleted test.
func TestSafeCallNamedTestsExist(t *testing.T) {
	src := readSource(t, "internal/pdfcore/errors_test.go")
	all := append([]string{}, safeCallNamedTests...)
	all = append(all, wrapPDFErrorNamedTests...)
	for _, name := range all {
		needle := "func " + name + "(t *testing.T)"
		if !strings.Contains(src, needle) {
			t.Errorf("internal/pdfcore/errors_test.go must still declare %s (the bump must not drop or rename the named tests)", name)
		}
	}
}

// ---------------------------------------------------------------------------
// image.go memory guards + pdfcpu_render call surface preserved
// ---------------------------------------------------------------------------

// TestImageMemoryGuardsUnchanged asserts the three numeric memory-guard constants
// in internal/pdfcore/image.go are unchanged: maxImageBytes = 50 MB,
// maxImagePixels = 100_000_000, and the io.LimitReader cap at maxImageBytes+1.
func TestImageMemoryGuardsUnchanged(t *testing.T) {
	src := readSource(t, "internal/pdfcore/image.go")
	guards := []string{
		"maxImageBytes = 50 * 1024 * 1024",
		"maxImagePixels = 100_000_000",
		"io.LimitReader(reader, maxImageBytes+1)",
	}
	for _, g := range guards {
		if !strings.Contains(src, g) {
			t.Errorf("internal/pdfcore/image.go must retain the literal %q (numeric values are unchanged by this story)", g)
		}
	}
}

// TestImageRenderCallsiteIntact asserts image.go still calls the two
// pdfcpu_render functions it depends on. An upstream rename of either is exactly
// the kind of regression a version bump can carry, and this surfaces it
// instantly.
func TestImageRenderCallsiteIntact(t *testing.T) {
	src := readSource(t, "internal/pdfcore/image.go")
	calls := []string{
		"pdfcpu_render.ColorSpaceComponents(",
		"pdfcpu_render.RenderImage(",
	}
	for _, c := range calls {
		if !strings.Contains(src, c) {
			t.Errorf("internal/pdfcore/image.go must still call %s -- a rename here is an upstream regression to investigate before bumping", c)
		}
	}
}

// ---------------------------------------------------------------------------
// Stream decode path + inline-image opacity test intact
// ---------------------------------------------------------------------------

// TestStreamDecodeWrappedInSafeCall asserts internal/pdfcore/stream.go retains the
// `safeCall(func() error { return sd.Decode() })` wrap, anchored on the substring
// shape rather than a line number.
func TestStreamDecodeWrappedInSafeCall(t *testing.T) {
	src := readSource(t, "internal/pdfcore/stream.go")
	if !strings.Contains(src, "sd.Decode()") {
		t.Errorf("internal/pdfcore/stream.go must still call sd.Decode (content-stream decode contract)")
	}
	// Anchor that the call sits inside a safeCall closure. A line-by-line
	// scan looking for `return sd.Decode()` immediately preceded (within 5
	// lines) by `safeCall(func() error {`.
	lines := strings.Split(src, "\n")
	wrapped := false
	for i, line := range lines {
		if !strings.Contains(line, "return sd.Decode()") {
			continue
		}
		// Look back up to 5 lines for the safeCall opener.
		start := max(i-5, 0)
		for j := i - 1; j >= start; j-- {
			if strings.Contains(lines[j], "safeCall(func() error {") {
				wrapped = true
				break
			}
		}
		if wrapped {
			break
		}
	}
	if !wrapped {
		t.Errorf("internal/pdfcore/stream.go must wrap `return sd.Decode` inside a `safeCall(func error {` closure (pdfcpu can panic on malformed content streams)")
	}
}

// TestInlineImagePayloadTestExists asserts internal/pdfcore/stream_test.go still
// declares TestTokenizeInlineImagePayloadOpaque, anchored on the function
// declaration substring rather than a line number.
func TestInlineImagePayloadTestExists(t *testing.T) {
	src := readSource(t, "internal/pdfcore/stream_test.go")
	needle := "func TestTokenizeInlineImagePayloadOpaque(t *testing.T)"
	if !strings.Contains(src, needle) {
		t.Errorf("internal/pdfcore/stream_test.go must still declare TestTokenizeInlineImagePayloadOpaque (inline-image payload opacity guard)")
	}
}

// ---------------------------------------------------------------------------
// pdfcpu blank-import pin preserved (Dev Notes invariant)
// ---------------------------------------------------------------------------

// TestPdfcpuBlankImportPinPreserved asserts internal/pdfcore/doc.go retains the
// blank import that pins pdfcpu in go.mod. Without it a `go mod tidy` would drop
// pdfcpu entirely, because every call site uses sub-packages and no direct import
// path remains.
func TestPdfcpuBlankImportPinPreserved(t *testing.T) {
	src := readSource(t, "internal/pdfcore/doc.go")
	if !strings.Contains(src, `import _ "github.com/pdfcpu/pdfcpu/pkg/api"`) {
		t.Errorf("internal/pdfcore/doc.go must retain the blank import `import _ \"github.com/pdfcpu/pdfcpu/pkg/api\"` (Dev Notes: prevents `go mod tidy` from removing pdfcpu when all call sites use sub-packages)")
	}
}

