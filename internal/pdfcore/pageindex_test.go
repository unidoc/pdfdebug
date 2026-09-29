package pdfcore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	pdfcpu_api "github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Most malformed page trees are refused by pdfcpu's validator inside
// Inspector.Open, so these tests read the raw bytes with pdfcpu's reader only
// (no validation) and register the resulting context under a tab directly.

// rawObj is one indirect object: its number and the text between "N 0 obj"
// and "endobj".
type rawObj struct {
	num  int
	body string
}

// rawPDF lays out objs as a classic-xref PDF whose catalog is object 1.
// Numbers absent from objs are free entries, so a reference to one dangles.
func rawPDF(objs ...rawObj) []byte {
	sorted := append([]rawObj{}, objs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].num < sorted[j].num })
	maxNum := sorted[len(sorted)-1].num

	var b strings.Builder
	b.WriteString("%PDF-1.7\n")
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

const rawCatalog = "<< /Type /Catalog /Pages 2 0 R >>"

// rawStream renders a stream object body whose /Length matches data.
func rawStream(data string) string {
	return fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(data), data)
}

// openUnvalidated reads pdf without pdfcpu validation and registers it under
// tabID on a fresh Inspector.
func openUnvalidated(t *testing.T, pdf []byte) (*Inspector, *DocumentState) {
	t.Helper()
	ctx, err := pdfcpu_api.ReadContext(bytes.NewReader(pdf), pdfcpu_model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("ReadContext: %v", err)
	}
	closeCtx, closeCancel := context.WithCancel(context.Background())
	doc := &DocumentState{
		PDFContext:  ctx,
		streamCache: make(map[string]*ContentStreamData),
		closeCtx:    closeCtx,
		closeCancel: closeCancel,
	}
	ins := NewInspector()
	ins.documents["raw"] = doc
	t.Cleanup(func() { _ = ins.Close("raw") })
	return ins, doc
}

// rawPageIndex returns the page index of an unvalidated document.
func rawPageIndex(t *testing.T, objs ...rawObj) []*PageIndexEntry {
	t.Helper()
	ins, _ := openUnvalidated(t, rawPDF(objs...))
	entries, err := ins.GetPageIndex("raw")
	if err != nil {
		t.Fatalf("GetPageIndex: %v", err)
	}
	return entries
}

