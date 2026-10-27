package pdfcore

import (
	"bytes"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// CertInfo is the decomposed identity of one X.509 certificate: distinguished
// names, serial, the validity window as RFC 3339 strings, and the key usage,
// extended key usage, CRL distribution point and AIA facts. It carries
// structural facts only - no trust evaluation and no network fetch of any
// kind. Each extension slice is nil when the extension is absent and non-nil
// (possibly empty) when it is present.
type CertInfo struct {
	// Subject is the certificate subject DN (e.g. "CN=...,O=...").
	Subject string `json:"subject"`
	// Issuer is the certificate issuer DN.
	Issuer string `json:"issuer"`
	// Serial is the decimal serial number.
	Serial string `json:"serial"`
	// NotBefore is the validity window start (RFC 3339, UTC).
	NotBefore string `json:"notBefore"`
	// NotAfter is the validity window end (RFC 3339, UTC).
	NotAfter string `json:"notAfter"`
	// KeyUsage lists the key usage bits set, by RFC 5280 name in bit order;
	// nil when the certificate has no key usage extension.
	KeyUsage []string `json:"keyUsage"`
	// ExtKeyUsage lists the extended key usage OIDs in certificate order;
	// nil when the extension is absent.
	ExtKeyUsage []ExtKeyUsage `json:"extKeyUsage"`
	// CRLDistributionPoints lists the URI distribution points; nil when the
	// extension is absent, empty when it has no URI entries.
	CRLDistributionPoints []string `json:"crlDistributionPoints"`
	// OCSPServers lists the AIA OCSP URIs; nil when the AIA extension is absent.
	OCSPServers []string `json:"ocspServers"`
	// CAIssuers lists the AIA CA Issuers URIs; nil when the AIA extension is
	// absent.
	CAIssuers []string `json:"caIssuers"`
}

// SignatureField is one signature field decomposed into structural facts:
// field identity, signature-dictionary entries, the PKCS#7/CMS certificate
// facts, and the /ByteRange coverage measurement. It NEVER carries a trust
// verdict - decompose-and-display only.
type SignatureField struct {
	// FieldName is the fully qualified field name (parent /T chain joined
	// with ".").
	FieldName string `json:"fieldName"`
	// Signed is false for a /FT /Sig placeholder field with no /V.
	Signed bool `json:"signed"`
	// SignatureRef is the "N G R" ref of the /V signature dict; empty when /V
	// is a direct (inline) dict.
	SignatureRef string `json:"signatureRef"`
	// SignatureNodeID is the obj:G:N tree-node id of the /V dict; empty when
	// /V is direct.
	SignatureNodeID string `json:"signatureNodeId"`
	// FieldNodeID is the obj:G:N tree-node id of the field object itself.
	FieldNodeID string `json:"fieldNodeId"`
	// SubFilter is the raw /SubFilter name (e.g. adbe.pkcs7.detached).
	SubFilter string `json:"subFilter"`
	// Type is "signature", or "document-timestamp" for ETSI.RFC3161 tokens.
	Type string `json:"type"`
	// SigningTimeRaw is the raw /M PDF date string.
	SigningTimeRaw string `json:"signingTimeRaw"`
	// SigningTime is the /M value parsed to RFC 3339; empty when unparseable.
	SigningTime string `json:"signingTime"`
	// Name is the /Name entry (the signer's human name) when present.
	Name string `json:"name"`
	// Reason is the /Reason entry when present.
	Reason string `json:"reason"`
	// Location is the /Location entry when present.
	Location string `json:"location"`
	// ContactInfo is the /ContactInfo entry when present.
	ContactInfo string `json:"contactInfo"`
	// DigestAlgorithm is read from SignerInfo (never guessed).
	DigestAlgorithm string `json:"digestAlgorithm"`
	// SignatureAlgorithm is read from SignerInfo (never guessed).
	SignatureAlgorithm string `json:"signatureAlgorithm"`
	// Signer is the IDENTIFIED signing certificate (matched from SignerInfo's
	// IssuerAndSerialNumber or SubjectKeyIdentifier against the embedded set);
	// nil when unsigned, not decomposed, or no cert matches.
	Signer *CertInfo `json:"signer"`
	// SignerIdentified is false when no embedded cert matches SignerInfo.
	SignerIdentified bool `json:"signerIdentified"`
	// Certificates is the full embedded certificate set (unordered per CMS).
	Certificates []CertInfo `json:"certificates"`
	// Notes carries labeled facts, including the mandatory trust note.
	Notes []string `json:"notes"`
	// DecomposeError is the per-signature CMS parse failure; the field is
	// still listed with its structural facts.
	DecomposeError string `json:"decomposeError"`
	// ByteRange is the raw /ByteRange integers (nil when absent/non-integer).
	ByteRange []int64 `json:"byteRange"`
	// CoversWholeFile reports whether /ByteRange covers the entire file except
	// the /Contents hole. A coverage MEASUREMENT, not a validity verdict.
	CoversWholeFile bool `json:"coversWholeFile"`
	// TrailingGap is the byte count past the signed range when the range stops
	// short of EOF (earlier-revision signatures legitimately do this).
	TrailingGap int64 `json:"trailingGap"`
	// HoleMatchesContents reports whether the excluded span exactly equals the
	// /Contents hex-string extent.
	HoleMatchesContents bool `json:"holeMatchesContents"`
	// CoverageError is the malformed-/ByteRange degradation note.
	CoverageError string `json:"coverageError"`
	// HoleCheckError is set when the hole could not be read from disk: the
	// ErrFileChanged or ErrFileBusy text. HoleMatchesContents is then false
	// and means "not checked"; every other field comes from the parse.
	HoleCheckError string `json:"holeCheckError"`
	// CMSEncoding describes the /Contents bytes as found: "DER", or "BER"
	// when they had to be normalized to DER before parsing (the parse runs on
	// a normalized copy; the PDF bytes are untouched). Empty for non-CMS
	// SubFilters and when the payload did not decode to a complete element.
	CMSEncoding string `json:"cmsEncoding"`
	// CMSTrailingBytes is set when non-zero bytes follow the CMS element in
	// /Contents, where only zero padding is expected; nil otherwise. The CMS
	// is decomposed regardless.
	CMSTrailingBytes *CMSTrailingBytes `json:"cmsTrailingBytes,omitempty"`
}

// CMSTrailingBytes locates non-zero bytes after the CMS element in a
// signature's decoded /Contents.
type CMSTrailingBytes struct {
	// Offset is the /Contents byte offset of the first byte after the CMS
	// element.
	Offset int `json:"offset"`
	// Count is the number of bytes from Offset through the last non-zero
	// byte; zero bytes after that are padding and not counted.
	Count int `json:"count"`
}

// SignatureList is the document-level enumeration of signature fields in
// /AcroForm /Fields walk order. Signatures is non-nil but may be empty (a
// document with no signature fields is a normal empty result, not an error).
type SignatureList struct {
	Signatures []SignatureField `json:"signatures"`
}

// trustNote is the mandatory non-verdict note attached to every signed
// entry. The output never claims "valid"/"trusted"/"verified".
const trustNote = "trust not verified - structural decomposition only"

// maxHoleProbeBytes bounds the on-disk /Contents-hole read for the
// hole-matches-Contents check so a malformed multi-GB hole claim cannot
// balloon memory.
const maxHoleProbeBytes = 4 * 1024 * 1024

// GetSignatures enumerates every signature field reachable from the AcroForm
// /Fields array (walking /Kids hierarchies with inherited /FT), decomposes
// each CMS-based /Contents into signer/chain/algorithm facts, and measures
// /ByteRange coverage against the file. Per-signature failures degrade to
// per-entry error fields; the call itself fails only for unknown tabs or a
// document-level pdfcpu failure. Only the hole check reads the file: a read
// refused because the file changed (ErrFileChanged) or is locked by another
// process (ErrFileBusy) sets HoleCheckError on that signature, and later
// signatures carry the same note without reading. Runs under doc.pdfMu and
// inside safeCall.
func (ins *Inspector) GetSignatures(tabID string) (*SignatureList, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	// Serialize pdfcpu access: the walk dereferences the AcroForm, fields,
	// kids, and each /V dict, all of which touch pdfcpu's XRefTable.
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	list := &SignatureList{Signatures: []SignatureField{}}
	err = safeCall(func() error {
		list.Signatures = collectSignatures(doc, nil)
		return nil
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}
	return list, nil
}

// SignatureBytes holds the raw bytes of one signature field for export,
// index-aligned with SignatureList.Signatures. It is all zero for an unsigned
// field and for a SubFilter that is neither CMS nor adbe.x509.rsa_sha1.
type SignatureBytes struct {
	// CMS is the decoded /Contents from offset 0 through the end of the
	// top-level element, in the encoding found (BER is not normalized). When
	// no element can be delimited it is every decoded byte and CMSUndelimited
	// is set. Nil for a non-CMS SubFilter or an unreadable /Contents.
	CMS []byte
	// CMSUndelimited is the reader error when /Contents holds no complete
	// element; empty otherwise.
	CMSUndelimited string
	// Certificates is the DER of each entry of SignatureField.Certificates, in
	// the same order: x509.Certificate.Raw for CMS, the /Cert element as read
	// for adbe.x509.rsa_sha1.
	Certificates [][]byte
}

// GetSignatureBytes returns what GetSignatures returns for the same document
// plus, index-aligned with its Signatures, the bytes each field can export.
// It walks the document once, under doc.pdfMu and inside safeCall.
func (ins *Inspector) GetSignatureBytes(tabID string) (*SignatureList, []SignatureBytes, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, nil, err
	}
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	list := &SignatureList{Signatures: []SignatureField{}}
	sink := []SignatureBytes{}
	err = safeCall(func() error {
		list.Signatures = collectSignatures(doc, &sink)
		return nil
	})
	if err != nil {
		return nil, nil, wrapPDFError(err)
	}
	return list, sink, nil
}

// holeRead carries the first refused hole read of one GetSignatures walk, so
// later signatures skip the read and carry the same note.
type holeRead struct {
	refused error
}

// collectSignatures walks /AcroForm /Fields in document order. When sink is
// non-nil it receives one SignatureBytes per listed field, in the same order.
// Caller MUST hold doc.pdfMu and run inside safeCall.
func collectSignatures(doc *DocumentState, sink *[]SignatureBytes) []SignatureField {
	sigs := []SignatureField{}
	cat, err := doc.PDFContext.Catalog()
	if err != nil {
		return sigs
	}
	acro := asDict(dereference(doc, cat["AcroForm"]))
	if acro == nil {
		return sigs
	}
	hr := &holeRead{}
	for _, elem := range dereferenceArray(doc, acro["Fields"]) {
		walkSignatureField(doc, hr, elem, "", "", 0, &sigs, sink)
	}
	return sigs
}

// walkSignatureField recurses one field-tree node. /FT is inheritable via the
// kid walk (inheritedFT); a kid carrying its own /T is a child FIELD to
// recurse into, while /T-less kids are widget annotations of THIS field.
// Terminal fields whose effective /FT is Sig are appended to out, and, when
// sink is non-nil, their export bytes to sink at the same index.
func walkSignatureField(doc *DocumentState, hr *holeRead, obj pdfcpu_types.Object, parentName, inheritedFT string, depth int, out *[]SignatureField, sink *[]SignatureBytes) {
	if depth > maxNodeIDDepth {
		return
	}
	fieldNodeID := ""
	if ref, ok := obj.(pdfcpu_types.IndirectRef); ok {
		fieldNodeID = nodeIDFromRef(ref)
	}
	d := asDict(dereference(doc, obj))
	if d == nil {
		return
	}

	name := parentName
	if t := textStringOrRaw(dereference(doc, d["T"])); t != "" {
		if name == "" {
			name = t
		} else {
			name = name + "." + t
		}
	}
	ft := inheritedFT
	if n, ok := dereference(doc, d["FT"]).(pdfcpu_types.Name); ok {
		ft = n.Value()
	}

	recursed := false
	for _, kid := range dereferenceArray(doc, d["Kids"]) {
		kd := asDict(dereference(doc, kid))
		if kd == nil {
			continue
		}
		if _, hasT := kd["T"]; hasT {
			walkSignatureField(doc, hr, kid, name, ft, depth+1, out, sink)
			recursed = true
		}
	}
	if recursed || ft != "Sig" {
		return
	}
	var sb *SignatureBytes
	if sink != nil {
		sb = &SignatureBytes{}
	}
	*out = append(*out, buildSignatureField(doc, hr, d, name, fieldNodeID, sb))
	if sink != nil {
		*sink = append(*sink, *sb)
	}
}

// buildSignatureField distills one terminal signature field into its
// structural facts: /V resolution, dictionary entries, coverage measurement,
// and per-SubFilter decomposition routing. sb, when non-nil, receives the
// field's export bytes.
func buildSignatureField(doc *DocumentState, hr *holeRead, field pdfcpu_types.Dict, name, fieldNodeID string, sb *SignatureBytes) SignatureField {
	f := SignatureField{
		FieldName:    name,
		FieldNodeID:  fieldNodeID,
		Notes:        []string{},
		Certificates: []CertInfo{},
	}

	vObj, hasV := field["V"]
	if !hasV {
		// An unsigned placeholder is listed with no decomposition and no
		// error.
		return f
	}
	var sig pdfcpu_types.Dict
	if ref, isRef := vObj.(pdfcpu_types.IndirectRef); isRef {
		sig = asDict(dereference(doc, vObj))
		if sig == nil {
			return f
		}
		f.SignatureRef = refString(ref)
		f.SignatureNodeID = nodeIDFromRef(ref)
	} else {
		// A direct (inline) /V dict is legal and must still decompose, with
		// the ref fields empty.
		sig = asDict(vObj)
		if sig == nil {
			return f
		}
	}

	f.Signed = true
	f.Type = "signature"
	f.Notes = append(f.Notes, trustNote)

	if n, ok := dereference(doc, sig["SubFilter"]).(pdfcpu_types.Name); ok {
		f.SubFilter = n.Value()
	}
	if raw := textStringOrRaw(dereference(doc, sig["M"])); raw != "" {
		f.SigningTimeRaw = raw
		if t, ok := pdfcpu_types.DateTime(raw, true); ok {
			f.SigningTime = t.Format(time.RFC3339)
		}
	}
	f.Name = textStringOrRaw(dereference(doc, sig["Name"]))
	f.Reason = textStringOrRaw(dereference(doc, sig["Reason"]))
	f.Location = textStringOrRaw(dereference(doc, sig["Location"]))
	f.ContactInfo = textStringOrRaw(dereference(doc, sig["ContactInfo"]))

	computeByteRangeCoverage(doc, hr, sig, &f)

	switch {
	case IsCMSSubFilter(f.SubFilter):
		if f.SubFilter == "ETSI.RFC3161" {
			// The token's own time lives in TSTInfo (out of scope); /M is
			// normally absent. Certs/algorithms still decompose as CMS.
			f.Type = "document-timestamp"
		}
		decomposeCMS(doc, sig, &f, sb)
	case f.SubFilter == "adbe.x509.rsa_sha1":
		// /Contents is PKCS#1, NOT CMS: the chain lives in the /Cert entry.
		decomposeCertEntry(doc, sig, &f, sb)
	default:
		f.Notes = append(f.Notes, "subFilter not decomposed")
	}
	return f
}

// IsCMSSubFilter reports whether sub names a CMS-based signature whose
// /Contents parses as a ContentInfo(SignedData).
func IsCMSSubFilter(sub string) bool {
	return strings.HasPrefix(sub, "adbe.pkcs7.") ||
		strings.HasPrefix(sub, "ETSI.CAdES") ||
		sub == "ETSI.RFC3161"
}

// --- CMS (PKCS#7 SignedData) minimal ASN.1 surface ---------------------------
//
// PKCS#7 is not a stdlib type; these are the minimal RFC 5652 structs needed
// to surface certificates, the signer identity, and the SignerInfo algorithms.
// encoding/asn1 is strict DER, so the parse runs on a DER-normalized copy of
// the /Contents bytes (normalizeBER). The copy never leaves decomposeCMS's
// call tree; the original bytes stay as read from the PDF.

// oidCMSSignedData is id-signedData (1.2.840.113549.1.7.2).
var oidCMSSignedData = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}

