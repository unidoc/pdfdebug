// Story 11.4: Expose existing pdfcore views as CLI commands -- RED PHASE.
//
// Usage/help discoverability (AC7) + compact/pretty parity (cross-cutting).
// These MUST FAIL until Story 11-4 extends printUsage with the new subcommands
// and examples. Black-box: build the CLI, run as a subprocess.
//
// Test level: Integration (Go) for help text; Unit-ish for the pretty/compact
// JSON contract. No browser.
//
// Run: cd tests/cli-views && go test -v -count=1 ./...
package cli_views_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// 11.4-INTG-015 [P1] (AC7): `pdfdebug --help` lists every new subcommand in
// the Commands block with a one-line description; ref-taking commands show the
// --ref "N G R" form.
// ---------------------------------------------------------------------------

func TestHelp_ListsNewSubcommands(t *testing.T) {
	bin := buildCLI(t)

	// --help exits 0 and (per main.go) writes usage to stderr.
	stdout, stderr, exitCode := runCLI(t, bin, "--help")
	if exitCode != 0 {
		t.Fatalf("[P1] 11.4-INTG-015: --help expected exit 0, got %d", exitCode)
	}
	help := stdout + stderr

	// Every new resource must appear as a `dump <resource>` command line.
	newCommands := []string{
		"dump font",
		"dump image",
		"dump source",
		"dump reverserefs",
		"dump xref",
		"dump objects",
		"dump plaintext",
	}
	for _, cmd := range newCommands {
		if !strings.Contains(help, cmd) {
			t.Errorf("[P1] 11.4-INTG-015: help text missing command listing for %q", cmd)
		}
	}

	// Ref-taking commands should advertise the --ref "N G R" form near them.
	if !strings.Contains(help, `--ref "N G R"`) {
		t.Errorf("[P1] 11.4-INTG-015: help text should show the --ref \"N G R\" form for ref-taking commands")
	}
}

// ---------------------------------------------------------------------------
// 11.4-INTG-016 [P1] (AC7): the Examples block shows working invocations for
// at least `dump reverserefs`, `dump xref`, and one flag-bearing case
// (`dump image --metadata` OR `dump plaintext --json`) so the non-default
// flags are discoverable.
// ---------------------------------------------------------------------------

func TestHelp_ExamplesCoverNewCommands(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "--help")
	if exitCode != 0 {
		t.Fatalf("[P1] 11.4-INTG-016: --help expected exit 0, got %d", exitCode)
	}
	help := stdout + stderr

	exStart := strings.Index(help, "Examples:")
	if exStart < 0 {
		t.Fatalf("[P1] 11.4-INTG-016: help text has no Examples block")
	}
	examples := help[exStart:]

	if !strings.Contains(examples, "dump reverserefs") {
		t.Errorf("[P1] 11.4-INTG-016: Examples block missing a `dump reverserefs` invocation")
	}
	if !strings.Contains(examples, "dump xref") {
		t.Errorf("[P1] 11.4-INTG-016: Examples block missing a `dump xref` invocation")
	}
	flagExample := strings.Contains(examples, "dump image --metadata") ||
		strings.Contains(examples, "dump plaintext --json")
	if !flagExample {
		t.Errorf("[P1] 11.4-INTG-016: Examples block should show a flag-bearing case (dump image --metadata or dump plaintext --json)")
	}
}

// ---------------------------------------------------------------------------
// 11.4-INTG-017 [P2] (AC7): an unknown dump resource still exits 1 and the
// new commands appear in the printed usage, so a typo surfaces the full menu.
// ---------------------------------------------------------------------------

func TestUnknownResource_ShowsUsageWithNewCommands(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	_, stderr, exitCode := runCLI(t, bin, "dump", "bogusresource", pdfPath)
	if exitCode != 1 {
		t.Errorf("[P2] 11.4-INTG-017: unknown resource expected exit 1, got %d", exitCode)
	}
	if !strings.Contains(stderr, "dump reverserefs") {
		t.Errorf("[P2] 11.4-INTG-017: unknown-resource usage should list the new commands (e.g. dump reverserefs)")
	}
}

// ---------------------------------------------------------------------------
// 11.4-INTG-018 [P2] (cross-cutting): JSON commands default to compact
// single-line and honor --pretty (the 11-3 emit helper). Verified on a
// document-level command (dump objects) to keep the parity contract uniform.
// ---------------------------------------------------------------------------

func TestNewCommands_PrettyVsCompact(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	compact, _, ec := runCLI(t, bin, "dump", "objects", "--json", pdfPath)
	if ec != 0 {
		t.Fatalf("[P2] 11.4-INTG-018: compact run exit %d", ec)
	}
	pretty, _, ep := runCLI(t, bin, "dump", "objects", "--json", "--pretty", pdfPath)
	if ep != 0 {
		t.Fatalf("[P2] 11.4-INTG-018: --pretty run exit %d", ep)
	}

	if strings.Count(strings.TrimRight(compact, "\n"), "\n") != 0 {
		t.Errorf("[P2] 11.4-INTG-018: default `dump objects` output is not single-line compact:\n%.200s", compact)
	}
	if !strings.Contains(pretty, "\n  ") {
		t.Errorf("[P2] 11.4-INTG-018: `dump objects --pretty` output is not indented multi-line:\n%.200s", pretty)
	}

	var a, b any
	mustParseJSON(t, compact, &a)
	mustParseJSON(t, pretty, &b)
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Errorf("[P2] 11.4-INTG-018: --pretty and compact decode to different content")
	}
}
