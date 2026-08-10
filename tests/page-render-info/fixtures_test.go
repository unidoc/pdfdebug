// Story 11-6 hand-authored fixture PDFs (AC9).
//
// REPRODUCIBILITY: every fixture is built from raw PDF bytes in this file via
// assembleXref (same approach Story 11-5 used for its cycle/chain fixtures).
// There is NO external tool, no checked-in binary blob: the fixture IS this Go
// source, so it is reviewable in diff and regenerated deterministically on every
// `go test` run. To inspect a fixture by hand, write its bytes to a file:
//
//	os.WriteFile("renderinfo.pdf", renderInfoFixturePDF(), 0o644)
//	pdfdebug dump page --info 1 renderinfo.pdf
//
// renderInfoFixturePDF is the headline fixture exercising AC1-3 + AC7:
//   - page 1 with INHERITED MediaBox/Rotate (set on /Pages, NOT on /Page) and a
//     page-local CropBox (geometry inheritance gotcha, AC1).
//   - /Resources/ExtGState GS0: /BM /Multiply, /ca 0.5, /CA 1.0, /SMask a
//     resolved luminosity soft-mask dict (AC2).
//   - /Resources/ExtGState GS1: /SMask /None (the literal-None branch, AC2).
//   - /Resources/XObject Fm0: a Form XObject with /BBox, /Matrix, and a
//     transparency /Group (/S /Transparency /CS /DeviceRGB /I true /K false) (AC3).
//   - /Resources/XObject Im0: an Image XObject with /Width /Height and an
//     ICCBased /ColorSpace carrying /N 3 + a profile stream (AC3 classifier).
//   - /Resources/Pattern P0 (/PatternType 1) and /Shading Sh0 (/ShadingType 2):
//     structural-only entries (AC1 name+ref+type, AC7 not evaluated).
//
// selfRefFormFixturePDF is the recursion-termination fixture (AC4): page 1 does
// `Do /Fm0`; Fm0's OWN content stream does `Do /Fm0` against its OWN /Resources
// (a self-referential Do chain). The recursive walk must terminate on the
// form-object-ref visited set rather than loop forever.
//
// nestedFormFixturePDF is the own-resources fixture (AC4): page does `Do /Fm0`;
// Fm0's content does `Do /Inner` where /Inner lives in Fm0's OWN /Resources
// (NOT the page's). Proves the walk resolves nested forms against the form's
// own resource dict (feedback item 11 gotcha).
//
// noResourcesFixturePDF is the empty-result fixture (AC6): a valid page with NO
// /Resources dict at all -> every resource array must come back empty [].
package page_render_info_test

