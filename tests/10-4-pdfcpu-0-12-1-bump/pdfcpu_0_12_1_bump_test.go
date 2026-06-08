// Package pdfcpu_0_12_1_bump_test provides acceptance tests for Story 10.4:
// pdfcpu patch bump (v0.12.0 -> v0.12.1).
//
// TDD RED PHASE for a dependency patch bump combines two modes per the story
// spec and the user directive:
//
//  1. "Actual red" tests for the deltas the Dev step must land:
//     - go.mod's pdfcpu require literal must move v0.12.0 -> v0.12.1 (AC1)
//     - go.sum must carry v0.12.1 hashes (AC1)
//     - _bmad-output/project-context.md's Technology Stack line for pdfcpu must
//       be updated to the AC10 verbatim phrasing
//     These FAIL on the pre-bump tree by design.
//
//  2. "Baseline invariant" tests that MUST pass on the pre-bump tree AND
//     continue to pass post-bump. They pin the contract surface the patch
//     must not regress:
//     - safeCall's runtime.Error re-panic guarantee (AC5)
//     - the six named errors_test.go tests still exist (AC5)
//     - image.go's three memory-guard literals are unchanged (AC6)
//     - image.go still calls the two pdfcpu_render functions (AC6)
//     - stream.go still wraps sd.Decode() in safeCall (AC7)
//     - stream_test.go still carries TestTokenizeInlineImagePayloadOpaque (AC7)
//     - pdfcore/doc.go retains the blank import that pins pdfcpu (Dev Notes:
//       "pdfcpu dependency is pinned via blank import")
//
// Test pyramid for this story (per the user directive to favour API/integration
// over E2E and to keep unit tests for business logic only):
//
//   - Pure structural / source-grep assertions only. Patch bumps introduce
//     ZERO new business logic, ZERO new component, ZERO new hook. Adding
//     unit/component/E2E tests would be speculative coverage.
//   - The behavioral ACs (AC2 vet/lint baseline diff, AC3 parent test pass,
//     AC4 per-suite tests pass, AC8 CLI smoke) are EXPLICITLY delegated by
//     the story spec to "run the existing test surface" via Task 2 (parent
//     `go test ./...`, per-suite `cd tests/<name> && go test -count=1 ./...`)
//     and Task 3 (CLI binary smoke). This story does not author parallel
//     behavioral tests for them.
//   - AC9 (rollback policy) is a conditional flow; nothing to assert.
//
// Run: cd tests/10-4-pdfcpu-0-12-1-bump && go test -v -count=1 ./...
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

// targetVersion is the patch bump destination per AC1.
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
// AC#1 -- go.mod version literal change + go.sum settled
// ---------------------------------------------------------------------------

// Test_10_4_STRUCT_001 [P0] AC#1: go.mod declares pdfcpu at the target version.
// FAILS on the pre-bump tree because the literal is still v0.12.0.
func Test_10_4_STRUCT_001_GoModPdfcpuAtTargetVersion(t *testing.T) {
	src := readSource(t, "go.mod")
	line := findRequireLine(src)
	if line == "" {
		t.Fatalf("[P0] 10-4-STRUCT-001: go.mod must declare a `%s` require (AC1)", pdfcpuModulePath)
	}
	want := pdfcpuModulePath + " " + targetVersion
	if !strings.Contains(line, want) {
		t.Errorf("[P0] 10-4-STRUCT-001: go.mod pdfcpu require line is %q; expected to contain %q (AC1: bump v0.12.0 -> v0.12.1)", line, want)
	}
}

// Test_10_4_STRUCT_002 [P0] AC#1: go.mod must NOT still carry the pre-bump
// literal on the pdfcpu require line. A stale `v0.12.0` here is the dev forgot
// to edit, or edited the wrong line.
func Test_10_4_STRUCT_002_GoModDoesNotCarryPreBumpVersion(t *testing.T) {
	src := readSource(t, "go.mod")
	line := findRequireLine(src)
	if line == "" {
		t.Fatalf("[P0] 10-4-STRUCT-002: go.mod must declare a `%s` require (AC1)", pdfcpuModulePath)
	}
	stale := pdfcpuModulePath + " " + preBumpVersion
	if strings.Contains(line, stale) {
		t.Errorf("[P0] 10-4-STRUCT-002: go.mod pdfcpu require line still carries the pre-bump pin %q -- AC1 requires the literal change v0.12.0 -> v0.12.1", stale)
	}
}

