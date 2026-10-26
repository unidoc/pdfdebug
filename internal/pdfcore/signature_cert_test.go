// Unit tests for the certificate extension facts on CertInfo: key usage,
// extended key usage, CRL distribution points, and the AIA OCSP and CA
// Issuers URLs.
//
// Contract pinned here:
//
//	CertInfo{ ...; KeyUsage []string; ExtKeyUsage []ExtKeyUsage;
//	          CRLDistributionPoints, OCSPServers, CAIssuers []string }
//	ExtKeyUsage{ OID string `json:"oid"`; Name string `json:"name"` }
//
// A slice is nil when its extension is absent from the certificate and
// non-nil (possibly empty) when the extension is present. OCSPServers and
// CAIssuers share the AIA extension. Certificates are built in the test with
// a throwaway ECDSA key.
package pdfcore

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/json"
	"math/big"
	"reflect"
	"sync"
	"testing"
	"time"
)

var (
	certExtKeyOnce sync.Once
	certExtKey     *ecdsa.PrivateKey
	certExtKeyErr  error
)

// certExtTestCert creates a self-signed certificate from a template edited by
// edit and returns it parsed.
func certExtTestCert(t *testing.T, edit func(tmpl *x509.Certificate)) *x509.Certificate {
	t.Helper()
	certExtKeyOnce.Do(func() {
		certExtKey, certExtKeyErr = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	})
	if certExtKeyErr != nil {
		t.Fatalf("key generation failed: %v", certExtKeyErr)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(31),
		Subject:      pkix.Name{CommonName: "Extension Test", Organization: []string{"UniDoc Tests"}},
		NotBefore:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:     time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if edit != nil {
		edit(tmpl)
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &certExtKey.PublicKey, certExtKey)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return c
}

var (
	oidExtKeyUsage    = asn1.ObjectIdentifier{2, 5, 29, 15}
	oidExtExtKeyUsage = asn1.ObjectIdentifier{2, 5, 29, 37}
	oidExtCRLDP       = asn1.ObjectIdentifier{2, 5, 29, 31}
)

// crlDPDirectoryNameOnly encodes a CRL distribution points extension value
// whose only distribution point names a directoryName and no URI.
func crlDPDirectoryNameOnly(t *testing.T) []byte {
	t.Helper()
	name := mustMarshal(t, pkix.Name{CommonName: "CRL Issuer"}.ToRDNSequence())
	directoryName := mustMarshal(t, asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 4, IsCompound: true, Bytes: name})
	fullName := mustMarshal(t, asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: directoryName})
	dpName := mustMarshal(t, asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: fullName})
	dp := mustMarshal(t, asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSequence, IsCompound: true, Bytes: dpName})
	return mustMarshal(t, asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSequence, IsCompound: true, Bytes: dp})
}

// assertAbsent fails when s is not nil.
func assertAbsent[T any](t *testing.T, field string, s []T) {
	t.Helper()
	if s != nil {
		t.Errorf("%s = %#v, want nil (extension absent)", field, s)
	}
}

// assertPresentEmpty fails when s is nil or not empty.
func assertPresentEmpty[T any](t *testing.T, field string, s []T) {
	t.Helper()
	if s == nil || len(s) != 0 {
		t.Errorf("%s = %#v, want an empty non-nil slice (extension present, nothing to list)", field, s)
	}
}

func TestCertInfoFrom_NoExtensionsLeavesEveryFieldNil(t *testing.T) {
	got := certInfoFrom(certExtTestCert(t, nil))
	assertAbsent(t, "KeyUsage", got.KeyUsage)
	assertAbsent(t, "ExtKeyUsage", got.ExtKeyUsage)
	assertAbsent(t, "CRLDistributionPoints", got.CRLDistributionPoints)
	assertAbsent(t, "OCSPServers", got.OCSPServers)
	assertAbsent(t, "CAIssuers", got.CAIssuers)
	if got.Subject != "CN=Extension Test,O=UniDoc Tests" || got.Serial != "31" {
		t.Errorf("identity = %q / %q, want the template subject and serial", got.Subject, got.Serial)
	}
}

