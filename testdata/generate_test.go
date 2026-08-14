package testdata_test

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math/big"
	"os"
	"strings"
	"testing"
	"time"

	pdfcpu_api "github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// minimalPDFContent returns a valid single-page PDF as raw bytes.
func minimalPDFContent() []byte {
	pdf := "%PDF-1.4\n"

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n"

	body := pdf + obj1 + obj2 + obj3

	o1 := len(pdf)
	o2 := o1 + len(obj1)
	o3 := o2 + len(obj2)
	xrefOffset := len(body)

	xref := fmt.Sprintf("xref\n0 4\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n",
		o1, o2, o3)
	trailer := fmt.Sprintf("trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOffset)

	return []byte(body + xref + trailer)
}

// multipagePDFContent returns a valid 3-page PDF as raw bytes.
func multipagePDFContent() []byte {
	pdf := "%PDF-1.4\n"

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 3 >>\nendobj\n\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n"
	obj4 := "4 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n"
	obj5 := "5 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n"

	body := pdf + obj1 + obj2 + obj3 + obj4 + obj5

	o1 := len(pdf)
	o2 := o1 + len(obj1)
	o3 := o2 + len(obj2)
	o4 := o3 + len(obj3)
	o5 := o4 + len(obj4)
	xrefOffset := len(body)

	xref := fmt.Sprintf("xref\n0 6\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n",
		o1, o2, o3, o4, o5)
	trailer := fmt.Sprintf("trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOffset)

	return []byte(body + xref + trailer)
}

// contentStreamPDFContent returns a valid single-page PDF with a content stream.
func contentStreamPDFContent() []byte {
	pdf := "%PDF-1.4\n"

	stream := "BT /F1 12 Tf 100 700 Td (Hello World) Tj ET"
	streamLen := len(stream)

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n"
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>\nendobj\n\n")
	obj4 := fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n\n", streamLen, stream)

	body := pdf + obj1 + obj2 + obj3 + obj4

	o1 := len(pdf)
	o2 := o1 + len(obj1)
	o3 := o2 + len(obj2)
	o4 := o3 + len(obj3)
	xrefOffset := len(body)

	xref := fmt.Sprintf("xref\n0 5\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n",
		o1, o2, o3, o4)
	trailer := fmt.Sprintf("trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOffset)

	return []byte(body + xref + trailer)
}

// emptyStreamPDFContent returns a valid single-page PDF with a zero-length content stream.
func emptyStreamPDFContent() []byte {
	pdf := "%PDF-1.4\n"

	stream := ""
	streamLen := len(stream)

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n"
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>\nendobj\n\n")
	obj4 := fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n\n", streamLen, stream)

	body := pdf + obj1 + obj2 + obj3 + obj4

	o1 := len(pdf)
	o2 := o1 + len(obj1)
	o3 := o2 + len(obj2)
	o4 := o3 + len(obj3)
	xrefOffset := len(body)

	xref := fmt.Sprintf("xref\n0 5\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n",
		o1, o2, o3, o4)
	trailer := fmt.Sprintf("trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOffset)

	return []byte(body + xref + trailer)
}

// imageXObjectPDFContent returns a valid single-page PDF with an embedded
// DCTDecode (JPEG) XObject image. The image is 4x4 pixels, DeviceRGB, 8bpc.
func imageXObjectPDFContent() []byte {
	// Create a 4x4 JPEG image
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var jpegBuf bytes.Buffer
	_ = jpeg.Encode(&jpegBuf, img, &jpeg.Options{Quality: 90})
	jpegBytes := jpegBuf.Bytes()

	pdf := "%PDF-1.4\n"

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /Im0 4 0 R >> >> >>\nendobj\n\n"
	obj4 := fmt.Sprintf("4 0 obj\n<< /Type /XObject /Subtype /Image /Width 4 /Height 4 /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n", len(jpegBytes))
	obj4end := "\nendstream\nendobj\n\n"

	body := pdf + obj1 + obj2 + obj3 + obj4
	bodyBytes := []byte(body)
	bodyBytes = append(bodyBytes, jpegBytes...)
	bodyBytes = append(bodyBytes, []byte(obj4end)...)

	o1 := len(pdf)
	o2 := o1 + len(obj1)
	o3 := o2 + len(obj2)
	o4 := o3 + len(obj3)
	xrefOffset := len(bodyBytes)

	xref := fmt.Sprintf("xref\n0 5\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n",
		o1, o2, o3, o4)
	trailer := fmt.Sprintf("trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOffset)

	bodyBytes = append(bodyBytes, []byte(xref+trailer)...)
	return bodyBytes
}

// fontsMixedPDFContent returns a multi-font PDF used by Story 9-9 tests.
// Covers: simple Type1 with named encoding, TrueType with /Differences, Type0
// composite with Identity-H + CIDFontType2 descendant + a bfchar ToUnicode
// CMap, and an unembedded reference font. The PDF references font streams in
// a structurally valid form; pdfcpu accepts the xref table without rendering
// the page content (the test suite only consults dict structure).
func fontsMixedPDFContent() []byte {
	pdf := "%PDF-1.7\n"

	// Object 1 -- catalog. Page tree only carries one minimal page; the
	// font dicts referenced from the page's /Resources drive the tests.
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	// Pages -- references one page that lists every font in /Resources.
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " +
		"/Resources << /Font << /F1 4 0 R /F2 5 0 R /F3 6 0 R /F4 7 0 R >> >> >>\nendobj\n\n"

	// Object 4 -- unembedded Type1 Helvetica (no FontDescriptor at all).
	obj4 := "4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>\nendobj\n\n"

	// Object 5 -- Type1 with /Differences encoding, FontDescriptor without
	// any FontFile (unembedded but has descriptor).
	obj5 := "5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /CustomFont " +
		"/FirstChar 32 /LastChar 34 " +
		"/Encoding << /Type /Encoding /BaseEncoding /WinAnsiEncoding " +
		"/Differences [32 /space /exclam /quotedbl] >> " +
		"/FontDescriptor 8 0 R >>\nendobj\n\n"

	// Object 6 -- Type0 composite with descendant CIDFontType2.
	obj6 := "6 0 obj\n<< /Type /Font /Subtype /Type0 /BaseFont /NotoSansCJK-Regular " +
		"/Encoding /Identity-H /DescendantFonts [9 0 R] /ToUnicode 10 0 R >>\nendobj\n\n"

	// Object 7 -- TrueType with FontDescriptor that has a FontFile2 stream.
	obj7 := "7 0 obj\n<< /Type /Font /Subtype /TrueType /BaseFont /MyTTFont " +
		"/FirstChar 32 /LastChar 126 /FontDescriptor 11 0 R >>\nendobj\n\n"

	// Object 8 -- FontDescriptor without FontFile (unembedded).
	obj8 := "8 0 obj\n<< /Type /FontDescriptor /FontName /CustomFont /Flags 32 " +
		"/ItalicAngle 0 /Ascent 718 /Descent -207 /CapHeight 718 /StemV 140 " +
		"/FontBBox [-170 -228 1003 962] >>\nendobj\n\n"

	// Object 9 -- CIDFontType2 descendant. References FontDescriptor 12.
	obj9 := "9 0 obj\n<< /Type /Font /Subtype /CIDFontType2 /BaseFont /NotoSansCJK-Regular " +
		"/CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> " +
		"/CIDToGIDMap /Identity /DW 1000 /FontDescriptor 12 0 R >>\nendobj\n\n"

	// Object 10 -- ToUnicode CMap stream. Minimal bfchar mapping 0x0041 -> "A".
	cmap := "/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n" +
		"/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n" +
		"/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n" +
		"1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n" +
		"2 beginbfchar\n<0041> <0041>\n<0042> <0042>\nendbfchar\n" +
		"endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n"
	obj10 := fmt.Sprintf("10 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n\n", len(cmap), cmap)

	// Object 11 -- FontDescriptor with FontFile2 (TrueType embedded).
	obj11 := "11 0 obj\n<< /Type /FontDescriptor /FontName /MyTTFont /Flags 32 " +
		"/ItalicAngle 0 /Ascent 750 /Descent -250 /CapHeight 700 /StemV 80 " +
		"/FontBBox [-100 -200 1000 900] /FontFile2 13 0 R >>\nendobj\n\n"

	// Object 12 -- Descendant CIDFontType2's FontDescriptor (TrueType embedded).
	obj12 := "12 0 obj\n<< /Type /FontDescriptor /FontName /NotoSansCJK-Regular /Flags 4 " +
		"/ItalicAngle 0 /Ascent 880 /Descent -120 /CapHeight 880 /StemV 80 " +
		"/FontBBox [-200 -300 1100 1000] /FontFile2 14 0 R >>\nendobj\n\n"

	// Object 13 -- TrueType FontFile2 stream (minimal placeholder bytes).
	ttBytes := "TRUETYPE FONT PROGRAM PLACEHOLDER BYTES FOR TESTING"
	obj13 := fmt.Sprintf("13 0 obj\n<< /Length %d /Length1 %d >>\nstream\n%s\nendstream\nendobj\n\n", len(ttBytes), len(ttBytes), ttBytes)

	// Object 14 -- Descendant TrueType FontFile2 stream.
	ttBytes2 := "DESCENDANT TRUETYPE FONT PROGRAM PLACEHOLDER"
	obj14 := fmt.Sprintf("14 0 obj\n<< /Length %d /Length1 %d >>\nstream\n%s\nendstream\nendobj\n\n", len(ttBytes2), len(ttBytes2), ttBytes2)

	body := pdf + obj1 + obj2 + obj3 + obj4 + obj5 + obj6 + obj7 + obj8 + obj9 + obj10 + obj11 + obj12 + obj13 + obj14

	o1 := len(pdf)
	o2 := o1 + len(obj1)
	o3 := o2 + len(obj2)
	o4 := o3 + len(obj3)
	o5 := o4 + len(obj4)
	o6 := o5 + len(obj5)
	o7 := o6 + len(obj6)
	o8 := o7 + len(obj7)
	o9 := o8 + len(obj8)
	o10 := o9 + len(obj9)
	o11 := o10 + len(obj10)
	o12 := o11 + len(obj11)
	o13 := o12 + len(obj12)
	o14 := o13 + len(obj13)
	xrefOffset := len(body)

	xref := fmt.Sprintf(
		"xref\n0 15\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n",
		o1, o2, o3, o4, o5, o6, o7, o8, o9, o10, o11, o12, o13, o14)
	trailer := fmt.Sprintf("trailer\n<< /Size 15 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOffset)

	return []byte(body + xref + trailer)
}

// --- Story 13-4 digital-signature fixtures ------------------------------
//
// A REAL adbe.pkcs7.detached signature built programmatically: self-signed CA
// + leaf signer cert via crypto/x509.CreateCertificate, a CMS SignedData
// assembled over the actual ByteRange digest with encoding/asn1, RSA-signed
// for real. The CA cert is FIRST in the certificate set so signer
// identification cannot be positional. The malformed-Contents and not-covers
// variants derive from the same builder by byte surgery.

// sigFixtureContentsHexCap is the reserved /Contents hex capacity; the real
// DER is ~2.5 KB, the remainder stays zero-padded (exercises the
// trailing-zero-trim path).
const sigFixtureContentsHexCap = 6144

type sigFixtureAlgID struct {
	Algorithm  asn1.ObjectIdentifier
	Parameters asn1.RawValue `asn1:"optional"`
}

type sigFixtureIssuerAndSerial struct {
	Issuer asn1.RawValue
	Serial *big.Int
}

type sigFixtureSignerInfo struct {
	Version         int
	IAS             sigFixtureIssuerAndSerial
	DigestAlg       sigFixtureAlgID
	SigAlg          sigFixtureAlgID
	EncryptedDigest []byte
}

type sigFixtureEncapContent struct {
	ContentType asn1.ObjectIdentifier
}

type sigFixtureSignedData struct {
	Version          int
	DigestAlgorithms []sigFixtureAlgID `asn1:"set"`
	ContentInfo      sigFixtureEncapContent
	Certificates     asn1.RawValue
	SignerInfos      []sigFixtureSignerInfo `asn1:"set"`
}

type sigFixtureContentInfo struct {
	ContentType asn1.ObjectIdentifier
	// Content carries the [0] EXPLICIT wrapper pre-built as a RawValue:
	// encoding/asn1 does not apply explicit-tag options to RawValue fields.
	Content asn1.RawValue
}

var (
	sigFixtureOIDSignedData = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	sigFixtureOIDData       = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	sigFixtureOIDSHA256     = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	sigFixtureOIDRSA        = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
)

// sigFixturePKI generates a self-signed CA and a leaf signer cert with a fixed
// validity window (2025-01-01 .. 2027-01-01 UTC).
func sigFixturePKI(t *testing.T) (caDER, leafDER []byte, leafCert *x509.Certificate, leafKey *rsa.PrivateKey) {
	t.Helper()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("fixture CA key: %v", err)
	}
	leafKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("fixture leaf key: %v", err)
	}
	nb := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	na := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(77001),
		Subject:               pkix.Name{CommonName: "Fixture Root CA", Organization: []string{"UniDoc Fixtures"}},
		NotBefore:             nb,
		NotAfter:              na,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDER, err = x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("fixture CA cert: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("fixture CA parse: %v", err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2026),
		Subject:      pkix.Name{CommonName: "Fixture Signer", Organization: []string{"UniDoc Fixtures"}},
		NotBefore:    nb,
		NotAfter:     na,
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	leafDER, err = x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("fixture leaf cert: %v", err)
	}
	leafCert, err = x509.ParseCertificate(leafDER)
	if err != nil {
		t.Fatalf("fixture leaf parse: %v", err)
	}
	return caDER, leafDER, leafCert, leafKey
}

// sigFixtureCMS assembles a DER ContentInfo(SignedData) over digest,
// RSA-signed for real, CA cert first in the certificate set.
func sigFixtureCMS(t *testing.T, caDER, leafDER []byte, leafCert *x509.Certificate, leafKey *rsa.PrivateKey, digest []byte) []byte {
	t.Helper()
	sig, err := rsa.SignPKCS1v15(rand.Reader, leafKey, crypto.SHA256, digest)
	if err != nil {
		t.Fatalf("fixture RSA sign: %v", err)
	}
	certsRaw := asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true,
		Bytes: append(append([]byte{}, caDER...), leafDER...)}
	sd := sigFixtureSignedData{
		Version:          1,
		DigestAlgorithms: []sigFixtureAlgID{{Algorithm: sigFixtureOIDSHA256, Parameters: asn1.NullRawValue}},
		ContentInfo:      sigFixtureEncapContent{ContentType: sigFixtureOIDData},
		Certificates:     certsRaw,
		SignerInfos: []sigFixtureSignerInfo{{
			Version: 1,
			IAS: sigFixtureIssuerAndSerial{
				Issuer: asn1.RawValue{FullBytes: leafCert.RawIssuer},
				Serial: leafCert.SerialNumber,
			},
			DigestAlg:       sigFixtureAlgID{Algorithm: sigFixtureOIDSHA256, Parameters: asn1.NullRawValue},
			SigAlg:          sigFixtureAlgID{Algorithm: sigFixtureOIDRSA, Parameters: asn1.NullRawValue},
			EncryptedDigest: sig,
		}},
	}
	sdDER, err := asn1.Marshal(sd)
	if err != nil {
		t.Fatalf("fixture SignedData marshal: %v", err)
	}
	ci := sigFixtureContentInfo{ContentType: sigFixtureOIDSignedData,
		Content: asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: sdDER}}
	ciDER, err := asn1.Marshal(ci)
	if err != nil {
		t.Fatalf("fixture ContentInfo marshal: %v", err)
	}
	return ciDER
}

// sigFixtureAssemblePDF stitches a header, object bodies (object i+1), an
// xref table, and a trailer with /Root 1 0 R.
func sigFixtureAssemblePDF(objs []string) []byte {
	body := "%PDF-1.7\n"
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = len(body)
		body += o
	}
	xrefOff := len(body)
	size := len(objs) + 1
	xref := fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", size)
	for _, off := range offsets {
		xref += fmt.Sprintf("%010d 00000 n \n", off)
	}
	trailer := fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", size, xrefOff)
	return []byte(body + xref + trailer)
}

// signedPDFContent builds a one-signature adbe.pkcs7.detached PDF whose
// ByteRange is computed against the true /Contents hole and whose CMS blob is
// signed over the actual ByteRange digest. shortfall subtracts from
// ByteRange[3] (the not-covers variant); corrupt breaks the leading DER bytes
// (the malformed-Contents variant, still a valid PDF).
func signedPDFContent(t *testing.T, shortfall int, corrupt bool) []byte {
	t.Helper()
	caDER, leafDER, leafCert, leafKey := sigFixturePKI(t)
	sig := "<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter /adbe.pkcs7.detached" +
		" /ByteRange [0000000000 0000000000 0000000000 0000000000]" +
		" /Contents <" + strings.Repeat("0", sigFixtureContentsHexCap) + ">" +
		" /M (D:20260101120000+00'00') /Name (Fixture Signer) /Reason (Fixture) /Location (Testville) >>"
	file := sigFixtureAssemblePDF([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] /SigFlags 3 >> >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Annots [4 0 R] >>\nendobj\n",
		"4 0 obj\n<< /Type /Annot /Subtype /Widget /FT /Sig /T (Sig1) /Rect [0 0 0 0] /P 3 0 R /F 132 /V 5 0 R >>\nendobj\n",
		"5 0 obj\n" + sig + "\nendobj\n",
	})

	ci := bytes.Index(file, []byte("/Contents <"))
	if ci < 0 {
		t.Fatal("signature fixture: no /Contents placeholder")
	}
	holeStart := ci + len("/Contents ")
	holeEnd := holeStart + 1 + sigFixtureContentsHexCap + 1 // '<' + hex + '>'

	br := fmt.Sprintf("[%010d %010d %010d %010d]", 0, holeStart, holeEnd, len(file)-holeEnd-shortfall)
	bi := bytes.Index(file, []byte("/ByteRange ["))
	if bi < 0 {
		t.Fatal("signature fixture: no /ByteRange placeholder")
	}
	copy(file[bi+len("/ByteRange "):], br)

	// Digest over the two spans around the TRUE hole, then sign for real.
	h := sha256.New()
	h.Write(file[:holeStart])
	h.Write(file[holeEnd:])
	hx := hex.EncodeToString(sigFixtureCMS(t, caDER, leafDER, leafCert, leafKey, h.Sum(nil)))
	if len(hx) > sigFixtureContentsHexCap {
		t.Fatalf("signature fixture: contents capacity exceeded: %d", len(hx))
	}
	if corrupt {
		// Break the leading DER tag/length; keep valid hex so the PDF parses.
		copy(file[holeStart+1:], "FFFFFFFFFFFFFFFF")
		copy(file[holeStart+1+16:], hx[16:])
	} else {
		copy(file[holeStart+1:], hx)
	}
	// The remainder of the hole stays "0"-filled = trailing zero padding.
	return file
}