type cmsAlgID struct {
	Algorithm  asn1.ObjectIdentifier
	Parameters asn1.RawValue `asn1:"optional"`
}

type cmsIssuerAndSerial struct {
	Issuer asn1.RawValue
	Serial *big.Int
}

type cmsSignerInfo struct {
	Version            int
	SID                asn1.RawValue // CHOICE: IssuerAndSerialNumber SEQUENCE | [0] SubjectKeyIdentifier
	DigestAlgorithm    cmsAlgID
	SignedAttrs        asn1.RawValue `asn1:"optional,tag:0"`
	SignatureAlgorithm cmsAlgID
	Signature          []byte
	UnsignedAttrs      asn1.RawValue `asn1:"optional,tag:1"`
}

type cmsSignedData struct {
	Version          int
	DigestAlgorithms []cmsAlgID `asn1:"set"`
	EncapContentInfo asn1.RawValue
	Certificates     asn1.RawValue   `asn1:"optional,tag:0"`
	CRLs             asn1.RawValue   `asn1:"optional,tag:1"`
	SignerInfos      []cmsSignerInfo `asn1:"set"`
}

type cmsContentInfo struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue `asn1:"explicit,optional,tag:0"`
}

// cmsAlgorithmNames maps well-known digest/signature OIDs to display names.
// Unknown OIDs surface as the dotted OID string, never a guess.
var cmsAlgorithmNames = map[string]string{
	"1.3.14.3.2.26":          "SHA-1",
	"2.16.840.1.101.3.4.2.1": "SHA-256",
	"2.16.840.1.101.3.4.2.2": "SHA-384",
	"2.16.840.1.101.3.4.2.3": "SHA-512",
	"1.2.840.113549.1.1.1":   "RSA",
	"1.2.840.113549.1.1.5":   "RSA with SHA-1",
	"1.2.840.113549.1.1.10":  "RSA-PSS",
	"1.2.840.113549.1.1.11":  "RSA with SHA-256",
	"1.2.840.113549.1.1.12":  "RSA with SHA-384",
	"1.2.840.113549.1.1.13":  "RSA with SHA-512",
	"1.2.840.10045.2.1":      "ECDSA",
	"1.2.840.10045.4.3.2":    "ECDSA with SHA-256",
	"1.2.840.10045.4.3.3":    "ECDSA with SHA-384",
	"1.2.840.10045.4.3.4":    "ECDSA with SHA-512",
	"1.2.840.113549.1.1.2.1": "SHA-1", // legacy alias seen in the wild
	"1.2.840.113549.2.5":     "MD5",
}

