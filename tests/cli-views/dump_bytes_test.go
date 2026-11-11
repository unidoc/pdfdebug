// `dump bytes`: the command that streams raw document bytes.
//
// Test level: Integration (Go) -- CLI binary build + execution. No browser.
//
// Covers: --pretty parity on the JSON wrapper; the per-path stderr shapes
// (runtime error is JSON, usage error is a bare usage line); usage errors for a
// flag after <file> and for an extra positional; the help text naming
// `dump bytes` and pdftotext while carrying no occurrence of the lowercase
// token `plaintext`; and `dump plaintext` failing as an unknown resource.
//
// Run: cd tests/cli-views && go test -v -count=1 ./...
package cli_views_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// bytesUsageLine is what parseDocViewFlags prints on a usage failure.
const bytesUsageLine = "Usage: pdfdebug dump bytes [--json] [--pretty] <file>"

// stderrLines splits stderr into lines, dropping the trailing empty element a
// final newline produces.
func stderrLines(s string) []string {
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

// ---------------------------------------------------------------------------
// `dump bytes --json --pretty` indents the same object the compact form emits.
// The two forms must decode to identical content; only the whitespace differs.
// ---------------------------------------------------------------------------

func TestBytesDump_PrettyJSONMatchesCompact(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	compact, _, ec := runCLI(t, bin, "dump", "bytes", "--json", pdfPath)
	if ec != 0 {
		t.Fatalf("`dump bytes --json` expected exit 0, got %d", ec)
	}
	pretty, _, ep := runCLI(t, bin, "dump", "bytes", "--json", "--pretty", pdfPath)
	if ep != 0 {
		t.Fatalf("`dump bytes --json --pretty` expected exit 0, got %d", ep)
	}

	if strings.Count(strings.TrimRight(compact, "\n"), "\n") != 0 {
		t.Errorf("`dump bytes --json` output is not single-line compact:\n%.200s", compact)
	}
	if !strings.Contains(pretty, "\n  ") {
		t.Errorf("`dump bytes --json --pretty` output is not indented multi-line:\n%.200s", pretty)
	}

	var a, b any
	mustParseJSON(t, compact, &a)
	mustParseJSON(t, pretty, &b)
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Errorf("--pretty and compact decode to different content")
	}
}

// ---------------------------------------------------------------------------
// Runtime-error stderr shape (exit 2): one JSON object carrying an `error` key
// and nothing else.
// ---------------------------------------------------------------------------

func TestBytesDump_NonexistentFile_StderrShapes(t *testing.T) {
	bin := buildCLI(t)
	missing := "/nonexistent/path/fake.pdf"

	stdout, stderr, ec := runCLI(t, bin, "dump", "bytes", missing)
	if ec != 2 {
		t.Errorf("expected exit 2, got %d", ec)
	}
	if stdout != "" {
		t.Errorf("stdout should be empty on a runtime error, got: %q", stdout)
	}
	var errObj map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &errObj); err != nil {
		t.Fatalf("stderr is not a single JSON object: %v\nraw: %s", err, stderr)
	}
	if _, ok := errObj["error"]; !ok {
		t.Errorf("stderr JSON missing 'error' key: %s", stderr)
	}
}

// ---------------------------------------------------------------------------
// Usage-error stderr shape (exit 1): a bare usage line, plain text, NOT JSON.
// The non-parse half is the assertion that matters: the package doc states
// usage errors are plain text, so a change that starts JSON-wrapping them has
// to update the package doc too.
// ---------------------------------------------------------------------------

func TestBytesDump_MissingFileArgument_UsageNotJSON(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, ec := runCLI(t, bin, "dump", "bytes")
	if ec != 1 {
		t.Errorf("`dump bytes` with no file expected exit 1 (usage), got %d", ec)
	}
	if stdout != "" {
		t.Errorf("usage failure should leave stdout empty, got: %q", stdout)
	}
	if got := strings.TrimSpace(stderr); got != bytesUsageLine {
		t.Errorf("stderr is not the bare usage line\n got: %q\nwant: %q", got, bytesUsageLine)
	}
	if json.Valid([]byte(strings.TrimSpace(stderr))) {
		t.Errorf("the usage-error line is plain text, not JSON; stderr parsed as JSON:\n%s", stderr)
	}
}

// ---------------------------------------------------------------------------
// A flag written after <file> is a usage error, not a silently dropped
// argument. Go's flag package stops parsing at the first non-flag argument, so
// `dump bytes file.pdf --json` would otherwise leave f.json false and stream
// the raw document to a caller who asked for the JSON wrapper -- with exit 0,
// so nothing downstream could tell. Stdout must stay empty: a usage failure
// never emits a partial document.
// ---------------------------------------------------------------------------

