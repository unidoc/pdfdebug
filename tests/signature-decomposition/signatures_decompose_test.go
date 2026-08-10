package signature_decomposition_test

// Story 13.4 -- PKCS#7/CMS decomposition (AC 2, 4, 7).
// RED PHASE: fails at runtime until `dump signatures` lands.

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// 13.4-INTG-003 [P0] AC2: the signer cert is IDENTIFIED from
// SignerInfo.IssuerAndSerialNumber -- the fixture places the CA cert FIRST in
// the (unordered) certificate set, so positional certificates[0] picking
// would surface the wrong subject.
// ---------------------------------------------------------------------------

func TestSignatures_SignerIdentifiedNotPositional(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed.pdf", signedPDF(t, fixturePKI(t), sigOpt{}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("[P0] 13.4-INTG-003: expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, "13.4-INTG-003", stdout)
	if !getBool(e, "signerIdentified") {
		t.Errorf("[P0] 13.4-INTG-003: signerIdentified = false, want true (issuer+serial match exists)")
	}
	signer := getMap(e, "signer")
	if signer == nil {
		t.Fatalf("[P0] 13.4-INTG-003: no signer object in entry:\n%s", stdout)
	}
	if got := getStr(signer, "subject"); !strings.Contains(got, signerCN) {
		t.Errorf("[P0] 13.4-INTG-003: signer.subject = %q, want the LEAF cert CN %q (certificates[0] is the CA)", got, signerCN)
	}
	if got := getStr(signer, "issuer"); !strings.Contains(got, issuerCN) {
		t.Errorf("[P0] 13.4-INTG-003: signer.issuer = %q, want CN %q", got, issuerCN)
	}
	if got := getStr(signer, "serial"); got != "2026" {
		t.Errorf("[P0] 13.4-INTG-003: signer.serial = %q, want 2026", got)
	}
	// Validity window: the fixture cert is valid 2025-01-01 .. 2027-01-01 UTC.
	if got := getStr(signer, "notBefore"); !strings.Contains(got, "2025-01-01") {
		t.Errorf("[P0] 13.4-INTG-003: signer.notBefore = %q, want 2025-01-01", got)
	}
	if got := getStr(signer, "notAfter"); !strings.Contains(got, "2027-01-01") {
		t.Errorf("[P0] 13.4-INTG-003: signer.notAfter = %q, want 2027-01-01", got)
	}
}

// ---------------------------------------------------------------------------
// 13.4-INTG-004 [P0] AC2: digest + signature algorithms are read from
// SignerInfo (sha256 / rsaEncryption in the fixture), not guessed.
// ---------------------------------------------------------------------------

func TestSignatures_AlgorithmsFromSignerInfo(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed.pdf", signedPDF(t, fixturePKI(t), sigOpt{}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("[P0] 13.4-INTG-004: expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, "13.4-INTG-004", stdout)
	dig := strings.ToLower(getStr(e, "digestAlgorithm"))
	if !strings.Contains(dig, "sha") || !strings.Contains(dig, "256") {
		t.Errorf("[P0] 13.4-INTG-004: digestAlgorithm = %q, want a SHA-256 identification", getStr(e, "digestAlgorithm"))
	}
	sig := strings.ToLower(getStr(e, "signatureAlgorithm"))
	if !strings.Contains(sig, "rsa") {
		t.Errorf("[P0] 13.4-INTG-004: signatureAlgorithm = %q, want an RSA identification", getStr(e, "signatureAlgorithm"))
	}
}

// ---------------------------------------------------------------------------
// 13.4-INTG-005 [P1] AC2: the FULL embedded certificate set is surfaced
// (2 certs in the fixture: CA + leaf), subject/issuer per cert.
// ---------------------------------------------------------------------------

func TestSignatures_FullCertificateSetSurfaced(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed.pdf", signedPDF(t, fixturePKI(t), sigOpt{}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("[P1] 13.4-INTG-005: expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, "13.4-INTG-005", stdout)
	certsAny, _ := e["certificates"].([]any)
	if len(certsAny) != 2 {
		t.Fatalf("[P1] 13.4-INTG-005: expected 2 embedded certificates, got %d:\n%s", len(certsAny), stdout)
	}
	joined, _ := json.Marshal(certsAny)
	for _, cn := range []string{signerCN, issuerCN} {
		if !strings.Contains(string(joined), cn) {
			t.Errorf("[P1] 13.4-INTG-005: certificate set missing subject CN %q:\n%s", cn, joined)
		}
	}
}

// ---------------------------------------------------------------------------
// 13.4-INTG-010 [P0] AC2/AC7: a corrupted (non-DER) /Contents yields a
// per-signature decomposeError -- the field is still listed, the whole
// document never fails, exit stays 0.
// ---------------------------------------------------------------------------