// unsignedSigFieldPDFContent builds a /FT /Sig placeholder field with NO /V.
func unsignedSigFieldPDFContent() []byte {
	return sigFixtureAssemblePDF([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] /SigFlags 3 >> >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Annots [4 0 R] >>\nendobj\n",
		"4 0 obj\n<< /Type /Annot /Subtype /Widget /FT /Sig /T (EmptySig) /Rect [0 0 0 0] /P 3 0 R /F 132 >>\nendobj\n",
	})
}

// --- Story 13-5 structural-compliance fixtures --------------------------
//
// Programmatic negative fixtures (non-embedded font, device color without
// OutputIntent, tagged vs untagged) plus the veraPDF-passing clean PDF/A-1b
// positive fixture. The tests/compliance-validation acceptance suite carries
// its own self-contained copies of the negative builders; these are the
// testdata/ equivalents Task 2.0 asks for. The clean fixture is the one the
// veraPDF oracle clean-case cross-check reads.

// complAssemblePDF stitches a binary-marker header, object bodies, an xref
// table, and a trailer with /Root 1 0 R plus optional trailerExtra.
func complAssemblePDF(objs []string, trailerExtra string) []byte {
	body := "%PDF-1.4\n%\xe2\xe3\xcf\xd3\n"
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = len(body)
		body += o
	}
	xrefOff := len(body)
	size := len(objs) + 1
	xref := fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", size)
	for _, off := range offsets {
		xref += fmt.Sprintf("%010d 00000 n \n", off)
	}
	trailer := fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R %s>>\nstartxref\n%d\n%%%%EOF\n", size, trailerExtra, xrefOff)
	return []byte(body + xref + trailer)
}