// renderInfoFixturePDF builds the headline AC1-3/AC7 fixture. Object map:
//
//	1 Catalog -> 2 Pages
//	2 Pages   (/MediaBox [0 0 612 792] /Rotate 90 INHERITED by the page) -> [3]
//	3 Page    (/CropBox local; NO MediaBox/Rotate -> inherited from 2)
//	4 page content stream  (q /Fm0 Do Q  q /Im0 Do Q)
//	5 ExtGState GS0  (/BM /Multiply /ca 0.5 /CA 1.0 /SMask 6 0 R)
//	6 SMask dict     (/S /Luminosity /G 7 0 R)
//	7 SMask group form (transparency group backing the soft mask)
//	8 Form XObject Fm0 (/BBox /Matrix /Group 9 0 R)
//	9 transparency Group dict (/S /Transparency /CS /DeviceRGB /I true /K false)
//	10 Image XObject Im0 (/Width 2 /Height 2 /ColorSpace [/ICCBased 11 0 R])
//	11 ICC profile stream (/N 3 + bytes)
//	12 Pattern P0 (/PatternType 1, tiling) -- structural only
//	13 Shading Sh0 (/ShadingType 2 /ColorSpace /DeviceRGB) -- structural only
//	14 ExtGState GS1 (/SMask /None)
func renderInfoFixturePDF() []byte {
	pdf := "%PDF-1.7\n"

	pageStream := "q /GS0 gs /Fm0 Do Q\nq /Im0 Do Q\n"
	smaskGroupStream := "0 0 100 100 re f\n"
	formStream := "0 0 100 100 re f\n"
	imgStream := "\xff\x00\x00\x00\xff\x00\x00\x00\xff\xff\xff\x00" // 2x2 RGB samples
	iccStream := "\x00\x00\x02\x0cICCprofilebytesplaceholder"      // opaque profile bytes
	patternStream := "0 0 10 10 re f\n"

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	// MediaBox + Rotate live on /Pages so the page INHERITS them (AC1 gotcha).
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 612 792] /Rotate 90 >>\nendobj\n\n"
	// Page has a LOCAL CropBox but NO MediaBox/Rotate -> those resolve from obj2.
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /CropBox [10 10 602 782] " +
		"/Resources << " +
		"/ExtGState << /GS0 5 0 R /GS1 14 0 R >> " +
		"/XObject << /Fm0 8 0 R /Im0 10 0 R >> " +
		"/Pattern << /P0 12 0 R >> " +
		"/Shading << /Sh0 13 0 R >> " +
		">> /Contents 4 0 R >>\nendobj\n\n"
	obj4 := "4 0 obj\n<< /Length " + itoa(len(pageStream)) + " >>\nstream\n" + pageStream + "\nendstream\nendobj\n\n"
	obj5 := "5 0 obj\n<< /Type /ExtGState /BM /Multiply /ca 0.5 /CA 1.0 /SMask 6 0 R >>\nendobj\n\n"
	obj6 := "6 0 obj\n<< /Type /Mask /S /Luminosity /G 7 0 R >>\nendobj\n\n"
	obj7 := "7 0 obj\n<< /Type /XObject /Subtype /Form /FormType 1 /BBox [0 0 100 100] " +
		"/Group << /Type /Group /S /Transparency /CS /DeviceGray >> /Length " +
		itoa(len(smaskGroupStream)) + " >>\nstream\n" + smaskGroupStream + "\nendstream\nendobj\n\n"
	obj8 := "8 0 obj\n<< /Type /XObject /Subtype /Form /FormType 1 /BBox [0 0 100 100] " +
		"/Matrix [1 0 0 1 0 0] /Group 9 0 R /Length " +
		itoa(len(formStream)) + " >>\nstream\n" + formStream + "\nendstream\nendobj\n\n"
	obj9 := "9 0 obj\n<< /Type /Group /S /Transparency /CS /DeviceRGB /I true /K false >>\nendobj\n\n"
	obj10 := "10 0 obj\n<< /Type /XObject /Subtype /Image /Width 2 /Height 2 " +
		"/ColorSpace [/ICCBased 11 0 R] /BitsPerComponent 8 /Length " +
		itoa(len(imgStream)) + " >>\nstream\n" + imgStream + "\nendstream\nendobj\n\n"
	obj11 := "11 0 obj\n<< /N 3 /Length " + itoa(len(iccStream)) + " >>\nstream\n" + iccStream + "\nendstream\nendobj\n\n"
	obj12 := "12 0 obj\n<< /Type /Pattern /PatternType 1 /PaintType 1 /TilingType 1 " +
		"/BBox [0 0 10 10] /XStep 10 /YStep 10 /Resources << >> /Length " +
		itoa(len(patternStream)) + " >>\nstream\n" + patternStream + "\nendstream\nendobj\n\n"
	obj13 := "13 0 obj\n<< /ShadingType 2 /ColorSpace /DeviceRGB /Coords [0 0 1 0] " +
		"/Function << /FunctionType 2 /Domain [0 1] /C0 [0 0 0] /C1 [1 1 1] /N 1 >> >>\nendobj\n\n"
	obj14 := "14 0 obj\n<< /Type /ExtGState /SMask /None >>\nendobj\n\n"

	return assembleXref(pdf, obj1, obj2, obj3, obj4, obj5, obj6, obj7, obj8, obj9, obj10, obj11, obj12, obj13, obj14)
}

