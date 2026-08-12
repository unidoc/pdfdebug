package cli_output_format_normalization_test

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// 13.1-INTG-010 [P0] TREE plain shape: hierarchical, two-space indents, key as
// the spine. AC#3 (Hierarchical). Structural: assert at least one indented
// child line under a less-indented parent, and node labels present.
// ---------------------------------------------------------------------------

func TestTree_PlainShapeIsIndentedHierarchy(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "tree", fixture(t, "minimal.pdf"))
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	lines := nonEmptyLines(stdout)
	if len(lines) < 2 {
		t.Fatalf("expected a multi-line tree, got %d non-empty lines:\n%s", len(lines), stdout)
	}

	// Root line is at indent 0.
	if leadingSpaces(lines[0]) != 0 {
		t.Errorf("root line is indented (want indent 0), got %d: %q", leadingSpaces(lines[0]), lines[0])
	}

	// At least one line must be indented by exactly two spaces (a direct child),
	// proving the two-space-per-level indent shape.
	sawTwoSpaceChild := false
	for _, ln := range lines {
		if leadingSpaces(ln) == 2 {
			sawTwoSpaceChild = true
			break
		}
	}
	if !sawTwoSpaceChild {
		t.Errorf("no line indented by exactly two spaces (expected two-space child indent):\n%s", stdout)
	}

	// The catalog is the spine: a "Catalog" (or "Pages") label must appear, with
	// the indirect ref metadata trailing. Structural label presence, not equality.
	if !strings.Contains(stdout, "Pages") && !strings.Contains(stdout, "Catalog") {
		t.Errorf("expected a Catalog/Pages node label in the tree:\n%s", stdout)
	}
	// At least one indirect node must show its "(N G R)"-style ref as trailing
	// metadata (the minimal.pdf catalog is object 1 0 R, pages 2 0 R).
	if !containsLineWith(stdout, "1 0 R") && !containsLineWith(stdout, "2 0 R") {
		t.Errorf("expected an indirect-object ref (e.g. \"1 0 R\") as trailing metadata:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// 13.1-INTG-011 [P1] OBJECT single-record plain shape: aligned key: value
// block. AC#3 (Single record). Structural: expected keys present with a colon
// separator. The bare (non-resolved) object dump is a single record.
// ---------------------------------------------------------------------------

func TestObject_PlainShapeIsKeyValueBlock(t *testing.T) {
	bin := buildCLI(t)
	// Object 2 0 R in minimal.pdf is the /Pages dict (has a /Type key).
	stdout, stderr, ec := runCLI(t, bin, "dump", "object", "--ref", "2 0 R", fixture(t, "minimal.pdf"))
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	// The single-record default must be plain text, not the JSON ObjectDetail.
	assertNotJSON(t, stdout)
	lines := nonEmptyLines(stdout)
	if len(lines) == 0 {
		t.Fatalf("empty object dump:\n%s", stdout)
	}
	// A single-record presenter renders "key: value" lines. At least one line
	// must contain a colon-separated key (structural, not whole-dump equality).
	sawKeyValue := false
	for _, ln := range lines {
		if idx := strings.Index(ln, ":"); idx > 0 && idx < len(strings.TrimRight(ln, " ")) {
			sawKeyValue = true
			break
		}
	}
	if !sawKeyValue {
		t.Errorf("no \"key: value\" line found in single-record object dump:\n%s", stdout)
	}
	// The /Type Pages dict should surface its Type somewhere in plain text.
	if !strings.Contains(stdout, "Pages") {
		t.Errorf("expected the /Type Pages value to appear in plain output:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// 13.1-INTG-012 [P0] XREF tabular plain shape: header row + N aligned data rows.
// AC#3 (Uniform repeated records). Structural: a header line naming the columns,
// then one data row per xref entry. Row count cross-checked against --json.
// ---------------------------------------------------------------------------

func TestXRef_PlainShapeIsTableWithHeader(t *testing.T) {
	bin := buildCLI(t)
	file := fixture(t, "minimal.pdf")

	// Ground-truth entry count from the JSON contract.
	jsonOut, _, ecj := runCLI(t, bin, "dump", "xref", "--json", file)
	if ecj != 0 {
		t.Fatalf("--json exit %d", ecj)
	}
	var table struct {
		Entries []map[string]any `json:"entries"`
	}
	mustParseJSON(t, jsonOut, &table)
	wantRows := len(table.Entries)
	if wantRows == 0 {
		t.Fatalf("fixture produced zero xref entries; cannot assert table shape")
	}

	plain, stderr, ec := runCLI(t, bin, "dump", "xref", file)
	if ec != 0 {
		t.Fatalf("plain exit %d (stderr: %s)", ec, stderr)
	}
	lines := nonEmptyLines(plain)

	// First non-empty line is the header row naming the columns. We assert the
	// presence of column labels structurally (case-insensitive), not exact text.
	header := strings.ToUpper(lines[0])
	for _, col := range []string{"OBJ", "GEN", "TYPE", "OFFSET"} {
		if !strings.Contains(header, col) {
			t.Errorf("header row missing %q column label: %q", col, lines[0])
		}
	}

	// Data rows = total lines minus the single header row.
	dataRows := len(lines) - 1
	if dataRows != wantRows {
		t.Errorf("expected %d data rows (one per xref entry) under a header, got %d\n%s",
			wantRows, dataRows, plain)
	}
}

// ---------------------------------------------------------------------------
// 13.1-INTG-013 [P1] OBJECTS (plural) tabular plain shape: header row + one row
// per object-index entry. AC#3 (Uniform repeated records).
// ---------------------------------------------------------------------------

func TestObjects_PlainShapeIsTableWithHeader(t *testing.T) {
	bin := buildCLI(t)
	file := fixture(t, "multipage.pdf")

	jsonOut, _, ecj := runCLI(t, bin, "dump", "objects", "--json", file)
	if ecj != 0 {
		t.Fatalf("--json exit %d", ecj)
	}
	var entries []map[string]any
	mustParseJSON(t, jsonOut, &entries)
	wantRows := len(entries)
	if wantRows == 0 {
		t.Fatalf("fixture produced zero object-index entries")
	}

	plain, stderr, ec := runCLI(t, bin, "dump", "objects", file)
	if ec != 0 {
		t.Fatalf("plain exit %d (stderr: %s)", ec, stderr)
	}
	lines := nonEmptyLines(plain)
	header := strings.ToUpper(lines[0])
	for _, col := range []string{"OBJ", "GEN", "TYPE"} {
		if !strings.Contains(header, col) {
			t.Errorf("header row missing %q column label: %q", col, lines[0])
		}
	}
	if dataRows := len(lines) - 1; dataRows != wantRows {
		t.Errorf("expected %d data rows (one per object entry), got %d\n%s",
			wantRows, dataRows, plain)
	}
}
