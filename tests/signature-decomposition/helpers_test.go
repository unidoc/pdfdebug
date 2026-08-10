// Story 13-4 RED-PHASE acceptance test harness for the new CLI resource
// `dump signatures` (AC 1, 2, 3, 4, 5, 7).
//
// Black-box: build the pdfdebug CLI binary and run it as a subprocess. These
// tests assert the EXPECTED post-implementation behavior of the NEW resource.
// They MUST FAIL against the current binary (which has no `signatures`
// resource arm) until Story 13-4 is implemented. They fail at RUNTIME (unknown
// resource / wrong output shape / wrong exit code), not at compile time, so
// the main `unidoc-pdf-debugger` module keeps building green (mirrors 13-2 /
// 13-3).
//
// Test pyramid: every case here is a Go integration-level black-box test
// against the built CLI binary -- the project's established level for CLI
// acceptance. No browser/E2E layer is warranted; the CLI surface has no UI.
//
// Fixtures are generated programmatically per the story's AC8 fixture plan:
// a REAL `adbe.pkcs7.detached` signature -- self-signed CA + leaf cert via
// crypto/x509.CreateCertificate, CMS SignedData assembled over the actual
// ByteRange digest with encoding/asn1, RSA-signed for real -- spliced into a
// hand-rolled PDF whose ByteRange is computed against the true /Contents hole.
// Malformed / not-covers / hole-mismatch variants derive from the same
// builder. Every fixture was VALIDATED during ATDD authoring to parse through
// the current CLI's open path (pdfcpu default validation, the exact
// Inspector.Open route) and the CMS blob round-trips through strict stdlib
// DER parsing with non-positional signer identification (the CA cert is FIRST
// in the certificate set on purpose).
//
// JSON wire contract pinned by this suite (camelCase per the IPC rules):
//
//	dump signatures --json  =>  top-level ARRAY, one object per signature
//	field in document /Fields walk order:
//	  fieldName            string  fully qualified (parent /T chain, ".")
//	  signed               bool    false for /FT /Sig with no /V
//	  signatureRef         string  "N G R" of the /V dict; "" when direct
//	  signatureNodeId      string  "obj:G:N" of the /V dict; "" when direct
//	  fieldNodeId          string  "obj:G:N" of the field object
//	  subFilter            string  raw /SubFilter name
//	  type                 string  "signature" | "document-timestamp"
//	  signingTimeRaw       string  raw /M PDF date string
//	  signingTime          string  ISO 8601 when parseable, else ""
//	  name/reason/location/contactInfo  strings when present
//	  digestAlgorithm      string  from SignerInfo (never guessed)
//	  signatureAlgorithm   string  from SignerInfo
//	  signer               object  CertInfo of the IDENTIFIED signer cert
//	  signerIdentified     bool    false when no cert matches SignerInfo
//	  certificates         array   full embedded set, CertInfo each
//	  notes                array   labeled facts incl. the trust note
//	  decomposeError       string  per-signature CMS parse failure
//	  byteRange            array   raw /ByteRange integers
//	  coversWholeFile      bool    AC3 coverage fact
//	  trailingGap          number  bytes past the signed range when not covered
//	  holeMatchesContents  bool    excluded span == /Contents extent
//	  coverageError        string  malformed-/ByteRange degradation
//	CertInfo: { subject, issuer, serial, notBefore, notAfter } all strings.
//
// Naming: 13.4-INTG-NNN [Px] per the story Testing Requirements.
//
// Run: cd tests/13-4-signature-decomposition && go test -v -count=1 ./...
package signature_decomposition_test

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
	"encoding/json"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- CLI harness (mirrors tests/13-2 / 13-3) ---------------------------------

// projectRoot walks up from the test directory to find the main module's go.mod.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if content, err := os.ReadFile(goModPath); err == nil {
			if strings.Contains(string(content), "module unidoc-pdf-debugger") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root (no go.mod with module unidoc-pdf-debugger found)")
		}
		dir = parent
	}
}

// testdataDir returns the absolute path to the testdata/ directory at project root.
func testdataDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(projectRoot(t), "testdata")
}

var (
	cliBuildOnce sync.Once
	cliBinPath   string
	cliBuildErr  string
)

