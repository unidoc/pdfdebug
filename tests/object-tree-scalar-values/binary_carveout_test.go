package object_tree_scalar_values_test

import (
	"fmt"
	"strings"
	"testing"
)

// signedContentsBytes is the binary payload length of testdata/signed.pdf's
// signature /Contents: 6144 hex digits, so 3072 bytes.
const signedContentsBytes = 3072

// binarySummary renders the fixed-width stand-in a carved-out value shows
// instead of the blob.
func binarySummary(n int) string {
	return fmt.Sprintf("<binary, %d bytes>", n)
}

// Binary-carrying strings are carved out at the callers, where the key is
// known. The match is by key name plus context, so the same key decodes in one
// place and not in another.

func TestCarveOut_SignatureContentsWithExplicitType(t *testing.T) {
	root := treeJSON(t, fixturePath(t, "scalar-values.pdf"))
	node := nodeAt(t, root, "/SigTyped", "/Contents")

	if got := mustValue(t, node); got != binarySummary(sigContentsBytes) {
		t.Errorf("/Type /Sig /Contents value = %q, want %q", got, binarySummary(sigContentsBytes))
	}
}

func TestCarveOut_SignatureContentsRecognisedByByteRangeAlone(t *testing.T) {
	root := treeJSON(t, fixturePath(t, "scalar-values.pdf"))
	node := nodeAt(t, root, "/SigByteRange", "/Contents")

	if got := mustValue(t, node); got != binarySummary(sigContentsBytes) {
		t.Errorf("/Contents in a /ByteRange-carrying dict with no /Type = %q, want %q",
			got, binarySummary(sigContentsBytes))
	}
}

func TestCarveOut_AnnotationContentsStillDecodes(t *testing.T) {
	root := treeJSON(t, fixturePath(t, "scalar-values.pdf"))
	node := nodeAt(t, root, "/Pages", "/Kids", "[0]", "/Annots", "[0]", "/Contents")

	got := mustValue(t, node)
	if got != annotContents {
		t.Errorf("annotation /Contents = %q, want the decoded %q", got, annotContents)
	}
	if strings.HasPrefix(got, "<binary,") {
		t.Errorf("annotation /Contents was carved out as binary; only a signature dictionary's is")
	}
}

func TestCarveOut_SignatureCertAsASingleString(t *testing.T) {
	root := treeJSON(t, fixturePath(t, "scalar-values.pdf"))
	node := nodeAt(t, root, "/SigTyped", "/Cert")

	if got := mustValue(t, node); got != binarySummary(sigCertBytes) {
		t.Errorf("/Cert value = %q, want %q", got, binarySummary(sigCertBytes))
	}
}

func TestCarveOut_SignatureCertAsAnArrayCarvesItsElements(t *testing.T) {
	root := treeJSON(t, fixturePath(t, "scalar-values.pdf"))
	certArray := nodeAt(t, root, "/SigCertArray", "/Cert")

	if len(certArray.Children) != 2 {
		t.Fatalf("/Cert array has %d elements, want 2", len(certArray.Children))
	}
	for i, elem := range certArray.Children {
		if elem.Label != binarySummary(sigCertBytes) {
			t.Errorf("/Cert[%d] label = %q, want %q; the carve-out is decided on the /Cert node and inherited",
				i, elem.Label, binarySummary(sigCertBytes))
		}
	}
}

func TestCarveOut_FilespecParamsCheckSum(t *testing.T) {
	root := treeJSON(t, fixturePath(t, "scalar-values.pdf"))
	node := nodeAt(t, root, "/Attachment", "/EF", "/F", "/Params", "/CheckSum")

	if got := mustValue(t, node); got != binarySummary(checkSumBytes) {
		t.Errorf("/Params /CheckSum value = %q, want %q", got, binarySummary(checkSumBytes))
	}
}

func TestCarveOut_LengthIsThePayloadNotTheStoredToken(t *testing.T) {
	root := treeJSON(t, committedFixture(t, "signed.pdf"))
	node := nodeAt(t, root, "/AcroForm", "/Fields", "[0]", "/V", "/Contents")

	want := binarySummary(signedContentsBytes)
	if got := mustValue(t, node); got != want {
		t.Errorf("signed.pdf /Contents value = %q, want %q (6144 hex digits are 3072 bytes)", got, want)
	}
}

func TestCarveOut_SummaryCarriesNoRawCounterpartAndNoMarker(t *testing.T) {
	root := treeJSON(t, fixturePath(t, "scalar-values.pdf"))

	for _, path := range [][]string{
		{"/SigTyped", "/Contents"},
		{"/SigTyped", "/Cert"},
		{"/Attachment", "/EF", "/F", "/Params", "/CheckSum"},
	} {
		node := nodeAt(t, root, path...)
		if raw, ok := valueRaw(node); ok {
			t.Errorf("%v emits valueRaw %q; a carved-out value has no raw counterpart", path, raw)
		}
		if strings.Contains(mustValue(t, node), "[truncated") {
			t.Errorf("%v carries a truncation marker; the summary is already fixed-width", path)
		}
	}
}

func TestCarveOut_PlainRowShowsTheSummaryNotTheBlob(t *testing.T) {
	out := treePlain(t, fixturePath(t, "scalar-values.pdf"))

	if !hasLine(out, "Contents string = "+binarySummary(sigContentsBytes)) {
		t.Errorf("dump tree does not summarise the signature /Contents\n--- output ---\n%s", out)
	}
	if !hasLine(out, "CheckSum string = "+binarySummary(checkSumBytes)) {
		t.Errorf("dump tree does not summarise the filespec /Params /CheckSum\n--- output ---\n%s", out)
	}
	if strings.Contains(out, strings.Repeat("AB", sigContentsBytes)) {
		t.Errorf("dump tree printed the signature blob")
	}
}

// The bytes stay reachable. dump source reserializes them and dump object keeps
// them in the raw field.

func TestCarveOut_BytesStayReachableThroughSourceAndObject(t *testing.T) {
	pdf := committedFixture(t, "signed.pdf")

	source, stderr, code := runCLI(t, "dump", "source", "--ref", "5 0 R", pdf)
	if code != 0 {
		t.Fatalf("dump source exited %d: %s", code, stderr)
	}
	if !strings.Contains(source, "/Contents <") {
		t.Errorf("dump source no longer emits the signature /Contents hex")
	}

	detail := objectJSON(t, pdf, "5 0 R")
	raw := propertyValue(t, detail, "/Contents").Raw
	if len(raw) != 6146 {
		t.Errorf("dump object --json /Contents raw is %d characters, want 6146 (6144 hex digits plus delimiters)",
			len(raw))
	}
}