func TestSignatures_CorruptContentsPerSignatureError(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed-corrupt.pdf", signedPDF(t, fixturePKI(t), sigOpt{corrupt: true}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("[P0] 13.4-INTG-010: expected exit 0 (per-signature degradation), got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, "13.4-INTG-010", stdout)
	if getStr(e, "fieldName") != "Sig1" {
		t.Errorf("[P0] 13.4-INTG-010: field must still be listed, fieldName = %q", getStr(e, "fieldName"))
	}
	if !getBool(e, "signed") {
		t.Errorf("[P0] 13.4-INTG-010: signed = false, want true (a /V exists, only its blob is bad)")
	}
	if de := getStr(e, "decomposeError"); de == "" {
		t.Errorf("[P0] 13.4-INTG-010: decomposeError empty, want the CMS parse failure surfaced:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// 13.4-INTG-015 [P1] AC2: adbe.x509.rsa_sha1 (/Contents is PKCS#1, NOT CMS)
// decomposes the cert chain from the field's /Cert entry instead and labels
// the entry accordingly.
// ---------------------------------------------------------------------------

func TestSignatures_X509RSASHA1CertsFromCertEntry(t *testing.T) {
	bin := buildCLI(t)
	p := fixturePKI(t)
	pdf := writeTempPDF(t, "signed-x509.pdf", signedPDF(t, p, sigOpt{
		subFilter: "adbe.x509.rsa_sha1", certEntry: true, rawContents: rawRSAContents(t, p)}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("[P1] 13.4-INTG-015: expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, "13.4-INTG-015", stdout)
	entryJSON, _ := json.Marshal(e)
	if !strings.Contains(string(entryJSON), signerCN) {
		t.Errorf("[P1] 13.4-INTG-015: cert from /Cert (CN %q) not surfaced:\n%s", signerCN, entryJSON)
	}
	if !strings.Contains(strings.ToLower(string(entryJSON)), "not cms") {
		t.Errorf("[P1] 13.4-INTG-015: entry not labeled as non-CMS (want \"subFilter not CMS - certs read from /Cert\"):\n%s", entryJSON)
	}
}

// ---------------------------------------------------------------------------
// 13.4-INTG-016 [P2] AC2: ETSI.RFC3161 doc timestamps parse the token as CMS
// for certs/algorithms and are labeled type: document-timestamp (/M absent is
// normal).
// ---------------------------------------------------------------------------

func TestSignatures_RFC3161LabeledDocumentTimestamp(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed-rfc3161.pdf", signedPDF(t, fixturePKI(t), sigOpt{
		subFilter: "ETSI.RFC3161", omitM: true}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("[P2] 13.4-INTG-016: expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, "13.4-INTG-016", stdout)
	if got := getStr(e, "type"); got != "document-timestamp" {
		t.Errorf("[P2] 13.4-INTG-016: type = %q, want document-timestamp", got)
	}
	signer := getMap(e, "signer")
	if signer == nil || !strings.Contains(getStr(signer, "subject"), signerCN) {
		t.Errorf("[P2] 13.4-INTG-016: timestamp token must still decompose as CMS (signer subject), got %v", signer)
	}
}

// ---------------------------------------------------------------------------
// 13.4-INTG-017 [P2] AC2/AC7: an unknown SubFilter degrades to a labeled
// "not decomposed" note -- listed, no crash, exit 0.
// ---------------------------------------------------------------------------

func TestSignatures_UnknownSubFilterLabeledNotDecomposed(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed-unknown.pdf", signedPDF(t, fixturePKI(t), sigOpt{
		subFilter: "acme.custom.sig"}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("[P2] 13.4-INTG-017: expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, "13.4-INTG-017", stdout)
	if got := getStr(e, "subFilter"); got != "acme.custom.sig" {
		t.Errorf("[P2] 13.4-INTG-017: subFilter = %q, want acme.custom.sig", got)
	}
	entryJSON, _ := json.Marshal(e)
	if !strings.Contains(strings.ToLower(string(entryJSON)), "not decomposed") {
		t.Errorf("[P2] 13.4-INTG-017: entry missing the \"subFilter not decomposed\" label:\n%s", entryJSON)
	}
}

// ---------------------------------------------------------------------------
// 13.4-INTG-018 [P0] AC4: NO trust claims anywhere -- plain and JSON. The
// words valid/trusted/verified may appear only in negated/factual forms, and
// the "trust not verified" note must be present.
// ---------------------------------------------------------------------------

func TestSignatures_NoTrustClaimsAndTrustNote(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed.pdf", signedPDF(t, fixturePKI(t), sigOpt{}))

	plain, stderr, ec := runCLI(t, bin, "dump", "signatures", pdf)
	if ec != 0 {
		t.Fatalf("[P0] 13.4-INTG-018: plain expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	assertNoTrustClaims(t, "13.4-INTG-018/plain", plain)
	if !strings.Contains(strings.ToLower(plain), "trust not verified") {
		t.Errorf("[P0] 13.4-INTG-018: plain output missing the \"trust not verified\" note:\n%s", plain)
	}

	jsonOut, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("[P0] 13.4-INTG-018: --json expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	assertNoTrustClaims(t, "13.4-INTG-018/json", jsonOut)
}
