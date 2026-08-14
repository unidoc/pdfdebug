package structural_diff_test

// The top-level `diff` command.

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Fixture sanity: every hand-assembled fixture must parse through the
// EXISTING open path (dump objects, exit 0). This test passes TODAY and
// guards the suite against an eternally-red fixture (13-4/13-5).
// ---------------------------------------------------------------------------

func TestDiff_FixturesParseThroughOpenPath(t *testing.T) {
	bin := buildCLI(t)
	cases := map[string][]byte{
		"one-page.pdf":        onePagePDF(),
		"one-renumbered.pdf":  onePageRenumberedPDF(),
		"two-page.pdf":        twoPagePDF(),
		"changed-mediabox.pdf": changedMediaBoxPDF(),
	}
	for name, content := range cases {
		pdf := writeTempPDF(t, name, content)
		_, stderr, ec := runCLI(t, bin, "dump", "objects", pdf)
		if ec != 0 {
			t.Fatalf("fixture %q rejected by the existing open path (exit %d): %s", name, ec, stderr)
		}
	}
}

// ---------------------------------------------------------------------------
// Two IDENTICAL files exit 0 (structurally identical).
// ---------------------------------------------------------------------------

func TestDiff_IdenticalFilesExitZero(t *testing.T) {
	bin := buildCLI(t)
	pdf := onePagePDF()
	a := writeTempPDF(t, "a.pdf", pdf)
	b := writeTempPDF(t, "b.pdf", pdf)

	stdout, stderr, ec := runCLI(t, bin, "diff", a, b)
	if ec != 0 {
		t.Fatalf("identical files must exit 0, got %d\nstdout: %s\nstderr: %s", ec, stdout, stderr)
	}
	if strings.Contains(strings.ToLower(stderr), "unknown command") {
		t.Errorf("`diff` must be a real command, not an unknown-command error: %s", stderr)
	}
}

// ---------------------------------------------------------------------------
// Two DIFFERING files exit 1 (the scriptable signal), distinct from the
// operational error code 2.
// ---------------------------------------------------------------------------

func TestDiff_DifferingFilesExitOne(t *testing.T) {
	bin := buildCLI(t)
	a := writeTempPDF(t, "one.pdf", onePagePDF())
	b := writeTempPDF(t, "two.pdf", twoPagePDF())

	stdout, stderr, ec := runCLI(t, bin, "diff", a, b)
	if ec != 1 {
		t.Fatalf("differing files must exit 1, got %d\nstdout: %s\nstderr: %s", ec, stdout, stderr)
	}
	// Guard against the current binary's unknown-command exit 1: a real run
	// writes the delta to stdout and never reports an unknown command.
	if strings.TrimSpace(stdout) == "" {
		t.Errorf("exit 1 must be a real diff run (non-empty stdout), got empty; stderr: %s", stderr)
	}
	if strings.Contains(strings.ToLower(stderr), "unknown command") {
		t.Errorf("`diff` must be a real command, not an unknown-command error: %s", stderr)
	}
}

// ---------------------------------------------------------------------------
// An unparseable (broken) second file is an OPERATIONAL error -> exit 2,
// distinct from the "differ" signal (exit 1).
// ---------------------------------------------------------------------------

func TestDiff_BrokenFileExitsTwo(t *testing.T) {
	bin := buildCLI(t)
	good := writeTempPDF(t, "good.pdf", onePagePDF())
	broken := writeTempPDF(t, "broken.pdf", []byte("this is not a pdf at all"))

	_, _, ec := runCLI(t, bin, "diff", good, broken)
	if ec != 2 {
		t.Fatalf("a broken input file must exit 2 (operational), got %d", ec)
	}
}

// ---------------------------------------------------------------------------
// A nonexistent second file is operational -> 2.
// ---------------------------------------------------------------------------

func TestDiff_MissingFileExitsTwo(t *testing.T) {
	bin := buildCLI(t)
	good := writeTempPDF(t, "good.pdf", onePagePDF())

	_, _, ec := runCLI(t, bin, "diff", good, "/no/such/file.pdf")
	if ec != 2 {
		t.Fatalf("a nonexistent input file must exit 2 (operational), got %d", ec)
	}
}

