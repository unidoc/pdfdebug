// Unit tests for GetSignatureBytes, the export-side companion of
// GetSignatures.
//
//	(ins *Inspector) GetSignatureBytes(tabID string) (*SignatureList, []SignatureBytes, error)
//	SignatureBytes{ CMS []byte; CMSUndelimited string; Certificates [][]byte }
//
// The returned list is what GetSignatures returns for the same document, and
// the bytes slice is index-aligned with list.Signatures. CMS is the decoded
// /Contents prefix through the end of the top-level element, in the encoding
// found; when no element can be delimited it is every decoded byte and
// CMSUndelimited says why.
package pdfcore

import (
	"bytes"
	"crypto/x509"
	"encoding/hex"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// signatureFixtures is every committed signature fixture in testdata/.
var signatureFixtures = []string{
	"signed.pdf",
	"signed-notcovers.pdf",
	"signed-badcontents.pdf",
	"unsigned-sig-field.pdf",
	"signed-ber.pdf",
	"signed-ber-detached.pdf",
	"signed-ber-badeoc.pdf",
	"signed-ber-trailing.pdf",
	"signed-cert-extensions.pdf",
}

// openForSignatureBytes opens a committed fixture and returns the inspector
// and tab id.
func openForSignatureBytes(t *testing.T, name string) (*Inspector, string) {
	t.Helper()
	ins := NewInspector()
	id := "sig-bytes-" + name
	if _, err := ins.Open(id, filepath.Join(testdataDir(t), name)); err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	t.Cleanup(func() { _ = ins.Close(id) })
	return ins, id
}

// getSignatureBytes calls GetSignatureBytes and checks the alignment of the
// two returned slices.
func getSignatureBytes(t *testing.T, ins *Inspector, id string) (*SignatureList, []SignatureBytes) {
	t.Helper()
	list, sb, err := ins.GetSignatureBytes(id)
	if err != nil {
		t.Fatalf("GetSignatureBytes: %v", err)
	}
	if list == nil {
		t.Fatalf("GetSignatureBytes returned a nil list")
	}
	if len(sb) != len(list.Signatures) {
		t.Fatalf("%d SignatureBytes for %d signature fields; want one per field", len(sb), len(list.Signatures))
	}
	return list, sb
}

// assertCertBytesMatch checks that each exported certificate parses to the
// certificate listed at the same index.
func assertCertBytesMatch(t *testing.T, f SignatureField, b SignatureBytes) {
	t.Helper()
	if len(b.Certificates) != len(f.Certificates) {
		t.Fatalf("%d certificate blobs for %d listed certificates", len(b.Certificates), len(f.Certificates))
	}
	for i, der := range b.Certificates {
		c, err := x509.ParseCertificate(der)
		if err != nil {
			t.Errorf("Certificates[%d] does not parse: %v", i, err)
			continue
		}
		if got := certInfoFrom(c); !reflect.DeepEqual(got, f.Certificates[i]) {
			t.Errorf("Certificates[%d] parses to %+v\nlisted as %+v", i, got, f.Certificates[i])
		}
	}
}

// assertZeroBytes checks that b carries nothing to export.
func assertZeroBytes(t *testing.T, b SignatureBytes) {
	t.Helper()
	if b.CMS != nil || b.CMSUndelimited != "" || len(b.Certificates) != 0 {
		t.Errorf("SignatureBytes = {CMS: %d bytes (nil=%v), CMSUndelimited: %q, %d certificates}, want nothing",
			len(b.CMS), b.CMS == nil, b.CMSUndelimited, len(b.Certificates))
	}
}

func TestGetSignatureBytes_ListMatchesGetSignatures(t *testing.T) {
	for _, name := range signatureFixtures {
		t.Run(name, func(t *testing.T) {
			ins, id := openForSignatureBytes(t, name)
			want, err := ins.GetSignatures(id)
			if err != nil {
				t.Fatalf("GetSignatures: %v", err)
			}
			got, _ := getSignatureBytes(t, ins, id)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("GetSignatureBytes list\n got %+v\nwant %+v", got, want)
			}
		})
	}
}

