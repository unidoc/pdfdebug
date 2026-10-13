package pdfcore

import (
	"bytes"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// CertInfo is the decomposed identity of one X.509 certificate: distinguished
// names, serial, and the validity window as RFC 3339 strings. It carries
// structural facts only - no trust evaluation of any kind.
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
// document-level pdfcpu failure. Runs under doc.pdfMu and inside safeCall.
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
		list.Signatures = collectSignatures(doc)
		return nil
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}
	return list, nil
}

// collectSignatures walks /AcroForm /Fields in document order. Caller MUST
// hold doc.pdfMu and run inside safeCall.
func collectSignatures(doc *DocumentState) []SignatureField {
	sigs := []SignatureField{}
	cat, err := doc.PDFContext.Catalog()
	if err != nil {
		return sigs
	}
	acro := asDict(dereference(doc, cat["AcroForm"]))
	if acro == nil {
		return sigs
	}
	for _, elem := range dereferenceArray(doc, acro["Fields"]) {
		walkSignatureField(doc, elem, "", "", 0, &sigs)
	}
	return sigs
}

// walkSignatureField recurses one field-tree node. /FT is inheritable via the
// kid walk (inheritedFT); a kid carrying its own /T is a child FIELD to
// recurse into, while /T-less kids are widget annotations of THIS field.
// Terminal fields whose effective /FT is Sig are appended to out.
func walkSignatureField(doc *DocumentState, obj pdfcpu_types.Object, parentName, inheritedFT string, depth int, out *[]SignatureField) {
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
	if t := stringValue(dereference(doc, d["T"])); t != "" {
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
			walkSignatureField(doc, kid, name, ft, depth+1, out)
			recursed = true
		}
	}
	if recursed || ft != "Sig" {
		return
	}
	*out = append(*out, buildSignatureField(doc, d, name, fieldNodeID))
}

// buildSignatureField distills one terminal signature field into its
// structural facts: /V resolution, dictionary entries, coverage measurement,
// and per-SubFilter decomposition routing.
func buildSignatureField(doc *DocumentState, field pdfcpu_types.Dict, name, fieldNodeID string) SignatureField {
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
	if raw := stringValue(dereference(doc, sig["M"])); raw != "" {
		f.SigningTimeRaw = raw
		if t, ok := pdfcpu_types.DateTime(raw, true); ok {
			f.SigningTime = t.Format(time.RFC3339)
		}
	}
	f.Name = stringValue(dereference(doc, sig["Name"]))
	f.Reason = stringValue(dereference(doc, sig["Reason"]))
	f.Location = stringValue(dereference(doc, sig["Location"]))
	f.ContactInfo = stringValue(dereference(doc, sig["ContactInfo"]))

	computeByteRangeCoverage(doc, sig, &f)

	switch {
	case isCMSSubFilter(f.SubFilter):
		if f.SubFilter == "ETSI.RFC3161" {
			// The token's own time lives in TSTInfo (out of scope); /M is
			// normally absent. Certs/algorithms still decompose as CMS.
			f.Type = "document-timestamp"
		}
		decomposeCMS(doc, sig, &f)
	case f.SubFilter == "adbe.x509.rsa_sha1":
		// /Contents is PKCS#1, NOT CMS: the chain lives in the /Cert entry.
		decomposeCertEntry(doc, sig, &f)
	default:
		f.Notes = append(f.Notes, "subFilter not decomposed")
	}
	return f
}

// isCMSSubFilter reports whether sub names a CMS-based signature whose
// /Contents parses as a ContentInfo(SignedData).
func isCMSSubFilter(sub string) bool {
	return strings.HasPrefix(sub, "adbe.pkcs7.") ||
		strings.HasPrefix(sub, "ETSI.CAdES") ||
		sub == "ETSI.RFC3161"
}