// ---------------------------------------------------------------------------
// `diff` takes TWO positional args. A single arg is a usage error -> exit 2
// (NOT a partial run, NOT a diff-vs-nothing).
// ---------------------------------------------------------------------------

func TestDiff_SingleArgIsUsageError(t *testing.T) {
	bin := buildCLI(t)
	only := writeTempPDF(t, "only.pdf", onePagePDF())

	_, _, ec := runCLI(t, bin, "diff", only)
	if ec != 2 {
		t.Fatalf("`diff` with one positional arg must exit 2 (usage), got %d", ec)
	}
}

// ---------------------------------------------------------------------------
// A THIRD positional arg is rejected -> exit 2.
// ---------------------------------------------------------------------------

func TestDiff_ThirdArgRejected(t *testing.T) {
	bin := buildCLI(t)
	a := writeTempPDF(t, "a.pdf", onePagePDF())
	b := writeTempPDF(t, "b.pdf", twoPagePDF())
	c := writeTempPDF(t, "c.pdf", onePagePDF())

	_, _, ec := runCLI(t, bin, "diff", a, b, c)
	if ec != 2 {
		t.Fatalf("a third positional arg must be rejected (exit 2), got %d", ec)
	}
}

// ---------------------------------------------------------------------------
// --json emits the {summary, root} envelope; the summary counts are numeric
// and the root is a DiffNode with a status.
// ---------------------------------------------------------------------------

func TestDiff_JSONEnvelope(t *testing.T) {
	bin := buildCLI(t)
	a := writeTempPDF(t, "one.pdf", onePagePDF())
	b := writeTempPDF(t, "two.pdf", twoPagePDF())

	stdout, stderr, ec := runCLI(t, bin, "diff", "--json", a, b)
	if ec != 1 {
		t.Fatalf("differing files --json must exit 1, got %d (stderr: %s)", ec, stderr)
	}
	res := diffResult(t, stdout)
	added, removed, changed := summaryOf(t, res)
	if added+removed+changed == 0 {
		t.Errorf("differing files reported a zero-delta summary (+%d -%d ~%d)", added, removed, changed)
	}
	root, ok := res["root"].(map[string]any)
	if !ok {
		t.Fatalf("result has no \"root\" DiffNode object: %v", res)
	}
	if getStr(root, "status") == "" {
		t.Errorf("root DiffNode has no \"status\"")
	}
	if _, hasPath := root["path"]; !hasPath {
		t.Errorf("root DiffNode has no \"path\"")
	}
}

// ---------------------------------------------------------------------------
// + 13-1 contract: the plain-text default is NOT JSON, is ASCII-only, and
// ends with a trailing newline.
// ---------------------------------------------------------------------------

func TestDiff_PlainTextContract(t *testing.T) {
	bin := buildCLI(t)
	a := writeTempPDF(t, "one.pdf", onePagePDF())
	b := writeTempPDF(t, "two.pdf", twoPagePDF())

	stdout, _, ec := runCLI(t, bin, "diff", a, b)
	if ec != 1 {
		t.Fatalf("expected exit 1, got %d", ec)
	}
	assertNotJSON(t, stdout)
	assertASCII(t, stdout)
	assertTrailingNewline(t, stdout)
}

// ---------------------------------------------------------------------------
// The plain-text delta uses +/-/~ markers for added/removed/changed
// paths.
// ---------------------------------------------------------------------------

