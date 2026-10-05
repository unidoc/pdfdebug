package images_navigator_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"sort"
	"strings"
	"testing"
)

// The fixtures are assembled from raw PDF bytes at run time rather than read
// from testdata/, so each resource-graph shape lives next to the assertions
// about it. Every image is uncompressed unless a case needs a filter, so its
// stream length is the declared geometry exactly.

// pdfObj is one indirect object: its number and the text between "N 0 obj" and
// "endobj". The body may hold binary stream data.
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

const catalog = "<< /Type /Catalog /Pages 2 0 R >>"

// streamBody renders a stream object body: dict entries plus a /Length that
// matches data.
func streamBody(dict, data string) string {
	return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", dict, len(data), data)
}

// grayImage is an uncompressed w x h DeviceGray image at 8 bits, so its
// estimated decoded size is w*h bytes. extra is spliced into the dictionary.
func grayImage(w, h int, extra string) string {
	dict := fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceGray /BitsPerComponent 8 %s", w, h, extra)
	return streamBody(dict, strings.Repeat("x", w*h))
}

// rgbImage is an uncompressed w x h DeviceRGB image at 8 bits.
func rgbImage(w, h int) string {
	dict := fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8", w, h)
	return streamBody(dict, strings.Repeat("x", w*h*3))
}

// form is a Form XObject whose /Resources /XObject dictionary holds xobjects
// (the text between << and >>). An empty xobjects writes no /Resources at all.
func form(xobjects string) string {
	dict := "/Type /XObject /Subtype /Form /BBox [0 0 10 10]"
	if xobjects != "" {
		dict += " /Resources << /XObject << " + xobjects + " >> >>"
	}
	return streamBody(dict, "q Q")
}

// pageWith is a /Page under obj 2 whose own /Resources /XObject holds xobjects.
func pageWith(xobjects string) string {
	return "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << " + xobjects + " >> >> >>"
}

// bareMediaPage is a /Page under obj 2 with no /Resources of its own.
const bareMediaPage = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>"

// pagesNode is the root /Pages node listing kids (object numbers) in order.
func pagesNode(extra string, kids ...int) string {
	refs := make([]string, len(kids))
	for i, k := range kids {
		refs[i] = fmt.Sprintf("%d 0 R", k)
	}
	return fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d %s >>", strings.Join(refs, " "), len(kids), extra)
}

// sharedPageCount is how many pages sharedImagePDF has; it is above the
// first-pages cap of 16.
const sharedPageCount = 20

// sharedImagePDF has 20 pages (objs 10-29) that all reference the gray 4x2
// image obj 5 as /Im1. Page 2 references it a second time as /Logo, and page 3
// also references the RGB 3x2 image obj 6 as /Im2.
func sharedImagePDF() []byte {
	objs := []pdfObj{{1, catalog}, {5, grayImage(4, 2, "")}, {6, rgbImage(3, 2)}}
	kids := make([]int, sharedPageCount)
	for i := range sharedPageCount {
		num := 10 + i
		kids[i] = num
		xobj := "/Im1 5 0 R"
		switch i + 1 {
		case 2:
			xobj += " /Logo 5 0 R"
		case 3:
			xobj += " /Im2 6 0 R"
		}
		objs = append(objs, pdfObj{num, pageWith(xobj)})
	}
	objs = append(objs, pdfObj{2, pagesNode("", kids...)})
	return assemblePDF(objs...)
}

// orderPDF has two pages. Page 1 (obj 3) references the image obj 9; page 2
// (obj 4) references obj 8 as /Im0 and obj 7 as /Im1. Default order is first
// use page, then object number: 9, 7, 8. All three are gray 2x2.
func orderPDF() []byte {
	return assemblePDF(
		pdfObj{1, catalog},
		pdfObj{2, pagesNode("", 3, 4)},
		pdfObj{3, pageWith("/Im0 9 0 R")},
		pdfObj{4, pageWith("/Im0 8 0 R /Im1 7 0 R")},
		pdfObj{7, grayImage(2, 2, "")},
		pdfObj{8, grayImage(2, 2, "")},
		pdfObj{9, grayImage(2, 2, "")},
	)
}

