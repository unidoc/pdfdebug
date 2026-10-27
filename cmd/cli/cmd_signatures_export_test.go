package main

// Selection of the signature fields `dump signatures --export-cms` and
// `--export-certs` act on, and the certificate extension rows of the plain
// listing.
//
// Selection contract pinned here (pure functions over the listing):
//
//	selectCMSExport(sigs []pdfcore.SignatureField, field string) (int, error)
//	selectCertsExport(sigs []pdfcore.SignatureField, field string) ([]int, error)
//
// field is the --field value ("" when the flag is not given). The returned
// indexes are positions in sigs, so the 1-based field number used in export
// file names counts unsigned placeholders. The error text is the message the
// CLI prints after "error: ".

import (
	"bytes"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"unidoc-pdf-debugger/internal/pdfcore"
)

func cmsField(name string) pdfcore.SignatureField {
	return pdfcore.SignatureField{FieldName: name, Signed: true, SubFilter: "adbe.pkcs7.detached"}
}

func unsignedField(name string) pdfcore.SignatureField {
	return pdfcore.SignatureField{FieldName: name}
}

func subFilterField(name, subFilter string) pdfcore.SignatureField {
	return pdfcore.SignatureField{FieldName: name, Signed: true, SubFilter: subFilter}
}

// selectionMessage strips an optional leading "error: " so the check reads the
// message itself.
func selectionMessage(err error) string {
	return strings.TrimPrefix(err.Error(), "error: ")
}

func TestSelectCMSExport_Selects(t *testing.T) {
	cases := []struct {
		name  string
		sigs  []pdfcore.SignatureField
		field string
		want  int
	}{
		{"the one CMS field", []pdfcore.SignatureField{cmsField("Sig1")}, "", 0},
		{"the CMS field after an unsigned placeholder", []pdfcore.SignatureField{unsignedField("Empty"), cmsField("Sig1")}, "", 1},
		{"--field picks one of two CMS fields", []pdfcore.SignatureField{cmsField("Sig1"), cmsField("Sig2")}, "Sig2", 1},
		{"a document timestamp is a CMS field", []pdfcore.SignatureField{subFilterField("TS", "ETSI.RFC3161")}, "", 0},
		{"the one CMS field next to an rsa_sha1 field", []pdfcore.SignatureField{subFilterField("Old", "adbe.x509.rsa_sha1"), subFilterField("New", "ETSI.CAdES.detached")}, "", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectCMSExport(tc.sigs, tc.field)
			if err != nil {
				t.Fatalf("selectCMSExport error: %v", err)
			}
			if got != tc.want {
				t.Errorf("selected index %d, want %d", got, tc.want)
			}
		})
	}
}

func TestSelectCMSExport_Rejects(t *testing.T) {
	cases := []struct {
		name  string
		sigs  []pdfcore.SignatureField
		field string
		want  string // leading text of the message
		also  []string
	}{
		{"no signature fields", nil, "",
			"no CMS signature to export", nil},
		{"only an unsigned placeholder", []pdfcore.SignatureField{unsignedField("EmptySig")}, "",
			"no CMS signature to export", nil},
		{"only an rsa_sha1 field", []pdfcore.SignatureField{subFilterField("Old", "adbe.x509.rsa_sha1")}, "",
			"no CMS signature to export", nil},
		{"two CMS fields without --field", []pdfcore.SignatureField{cmsField("Sig1"), cmsField("Sig2")}, "",
			"2 signatures with a CMS SubFilter (Sig1, Sig2); pick one with --field NAME", nil},
		{"an unnamed CMS field is listed as a dash", []pdfcore.SignatureField{cmsField(""), cmsField("Sig2")}, "",
			"2 signatures with a CMS SubFilter (-, Sig2); pick one with --field NAME", nil},
		{"unknown name", []pdfcore.SignatureField{cmsField("Sig1")}, "Nope",
			`no signature field named "Nope"`, nil},
		{"name shared by two fields", []pdfcore.SignatureField{cmsField("Sig1"), cmsField("Sig1")}, "Sig1",
			`2 signature fields named "Sig1"`, nil},
		{"unsigned field", []pdfcore.SignatureField{unsignedField("EmptySig"), cmsField("Sig1")}, "EmptySig",
			`signature field "EmptySig" is not signed`, nil},
		{"rsa_sha1 field", []pdfcore.SignatureField{subFilterField("Old", "adbe.x509.rsa_sha1")}, "Old",
			`signature field "Old" has SubFilter adbe.x509.rsa_sha1, not CMS`, nil},
		{"unknown SubFilter", []pdfcore.SignatureField{subFilterField("Odd", "foo.bar")}, "Odd",
			`signature field "Odd" has SubFilter foo.bar, not CMS`, nil},
		{"non-ASCII name is escaped", []pdfcore.SignatureField{cmsField("Sig1")}, "Sign\u00e9",
			`no signature field named `, []string{`Sign`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectCMSExport(tc.sigs, tc.field)
			if err == nil {
				t.Fatalf("selectCMSExport selected %d, want an error", got)
			}
			msg := selectionMessage(err)
			if !strings.HasPrefix(msg, tc.want) {
				t.Errorf("error = %q, want it to start with %q", msg, tc.want)
			}
			for _, s := range tc.also {
				if !strings.Contains(msg, s) {
					t.Errorf("error = %q, want it to contain %q", msg, s)
				}
			}
			for i := 0; i < len(msg); i++ {
				if msg[i] > 0x7e {
					t.Fatalf("error has non-ASCII byte 0x%02x: %q", msg[i], msg)
				}
			}
		})
	}
}

