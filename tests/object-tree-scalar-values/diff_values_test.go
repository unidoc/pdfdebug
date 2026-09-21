package object_tree_scalar_values_test

import (
	"strings"
	"testing"
)

// The diff displays decoded text, and the decode stays out of its equality
// decision: routing the status comparison through the decoder would make
// byte-different strings compare equal and report unchanged.

func TestDiff_SummariesShowDecodedText(t *testing.T) {
	result := diffJSON(t,
		fixturePath(t, "text-change-a.pdf"),
		fixturePath(t, "text-change-b.pdf"),
	)
	node := diffNodeAt(t, result, "/Root/StructTreeRoot/Alt")

	if node.Status != "changed" {
		t.Errorf("/Alt status = %q, want %q", node.Status, "changed")
	}
	if node.LeftSummary != altText {
		t.Errorf("left summary = %q, want the decoded %q", node.LeftSummary, altText)
	}
	if node.RightSummary != altTextChanged {
		t.Errorf("right summary = %q, want the decoded %q", node.RightSummary, altTextChanged)
	}
}

func TestDiff_PlainOutputShowsDecodedText(t *testing.T) {
	stdout, stderr, code := runCLI(t, "diff",
		fixturePath(t, "text-change-a.pdf"),
		fixturePath(t, "text-change-b.pdf"),
	)
	if code > 1 {
		t.Fatalf("diff exited %d: %s", code, stderr)
	}

	want := "~ /Root/StructTreeRoot/Alt  " + altText + " -> " + altTextChanged
	if !hasLine(stdout, want) {
		t.Errorf("diff has no row %q\n--- output ---\n%s", want, stdout)
	}
	if strings.Contains(stdout, altHex) {
		t.Errorf("diff still prints the undecoded hex\n--- output ---\n%s", stdout)
	}
}

func TestDiff_ByteDifferentStringsThatDecodeAlikeStayChanged(t *testing.T) {
	result := diffJSON(t,
		fixturePath(t, "decode-collision.pdf"),
		fixturePath(t, "decode-twin.pdf"),
	)

	// <FEFF0041> against (A): different bytes, both decode to "A".
	alt := diffNodeAt(t, result, "/Root/StructTreeRoot/Alt")
	if alt.Status != "changed" {
		t.Errorf("/Alt status = %q, want %q: the bytes differ even though both decode to %q",
			alt.Status, "changed", "A")
	}
	if alt.LeftSummary != "A" || alt.RightSummary != "A" {
		t.Errorf("/Alt summaries = %q / %q, want the decoded %q on both sides",
			alt.LeftSummary, alt.RightSummary, "A")
	}

	// <415C42> against <4142>: the upstream escape quirk makes both decode to "AB".
	quirk := diffNodeAt(t, result, "/Root/StructTreeRoot/Quirk")
	if quirk.Status != "changed" {
		t.Errorf("/Quirk status = %q, want %q: the bytes differ even though both decode to %q",
			quirk.Status, "changed", backslashText)
	}
	if quirk.LeftSummary != backslashText || quirk.RightSummary != backslashText {
		t.Errorf("/Quirk summaries = %q / %q, want the decoded %q on both sides",
			quirk.LeftSummary, quirk.RightSummary, backslashText)
	}
}

func TestDiff_ExitCodeStillSignalsTheDifference(t *testing.T) {
	_, _, code := runCLI(t, "diff",
		fixturePath(t, "decode-collision.pdf"),
		fixturePath(t, "decode-twin.pdf"),
	)
	if code != 1 {
		t.Errorf("diff exited %d, want 1: a decode collision must not hide a real difference", code)
	}
}