// openValidatedFile writes pdf to a temp file and opens it through
// Inspector.Open.
func openValidatedFile(t *testing.T, ins *Inspector, tabID string, pdf []byte) *DocumentInfo {
	t.Helper()
	path := filepath.Join(t.TempDir(), "doc.pdf")
	if err := os.WriteFile(path, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := ins.Open(tabID, path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return info
}

// rowShape is the part of an entry most walk tests compare.
type rowShape struct {
	page int
	obj  int
	err  string // substring expected in Err; "" requires no error
}

func checkRows(t *testing.T, entries []*PageIndexEntry, want []rowShape) {
	t.Helper()
	if len(entries) != len(want) {
		var got []string
		for _, e := range entries {
			got = append(got, fmt.Sprintf("%+v", *e))
		}
		t.Fatalf("got %d rows, want %d:\n%s", len(entries), len(want), strings.Join(got, "\n"))
	}
	for i, w := range want {
		e := entries[i]
		if e.PageNum != w.page || e.ObjNum != w.obj {
			t.Errorf("row %d: page %d obj %d, want page %d obj %d", i, e.PageNum, e.ObjNum, w.page, w.obj)
		}
		switch {
		case w.err == "" && e.Err != "":
			t.Errorf("row %d: unexpected error %q", i, e.Err)
		case w.err != "" && !strings.Contains(e.Err, w.err):
			t.Errorf("row %d: error %q, want it to contain %q", i, e.Err, w.err)
		}
	}
}

const box = "/MediaBox [0 0 612 792]"

func TestPageIndexInheritedAttributesAndBitmask(t *testing.T) {
	entries := rawPageIndex(t,
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 3 /Resources << >> " + box + " /CropBox [0 0 600 780] /Rotate 90 >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /Resources << >> /MediaBox [0 0 100 200] /CropBox [0 0 90 190] /Rotate 180 >>"},
		rawObj{4, "<< /Type /Page /Parent 2 0 R >>"},
		rawObj{5, "<< /Type /Pages /Parent 2 0 R /Kids [6 0 R] /Count 1 /MediaBox [0 0 595 842] >>"},
		rawObj{6, "<< /Type /Page /Parent 5 0 R /Rotate 270 >>"},
	)
	checkRows(t, entries, []rowShape{{1, 3, ""}, {2, 4, ""}, {3, 6, ""}})

	all := InheritedResources | InheritedMediaBox | InheritedCropBox | InheritedRotate
	if entries[0].Inherited != 0 || entries[0].MediaBox != [4]float64{0, 0, 100, 200} || entries[0].Rotate != 180 {
		t.Errorf("own values must win over inherited ones: %+v", *entries[0])
	}
	if entries[1].Inherited != all || entries[1].MediaBox != [4]float64{0, 0, 612, 792} || entries[1].Rotate != 90 {
		t.Errorf("a page declaring nothing inherits all four from the root: %+v", *entries[1])
	}
	want := InheritedResources | InheritedMediaBox | InheritedCropBox
	if entries[2].Inherited != want || entries[2].MediaBox != [4]float64{0, 0, 595, 842} || entries[2].Rotate != 270 {
		t.Errorf("nearest ancestor MediaBox, own Rotate: %+v", *entries[2])
	}
	for _, bit := range []uint8{InheritedResources, InheritedMediaBox, InheritedCropBox, InheritedRotate} {
		if bit == 0 || bit&(bit-1) != 0 {
			t.Errorf("inherited bit %08b is not a single bit", bit)
		}
	}
}

func TestPageIndexKidsCycleIsAnUnnumberedRow(t *testing.T) {
	entries := rawPageIndex(t,
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 5 0 R 2 0 R 4 0 R] /Count 2 " + box + " >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R >>"},
		rawObj{4, "<< /Type /Page /Parent 2 0 R >>"},
		rawObj{5, "<< /Type /Pages /Parent 2 0 R /Kids [5 0 R] /Count 0 >>"},
	)
	checkRows(t, entries, []rowShape{
		{1, 3, ""},
		{0, 5, "cycle"}, // obj 5 lists itself
		{0, 2, "cycle"}, // the root lists itself
		{2, 4, ""},
	})
	if entries[2].NodeID != "obj:0:2" {
		t.Errorf("a cycle row carries the reference: nodeId %q", entries[2].NodeID)
	}
}

func TestPageIndexNullNonDictAndDanglingKids(t *testing.T) {
	entries := rawPageIndex(t,
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R null 7 9 0 R 6 0 R 4 0 R] /Count 2 " + box + " >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R >>"},
		rawObj{4, "<< /Type /Page /Parent 2 0 R >>"},
		rawObj{6, "[1 2 3]"},
	)
	checkRows(t, entries, []rowShape{
		{1, 3, ""},
		{0, 0, "null"},
		{0, 0, "not a dictionary"},
		{0, 9, "dangling"},
		{0, 6, "not a dictionary"},
		{2, 4, ""},
	})
	if entries[1].NodeID != "" || entries[2].NodeID != "" {
		t.Errorf("rows with no reference carry no node id: %q %q", entries[1].NodeID, entries[2].NodeID)
	}
	if entries[3].NodeID != "obj:0:9" {
		t.Errorf("a dangling kid keeps its reference: %q", entries[3].NodeID)
	}
}

func TestPageIndexNonArrayKids(t *testing.T) {
	entries := rawPageIndex(t,
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 5 0 R 4 0 R] /Count 2 " + box + " >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R >>"},
		rawObj{4, "<< /Type /Page /Parent 2 0 R >>"},
		rawObj{5, "<< /Type /Pages /Parent 2 0 R /Kids 3 /Count 1 >>"},
	)
	checkRows(t, entries, []rowShape{{1, 3, ""}, {0, 5, "/Kids is not an array"}, {2, 4, ""}})
}

func TestPageIndexPageWithoutTypeOrWithAForeignType(t *testing.T) {
	entries := rawPageIndex(t,
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 3 " + box + " >>"},
		rawObj{3, "<< /Parent 2 0 R >>"},
		rawObj{4, "<< /Type /Annot /Parent 2 0 R >>"},
		rawObj{5, "<< /Type /Page /Parent 2 0 R >>"},
	)
	checkRows(t, entries, []rowShape{{1, 3, "no /Type"}, {2, 4, "/Type /Annot"}, {3, 5, ""}})
}

