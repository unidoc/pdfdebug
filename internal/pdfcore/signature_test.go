// Co-located unit tests for digital-signature decomposition.
//
// These exercise the NEW pdfcore surface:
//
//	(ins *Inspector) GetSignatures(tabID string) (*SignatureList, error)
//	types SignatureField, SignatureList, CertInfo
//
// Mirrors the embedded_test.go playbook.
//
// Go field contract pinned here (JSON tags are pinned by the CLI acceptance
// suite in tests/signature-decomposition/):
//
//	SignatureList{ Signatures []SignatureField }
//	SignatureField{
//	  FieldName, SignatureRef, SubFilter, SigningTimeRaw, SigningTime string
//	  Signed bool
//	  Notes []string
//	  Certificates []CertInfo
//	  DecomposeError string
//	  CoversWholeFile bool; TrailingGap int64
//	  HoleMatchesContents bool; CoverageError string
//	}
//	CertInfo{ Subject, Issuer, Serial, NotBefore, NotAfter string }
//
// Fixtures are hand-rolled raw PDF bytes assembled via the existing
// assemblexref helper (resolve_ref_atdd_test.go) and opened through
// writeTempPDF, which uses the SAME Inspector.Open path the app uses. The
// /Contents payloads here are deliberately FAKE (non-CMS) hex -- enumeration
// and ByteRange coverage are decomposition-independent facts, and a
// DecomposeError on these fixtures is expected and ignored. Real-CMS
// decomposition is covered black-box by the acceptance suite, which generates
// a genuinely signed fixture. Layouts here are byte-identical (modulo the
// /Contents capacity) to fixtures validated against pdfcpu default validation
// during ATDD authoring.
//
// Naming: [Px].
package pdfcore

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- fixture builders --------------------------------------------------------

// sigUnitHexCap is the reserved fake /Contents hex capacity (32 bytes).
const sigUnitHexCap = 64

// sigUnitOpt controls the hand-rolled signature-fixture variants.
type sigUnitOpt struct {
	subFilter string // default adbe.pkcs7.detached
	shortfall int    // subtract from ByteRange[3] -> trailing byte gap
	holeShift int    // add to ByteRange[1] -> hole/Contents mismatch
	kids      bool   // parent/kids hierarchy with inherited /FT
	directV   bool   // /V is a direct inline dict on the field
	certHex   string // when non-empty, adds /Cert <certHex>
}

// sigUnitDict renders a signature dict with a fixed-width /ByteRange
// placeholder and a fake zero-filled /Contents; both spliced after layout.
func sigUnitDict(o sigUnitOpt) string {
	sub := o.subFilter
	if sub == "" {
		sub = "adbe.pkcs7.detached"
	}
	d := "<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter /" + sub +
		" /ByteRange [0000000000 0000000000 0000000000 0000000000]" +
		" /Contents <" + strings.Repeat("0", sigUnitHexCap) + ">" +
		" /M (D:20260101120000+00'00') /Name (ATDD Signer) /Reason (Unit) /Location (Testville)"
	if o.certHex != "" {
		d += " /Cert <" + o.certHex + ">"
	}
	d += " >>"
	return d
}

