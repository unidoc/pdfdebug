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

// A decode collision on a changed row would otherwise print the same text on
// both sides, so a changed node whose decoded summaries match falls back to the
// byte-exact renderings and says what actually differs.

func TestDiff_ByteDifferentStringsThatDecodeAlikeShowTheirStoredForms(t *testing.T) {
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
	if alt.LeftSummary != "<FEFF0041>" || alt.RightSummary != "(A)" {
		t.Errorf("/Alt summaries = %q / %q, want the byte-exact %q / %q",
			alt.LeftSummary, alt.RightSummary, "<FEFF0041>", "(A)")
	}

	// <415C42> against <4142>: the upstream escape quirk makes both decode to "AB".
	quirk := diffNodeAt(t, result, "/Root/StructTreeRoot/Quirk")
	if quirk.Status != "changed" {
		t.Errorf("/Quirk status = %q, want %q: the bytes differ even though both decode to %q",
			quirk.Status, "changed", backslashText)
	}
	if quirk.LeftSummary != "<"+backslashHex+">" || quirk.RightSummary != "<4142>" {
		t.Errorf("/Quirk summaries = %q / %q, want the byte-exact %q / %q",
			quirk.LeftSummary, quirk.RightSummary, "<"+backslashHex+">", "<4142>")
	}
}

func TestDiff_PlainRowOnACollisionNamesBothStoredForms(t *testing.T) {
	stdout, stderr, code := runCLI(t, "diff",
		fixturePath(t, "decode-collision.pdf"),
		fixturePath(t, "decode-twin.pdf"),
	)
	if code > 1 {
		t.Fatalf("diff exited %d: %s", code, stderr)
	}

	want := "~ /Root/StructTreeRoot/Alt  <FEFF0041> -> (A)"
	if !hasLine(stdout, want) {
		t.Errorf("diff has no row %q\n--- output ---\n%s", want, stdout)
	}
	if hasLine(stdout, "~ /Root/StructTreeRoot/Alt  A -> A") {
		t.Errorf("diff still prints the same text on both sides of a changed row\n--- output ---\n%s", stdout)
	}
}

// The binary carve-out holds on the diff surface: a changed signature must not
// put its DER on the row, which is where the tree and detail views show the
// fixed-width stand-in.

func TestDiff_CarvedOutBinaryStringsSummarizeAsTheStandIn(t *testing.T) {
	result := diffJSON(t,
		fixturePath(t, "sig-change-a.pdf"),
		fixturePath(t, "sig-change-b.pdf"),
	)

	for _, tc := range []struct {
		path string
		want string
	}{
		{"/Root/SigTyped/Contents", "<binary, 64 bytes>"},
		{"/Root/SigTyped/Cert[0]", "<binary, 16 bytes>"},
		{"/Root/Attachment/Params/CheckSum", "<binary, 16 bytes>"},
	} {
		node := diffNodeAt(t, result, tc.path)
		if node.Status != "changed" {
			t.Errorf("%s status = %q, want changed", tc.path, node.Status)
		}
		if node.LeftSummary != tc.want || node.RightSummary != tc.want {
			t.Errorf("%s summaries = %q / %q, want %q on both sides",
				tc.path, node.LeftSummary, node.RightSummary, tc.want)
		}
	}
}

func TestDiff_PlainOutputNeverPrintsTheCarvedOutBlob(t *testing.T) {
	stdout, stderr, code := runCLI(t, "diff",
		fixturePath(t, "sig-change-a.pdf"),
		fixturePath(t, "sig-change-b.pdf"),
	)
	if code > 1 {
		t.Fatalf("diff exited %d: %s", code, stderr)
	}

	for _, blob := range []string{
		strings.Repeat("AB", sigContentsBytes),
		strings.Repeat("CD", sigContentsBytes),
		checkSumHex,
	} {
		if strings.Contains(stdout, blob) {
			t.Errorf("diff printed a carved-out blob on the row\n--- output ---\n%s", stdout)
		}
	}
	if !strings.Contains(stdout, "<binary, 64 bytes>") {
		t.Errorf("diff never printed the fixed-width stand-in\n--- output ---\n%s", stdout)
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

// A signature dictionary reached twice is cut by the diff's cross-path dedup
// and compared as a whole resolved dictionary. The two sides then summarize
// alike - both stand-ins - which is exactly the collision shape that falls back
// to the byte-exact rendering, so the carve-out has to survive the fallback.

func TestDiff_SignatureReachedTwiceKeepsTheStandInOnTheCutRow(t *testing.T) {
	result := diffJSON(t,
		fixturePath(t, "shared-sig-a.pdf"),
		fixturePath(t, "shared-sig-b.pdf"),
	)

	cut := diffNodeAt(t, result, "/Root/AcroForm/Fields[0]/V")
	if cut.Status != "changed" {
		t.Errorf("cut row status = %q, want changed", cut.Status)
	}
	for _, side := range []struct {
		name    string
		summary string
	}{{"left", cut.LeftSummary}, {"right", cut.RightSummary}} {
		if !strings.Contains(side.summary, "<binary, 64 bytes>") {
			t.Errorf("%s summary = %q, want the fixed-width stand-in", side.name, side.summary)
		}
	}
	for _, blob := range []string{
		strings.Repeat("AB", sigContentsBytes),
		strings.Repeat("CD", sigContentsBytes),
	} {
		if strings.Contains(cut.LeftSummary, blob) || strings.Contains(cut.RightSummary, blob) {
			t.Errorf("cut row carries the blob: %q / %q", cut.LeftSummary, cut.RightSummary)
		}
	}
}

func TestDiff_PlainOutputNeverPrintsTheBlobOfASignatureReachedTwice(t *testing.T) {
	stdout, stderr, code := runCLI(t, "diff", "--full",
		fixturePath(t, "shared-sig-a.pdf"),
		fixturePath(t, "shared-sig-b.pdf"),
	)
	if code > 1 {
		t.Fatalf("diff exited %d: %s", code, stderr)
	}

	for _, blob := range []string{
		strings.Repeat("AB", sigContentsBytes),
		strings.Repeat("CD", sigContentsBytes),
	} {
		if strings.Contains(stdout, blob) {
			t.Errorf("diff printed a carved-out blob on the row\n--- output ---\n%s", stdout)
		}
	}
	if !strings.Contains(stdout, "<binary, 64 bytes>") {
		t.Errorf("diff never printed the fixed-width stand-in\n--- output ---\n%s", stdout)
	}
}