// buildCLI compiles the CLI binary once per test package and returns its path.
// The build is cached via sync.Once: the binary is identical for every test in
// the module, so the expensive `go build` runs a single time instead of once
// per test. The build dir is a plain os.MkdirTemp (not t.TempDir) because it is
// shared across tests and must outlive any single one; the OS reclaims it.
func buildCLI(t *testing.T) string {
	t.Helper()
	cliBuildOnce.Do(func() {
		root := projectRoot(t)
		binName := "pdfdebug"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		tmpDir, err := os.MkdirTemp("", "pdfdebug-cli-")
		if err != nil {
			cliBuildErr = "failed to create temp dir: " + err.Error()
			return
		}
		binPath := filepath.Join(tmpDir, binName)
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/cli/")
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			cliBuildErr = "failed to build CLI binary: " + err.Error() + "\n" + string(output)
			return
		}
		cliBinPath = binPath
	})
	if cliBuildErr != "" {
		t.Fatalf("%s", cliBuildErr)
	}
	return cliBinPath
}

// runCLI executes the CLI binary with args and returns stdout, stderr, exit code.
func runCLI(t *testing.T, binPath string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run CLI: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// mustParseJSON parses s as a single JSON value into target, failing on error.
func mustParseJSON(t *testing.T, s string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(s), target); err != nil {
		t.Fatalf("failed to parse JSON: %v\nraw: %s", err, s)
	}
}

// parsesAsJSON reports whether s (trimmed) is a single well-formed JSON value.
func parsesAsJSON(s string) bool {
	var v any
	return json.Unmarshal([]byte(strings.TrimSpace(s)), &v) == nil
}

// assertNotJSON fails when out is a top-level JSON object/array document. The
// plain-text default must NOT parse as a JSON document (13-1 contract).
func assertNotJSON(t *testing.T, id, out string) {
	t.Helper()
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return
	}
	if (trimmed[0] == '{' || trimmed[0] == '[') && parsesAsJSON(trimmed) {
		t.Fatalf("[%s] default output parsed as a JSON document; expected plain text:\n%s", id, out)
	}
}

// assertASCII fails when out contains a non-ASCII byte (13-1 plain-text contract).
func assertASCII(t *testing.T, id, out string) {
	t.Helper()
	for i := 0; i < len(out); i++ {
		if out[i] > 0x7f {
			t.Errorf("[%s] plain-text output contains non-ASCII byte 0x%02x at offset %d", id, out[i], i)
			return
		}
	}
}

// assertTrailingNewline fails when out does not end in a newline (13-1 contract).
func assertTrailingNewline(t *testing.T, id, out string) {
	t.Helper()
	if out == "" {
		return
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("[%s] plain-text output does not end with a trailing newline", id)
	}
}

// assertNoTrustClaims enforces AC4: the output never makes a trust claim. The
// words valid/trusted/verified may appear ONLY in negated or factual forms:
// "not valid"/"invalid"/"validity", "not trusted"/"untrusted",
// "not verified"/"unverified". Anything else is a trust verdict and fails.
func assertNoTrustClaims(t *testing.T, id, out string) {
	t.Helper()
	lower := strings.ToLower(out)
	for _, w := range []string{"valid", "trusted", "verified"} {
		for i := 0; ; {
			j := strings.Index(lower[i:], w)
			if j < 0 {
				break
			}
			pos := i + j
			if !trustWordAllowed(lower, pos, w) {
				t.Errorf("[%s] AC4 trust-claim language %q at offset %d (context: %q)",
					id, w, pos, snippet(out, pos))
				break
			}
			i = pos + len(w)
		}
	}
}

// trustWordAllowed reports whether the trust word at pos is in an allowed
// negated/factual form.
func trustWordAllowed(lower string, pos int, w string) bool {
	for _, p := range []string{"not ", "not-", "un", "in"} {
		if pos >= len(p) && strings.HasSuffix(lower[:pos], p) {
			return true
		}
	}
	// "validity" (a cert-date fact, e.g. a validity window) is not a verdict.
	if w == "valid" && strings.HasPrefix(lower[pos:], "validity") {
		return true
	}
	return false
}