// sigUnitPDF builds a one-signature-field PDF per opts and splices the true
// ByteRange facts in place. Object map (default): 1 Catalog(+AcroForm),
// 2 Pages, 3 Page, 4 field widget, 5 sig dict.
func sigUnitPDF(t *testing.T, o sigUnitOpt) []byte {
	t.Helper()
	sig := sigUnitDict(o)
	var objs []string
	switch {
	case o.kids:
		objs = []string{
			"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] /SigFlags 3 >> >>\nendobj\n",
			"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
			"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Annots [6 0 R] >>\nendobj\n",
			"4 0 obj\n<< /FT /Sig /T (Parent) /Kids [6 0 R] >>\nendobj\n",
			"5 0 obj\n" + sig + "\nendobj\n",
			"6 0 obj\n<< /Type /Annot /Subtype /Widget /Parent 4 0 R /T (Child1) /Rect [0 0 0 0] /P 3 0 R /F 132 /V 5 0 R >>\nendobj\n",
		}
	case o.directV:
		objs = []string{
			"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] /SigFlags 3 >> >>\nendobj\n",
			"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
			"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Annots [4 0 R] >>\nendobj\n",
			"4 0 obj\n<< /Type /Annot /Subtype /Widget /FT /Sig /T (SigDirect) /Rect [0 0 0 0] /P 3 0 R /F 132 /V " + sig + " >>\nendobj\n",
		}
	default:
		objs = []string{
			"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] /SigFlags 3 >> >>\nendobj\n",
			"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
			"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Annots [4 0 R] >>\nendobj\n",
			"4 0 obj\n<< /Type /Annot /Subtype /Widget /FT /Sig /T (Sig1) /Rect [0 0 0 0] /P 3 0 R /F 132 /V 5 0 R >>\nendobj\n",
			"5 0 obj\n" + sig + "\nendobj\n",
		}
	}
	file := assemblexref(append([]string{"%PDF-1.7\n"}, objs...)...)

	ci := bytes.Index(file, []byte("/Contents <"))
	if ci < 0 {
		t.Fatal("sig fixture: no /Contents placeholder")
	}
	holeStart := ci + len("/Contents ")
	holeEnd := holeStart + 1 + sigUnitHexCap + 1 // '<' + hex + '>'

	a := 0
	b := holeStart + o.holeShift
	c := holeEnd
	d := len(file) - holeEnd - o.shortfall
	br := []byte("[" + pad10(a) + " " + pad10(b) + " " + pad10(c) + " " + pad10(d) + "]")
	bi := bytes.Index(file, []byte("/ByteRange ["))
	if bi < 0 {
		t.Fatal("sig fixture: no /ByteRange placeholder")
	}
	copy(file[bi+len("/ByteRange "):], br)

	// Fake non-CMS payload; trailing zeros exercise the padding trim path.
	copy(file[holeStart+1:], "DEADBEEF")
	return file
}

// sigUnitNoAcroFormPDF builds a plain document with no AcroForm at all.
func sigUnitNoAcroFormPDF() []byte {
	return assemblexref("%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
	)
}

// sigUnitUnsignedPDF builds one /FT /Sig field with NO /V.
func sigUnitUnsignedPDF() []byte {
	return assemblexref("%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] /SigFlags 3 >> >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Annots [4 0 R] >>\nendobj\n",
		"4 0 obj\n<< /Type /Annot /Subtype /Widget /FT /Sig /T (EmptySig) /Rect [0 0 0 0] /P 3 0 R /F 132 >>\nendobj\n",
	)
}

// sigUnitTwoUnsignedPDF builds two /V-less sig fields (SigA then SigB) to pin
// the deterministic /Fields walk order.
func sigUnitTwoUnsignedPDF() []byte {
	return assemblexref("%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R 5 0 R] /SigFlags 3 >> >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Annots [4 0 R 5 0 R] >>\nendobj\n",
		"4 0 obj\n<< /Type /Annot /Subtype /Widget /FT /Sig /T (SigA) /Rect [0 0 0 0] /P 3 0 R /F 132 >>\nendobj\n",
		"5 0 obj\n<< /Type /Annot /Subtype /Widget /FT /Sig /T (SigB) /Rect [0 0 0 0] /P 3 0 R /F 132 >>\nendobj\n",
	)
}

// sigUnitMalformedBRPDF builds a signed field with an odd-length /ByteRange.
func sigUnitMalformedBRPDF() []byte {
	return assemblexref("%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] /SigFlags 3 >> >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Annots [4 0 R] >>\nendobj\n",
		"4 0 obj\n<< /Type /Annot /Subtype /Widget /FT /Sig /T (BadBR) /Rect [0 0 0 0] /P 3 0 R /F 132 /V 5 0 R >>\nendobj\n",
		"5 0 obj\n<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter /adbe.pkcs7.detached /ByteRange [0 100 200] /Contents <00> >>\nendobj\n",
	)
}

var (
	sigUnitCertOnce sync.Once
	sigUnitCertVal  string
	sigUnitCertErr  error
)

