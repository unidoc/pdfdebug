// Expose existing pdfcore views as CLI commands.
//
// Usage/help discoverability + compact/pretty parity (cross-cutting), covering
// printUsage's subcommands and examples. Black-box: build the CLI, run as a
// subprocess.
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
// `pdfdebug --help` lists every new subcommand in the Commands block with a
// one-line description; ref-taking commands show the --ref "N G R" form.
// ---------------------------------------------------------------------------

func TestHelp_ListsNewSubcommands(t *testing.T) {
	bin := buildCLI(t)

	// --help exits 0 and (per main.go) writes usage to stderr.
	stdout, stderr, exitCode := runCLI(t, bin, "--help")
	if exitCode != 0 {
		t.Fatalf("--help expected exit 0, got %d", exitCode)
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
			t.Errorf("help text missing command listing for %q", cmd)
		}
	}

	// Ref-taking commands should advertise the --ref "N G R" form near them.
	if !strings.Contains(help, `--ref "N G R"`) {
		t.Errorf("help text should show the --ref \"N G R\" form for ref-taking commands")
	}
}

// ---------------------------------------------------------------------------
// The Examples block shows working invocations for at least `dump
// reverserefs`, `dump xref`, and one flag-bearing case (`dump image
// --metadata` OR `dump plaintext --json`) so the non-default flags are
// discoverable.
// ---------------------------------------------------------------------------

func TestHelp_ExamplesCoverNewCommands(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, exitCode := runCLI(t, bin, "--help")
	if exitCode != 0 {
		t.Fatalf("--help expected exit 0, got %d", exitCode)
	}
	help := stdout + stderr

	exStart := strings.Index(help, "Examples:")
	if exStart < 0 {
		t.Fatalf("help text has no Examples block")
	}
	examples := help[exStart:]

	if !strings.Contains(examples, "dump reverserefs") {
		t.Errorf("Examples block missing a `dump reverserefs` invocation")
	}
	if !strings.Contains(examples, "dump xref") {
		t.Errorf("Examples block missing a `dump xref` invocation")
	}
	flagExample := strings.Contains(examples, "dump image --metadata") ||
		strings.Contains(examples, "dump plaintext --json")
	if !flagExample {
		t.Errorf("Examples block should show a flag-bearing case (dump image --metadata or dump plaintext --json)")
	}
}

// ---------------------------------------------------------------------------
// An unknown dump resource still exits 1 and the new commands appear in the
// printed usage, so a typo surfaces the full menu.
// ---------------------------------------------------------------------------

func TestUnknownResource_ShowsUsageWithNewCommands(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	_, stderr, exitCode := runCLI(t, bin, "dump", "bogusresource", pdfPath)
	if exitCode != 1 {
		t.Errorf("unknown resource expected exit 1, got %d", exitCode)
	}
	if !strings.Contains(stderr, "dump reverserefs") {
		t.Errorf("unknown-resource usage should list the new commands (e.g. dump reverserefs)")
	}
}

// ---------------------------------------------------------------------------
// cross-cutting: JSON commands default to compact single-line and honor
// --pretty (the shared emit helper). Verified on a document-level command
// (dump objects) to keep the parity contract uniform.
// ---------------------------------------------------------------------------

func TestNewCommands_PrettyVsCompact(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "minimal.pdf")

	compact, _, ec := runCLI(t, bin, "dump", "objects", "--json", pdfPath)
	if ec != 0 {
		t.Fatalf("compact run exit %d", ec)
	}
	pretty, _, ep := runCLI(t, bin, "dump", "objects", "--json", "--pretty", pdfPath)
	if ep != 0 {
		t.Fatalf("--pretty run exit %d", ep)
	}

	if strings.Count(strings.TrimRight(compact, "\n"), "\n") != 0 {
		t.Errorf("default `dump objects` output is not single-line compact:\n%.200s", compact)
	}
	if !strings.Contains(pretty, "\n  ") {
		t.Errorf("`dump objects --pretty` output is not indented multi-line:\n%.200s", pretty)
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
