package signature_decomposition_test

// Story 13.4 -- ByteRange coverage facts (AC 3, 7).
// RED PHASE: fails at runtime until `dump signatures` lands.
//
// Coverage is a MEASUREMENT, never a validity verdict: the JSON carries
// coversWholeFile / trailingGap / holeMatchesContents / coverageError.

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// 13.4-INTG-006 [P0] AC3: the well-formed fixture's /ByteRange covers the
// whole file except the /Contents hole -> coversWholeFile true, hole matches.
// ---------------------------------------------------------------------------

func TestSignatures_CoverageCoversWholeFile(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed.pdf", signedPDF(t, fixturePKI(t), sigOpt{}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, "13.4-INTG-006", stdout)
	if !getBool(e, "coversWholeFile") {
		t.Errorf("coversWholeFile = false, want true:\n%s", stdout)
	}
	if !getBool(e, "holeMatchesContents") {
		t.Errorf("holeMatchesContents = false, want true (hole == /Contents extent)")
	}
	if ce := getStr(e, "coverageError"); ce != "" {
		t.Errorf("coverageError = %q, want empty", ce)
	}
	br, _ := e["byteRange"].([]any)
	if len(br) != 4 {
		t.Errorf("byteRange has %d elements, want the raw 4", len(br))
	}
}

// ---------------------------------------------------------------------------
// 13.4-INTG-007 [P0] AC3: a signature whose range stops 100 bytes short of
// EOF (the earlier-revision case) reports coversWholeFile false plus the
// trailing gap -- and the plain text states the coverage fact without
// implying breakage.
// ---------------------------------------------------------------------------

func TestSignatures_CoverageTrailingGapFact(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed-notcovers.pdf", signedPDF(t, fixturePKI(t), sigOpt{shortfall: 100}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, "13.4-INTG-007", stdout)
	if getBool(e, "coversWholeFile") {
		t.Errorf("coversWholeFile = true, want false (100-byte shortfall)")
	}
	if gap, _ := e["trailingGap"].(float64); gap != 100 {
		t.Errorf("trailingGap = %v, want 100", e["trailingGap"])
	}
	if !getBool(e, "holeMatchesContents") {
		t.Errorf("holeMatchesContents = false, want true (only the tail is short)")
	}

	plain, _, ec := runCLI(t, bin, "dump", "signatures", pdf)
	if ec != 0 {
		t.Fatalf("plain expected exit 0, got %d", ec)
	}
	if !strings.Contains(strings.ToLower(plain), "not cover") {
		t.Errorf("plain output must state the not-covered fact:\n%s", plain)
	}
}

// ---------------------------------------------------------------------------
// 13.4-INTG-008 [P1] AC3: a /ByteRange hole that does NOT exactly equal the
// /Contents hex-string extent is a distinct structural fact.
// ---------------------------------------------------------------------------

func TestSignatures_CoverageHoleMismatchFact(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed-holemismatch.pdf", signedPDF(t, fixturePKI(t), sigOpt{holeShift: 4}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, "13.4-INTG-008", stdout)
	if getBool(e, "holeMatchesContents") {
		t.Errorf("holeMatchesContents = true, want false (hole shifted +4 bytes)")
	}
}

// ---------------------------------------------------------------------------
// 13.4-INTG-009 [P0] AC3/AC7: a malformed /ByteRange (odd-length array)
// degrades to a per-signature coverageError -- listed, exit 0, never a crash.
// ---------------------------------------------------------------------------

func TestSignatures_MalformedByteRangeDegrades(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "malformed-br.pdf", malformedByteRangePDF())

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0 (per-signature degradation), got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, "13.4-INTG-009", stdout)
	if getStr(e, "fieldName") != "BadBR" {
		t.Errorf("field must still be listed, fieldName = %q", getStr(e, "fieldName"))
	}
	if ce := getStr(e, "coverageError"); ce == "" {
		t.Errorf("coverageError empty, want the malformed-/ByteRange fact:\n%s", stdout)
	}
}
