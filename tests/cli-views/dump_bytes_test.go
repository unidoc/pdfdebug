// `dump bytes` and its deprecated `dump plaintext` alias.
//
// The command that streams raw document bytes is spelled `dump bytes`. The old
// spelling keeps working as an undocumented alias that prints a one-line
// deprecation notice to stderr, so piped stdout is unaffected.
//
// Test level: Integration (Go) -- CLI binary build + execution. No browser.
//
// Covers: --pretty parity on the JSON wrapper; alias stdout/exit-code parity
// with the canonical spelling; stdout cleanliness under the alias; notice
// placement and exact text; the notice firing on a usage failure; the per-path
// stderr shapes (runtime error is JSON, usage error is a bare usage line); and
// the help text naming `dump bytes` and pdftotext while carrying no occurrence
// of the literal token the rename removes.
//
// Run: cd tests/cli-views && go test -v -count=1 ./...
package cli_views_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// deprecationNotice is the exact line the alias writes to stderr. It is pinned
// by equality rather than substring so a notice naming the wrong replacement or
// the wrong removal version cannot pass.
const deprecationNotice = `pdfdebug: "dump plaintext" is deprecated and will be removed in 0.6.0; use "dump bytes".`

// bytesUsageLine is what parseDocViewFlags prints on a usage failure. Both
// spellings print it, because the alias passes the canonical resource label.
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
// The alias produces byte-identical stdout and the same exit code as the
// canonical spelling, on the raw path and the --json path. Everything the
// alias adds goes to stderr.
//
// Fixture choice for the raw path: image-xobject.pdf carries DCTDecode bytes
// >= 0x80, which is what surfaces a re-encoding regression.
// ---------------------------------------------------------------------------

func TestBytesAlias_StdoutIdenticalToCanonical(t *testing.T) {
	bin := buildCLI(t)

	t.Run("raw", func(t *testing.T) {
		pdfPath := filepath.Join(testdataDir(t), "image-xobject.pdf")

		canonical, _, ecCanonical := runCLIRaw(t, bin, "dump", "bytes", pdfPath)
		alias, _, ecAlias := runCLIRaw(t, bin, "dump", "plaintext", pdfPath)

		if ecAlias != ecCanonical {
			t.Errorf("alias exit code %d != canonical exit code %d", ecAlias, ecCanonical)
		}
		if !bytes.Equal(alias, canonical) {
			t.Errorf("alias raw stdout differs from canonical raw stdout (got %d bytes, want %d bytes)",
				len(alias), len(canonical))
		}

		want, err := os.ReadFile(pdfPath)
		if err != nil {
			t.Fatalf("read source fixture: %v", err)
		}
		if !bytes.Equal(alias, want) {
			t.Errorf("alias raw stdout is not the verbatim source bytes (got %d bytes, want %d bytes)",
				len(alias), len(want))
		}
	})

	t.Run("json", func(t *testing.T) {
		pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

		canonical, _, ecCanonical := runCLI(t, bin, "dump", "bytes", "--json", pdfPath)
		alias, _, ecAlias := runCLI(t, bin, "dump", "plaintext", "--json", pdfPath)

		if ecAlias != ecCanonical {
			t.Errorf("alias exit code %d != canonical exit code %d", ecAlias, ecCanonical)
		}
		if alias != canonical {
			t.Errorf("alias --json stdout differs from canonical --json stdout\nalias:     %.200s\ncanonical: %.200s",
				alias, canonical)
		}
	})
}

// ---------------------------------------------------------------------------
// No part of the deprecation notice reaches stdout under the alias. The raw
// path is a machine format people pipe, so a notice leaking into stdout would
// corrupt it. Asserted on its own, separately from the stderr placement check.
// ---------------------------------------------------------------------------

