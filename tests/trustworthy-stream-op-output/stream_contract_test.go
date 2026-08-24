package trustworthy_stream_test

// Machine-contract correctness of `dump stream --json` / `--ops`.
// The two contract tests below pin leading-'+' operand tokenization and the
// absence of empty-operator --ops records. The fixture-sanity test guards the
// suite against an unparseable fixture.

import (
	"encoding/json"
	"strings"
	"testing"
)

// token mirrors the pdfcore.Token JSON wire shape emitted by `dump stream --json`.
type token struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// formattedLine mirrors the pdfcore.FormattedLine JSON wire shape.
type formattedLine struct {
	Tokens   []token `json:"tokens"`
	Operator string  `json:"operator"`
}

// streamJSON is the `dump stream --json` top-level object (subset).
type streamJSON struct {
	Tokenized []token         `json:"tokenized"`
	Formatted []formattedLine `json:"formatted"`
}

// opsRecord is one `dump stream --ops` NDJSON record (subset).
type opsRecord struct {
	Op     string   `json:"op"`
	Params []string `json:"params"`
}

// ---------------------------------------------------------------------------
// Fixture sanity: the hand-authored corpus fixtures must
// parse through the EXISTING open path (dump objects, exit 0). Passes TODAY;
// guards the suite against a broken fixture (signature-decomposition,
// compliance-validation and structural-diff precedent).
// ---------------------------------------------------------------------------

func TestStream_FixturesParseThroughOpenPath(t *testing.T) {
	bin := buildCLI(t)
	for _, name := range []string{"leading-plus.pdf", "comment-and-dangling.pdf"} {
		_, stderr, ec := runCLI(t, bin, "dump", "objects", fixturePath(t, name))
		if ec != 0 {
			t.Fatalf("fixture %q rejected by the existing open path (exit %d): %s", name, ec, stderr)
		}
	}
}

// ---------------------------------------------------------------------------
// Fixture F1: `dump stream --json` on leading-plus.pdf (content
// stream "+5 0 0 +5 0 0 cm").
//
// Each "+5" is a "number" token, "cm" is the sole operator, and no
// token/operator is named "+5" as an operator. A tokenizer that emitted "+5"
// as an OPERATOR would make Format group it as a standalone operation and
// --json report a bogus operator.
// ---------------------------------------------------------------------------

func TestStream_LeadingPlusJSON(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "stream", "--json", "--page", "1", fixturePath(t, "leading-plus.pdf"))
	if ec != 0 {
		t.Fatalf("--json must exit 0, got %d\nstdout: %s\nstderr: %s", ec, stdout, stderr)
	}

	var res streamJSON
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("failed to parse --json output: %v\nraw: %s", err, stdout)
	}

	// Every token whose value is "+5" must be classified as a number, never an
	// operator.
	plusCount := 0
	for _, tk := range res.Tokenized {
		if tk.Value == "+5" {
			plusCount++
			if tk.Type != "number" {
				t.Errorf("token %q has Type %q, want \"number\" (leading '+' mislabeled)", tk.Value, tk.Type)
			}
		}
	}
	if plusCount != 2 {
		t.Errorf("found %d \"+5\" tokens, want 2 (content stream is \"+5 0 0 +5 0 0 cm\")", plusCount)
	}

	// No formatted line may be named "+5"; exactly the "cm" operator terminates
	// the transform.
	sawCM := false
	for _, fl := range res.Formatted {
		if fl.Operator == "+5" {
			t.Errorf("a formatted operation is named operator %q; signed operands must not flush as their own operation", fl.Operator)
		}
		if fl.Operator == "cm" {
			sawCM = true
		}
	}
	if !sawCM {
		t.Errorf("no formatted operation with operator \"cm\"; the six signed operands should group under the single cm operator")
	}
}

// ---------------------------------------------------------------------------
// Fixture F2: `dump stream --ops` on comment-and-dangling.pdf (a "%
// comment" line plus a trailing dangling operand run with no operator).
//
// Zero records carry op == "" -- every emitted record has a non-empty op --
// and the real "cm" operator emits exactly one record. Without a guard on
// Operator, emitOps would yield a phantom {"op":""} record for the comment
// line and for each dangling operand.
// ---------------------------------------------------------------------------

func TestStream_CommentAndDanglingOps(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "stream", "--ops", "--page", "1", fixturePath(t, "comment-and-dangling.pdf"))
	if ec != 0 {
		t.Fatalf("--ops must exit 0, got %d\nstdout: %s\nstderr: %s", ec, stdout, stderr)
	}

	emptyOps := 0
	sawCM := false
	records := 0
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		records++
		var rec opsRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("NDJSON line is not valid JSON: %v\nline: %s", err, line)
		}
		if rec.Op == "" {
			emptyOps++
			t.Errorf("NDJSON record with empty op (phantom no-op record breaches the one-object-per-operator contract): %s", line)
		}
		if rec.Op == "cm" {
			sawCM = true
		}
	}
	if emptyOps != 0 {
		t.Errorf("%d record(s) with op == \"\"; want 0 (comment and dangling-operand lines must not emit)", emptyOps)
	}
	if !sawCM {
		t.Errorf("no record with op == \"cm\"; the real operator must still emit (got %d records)", records)
	}
}

// ---------------------------------------------------------------------------
// The F1 cascade on the --ops surface: closes traceability gap G1. The
// --ops-observable signal for leading-plus.pdf
// ("a single cm record whose params are the signed numbers, and NO +5 record")
// was only covered transitively. This drives
// `dump stream --ops --page 1 leading-plus.pdf` directly (content stream
// "+5 0 0 +5 0 0 cm") and asserts the operator-centric NDJSON contract: exactly
// one record, op == "cm", its params are the six signed operands (two "+5"s as
// bare strings), and no record is named op == "+5".
// ---------------------------------------------------------------------------

func TestStream_LeadingPlusOps(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "stream", "--ops", "--page", "1", fixturePath(t, "leading-plus.pdf"))
	if ec != 0 {
		t.Fatalf("--ops must exit 0, got %d\nstdout: %s\nstderr: %s", ec, stdout, stderr)
	}

	var cmRecords, plusRecords int
	var cmParams []string
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var rec opsRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("NDJSON line is not valid JSON: %v\nline: %s", err, line)
		}
		// A "+5" must never surface as an operator record: it is a numeric operand.
		if rec.Op == "+5" {
			plusRecords++
			t.Errorf("NDJSON record named op == %q; a signed operand must not flush as its own operation: %s", rec.Op, line)
		}
		if rec.Op == "cm" {
			cmRecords++
			cmParams = rec.Params
		}
	}

	if plusRecords != 0 {
		t.Errorf("%d record(s) named op == \"+5\"; want 0", plusRecords)
	}
	if cmRecords != 1 {
		t.Fatalf("found %d \"cm\" records, want exactly 1 (the six signed operands group under a single cm)", cmRecords)
	}

	// The cm record's params are the six operands verbatim, with the leading '+'
	// preserved as bare strings (--ops params are raw token values, not typed).
	want := []string{"+5", "0", "0", "+5", "0", "0"}
	if len(cmParams) != len(want) {
		t.Fatalf("cm record has %d params %v, want %d %v", len(cmParams), cmParams, len(want), want)
	}
	for i := range want {
		if cmParams[i] != want[i] {
			t.Errorf("cm param[%d] = %q, want %q (full: %v)", i, cmParams[i], want[i], cmParams)
		}
	}
}
