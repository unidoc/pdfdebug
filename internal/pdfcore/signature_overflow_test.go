package pdfcore

import (
	"strings"
	"testing"
)

// sigOverflowBRPDF builds a signed field whose /ByteRange holds values large
// enough that start+len would overflow int64 if summed unguarded. All four
// entries are individually non-negative so the naive negative-only check
// passes.
func sigOverflowBRPDF() []byte {
	const huge = "4611686018427387904" // 2^62; two of these sum past maxint64
	return assemblexref("%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] /SigFlags 3 >> >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Annots [4 0 R] >>\nendobj\n",
		"4 0 obj\n<< /Type /Annot /Subtype /Widget /FT /Sig /T (OverflowBR) /Rect [0 0 0 0] /P 3 0 R /F 132 /V 5 0 R >>\nendobj\n",
		"5 0 obj\n<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter /adbe.pkcs7.detached /ByteRange ["+
			huge+" "+huge+" 0 0] /Contents <deadbeef> >>\nendobj\n",
	)
}

// TestGetSignatures_ByteRangeOverflowDegrades pins the coverage guard against a
// crafted /ByteRange whose start+len sums would overflow int64. It must degrade
// to a per-signature CoverageError, never panic the whole GetSignatures call.
func TestGetSignatures_ByteRangeOverflowDegrades(t *testing.T) {
	ins, tabID := writeTempPDF(t, "overflow-br.pdf", sigOverflowBRPDF())

	list, err := ins.GetSignatures(tabID)
	f := oneSigField(t, list, err)
	if f.FieldName != "OverflowBR" {
		t.Errorf("FieldName = %q, want OverflowBR", f.FieldName)
	}
	if f.CoverageError == "" {
		t.Errorf("CoverageError empty, want the out-of-range /ByteRange fact")
	}
	if !strings.Contains(strings.ToLower(f.CoverageError), "exceeds") {
		t.Errorf("CoverageError = %q, want an 'exceeds the file size' fact", f.CoverageError)
	}
}