// snippet returns a short context window around pos for failure messages.
func snippet(s string, pos int) string {
	lo := pos - 20
	if lo < 0 {
		lo = 0
	}
	hi := pos + 30
	if hi > len(s) {
		hi = len(s)
	}
	return s[lo:hi]
}

// writeTempPDF writes content to a temp file and returns its path.
func writeTempPDF(t *testing.T, name string, content []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write temp pdf: %v", err)
	}
	return path
}

// sigArray parses `dump signatures --json` stdout as the contract's top-level
// array.
func sigArray(t *testing.T, id, stdout string) []map[string]any {
	t.Helper()
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" || trimmed[0] != '[' {
		t.Fatalf("[%s] --json must emit a top-level array, got:\n%s", id, stdout)
	}
	var arr []map[string]any
	mustParseJSON(t, stdout, &arr)
	return arr
}

// oneSig parses stdout and asserts exactly one signature entry, returning it.
func oneSig(t *testing.T, id, stdout string) map[string]any {
	t.Helper()
	arr := sigArray(t, id, stdout)
	if len(arr) != 1 {
		t.Fatalf("[%s] expected exactly 1 signature entry, got %d:\n%s", id, len(arr), stdout)
	}
	return arr[0]
}

// getStr returns m[key] as a string ("" when absent or not a string).
func getStr(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// getMap returns m[key] as a map (nil when absent or not an object).
func getMap(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	return v
}

// getBool returns m[key] as a bool (false when absent or not a bool).
func getBool(m map[string]any, key string) bool {
	b, _ := m[key].(bool)
	return b
}

// --- fixture generation (validated during ATDD authoring) --------------------
//
// One CA + one leaf signer cert are generated once per test process; each
// fixture assembles its own PDF around the shared PKI.

const (
	signerCN  = "ATDD Test Signer"
	issuerCN  = "ATDD Test Root CA"
	certOrg   = "UniDoc ATDD"
	sigSerial = 2026
	// contentsHexCap is the reserved /Contents hex capacity. The real DER is
	// ~2.5 KB; the remainder stays zero-padded -- exercising AC2's
	// trailing-zero-trim requirement on every decomposition.
	contentsHexCap = 6144
)

type algID struct {
	Algorithm  asn1.ObjectIdentifier
	Parameters asn1.RawValue `asn1:"optional"`
}

type issuerAndSerial struct {
	Issuer asn1.RawValue
	Serial *big.Int
}

type signerInfoT struct {
	Version         int
	IAS             issuerAndSerial
	DigestAlg       algID
	SigAlg          algID
	EncryptedDigest []byte
}

type encapContentInfo struct {
	ContentType asn1.ObjectIdentifier
}

type signedDataT struct {
	Version          int
	DigestAlgorithms []algID `asn1:"set"`
	ContentInfo      encapContentInfo
	Certificates     asn1.RawValue
	SignerInfos      []signerInfoT `asn1:"set"`
}

type contentInfoT struct {
	ContentType asn1.ObjectIdentifier
	// Content carries the [0] EXPLICIT wrapper pre-built as a RawValue --
	// encoding/asn1 does not apply explicit-tag options to RawValue fields.
	Content asn1.RawValue
}

var (
	oidSignedData    = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	oidData          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidSHA256        = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidRSAEncryption = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}
)

type testPKI struct {
	caDER, leafDER []byte
	leafCert       *x509.Certificate
	leafKey        *rsa.PrivateKey
}

var (
	pkiOnce sync.Once
	pkiVal  *testPKI
	pkiErr  error
)

// fixturePKI generates (once per process) a self-signed CA and a leaf signer
// cert with a fixed validity window the tests assert against.
func fixturePKI(t *testing.T) *testPKI {
	t.Helper()
	pkiOnce.Do(func() {
		pkiVal, pkiErr = newPKI()
	})
	if pkiErr != nil {
		t.Fatalf("fixture PKI generation failed: %v", pkiErr)
	}
	return pkiVal
}

func newPKI() (*testPKI, error) {
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	nb := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	na := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(77001),
		Subject:               pkix.Name{CommonName: issuerCN, Organization: []string{certOrg}},
		NotBefore:             nb,
		NotAfter:              na,
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return nil, err
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(sigSerial),
		Subject:      pkix.Name{CommonName: signerCN, Organization: []string{certOrg}},
		NotBefore:    nb,
		NotAfter:     na,
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	leafCert, err := x509.ParseCertificate(leafDER)
	if err != nil {
		return nil, err
	}
	return &testPKI{caDER: caDER, leafDER: leafDER, leafCert: leafCert, leafKey: leafKey}, nil
}

