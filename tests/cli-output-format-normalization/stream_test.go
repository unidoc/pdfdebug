package cli_output_format_normalization_test

import (
	"encoding/json"
	"strings"
	"testing"
)

// streamFixturePage1 returns the args (minus binary, plus appended file) for a
// page-1 content-stream dump on the content-stream fixture.
func streamArgs(extra ...string) []string {
	return append([]string{"dump", "stream", "--page", "1"}, extra...)
}

// ---------------------------------------------------------------------------
// Default (no flag) emits a human-readable operator listing (plain text,
// NOT JSON): an operator sequence, one operator per line, operands before
// operator.
// ---------------------------------------------------------------------------

func TestStream_DefaultPlainOperatorListing(t *testing.T) {
	bin := buildCLI(t)
	file := fixture(t, "content-stream.pdf")
	args := append(streamArgs(), file)

	stdout, stderr, ec := runCLI(t, bin, args...)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	assertNotJSON(t, stdout)
	assertTrailingNewline(t, stdout)

	// The content-stream fixture begins with a BT text block. A plain operator
	// listing must surface operator tokens (e.g. BT / ET / Tf / Tj) somewhere,
	// one per line. Structural: at least one known operator appears on its own
	// line (last token), not whole-dump equality.
	lines := nonEmptyLines(stdout)
	if len(lines) == 0 {
		t.Fatalf("empty operator listing:\n%s", stdout)
	}
	sawOperatorLine := false
	for _, ln := range lines {
		fields := strings.Fields(ln)
		if len(fields) == 0 {
			continue
		}
		last := fields[len(fields)-1]
		switch last {
		case "BT", "ET", "Tf", "Tj", "TJ", "Td", "TL", "Tm", "re", "f", "S", "q", "Q":
			sawOperatorLine = true
		}
		if sawOperatorLine {
			break
		}
	}
	if !sawOperatorLine {
		t.Errorf("no recognizable PDF operator at end of any line (expected operands-then-operator order):\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// --json emits structured JSON of the operators.
// ---------------------------------------------------------------------------

func TestStream_JSONFlag(t *testing.T) {
	bin := buildCLI(t)
	file := fixture(t, "content-stream.pdf")
	args := append(streamArgs("--json"), file)

	stdout, stderr, ec := runCLI(t, bin, args...)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	if !parsesAsJSON(stdout) {
		t.Errorf("--json did not emit a parseable JSON document:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// --raw emits decoded stream bytes, UNCHANGED.
// --raw is a payload selector, exempt from the format rule.
// ---------------------------------------------------------------------------

func TestStream_RawUnchanged(t *testing.T) {
	bin := buildCLI(t)
	file := fixture(t, "content-stream.pdf")
	args := append(streamArgs("--raw"), file)

	stdout, stderr, ec := runCLI(t, bin, args...)
	if ec != 0 {
		t.Fatalf("--raw exit %d (stderr: %s)", ec, stderr)
	}
	// Raw decoded bytes are the content stream itself; must not be the JSON
	// ContentStreamData envelope.
	if strings.Contains(stdout, `"formatted"`) || strings.Contains(stdout, `"tokenized"`) {
		t.Errorf("--raw must emit decoded bytes, not the JSON envelope:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// --ops emits NDJSON (one JSON object per line), UNCHANGED. documented machine
// format with no plain-text equivalent.
// ---------------------------------------------------------------------------

func TestStream_OpsNDJSONUnchanged(t *testing.T) {
	bin := buildCLI(t)
	file := fixture(t, "content-stream.pdf")
	args := append(streamArgs("--ops"), file)

	stdout, stderr, ec := runCLI(t, bin, args...)
	if ec != 0 {
		t.Fatalf("--ops exit %d (stderr: %s)", ec, stderr)
	}
	lines := nonEmptyLines(stdout)
	if len(lines) == 0 {
		t.Fatalf("--ops produced no NDJSON lines:\n%s", stdout)
	}
	// Each non-empty line must independently parse as one JSON object carrying
	// the "op" key (the NDJSON per-operator contract).
	for i, ln := range lines {
		var obj map[string]any
		if err := json.Unmarshal([]byte(ln), &obj); err != nil {
			t.Errorf("NDJSON line %d is not valid JSON: %v\nline: %s", i, err, ln)
			continue
		}
		if _, ok := obj["op"]; !ok {
			t.Errorf("NDJSON line %d missing \"op\" key: %s", i, ln)
		}
	}
}

// ---------------------------------------------------------------------------
// --raw --json is rejected with a USAGE error (NET-NEW validation).
// Nonsensical combinations are rejected, not given invented semantics.
// ---------------------------------------------------------------------------

func TestStream_RawAndJSONRejected(t *testing.T) {
	bin := buildCLI(t)
	file := fixture(t, "content-stream.pdf")
	args := append(streamArgs("--raw", "--json"), file)

	stdout, stderr, ec := runCLI(t, bin, args...)
	if ec == 0 {
		t.Fatalf("expected a non-zero usage exit for --raw --json, got 0\nstdout:\n%s", stdout)
	}
	if strings.TrimSpace(stderr) == "" {
		t.Errorf("expected a usage error on stderr for --raw --json")
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("rejected combo must not write a payload to stdout:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// --ops --json is rejected with a USAGE error (NET-NEW validation). do NOT
// retrofit --ops under --json.
// ---------------------------------------------------------------------------

func TestStream_OpsAndJSONRejected(t *testing.T) {
	bin := buildCLI(t)
	file := fixture(t, "content-stream.pdf")
	args := append(streamArgs("--ops", "--json"), file)

	stdout, stderr, ec := runCLI(t, bin, args...)
	if ec == 0 {
		t.Fatalf("expected a non-zero usage exit for --ops --json, got 0\nstdout:\n%s", stdout)
	}
	if strings.TrimSpace(stderr) == "" {
		t.Errorf("expected a usage error on stderr for --ops --json")
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("rejected combo must not write a payload to stdout:\n%s", stdout)
	}
}