func TestCertInfoFrom_CRLDistributionPointWithoutURIIsPresentAndEmpty(t *testing.T) {
	c := certExtTestCert(t, func(tmpl *x509.Certificate) {
		tmpl.ExtraExtensions = []pkix.Extension{{Id: oidExtCRLDP, Value: crlDPDirectoryNameOnly(t)}}
	})
	got := certInfoFrom(c)
	assertPresentEmpty(t, "CRLDistributionPoints", got.CRLDistributionPoints)
	assertAbsent(t, "OCSPServers", got.OCSPServers)
	assertAbsent(t, "CAIssuers", got.CAIssuers)
}

func TestCertInfoFrom_TwoCRLDistributionPointsKeepTheirOrder(t *testing.T) {
	want := []string{"http://crl.example.test/a.crl", "ldap://directory.example.test/cn=CA?certificateRevocationList"}
	c := certExtTestCert(t, func(tmpl *x509.Certificate) {
		tmpl.CRLDistributionPoints = want
	})
	got := certInfoFrom(c)
	if !reflect.DeepEqual(got.CRLDistributionPoints, want) {
		t.Errorf("CRLDistributionPoints = %#v, want %#v", got.CRLDistributionPoints, want)
	}
}

func TestCertInfoFrom_AIAWithOnlyCAIssuers(t *testing.T) {
	c := certExtTestCert(t, func(tmpl *x509.Certificate) {
		tmpl.IssuingCertificateURL = []string{"http://ca.example.test/ca.cer"}
	})
	got := certInfoFrom(c)
	assertPresentEmpty(t, "OCSPServers", got.OCSPServers)
	if !reflect.DeepEqual(got.CAIssuers, []string{"http://ca.example.test/ca.cer"}) {
		t.Errorf("CAIssuers = %#v, want the one CA Issuers URL", got.CAIssuers)
	}
	assertAbsent(t, "CRLDistributionPoints", got.CRLDistributionPoints)
}

func TestCertInfoFrom_AIAWithOnlyOCSP(t *testing.T) {
	c := certExtTestCert(t, func(tmpl *x509.Certificate) {
		tmpl.OCSPServer = []string{"http://ocsp.example.test"}
	})
	got := certInfoFrom(c)
	if !reflect.DeepEqual(got.OCSPServers, []string{"http://ocsp.example.test"}) {
		t.Errorf("OCSPServers = %#v, want the one OCSP URL", got.OCSPServers)
	}
	assertPresentEmpty(t, "CAIssuers", got.CAIssuers)
}

func TestCertInfoFrom_URLSlicesAreCopies(t *testing.T) {
	c := certExtTestCert(t, func(tmpl *x509.Certificate) {
		tmpl.CRLDistributionPoints = []string{"http://crl.example.test/a.crl"}
		tmpl.OCSPServer = []string{"http://ocsp.example.test"}
		tmpl.IssuingCertificateURL = []string{"http://ca.example.test/ca.cer"}
	})
	got := certInfoFrom(c)
	c.CRLDistributionPoints[0] = "changed"
	c.OCSPServer[0] = "changed"
	c.IssuingCertificateURL[0] = "changed"
	if got.CRLDistributionPoints[0] != "http://crl.example.test/a.crl" ||
		got.OCSPServers[0] != "http://ocsp.example.test" ||
		got.CAIssuers[0] != "http://ca.example.test/ca.cer" {
		t.Errorf("CertInfo URL slices share storage with the x509.Certificate: %+v", got)
	}
}

