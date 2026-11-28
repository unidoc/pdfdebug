package pdfcore

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	pdfcpu_api "github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/filter"
	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Inspector.Open parses with pdfcpu's reader only. A file that parses opens and
// shows what it contains, including shapes pdfcpu's validator refuses or
// repairs in place.

// openParsedFile writes pdf to a temp file and opens it through Inspector.Open,
// failing the test if Open refuses it.
func openParsedFile(t *testing.T, tabID string, pdf []byte) (*Inspector, *DocumentInfo) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "doc.pdf")
	if err := os.WriteFile(path, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	ins := NewInspector()
	info, err := ins.Open(tabID, path)
	if err != nil {
		t.Fatalf("Open refused a file that parses: %v", err)
	}
	t.Cleanup(func() { _ = ins.Close(tabID) })
	return ins, info
}

// within runs fn on its own goroutine and fails the test if it panics or has
// not returned after d, so a cyclic structure cannot hang or crash the run. fn
// must not call t.Fatal.
func within(t *testing.T, d time.Duration, what string, fn func()) {
	t.Helper()
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		fn()
	}()
	select {
	case p := <-done:
		if p != nil {
			t.Fatalf("%s panicked: %v", what, p)
		}
	case <-time.After(d):
		t.Fatalf("%s did not return within %s", what, d)
	}
}

func rawStreamObj(dict, data string) string {
	return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", dict, len(data), data)
}

func TestOpen_StructureTreeCycleFileOpens(t *testing.T) {
	ins := NewInspector()
	if _, err := ins.Open("broken", filepath.Join(testdataDir(t), "structure-broken.pdf")); err != nil {
		t.Fatalf("structure-broken.pdf must open so its findings can be shown, got %v", err)
	}
	_ = ins.Close("broken")
}

func TestOpen_KidsCycleOpensAndPageIndexReturns(t *testing.T) {
	pdf := rawPDF(
		rawObj{1, "<< /Type /Catalog /Pages 2 0 R >>"},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>"},
		rawObj{4, "<< /Type /Pages /Parent 2 0 R /Kids [2 0 R] /Count 1 >>"},
	)
	path := filepath.Join(t.TempDir(), "doc.pdf")
	if err := os.WriteFile(path, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	ins := NewInspector()
	t.Cleanup(func() { _ = ins.Close("kids-cycle") })
	var openErr error
	within(t, 10*time.Second, "Open on a /Kids cycle", func() {
		_, openErr = ins.Open("kids-cycle", path)
	})
	if openErr != nil {
		t.Fatalf("Open refused a file that parses: %v", openErr)
	}
	within(t, 10*time.Second, "GetPageIndex on a /Kids cycle", func() {
		_, _ = ins.GetPageIndex("kids-cycle")
	})
}

func TestOpen_PagesWithoutCountOpensWithStructuralWarning(t *testing.T) {
	pdf := rawPDF(
		rawObj{1, "<< /Type /Catalog /Pages 2 0 R >>"},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R] >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>"},
	)
	ins, info := openParsedFile(t, "no-count", pdf)
	const want = "This PDF has structural errors. Some objects may display incorrectly."
	if info.Error != want {
		t.Errorf("open warning = %q, want the page-count warning %q", info.Error, want)
	}
	entries, err := ins.GetPageIndex("no-count")
	if err != nil {
		t.Fatalf("GetPageIndex: %v", err)
	}
	found := false
	for _, e := range entries {
		if e.ObjNum == 3 {
			found = true
		}
	}
	if !found {
		t.Errorf("page index %+v does not list the page leaf, object 3", entries)
	}
}

func TestOpen_UndecodableMetadataDegradesToEmptyXMPAndWarning(t *testing.T) {
	garbage := "this-is-not-valid-flate-data"
	pdf := rawPDF(
		rawObj{1, "<< /Type /Catalog /Pages 2 0 R /Metadata 4 0 R >>"},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>"},
		rawObj{4, rawStreamObj("/Type /Metadata /Subtype /XML /Filter /FlateDecode", garbage)},
	)
	ins, _ := openParsedFile(t, "bad-xmp", pdf)
	md, err := ins.GetDocumentMetadata("bad-xmp")
	if err != nil {
		t.Fatalf("GetDocumentMetadata must not fail on an undecodable /Metadata, got %v", err)
	}
	if md.XMP != "" {
		t.Errorf("XMP = %q, want empty", md.XMP)
	}
	if !strings.Contains(md.Warning, "zlib") {
		t.Errorf("warning = %q, want one naming the zlib decode failure", md.Warning)
	}
}