// cmsDER assembles a DER ContentInfo(SignedData) over digest, RSA-signed for
// real. The CA cert is FIRST in the certificate set so signer identification
// cannot be positional (AC2: never take certificates[0]).
func (p *testPKI) cmsDER(t *testing.T, digest []byte) []byte {
	t.Helper()
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.leafKey, crypto.SHA256, digest)
	if err != nil {
		t.Fatalf("fixture RSA sign: %v", err)
	}
	certsRaw := asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true,
		Bytes: append(append([]byte{}, p.caDER...), p.leafDER...)}
	sd := signedDataT{
		Version:          1,
		DigestAlgorithms: []algID{{Algorithm: oidSHA256, Parameters: asn1.NullRawValue}},
		ContentInfo:      encapContentInfo{ContentType: oidData},
		Certificates:     certsRaw,
		SignerInfos: []signerInfoT{{
			Version: 1,
			IAS: issuerAndSerial{
				Issuer: asn1.RawValue{FullBytes: p.leafCert.RawIssuer},
				Serial: p.leafCert.SerialNumber,
			},
			DigestAlg:       algID{Algorithm: oidSHA256, Parameters: asn1.NullRawValue},
			SigAlg:          algID{Algorithm: oidRSAEncryption, Parameters: asn1.NullRawValue},
			EncryptedDigest: sig,
		}},
	}
	sdDER, err := asn1.Marshal(sd)
	if err != nil {
		t.Fatalf("fixture SignedData marshal: %v", err)
	}
	ci := contentInfoT{ContentType: oidSignedData,
		Content: asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: 0, IsCompound: true, Bytes: sdDER}}
	ciDER, err := asn1.Marshal(ci)
	if err != nil {
		t.Fatalf("fixture ContentInfo marshal: %v", err)
	}
	return ciDER
}

func pad10(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 10 {
		s = "0" + s
	}
	return s
}

// assemblePDF stitches a header, object bodies (object i+1), an xref table,
// and a trailer with /Root 1 0 R.
func assemblePDF(objs []string) []byte {
	body := "%PDF-1.7\n"
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = len(body)
		body += o
	}
	xrefOff := len(body)
	size := len(objs) + 1
	xref := "xref\n0 " + strconv.Itoa(size) + "\n0000000000 65535 f \n"
	for _, off := range offsets {
		xref += pad10(off) + " 00000 n \n"
	}
	trailer := "trailer\n<< /Size " + strconv.Itoa(size) + " /Root 1 0 R >>\nstartxref\n" +
		strconv.Itoa(xrefOff) + "\n%%EOF\n"
	return []byte(body + xref + trailer)
}

// sigOpt controls the signed-fixture variants.
type sigOpt struct {
	subFilter   string // default adbe.pkcs7.detached
	directV     bool   // /V is a direct inline dict on the field
	shortfall   int    // subtract from ByteRange[3] -> trailing byte gap
	holeShift   int    // add to ByteRange[1] -> hole/Contents mismatch
	corrupt     bool   // corrupt the DER hex -> per-signature parse error
	omitM       bool   // no /M (doc-timestamp style)
	kids        bool   // parent/kids hierarchy with inherited /FT
	certEntry   bool   // add /Cert with the signer cert (adbe.x509.rsa_sha1)
	rawContents []byte // override /Contents payload (PKCS#1, not CMS)
}

// sigDictBody renders the signature dictionary with a fixed-width /ByteRange
// placeholder and a zero-filled /Contents of contentsHexCap hex chars; both
// are spliced in place after layout so offsets stay stable.
func sigDictBody(p *testPKI, o sigOpt) string {
	sub := o.subFilter
	if sub == "" {
		sub = "adbe.pkcs7.detached"
	}
	d := "<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter /" + sub +
		" /ByteRange [0000000000 0000000000 0000000000 0000000000]" +
		" /Contents <" + strings.Repeat("0", contentsHexCap) + ">"
	if !o.omitM {
		d += " /M (D:20260101120000+00'00')"
	}
	d += " /Name (ATDD Signer) /Reason (Acceptance testing) /Location (Testville) /ContactInfo (atdd@example.com)"
	if o.certEntry {
		d += " /Cert <" + hex.EncodeToString(p.leafDER) + ">"
	}
	d += " >>"
	return d
}