func TestCertInfoFrom_EveryKeyUsageBitInBitOrder(t *testing.T) {
	c := certExtTestCert(t, func(tmpl *x509.Certificate) {
		tmpl.KeyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment |
			x509.KeyUsageKeyEncipherment | x509.KeyUsageDataEncipherment | x509.KeyUsageKeyAgreement |
			x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageEncipherOnly | x509.KeyUsageDecipherOnly
		tmpl.BasicConstraintsValid = true
		tmpl.IsCA = true
	})
	want := []string{"digitalSignature", "nonRepudiation", "keyEncipherment", "dataEncipherment",
		"keyAgreement", "keyCertSign", "cRLSign", "encipherOnly", "decipherOnly"}
	if got := certInfoFrom(c).KeyUsage; !reflect.DeepEqual(got, want) {
		t.Errorf("KeyUsage = %#v, want %#v", got, want)
	}
}

func TestCertInfoFrom_KeyUsageWithNoBitSetIsPresentAndEmpty(t *testing.T) {
	c := certExtTestCert(t, func(tmpl *x509.Certificate) {
		tmpl.ExtraExtensions = []pkix.Extension{{Id: oidExtKeyUsage, Critical: true, Value: []byte{0x03, 0x01, 0x00}}}
	})
	assertPresentEmpty(t, "KeyUsage", certInfoFrom(c).KeyUsage)
}

func TestCertInfoFrom_AnyExtendedKeyUsage(t *testing.T) {
	c := certExtTestCert(t, func(tmpl *x509.Certificate) {
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageAny}
	})
	want := []ExtKeyUsage{{OID: "2.5.29.37.0", Name: "anyExtendedKeyUsage"}}
	if got := certInfoFrom(c).ExtKeyUsage; !reflect.DeepEqual(got, want) {
		t.Errorf("ExtKeyUsage = %#v, want %#v", got, want)
	}
}

func TestCertInfoFrom_ExtKeyUsageKeepsCertificateOrderAndUnknownOIDs(t *testing.T) {
	value := mustMarshal(t, []asn1.ObjectIdentifier{
		{1, 2, 3, 4, 5},
		{1, 3, 6, 1, 5, 5, 7, 3, 1},
		{1, 2, 840, 113583, 1, 1, 5},
	})
	c := certExtTestCert(t, func(tmpl *x509.Certificate) {
		tmpl.ExtraExtensions = []pkix.Extension{{Id: oidExtExtKeyUsage, Value: value}}
	})
	want := []ExtKeyUsage{
		{OID: "1.2.3.4.5", Name: ""},
		{OID: "1.3.6.1.5.5.7.3.1", Name: "serverAuth"},
		{OID: "1.2.840.113583.1.1.5", Name: "adobeAuthenticDocumentsTrust"},
	}
	if got := certInfoFrom(c).ExtKeyUsage; !reflect.DeepEqual(got, want) {
		t.Errorf("ExtKeyUsage = %#v, want %#v", got, want)
	}
}