// algorithmName renders an algorithm OID as its display name, falling back to
// the dotted OID.
func algorithmName(oid asn1.ObjectIdentifier) string {
	if n, ok := cmsAlgorithmNames[oid.String()]; ok {
		return n
	}
	return oid.String()
}

// decomposeCMS parses the /Contents CMS blob and surfaces the certificate set,
// the identified signer, and the SignerInfo algorithms. Failures set the
// per-signature DecomposeError and leave the structural facts intact. Non-zero
// bytes after the CMS set CMSTrailingBytes; bytes after the SignedData inside
// the ContentInfo become a note. Decomposition goes on in both cases. sb, when
// non-nil, receives the CMS bytes as found and each certificate's DER.
func decomposeCMS(doc *DocumentState, sig pdfcpu_types.Dict, f *SignatureField, sb *SignatureBytes) {
	raw, err := pdfStringBytes(doc, sig["Contents"])
	if err != nil {
		f.DecomposeError = "/Contents unreadable: " + err.Error()
		return
	}
	res, err := parseCMSSignedData(raw)
	if sb != nil {
		// A delimited payload is exported even when it is not a SignedData.
		if res.encoding == "" {
			sb.CMS = bytes.Clone(raw)
			if sb.CMS == nil {
				sb.CMS = []byte{}
			}
			sb.CMSUndelimited = err.Error()
		} else {
			sb.CMS = bytes.Clone(raw[:res.end])
		}
	}
	f.CMSEncoding = res.encoding
	if res.tail.count > 0 {
		f.CMSTrailingBytes = &CMSTrailingBytes{Offset: res.tail.offset, Count: res.tail.count}
	}
	if res.innerRest > 0 {
		f.Notes = append(f.Notes, fmt.Sprintf("%d bytes after the SignedData inside the ContentInfo", res.innerRest))
	}
	if err != nil {
		f.DecomposeError = "CMS parse failed: " + err.Error()
		return
	}
	sd := res.sd

	certs, err := x509.ParseCertificates(sd.Certificates.Bytes)
	if err != nil {
		f.DecomposeError = "certificate set parse failed: " + err.Error()
		return
	}
	for _, c := range certs {
		f.Certificates = append(f.Certificates, certInfoFrom(c))
		if sb != nil {
			sb.Certificates = append(sb.Certificates, bytes.Clone(c.Raw))
		}
	}

	if len(sd.SignerInfos) == 0 {
		f.Notes = append(f.Notes, "no SignerInfo in CMS SignedData")
		return
	}
	si := sd.SignerInfos[0]
	f.DigestAlgorithm = algorithmName(si.DigestAlgorithm.Algorithm)
	f.SignatureAlgorithm = algorithmName(si.SignatureAlgorithm.Algorithm)

	// The certificate set is unordered - the signer is IDENTIFIED from
	// SignerInfo, never assumed to be certificates[0].
	if signer := matchSignerCert(si, certs); signer != nil {
		ci := certInfoFrom(signer)
		f.Signer = &ci
		f.SignerIdentified = true
	} else {
		f.Notes = append(f.Notes, "signer certificate not present in the embedded set")
	}
}

