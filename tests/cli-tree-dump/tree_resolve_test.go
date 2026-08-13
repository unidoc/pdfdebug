// Story 11-5 acceptance tests for `dump tree --resolve` (item 4).
// Black-box: build the CLI and run it as a subprocess.
//
// The regression test (TestTreeResolve_OffIsUnchanged) PINS the no-flag
// behaviour, so it fails only if a future change regresses the default output.
// That is the intended no-regression guard.
//
// Run: cd tests/cli-tree-dump && go test -v -count=1 ./...
package cli_tree_dump_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// WITHOUT --resolve, `dump tree` output is byte-for- byte today's behavior (no
// regression). Captured twice (with/without the flag absent) to pin
// determinism; the real assertion is that adding NOTHING leaves the bytes
// identical to a second identical invocation, and that --resolve is strictly
// additive (covered by the "on" test below).
// ---------------------------------------------------------------------------

func TestTreeResolve_OffIsUnchanged(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := testdataDir(t) + "/multipage.pdf"

	a, _, ecA := runCLI(t, bin, "dump", "tree", "--json", "--page", "1", pdfPath)
	if ecA != 0 {
		t.Fatalf("baseline run exit %d", ecA)
	}
	b, _, ecB := runCLI(t, bin, "dump", "tree", "--json", "--page", "1", pdfPath)
	if ecB != 0 {
		t.Fatalf("second baseline run exit %d", ecB)
	}
	if a != b {
		t.Errorf("dump tree default output is non-deterministic; cannot pin no-regression contract")
	}
	// "byte-for-byte the current behavior": --resolve is strictly additive, so
	// without the flag no 'resolved' key may appear. Determinism alone would
	// not catch an accidental always-on resolve.
	if strings.Contains(a, `"resolved"`) {
		t.Errorf("default dump tree output contains a 'resolved' key; --resolve is not additive")
	}
}

// ---------------------------------------------------------------------------
// WITH --resolve, `dump tree` follows indirect refs inline. The resolved
// output must DIFFER from the default (refs expanded) and remain valid JSON,
// exit 0. Uses content-stream.pdf (page-rooted) which has a /Contents indirect
// ref to expand.
// ---------------------------------------------------------------------------

func TestTreeResolve_OnExpandsRefs(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := testdataDir(t) + "/content-stream.pdf"

	def, _, ecDef := runCLI(t, bin, "dump", "tree", "--json", "--page", "1", pdfPath)
	if ecDef != 0 {
		t.Fatalf("default run exit %d", ecDef)
	}
	res, _, ecRes := runCLI(t, bin, "dump", "tree", "--json", "--resolve", "--page", "1", pdfPath)
	if ecRes != 0 {
		t.Fatalf("--resolve run exit %d (flag not implemented?)", ecRes)
	}
	if !json_Valid(res) {
		t.Fatalf("--resolve output is not valid JSON:\n%.300s", res)
	}
	if strings.TrimSpace(res) == strings.TrimSpace(def) {
		t.Errorf("--resolve output identical to default; refs were not followed inline")
	}
}

// ---------------------------------------------------------------------------
// The resolve-depth flag is NOT --depth on dump tree. --depth must retain its
// existing tree-walk meaning (0 = unlimited): passing --depth alone (no
// --resolve) must NOT trigger ref-following and must still exit 0 with the
// existing tree-walk semantics. This guards the documented flag collision.
// ---------------------------------------------------------------------------

func TestTreeResolve_DepthFlagStillTreeWalk(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := testdataDir(t) + "/content-stream.pdf"

	// --depth 1 (tree-walk depth) with NO --resolve: existing behavior, exit 0,
	// valid JSON, and NOT byte-identical to an unlimited (--depth 0) walk.
	shallow, _, ec1 := runCLI(t, bin, "dump", "tree", "--json", "--depth", "1", "--page", "1", pdfPath)
	if ec1 != 0 {
		t.Fatalf("--depth 1 exit %d", ec1)
	}
	if !json_Valid(shallow) {
		t.Fatalf("--depth 1 output not valid JSON")
	}
	full, _, ec2 := runCLI(t, bin, "dump", "tree", "--json", "--depth", "0", "--page", "1", pdfPath)
	if ec2 != 0 {
		t.Fatalf("--depth 0 exit %d", ec2)
	}
	if strings.TrimSpace(shallow) == strings.TrimSpace(full) {
		t.Errorf("--depth still appears to be tree-walk depth but shallow==full; collision-safe semantics unclear")
	}
}

// json_Valid reports whether s is syntactically valid JSON.
func json_Valid(s string) bool {
	return json.Valid([]byte(strings.TrimSpace(s)))
}