func TestGetSignatureBytes_CMSIsTheElementAsFound(t *testing.T) {
	cases := []struct {
		name    string
		end     int
		wantBER bool
	}{
		{"signed.pdf", 2026, false},
		{"signed-ber-detached.pdf", 2028, true},
		{"signed-ber.pdf", 2265, true},
		{"signed-ber-trailing.pdf", 2028, true},
		{"signed-notcovers.pdf", 2026, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ins, id := openForSignatureBytes(t, tc.name)
			list, sb := getSignatureBytes(t, ins, id)
			if len(sb) != 1 {
				t.Fatalf("got %d entries, want 1", len(sb))
			}
			payload := fixtureCMSPayload(t, tc.name)
			if !bytes.Equal(sb[0].CMS, payload[:tc.end]) {
				t.Errorf("CMS is %d bytes, want the first %d decoded /Contents bytes", len(sb[0].CMS), tc.end)
			}
			if sb[0].CMSUndelimited != "" {
				t.Errorf("CMSUndelimited = %q, want empty", sb[0].CMSUndelimited)
			}
			if tc.wantBER && !bytes.HasPrefix(sb[0].CMS, []byte{0x30, 0x80}) {
				t.Errorf("CMS starts % X, want 30 80 (BER kept, not normalized)", sb[0].CMS[:min(2, len(sb[0].CMS))])
			}
			assertCertBytesMatch(t, list.Signatures[0], sb[0])
		})
	}
}

func TestGetSignatureBytes_CMSCertificatesOnCertExtensionsFixture(t *testing.T) {
	ins, id := openForSignatureBytes(t, "signed-cert-extensions.pdf")
	list, sb := getSignatureBytes(t, ins, id)
	if len(sb) != 1 {
		t.Fatalf("got %d entries, want 1", len(sb))
	}
	if len(sb[0].Certificates) != 2 {
		t.Fatalf("%d certificate blobs, want 2", len(sb[0].Certificates))
	}
	assertCertBytesMatch(t, list.Signatures[0], sb[0])
	end, err := berElementEnd(fixtureCMSPayload(t, "signed-cert-extensions.pdf"))
	if err != nil {
		t.Fatalf("berElementEnd: %v", err)
	}
	if len(sb[0].CMS) != end || sb[0].CMSUndelimited != "" {
		t.Errorf("CMS is %d bytes (undelimited %q), want the %d-byte element", len(sb[0].CMS), sb[0].CMSUndelimited, end)
	}
}

func TestGetSignatureBytes_UndelimitedPayloadIsEveryDecodedByte(t *testing.T) {
	for _, name := range []string{"signed-ber-badeoc.pdf", "signed-badcontents.pdf"} {
		t.Run(name, func(t *testing.T) {
			ins, id := openForSignatureBytes(t, name)
			_, sb := getSignatureBytes(t, ins, id)
			if len(sb) != 1 {
				t.Fatalf("got %d entries, want 1", len(sb))
			}
			payload := fixtureCMSPayload(t, name)
			if len(payload) != 3072 {
				t.Fatalf("fixture payload is %d bytes, want 3072", len(payload))
			}
			if !bytes.Equal(sb[0].CMS, payload) {
				t.Errorf("CMS is %d bytes, want all %d decoded /Contents bytes", len(sb[0].CMS), len(payload))
			}
			if sb[0].CMSUndelimited == "" {
				t.Errorf("CMSUndelimited is empty, want the reader error")
			}
			if len(sb[0].Certificates) != 0 {
				t.Errorf("%d certificate blobs, want none (the CMS did not parse)", len(sb[0].Certificates))
			}
		})
	}
}

func TestGetSignatureBytes_UnsignedFieldHasNothingToExport(t *testing.T) {
	ins, id := openForSignatureBytes(t, "unsigned-sig-field.pdf")
	_, sb := getSignatureBytes(t, ins, id)
	if len(sb) != 1 {
		t.Fatalf("got %d entries, want 1", len(sb))
	}
	assertZeroBytes(t, sb[0])
}

func TestGetSignatureBytes_CertEntrySignatureExportsItsCertificates(t *testing.T) {
	certHex := sigUnitCertHex(t)
	ins, tabID := writeTempPDF(t, "signed-x509.pdf", sigUnitPDF(t, sigUnitOpt{
		subFilter: "adbe.x509.rsa_sha1", certHex: certHex}))
	list, sb := getSignatureBytes(t, ins, tabID)
	if len(sb) != 1 {
		t.Fatalf("got %d entries, want 1", len(sb))
	}
	if sb[0].CMS != nil || sb[0].CMSUndelimited != "" {
		t.Errorf("CMS = %d bytes, CMSUndelimited = %q; want nil and empty for a PKCS#1 /Contents", len(sb[0].CMS), sb[0].CMSUndelimited)
	}
	want, err := hex.DecodeString(certHex)
	if err != nil {
		t.Fatalf("cert hex: %v", err)
	}
	if len(sb[0].Certificates) != 1 || !bytes.Equal(sb[0].Certificates[0], want) {
		t.Errorf("Certificates = %d blobs, want the one /Cert certificate", len(sb[0].Certificates))
	}
	assertCertBytesMatch(t, list.Signatures[0], sb[0])
}