// cmsParse is what parseCMSSignedData read from a /Contents payload.
type cmsParse struct {
	sd        *cmsSignedData
	encoding  string  // "DER", "BER", or "" when the payload did not decode
	end       int     // offset of the first byte after the CMS element
	tail      berTail // non-zero bytes after the CMS element
	innerRest int     // bytes after the SignedData inside the [0] wrapper
}

// parseCMSSignedData normalizes a BER or DER ContentInfo(SignedData) blob to
// DER and unmarshals it. The encoding, end and tail are filled in as soon as the
// normalizer succeeds, even when the CMS parse then fails. The /Contents value
// is zero-padded to its reserved size; bytes after the ContentInfo are never
// an error, and non-zero ones are reported in tail. The padding is never
// stripped with bytes.TrimRight, which would corrupt a CMS blob whose encoding
// legitimately ends in 0x00. raw is not modified.
func parseCMSSignedData(raw []byte) (cmsParse, error) {
	if len(raw) == 0 {
		return cmsParse{}, errors.New("empty payload")
	}
	der, isBER, end, tail, err := normalizeBER(raw)
	if err != nil {
		return cmsParse{}, err
	}
	res := cmsParse{encoding: "DER", end: end, tail: tail}
	if isBER {
		res.encoding = "BER"
	}
	res.sd, res.innerRest, err = unmarshalCMS(der)
	return res, err
}