func TestCertInfoFrom_ExtKeyUsageNameTable(t *testing.T) {
	want := []ExtKeyUsage{
		{OID: "2.5.29.37.0", Name: "anyExtendedKeyUsage"},
		{OID: "1.3.6.1.5.5.7.3.1", Name: "serverAuth"},
		{OID: "1.3.6.1.5.5.7.3.2", Name: "clientAuth"},
		{OID: "1.3.6.1.5.5.7.3.3", Name: "codeSigning"},
		{OID: "1.3.6.1.5.5.7.3.4", Name: "emailProtection"},
		{OID: "1.3.6.1.5.5.7.3.5", Name: "ipsecEndSystem"},
		{OID: "1.3.6.1.5.5.7.3.6", Name: "ipsecTunnel"},
		{OID: "1.3.6.1.5.5.7.3.7", Name: "ipsecUser"},
		{OID: "1.3.6.1.5.5.7.3.8", Name: "timeStamping"},
		{OID: "1.3.6.1.5.5.7.3.9", Name: "OCSPSigning"},
		{OID: "1.3.6.1.5.5.7.3.36", Name: "documentSigning"},
		{OID: "1.2.840.113583.1.1.5", Name: "adobeAuthenticDocumentsTrust"},
		{OID: "1.3.6.1.4.1.311.10.3.12", Name: ""},
	}
	oids := []asn1.ObjectIdentifier{
		{2, 5, 29, 37, 0},
		{1, 3, 6, 1, 5, 5, 7, 3, 1},
		{1, 3, 6, 1, 5, 5, 7, 3, 2},
		{1, 3, 6, 1, 5, 5, 7, 3, 3},
		{1, 3, 6, 1, 5, 5, 7, 3, 4},
		{1, 3, 6, 1, 5, 5, 7, 3, 5},
		{1, 3, 6, 1, 5, 5, 7, 3, 6},
		{1, 3, 6, 1, 5, 5, 7, 3, 7},
		{1, 3, 6, 1, 5, 5, 7, 3, 8},
		{1, 3, 6, 1, 5, 5, 7, 3, 9},
		{1, 3, 6, 1, 5, 5, 7, 3, 36},
		{1, 2, 840, 113583, 1, 1, 5},
		{1, 3, 6, 1, 4, 1, 311, 10, 3, 12},
	}
	c := certExtTestCert(t, func(tmpl *x509.Certificate) {
		tmpl.ExtraExtensions = []pkix.Extension{{Id: oidExtExtKeyUsage, Value: mustMarshal(t, oids)}}
	})
	if got := certInfoFrom(c).ExtKeyUsage; !reflect.DeepEqual(got, want) {
		t.Errorf("ExtKeyUsage =\n%#v\nwant\n%#v", got, want)
	}
}