// sigUnitCertHex returns (once per process) a self-signed cert DER hex for
// the adbe.x509.rsa_sha1 /Cert routing test.
func sigUnitCertHex(t *testing.T) string {
	t.Helper()
	sigUnitCertOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			sigUnitCertErr = err
			return
		}
		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(4242),
			Subject:      pkix.Name{CommonName: "ATDD Unit Cert", Organization: []string{"UniDoc ATDD"}},
			NotBefore:    time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			NotAfter:     time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
			KeyUsage:     x509.KeyUsageDigitalSignature,
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
		if err != nil {
			sigUnitCertErr = err
			return
		}
		sigUnitCertVal = hex.EncodeToString(der)
	})
	if sigUnitCertErr != nil {
		t.Fatalf("unit cert generation failed: %v", sigUnitCertErr)
	}
	return sigUnitCertVal
}

// oneSigField asserts the list holds exactly one signature and returns it.
func oneSigField(t *testing.T, list *SignatureList, err error) SignatureField {
	t.Helper()
	if err != nil {
		t.Fatalf("GetSignatures error: %v", err)
	}
	if list == nil || len(list.Signatures) != 1 {
		t.Fatalf("expected exactly 1 signature field, got %+v", list)
	}
	return list.Signatures[0]
}

// ---------------------------------------------------------------------------
// A /FT /Sig field reachable from /AcroForm /Fields is enumerated with its
// name, signed flag, /V ref, and /SubFilter.
// ---------------------------------------------------------------------------

func TestGetSignatures_EnumeratesSigField(t *testing.T) {
	ins, tabID := writeTempPDF(t, "signed.pdf", sigUnitPDF(t, sigUnitOpt{}))

	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	if f.FieldName != "Sig1" {
		t.Errorf("FieldName = %q, want Sig1", f.FieldName)
	}
	if !f.Signed {
		t.Errorf("Signed = false, want true")
	}
	if f.SignatureRef != "5 0 R" {
		t.Errorf("SignatureRef = %q, want \"5 0 R\"", f.SignatureRef)
	}
	if f.SubFilter != "adbe.pkcs7.detached" {
		t.Errorf("SubFilter = %q, want adbe.pkcs7.detached", f.SubFilter)
	}
}

// ---------------------------------------------------------------------------
// /FT is INHERITABLE -- a terminal widget with no own /FT that inherits /Sig
// via /Parent counts, and the fully qualified name joins the parent /T chain
// with ".".
// ---------------------------------------------------------------------------

func TestGetSignatures_InheritedFTAndFQName(t *testing.T) {
	ins, tabID := writeTempPDF(t, "signed-kids.pdf", sigUnitPDF(t, sigUnitOpt{kids: true}))

	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	if f.FieldName != "Parent.Child1" {
		t.Errorf("FieldName = %q, want Parent.Child1", f.FieldName)
	}
	if !f.Signed {
		t.Errorf("Signed = false, want true")
	}
}

// ---------------------------------------------------------------------------
// A /FT /Sig field with NO /V is listed as Signed=false with no
// decomposition and no error.
// ---------------------------------------------------------------------------

func TestGetSignatures_UnsignedPlaceholder(t *testing.T) {
	ins, tabID := writeTempPDF(t, "unsigned.pdf", sigUnitUnsignedPDF())

	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	if f.FieldName != "EmptySig" {
		t.Errorf("FieldName = %q, want EmptySig", f.FieldName)
	}
	if f.Signed {
		t.Errorf("Signed = true for a /V-less placeholder, want false")
	}
	if f.SignatureRef != "" {
		t.Errorf("SignatureRef = %q, want empty", f.SignatureRef)
	}
	if f.DecomposeError != "" {
		t.Errorf("DecomposeError = %q, want empty (no /V is not an error)", f.DecomposeError)
	}
}

// ---------------------------------------------------------------------------
// A DIRECT (inline dict) /V is legal -- listed with an empty SignatureRef,
// never dropped.
// ---------------------------------------------------------------------------

func TestGetSignatures_DirectVEmptyRef(t *testing.T) {
	ins, tabID := writeTempPDF(t, "signed-directv.pdf", sigUnitPDF(t, sigUnitOpt{directV: true}))

	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	if f.FieldName != "SigDirect" {
		t.Errorf("FieldName = %q, want SigDirect", f.FieldName)
	}
	if !f.Signed {
		t.Errorf("Signed = false, want true (a direct /V is still a /V)")
	}
	if f.SignatureRef != "" {
		t.Errorf("SignatureRef = %q for a direct /V, want empty", f.SignatureRef)
	}
}

