package object_tree_scalar_values_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf16"
)

// The fixtures are assembled here as raw PDF bytes rather than read from
// testdata/, so this suite pins the behaviour without waiting on a committed
// file and without competing with the generator for a filename. Every value the
// acceptance criteria name has a dedicated key, and the keys are spelled so the
// sorted tree row order is predictable.

// Byte-level constants shared between the builders and the assertions.
const (
	// altHex is "Rapport cell" as UTF-16BE with a BOM.
	altHex = "FEFF0052006100700070006F00720074002000630065006C006C"
	// altText is what altHex decodes to.
	altText = "Rapport cell"
	// altTextChanged is the right-hand side of the diff pair.
	altTextChanged = "Rapport header"

	// eLiteral is "Exp" as UTF-16BE with a BOM, written with octal escapes.
	eLiteral = `(\376\377\000E\000x\000p)`
	// eText is what eLiteral decodes to.
	eText = "Exp"

	// backslashHex carries a 0x5C byte inside a hex text string. pdfcpu runs
	// escape processing over the already-decoded bytes and drops it, so this
	// decodes to "AB" rather than "A\B".
	backslashHex  = "415C42"
	backslashText = "AB"

	// langLiteral is all-ASCII: decoding is a no-op and no raw counterpart is
	// emitted for it.
	langText = "en-US"

	// sigContentsBytes is the stand-in signature payload length: 128 hex digits
	// in the file, so 64 bytes of binary.
	sigContentsBytes = 64
	// sigCertBytes is the stand-in certificate length: 32 hex digits, 16 bytes.
	sigCertBytes = 16
	// checkSumHex is a filespec /Params /CheckSum: 32 hex digits, 16 bytes.
	checkSumHex   = "DEADBEEFCAFEF00D0011223344556677"
	checkSumBytes = 16

	// annotContents is a markup annotation's /Contents. Same key as the
	// signature dictionary's, opposite outcome: this one decodes.
	annotContents = "Review this cell"

	// cjkRune is one multi-byte rune, used to prove the cap counts runes.
	cjkRune    = "中"
	cjkHexRune = "4E2D"
	// capMultiRunes is how many of them /CapMulti carries.
	capMultiRunes = 90
)

