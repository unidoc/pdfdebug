package pages_navigator_test

import (
	"fmt"
	"sort"
	"strings"
)

// The fixtures are assembled from raw PDF bytes at run time rather than read
// from testdata/, so each page-tree shape lives next to the assertions about it.

// pdfObj is one indirect object: its number and the text between "N 0 obj" and
// "endobj".
type pdfObj struct {
	num  int
	body string
}

// assemblePDF lays out objs as a classic-xref PDF. Numbers absent from objs
// are written as free entries, so a reference to one is dangling.
func assemblePDF(objs ...pdfObj) []byte {
	sorted := append([]pdfObj{}, objs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].num < sorted[j].num })
	maxNum := sorted[len(sorted)-1].num

	var b strings.Builder
	b.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	offsets := map[int]int{}
	for _, o := range sorted {
		offsets[o.num] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", o.num, o.body)
	}
	xrefOff := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", maxNum+1)
	for n := 1; n <= maxNum; n++ {
		if off, ok := offsets[n]; ok {
			fmt.Fprintf(&b, "%010d 00000 n \n", off)
		} else {
			b.WriteString("0000000000 00000 f \n")
		}
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", maxNum+1, xrefOff)
	return []byte(b.String())
}

// stream renders a stream object body whose /Length matches data.
func stream(data string) string {
	return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(data), data)
}

const catalog = "<< /Type /Catalog /Pages 2 0 R >>"

// Content payloads with known lengths.
const (
	contentA = "0 0 m 10 10 l S"        // 15 bytes
	contentB = "q 1 0 0 1 5 5 cm Q"     // 18 bytes
	contentC = "BT /F1 12 Tf (x) Tj ET" // 22 bytes
)