// complStreamObj renders object n as a stream object with a correct /Length.
func complStreamObj(n int, dictExtra, payload string) string {
	return fmt.Sprintf("%d 0 obj\n<< /Length %d %s>>\nstream\n%s\nendstream\nendobj\n", n, len(payload), dictExtra, payload)
}

// nonEmbeddedFontPDFContent references a Type1 font with no /FontDescriptor /
// FontFile* (PDF/A-1b 6.3.4 forbids non-embedded fonts). The font is object 4.
func nonEmbeddedFontPDFContent() []byte {
	return complAssemblePDF([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>\nendobj\n",
		"4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n",
		complStreamObj(5, "", "BT /F1 24 Tf 72 720 Td (Hi) Tj ET"),
	}, "")
}

// noOutputIntentPDFContent draws with a device RGB fill (rg operator) but
// declares no /OutputIntents (PDF/A-1b 6.2.2 requires an OutputIntent when
// device color is used).
func noOutputIntentPDFContent() []byte {
	return complAssemblePDF([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>\nendobj\n",
		complStreamObj(4, "", "1 0 0 rg 10 10 100 100 re f"),
	}, "")
}

// untaggedPDFContent has no /MarkInfo, /StructTreeRoot, or /Lang (the PDF/UA-1
// structural warnings; missing /Lang is the canonical document-level problem).
func untaggedPDFContent() []byte {
	return complAssemblePDF([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
	}, "")
}