func TestSelectCertsExport_Selects(t *testing.T) {
	mixed := []pdfcore.SignatureField{
		unsignedField("Empty"),
		cmsField("Sig1"),
		subFilterField("Old", "adbe.x509.rsa_sha1"),
		subFilterField("Odd", "foo.bar"),
	}
	cases := []struct {
		name  string
		sigs  []pdfcore.SignatureField
		field string
		want  []int
	}{
		{"every signed field without --field", mixed, "", []int{1, 2, 3}},
		{"--field naming an rsa_sha1 field", mixed, "Old", []int{2}},
		{"--field naming a CMS field", mixed, "Sig1", []int{1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectCertsExport(tc.sigs, tc.field)
			if err != nil {
				t.Fatalf("selectCertsExport error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("selected %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSelectCertsExport_Rejects(t *testing.T) {
	sigs := []pdfcore.SignatureField{unsignedField("EmptySig"), cmsField("Sig1"), cmsField("Dup"), cmsField("Dup")}
	cases := []struct {
		name  string
		field string
		want  string
	}{
		{"unknown name", "Nope", `no signature field named "Nope"`},
		{"name shared by two fields", "Dup", `2 signature fields named "Dup"`},
		{"unsigned field", "EmptySig", `signature field "EmptySig" is not signed`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectCertsExport(sigs, tc.field)
			if err == nil {
				t.Fatalf("selectCertsExport selected %v, want an error", got)
			}
			if msg := selectionMessage(err); !strings.HasPrefix(msg, tc.want) {
				t.Errorf("error = %q, want it to start with %q", msg, tc.want)
			}
		})
	}
}

// certRow returns the value of the row labelled label among the indented rows
// that follow the "  cert N:" row of the plain listing.
func certRow(t *testing.T, out string, n int, label string) string {
	t.Helper()
	lines := strings.Split(out, "\n")
	head := "  cert " + strconv.Itoa(n) + ":"
	for i, line := range lines {
		if !strings.HasPrefix(line, head) {
			continue
		}
		for _, next := range lines[i+1:] {
			if !strings.HasPrefix(next, "    ") {
				break
			}
			if rest, ok := strings.CutPrefix(strings.TrimSpace(next), label+":"); ok {
				return strings.TrimSpace(rest)
			}
		}
		t.Fatalf("no %q row under %q in:\n%s", label, head, out)
	}
	t.Fatalf("no %q row in:\n%s", head, out)
	return ""
}

func TestPrintSignaturesPlain_CertExtensionRows(t *testing.T) {
	full := pdfcore.CertInfo{
		Subject:  "CN=Leaf",
		Issuer:   "CN=CA",
		Serial:   "2",
		KeyUsage: []string{"digitalSignature", "nonRepudiation"},
		ExtKeyUsage: []pdfcore.ExtKeyUsage{
			{OID: "1.3.6.1.5.5.7.3.4", Name: "emailProtection"},
			{OID: "1.2.3.4.5", Name: ""},
		},
		CRLDistributionPoints: []string{"http://crl.example.test/a.crl", "ldap://dir.example.test/cn=CA"},
		OCSPServers:           []string{"http://ocsp.example.test"},
		CAIssuers:             []string{"http://ca.example.test/ca.cer"},
	}
	empty := pdfcore.CertInfo{
		Subject:               "CN=Intermediate",
		Issuer:                "CN=CA",
		Serial:                "3",
		KeyUsage:              []string{},
		ExtKeyUsage:           []pdfcore.ExtKeyUsage{},
		CRLDistributionPoints: []string{},
		OCSPServers:           []string{"http://ocsp.example.test"},
		CAIssuers:             []string{},
	}
	absent := pdfcore.CertInfo{Subject: "CN=CA", Issuer: "CN=CA", Serial: "1"}
	sig := pdfcore.SignatureField{
		FieldName:    "Sig1",
		Signed:       true,
		SubFilter:    "adbe.pkcs7.detached",
		Type:         "signature",
		Signer:       &full,
		Certificates: []pdfcore.CertInfo{full, empty, absent},
		Notes:        []string{},
	}

	var buf bytes.Buffer
	if err := printSignaturesPlain(&buf, []pdfcore.SignatureField{sig}); err != nil {
		t.Fatalf("printSignaturesPlain: %v", err)
	}
	out := buf.String()

	want := []struct {
		cert  int
		label string
		value string
	}{
		{1, "Key usage", "digitalSignature, nonRepudiation"},
		{1, "Ext key usage", "emailProtection (1.3.6.1.5.5.7.3.4), 1.2.3.4.5"},
		{1, "CRL DP", "http://crl.example.test/a.crl, ldap://dir.example.test/cn=CA"},
		{1, "OCSP", "http://ocsp.example.test"},
		{1, "CA Issuers", "http://ca.example.test/ca.cer"},
		{2, "Key usage", "present, no bits set"},
		{2, "CRL DP", "present, no URI"},
		{2, "OCSP", "http://ocsp.example.test"},
		{2, "CA Issuers", "present, no URI"},
		{3, "Key usage", "absent"},
		{3, "Ext key usage", "absent"},
		{3, "CRL DP", "absent"},
		{3, "OCSP", "absent"},
		{3, "CA Issuers", "absent"},
	}
	for _, w := range want {
		if got := certRow(t, out, w.cert, w.label); got != w.value {
			t.Errorf("cert %d %s = %q, want %q", w.cert, w.label, got, w.value)
		}
	}
}

func TestPrintSignaturesPlain_ExtensionURLsAreEscaped(t *testing.T) {
	c := pdfcore.CertInfo{
		Subject:               "CN=Leaf",
		Issuer:                "CN=CA",
		Serial:                "2",
		CRLDistributionPoints: []string{"http://crl.example.test/caf\u00e9.crl"},
	}
	sig := pdfcore.SignatureField{FieldName: "Sig1", Signed: true, SubFilter: "adbe.pkcs7.detached",
		Type: "signature", Certificates: []pdfcore.CertInfo{c}, Notes: []string{}}
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
	if got := certRow(t, out, 1, "CRL DP"); !strings.Contains(got, "http://crl.example.test/caf") {
		t.Errorf("CRL DP = %q, want the escaped URL", got)
	}
}

func TestSelectExport_UnknownFieldNameListsTheDocumentFields(t *testing.T) {
	cases := []struct {
		name string
		sigs []pdfcore.SignatureField
		want string
	}{
		{"no signature fields at all", nil, `no signature field named "Nope"`},
		{"unsigned and unnamed fields are listed too",
			[]pdfcore.SignatureField{unsignedField("EmptySig"), cmsField(""), cmsField("Sig1")},
			`no signature field named "Nope"; fields: EmptySig, -, Sig1`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := selectCMSExport(tc.sigs, "Nope"); err == nil || selectionMessage(err) != tc.want {
				t.Errorf("selectCMSExport error = %v, want %q", err, tc.want)
			}
			if _, err := selectCertsExport(tc.sigs, "Nope"); err == nil || selectionMessage(err) != tc.want {
				t.Errorf("selectCertsExport error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestPrintSignaturesPlain_EmptyExtKeyUsageReadsPresentNoOID(t *testing.T) {
	c := pdfcore.CertInfo{Subject: "CN=Leaf", Issuer: "CN=CA", Serial: "2", ExtKeyUsage: []pdfcore.ExtKeyUsage{}}
	sig := pdfcore.SignatureField{FieldName: "Sig1", Signed: true, SubFilter: "adbe.pkcs7.detached",
		Type: "signature", Certificates: []pdfcore.CertInfo{c}, Notes: []string{}}
	var buf bytes.Buffer
	if err := printSignaturesPlain(&buf, []pdfcore.SignatureField{sig}); err != nil {
		t.Fatalf("printSignaturesPlain: %v", err)
	}
	if got := certRow(t, buf.String(), 1, "Ext key usage"); got != "present, no OID" {
		t.Errorf("Ext key usage = %q, want %q", got, "present, no OID")
	}
}