func TestBytesDump_FlagAfterFile_UsageErrorNotSilentlyDropped(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, stderr, ec := runCLI(t, bin, "dump", "bytes", pdfPath, "--json")
	if ec != 1 {
		t.Errorf("a flag after <file> expected exit 1 (usage), got %d", ec)
	}
	if stdout != "" {
		t.Errorf("stdout must stay empty, got %d bytes of document", len(stdout))
	}
	if !strings.Contains(stderr, bytesUsageLine) {
		t.Errorf("stderr should carry the usage line, got: %q", stderr)
	}
}

// ---------------------------------------------------------------------------
// A second file path is a usage error too: the command dumps one document, so
// an extra positional is a typo, not an instruction to pick the first and
// ignore the rest.
// ---------------------------------------------------------------------------

func TestBytesDump_ExtraPositionalArgument_UsageError(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	stdout, stderr, ec := runCLI(t, bin, "dump", "bytes", pdfPath, pdfPath)
	if ec != 1 {
		t.Errorf("a second positional expected exit 1 (usage), got %d", ec)
	}
	if stdout != "" {
		t.Errorf("stdout must stay empty, got %d bytes of document", len(stdout))
	}
	if !strings.Contains(stderr, bytesUsageLine) {
		t.Errorf("stderr should carry the usage line, got: %q", stderr)
	}
}

// ---------------------------------------------------------------------------
// Help text: `dump bytes` is listed, its description says the output is raw
// document bytes rather than extracted page text and names pdftotext as the
// tool for page prose, and the lowercase token `plaintext` appears nowhere, so
// the help never names a command the dispatcher does not accept.
//
// The token check is case-sensitive and matches the nine characters exactly.
// It must not be relaxed to a case-insensitive or whitespace-normalised search:
// the help says "plain text" as two words on purpose and that copy stays.
//
// --help is read as stdout + stderr combined, because printUsage writes to
// stderr and main exits 0.
// ---------------------------------------------------------------------------

func TestHelp_ListsBytesWithoutPlaintextToken(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, ec := runCLI(t, bin, "--help")
	if ec != 0 {
		t.Fatalf("--help expected exit 0, got %d", ec)
	}
	help := stdout + stderr

	if !strings.Contains(help, "dump bytes") {
		t.Errorf("help text missing the `dump bytes` command listing")
	}
	if !strings.Contains(help, "pdftotext") {
		t.Errorf("the `dump bytes` help line should name pdftotext as the tool for page prose")
	}
	if strings.Contains(help, "plaintext") {
		var offending []string
		for _, line := range strings.Split(help, "\n") {
			if strings.Contains(line, "plaintext") {
				offending = append(offending, line)
			}
		}
		t.Errorf("help text still contains the literal lowercase token %q on %d line(s); `dump plaintext` is not a command:\n%s",
			"plaintext", len(offending), strings.Join(offending, "\n"))
	}
}

// ---------------------------------------------------------------------------
// `dump plaintext` is an unknown resource: the dispatcher prints
// "Unknown resource: plaintext" and the usage text, which lists `dump bytes`,
// then exits 1 with stdout empty. No handler runs, so a path that does not
// exist still exits 1 (usage) rather than 2 (runtime error from opening it).
// ---------------------------------------------------------------------------

func TestDump_PlaintextIsUnknownResource(t *testing.T) {
	bin := buildCLI(t)
	missing := "/nonexistent/path/fake.pdf"

	cases := []struct {
		name string
		args []string
	}{
		{"file", []string{"dump", "plaintext", missing}},
		{"json_file", []string{"dump", "plaintext", "--json", missing}},
		{"no_arguments", []string{"dump", "plaintext"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, ec := runCLI(t, bin, tc.args...)
			if ec != 1 {
				t.Errorf("expected exit 1 (unknown resource), got %d\nstderr: %s", ec, stderr)
			}
			if stdout != "" {
				t.Errorf("stdout should be empty, got: %q", stdout)
			}
			if first := stderrLines(stderr)[0]; first != "Unknown resource: plaintext" {
				t.Errorf("first stderr line\n got: %q\nwant: %q", first, "Unknown resource: plaintext")
			}
			if !strings.Contains(stderr, "dump bytes") {
				t.Errorf("stderr should carry the usage text listing `dump bytes`, got:\n%s", stderr)
			}
			if strings.Contains(stderr, "deprecated") {
				t.Errorf("stderr should carry no deprecation notice, got:\n%s", stderr)
			}
		})
	}
}