// taggedPDFContent satisfies the PDF/UA-1 structural subset: /MarkInfo /Marked
// true, a /StructTreeRoot, and a document /Lang.
func taggedPDFContent() []byte {
	return complAssemblePDF([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 4 0 R /Lang (en-US) >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		"4 0 obj\n<< /Type /StructTreeRoot >>\nendobj\n",
	}, "")
}

// pdfaCleanContent builds a minimal PDF/A-1b file that veraPDF --flavour 1b
// passes: a PDF/A-identification XMP packet (pdfaid part 1 / conformance B), a
// document /ID, no fonts, no device color, and a blank page. Provenance:
// hand-assembled during Story 13-5 implementation and verified to pass veraPDF
// 1.30.x --flavour 1b with zero failed clauses (the clean-case oracle fixture;
// our rule set also flags zero errors on it).
func pdfaCleanContent() []byte {
	xmp := `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>
<x:xmpmeta xmlns:x="adobe:ns:meta/">
 <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description rdf:about="" xmlns:pdfaid="http://www.aiim.org/pdfa/ns/id/">
   <pdfaid:part>1</pdfaid:part>
   <pdfaid:conformance>B</pdfaid:conformance>
  </rdf:Description>
  <rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/">
   <dc:format>application/pdf</dc:format>
  </rdf:Description>
 </rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>`
	return complAssemblePDF([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Metadata 4 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> >>\nendobj\n",
		complStreamObj(4, "/Type /Metadata /Subtype /XML ", xmp),
	}, "/ID [<0123456789ABCDEF0123456789ABCDEF> <0123456789ABCDEF0123456789ABCDEF>] ")
}