// signedPDF builds a one-signature PDF per opts. Object map (default layout):
// 1 Catalog(+AcroForm), 2 Pages, 3 Page, 4 sig field widget, 5 sig dict.
func signedPDF(t *testing.T, p *testPKI, o sigOpt) []byte {
	t.Helper()
	var objs []string
	sig := sigDictBody(p, o)
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
	file := assemblePDF(objs)

	// Locate the /Contents hole (the < > delimited hex string, inclusive).
	ci := bytes.Index(file, []byte("/Contents <"))
	if ci < 0 {
		t.Fatal("fixture: no /Contents placeholder")
	}
	holeStart := ci + len("/Contents ")
	holeEnd := holeStart + 1 + contentsHexCap + 1 // '<' + hex + '>'

	// ByteRange facts, spliced fixed-width in place.
	a := 0
	b := holeStart + o.holeShift
	c := holeEnd
	d := len(file) - holeEnd - o.shortfall
	br := []byte("[" + pad10(a) + " " + pad10(b) + " " + pad10(c) + " " + pad10(d) + "]")
	bi := bytes.Index(file, []byte("/ByteRange ["))
	if bi < 0 {
		t.Fatal("fixture: no /ByteRange placeholder")
	}
	copy(file[bi+len("/ByteRange "):], br)

	// Digest over the two spans around the TRUE hole, then sign for real.
	h := sha256.New()
	h.Write(file[:holeStart])
	h.Write(file[holeEnd:])
	digest := h.Sum(nil)

	payload := o.rawContents
	if payload == nil {
		payload = p.cmsDER(t, digest)
	}
	hx := hex.EncodeToString(payload)
	if len(hx) > contentsHexCap {
		t.Fatalf("fixture: contents capacity exceeded: %d", len(hx))
	}
	if o.corrupt {
		// Break the leading DER tag/length bytes; keep valid hex so the PDF
		// itself still parses.
		copy(file[holeStart+1:], "FFFFFFFFFFFFFFFF")
		copy(file[holeStart+1+16:], hx[16:])
	} else {
		copy(file[holeStart+1:], hx)
	}
	// Remainder of the hole stays "0"-filled = trailing 0x00 zero padding.
	return file
}

// unsignedPlaceholderPDF builds a /FT /Sig field with NO /V (AC1: listed as
// signed:false, no decomposition, no error).
func unsignedPlaceholderPDF() []byte {
	return assemblePDF([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] /SigFlags 3 >> >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Annots [4 0 R] >>\nendobj\n",
		"4 0 obj\n<< /Type /Annot /Subtype /Widget /FT /Sig /T (EmptySig) /Rect [0 0 0 0] /P 3 0 R /F 132 >>\nendobj\n",
	})
}

// malformedByteRangePDF builds a signed field whose /ByteRange is odd-length
// (AC3: degrades to a per-signature coverage error, never a crash).
func malformedByteRangePDF() []byte {
	return assemblePDF([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] /SigFlags 3 >> >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Annots [4 0 R] >>\nendobj\n",
		"4 0 obj\n<< /Type /Annot /Subtype /Widget /FT /Sig /T (BadBR) /Rect [0 0 0 0] /P 3 0 R /F 132 /V 5 0 R >>\nendobj\n",
		"5 0 obj\n<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter /adbe.pkcs7.detached /ByteRange [0 100 200] /Contents <00> >>\nendobj\n",
	})
}

// rawRSAContents returns a PKCS#1 (non-CMS) /Contents payload for the
// adbe.x509.rsa_sha1 fixture.
func rawRSAContents(t *testing.T, p *testPKI) []byte {
	t.Helper()
	digest := sha256.Sum256([]byte("adbe.x509.rsa_sha1 fixture payload"))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.leafKey, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("fixture PKCS#1 sign: %v", err)
	}
	return sig
}
