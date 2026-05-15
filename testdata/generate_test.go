package testdata_test

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"testing"

	pdfcpu_api "github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// minimalPDFContent returns a valid single-page PDF as raw bytes.
func minimalPDFContent() []byte {
	pdf := "%PDF-1.4\n"

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n"

	body := pdf + obj1 + obj2 + obj3

	o1 := len(pdf)
	o2 := o1 + len(obj1)
	o3 := o2 + len(obj2)
	xrefOffset := len(body)

	xref := fmt.Sprintf("xref\n0 4\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n",
		o1, o2, o3)
	trailer := fmt.Sprintf("trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOffset)

	return []byte(body + xref + trailer)
}

// multipagePDFContent returns a valid 3-page PDF as raw bytes.
func multipagePDFContent() []byte {
	pdf := "%PDF-1.4\n"

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 3 >>\nendobj\n\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n"
	obj4 := "4 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n"
	obj5 := "5 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n"

	body := pdf + obj1 + obj2 + obj3 + obj4 + obj5

	o1 := len(pdf)
	o2 := o1 + len(obj1)
	o3 := o2 + len(obj2)
	o4 := o3 + len(obj3)
	o5 := o4 + len(obj4)
	xrefOffset := len(body)

	xref := fmt.Sprintf("xref\n0 6\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n",
		o1, o2, o3, o4, o5)
	trailer := fmt.Sprintf("trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOffset)

	return []byte(body + xref + trailer)
}

// contentStreamPDFContent returns a valid single-page PDF with a content stream.
func contentStreamPDFContent() []byte {
	pdf := "%PDF-1.4\n"

	stream := "BT /F1 12 Tf 100 700 Td (Hello World) Tj ET"
	streamLen := len(stream)

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n"
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>\nendobj\n\n")
	obj4 := fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n\n", streamLen, stream)

	body := pdf + obj1 + obj2 + obj3 + obj4

	o1 := len(pdf)
	o2 := o1 + len(obj1)
	o3 := o2 + len(obj2)
	o4 := o3 + len(obj3)
	xrefOffset := len(body)

	xref := fmt.Sprintf("xref\n0 5\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n",
		o1, o2, o3, o4)
	trailer := fmt.Sprintf("trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOffset)

	return []byte(body + xref + trailer)
}

// emptyStreamPDFContent returns a valid single-page PDF with a zero-length content stream.
func emptyStreamPDFContent() []byte {
	pdf := "%PDF-1.4\n"

	stream := ""
	streamLen := len(stream)

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n"
	obj3 := fmt.Sprintf("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>\nendobj\n\n")
	obj4 := fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n\n", streamLen, stream)

	body := pdf + obj1 + obj2 + obj3 + obj4

	o1 := len(pdf)
	o2 := o1 + len(obj1)
	o3 := o2 + len(obj2)
	o4 := o3 + len(obj3)
	xrefOffset := len(body)

	xref := fmt.Sprintf("xref\n0 5\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n",
		o1, o2, o3, o4)
	trailer := fmt.Sprintf("trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOffset)

	return []byte(body + xref + trailer)
}

// imageXObjectPDFContent returns a valid single-page PDF with an embedded
// DCTDecode (JPEG) XObject image. The image is 4x4 pixels, DeviceRGB, 8bpc.
func imageXObjectPDFContent() []byte {
	// Create a 4x4 JPEG image
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var jpegBuf bytes.Buffer
	_ = jpeg.Encode(&jpegBuf, img, &jpeg.Options{Quality: 90})
	jpegBytes := jpegBuf.Bytes()

	pdf := "%PDF-1.4\n"

	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /XObject << /Im0 4 0 R >> >> >>\nendobj\n\n"
	obj4 := fmt.Sprintf("4 0 obj\n<< /Type /XObject /Subtype /Image /Width 4 /Height 4 /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length %d >>\nstream\n", len(jpegBytes))
	obj4end := "\nendstream\nendobj\n\n"

	body := pdf + obj1 + obj2 + obj3 + obj4
	bodyBytes := []byte(body)
	bodyBytes = append(bodyBytes, jpegBytes...)
	bodyBytes = append(bodyBytes, []byte(obj4end)...)

	o1 := len(pdf)
	o2 := o1 + len(obj1)
	o3 := o2 + len(obj2)
	o4 := o3 + len(obj3)
	xrefOffset := len(bodyBytes)

	xref := fmt.Sprintf("xref\n0 5\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n",
		o1, o2, o3, o4)
	trailer := fmt.Sprintf("trailer\n<< /Size 5 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOffset)

	bodyBytes = append(bodyBytes, []byte(xref+trailer)...)
	return bodyBytes
}

// fontsMixedPDFContent returns a multi-font PDF used by Story 9-9 tests.
// Covers: simple Type1 with named encoding, TrueType with /Differences, Type0
// composite with Identity-H + CIDFontType2 descendant + a bfchar ToUnicode
// CMap, and an unembedded reference font. The PDF references font streams in
// a structurally valid form; pdfcpu accepts the xref table without rendering
// the page content (the test suite only consults dict structure).
func fontsMixedPDFContent() []byte {
	pdf := "%PDF-1.7\n"

	// Object 1 -- catalog. Page tree only carries one minimal page; the
	// font dicts referenced from the page's /Resources drive the tests.
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n"
	// Pages -- references one page that lists every font in /Resources.
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] " +
		"/Resources << /Font << /F1 4 0 R /F2 5 0 R /F3 6 0 R /F4 7 0 R >> >> >>\nendobj\n\n"

	// Object 4 -- unembedded Type1 Helvetica (no FontDescriptor at all).
	obj4 := "4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>\nendobj\n\n"

	// Object 5 -- Type1 with /Differences encoding, FontDescriptor without
	// any FontFile (unembedded but has descriptor).
	obj5 := "5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /CustomFont " +
		"/FirstChar 32 /LastChar 34 " +
		"/Encoding << /Type /Encoding /BaseEncoding /WinAnsiEncoding " +
		"/Differences [32 /space /exclam /quotedbl] >> " +
		"/FontDescriptor 8 0 R >>\nendobj\n\n"

	// Object 6 -- Type0 composite with descendant CIDFontType2.
	obj6 := "6 0 obj\n<< /Type /Font /Subtype /Type0 /BaseFont /NotoSansCJK-Regular " +
		"/Encoding /Identity-H /DescendantFonts [9 0 R] /ToUnicode 10 0 R >>\nendobj\n\n"

	// Object 7 -- TrueType with FontDescriptor that has a FontFile2 stream.
	obj7 := "7 0 obj\n<< /Type /Font /Subtype /TrueType /BaseFont /MyTTFont " +
		"/FirstChar 32 /LastChar 126 /FontDescriptor 11 0 R >>\nendobj\n\n"

	// Object 8 -- FontDescriptor without FontFile (unembedded).
	obj8 := "8 0 obj\n<< /Type /FontDescriptor /FontName /CustomFont /Flags 32 " +
		"/ItalicAngle 0 /Ascent 718 /Descent -207 /CapHeight 718 /StemV 140 " +
		"/FontBBox [-170 -228 1003 962] >>\nendobj\n\n"

	// Object 9 -- CIDFontType2 descendant. References FontDescriptor 12.
	obj9 := "9 0 obj\n<< /Type /Font /Subtype /CIDFontType2 /BaseFont /NotoSansCJK-Regular " +
		"/CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> " +
		"/CIDToGIDMap /Identity /DW 1000 /FontDescriptor 12 0 R >>\nendobj\n\n"

	// Object 10 -- ToUnicode CMap stream. Minimal bfchar mapping 0x0041 -> "A".
	cmap := "/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n" +
		"/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n" +
		"/CMapName /Adobe-Identity-UCS def\n/CMapType 2 def\n" +
		"1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n" +
		"2 beginbfchar\n<0041> <0041>\n<0042> <0042>\nendbfchar\n" +
		"endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n"
	obj10 := fmt.Sprintf("10 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n\n", len(cmap), cmap)

	// Object 11 -- FontDescriptor with FontFile2 (TrueType embedded).
	obj11 := "11 0 obj\n<< /Type /FontDescriptor /FontName /MyTTFont /Flags 32 " +
		"/ItalicAngle 0 /Ascent 750 /Descent -250 /CapHeight 700 /StemV 80 " +
		"/FontBBox [-100 -200 1000 900] /FontFile2 13 0 R >>\nendobj\n\n"

	// Object 12 -- Descendant CIDFontType2's FontDescriptor (TrueType embedded).
	obj12 := "12 0 obj\n<< /Type /FontDescriptor /FontName /NotoSansCJK-Regular /Flags 4 " +
		"/ItalicAngle 0 /Ascent 880 /Descent -120 /CapHeight 880 /StemV 80 " +
		"/FontBBox [-200 -300 1100 1000] /FontFile2 14 0 R >>\nendobj\n\n"

	// Object 13 -- TrueType FontFile2 stream (minimal placeholder bytes).
	ttBytes := "TRUETYPE FONT PROGRAM PLACEHOLDER BYTES FOR TESTING"
	obj13 := fmt.Sprintf("13 0 obj\n<< /Length %d /Length1 %d >>\nstream\n%s\nendstream\nendobj\n\n", len(ttBytes), len(ttBytes), ttBytes)

	// Object 14 -- Descendant TrueType FontFile2 stream.
	ttBytes2 := "DESCENDANT TRUETYPE FONT PROGRAM PLACEHOLDER"
	obj14 := fmt.Sprintf("14 0 obj\n<< /Length %d /Length1 %d >>\nstream\n%s\nendstream\nendobj\n\n", len(ttBytes2), len(ttBytes2), ttBytes2)

	body := pdf + obj1 + obj2 + obj3 + obj4 + obj5 + obj6 + obj7 + obj8 + obj9 + obj10 + obj11 + obj12 + obj13 + obj14

	o1 := len(pdf)
	o2 := o1 + len(obj1)
	o3 := o2 + len(obj2)
	o4 := o3 + len(obj3)
	o5 := o4 + len(obj4)
	o6 := o5 + len(obj5)
	o7 := o6 + len(obj6)
	o8 := o7 + len(obj7)
	o9 := o8 + len(obj8)
	o10 := o9 + len(obj9)
	o11 := o10 + len(obj10)
	o12 := o11 + len(obj11)
	o13 := o12 + len(obj12)
	o14 := o13 + len(obj13)
	xrefOffset := len(body)

	xref := fmt.Sprintf(
		"xref\n0 15\n0000000000 65535 f \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n%010d 00000 n \n",
		o1, o2, o3, o4, o5, o6, o7, o8, o9, o10, o11, o12, o13, o14)
	trailer := fmt.Sprintf("trailer\n<< /Size 15 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefOffset)

	return []byte(body + xref + trailer)
}

// TestGenerateFixtures creates test PDF files used by the test suite.
// Run with: go test -run TestGenerateFixtures -v ./testdata/
func TestGenerateFixtures(t *testing.T) {
	t.Run("minimal.pdf", func(t *testing.T) {
		if _, err := os.Stat("minimal.pdf"); err == nil {
			t.Skip("minimal.pdf already exists")
		}
		if err := os.WriteFile("minimal.pdf", minimalPDFContent(), 0644); err != nil {
			t.Fatalf("failed to create minimal.pdf: %v", err)
		}
		// Verify pdfcpu can read it
		ctx, err := pdfcpu_api.ReadContextFile("minimal.pdf")
		if err != nil {
			os.Remove("minimal.pdf")
			t.Fatalf("minimal.pdf is not valid according to pdfcpu: %v", err)
		}
		t.Logf("minimal.pdf created: %d pages", ctx.PageCount)
	})

	t.Run("multipage.pdf", func(t *testing.T) {
		if _, err := os.Stat("multipage.pdf"); err == nil {
			t.Skip("multipage.pdf already exists")
		}
		if err := os.WriteFile("multipage.pdf", multipagePDFContent(), 0644); err != nil {
			t.Fatalf("failed to create multipage.pdf: %v", err)
		}
		ctx, err := pdfcpu_api.ReadContextFile("multipage.pdf")
		if err != nil {
			os.Remove("multipage.pdf")
			t.Fatalf("multipage.pdf is not valid according to pdfcpu: %v", err)
		}
		t.Logf("multipage.pdf created: %d pages", ctx.PageCount)
	})

	t.Run("content-stream.pdf", func(t *testing.T) {
		if _, err := os.Stat("content-stream.pdf"); err == nil {
			t.Skip("content-stream.pdf already exists")
		}
		if err := os.WriteFile("content-stream.pdf", contentStreamPDFContent(), 0644); err != nil {
			t.Fatalf("failed to create content-stream.pdf: %v", err)
		}
		ctx, err := pdfcpu_api.ReadContextFile("content-stream.pdf")
		if err != nil {
			os.Remove("content-stream.pdf")
			t.Fatalf("content-stream.pdf is not valid according to pdfcpu: %v", err)
		}
		t.Logf("content-stream.pdf created: %d pages", ctx.PageCount)
	})

	t.Run("empty-stream.pdf", func(t *testing.T) {
		if _, err := os.Stat("empty-stream.pdf"); err == nil {
			t.Skip("empty-stream.pdf already exists")
		}
		if err := os.WriteFile("empty-stream.pdf", emptyStreamPDFContent(), 0644); err != nil {
			t.Fatalf("failed to create empty-stream.pdf: %v", err)
		}
		ctx, err := pdfcpu_api.ReadContextFile("empty-stream.pdf")
		if err != nil {
			os.Remove("empty-stream.pdf")
			t.Fatalf("empty-stream.pdf is not valid according to pdfcpu: %v", err)
		}
		t.Logf("empty-stream.pdf created: %d pages", ctx.PageCount)
	})

	t.Run("malformed.pdf", func(t *testing.T) {
		if _, err := os.Stat("malformed.pdf"); err == nil {
			t.Skip("malformed.pdf already exists")
		}
		data := []byte("%PDF-1.4\n%%EOF\ngarbage xref corrupted data truncated")
		if err := os.WriteFile("malformed.pdf", data, 0644); err != nil {
			t.Fatalf("failed to create malformed.pdf: %v", err)
		}
	})

	t.Run("encrypted.pdf", func(t *testing.T) {
		if _, err := os.Stat("encrypted.pdf"); err == nil {
			t.Skip("encrypted.pdf already exists")
		}
		// Ensure minimal.pdf exists first
		if _, err := os.Stat("minimal.pdf"); os.IsNotExist(err) {
			if err := os.WriteFile("minimal.pdf", minimalPDFContent(), 0644); err != nil {
				t.Fatalf("failed to create minimal.pdf for encryption: %v", err)
			}
		}
		conf := pdfcpu_model.NewAESConfiguration("testpass", "ownerpass", 256)
		if err := pdfcpu_api.EncryptFile("minimal.pdf", "encrypted.pdf", conf); err != nil {
			t.Fatalf("failed to create encrypted.pdf: %v", err)
		}
	})

	t.Run("fonts-mixed.pdf", func(t *testing.T) {
		if _, err := os.Stat("fonts-mixed.pdf"); err == nil {
			t.Skip("fonts-mixed.pdf already exists")
		}
		if err := os.WriteFile("fonts-mixed.pdf", fontsMixedPDFContent(), 0644); err != nil {
			t.Fatalf("failed to create fonts-mixed.pdf: %v", err)
		}
		// Verify pdfcpu can read it.
		ctx, err := pdfcpu_api.ReadContextFile("fonts-mixed.pdf")
		if err != nil {
			os.Remove("fonts-mixed.pdf")
			t.Fatalf("fonts-mixed.pdf is not valid according to pdfcpu: %v", err)
		}
		t.Logf("fonts-mixed.pdf created: %d pages", ctx.PageCount)
	})

	t.Run("image-xobject.pdf", func(t *testing.T) {
		if _, err := os.Stat("image-xobject.pdf"); err == nil {
			t.Skip("image-xobject.pdf already exists")
		}
		if err := os.WriteFile("image-xobject.pdf", imageXObjectPDFContent(), 0644); err != nil {
			t.Fatalf("failed to create image-xobject.pdf: %v", err)
		}
		ctx, err := pdfcpu_api.ReadContextFile("image-xobject.pdf")
		if err != nil {
			os.Remove("image-xobject.pdf")
			t.Fatalf("image-xobject.pdf is not valid according to pdfcpu: %v", err)
		}
		t.Logf("image-xobject.pdf created: %d pages", ctx.PageCount)
	})
}