// pdfcpu's validator writes /Subtype /Form into an XObject that has none. The
// object must reach the viewer as the file stores it.
func TestOpen_XObjectWithoutSubtypeIsShownAsWritten(t *testing.T) {
	pdf := rawPDF(
		rawObj{1, "<< /Type /Catalog /Pages 2 0 R >>"},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /XObject << /No0 5 0 R >> >> /Contents 4 0 R >>"},
		rawObj{4, rawStreamObj("", "q /No0 Do Q\n")},
		rawObj{5, rawStreamObj("/Type /XObject /FormType 1 /BBox [0 0 10 10]", "0 0 10 10 re f\n")},
	)
	ins, _ := openParsedFile(t, "no-subtype", pdf)
	src, err := ins.GetObjectSource("no-subtype", "obj:0:5")
	if err != nil {
		t.Fatalf("GetObjectSource: %v", err)
	}
	if !strings.Contains(src, "/XObject") {
		t.Fatalf("source of object 5 does not look like the XObject:\n%s", src)
	}
	if strings.Contains(src, "/Subtype") {
		t.Errorf("object 5 has no /Subtype in the file, but the viewer shows one:\n%s", src)
	}
}

func TestOpen_EncryptedAndMalformedFixturesStillRefused(t *testing.T) {
	for _, tc := range []struct {
		file string
		want error
	}{
		{"encrypted.pdf", ErrEncryptedPDF},
		{"malformed.pdf", ErrMalformedPDF},
	} {
		ins := NewInspector()
		_, err := ins.Open("refused", filepath.Join(testdataDir(t), tc.file))
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: Open error = %v, want %v", tc.file, err, tc.want)
		}
	}
}

// Objects stored in an object stream are decoded at open, so walkers that read
// the cross-reference table directly see a font kept in one.
func TestOpen_ObjectStreamObjectsAreDecoded(t *testing.T) {
	src := assemblexref(
		"%PDF-1.4\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> >>\nendobj\n",
		"4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n",
	)
	ctx, err := pdfcpu_api.ReadContext(context.Background(), bytes.NewReader(src), pdfcpu_model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("ReadContext: %v", err)
	}
	ctx.WriteObjectStream = true
	ctx.WriteXRefStream = true
	var out bytes.Buffer
	if err := pdfcpu_api.WriteContext(context.Background(), ctx, &out); err != nil {
		t.Fatalf("WriteContext: %v", err)
	}

	raw, err := pdfcpu_api.ReadContext(context.Background(), bytes.NewReader(out.Bytes()), pdfcpu_model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("ReadContext of the rewritten file: %v", err)
	}
	lazy := 0
	for _, e := range raw.XRefTable.Table {
		if _, ok := e.Object.(pdfcpu_types.LazyObjectStreamObject); ok {
			lazy++
		}
	}
	if lazy == 0 {
		t.Fatal("fixture: the rewritten file keeps no object in an object stream")
	}

	ins, _ := openParsedFile(t, "objstm", out.Bytes())
	doc, err := ins.GetDocument("objstm")
	if err != nil {
		t.Fatal(err)
	}
	for nr, e := range doc.PDFContext.XRefTable.Table {
		if _, ok := e.Object.(pdfcpu_types.LazyObjectStreamObject); ok {
			t.Errorf("object %d is still a lazy object-stream placeholder after Open", nr)
		}
	}
	res, err := ins.Validate("objstm", ProfilePDFA1B)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if problemByRule(res.Problems, "font-embedding") == nil {
		t.Errorf("font-embedding did not fire on a non-embedded font kept in an object stream:\n%+v", res.Problems)
	}
}

// objectStreamFile rewrites a four-object document so its objects are kept in
// an object stream, and returns the file with the offset of that stream's data.
func objectStreamFile(t *testing.T) ([]byte, int) {
	t.Helper()
	src := assemblexref(
		"%PDF-1.4\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> >>\nendobj\n",
		"4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n",
	)
	ctx, err := pdfcpu_api.ReadContext(context.Background(), bytes.NewReader(src), pdfcpu_model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("ReadContext: %v", err)
	}
	ctx.WriteObjectStream = true
	ctx.WriteXRefStream = true
	var out bytes.Buffer
	if err := pdfcpu_api.WriteContext(context.Background(), ctx, &out); err != nil {
		t.Fatalf("WriteContext: %v", err)
	}
	b := out.Bytes()
	m := checksumStreamType.FindIndex(b)
	if m == nil {
		t.Fatal("fixture: the rewritten file has no object stream")
	}
	start := m[1] + bytes.Index(b[m[1]:], []byte("stream")) + len("stream")
	for b[start] == '\r' || b[start] == '\n' {
		start++
	}
	return b, start
}

// An object stream whose Adler-32 trailer is wrong, with intact deflate data,
// still opens and its objects resolve.
func TestOpen_ObjectStreamWithBrokenChecksumOpens(t *testing.T) {
	b, start := objectStreamFile(t)
	b = breakObjectStreamTrailer(b, start)

	if _, checksumFailed, err := readPDF(context.Background(), bytes.NewReader(b)); err != nil || len(checksumFailed) == 0 {
		t.Fatal("fixture: pdfcpu decodes the broken-checksum object stream, so the fallback is not exercised")
	}
	ins, info := openParsedFile(t, "objstm-checksum", b)
	if info.PageCount != 1 || info.Error != "" {
		t.Errorf("open = page count %d, warning %q; want 1 page and no warning", info.PageCount, info.Error)
	}
	doc, err := ins.GetDocument("objstm-checksum")
	if err != nil {
		t.Fatal(err)
	}
	for nr, e := range doc.PDFContext.XRefTable.Table {
		if _, ok := e.Object.(pdfcpu_types.LazyObjectStreamObject); ok {
			t.Errorf("object %d is still a lazy object-stream placeholder after Open", nr)
		}
	}
	font, err := doc.PDFContext.Dereference(*pdfcpu_types.NewIndirectRef(4, 0))
	if d, ok := font.(pdfcpu_types.Dict); err != nil || !ok || d.NameEntry("BaseFont") == nil || *d.NameEntry("BaseFont") != "Helvetica" {
		t.Errorf("object 4 = %v, %v; want the Helvetica font dictionary", font, err)
	}
	if r, ok := doc.PDFContext.Read.RS.(*bytes.Reader); !ok || r.Size() != 0 {
		t.Errorf("context reader = %T; want an empty *bytes.Reader, so the corrected copy of the file is not kept", doc.PDFContext.Read.RS)
	}
}