func TestPageIndexClassification(t *testing.T) {
	entries := rawPageIndex(t,
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 5 0 R 6 0 R 7 0 R] /Count 2 " + box + " >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /Kids [4 0 R] >>"},
		rawObj{4, "<< /Type /Page /Parent 3 0 R >>"},
		rawObj{5, "<< /Type /Pages /Parent 2 0 R >>"},
		rawObj{6, "<< /Type /Pages /Parent 2 0 R /Kids [] /Count 0 >>"},
		rawObj{7, "<< /Type /Page /Parent 2 0 R >>"},
	)
	checkRows(t, entries, []rowShape{
		{1, 3, "page has a /Kids entry"},   // /Type /Page is a page even with /Kids; its kids are not walked
		{0, 5, "/Pages node has no /Kids"}, // unnumbered
		{2, 7, ""},                         // /Kids [] yields nothing
	})
}

func TestPageIndexPageWithStrayKidsKeepsLaterNumbering(t *testing.T) {
	entries := rawPageIndex(t,
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R] /Count 3 " + box + " >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /Kids [] >>"},
		rawObj{4, "<< /Type /Page /Parent 2 0 R /Kids 7 >>"},
		rawObj{5, "<< /Type /Page /Parent 2 0 R >>"},
	)
	checkRows(t, entries, []rowShape{
		{1, 3, "page has a /Kids entry"},
		{2, 4, "page has a /Kids entry"},
		{3, 5, ""},
	})
	if entries[0].MediaBox != [4]float64{0, 0, 612, 792} {
		t.Errorf("page with an empty /Kids lost its inherited /MediaBox: %v", entries[0].MediaBox)
	}
}

func TestPageIndexDuplicatePageVersusSharedSubtree(t *testing.T) {
	entries := rawPageIndex(t,
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R 5 0 R 9 0 R] /Count 5 " + box + " >>"},
		rawObj{3, "<< /Type /Pages /Parent 2 0 R /Kids [6 0 R] /Count 1 >>"},
		rawObj{4, "<< /Type /Pages /Parent 2 0 R /Kids [6 0 R 7 0 R] /Count 2 >>"},
		rawObj{5, "<< /Type /Pages /Parent 2 0 R /Kids [8 0 R] /Count 1 >>"},
		rawObj{6, "<< /Type /Page /Parent 3 0 R >>"},
		rawObj{7, "<< /Type /Page /Parent 4 0 R >>"},
		rawObj{8, "<< /Type /Page /Parent 5 0 R >>"},
		rawObj{9, "<< /Type /Page /Parent 2 0 R >>"},
	)
	checkRows(t, entries, []rowShape{
		{1, 6, ""},
		{2, 6, "also listed as page 1"}, // same page under a second parent: numbered again
		{3, 7, ""},
		{4, 8, ""},
		{0, 5, "another parent"}, // a shared intermediate is walked once
		{5, 9, ""},
	})
}

func TestPageIndexMalformedAttributes(t *testing.T) {
	entries := rawPageIndex(t,
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 4 0 R 5 0 R 6 0 R 7 0 R] /Count 5 >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612] >>"},
		rawObj{4, "<< /Type /Page /Parent 2 0 R /MediaBox (x) >>"},
		rawObj{5, "<< /Type /Page /Parent 2 0 R " + box + " /Rotate 1.5 >>"},
		rawObj{6, "<< /Type /Page /Parent 2 0 R >>"},
		rawObj{7, "<< /Parent 2 0 R /Rotate /N >>"},
	)
	checkRows(t, entries, []rowShape{
		{1, 3, "/MediaBox is not an array of four numbers"},
		{2, 4, "/MediaBox is not an array of four numbers"},
		{3, 5, "/Rotate is not an integer; read as 2"},
		{4, 6, "no /MediaBox"},
		{5, 7, "no /Type; no /MediaBox, own or inherited; /Rotate is not an integer"},
	})
	if entries[0].MediaBox != [4]float64{} || entries[4].Rotate != 0 {
		t.Errorf("malformed values are left zero: %+v %+v", *entries[0], *entries[4])
	}
	if entries[2].Rotate != 2 {
		t.Errorf("a real /Rotate is rounded, as dump page reads it: got %d, want 2", entries[2].Rotate)
	}
	if entries[2].MediaBox != [4]float64{0, 0, 612, 792} {
		t.Errorf("a bad /Rotate does not disturb MediaBox: %v", entries[2].MediaBox)
	}
}