// unmarshalCMS parses one strict-DER ContentInfo(SignedData) holding exactly
// one element, as normalizeBER returns it. innerRest is the number of bytes
// after the SignedData inside the ContentInfo's [0] wrapper; they are not an
// error.
func unmarshalCMS(der []byte) (sd *cmsSignedData, innerRest int, err error) {
	var ci cmsContentInfo
	if _, err := asn1.Unmarshal(der, &ci); err != nil {
		return nil, 0, err
	}
	if !ci.ContentType.Equal(oidCMSSignedData) {
		return nil, 0, fmt.Errorf("not a CMS SignedData (content type %s)", ci.ContentType.String())
	}
	if len(ci.Content.Bytes) == 0 {
		return nil, 0, errors.New("ContentInfo carries no content")
	}
	sd = &cmsSignedData{}
	rest, err := asn1.Unmarshal(ci.Content.Bytes, sd)
	if err != nil {
		return nil, 0, err
	}
	return sd, len(rest), nil
}

// matchSignerCert resolves SignerInfo's SignerIdentifier CHOICE against the
// embedded certificate set: IssuerAndSerialNumber (SEQUENCE) matches on raw
// issuer DER + serial; SubjectKeyIdentifier ([0]) matches on the cert's SKI.
func matchSignerCert(si cmsSignerInfo, certs []*x509.Certificate) *x509.Certificate {
	if si.SID.Class == asn1.ClassUniversal && si.SID.Tag == asn1.TagSequence {
		var ias cmsIssuerAndSerial
		if _, err := asn1.Unmarshal(si.SID.FullBytes, &ias); err == nil && ias.Serial != nil {
			for _, c := range certs {
				if c.SerialNumber.Cmp(ias.Serial) == 0 && bytes.Equal(c.RawIssuer, ias.Issuer.FullBytes) {
					return c
				}
			}
		}
		return nil
	}
	if si.SID.Class == asn1.ClassContextSpecific && si.SID.Tag == 0 {
		for _, c := range certs {
			if len(c.SubjectKeyId) > 0 && bytes.Equal(c.SubjectKeyId, si.SID.Bytes) {
				return c
			}
		}
	}
	return nil
}