// selfRefFormFixturePDF builds the AC4 recursion-termination fixture: Fm0's own
// content stream does `Do /Fm0`, naming itself in its own /Resources/XObject.
// The recursive walk must break this with the form-object-ref visited set.
//
//	1 Catalog -> 2 Pages -> 3 Page (Do /Fm0) -> 4 page content
//	5 Form Fm0: content does `Do /Fm0`, /Resources/XObject maps /Fm0 -> 5 0 R (self)
func selfRefFormFixturePDF() []byte {
	pdf := "%PDF-1.7\n"

	pageStream := "q /Fm0 Do Q\n"
	formStream := "q /Fm0 Do Q\n" // the form invokes itself

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 200 200] >>\nendobj\n\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R " +
		"/Resources << /XObject << /Fm0 5 0 R >> >> /Contents 4 0 R >>\nendobj\n\n"
	obj4 := "4 0 obj\n<< /Length " + itoa(len(pageStream)) + " >>\nstream\n" + pageStream + "\nendstream\nendobj\n\n"
	// Fm0's OWN /Resources/XObject maps /Fm0 back to itself (5 0 R).
	obj5 := "5 0 obj\n<< /Type /XObject /Subtype /Form /FormType 1 /BBox [0 0 100 100] " +
		"/Resources << /XObject << /Fm0 5 0 R >> >> /Length " +
		itoa(len(formStream)) + " >>\nstream\n" + formStream + "\nendstream\nendobj\n\n"

	return assembleXref(pdf, obj1, obj2, obj3, obj4, obj5)
}

// nestedFormFixturePDF builds the AC4 own-resources fixture: the page does
// `Do /Fm0`; Fm0's content does `Do /Inner`, and /Inner lives in Fm0's OWN
// /Resources (NOT the page's). The page's /Resources has NO /Inner entry, so a
// walk that wrongly resolved against the page resources would miss it.
//
//	1 Catalog -> 2 Pages -> 3 Page (Do /Fm0, /Resources has only /Fm0) -> 4 content
//	5 Form Fm0 (Do /Inner; /Resources/XObject maps /Inner -> 6 0 R)
//	6 Form Inner (leaf form)
func nestedFormFixturePDF() []byte {
	pdf := "%PDF-1.7\n"

	pageStream := "q /Fm0 Do Q\n"
	outerStream := "q /Inner Do Q\n"
	innerStream := "0 0 50 50 re f\n"

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 200 200] >>\nendobj\n\n"
	// Page resources expose only /Fm0 (NOT /Inner).
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R " +
		"/Resources << /XObject << /Fm0 5 0 R >> >> /Contents 4 0 R >>\nendobj\n\n"
	obj4 := "4 0 obj\n<< /Length " + itoa(len(pageStream)) + " >>\nstream\n" + pageStream + "\nendstream\nendobj\n\n"
	// Fm0's OWN /Resources/XObject exposes /Inner -> 6 0 R.
	obj5 := "5 0 obj\n<< /Type /XObject /Subtype /Form /FormType 1 /BBox [0 0 100 100] " +
		"/Resources << /XObject << /Inner 6 0 R >> >> /Length " +
		itoa(len(outerStream)) + " >>\nstream\n" + outerStream + "\nendstream\nendobj\n\n"
	obj6 := "6 0 obj\n<< /Type /XObject /Subtype /Form /FormType 1 /BBox [0 0 50 50] /Length " +
		itoa(len(innerStream)) + " >>\nstream\n" + innerStream + "\nendstream\nendobj\n\n"

	return assembleXref(pdf, obj1, obj2, obj3, obj4, obj5, obj6)
}

// noResourcesFixturePDF builds the AC6 empty-result fixture: a valid single page
// with NO /Resources dict. Every resource array must come back empty [], exit 0.
func noResourcesFixturePDF() []byte {
	pdf := "%PDF-1.7\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>\nendobj\n\n"
	return assembleXref(pdf, obj1, obj2, obj3)
}

// --- raw-PDF assembly helpers (mirrors Story 11-5's assembleXref) -----------

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func pad10(n int) string {
	s := itoa(n)
	for len(s) < 10 {
		s = "0" + s
	}
	return s
}

// assembleXref concatenates header + objects and appends a classic xref +
// trailer (Root = 1 0 R). parts[0] is the PDF header.
func assembleXref(parts ...string) []byte {
	header := parts[0]
	objs := parts[1:]
	body := header
	offsets := make([]int, len(objs))
	cur := len(header)
	for i, o := range objs {
		offsets[i] = cur
		body += o
		cur += len(o)
	}
	xrefOffset := len(body)
	xref := "xref\n0 " + itoa(len(objs)+1) + "\n0000000000 65535 f \n"
	for _, off := range offsets {
		xref += pad10(off) + " 00000 n \n"
	}
	trailer := "trailer\n<< /Size " + itoa(len(objs)+1) + " /Root 1 0 R >>\nstartxref\n" + itoa(xrefOffset) + "\n%%EOF\n"
	return []byte(body + xref + trailer)
}