func TestPageIndexContentsAndLength(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 4 0 R 8 0 R 10 0 R 11 0 R 12 0 R 13 0 R 15 0 R] /Count 8 " + box + " >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /Contents 5 0 R >>"},
		rawObj{4, "<< /Type /Page /Parent 2 0 R /Contents [null 6 0 R 7 0 R] >>"},
		rawObj{5, "[6 0 R 7 0 R]"},
		rawObj{6, rawStream("0 0 m")},
		rawObj{7, rawStream("1 1 l S")},
		rawObj{8, "<< /Type /Page /Parent 2 0 R /Contents 9 0 R >>"},
		rawObj{9, "<< /Length 16 0 R >>\nstream\nq Q\nendstream"},
		rawObj{10, "<< /Type /Page /Parent 2 0 R /Contents [6 0 R 5] >>"},
		rawObj{11, "<< /Type /Page /Parent 2 0 R /Contents [6 0 R 2 0 R] >>"},
		rawObj{12, "<< /Type /Page /Parent 2 0 R /Contents /X >>"},
		rawObj{13, "<< /Type /Page /Parent 2 0 R /Contents 14 0 R >>"},
		rawObj{14, rawStream("abc")},
		rawObj{15, "<< /Type /Page /Parent 2 0 R /Contents 2 0 R >>"},
		rawObj{16, "3"},
	))
	// pdfcpu's reader rewrites /Length; remove it from obj 14 so the
	// missing-/Length rule is reachable.
	sd := doc.PDFContext.XRefTable.Table[14].Object.(pdfcpu_types.StreamDict)
	delete(sd.Dict, "Length")

	entries, err := ins.GetPageIndex("raw")
	if err != nil {
		t.Fatalf("GetPageIndex: %v", err)
	}
	checkRows(t, entries, []rowShape{
		{1, 3, ""},
		{2, 4, ""},
		{3, 8, ""},
		{4, 10, "not an indirect reference"},
		{5, 11, "is not a stream"},
		{6, 12, "not a stream reference or array"},
		{7, 13, ""},
		{8, 15, "is not a stream"},
	})
	want := []struct {
		id  string
		len int64
	}{
		{"obj:0:6", 5 + 7}, // indirect ref to an array: summed over the array
		{"obj:0:6", 5 + 7}, // direct array, null skipped
		{"obj:0:9", 3},     // indirect /Length
		{"obj:0:6", -1},
		{"obj:0:6", -1},
		{"", -1},
		{"obj:0:14", -1}, // no /Length
		{"", -1},
	}
	for i, w := range want {
		if entries[i].ContentNodeID != w.id || entries[i].ContentLen != w.len {
			t.Errorf("row %d: contentNodeId %q contentLen %d, want %q and %d",
				i, entries[i].ContentNodeID, entries[i].ContentLen, w.id, w.len)
		}
	}
}

func TestPageIndexNoContentsIsZeroLength(t *testing.T) {
	entries := rawPageIndex(t,
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R " + box + " /Annots 4 0 R >>"},
		rawObj{4, "[5 0 R 5 0 R 5 0 R]"},
		rawObj{5, "<< /Type /Annot /Subtype /Text /Rect [0 0 1 1] >>"},
	)
	checkRows(t, entries, []rowShape{{1, 3, ""}})
	if entries[0].ContentLen != 0 || entries[0].ContentNodeID != "" || entries[0].AnnotCount != 3 {
		t.Errorf("no /Contents and an indirect /Annots: %+v", *entries[0])
	}
}

func TestPageIndexEmptyAndMissingPageTree(t *testing.T) {
	cases := map[string][]rawObj{
		"empty /Kids": {{1, rawCatalog}, {2, "<< /Type /Pages /Kids [] /Count 0 >>"}},
		"no /Pages":   {{1, "<< /Type /Catalog >>"}},
		"non-dict":    {{1, rawCatalog}, {2, "[1 2]"}},
	}
	for name, objs := range cases {
		t.Run(name, func(t *testing.T) {
			entries := rawPageIndex(t, objs...)
			if entries == nil || len(entries) != 0 {
				t.Errorf("want a non-nil empty slice, got %#v", entries)
			}
		})
	}
}