// decomposeCertEntry reads the certificate chain from the signature dict's
// /Cert entry (a hex/literal string or an array of them) for the non-CMS
// adbe.x509.rsa_sha1 SubFilter. Per ISO 32000 the first element is the signing
// certificate; the labeled note records the provenance. sb, when non-nil,
// receives each readable certificate's bytes.
func decomposeCertEntry(doc *DocumentState, sig pdfcpu_types.Dict, f *SignatureField, sb *SignatureBytes) {
	f.Notes = append(f.Notes, "subFilter not CMS - certs read from /Cert")

	certObj := dereference(doc, sig["Cert"])
	var blobs []pdfcpu_types.Object
	if arr, ok := certObj.(pdfcpu_types.Array); ok {
		blobs = arr
	} else if certObj != nil {
		blobs = []pdfcpu_types.Object{certObj}
	}
	for _, b := range blobs {
		der, err := pdfStringBytes(doc, b)
		if err != nil {
			continue
		}
		// A DER certificate is a self-delimiting ASN.1 SEQUENCE. Slice to its
		// exact encoded length so any /Cert zero-padding is dropped without
		// corrupting a cert whose DER legitimately ends in 0x00 -- which a
		// bytes.TrimRight(der, "\x00") would eat, making ParseCertificate fail.
		var raw asn1.RawValue
		if _, err := asn1.Unmarshal(der, &raw); err != nil {
			continue
		}
		c, err := x509.ParseCertificate(raw.FullBytes)
		if err != nil {
			continue
		}
		f.Certificates = append(f.Certificates, certInfoFrom(c))
		if sb != nil {
			sb.Certificates = append(sb.Certificates, bytes.Clone(raw.FullBytes))
		}
	}
	if len(f.Certificates) == 0 {
		f.DecomposeError = "no certificate readable from the /Cert entry"
		return
	}
	// ISO 32000: /Cert[0] is defined as the signing certificate.
	signer := f.Certificates[0]
	f.Signer = &signer
	f.SignerIdentified = true
}

