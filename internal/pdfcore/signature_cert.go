package pdfcore

import (
	"crypto/x509"
	"encoding/asn1"
	"encoding/json"
	"time"
)

// MarshalJSON encodes c with the default struct encoding. Declaring it makes
// the Wails binding generator hand CertInfo to the frontend as plain decoded
// JSON instead of a generated class whose constructor turns a null list into
// [], so the frontend still tells an absent extension (null) from a present
// one with nothing to list ([]).
func (c CertInfo) MarshalJSON() ([]byte, error) {
	type plain CertInfo
	return json.Marshal(plain(c))
}

// ExtKeyUsage is one entry of a certificate's extended key usage extension.
type ExtKeyUsage struct {
	// OID is the dotted key purpose OID.
	OID string `json:"oid"`
	// Name is the purpose name for a well-known OID; empty otherwise.
	Name string `json:"name"`
}

// Certificate extension OIDs whose presence decides nil versus empty.
var (
	oidCertKeyUsage    = asn1.ObjectIdentifier{2, 5, 29, 15}
	oidCertExtKeyUsage = asn1.ObjectIdentifier{2, 5, 29, 37}
	oidCertCRLDP       = asn1.ObjectIdentifier{2, 5, 29, 31}
	oidCertAIA         = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 1}
)

// keyUsageNames are the RFC 5280 key usage bit names, indexed by bit number.
var keyUsageNames = []string{
	"digitalSignature",
	"nonRepudiation",
	"keyEncipherment",
	"dataEncipherment",
	"keyAgreement",
	"keyCertSign",
	"cRLSign",
	"encipherOnly",
	"decipherOnly",
}

// extKeyUsageNames maps well-known key purpose OIDs to their names. Any other
// OID gets an empty name.
var extKeyUsageNames = map[string]string{
	"2.5.29.37.0":          "anyExtendedKeyUsage",
	"1.3.6.1.5.5.7.3.1":    "serverAuth",
	"1.3.6.1.5.5.7.3.2":    "clientAuth",
	"1.3.6.1.5.5.7.3.3":    "codeSigning",
	"1.3.6.1.5.5.7.3.4":    "emailProtection",
	"1.3.6.1.5.5.7.3.5":    "ipsecEndSystem",
	"1.3.6.1.5.5.7.3.6":    "ipsecTunnel",
	"1.3.6.1.5.5.7.3.7":    "ipsecUser",
	"1.3.6.1.5.5.7.3.8":    "timeStamping",
	"1.3.6.1.5.5.7.3.9":    "OCSPSigning",
	"1.3.6.1.5.5.7.3.36":   "documentSigning",
	"1.2.840.113583.1.1.5": "adobeAuthenticDocumentsTrust",
}

// certInfoFrom distills the display facts from a parsed certificate. Each
// extension slice is nil when its extension is absent and non-nil when it is
// present, even if it lists nothing.
func certInfoFrom(c *x509.Certificate) CertInfo {
	ci := CertInfo{
		Subject:   c.Subject.String(),
		Issuer:    c.Issuer.String(),
		Serial:    c.SerialNumber.String(),
		NotBefore: c.NotBefore.UTC().Format(time.RFC3339),
		NotAfter:  c.NotAfter.UTC().Format(time.RFC3339),
	}
	for _, ext := range c.Extensions {
		switch {
		case ext.Id.Equal(oidCertKeyUsage):
			ci.KeyUsage = []string{}
			for bit, name := range keyUsageNames {
				if c.KeyUsage&(1<<bit) != 0 {
					ci.KeyUsage = append(ci.KeyUsage, name)
				}
			}
		case ext.Id.Equal(oidCertExtKeyUsage):
			// Read from the extension so certificate order and unknown OIDs
			// are kept; crypto/x509 has already accepted this value.
			ci.ExtKeyUsage = []ExtKeyUsage{}
			var oids []asn1.ObjectIdentifier
			if _, err := asn1.Unmarshal(ext.Value, &oids); err == nil {
				for _, oid := range oids {
					s := oid.String()
					ci.ExtKeyUsage = append(ci.ExtKeyUsage, ExtKeyUsage{OID: s, Name: extKeyUsageNames[s]})
				}
			}
		case ext.Id.Equal(oidCertCRLDP):
			ci.CRLDistributionPoints = append([]string{}, c.CRLDistributionPoints...)
		case ext.Id.Equal(oidCertAIA):
			ci.OCSPServers = append([]string{}, c.OCSPServer...)
			ci.CAIssuers = append([]string{}, c.IssuingCertificateURL...)
		}
	}
	return ci
}