func TestGetSignatureBytes_UnknownSubFilterHasNothingToExport(t *testing.T) {
	ins, tabID := writeTempPDF(t, "signed-unknown.pdf", sigUnitPDF(t, sigUnitOpt{subFilter: "foo.bar"}))
	_, sb := getSignatureBytes(t, ins, tabID)
	if len(sb) != 1 {
		t.Fatalf("got %d entries, want 1", len(sb))
	}
	assertZeroBytes(t, sb[0])
}

func TestGetSignatureBytes_DelimitedNonSignedDataIsStillExported(t *testing.T) {
	ins, tabID := writeTempPDF(t, "signed-notcms.pdf", sigUnitPDF(t, sigUnitOpt{contentsHex: "3003020101"}))
	list, sb := getSignatureBytes(t, ins, tabID)
	if len(sb) != 1 {
		t.Fatalf("got %d entries, want 1", len(sb))
	}
	if !strings.HasPrefix(list.Signatures[0].DecomposeError, "CMS parse failed: ") {
		t.Errorf("DecomposeError = %q, want a CMS parse failure", list.Signatures[0].DecomposeError)
	}
	if !bytes.Equal(sb[0].CMS, []byte{0x30, 0x03, 0x02, 0x01, 0x01}) {
		t.Errorf("CMS = % X, want 30 03 02 01 01", sb[0].CMS)
	}
	if sb[0].CMSUndelimited != "" {
		t.Errorf("CMSUndelimited = %q, want empty", sb[0].CMSUndelimited)
	}
}

// sigUnsignedThenSignedPDF builds two /FT /Sig fields: an unsigned
// placeholder first in /Fields, then a field signed with contentsHex as the
// whole /Contents payload. Objects: 1 Catalog, 2 Pages, 3 Page, 4 unsigned
// field, 5 sig dict, 6 signed field.
func sigUnsignedThenSignedPDF(t *testing.T, contentsHex string) []byte {
	t.Helper()
	sig := "<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter /adbe.pkcs7.detached" +
		" /ByteRange [0000000000 0000000000 0000000000 0000000000]" +
		" /Contents <" + strings.Repeat("0", len(contentsHex)) + "> >>"
	file := assemblexref("%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R 6 0 R] /SigFlags 3 >> >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Annots [4 0 R 6 0 R] >>\nendobj\n",
		"4 0 obj\n<< /Type /Annot /Subtype /Widget /FT /Sig /T (Placeholder) /Rect [0 0 0 0] /P 3 0 R /F 132 >>\nendobj\n",
		"5 0 obj\n"+sig+"\nendobj\n",
		"6 0 obj\n<< /Type /Annot /Subtype /Widget /FT /Sig /T (Signed) /Rect [0 0 0 0] /P 3 0 R /F 132 /V 5 0 R >>\nendobj\n",
	)
	ci := bytes.Index(file, []byte("/Contents <"))
	if ci < 0 {
		t.Fatal("no /Contents placeholder")
	}
	holeStart := ci + len("/Contents ")
	holeEnd := holeStart + 1 + len(contentsHex) + 1
	br := "[" + pad10(0) + " " + pad10(holeStart) + " " + pad10(holeEnd) + " " + pad10(len(file)-holeEnd) + "]"
	bi := bytes.Index(file, []byte("/ByteRange ["))
	copy(file[bi+len("/ByteRange "):], br)
	copy(file[holeStart+1:], contentsHex)
	return file
}

