package main

import (
	"bytes"
	"strings"
	"testing"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// Decoded text strings and DNs with non-ASCII characters stay ASCII on the
// plain surface, and a single trailing byte reads "1 byte".
func TestPrintSignaturesPlain_NonASCIITextIsEscaped(t *testing.T) {
	cert := pdfcore.CertInfo{Subject: "CN=J\u00e9r\u00f4me,O=Ex", Issuer: "CN=Caf\u00e9 CA", Serial: "7"}
	sig := pdfcore.SignatureField{
		FieldName:        "Signature \u00e9",
		Signed:           true,
		SubFilter:        "adbe.pkcs7.detached",
		Type:             "signature",
		SigningTimeRaw:   "D:20260101120000Z",
		Name:             "J\u00e9r\u00f4me",
		Reason:           "R\u00e9vision",
		Location:         "Z\u00fcrich",
		ContactInfo:      "\u00e9@example.com",
		Signer:           &cert,
		Certificates:     []pdfcore.CertInfo{cert},
		CMSEncoding:      "DER",
		CMSTrailingBytes: &pdfcore.CMSTrailingBytes{Offset: 10, Count: 1},
		Notes:            []string{},
	}

	var buf bytes.Buffer
	if err := printSignaturesPlain(&buf, []pdfcore.SignatureField{sig}); err != nil {
		t.Fatalf("printSignaturesPlain: %v", err)
	}
	out := buf.String()
	for i := 0; i < len(out); i++ {
		if out[i] > 0x7e {
			t.Fatalf("non-ASCII byte 0x%02x at %d in:\n%s", out[i], i, out)
		}
	}
	for _, want := range []string{`"J\u00e9r\u00f4me"`, `"R\u00e9vision"`, `"Z\u00fcrich"`, `"Signature \u00e9"`, "1 byte after the CMS at offset 10 contains non-zero data"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}
