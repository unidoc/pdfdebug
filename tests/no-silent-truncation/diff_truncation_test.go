package no_silent_truncation_test

// Story 14.3 #2 -- the diff depth-cap quiet lie (AC1, AC2).
//
// RED PHASE: the depth-32 diff cap compares any subtree below it by SHALLOW
// SUMMARY only. deep-change-{a,b}.pdf differ by one scalar (/V 111 vs /V 222)
// nested ~45 catalog-levels deep; the cap bites at catalog-depth 33 (the
// `/Root/Deep/L/.../L` ref node) and its one-level summary `<< /L <ref> >>` is
// identical on both sides, so the differing leaf is never reached. Today the
// run reports "Documents are structurally identical." at exit 0 -- an INVERTED
// answer for a script keying on the exit code. These tests encode the GREEN
// target and MUST FAIL today.

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// 14.3-INTG-000 [P0] fixture sanity: both deep-change fixtures parse through
// the EXISTING open path (dump objects, exit 0). Passes TODAY; guards the suite
// against an eternally-red fixture.
// ---------------------------------------------------------------------------

func TestDeepChange_FixturesParseThroughOpenPath(t *testing.T) {
	bin := buildCLI(t)
	for _, name := range []string{"deep-change-a.pdf", "deep-change-b.pdf"} {
		_, stderr, ec := runCLI(t, bin, "dump", "objects", fixturePath(t, name))
		if ec != 0 {
			t.Fatalf("fixture %q rejected by the existing open path (exit %d): %s", name, ec, stderr)
		}
	}
}

// ---------------------------------------------------------------------------
// 14.3-INTG-001 [P1] AC2: a depth-cap-bounded comparison must NOT claim
// "identical". The plain-text run must withhold the identical banner and exit 1
// (not a false 0), and must state that a subtree was compared only to the depth
// cap. RED today: prints "Documents are structurally identical.", exit 0.
// ---------------------------------------------------------------------------

func TestDiff_DepthCappedNotIdentical_PlainText(t *testing.T) {
	bin := buildCLI(t)
	a := fixturePath(t, "deep-change-a.pdf")
	b := fixturePath(t, "deep-change-b.pdf")

	stdout, stderr, ec := runCLI(t, bin, "diff", a, b)
	if ec != 1 {
		t.Fatalf("a depth-capped comparison must exit 1 (not provably identical), got %d\nstdout: %s\nstderr: %s", ec, stdout, stderr)
	}
	low := strings.ToLower(stdout)
	if strings.Contains(low, "structurally identical") {
		t.Errorf("a truncated comparison must NOT claim \"structurally identical\":\n%s", stdout)
	}
	// The document-level depth-cap note (and/or the per-node [truncated: depth
	// cap] tag) must be visible so no consumer mistakes the bounded walk for a
	// complete one.
	if !strings.Contains(low, "truncat") && !strings.Contains(low, "depth cap") {
		t.Errorf("plain-text output must state the walk was bounded by the depth cap (want \"truncat\"/\"depth cap\"):\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// 14.3-INTG-001 [P1] AC1/AC2: `diff --json` on the same pair must exit 1, count
// the depth-capped subtree in summary.truncatedSubtrees (> 0), and mark the cut
// node with "truncated": true. RED today: exit 0, no such field.
// ---------------------------------------------------------------------------

func TestDiff_DepthCappedNotIdentical_JSON(t *testing.T) {
	bin := buildCLI(t)
	a := fixturePath(t, "deep-change-a.pdf")
	b := fixturePath(t, "deep-change-b.pdf")

	stdout, stderr, ec := runCLI(t, bin, "diff", "--json", a, b)
	if ec != 1 {
		t.Fatalf("`diff --json` on a depth-capped pair must exit 1, got %d\nstderr: %s", ec, stderr)
	}
	res := parseObject(t, "14.3-INTG-001", stdout)

	sum, ok := res["summary"].(map[string]any)
	if !ok {
		t.Fatalf("result has no \"summary\" object: %v", res)
	}
	if _, present := sum["truncatedSubtrees"]; !present {
		t.Fatalf("summary is missing the additive \"truncatedSubtrees\" count: %v", sum)
	}
	if n := jsonInt(sum["truncatedSubtrees"]); n < 1 {
		t.Errorf("summary.truncatedSubtrees = %d, want >= 1 (the deep chain is cut once)", n)
	}

	root, ok := res["root"].(map[string]any)
	if !ok {
		t.Fatalf("result has no \"root\" DiffNode object")
	}
	if !anyNodeTruncated(root) {
		t.Errorf("no DiffNode carries \"truncated\": true; the depth-cap cut node must be marked")
	}
}