// pdfStringBytes renders a PDF string object as raw bytes: hex strings are
// whitespace-stripped and hex-decoded (odd length padded per the PDF spec),
// literal strings have each unescaped end-of-line (CR, LF or CRLF) read as
// LF and their backslash escapes resolved (ISO 32000-1 7.3.4.2).
func pdfStringBytes(doc *DocumentState, obj pdfcpu_types.Object) ([]byte, error) {
	switch v := dereference(doc, obj).(type) {
	case pdfcpu_types.HexLiteral:
		// pdfcpu already stripped whitespace and odd-padded at parse time, so
		// its Bytes() is the canonical decode.
		return v.Bytes()
	case pdfcpu_types.StringLiteral:
		// pdfcpu keeps a literal string as written, escapes and raw CRs
		// included. A backslash before a normalized EOL is still a line
		// continuation, so normalizing first keeps escapes intact.
		lit := strings.ReplaceAll(string(v), "\r\n", "\n")
		lit = strings.ReplaceAll(lit, "\r", "\n")
		return pdfcpu_types.Unescape(lit)
	case nil:
		return nil, errors.New("entry missing")
	default:
		return nil, errors.New("entry is not a string")
	}
}

// stripPDFWhitespace drops the PDF whitespace characters (incl. NUL) that are
// legal inside hex strings.
func stripPDFWhitespace(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case 0, '\t', '\n', '\f', '\r', ' ':
			return -1
		}
		return r
	}, s)
}

// --- /ByteRange coverage measurement -----------------------------------------