// ---------------------------------------------------------------------------
// Output order is deterministic -- the document /Fields walk order.
// ---------------------------------------------------------------------------

func TestGetSignatures_DeterministicFieldsOrder(t *testing.T) {
	ins, tabID := writeTempPDF(t, "two-unsigned.pdf", sigUnitTwoUnsignedPDF())

	list, err := ins.GetSignatures(tabID)
	if err != nil {
		t.Fatalf("GetSignatures error: %v", err)
	}
	if len(list.Signatures) != 2 {
		t.Fatalf("expected 2 signature fields, got %d", len(list.Signatures))
	}
	if list.Signatures[0].FieldName != "SigA" || list.Signatures[1].FieldName != "SigB" {
		t.Errorf("order = [%q, %q], want [SigA, SigB] (/Fields walk order)",
			list.Signatures[0].FieldName, list.Signatures[1].FieldName)
	}
}

// ---------------------------------------------------------------------------
// A /ByteRange covering the whole file except the exact /Contents hole ->
// CoversWholeFile true, HoleMatchesContents true.
// ---------------------------------------------------------------------------

func TestGetSignatures_CoverageCoversWholeFile(t *testing.T) {
	ins, tabID := writeTempPDF(t, "signed.pdf", sigUnitPDF(t, sigUnitOpt{}))

	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	if !f.CoversWholeFile {
		t.Errorf("CoversWholeFile = false, want true")
	}
	if f.TrailingGap != 0 {
		t.Errorf("TrailingGap = %d, want 0", f.TrailingGap)
	}
	if !f.HoleMatchesContents {
		t.Errorf("HoleMatchesContents = false, want true")
	}
	if f.CoverageError != "" {
		t.Errorf("CoverageError = %q, want empty", f.CoverageError)
	}
}

// ---------------------------------------------------------------------------
// A range stopping 100 bytes short of EOF (the earlier-revision case)
// -> CoversWholeFile false with the trailing gap reported as a fact.
// ---------------------------------------------------------------------------

func TestGetSignatures_CoverageTrailingGap(t *testing.T) {
	ins, tabID := writeTempPDF(t, "signed-short.pdf", sigUnitPDF(t, sigUnitOpt{shortfall: 100}))

	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	if f.CoversWholeFile {
		t.Errorf("CoversWholeFile = true, want false")
	}
	if f.TrailingGap != 100 {
		t.Errorf("TrailingGap = %d, want 100", f.TrailingGap)
	}
	if !f.HoleMatchesContents {
		t.Errorf("HoleMatchesContents = false, want true (only the tail is short)")
	}
}

// ---------------------------------------------------------------------------
// An excluded span that does not exactly equal the /Contents extent ->
// HoleMatchesContents false (a distinct structural fact).
// ---------------------------------------------------------------------------

func TestGetSignatures_CoverageHoleMismatch(t *testing.T) {
	ins, tabID := writeTempPDF(t, "signed-shift.pdf", sigUnitPDF(t, sigUnitOpt{holeShift: 4}))

	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	if f.HoleMatchesContents {
		t.Errorf("HoleMatchesContents = true, want false (hole shifted +4)")
	}
}

// ---------------------------------------------------------------------------
// A malformed /ByteRange (odd-length) degrades to a per-signature
// CoverageError -- the list call still succeeds.
// ---------------------------------------------------------------------------

func TestGetSignatures_MalformedByteRangeDegrades(t *testing.T) {
	ins, tabID := writeTempPDF(t, "bad-br.pdf", sigUnitMalformedBRPDF())

	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	if f.FieldName != "BadBR" {
		t.Errorf("FieldName = %q, want BadBR", f.FieldName)
	}
	if f.CoverageError == "" {
		t.Errorf("CoverageError empty, want the malformed-/ByteRange fact")
	}
}

// ---------------------------------------------------------------------------
// A document with NO AcroForm yields an empty list and no error.
// ---------------------------------------------------------------------------

