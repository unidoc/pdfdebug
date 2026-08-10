// Story 11-5 RED-PHASE acceptance tests for `dump tree --resolve` (item 4;
// AC6). Black-box: build the CLI and run it as a subprocess.
//
// These assert the EXPECTED post-implementation behavior and MUST FAIL against
// the current binary (no --resolve flag) until Story 11-5 is implemented, with
// one exception: the regression test (TestTreeResolve_OffIsUnchanged) PINS
// today's no-flag behavior and should already pass -- it turns red only if a
// future change regresses the default output. That is the intended AC6
// no-regression guard.
//
// Run: cd tests/cli-tree-dump && go test -v -count=1 ./...
package cli_tree_dump_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// 11.5-INTG-AC6-001 [P0]: WITHOUT --resolve, `dump tree` output is byte-for-
// byte today's behavior (no regression). Captured twice (with/without the flag
// absent) to pin determinism; the real assertion is that adding NOTHING leaves
// the bytes identical to a second identical invocation, and that --resolve is
// strictly additive (covered by the "on" test below).
// ---------------------------------------------------------------------------

func TestTreeResolve_OffIsUnchanged(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := testdataDir(t) + "/multipage.pdf"

	a, _, ecA := runCLI(t, bin, "dump", "tree", "--json", "--page", "1", pdfPath)
	if ecA != 0 {
		t.Fatalf("[P0] 11.5-INTG-AC6-001: baseline run exit %d", ecA)
	}
	b, _, ecB := runCLI(t, bin, "dump", "tree", "--json", "--page", "1", pdfPath)
	if ecB != 0 {
		t.Fatalf("[P0] 11.5-INTG-AC6-001: second baseline run exit %d", ecB)
	}
	if a != b {
		t.Errorf("[P0] 11.5-INTG-AC6-001: dump tree default output is non-deterministic; cannot pin no-regression contract")
	}
	// AC6 "byte-for-byte the current behavior": --resolve is strictly additive,
	// so without the flag no 'resolved' key may appear. Determinism alone would
	// not catch an accidental always-on resolve.
	if strings.Contains(a, `"resolved"`) {
		t.Errorf("[P0] 11.5-INTG-AC6-001: default dump tree output contains a 'resolved' key; --resolve is not additive")
	}
}

// ---------------------------------------------------------------------------
// 11.5-INTG-AC6-002 [P0]: WITH --resolve, `dump tree` follows indirect refs
// inline. The resolved output must DIFFER from the default (refs expanded) and
// remain valid JSON, exit 0. Uses content-stream.pdf (page-rooted) which has a
// /Contents indirect ref to expand.
// ---------------------------------------------------------------------------

func TestTreeResolve_OnExpandsRefs(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := testdataDir(t) + "/content-stream.pdf"

	def, _, ecDef := runCLI(t, bin, "dump", "tree", "--json", "--page", "1", pdfPath)
	if ecDef != 0 {
		t.Fatalf("[P0] 11.5-INTG-AC6-002: default run exit %d", ecDef)
	}
	res, _, ecRes := runCLI(t, bin, "dump", "tree", "--json", "--resolve", "--page", "1", pdfPath)
	if ecRes != 0 {
		t.Fatalf("[P0] 11.5-INTG-AC6-002: --resolve run exit %d (flag not implemented?)", ecRes)
	}
	if !json_Valid(res) {
		t.Fatalf("[P0] 11.5-INTG-AC6-002: --resolve output is not valid JSON:\n%.300s", res)
	}
	if strings.TrimSpace(res) == strings.TrimSpace(def) {
		t.Errorf("[P0] 11.5-INTG-AC6-002: --resolve output identical to default; refs were not followed inline")
	}
}

// ---------------------------------------------------------------------------
// 11.5-INTG-AC6-003 [P1]: The resolve-depth flag is NOT --depth on dump tree.
// --depth must retain its existing tree-walk meaning (0 = unlimited): passing
// --depth alone (no --resolve) must NOT trigger ref-following and must still
// exit 0 with the existing tree-walk semantics. This guards the documented
// flag collision.
// ---------------------------------------------------------------------------

func TestTreeResolve_DepthFlagStillTreeWalk(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := testdataDir(t) + "/content-stream.pdf"

	// --depth 1 (tree-walk depth) with NO --resolve: existing behavior, exit 0,
	// valid JSON, and NOT byte-identical to an unlimited (--depth 0) walk.
	shallow, _, ec1 := runCLI(t, bin, "dump", "tree", "--json", "--depth", "1", "--page", "1", pdfPath)
	if ec1 != 0 {
		t.Fatalf("[P1] 11.5-INTG-AC6-003: --depth 1 exit %d", ec1)
	}
	if !json_Valid(shallow) {
		t.Fatalf("[P1] 11.5-INTG-AC6-003: --depth 1 output not valid JSON")
	}
	full, _, ec2 := runCLI(t, bin, "dump", "tree", "--json", "--depth", "0", "--page", "1", pdfPath)
	if ec2 != 0 {
		t.Fatalf("[P1] 11.5-INTG-AC6-003: --depth 0 exit %d", ec2)
	}
	if strings.TrimSpace(shallow) == strings.TrimSpace(full) {
		t.Errorf("[P1] 11.5-INTG-AC6-003: --depth still appears to be tree-walk depth but shallow==full; collision-safe semantics unclear")
	}
}

// json_Valid reports whether s is syntactically valid JSON.
func json_Valid(s string) bool {
	return json.Valid([]byte(strings.TrimSpace(s)))
}