// computeByteRangeCoverage measures /ByteRange against the file: whole-file
// coverage, the trailing gap, and whether the excluded hole exactly equals the
// /Contents hex-string extent. Malformed arrays degrade to CoverageError. This
// is explicitly a measurement, never a validity verdict.
func computeByteRangeCoverage(doc *DocumentState, hr *holeRead, sig pdfcpu_types.Dict, f *SignatureField) {
	brObj, ok := sig["ByteRange"]
	if !ok {
		f.CoverageError = "/ByteRange missing"
		return
	}
	arr := dereferenceArray(doc, brObj)
	if arr == nil {
		f.CoverageError = "/ByteRange is not an array"
		return
	}
	ints := make([]int64, 0, len(arr))
	for _, e := range arr {
		n, isInt := dereference(doc, e).(pdfcpu_types.Integer)
		if !isInt {
			f.CoverageError = "/ByteRange contains a non-integer entry"
			return
		}
		ints = append(ints, int64(n))
	}
	f.ByteRange = ints
	if len(ints)%2 == 1 {
		f.CoverageError = fmt.Sprintf("/ByteRange has an odd number of entries (%d)", len(ints))
		return
	}
	if len(ints) != 4 {
		f.CoverageError = fmt.Sprintf("/ByteRange has %d entries; expected 4 (offset length offset length)", len(ints))
		return
	}
	size := doc.FileSize
	for _, n := range ints {
		if n < 0 {
			f.CoverageError = "/ByteRange contains a negative value"
			return
		}
		// Bound every entry by the file size BEFORE summing. A crafted value
		// (e.g. 2^62) would otherwise overflow the start+len sums below to a
		// negative int64, slip past the past-EOF guard, and drive a negative
		// make([]byte, ...) length in readFileSpan -> panic -> whole-document
		// failure. Bounding each entry keeps every sum in [0, 2*size].
		if n > size {
			f.CoverageError = "/ByteRange value exceeds the file size"
			return
		}
	}
	start1, len1, start2, len2 := ints[0], ints[1], ints[2], ints[3]
	if start1+len1 > size || start2+len2 > size {
		f.CoverageError = "/ByteRange extends past the end of the file"
		return
	}
	holeStart := start1 + len1
	if start2 < holeStart {
		f.CoverageError = "/ByteRange ranges overlap"
		return
	}
	f.TrailingGap = size - (start2 + len2)
	f.CoversWholeFile = start1 == 0 && f.TrailingGap == 0
	match, err := holeMatchesContents(doc, hr, sig, holeStart, start2)
	f.HoleMatchesContents = match
	if err != nil {
		f.HoleCheckError = err.Error()
	}
}

// holeMatchesContents reads the excluded span from disk and reports whether it
// is exactly the /Contents hex string: '<' + the dict's hex payload + '>'. A
// file that changed or is locked by another process returns ErrFileChanged or
// ErrFileBusy (the bare sentinel) and is recorded in hr, so later holes are
// not read; any other read failure is a mismatch.
func holeMatchesContents(doc *DocumentState, hr *holeRead, sig pdfcpu_types.Dict, holeStart, holeEnd int64) (bool, error) {
	hx, ok := dereference(doc, sig["Contents"]).(pdfcpu_types.HexLiteral)
	if !ok {
		return false, nil
	}
	if holeEnd <= holeStart || holeEnd-holeStart > maxHoleProbeBytes {
		return false, nil
	}
	if hr.refused != nil {
		return false, hr.refused
	}
	span, err := readFileSpan(doc.FilePath, doc.fingerprint, holeStart, holeEnd)
	switch {
	case errors.Is(err, ErrFileChanged):
		hr.refused = ErrFileChanged
		return false, hr.refused
	case errors.Is(err, ErrFileBusy):
		hr.refused = ErrFileBusy
		return false, hr.refused
	}
	if err != nil || len(span) < 2 {
		return false, nil
	}
	if span[0] != '<' || span[len(span)-1] != '>' {
		return false, nil
	}
	inner := stripPDFWhitespace(string(span[1 : len(span)-1]))
	want := stripPDFWhitespace(string(hx))
	return strings.EqualFold(inner, want), nil
}

// beforeFileSpanRead, when set, runs right before readFileSpan opens the file.
// Nil in production; tests use it to change the file during a walk.
var beforeFileSpanRead func(path string)

// readFileSpan reads the byte range [start, end) from the file at path. The
// open handle must match fp before and after the read; otherwise it returns
// ErrFileChanged. A read refused by another process's lock is ErrFileBusy.
func readFileSpan(path string, fp FileFingerprint, start, end int64) ([]byte, error) {
	if beforeFileSpanRead != nil {
		beforeFileSpanRead(path)
	}
	fh, err := openVerified(path, fp)
	if err != nil {
		return nil, err
	}
	defer func() { _ = fh.Close() }()
	buf := make([]byte, end-start)
	if _, err := fh.ReadAt(buf, start); err != nil {
		if isFileBusy(err) {
			return nil, fmt.Errorf("%w: %w", ErrFileBusy, err)
		}
		return nil, err
	}
	if err := verifyHandle(fh, fp); err != nil {
		return nil, err
	}
	return buf, nil
}