func TestPageIndexCountDisagreementSurfacesBothNumbers(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 5 " + box + " >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R >>"},
		rawObj{4, "<< /Type /Page /Parent 2 0 R >>"},
	))
	if err := doc.PDFContext.EnsurePageCount(); err != nil {
		t.Fatalf("EnsurePageCount: %v", err)
	}
	entries, err := ins.GetPageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	leaves := 0
	for _, e := range entries {
		if e.PageNum > 0 {
			leaves++
		}
	}
	if leaves != 2 || doc.PDFContext.PageCount != 5 {
		t.Errorf("leaves %d and /Count %d, want 2 and 5 reported side by side", leaves, doc.PDFContext.PageCount)
	}
}

func TestPageIndexOrderMatchesGetPageNode(t *testing.T) {
	ins, tabID := openMultipage(t)
	entries, err := ins.GetPageIndex(tabID)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatalf("multipage.pdf gave %d rows", len(entries))
	}
	for i, e := range entries {
		node, err := ins.GetPageNode(tabID, i+1)
		if err != nil {
			t.Fatalf("GetPageNode(%d): %v", i+1, err)
		}
		if e.PageNum != i+1 || e.NodeID != node.ID || e.Err != "" {
			t.Errorf("row %d: page %d node %q error %q, GetPageNode resolves %q", i, e.PageNum, e.NodeID, e.Err, node.ID)
		}
	}
}

func TestPageIndexIsCachedAcrossCalls(t *testing.T) {
	ins, tabID := openMultipage(t)
	first, err := ins.GetPageIndex(tabID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ins.GetPageIndex(tabID)
	if err != nil {
		t.Fatal(err)
	}
	if &first[0] != &second[0] {
		t.Errorf("the second call rebuilt the index instead of returning the cached slice")
	}
	doc, _ := ins.GetDocument(tabID)
	builds := 0
	if _, err := doc.pageIndex.get(func() ([]*PageIndexEntry, error) { builds++; return nil, nil }); err != nil {
		t.Fatal(err)
	}
	if builds != 0 {
		t.Errorf("the cache was empty after GetPageIndex: %d builds", builds)
	}
}

func TestPageIndexUnknownTab(t *testing.T) {
	if _, err := NewInspector().GetPageIndex("missing"); err == nil {
		t.Error("want ErrDocumentNotFound for an unknown tab")
	}
}

func TestPageIndexConcurrentWithObjectIndexAndChildren(t *testing.T) {
	ins, tabID := openMultipage(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(3)
		go func() {
			defer wg.Done()
			if _, err := ins.GetPageIndex(tabID); err != nil {
				t.Errorf("GetPageIndex: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := ins.GetObjectIndex(tabID); err != nil {
				t.Errorf("GetObjectIndex: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := ins.GetChildren(tabID, "root"); err != nil {
				t.Errorf("GetChildren: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestPageIndexIsFreshAfterReopenUnderTheSameTab(t *testing.T) {
	ins := NewInspector()
	onePage := rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 " + box + " >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R >>"},
	)
	twoPages := rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 " + box + " >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R >>"},
		rawObj{4, "<< /Type /Page /Parent 2 0 R >>"},
	)
	openValidatedFile(t, ins, "tab", onePage)
	t.Cleanup(func() { _ = ins.Close("tab") })
	if entries, err := ins.GetPageIndex("tab"); err != nil || len(entries) != 1 {
		t.Fatalf("first document: %d rows, err %v; want 1", len(entries), err)
	}
	firstObjects, err := ins.GetObjectIndex("tab")
	if err != nil {
		t.Fatal(err)
	}

	openValidatedFile(t, ins, "tab", twoPages)
	entries, err := ins.GetPageIndex("tab")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("the re-opened document's index has %d rows, want 2", len(entries))
	}
	objects, err := ins.GetObjectIndex("tab")
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != len(firstObjects)+1 {
		t.Errorf("the re-opened document's object index has %d entries, want %d", len(objects), len(firstObjects)+1)
	}
}

func TestPageIndexDepthCapStopsADeepChain(t *testing.T) {
	// Root (obj 2) lists page 3, then a chain of /Pages nodes 4, 5, ... each
	// listing the next, deeper than the cap, ending in a page.
	chainLen := maxPageTreeDepth + 5
	last := 3 + chainLen
	objs := []rawObj{
		{1, rawCatalog},
		{2, "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 " + box + " >>"},
		{3, "<< /Type /Page /Parent 2 0 R >>"},
	}
	for n := 4; n < last; n++ {
		objs = append(objs, rawObj{n, fmt.Sprintf("<< /Type /Pages /Kids [%d 0 R] /Count 1 >>", n+1)})
	}
	objs = append(objs, rawObj{last, "<< /Type /Page >>"})
	entries := rawPageIndex(t, objs...)

	// Obj 2 is the first intermediate on the path and obj n (n >= 4) is the
	// (n-2)-th, so the node one past the cap is obj maxPageTreeDepth+3.
	checkRows(t, entries, []rowShape{
		{1, 3, ""},
		{0, maxPageTreeDepth + 3, fmt.Sprintf("deeper than %d levels", maxPageTreeDepth)},
	})
}

func TestPageIndexDirectDictCycleTerminates(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 1 " + box + " >>"},
		rawObj{3, "<< /Type /Pages /Kids [] /Count 0 >>"},
		rawObj{4, "<< /Type /Page /Parent 2 0 R >>"},
	))
	// A direct dictionary whose /Kids holds itself has no object number, so
	// only the depth cap can stop the walk.
	node := doc.PDFContext.XRefTable.Table[3].Object.(pdfcpu_types.Dict)
	node["Kids"] = pdfcpu_types.Array{node}

	entries, err := ins.GetPageIndex("raw")
	if err != nil {
		t.Fatalf("GetPageIndex: %v", err)
	}
	checkRows(t, entries, []rowShape{
		{0, 0, "deeper than"},
		{1, 4, ""},
	})
}

func TestPageIndexNonFiniteMediaBoxIsMalformedAndEncodes(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612.5 792] >>"},
		rawObj{4, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612.5 792] >>"},
	))
	doc.PDFContext.XRefTable.Table[3].Object.(pdfcpu_types.Dict)["MediaBox"].(pdfcpu_types.Array)[2] = pdfcpu_types.Float(math.NaN())
	doc.PDFContext.XRefTable.Table[4].Object.(pdfcpu_types.Dict)["MediaBox"].(pdfcpu_types.Array)[3] = pdfcpu_types.Float(math.Inf(1))

	entries, err := ins.GetPageIndex("raw")
	if err != nil {
		t.Fatalf("GetPageIndex: %v", err)
	}
	checkRows(t, entries, []rowShape{
		{1, 3, "/MediaBox is not an array of four numbers"},
		{2, 4, "/MediaBox is not an array of four numbers"},
	})
	for i, e := range entries {
		if e.MediaBox != [4]float64{} {
			t.Errorf("row %d: a non-finite MediaBox is left zero, got %v", i, e.MediaBox)
		}
	}
	if _, err := json.Marshal(entries); err != nil {
		t.Errorf("the index must encode as JSON: %v", err)
	}
}

