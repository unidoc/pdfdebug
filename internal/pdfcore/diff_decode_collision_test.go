package pdfcore

import (
	"testing"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Two strings whose bytes differ but whose text decodes alike show their
// byte-exact forms, so a changed row never shows two identical summaries.
func TestScalarLeaf_StringsThatDecodeAlikeShowByteExactSummaries(t *testing.T) {
	for _, tc := range []struct {
		name                string
		left, right         pdfcpu_types.Object
		wantLeft, wantRight string
	}{
		{
			name:      "UTF-8 BOM against plain ASCII",
			left:      pdfcpu_types.HexLiteral("EFBBBF41"),
			right:     pdfcpu_types.HexLiteral("41"),
			wantLeft:  "<EFBBBF41>",
			wantRight: "<41>",
		},
		{
			name:      "two different undefined PDFDocEncoding bytes",
			left:      pdfcpu_types.HexLiteral("9F41"),
			right:     pdfcpu_types.HexLiteral("AD41"),
			wantLeft:  "<9F41>",
			wantRight: "<AD41>",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := scalarLeaf("/Root/StructTreeRoot/Alt", "scalar", tc.left, tc.right, false)
			if node.Status != "changed" {
				t.Errorf("status = %q, want changed", node.Status)
			}
			if node.LeftSummary != tc.wantLeft || node.RightSummary != tc.wantRight {
				t.Errorf("summaries = %q / %q, want the byte-exact %q / %q",
					node.LeftSummary, node.RightSummary, tc.wantLeft, tc.wantRight)
			}
		})
	}
}

// A 0x5C byte in a hex string is kept in the decoded text, so 415C42 and 4142
// do not collide and the row shows the decoded summaries.
func TestScalarLeaf_HexStringBackslashByteShowsDecodedText(t *testing.T) {
	node := scalarLeaf("/Root/StructTreeRoot/Alt", "scalar",
		pdfcpu_types.HexLiteral("415C42"), pdfcpu_types.HexLiteral("4142"), false)
	if node.Status != "changed" {
		t.Errorf("status = %q, want changed", node.Status)
	}
	if node.LeftSummary != `A\B` || node.RightSummary != "AB" {
		t.Errorf("summaries = %q / %q, want the decoded %q / %q",
			node.LeftSummary, node.RightSummary, `A\B`, "AB")
	}
}