func TestCertInfo_JSONNullForAbsentAndEmptyArrayForPresent(t *testing.T) {
	keys := []string{"keyUsage", "extKeyUsage", "crlDistributionPoints", "ocspServers", "caIssuers"}

	absent, err := json.Marshal(CertInfo{Subject: "CN=x"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(absent, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			t.Errorf("key %q missing from %s", k, absent)
			continue
		}
		if string(v) != "null" {
			t.Errorf("%q = %s, want null", k, v)
		}
	}

	present, err := json.Marshal(CertInfo{
		Subject:               "CN=x",
		KeyUsage:              []string{},
		ExtKeyUsage:           []ExtKeyUsage{},
		CRLDistributionPoints: []string{},
		OCSPServers:           []string{},
		CAIssuers:             []string{},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	m = nil
	if err := json.Unmarshal(present, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range keys {
		if string(m[k]) != "[]" {
			t.Errorf("%q = %s, want []", k, m[k])
		}
	}

	eku, err := json.Marshal(ExtKeyUsage{OID: "1.2.3.4.5"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(eku) != `{"oid":"1.2.3.4.5","name":""}` {
		t.Errorf("ExtKeyUsage JSON = %s, want {\"oid\":\"1.2.3.4.5\",\"name\":\"\"}", eku)
	}
}

// findCert returns the certificate with the given subject from f.Certificates.
func findCert(t *testing.T, f SignatureField, subject string) CertInfo {
	t.Helper()
	for _, c := range f.Certificates {
		if c.Subject == subject {
			return c
		}
	}
	t.Fatalf("no certificate with subject %q in %+v", subject, f.Certificates)
	return CertInfo{}
}

const (
	fixtureLeafSubject = "CN=Fixture Signer,O=UniDoc Fixtures"
	fixtureCASubject   = "CN=Fixture Root CA,O=UniDoc Fixtures"
)

func TestGetSignatures_CertExtensionsFixtureShowsLeafExtensions(t *testing.T) {
	f := openSigFixture(t, "signed-cert-extensions.pdf")
	if f.DecomposeError != "" {
		t.Fatalf("DecomposeError = %q, want empty", f.DecomposeError)
	}
	if !f.Signed || len(f.Certificates) != 2 {
		t.Fatalf("Signed = %v, %d certificates; want a signed field with 2", f.Signed, len(f.Certificates))
	}

	leaf := findCert(t, f, fixtureLeafSubject)
	if want := []string{"digitalSignature", "nonRepudiation"}; !reflect.DeepEqual(leaf.KeyUsage, want) {
		t.Errorf("leaf KeyUsage = %#v, want %#v", leaf.KeyUsage, want)
	}
	wantEKU := []ExtKeyUsage{
		{OID: "1.3.6.1.5.5.7.3.4", Name: "emailProtection"},
		{OID: "1.2.840.113583.1.1.5", Name: "adobeAuthenticDocumentsTrust"},
		{OID: "1.2.3.4.5", Name: ""},
	}
	if !reflect.DeepEqual(leaf.ExtKeyUsage, wantEKU) {
		t.Errorf("leaf ExtKeyUsage = %#v, want %#v", leaf.ExtKeyUsage, wantEKU)
	}
	if want := []string{"http://crl.example.test/fixture-ca.crl"}; !reflect.DeepEqual(leaf.CRLDistributionPoints, want) {
		t.Errorf("leaf CRLDistributionPoints = %#v, want %#v", leaf.CRLDistributionPoints, want)
	}
	if want := []string{"http://ocsp.example.test"}; !reflect.DeepEqual(leaf.OCSPServers, want) {
		t.Errorf("leaf OCSPServers = %#v, want %#v", leaf.OCSPServers, want)
	}
	if want := []string{"http://ca.example.test/fixture-ca.cer"}; !reflect.DeepEqual(leaf.CAIssuers, want) {
		t.Errorf("leaf CAIssuers = %#v, want %#v", leaf.CAIssuers, want)
	}

	ca := findCert(t, f, fixtureCASubject)
	if want := []string{"keyCertSign"}; !reflect.DeepEqual(ca.KeyUsage, want) {
		t.Errorf("CA KeyUsage = %#v, want %#v", ca.KeyUsage, want)
	}
	assertAbsent(t, "CA ExtKeyUsage", ca.ExtKeyUsage)
	assertAbsent(t, "CA CRLDistributionPoints", ca.CRLDistributionPoints)
	assertAbsent(t, "CA OCSPServers", ca.OCSPServers)
	assertAbsent(t, "CA CAIssuers", ca.CAIssuers)

	if f.Signer == nil || !f.SignerIdentified {
		t.Fatalf("Signer = %+v, SignerIdentified = %v; want the identified leaf", f.Signer, f.SignerIdentified)
	}
	if !reflect.DeepEqual(*f.Signer, leaf) {
		t.Errorf("Signer = %#v\nwant the leaf entry %#v", *f.Signer, leaf)
	}
}

func TestGetSignatures_ExistingFixturesShowKeyUsageOnly(t *testing.T) {
	for _, name := range []string{"signed.pdf", "signed-ber.pdf", "signed-ber-detached.pdf"} {
		t.Run(name, func(t *testing.T) {
			f := openSigFixture(t, name)
			if f.DecomposeError != "" {
				t.Fatalf("DecomposeError = %q, want empty", f.DecomposeError)
			}
			for subject, wantKU := range map[string][]string{
				fixtureCASubject:   {"keyCertSign"},
				fixtureLeafSubject: {"digitalSignature"},
			} {
				c := findCert(t, f, subject)
				if !reflect.DeepEqual(c.KeyUsage, wantKU) {
					t.Errorf("%s KeyUsage = %#v, want %#v", subject, c.KeyUsage, wantKU)
				}
				assertAbsent(t, subject+" ExtKeyUsage", c.ExtKeyUsage)
				assertAbsent(t, subject+" CRLDistributionPoints", c.CRLDistributionPoints)
				assertAbsent(t, subject+" OCSPServers", c.OCSPServers)
				assertAbsent(t, subject+" CAIssuers", c.CAIssuers)
			}
			if f.Signer == nil || !reflect.DeepEqual(*f.Signer, findCert(t, f, fixtureLeafSubject)) {
				t.Errorf("Signer = %+v, want the leaf entry", f.Signer)
			}
		})
	}
}