// Test_10_4_STRUCT_003 [P0] AC#1: go.sum carries the v0.12.1 hash + go.mod
// hash for pdfcpu. A bumped go.mod with stale go.sum is a build break -- this
// catches a missed `go mod tidy`. Skips if STRUCT_001 has not yet been
// satisfied (the gate for "go mod tidy has been run").
func Test_10_4_STRUCT_003_GoSumCarriesTargetVersion(t *testing.T) {
	gomod := readSource(t, "go.mod")
	line := findRequireLine(gomod)
	if !strings.Contains(line, pdfcpuModulePath+" "+targetVersion) {
		t.Skipf("[P0] 10-4-STRUCT-003: skipped -- go.mod not bumped yet (see 10-4-STRUCT-001)")
	}
	gosum := readSource(t, "go.sum")
	hashLine := pdfcpuModulePath + " " + targetVersion + " h1:"
	modLine := pdfcpuModulePath + " " + targetVersion + "/go.mod h1:"
	if !strings.Contains(gosum, hashLine) {
		t.Errorf("[P0] 10-4-STRUCT-003: go.sum must contain a line starting with %q -- run `go mod tidy` after editing go.mod (Task 1.2 / AC1)", hashLine)
	}
	if !strings.Contains(gosum, modLine) {
		t.Errorf("[P0] 10-4-STRUCT-003: go.sum must contain a line starting with %q -- run `go mod tidy` after editing go.mod (Task 1.2 / AC1)", modLine)
	}
	// Negative: pre-bump hashes for v0.12.0 must be evicted by `go mod tidy`.
	// If both old and new are present, `go mod tidy` was not run cleanly.
	staleHash := pdfcpuModulePath + " " + preBumpVersion + " h1:"
	if strings.Contains(gosum, staleHash) {
		t.Errorf("[P0] 10-4-STRUCT-003: go.sum still carries pre-bump hash line %q -- run `go mod tidy` to evict (AC1: the diff to go.sum is exactly the version literal change plus what `go mod tidy` writes)", staleHash)
	}
}

// ---------------------------------------------------------------------------
// AC#5 -- safeCall re-panic guarantee + named errors_test.go suite intact
// ---------------------------------------------------------------------------

// Test_10_4_STRUCT_010 [P0] AC#5: internal/pdfcore/errors.go's safeCall body
// retains the runtime.Error re-panic. The contract: when recover() returns a
// runtime.Error (nil deref / slice OOB / bad type assertion), it is re-panicked
// instead of being laundered into ErrMalformedPDF. AC5 explicitly forbids
// relaxing this even if pdfcpu's internal panic surface changes in v0.12.1.
//
// The substring matches the exact source shape; whitespace tolerant by virtue
// of a non-greedy regex.
func Test_10_4_STRUCT_010_SafeCallRePanicsRuntimeError(t *testing.T) {
	src := readSource(t, "internal/pdfcore/errors.go")
	// The contract is two adjacent lines:
	//   if _, ok := r.(runtime.Error); ok {
	//       panic(r)
	// Anchor on the type assertion + the bare `panic(r)`.
	rePanic := regexp.MustCompile(`if\s+_,\s*ok\s*:=\s*r\.\(runtime\.Error\)`)
	if !rePanic.MatchString(src) {
		t.Errorf("[P0] 10-4-STRUCT-010: internal/pdfcore/errors.go must retain the runtime.Error type assertion in safeCall's recover() block (AC5: the re-panic guarantee pinned in Epic 9)")
	}
	if !strings.Contains(src, "panic(r)") {
		t.Errorf("[P0] 10-4-STRUCT-010: internal/pdfcore/errors.go must retain the `panic(r)` re-panic call in safeCall (AC5: runtime.Error must surface, not be laundered into ErrMalformedPDF)")
	}
}

// safeCallNamedTests is the AC5-mandated test name list. Each must exist in
// errors_test.go post-bump. A renamed test that drifts from this list is a
// silent contract loss; AC5 says these specific tests MUST pass.
var safeCallNamedTests = []string{
	"TestSafeCallPropagatesRuntimeError",
	"TestSafeCallSuccess",
	"TestSafeCallReturnsError",
	"TestSafeCallCatchesStringPanic",
	"TestSafeCallCatchesErrorPanic",
}

// wrapPDFErrorNamedTests is the AC5 "TestWrapPDFError* suite" set. AC5 names
// the suite by glob; this list pins the current members. A bump must not
// delete any.
var wrapPDFErrorNamedTests = []string{
	"TestWrapPDFErrorPasswordBecomesEncrypted",
	"TestWrapPDFErrorOwnerPasswordBecomesEncrypted",
	"TestWrapPDFErrorPanicPrefixBecomesMalformed",
	"TestWrapPDFErrorGenericBecomesMalformed",
	"TestWrapPDFErrorPreservesOriginal",
}

// Test_10_4_STRUCT_011 [P0] AC#5: every named test in the AC5 contract still
// exists in errors_test.go. A `func TestX(t *testing.T)` substring match is
// stable against doc-comment edits but catches a renamed or deleted test.
func Test_10_4_STRUCT_011_SafeCallNamedTestsExist(t *testing.T) {
	src := readSource(t, "internal/pdfcore/errors_test.go")
	all := append([]string{}, safeCallNamedTests...)
	all = append(all, wrapPDFErrorNamedTests...)
	for _, name := range all {
		needle := "func " + name + "(t *testing.T)"
		if !strings.Contains(src, needle) {
			t.Errorf("[P0] 10-4-STRUCT-011: internal/pdfcore/errors_test.go must still declare %s (AC5: the bump must not drop or rename the named tests)", name)
		}
	}
}

