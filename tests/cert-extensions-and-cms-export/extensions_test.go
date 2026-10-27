package cert_extensions_and_cms_export_test

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

const (
	leafSubject = "CN=Fixture Signer,O=UniDoc Fixtures"
	caSubject   = "CN=Fixture Root CA,O=UniDoc Fixtures"
)

// certIndex returns the index of the certificate with subject.
func certIndex(t *testing.T, certs []certInfo, subject string) int {
	t.Helper()
	for i, c := range certs {
		if c.Subject == subject {
			return i
		}
	}
	t.Fatalf("no certificate with subject %q in %+v", subject, certs)
	return -1
}

// assertRawNull fails unless m holds key with a JSON null value.
func assertRawNull(t *testing.T, where string, m map[string]json.RawMessage, key string) {
	t.Helper()
	v, ok := m[key]
	if !ok {
		t.Errorf("%s: key %q missing, want null", where, key)
		return
	}
	if string(v) != "null" {
		t.Errorf("%s: %q = %s, want null", where, key, v)
	}
}

func TestDumpSignaturesJSON_CertExtensionsFixtureListsExtensions(t *testing.T) {
	entries, raw, _ := dumpJSON(t, fixture(t, "signed-cert-extensions.pdf"))
	e := oneEntry(t, entries)
	if !e.Signed || e.DecomposeError != "" || len(e.Certificates) != 2 {
		t.Fatalf("entry = signed %v, decomposeError %q, %d certificates; want signed, no error, 2 certificates",
			e.Signed, e.DecomposeError, len(e.Certificates))
	}

	leaf := e.Certificates[certIndex(t, e.Certificates, leafSubject)]
	if want := []string{"digitalSignature", "nonRepudiation"}; !reflect.DeepEqual(leaf.KeyUsage, want) {
		t.Errorf("leaf keyUsage = %v, want %v", leaf.KeyUsage, want)
	}
	wantEKU := []extKeyUsage{
		{OID: "1.3.6.1.5.5.7.3.4", Name: "emailProtection"},
		{OID: "1.2.840.113583.1.1.5", Name: "adobeAuthenticDocumentsTrust"},
		{OID: "1.2.3.4.5", Name: ""},
	}
	if !reflect.DeepEqual(leaf.ExtKeyUsage, wantEKU) {
		t.Errorf("leaf extKeyUsage = %+v, want %+v", leaf.ExtKeyUsage, wantEKU)
	}
	if want := []string{"http://crl.example.test/fixture-ca.crl"}; !reflect.DeepEqual(leaf.CRLDistributionPoints, want) {
		t.Errorf("leaf crlDistributionPoints = %v, want %v", leaf.CRLDistributionPoints, want)
	}
	if want := []string{"http://ocsp.example.test"}; !reflect.DeepEqual(leaf.OCSPServers, want) {
		t.Errorf("leaf ocspServers = %v, want %v", leaf.OCSPServers, want)
	}
	if want := []string{"http://ca.example.test/fixture-ca.cer"}; !reflect.DeepEqual(leaf.CAIssuers, want) {
		t.Errorf("leaf caIssuers = %v, want %v", leaf.CAIssuers, want)
	}

	if e.Signer == nil || !reflect.DeepEqual(*e.Signer, leaf) {
		t.Errorf("signer = %+v, want the leaf entry %+v", e.Signer, leaf)
	}

	caIdx := certIndex(t, e.Certificates, caSubject)
	if want := []string{"keyCertSign"}; !reflect.DeepEqual(e.Certificates[caIdx].KeyUsage, want) {
		t.Errorf("CA keyUsage = %v, want %v", e.Certificates[caIdx].KeyUsage, want)
	}
	rawCA := oneEntry(t, raw).Certificates[caIdx]
	for _, k := range []string{"extKeyUsage", "crlDistributionPoints", "ocspServers", "caIssuers"} {
		assertRawNull(t, "CA certificate", rawCA, k)
	}
}

func TestDumpSignaturesJSON_ExistingFixtureHasNullForAbsentExtensions(t *testing.T) {
	entries, raw, _ := dumpJSON(t, fixture(t, "signed.pdf"))
	e := oneEntry(t, entries)
	r := oneEntry(t, raw)
	for subject, wantKU := range map[string][]string{caSubject: {"keyCertSign"}, leafSubject: {"digitalSignature"}} {
		i := certIndex(t, e.Certificates, subject)
		if !reflect.DeepEqual(e.Certificates[i].KeyUsage, wantKU) {
			t.Errorf("%s keyUsage = %v, want %v", subject, e.Certificates[i].KeyUsage, wantKU)
		}
		m := r.Certificates[i]
		for _, k := range []string{"extKeyUsage", "crlDistributionPoints", "ocspServers", "caIssuers"} {
			assertRawNull(t, subject, m, k)
		}
	}
	signer := r.Signer
	for _, k := range []string{"extKeyUsage", "crlDistributionPoints", "ocspServers", "caIssuers"} {
		assertRawNull(t, "signer", signer, k)
	}
}

func TestDumpSignaturesPlain_CertExtensionsFixtureShowsExtensionRows(t *testing.T) {
	path := fixture(t, "signed-cert-extensions.pdf")
	entries, _, _ := dumpJSON(t, path)
	e := oneEntry(t, entries)
	leafRow := "  cert " + strconv.Itoa(certIndex(t, e.Certificates, leafSubject)+1)
	caRow := "  cert " + strconv.Itoa(certIndex(t, e.Certificates, caSubject)+1)

	stdout, stderr, code := runCLI(t, "dump", "signatures", path)
	if code != 0 {
		t.Fatalf("dump signatures exit = %d, want 0\nstderr: %s", code, stderr)
	}
	if got := plainCertRow(t, stdout, leafRow, "Key usage"); got != "digitalSignature, nonRepudiation" {
		t.Errorf("leaf Key usage = %q, want %q", got, "digitalSignature, nonRepudiation")
	}
	eku := plainCertRow(t, stdout, leafRow, "Ext key usage")
	if !strings.Contains(eku, "emailProtection (1.3.6.1.5.5.7.3.4)") {
		t.Errorf("leaf Ext key usage = %q, want it to name emailProtection with its OID", eku)
	}
	if !strings.HasSuffix(eku, ", 1.2.3.4.5") {
		t.Errorf("leaf Ext key usage = %q, want the unknown OID last and bare", eku)
	}
	if got := plainCertRow(t, stdout, leafRow, "CRL DP"); got != "http://crl.example.test/fixture-ca.crl" {
		t.Errorf("leaf CRL DP = %q, want the fixture URL", got)
	}
	if got := plainCertRow(t, stdout, leafRow, "OCSP"); got != "http://ocsp.example.test" {
		t.Errorf("leaf OCSP = %q, want the fixture URL", got)
	}
	if got := plainCertRow(t, stdout, leafRow, "CA Issuers"); got != "http://ca.example.test/fixture-ca.cer" {
		t.Errorf("leaf CA Issuers = %q, want the fixture URL", got)
	}
	if got := plainCertRow(t, stdout, caRow, "Key usage"); got != "keyCertSign" {
		t.Errorf("CA Key usage = %q, want keyCertSign", got)
	}
	for _, label := range []string{"Ext key usage", "CRL DP", "OCSP", "CA Issuers"} {
		if got := plainCertRow(t, stdout, caRow, label); got != "absent" {
			t.Errorf("CA %s = %q, want absent", label, got)
		}
	}
}
