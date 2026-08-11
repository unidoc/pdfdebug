package cli_output_format_normalization_test

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// 13.1-INTG-020 [P1] FONT single-record plain shape: aligned key: value block.
// AC#2/#3 (Single record). Structural: a "key: value" line is present and the
// font-view kind is surfaced in plain text.
// ---------------------------------------------------------------------------

func TestFont_PlainShapeIsKeyValueBlock(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "font", "--ref", "4 0 R", fixture(t, "fonts-mixed.pdf"))
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	assertNotJSON(t, "13.1-INTG-020", stdout)
	if !containsLineWith(stdout, ":") {
		t.Errorf("expected a \"key: value\" line in font plain output:\n%s", stdout)
	}
	// The FontView kind ("detail"/"roster") drives what is rendered; the kind or
	// a font subtype label must be visible in plain text (structural presence).
	if !strings.Contains(strings.ToLower(stdout), "detail") &&
		!strings.Contains(strings.ToLower(stdout), "roster") &&
		!strings.Contains(strings.ToLower(stdout), "font") {
		t.Errorf("expected font kind/subtype label in plain output:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// 13.1-INTG-021 [P1] IMAGE single-record plain shape: aligned key: value block
// with the metadata fields (width/height). AC#3 (Single record). --metadata is
// an orthogonal payload selector and is unchanged; here it just keeps output
// small while still exercising the plain presenter.
// ---------------------------------------------------------------------------

func TestImage_PlainShapeShowsMetadata(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "image", "--metadata", "--ref", "4 0 R", fixture(t, "image-xobject.pdf"))
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	assertNotJSON(t, "13.1-INTG-021", stdout)
	lower := strings.ToLower(stdout)
	for _, label := range []string{"width", "height"} {
		if !strings.Contains(lower, label) {
			t.Errorf("expected %q label in image plain output:\n%s", label, stdout)
		}
	}
}

// ---------------------------------------------------------------------------
// 13.1-INTG-022 [P1] SOURCE default is plain text; --json wraps. AC#1/#7.
// The non-raw default flips from JSON-envelope to plain text; --json restores
// the {"objectRef","source"} JSON envelope.
// ---------------------------------------------------------------------------

func TestSource_DefaultPlain_JSONWraps(t *testing.T) {
	bin := buildCLI(t)
	file := fixture(t, "minimal.pdf")

	plain, stderr, ec := runCLI(t, bin, "dump", "source", "--ref", "2 0 R", file)
	if ec != 0 {
		t.Fatalf("plain exit %d (stderr: %s)", ec, stderr)
	}
	assertNotJSON(t, "13.1-INTG-022", plain)
	// Reserialized PDF object syntax must contain the object envelope keyword.
	if !strings.Contains(plain, "obj") {
		t.Errorf("expected reserialized PDF source (containing \"obj\") in plain output:\n%s", plain)
	}

	jsonOut, _, ecj := runCLI(t, bin, "dump", "source", "--json", "--ref", "2 0 R", file)
	if ecj != 0 {
		t.Fatalf("--json exit %d", ecj)
	}
	var env struct {
		ObjectRef string `json:"objectRef"`
		Source    string `json:"source"`
	}
	mustParseJSON(t, jsonOut, &env)
	if env.ObjectRef == "" || env.Source == "" {
		t.Errorf("--json envelope missing objectRef/source: %s", jsonOut)
	}
}

// ---------------------------------------------------------------------------
// 13.1-INTG-023 [P1] SOURCE --raw payload axis is UNCHANGED (verbatim bytes,
// not JSON, no trailing-newline contract imposed). AC#7: --raw is a payload
// selector orthogonal to --json, exempt from the format rule.
// ---------------------------------------------------------------------------

func TestSource_RawPayloadUnchanged(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "source", "--raw", "--ref", "2 0 R", fixture(t, "minimal.pdf"))
	if ec != 0 {
		t.Fatalf("--raw exit %d (stderr: %s)", ec, stderr)
	}
	assertNotJSON(t, "13.1-INTG-023", stdout)
	// Raw reserialized source is the verbatim object body, not the JSON envelope.
	if strings.Contains(stdout, `"objectRef"`) || strings.Contains(stdout, `"source"`) {
		t.Errorf("--raw must emit verbatim source bytes, not the JSON envelope:\n%s", stdout)
	}
	if !strings.Contains(stdout, "obj") {
		t.Errorf("--raw output is not reserialized PDF source:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// 13.1-INTG-023b [P1] SOURCE rejects --raw --json (mutually-exclusive payload
// vs format), mirroring dump stream. AC#7: --json no longer combines silently.
// ---------------------------------------------------------------------------

func TestSource_RawAndJSONRejected(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "source", "--raw", "--json", "--ref", "2 0 R", fixture(t, "minimal.pdf"))
	if ec != 1 {
		t.Fatalf("--raw --json must exit 1, got %d (stdout: %s)", ec, stdout)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("--raw --json must emit no stdout, got:\n%s", stdout)
	}
	if !strings.Contains(stderr, "mutually exclusive") {
		t.Errorf("expected usage error on stderr, got:\n%s", stderr)
	}
}

// ---------------------------------------------------------------------------
// 13.1-INTG-024 [P1] REVERSEREFS tabular plain shape: header row + one row per
// inbound ref. AC#3 (Uniform repeated records). Row count cross-checked vs JSON.
// ---------------------------------------------------------------------------

func TestReverseRefs_PlainShapeIsTableWithHeader(t *testing.T) {
	bin := buildCLI(t)
	file := fixture(t, "minimal.pdf")

	// Object 2 0 R (the /Pages dict) is pointed at by the catalog -> >= 1 ref.
	jsonOut, _, ecj := runCLI(t, bin, "dump", "reverserefs", "--json", "--ref", "2 0 R", file)
	if ecj != 0 {
		t.Fatalf("--json exit %d", ecj)
	}
	var refs []map[string]any
	mustParseJSON(t, jsonOut, &refs)
	wantRows := len(refs)
	if wantRows == 0 {
		t.Fatalf("expected at least one inbound ref to 2 0 R")
	}

	plain, stderr, ec := runCLI(t, bin, "dump", "reverserefs", "--ref", "2 0 R", file)
	if ec != 0 {
		t.Fatalf("plain exit %d (stderr: %s)", ec, stderr)
	}
	assertNotJSON(t, "13.1-INTG-024", plain)
	lines := nonEmptyLines(plain)
	if len(lines) < 2 {
		t.Fatalf("expected a header row plus >=1 data row, got %d lines:\n%s", len(lines), plain)
	}
	// Data rows (lines after the header) must equal the ref count.
	if dataRows := len(lines) - 1; dataRows != wantRows {
		t.Errorf("expected %d data rows (one per inbound ref), got %d\n%s",
			wantRows, dataRows, plain)
	}
}