// wellFormedPDF has two pages: page 1 with its own MediaBox and one content
// stream, page 2 with a fractional MediaBox, /Rotate 90, two annotations and no
// /Contents.
func wellFormedPDF() []byte {
	return assemblePDF(
		pdfObj{1, catalog},
		pdfObj{2, "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>"},
		pdfObj{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> /Contents 4 0 R >>"},
		pdfObj{4, stream(contentA)},
		pdfObj{5, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595.28 841.89] /Rotate 90 /Resources << >> /Annots [6 0 R 7 0 R] >>"},
		pdfObj{6, "<< /Type /Annot /Subtype /Text /Rect [0 0 10 10] >>"},
		pdfObj{7, "<< /Type /Annot /Subtype /Text /Rect [0 0 20 20] >>"},
	)
}

// Own values used by inheritedPDF's leaves, distinct from every inherited one.
const (
	ownRes    = "/Resources << /ProcSet [/PDF] >>"
	ownMedia  = "/MediaBox [0 0 100 200]"
	ownCrop   = "/CropBox [0 0 90 190]"
	ownRotate = "/Rotate 180"
)

// inheritedPDF puts all four inheritable attributes on the root /Pages and an
// overriding /MediaBox on an intermediate node. Leaves, in document order:
//
//	1 (obj 10) declares all four                 -> inherits nothing
//	2 (obj 11) declares all but /Resources       -> inherits Resources
//	3 (obj 12) declares all but /MediaBox        -> inherits MediaBox 0 0 612 792
//	4 (obj 13) declares all but /CropBox         -> inherits CropBox
//	5 (obj 14) declares all but /Rotate          -> inherits Rotate 90
//	6 (obj 15) declares none                     -> inherits all four
//	7 (obj 16) under obj 3, declares none        -> nearest MediaBox 0 0 595 842
func inheritedPDF() []byte {
	leaf := func(num, parent int, attrs ...string) pdfObj {
		return pdfObj{num, fmt.Sprintf("<< /Type /Page /Parent %d 0 R %s >>", parent, strings.Join(attrs, " "))}
	}
	return assemblePDF(
		pdfObj{1, catalog},
		pdfObj{2, "<< /Type /Pages /Kids [10 0 R 11 0 R 12 0 R 13 0 R 14 0 R 15 0 R 3 0 R] /Count 7 " +
			"/Resources << >> /MediaBox [0 0 612 792] /CropBox [0 0 600 780] /Rotate 90 >>"},
		pdfObj{3, "<< /Type /Pages /Parent 2 0 R /Kids [16 0 R] /Count 1 /MediaBox [0 0 595 842] >>"},
		leaf(10, 2, ownRes, ownMedia, ownCrop, ownRotate),
		leaf(11, 2, ownMedia, ownCrop, ownRotate),
		leaf(12, 2, ownRes, ownCrop, ownRotate),
		leaf(13, 2, ownRes, ownMedia, ownRotate),
		leaf(14, 2, ownRes, ownMedia, ownCrop),
		leaf(15, 2),
		leaf(16, 3),
	)
}

// nullKidPDF has a null between two real pages in the root /Kids. pdfcpu's
// validator drops the null at open, so rows are page 1 (obj 3), page 2 (obj 4).
func nullKidPDF() []byte {
	return assemblePDF(
		pdfObj{1, catalog},
		pdfObj{2, "<< /Type /Pages /Kids [3 0 R null 4 0 R] /Count 2 /MediaBox [0 0 612 792] >>"},
		pdfObj{3, "<< /Type /Page /Parent 2 0 R >>"},
		pdfObj{4, "<< /Type /Page /Parent 2 0 R >>"},
	)
}

// duplicatePagePDF lists page obj 3 twice in the root /Kids. Viewers count it
// twice, so rows are page 1 (obj 3), page 2 (obj 3 again), page 3 (obj 4).
func duplicatePagePDF() []byte {
	return assemblePDF(
		pdfObj{1, catalog},
		pdfObj{2, "<< /Type /Pages /Kids [3 0 R 3 0 R 4 0 R] /Count 3 /MediaBox [0 0 612 792] >>"},
		pdfObj{3, "<< /Type /Page /Parent 2 0 R >>"},
		pdfObj{4, "<< /Type /Page /Parent 2 0 R >>"},
	)
}

// pageWithKidsPDF has a /Type /Page node (obj 3) that also carries /Kids;
// viewers number it as page 1 and never reach its kid obj 4.
func pageWithKidsPDF() []byte {
	return assemblePDF(
		pdfObj{1, catalog},
		pdfObj{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 612 792] >>"},
		pdfObj{3, "<< /Type /Page /Parent 2 0 R /Kids [4 0 R] /Count 1 >>"},
		pdfObj{4, "<< /Type /Page /Parent 3 0 R >>"},
	)
}

// pageWithEmptyKidsPDF has three pages, the first a /Type /Page carrying an
// empty /Kids; viewers number it page 1.
func pageWithEmptyKidsPDF() []byte {
	return assemblePDF(
		pdfObj{1, catalog},
		pdfObj{2, "<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 3 /MediaBox [0 0 612 792] >>"},
		pdfObj{3, "<< /Type /Page /Parent 2 0 R /Kids [] >>"},
		pdfObj{4, "<< /Type /Page /Parent 2 0 R >>"},
		pdfObj{5, "<< /Type /Page /Parent 2 0 R >>"},
	)
}

// emptyKidsPDF has an intermediate with /Kids [] ahead of the only page.
func emptyKidsPDF() []byte {
	return assemblePDF(
		pdfObj{1, catalog},
		pdfObj{2, "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 1 /MediaBox [0 0 612 792] >>"},
		pdfObj{3, "<< /Type /Pages /Parent 2 0 R /Kids [] /Count 4 >>"},
		pdfObj{4, "<< /Type /Page /Parent 2 0 R >>"},
	)
}

// untypedRootPDF's root /Pages has no /Type. It still has /Kids, so it is an
// intermediate and both pages resolve cleanly.
func untypedRootPDF() []byte {
	return assemblePDF(
		pdfObj{1, catalog},
		pdfObj{2, "<< /Kids [3 0 R 4 0 R] /Count 2 /MediaBox [0 0 612 792] >>"},
		pdfObj{3, "<< /Type /Page /Parent 2 0 R >>"},
		pdfObj{4, "<< /Type /Page /Parent 2 0 R >>"},
	)
}

// contentsPDF exercises how /Contents and /Length are read:
//
//	page 1 (obj 3):  /Contents 5 0 R, where obj 5 is the array [6 0 R 7 0 R]
//	page 2 (obj 4):  /Contents [6 0 R 7 0 R], a direct array
//	page 3 (obj 8):  one stream whose /Length is the indirect object 16
//	page 4 (obj 10): one stream with no /Length
//	page 5 (obj 12): one stream whose /Length is a name
//	page 6 (obj 14): no /Contents
func contentsPDF() []byte {
	return assemblePDF(
		pdfObj{1, catalog},
		pdfObj{2, "<< /Type /Pages /Kids [3 0 R 4 0 R 8 0 R 10 0 R 12 0 R 14 0 R] /Count 6 /MediaBox [0 0 612 792] >>"},
		pdfObj{3, "<< /Type /Page /Parent 2 0 R /Contents 5 0 R >>"},
		pdfObj{4, "<< /Type /Page /Parent 2 0 R /Contents [6 0 R 7 0 R] >>"},
		pdfObj{5, "[6 0 R 7 0 R]"},
		pdfObj{6, stream(contentA)},
		pdfObj{7, stream(contentB)},
		pdfObj{8, "<< /Type /Page /Parent 2 0 R /Contents 9 0 R >>"},
		pdfObj{9, fmt.Sprintf("<< /Length 16 0 R >>\nstream\n%s\nendstream", contentB)},
		pdfObj{10, "<< /Type /Page /Parent 2 0 R /Contents 11 0 R >>"},
		pdfObj{11, "<< >>\nstream\nabc\nendstream"},
		pdfObj{12, "<< /Type /Page /Parent 2 0 R /Contents 13 0 R >>"},
		pdfObj{13, "<< /Length /X >>\nstream\nabc\nendstream"},
		pdfObj{14, "<< /Type /Page /Parent 2 0 R >>"},
		pdfObj{16, fmt.Sprintf("%d", len(contentB))},
	)
}

// indirectAttrsPDF stores page attributes behind indirect references:
//
//	root: /MediaBox 8 0 R, obj 8 is [0 0 10 20]
//	page 1 (obj 3): own /MediaBox 9 0 R, obj 9 is [0 0 300 400]
//	page 2 (obj 4): nothing of its own, inherits the root's indirect MediaBox
//	page 3 (obj 5): /Rotate 10 0 R, obj 10 is 90
//	page 4 (obj 6): /Rotate -90, reported as stored
//	page 5 (obj 7): /Annots 11 0 R, obj 11 is [12 0 R 13 0 R]
func indirectAttrsPDF() []byte {
	return assemblePDF(
		pdfObj{1, catalog},
		pdfObj{2, "<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R 6 0 R 7 0 R] /Count 5 /MediaBox 8 0 R >>"},
		pdfObj{3, "<< /Type /Page /Parent 2 0 R /MediaBox 9 0 R >>"},
		pdfObj{4, "<< /Type /Page /Parent 2 0 R >>"},
		pdfObj{5, "<< /Type /Page /Parent 2 0 R /Rotate 10 0 R >>"},
		pdfObj{6, "<< /Type /Page /Parent 2 0 R /Rotate -90 >>"},
		pdfObj{7, "<< /Type /Page /Parent 2 0 R /Annots 11 0 R >>"},
		pdfObj{8, "[0 0 10 20]"},
		pdfObj{9, "[0 0 300 400]"},
		pdfObj{10, "90"},
		pdfObj{11, "[12 0 R 13 0 R]"},
		pdfObj{12, "<< /Type /Annot /Subtype /Text /Rect [0 0 1 1] >>"},
		pdfObj{13, "<< /Type /Annot /Subtype /Text /Rect [0 0 2 2] >>"},
	)
}

// noPagesPDF has an empty page tree.
func noPagesPDF() []byte {
	return assemblePDF(
		pdfObj{1, catalog},
		pdfObj{2, "<< /Type /Pages /Kids [] /Count 0 >>"},
	)
}

// allFixtures names every generated fixture, for checks that apply to all.
func allFixtures() map[string][]byte {
	return map[string][]byte{
		"well-formed.pdf":    wellFormedPDF(),
		"inherited.pdf":      inheritedPDF(),
		"null-kid.pdf":       nullKidPDF(),
		"duplicate-page.pdf": duplicatePagePDF(),
		"page-with-kids.pdf": pageWithKidsPDF(),
		"empty-kids.pdf":     emptyKidsPDF(),
		"untyped-root.pdf":   untypedRootPDF(),
		"contents.pdf":       contentsPDF(),
		"indirect-attrs.pdf": indirectAttrsPDF(),
		"no-pages.pdf":       noPagesPDF(),
	}
}
