// CLI ergonomics & discoverability -- acceptance tests.
//
// Black-box: build the CLI, run as a subprocess, parse the {"error": ...} JSON
// value (never byte-match escaped quotes).
//
// Covers in this suite: obj:G:N accepted by --ref; the malformed obj: form
// rejected with the both-forms error; the reversed-ref "did you mean"
// suggestion; the malformed --ref format tip; --pretty for dump object.
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
// `--ref "obj:0:N"` resolves the SAME object as `--ref "N 0 R"`. The
// obj:G:N id (as emitted by dump tree) is accepted directly; output is
// identical to the canonical form.
// ---------------------------------------------------------------------------

func TestObjectDump_AcceptsObjGNForm(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	// Discover a real indirect node id (obj:G:N) and its canonical ref.
	ref, nodeID := discoverValidRef(t, bin, pdfPath)

	// Canonical "N G R" form.
	canonOut, _, canonExit := runCLI(t, bin, "dump", "object", "--json", "--ref", ref, pdfPath)
	if canonExit != 0 {
		t.Fatalf("canonical ref %q failed (exit %d)", ref, canonExit)
	}

	// obj:G:N form (paste the tree node id straight in).
	objOut, _, objExit := runCLI(t, bin, "dump", "object", "--json", "--ref", nodeID, pdfPath)
	if objExit != 0 {
		t.Fatalf("obj: form %q failed (exit %d) -- should be accepted", nodeID, objExit)
	}

	if objOut != canonOut {
		t.Errorf("obj: form output differs from canonical form\nobj: %s\ncanon:%s", objOut, canonOut)
	}
}

// ---------------------------------------------------------------------------
// A malformed obj: form (wrong colon count, non-numeric parts) is rejected
// with exit code 1, and the error text names BOTH accepted forms (canonical
// "N G R" and the obj:G:N id form) so it is coherent regardless of which
// form the user attempted.
// ---------------------------------------------------------------------------

func TestObjectDump_MalformedObjForm_Rejected(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	cases := []struct {
		name string
		ref  string
	}{
		{"too few colons", "obj:5"},
		{"non-numeric parts", "obj:a:b"},
		{"trailing colon", "obj:0:"},
		{"too many colons", "obj:0:5:1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, exitCode := runCLI(t, bin, "dump", "object", "--ref", tc.ref, pdfPath)

			if exitCode != 1 {
				t.Errorf("%s: expected exit code 1, got %d", tc.name, exitCode)
			}
			if strings.TrimSpace(stdout) != "" {
				t.Errorf("%s: stdout should be empty, got: %s", tc.name, stdout)
			}

			var errObj map[string]string
			if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
				t.Fatalf("%s: stderr is not valid JSON: %v\nraw: %s", tc.name, err, stderr)
			}
			msg := strings.ToLower(errObj["error"])

			// Error must reference the canonical "N G R" form AND the obj: form.
			if !strings.Contains(msg, "n g r") {
				t.Errorf("%s: error should name the \"N G R\" form\nerror: %s", tc.name, errObj["error"])
			}
			if !strings.Contains(msg, "obj:") {
				t.Errorf("%s: error should also name the obj:G:N form\nerror: %s", tc.name, errObj["error"])
			}
		})
	}
}

// ---------------------------------------------------------------------------
// A reversed reference (objNum 0, genNum > 0) that is not found gets the
// heuristic suggestion appended to the JSON `error` value: `did you mean:
// dump object --ref "25 0 R"` (operands swapped). Exit code stays 2 and the
// base `object not found: 0 25 R` text is preserved.
// ---------------------------------------------------------------------------

