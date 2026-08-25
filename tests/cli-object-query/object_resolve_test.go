// Acceptance tests for `dump object --resolve`.
// Black-box: build the CLI and run it as a subprocess.
//
// The regression test PINS the no-flag output.
//
// Run: cd tests/cli-object-query && go test -v -count=1 ./...
package cli_object_query_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// WITHOUT --resolve, `dump object --ref REF` must not carry a `resolved` key.
// This checks two things: repeated invocations agree, and the key is absent.
// It does not compare against a stored baseline -- byte-for-byte equality with
// a previous build is what scripts/verify-cli-output-parity.sh covers.
// ---------------------------------------------------------------------------

func TestObjectResolve_OffIsUnchanged(t *testing.T) {
	bin := buildCLI(t)
	// 2 0 R is the /Pages node in the fixtures (has indirect ref children).
	pdfPath := filepath.Join(testdataDir(t), "multipage.pdf")

	a, _, ecA := runCLI(t, bin, "dump", "object", "--json", "--ref", "2 0 R", pdfPath)
	if ecA != 0 {
		t.Fatalf("baseline run exit %d", ecA)
	}
	b, _, ecB := runCLI(t, bin, "dump", "object", "--json", "--ref", "2 0 R", pdfPath)
	if ecB != 0 {
		t.Fatalf("second baseline run exit %d", ecB)
	}
	if a != b {
		t.Errorf("dump object default output non-deterministic; cannot pin no-regression contract")
	}
	// Is "byte-for-byte the current behavior": the --resolve field is strictly
	// additive, so without the flag the 'resolved' key must be absent.
	// Determinism alone would not catch an accidental always-on resolve.
	if strings.Contains(a, `"resolved"`) {
		t.Errorf("default dump object output contains a 'resolved' key; --resolve is not additive")
	}
}

// ---------------------------------------------------------------------------
// WITH --resolve, `dump object --ref REF` follows indirect-ref property
// values inline. The resolved output must DIFFER from the default
// (sub-objects expanded) and remain valid JSON, exit 0. Object 2 (/Pages)
// has a /Kids array of indirect refs to expand.
// ---------------------------------------------------------------------------

func TestObjectResolve_OnInlinesRefValues(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "multipage.pdf")

	def, _, ecDef := runCLI(t, bin, "dump", "object", "--json", "--ref", "2 0 R", pdfPath)
	if ecDef != 0 {
		t.Fatalf("default run exit %d", ecDef)
	}
	res, _, ecRes := runCLI(t, bin, "dump", "object", "--json", "--ref", "2 0 R", "--resolve", pdfPath)
	if ecRes != 0 {
		t.Fatalf("--resolve run exit %d (flag not implemented?)", ecRes)
	}
	if !json.Valid([]byte(strings.TrimSpace(res))) {
		t.Fatalf("--resolve output not valid JSON:\n%.300s", res)
	}
	if strings.TrimSpace(res) == strings.TrimSpace(def) {
		t.Errorf("--resolve output identical to default; ref values were not inlined")
	}
}

// ---------------------------------------------------------------------------
// --resolve on dump object is cycle-guarded: resolving an object whose ref
// graph contains a cycle terminates (does not hang) and exits 0 with valid
// JSON. The /Pages <-> /Page parent backref (Kids -> child -> /Parent ->
// Pages) is the natural cycle in the page tree.
// ---------------------------------------------------------------------------

func TestObjectResolve_CycleGuarded(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "multipage.pdf")

	res, _, ec := runCLI(t, bin, "dump", "object", "--json", "--ref", "2 0 R", "--resolve", pdfPath)
	if ec != 0 {
		t.Fatalf("--resolve on cyclic page tree exit %d (hang/overflow?)", ec)
	}
	if !json.Valid([]byte(strings.TrimSpace(res))) {
		t.Errorf("--resolve output not valid JSON under cycle")
	}
}
