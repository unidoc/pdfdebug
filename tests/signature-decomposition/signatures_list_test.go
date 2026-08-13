package signature_decomposition_test

// Story 13.4 -- signature enumeration + CLI surface.

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Fixture sanity: the programmatically signed fixture must parse through the
// EXISTING open path (dump objects). This test passes today and guards the
// suite against an eternally-red fixture.
// ---------------------------------------------------------------------------

func TestSignatures_FixtureParsesThroughOpenPath(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed.pdf", signedPDF(t, fixturePKI(t), sigOpt{}))

	_, stderr, ec := runCLI(t, bin, "dump", "objects", pdf)
	if ec != 0 {
		t.Fatalf("signed fixture rejected by the existing open path (exit %d): %s", ec, stderr)
	}
}

// ---------------------------------------------------------------------------
// plain-text default emits a per-signature block carrying the field name,
// SubFilter, and signer CN -- and is NOT JSON.
// ---------------------------------------------------------------------------

func TestSignatures_PlainBlockHasCoreFacts(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed.pdf", signedPDF(t, fixturePKI(t), sigOpt{}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	assertNotJSON(t, stdout)
	for _, want := range []string{"Sig1", "adbe.pkcs7.detached", signerCN, issuerCN} {
		if !strings.Contains(stdout, want) {
			t.Errorf("plain block missing %q:\n%s", want, stdout)
		}
	}
}

// ---------------------------------------------------------------------------
// --json emits a top-level array with one entry carrying fieldName, signed,
// signatureRef, node ids, and subFilter.
// ---------------------------------------------------------------------------

func TestSignatures_JSONEntryCoreShape(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed.pdf", signedPDF(t, fixturePKI(t), sigOpt{}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, stdout)
	if got := getStr(e, "fieldName"); got != "Sig1" {
		t.Errorf("fieldName = %q, want Sig1", got)
	}
	if !getBool(e, "signed") {
		t.Errorf("signed = false, want true")
	}
	if got := getStr(e, "signatureRef"); got != "5 0 R" {
		t.Errorf("signatureRef = %q, want \"5 0 R\"", got)
	}
	if got := getStr(e, "signatureNodeId"); got != "obj:0:5" {
		t.Errorf("signatureNodeId = %q, want obj:0:5", got)
	}
	if got := getStr(e, "fieldNodeId"); got != "obj:0:4" {
		t.Errorf("fieldNodeId = %q, want obj:0:4", got)
	}
	if got := getStr(e, "subFilter"); got != "adbe.pkcs7.detached" {
		t.Errorf("subFilter = %q, want adbe.pkcs7.detached", got)
	}
}

// ---------------------------------------------------------------------------
// An /FT /Sig field with NO /V is listed as signed:false with no
// decomposition and no error -- exit 0.
// ---------------------------------------------------------------------------

func TestSignatures_UnsignedPlaceholderListed(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "unsigned.pdf", unsignedPlaceholderPDF())

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, stdout)
	if getStr(e, "fieldName") != "EmptySig" {
		t.Errorf("fieldName = %q, want EmptySig", getStr(e, "fieldName"))
	}
	if getBool(e, "signed") {
		t.Errorf("signed = true for a /V-less placeholder, want false")
	}
	if s := getMap(e, "signer"); s != nil {
		t.Errorf("unsigned placeholder must carry no signer decomposition, got %v", s)
	}
	if de := getStr(e, "decomposeError"); de != "" {
		t.Errorf("unsigned placeholder must carry no error, got %q", de)
	}
}

// ---------------------------------------------------------------------------
// Zero signature fields -> plain "no signature fields" line and --json
// empty array, both exit 0.
// ---------------------------------------------------------------------------