// inheritedResourcesPDF puts the only /XObject dictionary on the root /Pages
// node. Pages 1 and 2 (objs 3, 4) have no /Resources and inherit it; page 3
// (obj 5) declares an empty /Resources of its own, which overrides it.
func inheritedResourcesPDF() []byte {
	return assemblePDF(
		pdfObj{1, catalog},
		pdfObj{2, pagesNode("/Resources << /XObject << /Im0 8 0 R >> >>", 3, 4, 5)},
		pdfObj{3, bareMediaPage},
		pdfObj{4, bareMediaPage},
		pdfObj{5, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> >>"},
		pdfObj{8, grayImage(2, 2, "")},
	)
}

// nestedFormsPDF reaches the image obj 8 two forms deep on page 1 (obj 3):
// /Fm1 (obj 6) -> /Fm2 (obj 7) -> /Im0. Page 1 also lists /Fm3 (obj 9), a form
// with no /Resources. Page 2 (obj 4) references obj 8 both directly as /Im9 and
// through /Fm1, so it counts page 2 once.
func nestedFormsPDF() []byte {
	return assemblePDF(
		pdfObj{1, catalog},
		pdfObj{2, pagesNode("", 3, 4)},
		pdfObj{3, pageWith("/Fm1 6 0 R /Fm3 9 0 R")},
		pdfObj{4, pageWith("/Fm1 6 0 R /Im9 8 0 R")},
		pdfObj{6, form("/Fm2 7 0 R")},
		pdfObj{7, form("/Im0 8 0 R")},
		pdfObj{8, grayImage(2, 2, "")},
		pdfObj{9, form("")},
	)
}

// formCyclePDF has a form that lists itself and a two-form cycle: page 1
// (obj 3) -> /Fm1 (obj 6), which lists /Self 6 0 R and /Fm2 (obj 7); obj 7
// lists /Back 6 0 R and the image /Im0 (obj 8).
func formCyclePDF() []byte {
	return assemblePDF(
		pdfObj{1, catalog},
		pdfObj{2, pagesNode("", 3)},
		pdfObj{3, pageWith("/Fm1 6 0 R")},
		pdfObj{6, form("/Self 6 0 R /Fm2 7 0 R")},
		pdfObj{7, form("/Back 6 0 R /Im0 8 0 R")},
		pdfObj{8, grayImage(2, 2, "")},
	)
}

// deepFormChainLength is the number of chained forms in deepFormChainPDF,
// past the form nesting cap of 32.
const deepFormChainLength = 40

// deepFormChainPDF chains 40 forms (objs 10-49) from page 1 (obj 3): each lists
// the next as /Fm. The first form also lists the image obj 5 as /ImTop; the
// last lists obj 6 as /ImDeep, which is past the nesting cap.
func deepFormChainPDF() []byte {
	objs := []pdfObj{
		{1, catalog},
		{2, pagesNode("", 3)},
		{3, pageWith("/Fm 10 0 R")},
		{5, grayImage(2, 2, "")},
		{6, grayImage(2, 2, "")},
	}
	for i := range deepFormChainLength {
		num := 10 + i
		var xobj string
		switch i {
		case 0:
			xobj = fmt.Sprintf("/ImTop 5 0 R /Fm %d 0 R", num+1)
		case deepFormChainLength - 1:
			xobj = "/ImDeep 6 0 R"
		default:
			xobj = fmt.Sprintf("/Fm %d 0 R", num+1)
		}
		objs = append(objs, pdfObj{num, form(xobj)})
	}
	return assemblePDF(objs...)
}

// noImagesPDF has two pages and no image: page 1 (obj 3) lists only a form
// with no images, page 2 (obj 4) has no /Resources.
func noImagesPDF() []byte {
	return assemblePDF(
		pdfObj{1, catalog},
		pdfObj{2, pagesNode("", 3, 4)},
		pdfObj{3, pageWith("/Fm1 6 0 R")},
		pdfObj{4, bareMediaPage},
		pdfObj{6, form("")},
	)
}

// skippedEntriesPDF lists, beside the real image obj 10, /XObject entries that
// are not images: a reference to the free object 20 (dangling), a stream with
// no /Subtype (obj 7), and a stream whose /Subtype is an indirect reference to
// the name /Image (obj 9, which image extraction refuses as not an image).
func skippedEntriesPDF() []byte {
	return assemblePDF(
		pdfObj{1, catalog},
		pdfObj{2, pagesNode("", 3)},
		pdfObj{3, pageWith("/Dangling 20 0 R /NoSub 7 0 R /IndSub 9 0 R /Im0 10 0 R")},
		pdfObj{7, streamBody("/Type /XObject", "q Q")},
		pdfObj{9, streamBody("/Type /XObject /Subtype 11 0 R /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8", "xxxx")},
		pdfObj{10, grayImage(2, 2, "")},
		pdfObj{11, "/Image"},
	)
}

// flagsPDF puts one image per badge on page 1 (obj 3):
//
//	obj 5  stencil mask, 8x1, no /ColorSpace                -> mask
//	obj 6  gray 2x2 with /SMask 7 0 R                        -> SMask "7 0 R"
//	obj 7  gray 2x2, only ever an /SMask target, not listed in any /XObject
//	obj 8  gray 2x2 with /Decode [1 0]                       -> Decode (inverting)
//	obj 9  gray 2x2 with /Decode [0 1]                       -> identity, no badge
//	obj 10 DCT RGB 8x8 with an Adobe APP14, transform 1      -> APP14 t=1
func flagsPDF(t *testing.T) []byte {
	t.Helper()
	jpg := spliceAfterAPP0(t, rgbJPEG(t), adobeAPP14(1))
	return assemblePDF(
		pdfObj{1, catalog},
		pdfObj{2, pagesNode("", 3)},
		pdfObj{3, pageWith("/Mask 5 0 R /Soft 6 0 R /Inv 8 0 R /Ident 9 0 R /Jpg 10 0 R")},
		pdfObj{5, streamBody("/Type /XObject /Subtype /Image /Width 8 /Height 1 /ImageMask true /BitsPerComponent 1", "\xaa")},
		pdfObj{6, grayImage(2, 2, "/SMask 7 0 R")},
		pdfObj{7, grayImage(2, 2, "")},
		pdfObj{8, grayImage(2, 2, "/Decode [1 0]")},
		pdfObj{9, grayImage(2, 2, "/Decode [0 1]")},
		pdfObj{10, streamBody("/Type /XObject /Subtype /Image /Width 8 /Height 8 /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode", string(jpg))},
	)
}

// malformedDecodePDF has one gray 2x2 image (obj 5) whose /Decode has an odd
// number of entries, which the decode-array reader rejects.
func malformedDecodePDF() []byte {
	return assemblePDF(
		pdfObj{1, catalog},
		pdfObj{2, pagesNode("", 3)},
		pdfObj{3, pageWith("/Im0 5 0 R")},
		pdfObj{5, grayImage(2, 2, "/Decode [0 1 0]")},
	)
}

// rgbJPEG encodes an 8x8 RGB image with a JFIF APP0 and no APP14.
func rgbJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.Set(x, y, color.RGBA{R: uint8(x * 30), G: uint8(y * 30), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode jpeg fixture: %v", err)
	}
	return buf.Bytes()
}

// adobeAPP14 returns the 16-byte Adobe APP14 segment carrying transform.
func adobeAPP14(transform byte) []byte {
	return []byte{
		0xFF, 0xEE, 0x00, 0x0E,
		'A', 'd', 'o', 'b', 'e',
		0x00, 0x64,
		0x00, 0x00,
		0x00, 0x00,
		transform,
	}
}

// spliceAfterAPP0 inserts seg directly after the JFIF APP0 of an encoded JPEG.
func spliceAfterAPP0(t *testing.T, src, seg []byte) []byte {
	t.Helper()
	at := 2
	if len(src) >= 6 && src[0] == 0xFF && src[1] == 0xD8 && src[2] == 0xFF && src[3] == 0xE0 {
		at = 4 + int(binary.BigEndian.Uint16(src[4:6]))
	}
	if at > len(src) {
		t.Fatalf("APP0 length runs past the encoded JPEG (%d > %d)", at, len(src))
	}
	out := make([]byte, 0, len(src)+len(seg))
	out = append(out, src[:at]...)
	out = append(out, seg...)
	return append(out, src[at:]...)
}

// allFixtures names every generated fixture, for checks that apply to all.
func allFixtures(t *testing.T) map[string][]byte {
	t.Helper()
	return map[string][]byte{
		"shared.pdf":           sharedImagePDF(),
		"order.pdf":            orderPDF(),
		"inherited.pdf":        inheritedResourcesPDF(),
		"nested.pdf":           nestedFormsPDF(),
		"cycle.pdf":            formCyclePDF(),
		"deep.pdf":             deepFormChainPDF(),
		"no-images.pdf":        noImagesPDF(),
		"skipped.pdf":          skippedEntriesPDF(),
		"flags.pdf":            flagsPDF(t),
		"malformed-decode.pdf": malformedDecodePDF(),
	}
}
