// Unit tests for CMS decomposition of BER-encoded signatures and the
// per-signature CMSEncoding fact.
//
// Fixture tests open the committed signature fixtures in testdata/ through
// Inspector.Open. Unit-built PDFs use sigUnitPDF with the contentsHex option,
// which sizes the /Contents hole to the given payload.
package pdfcore

import (
	"bytes"
	"encoding/asn1"
	"encoding/hex"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// openSigFixture opens a committed testdata fixture and returns its single
// signature field.
func openSigFixture(t *testing.T, name string) SignatureField {
	t.Helper()
	ins := NewInspector()
	id := "sig-ber-" + name
	if _, err := ins.Open(id, filepath.Join(testdataDir(t), name)); err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	t.Cleanup(func() { _ = ins.Close(id) })
	list, err := ins.GetSignatures(id)
	return oneSigField(t, list, err)
}

// signedPDFContentInfo returns the DER ContentInfo from signed.pdf without
// its zero padding.
func signedPDFContentInfo(t *testing.T) []byte {
	t.Helper()
	var rv asn1.RawValue
	if _, err := asn1.Unmarshal(fixtureCMSPayload(t, "signed.pdf"), &rv); err != nil {
		t.Fatalf("signed.pdf payload: %v", err)
	}
	return rv.FullBytes
}

// assertCleanCMSFailure checks a signature whose CMS failed to parse: a
// "CMS parse failed: " error, no chain, no signer, no algorithms, no encoding,
// and the structural facts intact.
func assertCleanCMSFailure(t *testing.T, f SignatureField, fieldName string) {
	t.Helper()
	if !strings.HasPrefix(f.DecomposeError, "CMS parse failed: ") {
		t.Errorf("DecomposeError = %q, want prefix %q", f.DecomposeError, "CMS parse failed: ")
	}
	if len(f.Certificates) != 0 {
		t.Errorf("Certificates = %v, want none (no partial chain)", f.Certificates)
	}
	if f.Signer != nil || f.SignerIdentified {
		t.Errorf("Signer = %+v, SignerIdentified = %v, want nil/false", f.Signer, f.SignerIdentified)
	}
	if f.DigestAlgorithm != "" || f.SignatureAlgorithm != "" {
		t.Errorf("algorithms = %q / %q, want empty", f.DigestAlgorithm, f.SignatureAlgorithm)
	}
	if f.CMSEncoding != "" {
		t.Errorf("CMSEncoding = %q, want empty (payload did not decode to a complete element)", f.CMSEncoding)
	}
	if f.FieldName != fieldName || !f.Signed {
		t.Errorf("FieldName = %q, Signed = %v, want %q / true", f.FieldName, f.Signed, fieldName)
	}
	if len(f.ByteRange) != 4 {
		t.Errorf("ByteRange = %v, want 4 integers", f.ByteRange)
	}
	if !f.CoversWholeFile {
		t.Errorf("CoversWholeFile = false, want true")
	}
}

// fixtureCert builds the CertInfo the signature fixtures carry.
func fixtureCert(cn, serial string) CertInfo {
	return CertInfo{
		Subject:   "CN=" + cn + ",O=UniDoc Fixtures",
		Issuer:    "CN=Fixture Root CA,O=UniDoc Fixtures",
		Serial:    serial,
		NotBefore: "2025-01-01T00:00:00Z",
		NotAfter:  "2027-01-01T00:00:00Z",
	}
}

func TestGetSignatures_DERFixtureReportsDERAndIsOtherwiseUnchanged(t *testing.T) {
	got := openSigFixture(t, "signed.pdf")
	signer := fixtureCert("Fixture Signer", "2026")
	want := SignatureField{
		FieldName:           "Sig1",
		Signed:              true,
		SignatureRef:        "5 0 R",
		SignatureNodeID:     "obj:0:5",
		FieldNodeID:         "obj:0:4",
		SubFilter:           "adbe.pkcs7.detached",
		Type:                "signature",
		SigningTimeRaw:      "D:20260101120000+00'00'",
		SigningTime:         "2026-01-01T12:00:00Z",
		Name:                "Fixture Signer",
		Reason:              "Fixture",
		Location:            "Testville",
		DigestAlgorithm:     "SHA-256",
		SignatureAlgorithm:  "RSA",
		Signer:              &signer,
		SignerIdentified:    true,
		Certificates:        []CertInfo{fixtureCert("Fixture Root CA", "77001"), fixtureCert("Fixture Signer", "2026")},
		Notes:               []string{trustNote},
		ByteRange:           []int64{0, 501, 6647, 287},
		CoversWholeFile:     true,
		HoleMatchesContents: true,
		CMSEncoding:         "DER",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("signed.pdf decomposition\n got %+v\nwant %+v", got, want)
	}
}

func TestGetSignatures_BERDetachedFixtureDecomposes(t *testing.T) {
	f := openSigFixture(t, "signed-ber-detached.pdf")
	if f.DecomposeError != "" {
		t.Fatalf("DecomposeError = %q, want empty", f.DecomposeError)
	}
	if f.CMSEncoding != "BER" {
		t.Errorf("CMSEncoding = %q, want BER", f.CMSEncoding)
	}
	if f.SubFilter != "adbe.pkcs7.detached" {
		t.Errorf("SubFilter = %q, want adbe.pkcs7.detached", f.SubFilter)
	}
	if len(f.Certificates) != 2 ||
		f.Certificates[0].Subject != "CN=Fixture Root CA,O=UniDoc Fixtures" ||
		f.Certificates[1].Subject != "CN=Fixture Signer,O=UniDoc Fixtures" {
		t.Errorf("Certificates = %+v, want the root CA then the signer", f.Certificates)
	}
	if !f.SignerIdentified || f.Signer == nil || f.Signer.Subject != "CN=Fixture Signer,O=UniDoc Fixtures" {
		t.Errorf("Signer = %+v, SignerIdentified = %v, want the fixture signer", f.Signer, f.SignerIdentified)
	}
	if f.DigestAlgorithm != "SHA-256" || f.SignatureAlgorithm != "RSA" {
		t.Errorf("algorithms = %q / %q, want SHA-256 / RSA", f.DigestAlgorithm, f.SignatureAlgorithm)
	}
	if !f.HoleMatchesContents {
		t.Errorf("HoleMatchesContents = false, want true (hole check reads the PDF hex)")
	}
	if !f.CoversWholeFile {
		t.Errorf("CoversWholeFile = false, want true")
	}

	der := openSigFixture(t, "signed.pdf")
	if !reflect.DeepEqual(f.Certificates, der.Certificates) {
		t.Errorf("certificates differ from signed.pdf\n got %+v\nwant %+v", f.Certificates, der.Certificates)
	}
	if !reflect.DeepEqual(f.Signer, der.Signer) {
		t.Errorf("signer differs from signed.pdf\n got %+v\nwant %+v", f.Signer, der.Signer)
	}
	if f.DigestAlgorithm != der.DigestAlgorithm || f.SignatureAlgorithm != der.SignatureAlgorithm {
		t.Errorf("algorithms differ from signed.pdf: %q/%q vs %q/%q",
			f.DigestAlgorithm, f.SignatureAlgorithm, der.DigestAlgorithm, der.SignatureAlgorithm)
	}
}

func TestGetSignatures_OpenSSLStreamedBERFixtureDecomposes(t *testing.T) {
	f := openSigFixture(t, "signed-ber.pdf")
	if f.DecomposeError != "" {
		t.Fatalf("DecomposeError = %q, want empty", f.DecomposeError)
	}
	if f.CMSEncoding != "BER" {
		t.Errorf("CMSEncoding = %q, want BER", f.CMSEncoding)
	}
	if f.SubFilter != "adbe.pkcs7.sha1" {
		t.Errorf("SubFilter = %q, want adbe.pkcs7.sha1", f.SubFilter)
	}
	if len(f.Certificates) != 2 {
		t.Errorf("Certificates = %+v, want 2", f.Certificates)
	}
	if !f.SignerIdentified || f.Signer == nil || f.Signer.Subject != "CN=Fixture Signer,O=UniDoc Fixtures" {
		t.Errorf("Signer = %+v, SignerIdentified = %v, want the fixture signer", f.Signer, f.SignerIdentified)
	}
	if f.DigestAlgorithm != "SHA-1" {
		t.Errorf("DigestAlgorithm = %q, want SHA-1", f.DigestAlgorithm)
	}
	if f.SignatureAlgorithm != "RSA" {
		t.Errorf("SignatureAlgorithm = %q, want RSA", f.SignatureAlgorithm)
	}
	if !f.HoleMatchesContents {
		t.Errorf("HoleMatchesContents = false, want true")
	}
	if !f.CoversWholeFile {
		t.Errorf("CoversWholeFile = false, want true")
	}
}

func TestGetSignatures_UnsupportedTagPayloadReportsNoEncoding(t *testing.T) {
	f := openSigFixture(t, "signed-badcontents.pdf")
	if f.DecomposeError == "" {
		t.Errorf("DecomposeError empty, want the CMS parse failure")
	}
	if len(f.Certificates) != 0 {
		t.Errorf("Certificates = %v, want none", f.Certificates)
	}
	if f.CMSEncoding != "" {
		t.Errorf("CMSEncoding = %q, want empty", f.CMSEncoding)
	}
}

func TestGetSignatures_NonCMSSubFiltersReportNoEncoding(t *testing.T) {
	cases := []struct {
		name string
		opt  sigUnitOpt
	}{
		{"adbe.x509.rsa_sha1", sigUnitOpt{subFilter: "adbe.x509.rsa_sha1", certHex: sigUnitCertHex(t)}},
		{"unknown subfilter", sigUnitOpt{subFilter: "acme.custom.sig"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ins, tabID := writeTempPDF(t, "signed-noncms.pdf", sigUnitPDF(t, tc.opt))
			list, err := ins.GetSignatures(tabID)
			f := oneSigField(t, list, err)
			if f.CMSEncoding != "" {
				t.Errorf("CMSEncoding = %q, want empty for a non-CMS SubFilter", f.CMSEncoding)
			}
		})
	}
}

func TestGetSignatures_WellFormedNonSignedDataReportsEncoding(t *testing.T) {
	ins, tabID := writeTempPDF(t, "signed-notcms.pdf", sigUnitPDF(t, sigUnitOpt{contentsHex: "3003020101"}))
	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	if !strings.HasPrefix(f.DecomposeError, "CMS parse failed: ") {
		t.Errorf("DecomposeError = %q, want prefix %q", f.DecomposeError, "CMS parse failed: ")
	}
	if f.CMSEncoding != "DER" {
		t.Errorf("CMSEncoding = %q, want DER (the payload decoded to a complete element)", f.CMSEncoding)
	}
	if len(f.Certificates) != 0 {
		t.Errorf("Certificates = %v, want none", f.Certificates)
	}
}

func TestGetSignatures_MalformedEndOfContentsFixtureFailsCleanly(t *testing.T) {
	f := openSigFixture(t, "signed-ber-badeoc.pdf")
	assertCleanCMSFailure(t, f, "Sig1")
}

func TestGetSignatures_BERWithTrailingGarbageFailsCleanly(t *testing.T) {
	payload := append(berAllIndef(t, signedPDFContentInfo(t)), 0x01, 0x02)
	ins, tabID := writeTempPDF(t, "signed-ber-trailing.pdf",
		sigUnitPDF(t, sigUnitOpt{contentsHex: hex.EncodeToString(payload)}))
	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	assertCleanCMSFailure(t, f, "Sig1")
	if !strings.Contains(f.DecomposeError, "trailing data") {
		t.Errorf("DecomposeError = %q, want the trailing-data reason", f.DecomposeError)
	}
}

func TestGetSignatures_DERWithTrailingGarbageFailsCleanly(t *testing.T) {
	payload := append(bytes.Clone(signedPDFContentInfo(t)), 0x00, 0x00, 0x05)
	ins, tabID := writeTempPDF(t, "signed-der-trailing.pdf",
		sigUnitPDF(t, sigUnitOpt{contentsHex: hex.EncodeToString(payload)}))
	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	assertCleanCMSFailure(t, f, "Sig1")
	if !strings.Contains(f.DecomposeError, "trailing data") {
		t.Errorf("DecomposeError = %q, want the trailing-data reason", f.DecomposeError)
	}
}

func TestParseCMSSignedData_LeavesInputBytesUnchanged(t *testing.T) {
	ber := berAllIndef(t, signedPDFContentInfo(t))
	ber = append(ber, make([]byte, 64)...) // /Contents zero padding
	orig := bytes.Clone(ber)

	sd, enc, err := parseCMSSignedData(ber)
	if err != nil {
		t.Fatalf("parseCMSSignedData error: %v", err)
	}
	if enc != "BER" {
		t.Errorf("encoding = %q, want BER", enc)
	}
	if sd == nil || len(sd.Certificates.Bytes) == 0 || len(sd.SignerInfos) != 1 {
		t.Errorf("parsed SignedData incomplete: %+v", sd)
	}
	if !bytes.Equal(ber, orig) {
		t.Errorf("parseCMSSignedData modified its input")
	}
}

func TestParseCMSSignedData_DERPayloadReportsDER(t *testing.T) {
	raw := fixtureCMSPayload(t, "signed.pdf")
	_, enc, err := parseCMSSignedData(raw)
	if err != nil {
		t.Fatalf("parseCMSSignedData error: %v", err)
	}
	if enc != "DER" {
		t.Errorf("encoding = %q, want DER", enc)
	}
}

func TestUnmarshalCMS_RejectsJunkAfterSignedDataInsideContent(t *testing.T) {
	ci := signedPDFContentInfo(t)
	var outer struct {
		ContentType asn1.ObjectIdentifier
		Content     asn1.RawValue `asn1:"explicit,tag:0"`
	}
	if _, err := asn1.Unmarshal(ci, &outer); err != nil {
		t.Fatalf("signed.pdf ContentInfo: %v", err)
	}
	// Rebuild [0] with the SignedData followed by a stray NULL.
	inner := append(bytes.Clone(outer.Content.Bytes), 0x05, 0x00)
	oid := mustMarshal(t, outer.ContentType)
	wrapped := berDef(0xA0, inner)
	junk := berDef(0x30, oid, wrapped)

	if _, err := unmarshalCMS(junk); err == nil {
		t.Errorf("unmarshalCMS accepted a ContentInfo whose [0] carries bytes after the SignedData")
	}
}

func TestUnmarshalCMS_RejectsBytesAfterContentInfo(t *testing.T) {
	der := append(bytes.Clone(signedPDFContentInfo(t)), 0x00)

	_, err := unmarshalCMS(der)
	if err == nil {
		t.Fatal("unmarshalCMS accepted a ContentInfo followed by a stray byte")
	}
	if !strings.Contains(err.Error(), "1 bytes after the ContentInfo") {
		t.Errorf("error = %q, want the bytes-after-ContentInfo reason", err.Error())
	}
}

func TestGetSignatures_DeepNestingDegradesPerSignature(t *testing.T) {
	payload := berNestIndef(65, nil)
	ins, tabID := writeTempPDF(t, "signed-deep.pdf",
		sigUnitPDF(t, sigUnitOpt{contentsHex: hex.EncodeToString(payload)}))
	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	if f.DecomposeError == "" {
		t.Errorf("DecomposeError empty, want the nesting-depth failure")
	}
	if !strings.Contains(f.DecomposeError, "deeper than 64") {
		t.Errorf("DecomposeError = %q, want the nesting-depth reason", f.DecomposeError)
	}
	if len(f.Certificates) != 0 {
		t.Errorf("Certificates = %v, want none", f.Certificates)
	}
}

func TestGetSignatures_OversizedPayloadDegradesPerSignature(t *testing.T) {
	payload := berDef(0x04, make([]byte, maxCMSBytes+1-5))
	ins, tabID := writeTempPDF(t, "signed-huge.pdf",
		sigUnitPDF(t, sigUnitOpt{contentsHex: hex.EncodeToString(payload)}))
	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	if f.DecomposeError == "" {
		t.Errorf("DecomposeError empty, want the size-cap failure")
	}
	if !strings.Contains(f.DecomposeError, "larger than 8 MiB") {
		t.Errorf("DecomposeError = %q, want the size-cap reason", f.DecomposeError)
	}
}
