package signature_decomposition_test

// Story 13.4 -- PKCS#7/CMS decomposition.

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// The signer cert is IDENTIFIED from SignerInfo.IssuerAndSerialNumber -- the
// fixture places the CA cert FIRST in the (unordered) certificate set, so
// positional certificates[0] picking would surface the wrong subject.
// ---------------------------------------------------------------------------

func TestSignatures_SignerIdentifiedNotPositional(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed.pdf", signedPDF(t, fixturePKI(t), sigOpt{}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, stdout)
	if !getBool(e, "signerIdentified") {
		t.Errorf("signerIdentified = false, want true (issuer+serial match exists)")
	}
	signer := getMap(e, "signer")
	if signer == nil {
		t.Fatalf("no signer object in entry:\n%s", stdout)
	}
	if got := getStr(signer, "subject"); !strings.Contains(got, signerCN) {
		t.Errorf("signer.subject = %q, want the LEAF cert CN %q (certificates[0] is the CA)", got, signerCN)
	}
	if got := getStr(signer, "issuer"); !strings.Contains(got, issuerCN) {
		t.Errorf("signer.issuer = %q, want CN %q", got, issuerCN)
	}
	if got := getStr(signer, "serial"); got != "2026" {
		t.Errorf("signer.serial = %q, want 2026", got)
	}
	// Validity window: the fixture cert is valid 2025-01-01 .. 2027-01-01 UTC.
	if got := getStr(signer, "notBefore"); !strings.Contains(got, "2025-01-01") {
		t.Errorf("signer.notBefore = %q, want 2025-01-01", got)
	}
	if got := getStr(signer, "notAfter"); !strings.Contains(got, "2027-01-01") {
		t.Errorf("signer.notAfter = %q, want 2027-01-01", got)
	}
}

// ---------------------------------------------------------------------------
// Digest + signature algorithms are read from SignerInfo (sha256 /
// rsaEncryption in the fixture), not guessed.
// ---------------------------------------------------------------------------

func TestSignatures_AlgorithmsFromSignerInfo(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed.pdf", signedPDF(t, fixturePKI(t), sigOpt{}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, stdout)
	dig := strings.ToLower(getStr(e, "digestAlgorithm"))
	if !strings.Contains(dig, "sha") || !strings.Contains(dig, "256") {
		t.Errorf("digestAlgorithm = %q, want a SHA-256 identification", getStr(e, "digestAlgorithm"))
	}
	sig := strings.ToLower(getStr(e, "signatureAlgorithm"))
	if !strings.Contains(sig, "rsa") {
		t.Errorf("signatureAlgorithm = %q, want an RSA identification", getStr(e, "signatureAlgorithm"))
	}
}

// ---------------------------------------------------------------------------
// The FULL embedded certificate set is surfaced (2 certs in the
// fixture: CA + leaf), subject/issuer per cert.
// ---------------------------------------------------------------------------

func TestSignatures_FullCertificateSetSurfaced(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed.pdf", signedPDF(t, fixturePKI(t), sigOpt{}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, stdout)
	certsAny, _ := e["certificates"].([]any)
	if len(certsAny) != 2 {
		t.Fatalf("expected 2 embedded certificates, got %d:\n%s", len(certsAny), stdout)
	}
	joined, _ := json.Marshal(certsAny)
	for _, cn := range []string{signerCN, issuerCN} {
		if !strings.Contains(string(joined), cn) {
			t.Errorf("certificate set missing subject CN %q:\n%s", cn, joined)
		}
	}
}

// ---------------------------------------------------------------------------
// A corrupted (non-DER) /Contents yields a per-signature
// decomposeError -- the field is still listed, the whole document
// never fails, exit stays 0.
// ---------------------------------------------------------------------------

func TestSignatures_CorruptContentsPerSignatureError(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed-corrupt.pdf", signedPDF(t, fixturePKI(t), sigOpt{corrupt: true}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0 (per-signature degradation), got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, stdout)
	if getStr(e, "fieldName") != "Sig1" {
		t.Errorf("field must still be listed, fieldName = %q", getStr(e, "fieldName"))
	}
	if !getBool(e, "signed") {
		t.Errorf("signed = false, want true (a /V exists, only its blob is bad)")
	}
	if de := getStr(e, "decomposeError"); de == "" {
		t.Errorf("decomposeError empty, want the CMS parse failure surfaced:\n%s", stdout)
	}
}