func TestObjectDump_ReversedRef_SuggestsSwap(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, stderr, exitCode := runCLI(t, bin, "dump", "object", "--ref", "0 25 R", pdfPath)

	if exitCode != 2 {
		t.Errorf("expected exit code 2 (runtime not-found), got %d", exitCode)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout should be empty, got: %s", stdout)
	}

	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}
	msg := errObj["error"]

	// Base not-found text preserved.
	if !strings.Contains(msg, "object not found: 0 25 R") {
		t.Errorf("base not-found text not preserved\nerror: %s", msg)
	}
	// Heuristic suggestion with swapped operands present (matched on the
	// decoded error VALUE, not raw escaped bytes).
	if !strings.Contains(msg, `did you mean: dump object --ref "25 0 R"`) {
		t.Errorf("missing swapped-operand suggestion\nerror: %s", msg)
	}
}

// ---------------------------------------------------------------------------
// Negative: "0 0 R" (free-list head) stays a PLAIN not-found -- the reversal
// heuristic must NOT fire (genNum is not > 0).
// ---------------------------------------------------------------------------

func TestObjectDump_ZeroZeroRef_NoSuggestion(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	_, stderr, exitCode := runCLI(t, bin, "dump", "object", "--ref", "0 0 R", pdfPath)
	if exitCode != 2 {
		t.Errorf("expected exit code 2, got %d", exitCode)
	}

	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}
	if strings.Contains(strings.ToLower(errObj["error"]), "did you mean") {
		t.Errorf("'0 0 R' must NOT trigger the reversal suggestion\nerror: %s", errObj["error"])
	}
}

// ---------------------------------------------------------------------------
// A malformed --ref error states the canonical "<objNum> <gen> R" format,
// mentions the obj:G:N form is also accepted, and notes that `dump tree`
// emits a ready-to-use pdfRef field. The legacy
// TestObjectDump_MalformedRef_ClearError substring set is still satisfiable.
// ---------------------------------------------------------------------------

func TestObjectDump_MalformedRef_FormatTip(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	_, stderr, exitCode := runCLI(t, bin, "dump", "object", "--ref", "abc", pdfPath)
	if exitCode != 1 {
		t.Errorf("expected exit code 1, got %d", exitCode)
	}

	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("stderr is not valid JSON: %v\nraw: %s", err, stderr)
	}
	msg := strings.ToLower(errObj["error"])

	// Mentions the obj: form is also accepted.
	if !strings.Contains(msg, "obj:") {
		t.Errorf("error should mention the obj:G:N form is accepted\nerror: %s", errObj["error"])
	}
	// Tip pointing at the tree's pdfRef field.
	if !strings.Contains(msg, "pdfref") {
		t.Errorf("error should tip that dump tree emits a ready-to-paste pdfRef\nerror: %s", errObj["error"])
	}
	// Legacy format/reference language preserved.
	if !strings.Contains(msg, "n g r") && !strings.Contains(msg, "format") && !strings.Contains(msg, "reference") {
		t.Errorf("error should still describe the expected ref format\nerror: %s", errObj["error"])
	}
}

// ---------------------------------------------------------------------------
// `dump object --pretty` emits indented multi-line JSON; default stays
// compact single-line. Both decode to the same object.
// ---------------------------------------------------------------------------

func TestObjectDump_PrettyVsCompact(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	ref, _ := discoverValidRef(t, bin, pdfPath)

	compact, _, ec := runCLI(t, bin, "dump", "object", "--json", "--ref", ref, pdfPath)
	if ec != 0 {
		t.Fatalf("compact run exit %d", ec)
	}
	pretty, _, ep := runCLI(t, bin, "dump", "object", "--json", "--pretty", "--ref", ref, pdfPath)
	if ep != 0 {
		t.Fatalf("--pretty run exit %d", ep)
	}

	if strings.Count(strings.TrimRight(compact, "\n"), "\n") != 0 {
		t.Errorf("default object output is not single-line compact:\n%s", compact)
	}
	if !strings.Contains(pretty, "\n  ") {
		t.Errorf("--pretty object output is not indented multi-line:\n%s", pretty)
	}

	var a, b any
	mustParseJSON(t, compact, &a)
	mustParseJSON(t, pretty, &b)
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Error("--pretty and compact object decode to different content")
	}
}