// checksumFileOpts shapes the file checksumFile builds.
type checksumFileOpts struct {
	objStmType   string // the /Type entry of object stream 4
	xrefType     string // the /Type entry of cross-reference stream 5
	breakObjStm  bool   // flip the last byte of object stream 4's Adler-32 trailer
	breakXRef    bool   // flip the last byte of cross-reference stream 5's Adler-32 trailer
	breakFontStm bool   // flip the last byte of object stream 8's Adler-32 trailer
	streamEOL    string // what follows each stream keyword; "\n" when empty
}

// checksumFile builds a file whose catalog, page tree and page are kept in
// Flate object stream 4, indexed by Flate cross-reference stream 5 with no
// /DecodeParms. A font, object 7, is kept in Flate object stream 8, whose
// /Type is an indirect reference to object 6, the name /ObjStm.
func checksumFile(o checksumFileOpts) []byte {
	deflate := func(b []byte) []byte {
		var z bytes.Buffer
		zw := zlib.NewWriter(&z)
		_, _ = zw.Write(b)
		_ = zw.Close()
		return z.Bytes()
	}
	objStm := func(first int, objs []string) (head string, data []byte) {
		var body string
		for i, obj := range objs {
			head += fmt.Sprintf("%d %d ", first+i, len(body))
			body += obj + "\n"
		}
		return head, deflate([]byte(head + body))
	}
	type row struct{ typ, field2, field3 int }
	rows := map[int]row{0: {0, 0, 255}}
	out := []byte("%PDF-1.5\n%\xe2\xe3\xcf\xd3\n")
	eol := o.streamEOL
	if eol == "" {
		eol = "\n"
	}
	stream := func(nr int, dict string, data []byte, broken bool) {
		rows[nr] = row{1, len(out), 0}
		if broken {
			data = bytes.Clone(data)
			data[len(data)-1] ^= 0xFF
		}
		out = fmt.Appendf(out, "%d 0 obj\n<< %s /Filter /FlateDecode /Length %d >>\nstream%s", nr, dict, len(data), eol)
		out = append(out, data...)
		out = append(out, "\nendstream\nendobj\n"...)
	}

	head, data := objStm(1, []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
	})
	for i := range 3 {
		rows[1+i] = row{2, 4, i}
	}
	stream(4, fmt.Sprintf("%s /N 3 /First %d", o.objStmType, len(head)), data, o.breakObjStm)
	rows[6] = row{1, len(out), 0}
	out = append(out, "6 0 obj\n/ObjStm\nendobj\n"...)
	head, data = objStm(7, []string{"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"})
	rows[7] = row{2, 8, 0}
	stream(8, fmt.Sprintf("/Type 6 0 R /N 1 /First %d", len(head)), data, o.breakFontStm)
	const size = 9

	xrefOff := len(out)
	rows[5] = row{1, xrefOff, 0}
	var raw []byte
	for nr := range size {
		r := rows[nr]
		raw = append(raw, byte(r.typ), byte(r.field2>>16), byte(r.field2>>8), byte(r.field2), byte(r.field3))
	}
	stream(5, fmt.Sprintf("%s /Size %d /Root 1 0 R /W [1 3 1]", o.xrefType, size), deflate(raw), o.breakXRef)
	return fmt.Appendf(out, "startxref\n%d\n%%%%EOF\n", xrefOff)
}

// openChecksumFile opens pdf through Inspector.Open and checks that it has one
// page, no warning, that object 3 is the page and, with font set, that object 7
// is the font.
func openChecksumFile(t *testing.T, pdf []byte, font bool) {
	t.Helper()
	ins, info := openParsedFile(t, "checksum", pdf)
	if info.PageCount != 1 || info.Error != "" {
		t.Errorf("open = page count %d, warning %q; want 1 page and no warning", info.PageCount, info.Error)
	}
	doc, err := ins.GetDocument("checksum")
	if err != nil {
		t.Fatal(err)
	}
	page, err := doc.PDFContext.Dereference(*pdfcpu_types.NewIndirectRef(3, 0))
	if d, ok := page.(pdfcpu_types.Dict); err != nil || !ok || d.Type() == nil || *d.Type() != "Page" {
		t.Errorf("object 3 = %v, %v; want the page dictionary", page, err)
	}
	if font {
		f, err := doc.PDFContext.Dereference(*pdfcpu_types.NewIndirectRef(7, 0))
		if d, ok := f.(pdfcpu_types.Dict); err != nil || !ok || d.NameEntry("BaseFont") == nil || *d.NameEntry("BaseFont") != "Helvetica" {
			t.Errorf("object 7 = %v, %v; want the Helvetica font dictionary", f, err)
		}
	}
	if r, ok := doc.PDFContext.Read.RS.(*bytes.Reader); !ok || r.Size() != 0 {
		t.Errorf("context reader = %T; want an empty *bytes.Reader, so the corrected copy of the file is not kept", doc.PDFContext.Read.RS)
	}
}