func TestDiff_PlainTextDeltaMarkers(t *testing.T) {
	bin := buildCLI(t)
	a := writeTempPDF(t, "one.pdf", onePagePDF())
	b := writeTempPDF(t, "meta.pdf", twoPagePDF())

	stdout, _, ec := runCLI(t, bin, "diff", a, b)
	if ec != 1 {
		t.Fatalf("expected exit 1, got %d", ec)
	}
	// A change between one-page and two-page yields at least an added and/or a
	// changed path line. Require at least one of the delta markers to appear.
	if !strings.ContainsAny(stdout, "+-~") {
		t.Errorf("plain-text delta lacks any +/-/~ marker:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// --pretty indents the JSON (multi-line); the default --json is compact
// (single line).
// ---------------------------------------------------------------------------

func TestDiff_PrettyIndentsJSON(t *testing.T) {
	bin := buildCLI(t)
	a := writeTempPDF(t, "one.pdf", onePagePDF())
	b := writeTempPDF(t, "two.pdf", twoPagePDF())

	compact, _, ec := runCLI(t, bin, "diff", "--json", a, b)
	if ec != 1 {
		t.Fatalf("expected exit 1 for compact --json, got %d", ec)
	}
	pretty, _, ec2 := runCLI(t, bin, "diff", "--json", "--pretty", a, b)
	if ec2 != 1 {
		t.Fatalf("expected exit 1 for --pretty, got %d", ec2)
	}
	if !parsesAsJSON(compact) || !parsesAsJSON(pretty) {
		t.Fatalf("both outputs must be valid JSON")
	}
	compactLines := strings.Count(strings.TrimSpace(compact), "\n")
	prettyLines := strings.Count(strings.TrimSpace(pretty), "\n")
	if compactLines != 0 {
		t.Errorf("default --json should be single-line, saw %d newlines", compactLines)
	}
	if prettyLines <= compactLines {
		t.Errorf("--pretty should be multi-line (more newlines than compact); compact=%d pretty=%d", compactLines, prettyLines)
	}
}

// ---------------------------------------------------------------------------
// --full includes unchanged paths; the default omits them (so --full
// output is strictly larger).
// ---------------------------------------------------------------------------

func TestDiff_FullIncludesUnchanged(t *testing.T) {
	bin := buildCLI(t)
	a := writeTempPDF(t, "one.pdf", onePagePDF())
	b := writeTempPDF(t, "changed.pdf", changedMediaBoxPDF())

	def, _, ec := runCLI(t, bin, "diff", a, b)
	if ec != 1 {
		t.Fatalf("expected exit 1, got %d", ec)
	}
	full, _, ec2 := runCLI(t, bin, "diff", "--full", a, b)
	if ec2 != 1 {
		t.Fatalf("expected exit 1 with --full, got %d", ec2)
	}
	if len(full) <= len(def) {
		t.Errorf("--full (%d bytes) should include unchanged paths and exceed the default (%d bytes)", len(full), len(def))
	}
}

// ---------------------------------------------------------------------------
// The CLI-level ALIGNMENT GUARDRAIL. A renumbered-but- structurally-identical
// pair is reported as identical -> exit 0 and a zero-delta --json summary.
// Proves path-alignment beats object-number alignment end-to-end.
// ---------------------------------------------------------------------------

func TestDiff_RenumberedIdenticalIsZeroDelta(t *testing.T) {
	bin := buildCLI(t)
	a := writeTempPDF(t, "natural.pdf", onePagePDF())
	b := writeTempPDF(t, "renumbered.pdf", onePageRenumberedPDF())

	stdout, stderr, ec := runCLI(t, bin, "diff", "--json", a, b)
	if ec != 0 {
		t.Fatalf("renumbered-but-identical pair must exit 0 (identical), got %d\nstdout: %s\nstderr: %s", ec, stdout, stderr)
	}
	res := diffResult(t, stdout)
	added, removed, changed := summaryOf(t, res)
	if added != 0 || removed != 0 || changed != 0 {
		t.Errorf("path-alignment failed -- renumbered-but-identical shows delta +%d -%d ~%d (want 0/0/0)", added, removed, changed)
	}
}

// ---------------------------------------------------------------------------
// The summary surfaces the page-count change in the --json envelope
// (pageCountLeft/pageCountRight).
// ---------------------------------------------------------------------------

func TestDiff_SummaryPageCount(t *testing.T) {
	bin := buildCLI(t)
	a := writeTempPDF(t, "one.pdf", onePagePDF())
	b := writeTempPDF(t, "two.pdf", twoPagePDF())

	stdout, _, ec := runCLI(t, bin, "diff", "--json", a, b)
	if ec != 1 {
		t.Fatalf("expected exit 1, got %d", ec)
	}
	res := diffResult(t, stdout)
	sum, ok := res["summary"].(map[string]any)
	if !ok {
		t.Fatalf("no summary object")
	}
	if jsonInt(sum["pageCountLeft"]) != 1 {
		t.Errorf("summary.pageCountLeft = %d, want 1", jsonInt(sum["pageCountLeft"]))
	}
	if jsonInt(sum["pageCountRight"]) != 2 {
		t.Errorf("summary.pageCountRight = %d, want 2", jsonInt(sum["pageCountRight"]))
	}
}

// ---------------------------------------------------------------------------
// `pdfdebug --help` documents the `diff` command and its three-way 0/1/2
// exit contract.
// ---------------------------------------------------------------------------

func TestDiff_HelpDocumentsCommandAndExitCodes(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, _ := runCLI(t, bin, "--help")
	help := stdout + stderr
	lower := strings.ToLower(help)
	if !strings.Contains(lower, "diff") {
		t.Errorf("--help does not mention the `diff` command:\n%s", help)
	}
	// The exit-code contract (0 identical / 1 differ / 2 operational) must be
	// documented somewhere in the diff help block.
	if !strings.Contains(lower, "identical") || !strings.Contains(lower, "differ") {
		t.Errorf("--help must document the diff 0/1/2 exit contract (identical/differ):\n%s", help)
	}
}

// ---------------------------------------------------------------------------
// A difference that lives OFF the catalog walk (trailer /Info only) still
// exits 1 (differ), NOT 0. The node counts are 0/0/0 because /Info is not
// catalog-reachable, so exit 0 here would be a silent false "identical" for
// scripts. Regression guard for diffIsIdentical folding the document-level
// flags into the exit decision (the review-fixed bug). Plain text must name
// the /Info change; --json summary must set infoChanged with zero node
// counts.
// ---------------------------------------------------------------------------

func TestDiff_InfoOnlyChangeExitsOne(t *testing.T) {
	bin := buildCLI(t)
	a := writeTempPDF(t, "prodA.pdf", infoProducerPDF("AlphaLib"))
	b := writeTempPDF(t, "prodB.pdf", infoProducerPDF("BetaXLib"))

	stdout, stderr, ec := runCLI(t, bin, "diff", a, b)
	if ec != 1 {
		t.Fatalf("an /Info-only difference must exit 1 (differ), got %d\nstdout: %s\nstderr: %s", ec, stdout, stderr)
	}
	if !strings.Contains(strings.ToLower(stdout), "/info") {
		t.Errorf("plain text must name the /Info change:\n%s", stdout)
	}

	jout, jerr, jec := runCLI(t, bin, "diff", "--json", a, b)
	if jec != 1 {
		t.Fatalf("--json /Info-only diff must exit 1, got %d\nstderr: %s", jec, jerr)
	}
	res := diffResult(t, jout)
	added, removed, changed := summaryOf(t, res)
	if added != 0 || removed != 0 || changed != 0 {
		t.Errorf("Info lives off the catalog walk, node counts must be 0/0/0, got +%d -%d ~%d", added, removed, changed)
	}
	sum, ok := res["summary"].(map[string]any)
	if !ok {
		t.Fatalf("no summary object")
	}
	if ic, _ := sum["infoChanged"].(bool); !ic {
		t.Errorf("summary.infoChanged = false, want true")
	}
}

// ---------------------------------------------------------------------------
// Diffing a file against ITSELF (same path twice) is a valid all-unchanged run
// -> exit 0, zero-delta summary.
// ---------------------------------------------------------------------------

func TestDiff_SelfPathIsIdentical(t *testing.T) {
	bin := buildCLI(t)
	p := writeTempPDF(t, "self.pdf", onePagePDF())

	stdout, stderr, ec := runCLI(t, bin, "diff", "--json", p, p)
	if ec != 0 {
		t.Fatalf("file-vs-itself must exit 0, got %d\nstderr: %s", ec, stderr)
	}
	res := diffResult(t, stdout)
	added, removed, changed := summaryOf(t, res)
	if added != 0 || removed != 0 || changed != 0 {
		t.Errorf("self-diff summary not zero: +%d -%d ~%d", added, removed, changed)
	}
}