// assemblePDF lays out objs as a classic xref PDF with obj n at index n-1.
func assemblePDF(objs []string) []byte {
	body := "%PDF-1.7\n%\xe2\xe3\xcf\xd3\n"
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

// scalarValuesPDF carries one key per behaviour the acceptance criteria name:
// decodable text strings, an ASCII string whose decode is a no-op, the three
// empty forms, every non-string scalar kind, control characters, the cap
// boundaries, arrays of scalars, binary-carrying keys in both signature shapes,
// a filespec /Params /CheckSum, and a markup annotation /Contents.
func scalarValuesPDF() []byte {
	sigContents := strings.Repeat("AB", sigContentsBytes)
	sigCert := strings.Repeat("CD", sigCertBytes)

	scalars := "5 0 obj\n<< " +
		"/Nm /Foo " +
		"/Int 2 " +
		"/Real 1.5 " +
		"/Flag true " +
		"/Off false " +
		"/Empty () " +
		"/HexEmpty <> " +
		"/BomOnly <FEFF> " +
		"/Lang (" + langText + ") " +
		"/Backslash <" + backslashHex + "> " +
		">>\nendobj\n"

	// Control characters. Each decodes to a character that must never reach the
	// terminal unescaped, so each row must stay exactly one line.
	controls := "6 0 obj\n<< " +
		`/Newline (a\nb) ` +
		`/Return (a\rb) ` +
		`/Tab (a\tb) ` +
		`/Backslash2 (a\\b) ` +
		`/Nul (a\000b) ` +
		`/Curly (don\222t) ` +
		">>\nendobj\n"

	// Cap boundaries, measured over the escaped value.
	caps := "7 0 obj\n<< " +
		"/CapA (" + strings.Repeat("x", 79) + ") " +
		"/CapB (" + strings.Repeat("x", 80) + ") " +
		"/CapC (" + strings.Repeat("x", 81) + ") " +
		`/CapEscaped (` + strings.Repeat(`\t`, 41) + `) ` +
		"/CapMulti <FEFF" + strings.Repeat(cjkHexRune, capMultiRunes) + "> " +
		">>\nendobj\n"

	arrays := "8 0 obj\n<< " +
		"/Nums [0 1 612] " +
		"/Strs [" + eLiteral + " (plain)] " +
		"/LongStrs [(" + strings.Repeat("x", 81) + ")] " +
		">>\nendobj\n"

	return assemblePDF([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Lang (" + langText + ") " +
			"/StructTreeRoot 9 0 R /Scalars 5 0 R /Controls 6 0 R /Caps 7 0 R " +
			"/Arrays 8 0 R /SigTyped 12 0 R /SigByteRange 13 0 R /SigCertArray 14 0 R " +
			"/Attachment 15 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Annots [4 0 R] >>\nendobj\n",
		"4 0 obj\n<< /Type /Annot /Subtype /Text /Rect [0 0 10 10] /Contents (" + annotContents + ") >>\nendobj\n",
		scalars,
		controls,
		caps,
		arrays,
		"9 0 obj\n<< /Type /StructTreeRoot /K [10 0 R 11 0 R] >>\nendobj\n",
		"10 0 obj\n<< /Type /StructElem /S /TD /Alt <" + altHex + "> /E " + eLiteral +
			" /A << /O /Table /RowSpan 2 /ColSpan 3 >> >>\nendobj\n",
		"11 0 obj\n<< /Type /StructElem /S /TD /Alt (Second cell)" +
			" /A << /O /Table /RowSpan 1 /ColSpan 4 >> >>\nendobj\n",
		// Signature dictionary carrying an explicit /Type /Sig.
		"12 0 obj\n<< /Type /Sig /SubFilter /adbe.pkcs7.detached /ByteRange [0 100 200 300]" +
			" /Contents <" + sigContents + "> /Cert <" + sigCert + "> >>\nendobj\n",
		// Signature dictionary with no /Type: recognised by /ByteRange alone.
		"13 0 obj\n<< /SubFilter /adbe.pkcs7.detached /ByteRange [0 100 200 300]" +
			" /Contents <" + sigContents + "> >>\nendobj\n",
		// /Cert as an array of strings. The carve-out is decided on the /Cert
		// node and inherited by its element rows.
		"14 0 obj\n<< /Type /Sig /ByteRange [0 100 200 300] /Cert [<" + sigCert + "> <" + sigCert + ">] >>\nendobj\n",
		"15 0 obj\n<< /Type /Filespec /F (a.xml) /Desc (Attachment) /EF << /F 16 0 R >> >>\nendobj\n",
		"16 0 obj\n<< /Length 3 /Params << /CheckSum <" + checkSumHex + "> /Size 3 >> >>\nstream\nabc\nendstream\nendobj\n",
	})
}

// decodeCollisionPDF pairs two documents whose strings are byte-different but
// decode identically, so the diff's equality decision can be pinned. side "a"
// stores /Alt as UTF-16BE hex and /Quirk with an embedded 0x5C; side "b" stores
// the same two values as a plain literal and as plain hex.
func decodeCollisionPDF(side string) []byte {
	alt := "<FEFF0041>"
	quirk := "<415C42>"
	if side == "b" {
		alt = "(A)"
		quirk = "<4142>"
	}
	return assemblePDF([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 4 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		"4 0 obj\n<< /Type /StructTreeRoot /Alt " + alt + " /Quirk " + quirk + " >>\nendobj\n",
	})
}

// utf16beHex renders s as the hex digits of its UTF-16BE encoding, BOM first.
func utf16beHex(s string) string {
	var b strings.Builder
	b.WriteString("FEFF")
	for _, r := range utf16.Encode([]rune(s)) {
		fmt.Fprintf(&b, "%04X", r)
	}
	return b.String()
}

// textChangePDF is the pair behind the diff's displayed summaries: the same
// key, UTF-16BE on both sides, with genuinely different text.
func textChangePDF(side string) []byte {
	alt := "<" + utf16beHex(altText) + ">"
	if side == "b" {
		alt = "<" + utf16beHex(altTextChanged) + ">"
	}
	return assemblePDF([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 4 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		"4 0 obj\n<< /Type /StructTreeRoot /Alt " + alt + " >>\nendobj\n",
	})
}

var (
	fixtureOnce sync.Once
	fixtureDir  string
	fixtureErr  string
)

// fixtures writes every generated PDF once per module run and returns the
// directory holding them. The directory is a plain os.MkdirTemp, not
// t.TempDir: it is shared across tests and must outlive any single one.
func fixtures(t *testing.T) string {
	t.Helper()
	fixtureOnce.Do(func() {
		dir, err := os.MkdirTemp("", "pdfdebug-scalar-values-")
		if err != nil {
			fixtureErr = "failed to create temp dir: " + err.Error()
			return
		}
		files := map[string][]byte{
			"scalar-values.pdf":    scalarValuesPDF(),
			"decode-collision.pdf": decodeCollisionPDF("a"),
			"decode-twin.pdf":      decodeCollisionPDF("b"),
			"text-change-a.pdf":    textChangePDF("a"),
			"text-change-b.pdf":    textChangePDF("b"),
		}
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(dir, name), content, 0o600); err != nil {
				fixtureErr = "failed to write " + name + ": " + err.Error()
				return
			}
		}
		fixtureDir = dir
	})
	if fixtureErr != "" {
		t.Fatalf("%s", fixtureErr)
	}
	return fixtureDir
}

// fixturePath returns the absolute path of one generated fixture.
func fixturePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(fixtures(t), name)
}