func TestGetSignatureBytes_EntriesStayAlignedPastAnUnsignedField(t *testing.T) {
	ci := signedPDFContentInfo(t)
	ins, tabID := writeTempPDF(t, "unsigned-then-signed.pdf", sigUnsignedThenSignedPDF(t, strings.ToUpper(hex.EncodeToString(ci))))
	list, sb := getSignatureBytes(t, ins, tabID)
	if len(list.Signatures) != 2 {
		t.Fatalf("got %d signature fields, want 2", len(list.Signatures))
	}
	if list.Signatures[0].FieldName != "Placeholder" || list.Signatures[0].Signed {
		t.Fatalf("Signatures[0] = %q signed=%v, want the unsigned Placeholder", list.Signatures[0].FieldName, list.Signatures[0].Signed)
	}
	if list.Signatures[1].FieldName != "Signed" || !list.Signatures[1].Signed {
		t.Fatalf("Signatures[1] = %q signed=%v, want the signed field", list.Signatures[1].FieldName, list.Signatures[1].Signed)
	}
	assertZeroBytes(t, sb[0])
	if !bytes.Equal(sb[1].CMS, ci) {
		t.Errorf("Signatures[1] CMS is %d bytes, want the %d-byte ContentInfo", len(sb[1].CMS), len(ci))
	}
	assertCertBytesMatch(t, list.Signatures[1], sb[1])
	if len(sb[1].Certificates) != 2 {
		t.Errorf("Signatures[1] has %d certificate blobs, want 2", len(sb[1].Certificates))
	}
}

func TestGetSignatureBytes_UnknownTabFails(t *testing.T) {
	ins := NewInspector()
	if _, _, err := ins.GetSignatureBytes("no-such-tab"); err == nil {
		t.Errorf("GetSignatureBytes on an unknown tab returned no error")
	}
}

func TestGetSignatureBytes_LiteralStringContentsIsUnescaped(t *testing.T) {
	// /Contents is the literal string 0\005\004\003\(\)\\, which decodes to
	// SEQUENCE { OCTET STRING "()\" }: 30 05 04 03 28 29 5C.
	file := assemblexref("%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] /SigFlags 3 >> >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Annots [4 0 R] >>\nendobj\n",
		"4 0 obj\n<< /Type /Annot /Subtype /Widget /FT /Sig /T (Sig1) /Rect [0 0 0 0] /P 3 0 R /F 132 /V 5 0 R >>\nendobj\n",
		"5 0 obj\n<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter /adbe.pkcs7.detached /ByteRange [0 10 20 10]"+
			` /Contents (0\005\004\003\(\)\\) >>`+"\nendobj\n",
	)
	ins, tabID := writeTempPDF(t, "literal-contents.pdf", file)
	list, sb := getSignatureBytes(t, ins, tabID)
	if len(sb) != 1 {
		t.Fatalf("got %d entries, want 1", len(sb))
	}
	want := []byte{0x30, 0x05, 0x04, 0x03, 0x28, 0x29, 0x5C}
	if !bytes.Equal(sb[0].CMS, want) {
		t.Errorf("CMS = % X, want % X", sb[0].CMS, want)
	}
	if sb[0].CMSUndelimited != "" {
		t.Errorf("CMSUndelimited = %q, want empty", sb[0].CMSUndelimited)
	}
	if !strings.HasPrefix(list.Signatures[0].DecomposeError, "CMS parse failed: ") {
		t.Errorf("DecomposeError = %q, want a CMS parse failure of the unescaped bytes", list.Signatures[0].DecomposeError)
	}
}

func TestGetSignatureBytes_EmptyContentsExportsZeroBytes(t *testing.T) {
	literal := assemblexref("%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] /SigFlags 3 >> >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Annots [4 0 R] >>\nendobj\n",
		"4 0 obj\n<< /Type /Annot /Subtype /Widget /FT /Sig /T (Sig1) /Rect [0 0 0 0] /P 3 0 R /F 132 /V 5 0 R >>\nendobj\n",
		"5 0 obj\n<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter /adbe.pkcs7.detached /ByteRange [0 10 20 10] /Contents () >>\nendobj\n",
	)
	cases := []struct {
		name string
		file []byte
	}{
		{"empty hex string", sigUnsignedThenSignedPDF(t, "")},
		{"empty literal string", literal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ins, tabID := writeTempPDF(t, "empty-contents.pdf", tc.file)
			list, sb := getSignatureBytes(t, ins, tabID)
			last := len(sb) - 1
			if last < 0 || !list.Signatures[last].Signed {
				t.Fatalf("got %d entries, want the signed field last", len(sb))
			}
			if sb[last].CMS == nil || len(sb[last].CMS) != 0 {
				t.Errorf("CMS = %#v, want an empty non-nil slice so the export writes an empty file", sb[last].CMS)
			}
			if sb[last].CMSUndelimited == "" {
				t.Errorf("CMSUndelimited is empty, want the reader error for an empty /Contents")
			}
			if list.Signatures[last].DecomposeError == "" {
				t.Errorf("DecomposeError is empty, want a CMS parse failure")
			}
		})
	}
}