func TestGetSignatures_NoAcroFormEmptyList(t *testing.T) {
	ins, tabID := writeTempPDF(t, "plain.pdf", sigUnitNoAcroFormPDF())

	list, err := ins.GetSignatures(tabID)
	if err != nil {
		t.Fatalf("GetSignatures error: %v (want empty list, nil error)", err)
	}
	if list == nil || len(list.Signatures) != 0 {
		t.Errorf("expected empty signature list, got %+v", list)
	}
}

// ---------------------------------------------------------------------------
// A non-CMS /Contents blob yields a per-signature DecomposeError -- the field
// is still listed with its structural facts.
// ---------------------------------------------------------------------------

func TestGetSignatures_BadContentsPerSignatureError(t *testing.T) {
	ins, tabID := writeTempPDF(t, "signed.pdf", sigUnitPDF(t, sigUnitOpt{}))

	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	if f.DecomposeError == "" {
		t.Errorf("DecomposeError empty, want the CMS parse failure (fixture /Contents is fake hex)")
	}
	if f.FieldName != "Sig1" || !f.Signed {
		t.Errorf("structural facts must survive the parse failure, got %+v", f)
	}
}

// ---------------------------------------------------------------------------
// /M surfaces both the raw PDF date string and the parsed ISO 8601 form.
// ---------------------------------------------------------------------------

func TestGetSignatures_SigningTimeRawAndISO(t *testing.T) {
	ins, tabID := writeTempPDF(t, "signed.pdf", sigUnitPDF(t, sigUnitOpt{}))

	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	if f.SigningTimeRaw != "D:20260101120000+00'00'" {
		t.Errorf("SigningTimeRaw = %q, want the raw D: string", f.SigningTimeRaw)
	}
	if !strings.HasPrefix(f.SigningTime, "2026-01-01T12:00:00") {
		t.Errorf("SigningTime = %q, want ISO 8601 2026-01-01T12:00:00...", f.SigningTime)
	}
}

// ---------------------------------------------------------------------------
// adbe.x509.rsa_sha1 -- /Contents is PKCS#1, so the cert chain is read from
// the field's /Cert entry instead.
// ---------------------------------------------------------------------------

func TestGetSignatures_X509RSASHA1CertFromCertEntry(t *testing.T) {
	ins, tabID := writeTempPDF(t, "signed-x509.pdf", sigUnitPDF(t, sigUnitOpt{
		subFilter: "adbe.x509.rsa_sha1", certHex: sigUnitCertHex(t)}))

	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	if len(f.Certificates) == 0 {
		t.Fatalf("Certificates empty, want the /Cert chain")
	}
	if !strings.Contains(f.Certificates[0].Subject, "ATDD Unit Cert") {
		t.Errorf("Certificates[0].Subject = %q, want the /Cert CN", f.Certificates[0].Subject)
	}
}

// ---------------------------------------------------------------------------
// An unknown SubFilter degrades to a labeled "not decomposed" note --
// never an error, never a crash.
// ---------------------------------------------------------------------------

func TestGetSignatures_UnknownSubFilterNote(t *testing.T) {
	ins, tabID := writeTempPDF(t, "signed-unknown.pdf", sigUnitPDF(t, sigUnitOpt{
		subFilter: "acme.custom.sig"}))

	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	if f.SubFilter != "acme.custom.sig" {
		t.Errorf("SubFilter = %q, want acme.custom.sig", f.SubFilter)
	}
	if !strings.Contains(strings.ToLower(strings.Join(f.Notes, " ")), "not decomposed") {
		t.Errorf("Notes = %v, want a \"subFilter not decomposed\" label", f.Notes)
	}
}

// sigUnitBRVariantPDF builds a signed field whose /Sig dict carries the given
// raw /ByteRange fragment verbatim (e.g. "/ByteRange [0 100 -50 100] "), plus a
// tiny fake /Contents. Passing an empty fragment omits /ByteRange entirely.
// Used for the malformed-/ByteRange degradation branches.
func sigUnitBRVariantPDF(byteRange string) []byte {
	return assemblexref("%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] /SigFlags 3 >> >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Annots [4 0 R] >>\nendobj\n",
		"4 0 obj\n<< /Type /Annot /Subtype /Widget /FT /Sig /T (BRVariant) /Rect [0 0 0 0] /P 3 0 R /F 132 /V 5 0 R >>\nendobj\n",
		"5 0 obj\n<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter /adbe.pkcs7.detached "+byteRange+"/Contents <00> >>\nendobj\n",
	)
}