// TestGenerateFixtures creates test PDF files used by the test suite.
// Run with: go test -run TestGenerateFixtures -v ./testdata/
func TestGenerateFixtures(t *testing.T) {
	t.Run("minimal.pdf", func(t *testing.T) {
		if _, err := os.Stat("minimal.pdf"); err == nil {
			t.Skip("minimal.pdf already exists")
		}
		if err := os.WriteFile("minimal.pdf", minimalPDFContent(), 0644); err != nil {
			t.Fatalf("failed to create minimal.pdf: %v", err)
		}
		// Verify pdfcpu can read it
		ctx, err := pdfcpu_api.ReadContextFile("minimal.pdf")
		if err != nil {
			os.Remove("minimal.pdf")
			t.Fatalf("minimal.pdf is not valid according to pdfcpu: %v", err)
		}
		t.Logf("minimal.pdf created: %d pages", ctx.PageCount)
	})

	t.Run("multipage.pdf", func(t *testing.T) {
		if _, err := os.Stat("multipage.pdf"); err == nil {
			t.Skip("multipage.pdf already exists")
		}
		if err := os.WriteFile("multipage.pdf", multipagePDFContent(), 0644); err != nil {
			t.Fatalf("failed to create multipage.pdf: %v", err)
		}
		ctx, err := pdfcpu_api.ReadContextFile("multipage.pdf")
		if err != nil {
			os.Remove("multipage.pdf")
			t.Fatalf("multipage.pdf is not valid according to pdfcpu: %v", err)
		}
		t.Logf("multipage.pdf created: %d pages", ctx.PageCount)
	})

	t.Run("content-stream.pdf", func(t *testing.T) {
		if _, err := os.Stat("content-stream.pdf"); err == nil {
			t.Skip("content-stream.pdf already exists")
		}
		if err := os.WriteFile("content-stream.pdf", contentStreamPDFContent(), 0644); err != nil {
			t.Fatalf("failed to create content-stream.pdf: %v", err)
		}
		ctx, err := pdfcpu_api.ReadContextFile("content-stream.pdf")
		if err != nil {
			os.Remove("content-stream.pdf")
			t.Fatalf("content-stream.pdf is not valid according to pdfcpu: %v", err)
		}
		t.Logf("content-stream.pdf created: %d pages", ctx.PageCount)
	})

	t.Run("empty-stream.pdf", func(t *testing.T) {
		if _, err := os.Stat("empty-stream.pdf"); err == nil {
			t.Skip("empty-stream.pdf already exists")
		}
		if err := os.WriteFile("empty-stream.pdf", emptyStreamPDFContent(), 0644); err != nil {
			t.Fatalf("failed to create empty-stream.pdf: %v", err)
		}
		ctx, err := pdfcpu_api.ReadContextFile("empty-stream.pdf")
		if err != nil {
			os.Remove("empty-stream.pdf")
			t.Fatalf("empty-stream.pdf is not valid according to pdfcpu: %v", err)
		}
		t.Logf("empty-stream.pdf created: %d pages", ctx.PageCount)
	})

	t.Run("malformed.pdf", func(t *testing.T) {
		if _, err := os.Stat("malformed.pdf"); err == nil {
			t.Skip("malformed.pdf already exists")
		}
		data := []byte("%PDF-1.4\n%%EOF\ngarbage xref corrupted data truncated")
		if err := os.WriteFile("malformed.pdf", data, 0644); err != nil {
			t.Fatalf("failed to create malformed.pdf: %v", err)
		}
	})

	t.Run("encrypted.pdf", func(t *testing.T) {
		if _, err := os.Stat("encrypted.pdf"); err == nil {
			t.Skip("encrypted.pdf already exists")
		}
		// Ensure minimal.pdf exists first
		if _, err := os.Stat("minimal.pdf"); os.IsNotExist(err) {
			if err := os.WriteFile("minimal.pdf", minimalPDFContent(), 0644); err != nil {
				t.Fatalf("failed to create minimal.pdf for encryption: %v", err)
			}
		}
		conf := pdfcpu_model.NewAESConfiguration("testpass", "ownerpass", 256)
		if err := pdfcpu_api.EncryptFile("minimal.pdf", "encrypted.pdf", conf); err != nil {
			t.Fatalf("failed to create encrypted.pdf: %v", err)
		}
	})

	t.Run("fonts-mixed.pdf", func(t *testing.T) {
		if _, err := os.Stat("fonts-mixed.pdf"); err == nil {
			t.Skip("fonts-mixed.pdf already exists")
		}
		if err := os.WriteFile("fonts-mixed.pdf", fontsMixedPDFContent(), 0644); err != nil {
			t.Fatalf("failed to create fonts-mixed.pdf: %v", err)
		}
		// Verify pdfcpu can read it.
		ctx, err := pdfcpu_api.ReadContextFile("fonts-mixed.pdf")
		if err != nil {
			os.Remove("fonts-mixed.pdf")
			t.Fatalf("fonts-mixed.pdf is not valid according to pdfcpu: %v", err)
		}
		t.Logf("fonts-mixed.pdf created: %d pages", ctx.PageCount)
	})

	t.Run("image-xobject.pdf", func(t *testing.T) {
		if _, err := os.Stat("image-xobject.pdf"); err == nil {
			t.Skip("image-xobject.pdf already exists")
		}
		if err := os.WriteFile("image-xobject.pdf", imageXObjectPDFContent(), 0644); err != nil {
			t.Fatalf("failed to create image-xobject.pdf: %v", err)
		}
		ctx, err := pdfcpu_api.ReadContextFile("image-xobject.pdf")
		if err != nil {
			os.Remove("image-xobject.pdf")
			t.Fatalf("image-xobject.pdf is not valid according to pdfcpu: %v", err)
		}
		t.Logf("image-xobject.pdf created: %d pages", ctx.PageCount)
	})

	// Signature fixtures: a real programmatically signed
	// adbe.pkcs7.detached PDF plus its byte-surgery variants and the unsigned
	// placeholder. Each must parse through pdfcpu's default validation.
	sigFixtures := []struct {
		name    string
		content func(t *testing.T) []byte
	}{
		{"signed.pdf", func(t *testing.T) []byte { return signedPDFContent(t, 0, false) }},
		{"signed-notcovers.pdf", func(t *testing.T) []byte { return signedPDFContent(t, 100, false) }},
		{"signed-badcontents.pdf", func(t *testing.T) []byte { return signedPDFContent(t, 0, true) }},
		{"unsigned-sig-field.pdf", func(t *testing.T) []byte { return unsignedSigFieldPDFContent() }},
	}
	for _, fx := range sigFixtures {
		t.Run(fx.name, func(t *testing.T) {
			if _, err := os.Stat(fx.name); err == nil {
				t.Skip(fx.name + " already exists")
			}
			if err := os.WriteFile(fx.name, fx.content(t), 0644); err != nil {
				t.Fatalf("failed to create %s: %v", fx.name, err)
			}
			ctx, err := pdfcpu_api.ReadContextFile(fx.name)
			if err != nil {
				os.Remove(fx.name)
				t.Fatalf("%s is not valid according to pdfcpu: %v", fx.name, err)
			}
			t.Logf("%s created: %d pages", fx.name, ctx.PageCount)
		})
	}

	// Structural-compliance fixtures. Each must parse through
	// pdfcpu's default validation; pdfa-1b-clean.pdf is the veraPDF oracle
	// clean-case fixture.
	complFixtures := []struct {
		name    string
		content func() []byte
	}{
		{"non-embedded-font.pdf", nonEmbeddedFontPDFContent},
		{"no-output-intent.pdf", noOutputIntentPDFContent},
		{"untagged.pdf", untaggedPDFContent},
		{"tagged.pdf", taggedPDFContent},
		{"pdfa-1b-clean.pdf", pdfaCleanContent},
	}
	for _, fx := range complFixtures {
		t.Run(fx.name, func(t *testing.T) {
			if _, err := os.Stat(fx.name); err == nil {
				t.Skip(fx.name + " already exists")
			}
			if err := os.WriteFile(fx.name, fx.content(), 0644); err != nil {
				t.Fatalf("failed to create %s: %v", fx.name, err)
			}
			ctx, err := pdfcpu_api.ReadContextFile(fx.name)
			if err != nil {
				os.Remove(fx.name)
				t.Fatalf("%s is not valid according to pdfcpu: %v", fx.name, err)
			}
			t.Logf("%s created: %d pages", fx.name, ctx.PageCount)
		})
	}
}