// ---------------------------------------------------------------------------
// adbe.x509.rsa_sha1 (/Contents is PKCS#1, NOT CMS) decomposes the cert
// chain from the field's /Cert entry instead and labels the entry
// accordingly.
// ---------------------------------------------------------------------------

func TestSignatures_X509RSASHA1CertsFromCertEntry(t *testing.T) {
	bin := buildCLI(t)
	p := fixturePKI(t)
	pdf := writeTempPDF(t, "signed-x509.pdf", signedPDF(t, p, sigOpt{
		subFilter: "adbe.x509.rsa_sha1", certEntry: true, rawContents: rawRSAContents(t, p)}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, stdout)
	entryJSON, _ := json.Marshal(e)
	if !strings.Contains(string(entryJSON), signerCN) {
		t.Errorf("cert from /Cert (CN %q) not surfaced:\n%s", signerCN, entryJSON)
	}
	if !strings.Contains(strings.ToLower(string(entryJSON)), "not cms") {
		t.Errorf("entry not labeled as non-CMS (want \"subFilter not CMS - certs read from /Cert\"):\n%s", entryJSON)
	}
}

// ---------------------------------------------------------------------------
// ETSI.RFC3161 doc timestamps parse the token as CMS for certs/algorithms and
// are labeled type: document-timestamp (/M absent is normal).
// ---------------------------------------------------------------------------

func TestSignatures_RFC3161LabeledDocumentTimestamp(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed-rfc3161.pdf", signedPDF(t, fixturePKI(t), sigOpt{
		subFilter: "ETSI.RFC3161", omitM: true}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, stdout)
	if got := getStr(e, "type"); got != "document-timestamp" {
		t.Errorf("type = %q, want document-timestamp", got)
	}
	signer := getMap(e, "signer")
	if signer == nil || !strings.Contains(getStr(signer, "subject"), signerCN) {
		t.Errorf("timestamp token must still decompose as CMS (signer subject), got %v", signer)
	}
}

// ---------------------------------------------------------------------------
// An unknown SubFilter degrades to a labeled "not decomposed" note --
// listed, no crash, exit 0.
// ---------------------------------------------------------------------------

func TestSignatures_UnknownSubFilterLabeledNotDecomposed(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed-unknown.pdf", signedPDF(t, fixturePKI(t), sigOpt{
		subFilter: "acme.custom.sig"}))

	stdout, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	e := oneSig(t, stdout)
	if got := getStr(e, "subFilter"); got != "acme.custom.sig" {
		t.Errorf("subFilter = %q, want acme.custom.sig", got)
	}
	entryJSON, _ := json.Marshal(e)
	if !strings.Contains(strings.ToLower(string(entryJSON)), "not decomposed") {
		t.Errorf("entry missing the \"subFilter not decomposed\" label:\n%s", entryJSON)
	}
}

// ---------------------------------------------------------------------------
// NO trust claims anywhere -- plain and JSON. The words
// valid/trusted/verified may appear only in negated/factual forms, and the
// "trust not verified" note must be present.
// ---------------------------------------------------------------------------

func TestSignatures_NoTrustClaimsAndTrustNote(t *testing.T) {
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "signed.pdf", signedPDF(t, fixturePKI(t), sigOpt{}))

	plain, stderr, ec := runCLI(t, bin, "dump", "signatures", pdf)
	if ec != 0 {
		t.Fatalf("plain expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	assertNoTrustClaims(t, "plain", plain)
	if !strings.Contains(strings.ToLower(plain), "trust not verified") {
		t.Errorf("plain output missing the \"trust not verified\" note:\n%s", plain)
	}

	jsonOut, stderr, ec := runCLI(t, bin, "dump", "signatures", "--json", pdf)
	if ec != 0 {
		t.Fatalf("--json expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	assertNoTrustClaims(t, "json", jsonOut)
}