// --- CMS (PKCS#7 SignedData) minimal ASN.1 surface ---------------------------
//
// PKCS#7 is not a stdlib type; these are the minimal RFC 5652 structs needed
// to surface certificates, the signer identity, and the SignerInfo algorithms.
// encoding/asn1 is strict DER: BER indefinite-length blobs (some CAdES
// signers) degrade to the per-signature DecomposeError, which is acceptable
// decompose-and-display behavior.

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
// per-signature DecomposeError and leave the structural facts intact.
func decomposeCMS(doc *DocumentState, sig pdfcpu_types.Dict, f *SignatureField) {
	raw, err := pdfStringBytes(doc, sig["Contents"])
	if err != nil {
		f.DecomposeError = "/Contents unreadable: " + err.Error()
		return
	}
	sd, err := parseCMSSignedData(raw)
	if err != nil {
		f.DecomposeError = "CMS parse failed: " + err.Error()
		return
	}

	certs, err := x509.ParseCertificates(sd.Certificates.Bytes)
	if err != nil {
		f.DecomposeError = "certificate set parse failed: " + err.Error()
		return
	}
	for _, c := range certs {
		f.Certificates = append(f.Certificates, certInfoFrom(c))
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

// parseCMSSignedData unmarshals a ContentInfo(SignedData) DER blob. The
// /Contents value is zero-padded to its reserved size; DER is length-prefixed
// so asn1.Unmarshal reads exactly the SignedData and ignores the trailing
// padding. We deliberately do NOT bytes.TrimRight the 0x00 padding: that would
// corrupt a CMS blob whose DER legitimately ends in 0x00 (same class of bug as
// the /Cert path).
func parseCMSSignedData(raw []byte) (*cmsSignedData, error) {
	if len(raw) == 0 {
		return nil, errors.New("empty payload")
	}
	return unmarshalCMS(raw)
}

// unmarshalCMS parses one strict-DER ContentInfo(SignedData).
func unmarshalCMS(der []byte) (*cmsSignedData, error) {
	if len(der) == 0 {
		return nil, errors.New("empty payload")
	}
	var ci cmsContentInfo
	if _, err := asn1.Unmarshal(der, &ci); err != nil {
		return nil, err
	}
	if !ci.ContentType.Equal(oidCMSSignedData) {
		return nil, fmt.Errorf("not a CMS SignedData (content type %s)", ci.ContentType.String())
	}
	if len(ci.Content.Bytes) == 0 {
		return nil, errors.New("ContentInfo carries no content")
	}
	var sd cmsSignedData
	if _, err := asn1.Unmarshal(ci.Content.Bytes, &sd); err != nil {
		return nil, err
	}
	return &sd, nil
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
// certificate; the labeled note records the provenance.
func decomposeCertEntry(doc *DocumentState, sig pdfcpu_types.Dict, f *SignatureField) {
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

// certInfoFrom distills the display facts from a parsed certificate.
func certInfoFrom(c *x509.Certificate) CertInfo {
	return CertInfo{
		Subject:   c.Subject.String(),
		Issuer:    c.Issuer.String(),
		Serial:    c.SerialNumber.String(),
		NotBefore: c.NotBefore.UTC().Format(time.RFC3339),
		NotAfter:  c.NotAfter.UTC().Format(time.RFC3339),
	}
}

// pdfStringBytes renders a PDF string object as raw bytes: hex strings are
// whitespace-stripped and hex-decoded (odd length padded per the PDF spec),
// literal strings are taken verbatim.
func pdfStringBytes(doc *DocumentState, obj pdfcpu_types.Object) ([]byte, error) {
	switch v := dereference(doc, obj).(type) {
	case pdfcpu_types.HexLiteral:
		// pdfcpu already stripped whitespace and odd-padded at parse time, so
		// its Bytes() is the canonical decode.
		return v.Bytes()
	case pdfcpu_types.StringLiteral:
		return []byte(v), nil
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
func computeByteRangeCoverage(doc *DocumentState, sig pdfcpu_types.Dict, f *SignatureField) {
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
	f.HoleMatchesContents = holeMatchesContents(doc, sig, holeStart, start2)
}

// holeMatchesContents reads the excluded span from disk and reports whether it
// is exactly the /Contents hex string: '<' + the dict's hex payload + '>'.
func holeMatchesContents(doc *DocumentState, sig pdfcpu_types.Dict, holeStart, holeEnd int64) bool {
	hx, ok := dereference(doc, sig["Contents"]).(pdfcpu_types.HexLiteral)
	if !ok {
		return false
	}
	if holeEnd <= holeStart || holeEnd-holeStart > maxHoleProbeBytes {
		return false
	}
	span, err := readFileSpan(doc.FilePath, holeStart, holeEnd)
	if err != nil || len(span) < 2 {
		return false
	}
	if span[0] != '<' || span[len(span)-1] != '>' {
		return false
	}
	inner := stripPDFWhitespace(string(span[1 : len(span)-1]))
	want := stripPDFWhitespace(string(hx))
	return strings.EqualFold(inner, want)
}

// readFileSpan reads the byte range [start, end) from the file at path.
func readFileSpan(path string, start, end int64) ([]byte, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = fh.Close() }()
	buf := make([]byte, end-start)
	if _, err := fh.ReadAt(buf, start); err != nil {
		return nil, err
	}
	return buf, nil
}