func TestBytesAlias_StdoutCarriesNoNotice(t *testing.T) {
	bin := buildCLI(t)

	fragments := []string{deprecationNotice, `pdfdebug: "dump plaintext"`, "deprecated", "0.6.0"}

	t.Run("raw", func(t *testing.T) {
		pdfPath := filepath.Join(testdataDir(t), "image-xobject.pdf")
		stdout, _, ec := runCLIRaw(t, bin, "dump", "plaintext", pdfPath)
		if ec != 0 {
			t.Fatalf("alias raw run expected exit 0, got %d", ec)
		}
		for _, fragment := range fragments {
			if bytes.Contains(stdout, []byte(fragment)) {
				t.Errorf("alias raw stdout contains notice fragment %q; the notice belongs on stderr only", fragment)
			}
		}
	})

	t.Run("json", func(t *testing.T) {
		pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")
		stdout, _, ec := runCLI(t, bin, "dump", "plaintext", "--json", pdfPath)
		if ec != 0 {
			t.Fatalf("alias --json run expected exit 0, got %d", ec)
		}
		for _, fragment := range fragments {
			if strings.Contains(stdout, fragment) {
				t.Errorf("alias --json stdout contains notice fragment %q; the notice belongs on stderr only", fragment)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// On a successful alias run stderr is exactly the notice: one line, equal to
// the pinned string, and nothing else. Equality rather than a substring check,
// so a notice naming the wrong replacement or removal version fails here.
// ---------------------------------------------------------------------------

func TestBytesAlias_NoticeIsFirstAndOnlyLineOnStderr(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	_, aliasErr, ecAlias := runCLI(t, bin, "dump", "plaintext", pdfPath)
	_, canonicalErr, ecCanonical := runCLI(t, bin, "dump", "bytes", pdfPath)

	if ecAlias != ecCanonical {
		t.Errorf("alias exit code %d != canonical exit code %d on the same arguments", ecAlias, ecCanonical)
	}
	if canonicalErr != "" {
		t.Errorf("canonical spelling wrote to stderr on a clean run: %q", canonicalErr)
	}

	lines := stderrLines(aliasErr)
	if lines[0] != deprecationNotice {
		t.Errorf("first stderr line is not the deprecation notice\n got: %q\nwant: %q", lines[0], deprecationNotice)
	}
	if len(lines) != 1 {
		t.Errorf("alias stderr on a clean run should be the notice and nothing else, got %d lines:\n%s",
			len(lines), aliasErr)
	}

	var noticeCount int
	for _, line := range lines {
		if strings.Contains(line, "deprecated") {
			noticeCount++
		}
	}
	if noticeCount != 1 {
		t.Errorf("expected exactly one deprecation line on stderr, got %d:\n%s", noticeCount, aliasErr)
	}
}

// ---------------------------------------------------------------------------
// The notice is written in the dispatch arm, before flags are parsed, so it
// fires on a usage failure too. The usage line printed alongside it names the
// live spelling: the alias hands parseDocViewFlags the canonical resource
// label, so the pairing reads as an instruction, not a contradiction.
// ---------------------------------------------------------------------------

func TestBytesAlias_NoticeFiresOnUsageError(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, ec := runCLI(t, bin, "dump", "plaintext")
	if ec != 1 {
		t.Errorf("alias with no file argument expected exit 1 (usage), got %d", ec)
	}
	if stdout != "" {
		t.Errorf("usage failure should leave stdout empty, got: %q", stdout)
	}

	lines := stderrLines(stderr)
	if lines[0] != deprecationNotice {
		t.Errorf("first stderr line is not the deprecation notice\n got: %q\nwant: %q", lines[0], deprecationNotice)
	}
	if len(lines) < 2 {
		t.Fatalf("expected the notice followed by the usage line, got %d line(s):\n%s", len(lines), stderr)
	}
	if lines[1] != bytesUsageLine {
		t.Errorf("second stderr line is not the usage line naming the live spelling\n got: %q\nwant: %q",
			lines[1], bytesUsageLine)
	}
	if strings.Contains(lines[1], "plaintext") {
		t.Errorf("the alias usage line must point forward to the live spelling, got: %q", lines[1])
	}
}

// ---------------------------------------------------------------------------
// Runtime-error stderr shape (exit 2), both spellings. The canonical spelling
// writes one JSON object carrying an `error` key and nothing else. Under the
// alias the notice line precedes it, so stderr taken whole is not JSON and the
// object has to be read from the remainder.
// ---------------------------------------------------------------------------

func TestBytesDump_NonexistentFile_StderrShapes(t *testing.T) {
	bin := buildCLI(t)
	missing := "/nonexistent/path/fake.pdf"

	t.Run("canonical", func(t *testing.T) {
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
	})

	t.Run("alias", func(t *testing.T) {
		stdout, stderr, ec := runCLI(t, bin, "dump", "plaintext", missing)
		if ec != 2 {
			t.Errorf("expected exit 2, got %d", ec)
		}
		if stdout != "" {
			t.Errorf("stdout should be empty on a runtime error, got: %q", stdout)
		}

		lines := stderrLines(stderr)
		if lines[0] != deprecationNotice {
			t.Errorf("first stderr line is not the deprecation notice\n got: %q\nwant: %q", lines[0], deprecationNotice)
		}
		if json.Valid([]byte(strings.TrimSpace(stderr))) {
			t.Errorf("stderr under the alias carries the notice first, so it must not parse whole as JSON:\n%s", stderr)
		}

		remainder := strings.TrimSpace(strings.Join(lines[1:], "\n"))
		var errObj map[string]string
		if err := json.Unmarshal([]byte(remainder), &errObj); err != nil {
			t.Fatalf("stderr after the notice is not a single JSON object: %v\nraw: %s", err, remainder)
		}
		if _, ok := errObj["error"]; !ok {
			t.Errorf("stderr JSON missing 'error' key: %s", remainder)
		}
	})
}

// ---------------------------------------------------------------------------
// Usage-error stderr shape (exit 1): a bare usage line, plain text, NOT JSON.
// The non-parse half is the assertion that matters -- it is the half the old
// blanket "errors are always JSON on stderr" claim got wrong, so a change that
// starts JSON-wrapping usage errors has to come back and update the package
// doc too.
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
// Help text: `dump bytes` is listed, its description says the output is raw
// document bytes rather than extracted page text and names pdftotext as the
// tool for page prose, and the literal lowercase token the rename removes
// appears nowhere.
//
// The token check is case-sensitive and matches the nine characters exactly.
// It must not be relaxed to a case-insensitive or whitespace-normalised search:
// the help says "plain text" as two words on purpose and that copy stays.
//
// --help is read as stdout + stderr combined, because printUsage writes to
// stderr and main exits 0.
// ---------------------------------------------------------------------------

func TestHelp_BytesReplacesPlaintext(t *testing.T) {
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
		t.Errorf("help text still contains the literal lowercase token %q on %d line(s); the alias is undocumented-but-working:\n%s",
			"plaintext", len(offending), strings.Join(offending, "\n"))
	}
}