func TestSignatures_ZeroSignaturesEmptyState(t *testing.T) {
	bin := buildCLI(t)
	pdf := testdataDir(t) + "/minimal.pdf"

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", pdf)
	if ec != 0 {
		t.Fatalf("plain expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	assertNotJSON(t, stdout)
	if !strings.Contains(strings.ToLower(stdout), "no signature fields") {
		t.Errorf("plain empty state must say \"no signature fields\":\n%s", stdout)
	}

	stdout, stderr, ec = runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("--json expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	arr := sigArray(t, stdout)
	if len(arr) != 0 {
		t.Errorf("expected empty array for zero signatures, got %d entries:\n%s", len(arr), stdout)
	}
}

// ---------------------------------------------------------------------------
// /Kids hierarchy with INHERITED /FT -- the terminal widget has no own /FT
// but inherits /Sig via /Parent; the fully qualified name joins the parent
// /T chain with ".".
// ---------------------------------------------------------------------------

func TestSignatures_KidsInheritedFTFullyQualifiedName(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed-kids.pdf", signedPDF(t, fixturePKI(t), sigOpt{kids: true}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, stdout)
	if got := getStr(e, "fieldName"); got != "Parent.Child1" {
		t.Errorf("fieldName = %q, want Parent.Child1 (inherited /FT, FQ name)", got)
	}
	if !getBool(e, "signed") {
		t.Errorf("signed = false, want true")
	}
}

// ---------------------------------------------------------------------------
// A DIRECT (inline dict) /V is legal and must still decompose --
// signatureRef empty, signer CN still surfaced.
// ---------------------------------------------------------------------------

func TestSignatures_DirectVDecomposesWithEmptyRef(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed-directv.pdf", signedPDF(t, fixturePKI(t), sigOpt{directV: true}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, stdout)
	if got := getStr(e, "signatureRef"); got != "" {
		t.Errorf("signatureRef = %q for a direct /V, want empty", got)
	}
	signer := getMap(e, "signer")
	if signer == nil || !strings.Contains(getStr(signer, "subject"), signerCN) {
		t.Errorf("direct /V must still decompose (signer subject with %q), got %v", signerCN, signer)
	}
}

// ---------------------------------------------------------------------------
// 13-1 contract: the plain-text default is ASCII-only and ends with a
// trailing newline.
// ---------------------------------------------------------------------------

func TestSignatures_PlainIsASCIIWithTrailingNewline(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed.pdf", signedPDF(t, fixturePKI(t), sigOpt{}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	assertASCII(t, stdout)
	assertTrailingNewline(t, stdout)
}

// ---------------------------------------------------------------------------
// The top-level usage/help text documents the new `dump signatures`
// resource so it is discoverable.
// ---------------------------------------------------------------------------

func TestSignatures_UsageDocumentsResource(t *testing.T) {
	bin := buildCLI(t)

	stdout, stderr, _ := runCLI(t, bin, "--help")
	combined := stdout + stderr
	if !strings.Contains(combined, "dump signatures") {
		t.Errorf("--help does not mention `dump signatures`:\n%s", combined)
	}
}

// ---------------------------------------------------------------------------
// An encrypted document degrades exactly like every other dump resource --
// stderr + exit 2, empty stdout, no crash.
// ---------------------------------------------------------------------------

func TestSignatures_EncryptedDocExitTwo(t *testing.T) {
	bin := buildCLI(t)
	pdf := testdataDir(t) + "/encrypted.pdf"

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", pdf)
	if ec != 2 {
		t.Fatalf("expected exit 2 for encrypted doc, got %d (stdout: %s / stderr: %s)", ec, stdout, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("stdout must stay empty on open failure, got:\n%s", stdout)
	}
	if !strings.Contains(strings.ToLower(stderr), "encrypt") {
		t.Errorf("stderr should mention encryption, got: %s", stderr)
	}
}

// ---------------------------------------------------------------------------
// /M surfaces BOTH the raw PDF date string and the parsed ISO 8601 form.
// ---------------------------------------------------------------------------

func TestSignatures_SigningTimeRawAndISO(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed.pdf", signedPDF(t, fixturePKI(t), sigOpt{}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, stdout)
	if got := getStr(e, "signingTimeRaw"); got != "D:20260101120000+00'00'" {
		t.Errorf("signingTimeRaw = %q, want the raw D: string", got)
	}
	if got := getStr(e, "signingTime"); !strings.HasPrefix(got, "2026-01-01T12:00:00") {
		t.Errorf("signingTime = %q, want ISO 8601 2026-01-01T12:00:00...", got)
	}
}