func TestPageIndexNegativeAndOverflowingLengthAreUnreadable(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 " + box + " >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /Contents 5 0 R >>"},
		rawObj{4, "<< /Type /Page /Parent 2 0 R /Contents [6 0 R 7 0 R] >>"},
		rawObj{5, rawStream("q Q")},
		rawObj{6, rawStream("q Q")},
		rawObj{7, rawStream("q Q")},
	))
	// pdfcpu's reader rewrites /Length, so the values are set afterwards.
	setLength := func(num int, n int) {
		doc.PDFContext.XRefTable.Table[num].Object.(pdfcpu_types.StreamDict).Dict["Length"] = pdfcpu_types.Integer(n)
	}
	setLength(5, -5)
	setLength(6, math.MaxInt64)
	setLength(7, math.MaxInt64)

	entries, err := ins.GetPageIndex("raw")
	if err != nil {
		t.Fatalf("GetPageIndex: %v", err)
	}
	checkRows(t, entries, []rowShape{{1, 3, ""}, {2, 4, ""}})
	if entries[0].ContentLen != -1 {
		t.Errorf("a negative /Length is unreadable: contentLen %d, want -1", entries[0].ContentLen)
	}
	if entries[1].ContentLen != -1 {
		t.Errorf("a /Length sum past int64 is unreadable: contentLen %d, want -1", entries[1].ContentLen)
	}
}