// A cross-reference stream whose Adler-32 trailer is wrong, with intact
// deflate data, still opens, and the catalog, page tree and page kept in an
// object stream it indexes resolve.
func TestOpen_XRefStreamWithBrokenChecksumOpens(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "xref-stream-checksum.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := readPDF(context.Background(), bytes.NewReader(b)); err == nil {
		t.Fatal("fixture: pdfcpu parses the file with the broken cross-reference stream trailer, so the repair is not exercised")
	}
	openChecksumFile(t, b, false)
}

// Streams whose /Type is #-escaped, separated from its key by a comment or
// NUL, or an indirect reference, have their broken trailers corrected. An
// object stream with an indirect /Type is found by its offset in the parsed
// cross-reference table, also when that table was read from a corrected
// cross-reference stream.
func TestOpen_BrokenChecksumUnderEveryTypeSpelling(t *testing.T) {
	cases := []struct {
		name string
		opts checksumFileOpts
	}{
		{"escaped xref type", checksumFileOpts{objStmType: "/Type /ObjStm", xrefType: "/Typ#65 /XR#65f", breakXRef: true}},
		{"comment and NUL before xref type", checksumFileOpts{objStmType: "/Type /ObjStm", xrefType: "/Type %c\n\x00/XRef", breakXRef: true}},
		{"escaped object stream type", checksumFileOpts{objStmType: "/Type /Obj#53tm", xrefType: "/Type /XRef", breakObjStm: true}},
		{"comment and NUL before object stream type", checksumFileOpts{objStmType: "/Type%c\r\x00/ObjStm", xrefType: "/Type /XRef", breakObjStm: true}},
		{"indirect object stream type", checksumFileOpts{objStmType: "/Type /ObjStm", xrefType: "/Type /XRef", breakFontStm: true}},
		{"indirect object stream type and broken xref", checksumFileOpts{objStmType: "/Type /ObjStm", xrefType: "/Type /XRef", breakFontStm: true, breakXRef: true}},
		{"space before stream EOL, broken xref", checksumFileOpts{objStmType: "/Type /ObjStm", xrefType: "/Type /XRef", breakXRef: true, streamEOL: " \n"}},
		{"space before stream EOL, broken object stream", checksumFileOpts{objStmType: "/Type /ObjStm", xrefType: "/Type /XRef", breakObjStm: true, streamEOL: " \n"}},
		{"spaces before stream CRLF, broken xref", checksumFileOpts{objStmType: "/Type /ObjStm", xrefType: "/Type /XRef", breakXRef: true, streamEOL: "  \r\n"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			intact := tc.opts
			intact.breakObjStm, intact.breakXRef, intact.breakFontStm = false, false, false
			if _, _, err := readPDF(context.Background(), bytes.NewReader(checksumFile(intact))); err != nil {
				t.Fatalf("fixture: pdfcpu refuses the file with intact trailers: %v", err)
			}
			b := checksumFile(tc.opts)
			if _, failed, err := readPDF(context.Background(), bytes.NewReader(b)); err == nil && len(failed) == 0 {
				t.Fatal("fixture: pdfcpu reads the file with the broken trailer, so the repair is not exercised")
			}
			openChecksumFile(t, b, true)
		})
	}
}