// ---------------------------------------------------------------------------
// A /ByteRange with a non-integer entry (a real number) degrades to a
// per-signature CoverageError -- the list call succeeds.
// ---------------------------------------------------------------------------

func TestGetSignatures_ByteRangeNonIntegerDegrades(t *testing.T) {
	ins, tabID := writeTempPDF(t, "br-noninteger.pdf", sigUnitBRVariantPDF("/ByteRange [0 100 200 3.5] "))

	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	if f.CoverageError == "" {
		t.Errorf("CoverageError empty, want the non-integer /ByteRange fact")
	}
	if !strings.Contains(strings.ToLower(f.CoverageError), "non-integer") {
		t.Errorf("CoverageError = %q, want a non-integer fact", f.CoverageError)
	}
}

// ---------------------------------------------------------------------------
// A /ByteRange with a negative offset degrades to a per-signature
// CoverageError.
// ---------------------------------------------------------------------------

func TestGetSignatures_ByteRangeNegativeDegrades(t *testing.T) {
	ins, tabID := writeTempPDF(t, "br-negative.pdf", sigUnitBRVariantPDF("/ByteRange [0 100 -50 100] "))

	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	if f.CoverageError == "" {
		t.Errorf("CoverageError empty, want the negative-value /ByteRange fact")
	}
	if !strings.Contains(strings.ToLower(f.CoverageError), "negative") {
		t.Errorf("CoverageError = %q, want a negative-value fact", f.CoverageError)
	}
}

// ---------------------------------------------------------------------------
// Overlapping /ByteRange ranges (the second range starts inside the first)
// degrade to a per-signature CoverageError.
// ---------------------------------------------------------------------------

func TestGetSignatures_ByteRangeOverlapDegrades(t *testing.T) {
	ins, tabID := writeTempPDF(t, "br-overlap.pdf", sigUnitBRVariantPDF("/ByteRange [0 50 20 10] "))

	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	if f.CoverageError == "" {
		t.Errorf("CoverageError empty, want the overlapping-range fact")
	}
	if !strings.Contains(strings.ToLower(f.CoverageError), "overlap") {
		t.Errorf("CoverageError = %q, want an overlap fact", f.CoverageError)
	}
}

// ---------------------------------------------------------------------------
// A signed field whose /Sig dict has NO /ByteRange at all degrades to a
// per-signature CoverageError -- the field is still enumerated with its
// structural facts.
// ---------------------------------------------------------------------------

func TestGetSignatures_ByteRangeMissingDegrades(t *testing.T) {
	ins, tabID := writeTempPDF(t, "br-missing.pdf", sigUnitBRVariantPDF(""))

	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	if !f.Signed {
		t.Errorf("Signed = false, want true (a /V exists)")
	}
	if f.CoverageError == "" {
		t.Errorf("CoverageError empty, want the missing-/ByteRange fact")
	}
	if !strings.Contains(strings.ToLower(f.CoverageError), "missing") {
		t.Errorf("CoverageError = %q, want a missing fact", f.CoverageError)
	}
}

// ---------------------------------------------------------------------------
// adbe.x509.rsa_sha1 whose /Cert entry holds an unreadable (non-DER) blob
// degrades to a per-signature DecomposeError -- the field is still listed, no
// crash.
// ---------------------------------------------------------------------------

func TestGetSignatures_X509UnreadableCertDegrades(t *testing.T) {
	ins, tabID := writeTempPDF(t, "signed-x509-badcert.pdf", sigUnitPDF(t, sigUnitOpt{
		subFilter: "adbe.x509.rsa_sha1", certHex: "00"}))

	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	if len(f.Certificates) != 0 {
		t.Errorf("Certificates = %v, want none (the /Cert blob is unreadable)", f.Certificates)
	}
	if f.DecomposeError == "" {
		t.Errorf("DecomposeError empty, want the unreadable-/Cert fact")
	}
}