func TestPageIndexContentLenIsTheLengthEntryNotADecode(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 " + box + " >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /Contents 4 0 R >>"},
		rawObj{4, rawStream("0 0 m 10 10 l S")},
	))
	// A /Length that differs from both the raw and the decoded byte counts
	// can only come back from reading the entry itself.
	sd := doc.PDFContext.XRefTable.Table[4].Object.(pdfcpu_types.StreamDict)
	sd.Dict["Length"] = pdfcpu_types.Integer(4242)
	doc.PDFContext.XRefTable.Table[4].Object = sd

	entries, err := ins.GetPageIndex("raw")
	if err != nil {
		t.Fatalf("GetPageIndex: %v", err)
	}
	checkRows(t, entries, []rowShape{{1, 3, ""}})
	if entries[0].ContentLen != 4242 {
		t.Errorf("contentLen %d, want the /Length entry 4242", entries[0].ContentLen)
	}
}

func TestPageIndexDirectDictPageIsNumberedWithoutAReference(t *testing.T) {
	entries := rawPageIndex(t,
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R << /Type /Page /Rotate 90 >> 4 0 R] /Count 3 " + box + " >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R >>"},
		rawObj{4, "<< /Type /Page /Parent 2 0 R >>"},
	)
	checkRows(t, entries, []rowShape{
		{1, 3, ""},
		{2, 0, "page is a direct dictionary, not an indirect reference"},
		{3, 4, ""},
	})
	direct := entries[1]
	if direct.NodeID != "" || direct.Gen != 0 {
		t.Errorf("a direct page has no reference to carry: nodeId %q gen %d", direct.NodeID, direct.Gen)
	}
	if direct.MediaBox != [4]float64{0, 0, 612, 792} || direct.Inherited != InheritedMediaBox || direct.Rotate != 90 {
		t.Errorf("a direct page still resolves own and inherited attributes: %+v", *direct)
	}
}

func TestFindPageNumbersLikeThePageIndex(t *testing.T) {
	_, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 5 0 R 6 0 R] /Count 3 " + box + " >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /Kids [4 0 R] >>"},
		rawObj{4, "<< /Type /Page /Parent 3 0 R >>"},
		rawObj{5, "<< /Type /Page /Parent 2 0 R /Kids [] >>"},
		rawObj{6, "<< /Type /Page /Parent 2 0 R >>"},
	))
	for pageNum, wantObj := range map[int]int{1: 3, 2: 5, 3: 6} {
		leaf := findPage(doc.PDFContext, pageNum)
		if leaf == nil || leaf.ref == nil || leaf.ref.ObjectNumber.Value() != wantObj {
			t.Errorf("findPage(%d) = %+v, want obj %d", pageNum, leaf, wantObj)
		}
	}
	for _, pageNum := range []int{0, -1, 4} {
		if leaf := findPage(doc.PDFContext, pageNum); leaf != nil {
			t.Errorf("findPage(%d) = obj %v, want nil", pageNum, leaf.ref)
		}
	}
}

func TestInheritedPageAttrsTakeTheNearestValue(t *testing.T) {
	_, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 /MediaBox [0 0 612 792] /CropBox [9 9 99 99] /Rotate 90 /Resources << /Font << >> >> >>"},
		rawObj{3, "<< /Type /Pages /Parent 2 0 R /Kids [4 0 R] /Count 1 /MediaBox [0 0 100 200] >>"},
		rawObj{4, "<< /Type /Page /Parent 3 0 R /Rotate 179.6 /CropBox [1 2 3 4] >>"},
	))
	leaf := findPage(doc.PDFContext, 1)
	if leaf == nil {
		t.Fatal("page 1 not found")
	}
	inh, err := inheritedPageAttrs(doc.PDFContext, leaf)
	if err != nil {
		t.Fatal(err)
	}
	if inh.MediaBox == nil || inh.MediaBox.Width() != 100 || inh.MediaBox.Height() != 200 {
		t.Errorf("MediaBox = %v, want the nearest ancestor's 0 0 100 200", inh.MediaBox)
	}
	if inh.CropBox == nil || inh.CropBox.LL.X != 1 || inh.CropBox.UR.Y != 4 {
		t.Errorf("CropBox = %v, want the page's own 1 2 3 4", inh.CropBox)
	}
	if inh.Rotate != 180 {
		t.Errorf("Rotate = %d, want the page's 179.6 rounded to 180", inh.Rotate)
	}
	if _, ok := inh.Resources.Find("Font"); !ok {
		t.Errorf("Resources = %v, want the root's dictionary", inh.Resources)
	}
}