// The offset repair leaves object streams with a correct trailer unchanged.
func TestFixChecksumsAt_LeavesIntactStreamsAlone(t *testing.T) {
	b := checksumFile(checksumFileOpts{objStmType: "/Type /ObjStm", xrefType: "/Type /XRef"})
	ctx, _, err := readPDF(context.Background(), bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	fixed := bytes.Clone(b)
	budget := filter.DefaultMaxDecodeBytes
	if fixChecksumsAt(fixed, objectStreamDataOffsets(ctx.XRefTable, []int{4, 8}), &budget) || !bytes.Equal(fixed, b) {
		t.Error("an object stream with a correct trailer was changed")
	}
}

// fixTyped runs fixTypedStreamChecksums on a copy of data and returns the
// copy.
func fixTyped(data []byte, budget int64) ([]byte, bool) {
	fixed := bytes.Clone(data)
	ok := fixTypedStreamChecksums(fixed, &budget)
	return fixed, ok
}

// Only a wrong trailer on a zlib stream that inflates is rewritten.
func TestFixZlibChecksum_LeavesOtherStreamsAlone(t *testing.T) {
	b, start := objectStreamFile(t)
	if fixed, ok := fixTyped(b, filter.DefaultMaxDecodeBytes); ok || !bytes.Equal(fixed, b) {
		t.Error("an object stream with a correct trailer was changed")
	}
	corrupt := bytes.Clone(b)
	corrupt[start+4] ^= 0xFF // inside the deflate data
	if fixed, ok := fixTyped(corrupt, filter.DefaultMaxDecodeBytes); ok || !bytes.Equal(fixed, corrupt) {
		t.Error("an object stream whose deflate data does not inflate cleanly was changed")
	}
}

// A zlib object stream that ends without its Adler-32 trailer is left alone:
// the four bytes after its deflate data belong to the endstream keyword.
func TestFixTypedStreamChecksums_StreamWithoutTrailerIsLeftAlone(t *testing.T) {
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	_, _ = zw.Write([]byte("1 0 << /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"))
	_ = zw.Close()
	body := z.Bytes()[:z.Len()-4]
	for _, eol := range []string{"\n", "\r\n", ""} {
		data := []byte("5 0 obj\n<< /Type /ObjStm /N 1 /First 4 /Length " + strconv.Itoa(len(body)) + " >>\nstream\n")
		data = append(data, body...)
		data = append(data, eol+"endstream\nendobj\n"...)
		if fixed, ok := fixTyped(data, filter.DefaultMaxDecodeBytes); ok || !bytes.Equal(fixed, data) {
			t.Errorf("EOL %q before endstream: bytes after a trailer-less stream were rewritten", eol)
		}
	}
}

// breakObjectStreamTrailer returns a copy of b, the objectStreamFile bytes,
// with the last byte of the object stream's Adler-32 trailer flipped.
func breakObjectStreamTrailer(b []byte, start int) []byte {
	broken := bytes.Clone(b)
	end := start + bytes.Index(broken[start:], []byte("endstream"))
	for broken[end-1] == '\r' || broken[end-1] == '\n' {
		end--
	}
	broken[end-1] ^= 0xFF
	return broken
}

// Many /Type /ObjStm entries ahead of one stream inflate it once, so they do
// not spend the budget that a later broken object stream needs.
func TestFixTypedStreamChecksums_RepeatedTypeEntriesInflateTheirStreamOnce(t *testing.T) {
	b, start := objectStreamFile(t)
	const size = 64 << 10
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	_, _ = zw.Write(make([]byte, size))
	_ = zw.Close()

	head := []byte(strings.Repeat("%/Type /ObjStm\n", 64) + "9 0 obj\n<< /Length 1 >>\nstream\n")
	head = append(head, z.Bytes()...)
	head = append(head, "\nendstream\nendobj\n"...)
	data := append(bytes.Clone(head), breakObjectStreamTrailer(b, start)...)

	fixed, ok := fixTyped(data, 2*size)
	if !ok || !bytes.Equal(fixed, append(head, b...)) {
		t.Errorf("ok = %v; want the broken trailer after the decoy entries corrected and nothing else changed", ok)
	}
}

// Once the inflate budget is spent no further trailer is corrected.
func TestFixTypedStreamChecksums_StopsWhenTheBudgetIsSpent(t *testing.T) {
	b, start := objectStreamFile(t)
	broken := breakObjectStreamTrailer(b, start)
	if fixed, ok := fixTyped(broken, 1); ok || !bytes.Equal(fixed, broken) {
		t.Error("a stream that inflates past the budget had its trailer changed")
	}
}

// /Type /ObjStm entries that each end at an endobj before a far stream
// keyword are scanned in linear time.
func TestFixTypedStreamChecksums_ScanIsLinear(t *testing.T) {
	data := []byte(strings.Repeat("/Type /ObjStm endobj ", 500_000) + "stream\n")
	within(t, 5*time.Second, "fixTypedStreamChecksums over 500000 entries", func() {
		budget := filter.DefaultMaxDecodeBytes
		_ = fixTypedStreamChecksums(data, &budget)
	})
}

// A signature field listing itself in /Kids, twice, is walked once.
func TestOpen_SignatureFieldCycleReturns(t *testing.T) {
	ins, _ := openParsedFile(t, "sigcycle", assemblexref(
		"%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] >> >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>\nendobj\n",
		"4 0 obj\n<< /T (a) /FT /Sig /Kids [4 0 R 4 0 R] >>\nendobj\n",
	))
	within(t, 5*time.Second, "GetSignatures on a field cycle", func() {
		_, _ = ins.GetSignatures("sigcycle")
	})
}

// An /EmbeddedFiles name-tree node listing itself in /Kids, twice, is walked
// once.
func TestOpen_EmbeddedFilesNameTreeCycleReturns(t *testing.T) {
	ins, _ := openParsedFile(t, "namecycle", assemblexref(
		"%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Names << /EmbeddedFiles 4 0 R >> >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>\nendobj\n",
		"4 0 obj\n<< /Kids [4 0 R 4 0 R] >>\nendobj\n",
	))
	within(t, 5*time.Second, "GetEmbeddedFiles on a name-tree cycle", func() {
		_, _ = ins.GetEmbeddedFiles("namecycle")
	})
}

// sharedChain returns objects 4..4+n-1, each built by node from its own
// number and the next one's, for a chain in which every node lists the next
// node twice.
func sharedChain(n int, node func(nr, next int) string) []string {
	objs := make([]string, 0, n)
	for i := range n {
		nr := 4 + i
		objs = append(objs, fmt.Sprintf("%d 0 obj\n%s\nendobj\n", nr, node(nr, nr+1)))
	}
	return objs
}

// A name tree whose intermediate nodes each list the next node twice is
// walked once per node, and the leaf's file is listed once.
func TestOpen_EmbeddedFilesSharedNameTreeNodesWalkedOnce(t *testing.T) {
	const depth = 40
	parts := []string{
		"%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Names << /EmbeddedFiles 4 0 R >> >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>\nendobj\n",
	}
	parts = append(parts, sharedChain(depth, func(nr, next int) string {
		if nr == 4+depth-1 {
			return fmt.Sprintf("<< /Names [(a.txt) %d 0 R] >>", next)
		}
		return fmt.Sprintf("<< /Kids [%d 0 R %d 0 R] >>", next, next)
	})...)
	parts = append(parts, fmt.Sprintf("%d 0 obj\n<< /Type /Filespec /F (a.txt) >>\nendobj\n", 4+depth))
	ins, _ := openParsedFile(t, "shared-names", assemblexref(parts...))
	var list *EmbeddedFileList
	var err error
	within(t, 5*time.Second, "GetEmbeddedFiles on shared name-tree nodes", func() {
		list, err = ins.GetEmbeddedFiles("shared-names")
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Files) != 1 {
		t.Errorf("listed %d embedded files, want 1: %+v", len(list.Files), list.Files)
	}
}

// A field tree whose fields each list the next field twice is walked once per
// field, and the terminal signature field is listed once.
func TestOpen_SharedSignatureFieldsWalkedOnce(t *testing.T) {
	const depth = 40
	parts := []string{
		"%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] >> >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>\nendobj\n",
	}
	parts = append(parts, sharedChain(depth, func(nr, next int) string {
		if nr == 4+depth-1 {
			return "<< /T (s) /FT /Sig >>"
		}
		return fmt.Sprintf("<< /T (f%d) /Kids [%d 0 R %d 0 R] >>", nr, next, next)
	})...)
	ins, _ := openParsedFile(t, "shared-sigs", assemblexref(parts...))
	var list *SignatureList
	var err error
	within(t, 5*time.Second, "GetSignatures on shared signature fields", func() {
		list, err = ins.GetSignatures("shared-sigs")
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Signatures) != 1 {
		t.Errorf("listed %d signature fields, want 1: %+v", len(list.Signatures), list.Signatures)
	}
}

// A negative root /Count is treated like a missing one: the document opens
// with the page-count warning and a page count of 0.
func TestOpen_NegativePageCountOpensWithStructuralWarning(t *testing.T) {
	_, info := openParsedFile(t, "negative-count", rawPDF(
		rawObj{1, "<< /Type /Catalog /Pages 2 0 R >>"},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R] /Count -2 >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>"},
	))
	const want = "This PDF has structural errors. Some objects may display incorrectly."
	if info.Error != want || info.PageCount != 0 {
		t.Errorf("open = warning %q, page count %d; want %q and 0", info.Error, info.PageCount, want)
	}
}

// An image /SMask reference to a missing object or to an object whose body is
// null is the null object: the image renders without a soft mask instead of
// faulting pdfcpu's renderer, and the document keeps its /SMask entry.
func TestGetImageData_SMaskResolvingToNullRendersWithoutMask(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra []rawObj
	}{
		{"dangling reference", nil},
		{"null object", []rawObj{{6, "null"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objs := append([]rawObj{
				{1, rawCatalog},
				{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
				{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /XObject << /Im1 5 0 R >> >> >>"},
				{5, rawStreamObj("/Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /SMask 6 0 R", "\x01\x02\x03\x04")},
			}, tc.extra...)
			ins, _ := openParsedFile(t, "smask", rawPDF(objs...))
			var data *ImageData
			var err error
			within(t, 5*time.Second, "GetImageData on an image with a null /SMask", func() {
				data, err = ins.GetImageData(context.Background(), "smask", "obj:0:5")
			})
			if err != nil {
				t.Fatalf("GetImageData: %v", err)
			}
			if data.Error != "" || data.MimeType == "" {
				t.Errorf("image = error %q, mime %q; want a rendered image", data.Error, data.MimeType)
			}
			if data.SMask == nil || *data.SMask != "6 0 R" {
				t.Errorf("SMask = %v; want the reference as written, 6 0 R", data.SMask)
			}
			within(t, 5*time.Second, "GetImageBytes on an image with a null /SMask", func() {
				_, _, err = ins.GetImageBytes("smask", "obj:0:5")
			})
			if err != nil {
				t.Errorf("GetImageBytes: %v", err)
			}
			doc, err := ins.GetDocument("smask")
			if err != nil {
				t.Fatal(err)
			}
			img, _, err := doc.PDFContext.DereferenceStreamDict(*pdfcpu_types.NewIndirectRef(5, 0))
			if err != nil || img == nil {
				t.Fatalf("image object after render: %v", err)
			}
			if ref := img.IndirectRefEntry("SMask"); ref == nil || ref.ObjectNumber.Value() != 6 {
				t.Errorf("image /SMask after render = %v; want the entry as written, 6 0 R", img.Dict["SMask"])
			}
		})
	}
}

// brokenChecksumImageFile builds a one-page file with a 2x2 DeviceGray Flate
// image (object 5) and, when mask is non-nil, a Flate /SMask (object 6) whose
// compressed data is mask.
func brokenChecksumImageFile(image, mask []byte) []byte {
	imgDict := "/Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /Filter /FlateDecode"
	objs := []rawObj{
		{1, rawCatalog},
		{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /XObject << /Im1 5 0 R >> >> >>"},
	}
	if mask != nil {
		imgDict += " /SMask 6 0 R"
		objs = append(objs, rawObj{6, rawStreamObj("/Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /Filter /FlateDecode", string(mask))})
	}
	objs = append(objs, rawObj{5, rawStreamObj(imgDict, string(image))})
	return rawPDF(objs...)
}

// A Flate image whose data has a broken Adler-32 checksum renders to the same
// bytes as the same image with a correct checksum.
func TestGetImageData_BrokenChecksumFlateImageRenders(t *testing.T) {
	pixels := []byte{0x00, 0x55, 0xaa, 0xff}

	intact, _ := openParsedFile(t, "intact", brokenChecksumImageFile(zlibBytes(t, pixels), nil))
	want, _, err := intact.GetImageBytes("intact", "obj:0:5")
	if err != nil {
		t.Fatalf("GetImageBytes on the intact image: %v", err)
	}

	ins, _ := openParsedFile(t, "broken", brokenChecksumImageFile(brokenChecksum(t, pixels), nil))
	var data *ImageData
	within(t, 5*time.Second, "GetImageData on a broken-checksum image", func() {
		data, err = ins.GetImageData(context.Background(), "broken", "obj:0:5")
	})
	if err != nil {
		t.Fatalf("GetImageData: %v", err)
	}
	if data.Error != "" || data.Kind != imageKindOK || data.MimeType == "" {
		t.Fatalf("image = error %q, kind %q, mime %q; want a rendered image", data.Error, data.Kind, data.MimeType)
	}
	got, _, err := ins.GetImageBytes("broken", "obj:0:5")
	if err != nil {
		t.Fatalf("GetImageBytes on the broken-checksum image: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("broken-checksum image rendered %d bytes that differ from the intact render (%d bytes)", len(got), len(want))
	}
}

// pdfcpu decodes an image's /SMask itself when rendering, so a soft mask with a
// broken Adler-32 checksum fails the render: GetImageData returns no error and
// reports the soft-mask decode failure in ImageData.Error. The same mask with a
// correct checksum renders.
func TestGetImageData_BrokenChecksumSoftMaskIsAPerImageError(t *testing.T) {
	pixels := []byte{0x00, 0x55, 0xaa, 0xff}
	alpha := []byte{0xff, 0xff, 0x00, 0x00}

	intact, _ := openParsedFile(t, "intact", brokenChecksumImageFile(brokenChecksum(t, pixels), zlibBytes(t, alpha)))
	data, err := intact.GetImageData(context.Background(), "intact", "obj:0:5")
	if err != nil {
		t.Fatalf("GetImageData with an intact /SMask: %v", err)
	}
	if data.Error != "" || data.Kind != imageKindOK {
		t.Fatalf("image with an intact /SMask = error %q, kind %q; want a rendered image", data.Error, data.Kind)
	}

	ins, _ := openParsedFile(t, "broken", brokenChecksumImageFile(zlibBytes(t, pixels), brokenChecksum(t, alpha)))
	within(t, 5*time.Second, "GetImageData on an image with a broken-checksum /SMask", func() {
		data, err = ins.GetImageData(context.Background(), "broken", "obj:0:5")
	})
	if err != nil {
		t.Fatalf("GetImageData: %v; want the failure in ImageData.Error", err)
	}
	if !strings.Contains(data.Error, "soft mask decode") || !strings.Contains(data.Error, zlib.ErrChecksum.Error()) {
		t.Errorf("Error = %q; want the soft mask decode failure with %q", data.Error, zlib.ErrChecksum.Error())
	}
	if data.Base64 != "" || data.Kind == imageKindOK {
		t.Errorf("image with a broken-checksum /SMask = kind %q, %d base64 bytes; want no preview", data.Kind, len(data.Base64))
	}
}

// Shapes found in UniPDF output. Each opens through Inspector.Open with no
// warning, and the object that carries the shape reads back as written. For a
// case with validatorRejects set, pdfcpu's relaxed validator run on its own
// parse of the same bytes must refuse the file with an error naming that
// shape (validatorSays), so the case keeps covering a shape the validator
// rejects and not some other defect of the hand-built file.
func TestOpen_ShapesThePdfcpuValidatorRejectsOpen(t *testing.T) {
	page := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>\nendobj\n"
	pages := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"
	cases := []struct {
		name             string
		pdf              []byte
		validatorRejects bool
		validatorSays    string   // substring of the validator's error for this shape
		nodeID           string   // object whose source carries the shape
		want             []string // substrings the source must contain
		absent           []string // substrings the source must not contain
	}{
		{
			name: "catalog associated files in a PDF 2.0 file",
			pdf: assemblexref("%PDF-2.0\n",
				"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AF [4 0 R] >>\nendobj\n",
				pages, page,
				"4 0 obj\n<< /Type /Filespec /F (a.txt) /UF (a.txt) /AFRelationship /Data >>\nendobj\n",
			),
			validatorRejects: true,
			validatorSays:    "AF: PDF 2.0 associated files not supported",
			nodeID:           "obj:0:1",
			want:             []string{"/AF"},
		},
		{
			name: "font without Subtype in the page resources",
			pdf: assemblexref("%PDF-1.7\n",
				"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
				pages,
				"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>\nendobj\n",
				"4 0 obj\n<< /Type /Font /BaseFont /Helvetica >>\nendobj\n",
				"5 0 obj\n"+rawStreamObj("", "BT /F1 12 Tf 10 10 Td (a) Tj ET")+"\nendobj\n",
			),
			validatorRejects: true,
			validatorSays:    "Font resource F1: font: missing Subtype",
			nodeID:           "obj:0:4",
			want:             []string{"/BaseFont", "/Helvetica"},
			absent:           []string{"/Subtype"},
		},
		{
			name: "text field widget kid with no DA set or inherited",
			pdf: assemblexref("%PDF-1.7\n",
				"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] >> >>\nendobj\n",
				pages,
				"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Annots [5 0 R] >>\nendobj\n",
				"4 0 obj\n<< /FT /Tx /T (name) /Kids [5 0 R] >>\nendobj\n",
				"5 0 obj\n<< /Type /Annot /Subtype /Widget /Parent 4 0 R /Rect [10 10 100 30] /P 3 0 R >>\nendobj\n",
			),
			validatorRejects: true,
			validatorSays:    "required entry=DA missing",
			nodeID:           "obj:0:4",
			want:             []string{"/FT", "/Tx"},
			absent:           []string{"/DA"},
		},
		{
			name: "name-typed catalog Version 1.1 in a PDF 1.1 file",
			pdf: assemblexref("%PDF-1.1\n",
				"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Version /1.1 >>\nendobj\n",
				pages, page,
			),
			nodeID: "obj:0:1",
			want:   []string{"/Version", "/1.1"},
		},
		{
			name: "structure element whose K holds an annotation dict",
			pdf: assemblexref("%PDF-1.7\n",
				"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 4 0 R /MarkInfo << /Marked true >> >>\nendobj\n",
				pages,
				"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Annots [6 0 R] /StructParents 0 >>\nendobj\n",
				"4 0 obj\n<< /Type /StructTreeRoot /K 5 0 R >>\nendobj\n",
				"5 0 obj\n<< /Type /StructElem /S /Link /P 4 0 R /Pg 3 0 R /K 6 0 R >>\nendobj\n",
				"6 0 obj\n<< /Type /Annot /Subtype /Link /Rect [10 10 100 30] /P 3 0 R /StructParent 1 >>\nendobj\n",
			),
			nodeID: "obj:0:5",
			want:   []string{"/StructElem", "/K 6 0 R"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.validatorRejects {
				ctx, err := pdfcpu_api.ReadContext(context.Background(), bytes.NewReader(tc.pdf), pdfcpu_model.NewStatelessConfiguration())
				if err != nil {
					t.Fatalf("fixture: pdfcpu's reader refuses the file: %v", err)
				}
				err = pdfcpu_api.ValidateContext(context.Background(), ctx)
				if err == nil {
					t.Fatal("pdfcpu's relaxed validator accepts this shape; replace it with a shape the validator still rejects or mark the case as not rejected")
				}
				if !strings.Contains(err.Error(), tc.validatorSays) {
					t.Fatalf("pdfcpu's relaxed validator refuses the file for another reason: %v; want an error containing %q", err, tc.validatorSays)
				}
			}
			ins, info := openParsedFile(t, "validator-shape", tc.pdf)
			if info.Error != "" {
				t.Errorf("open warning = %q, want none", info.Error)
			}
			if info.PageCount != 1 {
				t.Errorf("page count = %d, want 1", info.PageCount)
			}
			src, err := ins.GetObjectSource("validator-shape", tc.nodeID)
			if err != nil {
				t.Fatalf("GetObjectSource %s: %v", tc.nodeID, err)
			}
			for _, w := range tc.want {
				if !strings.Contains(src, w) {
					t.Errorf("source of %s lacks %q as written in the file:\n%s", tc.nodeID, w, src)
				}
			}
			for _, a := range tc.absent {
				if strings.Contains(src, a) {
					t.Errorf("source of %s shows %q, which the file does not have:\n%s", tc.nodeID, a, src)
				}
			}
		})
	}
}