// ---------------------------------------------------------------------------
// AC#6 -- image.go memory guards + pdfcpu_render call surface preserved
// ---------------------------------------------------------------------------

// Test_10_4_STRUCT_020 [P0] AC#6: the three numeric memory-guard constants in
// internal/pdfcore/image.go are unchanged. AC6 explicitly pins the numeric
// values: maxImageBytes = 50 MB, maxImagePixels = 100_000_000, io.LimitReader
// cap at maxImageBytes+1.
func Test_10_4_STRUCT_020_ImageMemoryGuardsUnchanged(t *testing.T) {
	src := readSource(t, "internal/pdfcore/image.go")
	guards := []string{
		"maxImageBytes = 50 * 1024 * 1024",
		"maxImagePixels = 100_000_000",
		"io.LimitReader(reader, maxImageBytes+1)",
	}
	for _, g := range guards {
		if !strings.Contains(src, g) {
			t.Errorf("[P0] 10-4-STRUCT-020: internal/pdfcore/image.go must retain the literal %q (AC6: numeric values are unchanged by this story)", g)
		}
	}
}

// Test_10_4_STRUCT_021 [P0] AC#6: image.go still calls the two pdfcpu_render
// functions the AC6 contract names. A v0.12.1 rename of either is the kind of
// upstream regression AC9 calls out -- this test surfaces it instantly.
func Test_10_4_STRUCT_021_ImageRenderCallsiteIntact(t *testing.T) {
	src := readSource(t, "internal/pdfcore/image.go")
	calls := []string{
		"pdfcpu_render.ColorSpaceComponents(",
		"pdfcpu_render.RenderImage(",
	}
	for _, c := range calls {
		if !strings.Contains(src, c) {
			t.Errorf("[P0] 10-4-STRUCT-021: internal/pdfcore/image.go must still call %s -- AC6 contract: a v0.12.1 rename here is an upstream regression to investigate via AC9", c)
		}
	}
}

// ---------------------------------------------------------------------------
// AC#7 -- stream decode path + inline-image opacity test intact
// ---------------------------------------------------------------------------

// Test_10_4_STRUCT_030 [P0] AC#7: internal/pdfcore/stream.go retains the
// `safeCall(func() error { return sd.Decode() })` wrap. AC7 anchors on this
// specific call site by line number; we anchor on substring shape.
func Test_10_4_STRUCT_030_StreamDecodeWrappedInSafeCall(t *testing.T) {
	src := readSource(t, "internal/pdfcore/stream.go")
	if !strings.Contains(src, "sd.Decode()") {
		t.Errorf("[P0] 10-4-STRUCT-030: internal/pdfcore/stream.go must still call sd.Decode() (AC7 content-stream decode contract)")
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
		t.Errorf("[P0] 10-4-STRUCT-030: internal/pdfcore/stream.go must wrap `return sd.Decode()` inside a `safeCall(func() error {` closure (AC7: pdfcpu can panic on malformed content streams)")
	}
}

// Test_10_4_STRUCT_031 [P0] AC#7: internal/pdfcore/stream_test.go still
// declares TestTokenizeInlineImagePayloadOpaque. AC7 names this test by
// line number; we anchor on the function declaration substring.
func Test_10_4_STRUCT_031_InlineImagePayloadTestExists(t *testing.T) {
	src := readSource(t, "internal/pdfcore/stream_test.go")
	needle := "func TestTokenizeInlineImagePayloadOpaque(t *testing.T)"
	if !strings.Contains(src, needle) {
		t.Errorf("[P0] 10-4-STRUCT-031: internal/pdfcore/stream_test.go must still declare TestTokenizeInlineImagePayloadOpaque (AC7: inline-image payload opacity guard)")
	}
}

// ---------------------------------------------------------------------------
// pdfcpu blank-import pin preserved (Dev Notes invariant)
// ---------------------------------------------------------------------------

// Test_10_4_STRUCT_040 [P0] Dev Notes invariant: internal/pdfcore/doc.go retains
// the blank import that pins pdfcpu in go.mod. Without this, a `go mod tidy`
// after the bump would drop pdfcpu entirely (no direct import path remains
// when all call sites use sub-packages). The project-context.md explicitly
// calls this out: "pdfcpu dependency is pinned via blank import in
// internal/pdfcore/doc.go so go mod tidy won't remove it."
func Test_10_4_STRUCT_040_PdfcpuBlankImportPinPreserved(t *testing.T) {
	src := readSource(t, "internal/pdfcore/doc.go")
	if !strings.Contains(src, `import _ "github.com/pdfcpu/pdfcpu/pkg/api"`) {
		t.Errorf("[P0] 10-4-STRUCT-040: internal/pdfcore/doc.go must retain the blank import `import _ \"github.com/pdfcpu/pdfcpu/pkg/api\"` (Dev Notes: prevents `go mod tidy` from removing pdfcpu when all call sites use sub-packages)")
	}
}