func TestInheritedPageAttrsAbsentAndMalformed(t *testing.T) {
	_, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R >>"},
		rawObj{4, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612] >>"},
	))
	inh, err := inheritedPageAttrs(doc.PDFContext, findPage(doc.PDFContext, 1))
	if err != nil {
		t.Fatal(err)
	}
	if inh.MediaBox != nil || inh.CropBox != nil || inh.Resources != nil || inh.Rotate != 0 {
		t.Errorf("a page with nothing declared anywhere = %+v, want all zero", inh)
	}
	if _, err := inheritedPageAttrs(doc.PDFContext, findPage(doc.PDFContext, 2)); err == nil || !strings.Contains(err.Error(), "/MediaBox is not an array of four numbers") {
		t.Errorf("a three-element /MediaBox: err %v, want a /MediaBox error", err)
	}
}

func TestPageNumberLookupsResolveAPageCarryingKids(t *testing.T) {
	ins, _ := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 " + box + " >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /Kids [] /Contents 4 0 R >>"},
		rawObj{4, rawStream("0 0 m")},
		rawObj{5, "<< /Type /Page /Parent 2 0 R >>"},
	))
	node, err := ins.GetPageNode("raw", 1)
	if err != nil || node.ObjectRef != "3 0 R" {
		t.Errorf("GetPageNode(1) = %+v, %v; want 3 0 R", node, err)
	}
	ids, err := ins.pageContentStreamNodeIDs("raw", 1)
	if err != nil || len(ids) != 1 || ids[0] != "obj:0:4" {
		t.Errorf("pageContentStreamNodeIDs(1) = %v, %v; want [obj:0:4]", ids, err)
	}
	info, err := ins.PageRenderInfo("raw", 1, PageRenderOpts{})
	if err != nil || info.PageRef != "3 0 R" {
		t.Errorf("PageRenderInfo(1) = %+v, %v; want PageRef 3 0 R", info, err)
	}
	if _, err := ins.GetPageNode("raw", 3); err == nil || err.Error() != "page 3 not found" {
		t.Errorf("GetPageNode(3) err = %v, want page 3 not found", err)
	}
	if _, err := ins.pageContentStreamNodeIDs("raw", 3); err == nil || err.Error() != "page 3 not found" {
		t.Errorf("pageContentStreamNodeIDs(3) err = %v, want page 3 not found", err)
	}
}

func TestPageNumberLookupsRejectADirectPageDictionary(t *testing.T) {
	ins, _ := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [<< /Type /Page /Parent 2 0 R >>] /Count 1 " + box + " >>"},
	))
	for name, call := range map[string]func() error{
		"GetPageNode":    func() error { _, err := ins.GetPageNode("raw", 1); return err },
		"PageRenderInfo": func() error { _, err := ins.PageRenderInfo("raw", 1, PageRenderOpts{}); return err },
	} {
		if err := call(); err == nil || !strings.Contains(err.Error(), "not found: the page is a direct dictionary") {
			t.Errorf("%s err = %v, want a direct-dictionary not found error", name, err)
		}
	}
}

func TestPageRenderInfoNamesAMalformedAttributeRatherThanAMissingPage(t *testing.T) {
	ins, _ := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612] >>"},
	))
	_, err := ins.PageRenderInfo("raw", 1, PageRenderOpts{})
	if err == nil || strings.Contains(err.Error(), "not found") || !strings.Contains(err.Error(), "page 1: /MediaBox is not an array of four numbers") {
		t.Errorf("err = %v, want page 1: /MediaBox is not an array of four numbers", err)
	}
}

func TestInheritedPageAttrsRejectANonFiniteBox(t *testing.T) {
	_, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>"},
	))
	leaf := findPage(doc.PDFContext, 1)
	leaf.attrs.mediaBox = pdfcpu_types.Array{pdfcpu_types.Integer(0), pdfcpu_types.Integer(0), pdfcpu_types.Float(math.Inf(1)), pdfcpu_types.Integer(792)}
	if _, err := inheritedPageAttrs(doc.PDFContext, leaf); err == nil || !strings.Contains(err.Error(), "/MediaBox") {
		t.Errorf("an infinite /MediaBox element: err %v, want a /MediaBox error", err)
	}
}
