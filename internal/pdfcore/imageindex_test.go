package pdfcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Shapes pdfcpu's validator refuses at Open (a /PS XObject, a direct or
// non-stream /XObject entry, a non-integer /Width) are read with
// openUnvalidated and registered under a tab directly; the rest also go
// through Inspector.Open where a test needs the validated path.

// imgStream renders a stream object body with dict entries and a matching
// /Length.
func imgStream(dict, data string) string {
	return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", dict, len(data), data)
}

// grayImg is an uncompressed 2x2 DeviceGray image with extra spliced into its
// dictionary.
func grayImg(extra string) string {
	return imgStream("/Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 "+extra, "\x00\x01\x01\x00")
}

// rgbImg is an uncompressed 2x2 DeviceRGB image with extra spliced into its
// dictionary.
func rgbImg(extra string) string {
	return imgStream("/Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceRGB /BitsPerComponent 8 "+extra, strings.Repeat("\x00", 12))
}

// unresolvedImg is a 2x2 image whose /ColorSpace names no colour space pdfcpu
// can count components for, with extra spliced into its dictionary.
func unresolvedImg(extra string) string {
	return imgStream("/Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /Foo /BitsPerComponent 8 "+extra, "xxxx")
}

// labImg is an uncompressed 2x2 Lab image whose colour space dictionary holds
// labExtra beside its /WhitePoint, with extra spliced into its dictionary.
func labImg(labExtra, extra string) string {
	return imgStream("/Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace [/Lab << /WhitePoint [0.9505 1 1.089] "+labExtra+" >>] /BitsPerComponent 8 "+extra, strings.Repeat("\x00", 12))
}

// formXObj is a Form XObject whose /Resources /XObject holds xobjects; an
// empty xobjects writes no /Resources.
func formXObj(xobjects string) string {
	dict := "/Type /XObject /Subtype /Form /BBox [0 0 10 10]"
	if xobjects != "" {
		dict += " /Resources << /XObject << " + xobjects + " >> >>"
	}
	return imgStream(dict, "q Q")
}

// imgPage is a /Page under obj 2 whose own /Resources /XObject holds xobjects.
func imgPage(xobjects string) string {
	return "<< /Type /Page /Parent 2 0 R " + box + " /Resources << /XObject << " + xobjects + " >> >> >>"
}

// imgPages is the root /Pages node listing kids in order.
func imgPages(extra string, kids ...int) string {
	refs := make([]string, len(kids))
	for i, k := range kids {
		refs[i] = fmt.Sprintf("%d 0 R", k)
	}
	return fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d %s >>", strings.Join(refs, " "), len(kids), extra)
}

// rawImageIndex returns the image index of an unvalidated document.
func rawImageIndex(t *testing.T, objs ...rawObj) []*ImageIndexEntry {
	t.Helper()
	ins, _ := openUnvalidated(t, rawPDF(objs...))
	entries, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatalf("GetImageIndex: %v", err)
	}
	return entries
}

// imageEntryFor returns the image entry for objNum.
func imageEntryFor(t *testing.T, entries []*ImageIndexEntry, objNum int) *ImageIndexEntry {
	t.Helper()
	for _, e := range entries {
		if e.NodeID != "" && e.ObjNum == objNum {
			return e
		}
	}
	t.Fatalf("no entry for object %d", objNum)
	return nil
}

// entryObjNums lists the object numbers of entries, error rows as 0.
func entryObjNums(entries []*ImageIndexEntry) []int {
	out := make([]int, len(entries))
	for i, e := range entries {
		out[i] = e.ObjNum
	}
	return out
}

// sharedImageObjs has 20 pages (objs 10-29) all naming the image obj 5; page 2
// names it twice, and page 3 also names obj 6.
func sharedImageObjs() []rawObj {
	objs := []rawObj{{1, rawCatalog}, {5, grayImg("")}, {6, rgbImg("")}}
	kids := make([]int, 20)
	for i := range kids {
		kids[i] = 10 + i
		x := "/Im1 5 0 R"
		switch i + 1 {
		case 2:
			x += " /Logo 5 0 R"
		case 3:
			x += " /Im2 6 0 R"
		}
		objs = append(objs, rawObj{10 + i, imgPage(x)})
	}
	return append(objs, rawObj{2, imgPages("", kids...)})
}

// nestedImageObjs reaches image 8 two forms deep on page 1 (Fm1 > Fm2 > Im0),
// lists a form with no /Resources (Fm3) beside it, and references image 8
// both through Fm1 and directly as Im9 on page 2. Page 3 has no images.
func nestedImageObjs() []rawObj {
	return []rawObj{
		{1, rawCatalog},
		{2, imgPages("", 3, 4, 5)},
		{3, imgPage("/Fm1 6 0 R /Fm3 9 0 R")},
		{4, imgPage("/Fm1 6 0 R /Im9 8 0 R")},
		{5, "<< /Type /Page /Parent 2 0 R " + box + " >>"},
		{6, formXObj("/Fm2 7 0 R")},
		{7, formXObj("/Im0 8 0 R")},
		{8, grayImg("")},
		{9, formXObj("")},
	}
}

func TestImageIndexClickTargetsResolveThroughInheritedResourcesAndNestedForms(t *testing.T) {
	ins := NewInspector()
	openValidatedFile(t, ins, "tab", rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("/Resources << /XObject << /Im0 5 0 R /Fm1 6 0 R >> >> "+box, 3)},
		rawObj{3, "<< /Type /Page /Parent 2 0 R >>"},
		rawObj{5, grayImg("")},
		rawObj{6, formXObj("/Fm2 7 0 R")},
		rawObj{7, formXObj("/Im1 8 0 R")},
		rawObj{8, rgbImg("")},
	))
	t.Cleanup(func() { _ = ins.Close("tab") })

	entries, err := ins.GetImageIndex("tab")
	if err != nil {
		t.Fatal(err)
	}
	if got := entryObjNums(entries); !reflect.DeepEqual(got, []int{5, 8}) {
		t.Fatalf("entries %v, want [5 8]", got)
	}
	for _, e := range entries {
		if e.NodeID != fmt.Sprintf("obj:%d:%d", e.Gen, e.ObjNum) {
			t.Errorf("node id %q does not encode %d %d R generation first", e.NodeID, e.ObjNum, e.Gen)
		}
		desc, err := ins.DescribeImage("tab", e.NodeID)
		if err != nil || desc.Error != "" || desc.Width != 2 {
			t.Errorf("DescribeImage(%s) = %+v, %v", e.NodeID, desc, err)
		}
		detail, err := ins.GetObjectDetail("tab", e.NodeID)
		if err != nil || detail.Type != "stream" {
			t.Errorf("GetObjectDetail(%s) = %+v, %v", e.NodeID, detail, err)
		}
		path, err := ins.GetAncestorPath("tab", e.NodeID)
		if err != nil || len(path) == 0 || path[len(path)-1] != e.NodeID {
			t.Errorf("GetAncestorPath(%s) = %v, %v; want a path ending at the image", e.NodeID, path, err)
		}
	}
}

func TestImageIndexSharedImageIsOneEntryWithCappedPages(t *testing.T) {
	entries := rawImageIndex(t, sharedImageObjs()...)
	if got := entryObjNums(entries); !reflect.DeepEqual(got, []int{5, 6}) {
		t.Fatalf("entries %v, want [5 6]", got)
	}
	logo := entries[0]
	if logo.PageCount != 20 || logo.FirstPage != 1 || len(logo.FirstPages) != maxImageFirstPages {
		t.Errorf("pageCount %d firstPage %d firstPages %v; want 20, 1 and the first %d pages",
			logo.PageCount, logo.FirstPage, logo.FirstPages, maxImageFirstPages)
	}
	for i, p := range logo.FirstPages {
		if p != i+1 {
			t.Errorf("firstPages %v, want 1..16 ascending", logo.FirstPages)
			break
		}
	}
	if e := entries[1]; e.FirstPage != 3 || e.PageCount != 1 || !reflect.DeepEqual(e.FirstPages, []int{3}) {
		t.Errorf("object 6: %+v, want page 3 only", *e)
	}
}

func TestGetImagePagesReturnsTheFullList(t *testing.T) {
	ins, _ := openUnvalidated(t, rawPDF(sharedImageObjs()...))
	pages, err := ins.GetImagePages("raw", 5)
	if err != nil {
		t.Fatal(err)
	}
	want := make([]ImagePageRef, 20)
	for i := range want {
		want[i] = ImagePageRef{PageNum: i + 1, NodeID: fmt.Sprintf("obj:0:%d", 10+i)}
	}
	if !reflect.DeepEqual(pages, want) {
		t.Errorf("pages %v, want %v", pages, want)
	}
}

// pageNums lists the page numbers of refs.
func pageNums(refs []ImagePageRef) []int {
	out := make([]int, len(refs))
	for i, r := range refs {
		out[i] = r.PageNum
	}
	return out
}

func TestImageIndexFirstPageNodeIDsMatchFirstPages(t *testing.T) {
	ins, _ := openUnvalidated(t, rawPDF(sharedImageObjs()...))
	entries, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	e := imageEntryFor(t, entries, 6)
	if !reflect.DeepEqual(e.FirstPages, []int{3}) || !reflect.DeepEqual(e.FirstPageNodeIDs, []string{"obj:0:12"}) {
		t.Errorf("object 6 first pages %v node ids %v, want [3] and [obj:0:12]", e.FirstPages, e.FirstPageNodeIDs)
	}
	e = imageEntryFor(t, entries, 5)
	if len(e.FirstPageNodeIDs) != len(e.FirstPages) || e.FirstPageNodeIDs[15] != "obj:0:25" {
		t.Errorf("object 5 node ids %v, want one per first page ending at obj:0:25", e.FirstPageNodeIDs)
	}
}

func TestGetImagePagesUnknownObjectNumberIsAnError(t *testing.T) {
	ins, _ := openUnvalidated(t, rawPDF(sharedImageObjs()...))
	if _, err := ins.GetImagePages("raw", 10); err == nil || !strings.Contains(err.Error(), "10") {
		t.Errorf("err = %v, want an error naming object 10 (a page, not an image)", err)
	}
	if _, err := NewInspector().GetImagePages("missing", 5); !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("unknown tab: err = %v, want ErrDocumentNotFound", err)
	}
}

func TestImageIndexImageNamedTwiceOnOnePageCountsThatPageOnce(t *testing.T) {
	entries := rawImageIndex(t, nestedImageObjs()...)
	if got := entryObjNums(entries); !reflect.DeepEqual(got, []int{8}) {
		t.Fatalf("entries %v, want [8]; forms are not listed", got)
	}
	if e := entries[0]; e.PageCount != 2 || !reflect.DeepEqual(e.FirstPages, []int{1, 2}) {
		t.Errorf("pageCount %d firstPages %v, want 2 and [1 2]", e.PageCount, e.FirstPages)
	}
}

func TestImageIndexImageReachedOnlyThroughInheritedResources(t *testing.T) {
	entries := rawImageIndex(t,
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("/Resources << /XObject << /Im0 8 0 R >> >>", 3, 4, 5)},
		rawObj{3, "<< /Type /Page /Parent 2 0 R " + box + " >>"},
		rawObj{4, "<< /Type /Page /Parent 2 0 R " + box + " >>"},
		rawObj{5, "<< /Type /Page /Parent 2 0 R " + box + " /Resources << >> >>"},
		rawObj{8, grayImg("")},
	)
	if len(entries) != 1 || entries[0].ObjNum != 8 || !reflect.DeepEqual(entries[0].FirstPages, []int{1, 2}) {
		t.Errorf("entries %+v, want object 8 on pages 1 and 2 only", entries)
	}
}

func TestImageIndexFormCyclesAreWalkedOnce(t *testing.T) {
	entries := rawImageIndex(t,
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3)},
		rawObj{3, imgPage("/Fm1 6 0 R")},
		rawObj{6, formXObj("/Self 6 0 R /Fm2 7 0 R")},
		rawObj{7, formXObj("/Back 6 0 R /Im0 8 0 R")},
		rawObj{8, grayImg("")},
	)
	if len(entries) != 1 || entries[0].ObjNum != 8 || entries[0].PageCount != 1 {
		t.Errorf("entries %+v, want object 8 once, on page 1, and no error row", entries)
	}
}

// deepChainObjs chains n forms (objs 10..) from page 1: the first also lists
// image 5, the last lists image 6.
func deepChainObjs(n int) []rawObj {
	objs := []rawObj{{1, rawCatalog}, {2, imgPages("", 3)}, {3, imgPage("/Fm 10 0 R")}, {5, grayImg("")}, {6, grayImg("")}}
	for i := range n {
		num := 10 + i
		var x string
		switch i {
		case 0:
			x = fmt.Sprintf("/ImTop 5 0 R /Fm %d 0 R", num+1)
		case n - 1:
			x = "/ImDeep 6 0 R"
		default:
			x = fmt.Sprintf("/Fm %d 0 R", num+1)
		}
		objs = append(objs, rawObj{num, formXObj(x)})
	}
	return objs
}

func TestImageIndexFormChainPastTheCapWritesOneErrorRow(t *testing.T) {
	entries := rawImageIndex(t, deepChainObjs(maxFormWalkDepth+8)...)
	if got := entryObjNums(entries); !reflect.DeepEqual(got, []int{5, 0}) {
		t.Fatalf("entries %v, want image 5 then one error row", got)
	}
	row := entries[1]
	want := fmt.Sprintf("Form XObject nesting deeper than 32 at %d 0 R on page 1; deeper forms were not walked", 10+maxFormWalkDepth)
	if row.NodeID != "" || row.Err != want {
		t.Errorf("error row %+v, want error %q", *row, want)
	}

	// A chain exactly at the cap is walked to its end.
	entries = rawImageIndex(t, deepChainObjs(maxFormWalkDepth)...)
	if got := entryObjNums(entries); !reflect.DeepEqual(got, []int{5, 6}) {
		t.Errorf("a chain of %d forms: entries %v, want [5 6] and no error row", maxFormWalkDepth, got)
	}
}

func TestImageIndexBudgetStopsTheWalkAndKeepsEarlierPages(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3, 4, 5)},
		rawObj{3, imgPage("/A 10 0 R /B 11 0 R")},
		rawObj{4, imgPage("/A 12 0 R /B 13 0 R")},
		rawObj{5, imgPage("/A 14 0 R")},
		rawObj{10, grayImg("")}, rawObj{11, grayImg("")}, rawObj{12, grayImg("")},
		rawObj{13, grayImg("")}, rawObj{14, grayImg("")},
	))
	pt, _ := doc.pageTree()
	w := newImageWalker(doc.PDFContext)
	w.budget = 3
	if _, err := doc.imageIndex.get(func() (*imageTree, error) { return w.build(pt) }); err != nil {
		t.Fatal(err)
	}
	entries, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	if got := entryObjNums(entries); !reflect.DeepEqual(got, []int{10, 11, 12, 0}) {
		t.Fatalf("entries %v, want the three images examined, then the error row", got)
	}
	want := "image walk stopped after 3 resource entries at page 2; page 3 was not walked"
	if entries[3].Err != want {
		t.Errorf("error row %q, want %q", entries[3].Err, want)
	}
	groups, err := ins.GetImagePageGroups("raw")
	if err != nil {
		t.Fatal(err)
	}
	var nums []int
	var incomplete []bool
	for _, g := range groups {
		nums = append(nums, g.PageNum)
		incomplete = append(incomplete, g.Incomplete)
	}
	if !reflect.DeepEqual(nums, []int{1, 2}) || !reflect.DeepEqual(incomplete, []bool{false, true}) {
		t.Errorf("pages %v incomplete %v, want pages 1 and 2 with only the stopped page incomplete, and no group for the unwalked page", nums, incomplete)
	}
}

func TestImagePageGroupsMarkAPageWhoseFormsPassTheCapIncomplete(t *testing.T) {
	ins, _ := openUnvalidated(t, rawPDF(deepChainObjs(maxFormWalkDepth+8)...))
	groups, err := ins.GetImagePageGroups("raw")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || !groups[0].Incomplete {
		t.Errorf("groups %+v, want the one page marked incomplete", groups)
	}

	ins, _ = openUnvalidated(t, rawPDF(deepChainObjs(maxFormWalkDepth)...))
	groups, err = ins.GetImagePageGroups("raw")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Incomplete {
		t.Errorf("a chain at the cap: groups %+v, want the page complete", groups)
	}
}

func TestImageIndexFormWithoutResourcesContributesNothing(t *testing.T) {
	ins, _ := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3)},
		rawObj{3, imgPage("/Fm 6 0 R /Im0 8 0 R")},
		rawObj{6, formXObj("")},
		rawObj{8, grayImg("")},
	))
	groups, err := ins.GetImagePageGroups("raw")
	if err != nil {
		t.Fatal(err)
	}
	want := []ImagePageUse{{ObjNum: 8, Gen: 0, Path: []string{"Im0"}}}
	if len(groups) != 1 || !reflect.DeepEqual(groups[0].Images, want) {
		t.Errorf("groups %+v, want only Im0 directly on the page", groups)
	}
}

func TestImageIndexSkipsEntriesThatAreNotImages(t *testing.T) {
	entries := rawImageIndex(t,
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3)},
		rawObj{3, imgPage("/Dangling 20 0 R /Direct << /Subtype /Image >> /Dict 7 0 R /NoSub 8 0 R /PS 9 0 R /IndSub 11 0 R /Im0 10 0 R")},
		rawObj{7, "<< /Type /XObject /Subtype /Image >>"},
		rawObj{8, imgStream("/Type /XObject", "q Q")},
		rawObj{9, imgStream("/Type /XObject /Subtype /PS", "%!")},
		rawObj{10, grayImg("")},
		rawObj{11, imgStream("/Type /XObject /Subtype 12 0 R /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8", "xxxx")},
		rawObj{12, "/Image"},
	)
	if got := entryObjNums(entries); !reflect.DeepEqual(got, []int{10}) {
		t.Errorf("entries %v, want only [10]", got)
	}
}

func TestImageIndexFactsReadPanicKeepsTheEntry(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3)},
		rawObj{3, imgPage("/Im0 5 0 R")},
		rawObj{5, grayImg("")},
	))
	pt, _ := doc.pageTree()
	w := newImageWalker(doc.PDFContext)
	w.readFacts = func(*pdfcpu_model.XRefTable, *pdfcpu_types.StreamDict) imageDictFacts { panic("corrupt object stream") }
	if _, err := doc.imageIndex.get(func() (*imageTree, error) { return w.build(pt) }); err != nil {
		t.Fatal(err)
	}
	entries, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries %+v, want the image kept", entries)
	}
	e := entries[0]
	if e.NodeID != "obj:0:5" || !strings.Contains(e.Err, "corrupt object stream") || e.PageCount != 1 {
		t.Errorf("entry %+v, want obj:0:5 on page 1 carrying the panic", *e)
	}
}

func TestImageIndexPartialPageWalkStillListsReachedPages(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3, 4)},
		rawObj{3, imgPage("/Im0 5 0 R")},
		rawObj{4, imgPage("/Im0 6 0 R")},
		rawObj{5, grayImg("")},
		rawObj{6, grayImg("")},
	))
	pw := newPageWalker(doc.PDFContext)
	pw.walk()
	if _, err := doc.pageIndex.get(func() (*pageTree, error) {
		return &pageTree{entries: pw.entries[:1], leaves: pw.leaves[:1], err: errors.New("pdf parsing panic: boom")}, nil
	}); err != nil {
		t.Fatal(err)
	}
	entries, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	if got := entryObjNums(entries); !reflect.DeepEqual(got, []int{5, 0}) {
		t.Fatalf("entries %v, want image 5 then an error row", got)
	}
	if want := "the page tree could not be read past page 1: pdf parsing panic: boom"; entries[1].Err != want {
		t.Errorf("error row %q, want %q", entries[1].Err, want)
	}
}

func TestImageIndexOrderIsFirstUsePageThenObjectNumberAndDeterministic(t *testing.T) {
	objs := []rawObj{
		{1, rawCatalog},
		{2, imgPages("", 3, 4)},
		{3, imgPage("/Z 9 0 R")},
		{4, imgPage("/A 8 0 R /B 7 0 R /C 9 0 R")},
		{7, grayImg("")}, {8, grayImg("")}, {9, grayImg("")},
	}
	first := rawImageIndex(t, objs...)
	if got := entryObjNums(first); !reflect.DeepEqual(got, []int{9, 7, 8}) {
		t.Fatalf("order %v, want [9 7 8]", got)
	}
	a, _ := json.Marshal(first)
	for range 3 {
		b, _ := json.Marshal(rawImageIndex(t, objs...))
		if string(a) != string(b) {
			t.Fatalf("two builds differ:\n%s\n%s", a, b)
		}
	}
}

func TestImageIndexNoImagesIsANonNilEmptySlice(t *testing.T) {
	ins, _ := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3, 4)},
		rawObj{3, imgPage("/Fm 6 0 R")},
		rawObj{4, "<< /Type /Page /Parent 2 0 R " + box + " >>"},
		rawObj{6, formXObj("")},
	))
	entries, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(entries); string(b) != "[]" {
		t.Errorf("entries marshal to %s, want []", b)
	}
}

func TestImagePageGroupsListPagesWithImagesInWalkOrderWithPaths(t *testing.T) {
	ins, _ := openUnvalidated(t, rawPDF(nestedImageObjs()...))
	groups, err := ins.GetImagePageGroups("raw")
	if err != nil {
		t.Fatal(err)
	}
	via := []ImagePageUse{{ObjNum: 8, Gen: 0, Path: []string{"Fm1", "Fm2", "Im0"}}}
	want := []ImagePageGroup{
		{PageNum: 1, NodeID: "obj:0:3", Images: via},
		{PageNum: 2, NodeID: "obj:0:4", Images: via},
	}
	if !reflect.DeepEqual(groups, want) {
		t.Errorf("groups %+v, want %+v; page 3 has no images and is left out", groups, want)
	}
}

func TestImagePageGroupsOfADocumentWithNoImages(t *testing.T) {
	ins, _ := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3, 4)},
		rawObj{3, "<< /Type /Page /Parent 2 0 R " + box + " >>"},
		rawObj{4, "<< /Type /Page /Parent 2 0 R " + box + " >>"},
	))
	groups, err := ins.GetImagePageGroups("raw")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(groups); string(b) != "[]" {
		t.Errorf("groups marshal to %s, want [] since no page has images", b)
	}
	if _, err := NewInspector().GetImagePageGroups("missing"); !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("unknown tab: err = %v, want ErrDocumentNotFound", err)
	}
}

func TestImageIndexDecodeNonDefaultAndVerdicts(t *testing.T) {
	cases := []struct {
		name       string
		obj        string
		nonDefault bool
		verdict    string
	}{
		{"absent", grayImg(""), false, verdictNormalDefault},
		{"identity", grayImg("/Decode [0 1]"), false, verdictNormalDefault},
		{"inverting", grayImg("/Decode [1 0]"), true, verdictInvertedDecode},
		{"partial", rgbImg("/Decode [0 1 1 0 0 1]"), true, verdictNonDefault},
		{"wrong arity", rgbImg("/Decode [0 1]"), true, verdictNonDefault},
		{"odd length", grayImg("/Decode [0 1 0]"), true, verdictUnknownDecode},
		{"non-number element", grayImg("/Decode [0 /One]"), true, verdictUnknownDecode},
		{"indexed with an explicit array", imgStream("/Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace [/Indexed /DeviceGray 1 <00FF>] /BitsPerComponent 8 /Decode [0 1]", "\x00\x01\x01\x00"), true, verdictNotClassified},
		{"indexed with the default array", imgStream("/Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace [/Indexed /DeviceGray 1 <00FF>] /BitsPerComponent 8 /Decode [0 255]", "\x00\x01\x01\x00"), false, verdictNotClassified},
		{"4-bit indexed with the default array", imgStream("/Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace [/Indexed /DeviceGray 1 <00FF>] /BitsPerComponent 4 /Decode [0 15]", "\x00\x10"), false, verdictNotClassified},
		{"indexed with an inverting array", imgStream("/Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace [/Indexed /DeviceGray 1 <00FF>] /BitsPerComponent 8 /Decode [255 0]", "\x00\x01\x01\x00"), true, verdictNotClassified},
		{"lab with the default array and no /Range", labImg("", "/Decode [0 100 -100 100 -100 100]"), false, verdictNotClassified},
		{"lab with the default array for its /Range", labImg("/Range [-128 127 -128 127]", "/Decode [0 100 -128 127 -128 127]"), false, verdictNotClassified},
		{"lab with an array that ignores its /Range", labImg("/Range [-128 127 -128 127]", "/Decode [0 100 -100 100 -100 100]"), true, verdictNotClassified},
		{"lab with an inverted lightness", labImg("", "/Decode [100 0 -100 100 -100 100]"), true, verdictNotClassified},
		{"lab whose /Range is unreadable", labImg("/Range [-128 127]", "/Decode [0 100 -100 100 -100 100]"), true, verdictNotClassified},
		{"stencil mask without an array", imgStream("/Type /XObject /Subtype /Image /Width 8 /Height 1 /ImageMask true /BitsPerComponent 1", "\xaa"), false, verdictNormalDefault},
		{"stencil mask identity", imgStream("/Type /XObject /Subtype /Image /Width 8 /Height 1 /ImageMask true /BitsPerComponent 1 /Decode [0 1]", "\xaa"), false, verdictNormalDefault},
		{"stencil mask inverting", imgStream("/Type /XObject /Subtype /Image /Width 8 /Height 1 /ImageMask true /BitsPerComponent 1 /Decode [1 0]", "\xaa"), true, verdictInvertedDecode},
		{"unresolved count with an identity array", unresolvedImg("/Decode [0 1 0 1 0 1 0 1]"), false, verdictUnknownArity},
		{"unresolved count with an inverting array", unresolvedImg("/Decode [1 0 1 0]"), true, verdictUnknownArity},
		{"unresolved count with a partial array", unresolvedImg("/Decode [0 1 1 0]"), true, verdictUnknownArity},
		{"unresolved count with a non-unit range", unresolvedImg("/Decode [0 0.5]"), true, verdictUnknownArity},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			entries := rawImageIndex(t,
				rawObj{1, rawCatalog},
				rawObj{2, imgPages("", 3)},
				rawObj{3, imgPage("/Im0 5 0 R")},
				rawObj{5, c.obj},
			)
			e := imageEntryFor(t, entries, 5)
			if e.DecodeNonDefault != c.nonDefault || e.SampleInterpretation != c.verdict {
				t.Errorf("decodeNonDefault %v verdict %q, want %v and %q", e.DecodeNonDefault, e.SampleInterpretation, c.nonDefault, c.verdict)
			}
			rejected := c.verdict == verdictUnknownDecode
			if rejected != strings.Contains(e.Warning, "decode array metadata") {
				t.Errorf("warning %q; a rejected array must be named there and nothing else", e.Warning)
			}
			if rejected && e.Decode != nil {
				t.Errorf("a rejected array reads as nil, got %v", e.Decode)
			}
			if e.Err != "" {
				t.Errorf("error %q; a /Decode problem never makes an error row", e.Err)
			}
		})
	}
}

func TestImageIndexNonIntegerWidthIsAWarningOnAKeptEntry(t *testing.T) {
	ins, _ := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3)},
		rawObj{3, imgPage("/Im0 5 0 R /Im1 6 0 R")},
		rawObj{5, imgStream("/Type /XObject /Subtype /Image /Width 2.5 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8", "xxxx")},
		rawObj{6, grayImg("/Decode [0 1 0]")},
	))
	entries, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	e := imageEntryFor(t, entries, 5)
	if e.Width != 0 || e.Height != 2 || !strings.Contains(e.Warning, "Width metadata") || e.Err != "" {
		t.Errorf("entry %+v, want 0x2 with a Width warning and no error", *e)
	}

	// DescribeImage reads the same helper but reports metadata warnings only.
	desc, err := ins.DescribeImage("raw", "obj:0:5")
	if err != nil || desc.Warning != e.Warning {
		t.Errorf("DescribeImage warning %q (%v), want %q", desc.Warning, err, e.Warning)
	}
	desc, err = ins.DescribeImage("raw", "obj:0:6")
	if err != nil || desc.Warning != "" || desc.Width != 2 || desc.EstimatedBytes != 4 {
		t.Errorf("DescribeImage on a rejected /Decode = %+v (%v), want no warning, 2 wide, 4 bytes", desc, err)
	}
}

func TestImageIndexEmptyListsMarshalAsArrays(t *testing.T) {
	entries := rawImageIndex(t, deepChainObjs(maxFormWalkDepth+2)...)
	b, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	var raw []map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil || len(raw) != 2 {
		t.Fatalf("got %s", b)
	}
	if string(raw[0]["filters"]) != "[]" {
		t.Errorf("an unfiltered image marshals filters %s, want []", raw[0]["filters"])
	}
	for _, k := range []string{"filters", "firstPages", "firstPageNodeIds"} {
		if string(raw[1][k]) != "[]" {
			t.Errorf("an error row marshals %s %s, want []", k, raw[1][k])
		}
	}
}

func TestImageIndexIsCachedAndBuiltOnce(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(sharedImageObjs()...))
	if doc.imageIndex.isBuilt() {
		t.Fatal("the image index was built before the first call")
	}
	first, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	if !doc.imageIndex.isBuilt() {
		t.Fatal("the image index was not cached")
	}
	second, _ := ins.GetImageIndex("raw")
	if !reflect.DeepEqual(first, second) {
		t.Errorf("the second call returned %+v, want %+v", second, first)
	}
	builds := 0
	if _, err := doc.imageIndex.get(func() (*imageTree, error) { builds++; return &imageTree{}, nil }); err != nil || builds != 0 {
		t.Errorf("the cache rebuilt: %d builds, err %v", builds, err)
	}
}

func TestImageIndexIsFreshAfterReopenUnderTheSameTab(t *testing.T) {
	ins := NewInspector()
	t.Cleanup(func() { _ = ins.Close("tab") })
	one := rawPDF(rawObj{1, rawCatalog}, rawObj{2, imgPages(box, 3)}, rawObj{3, imgPage("/Im0 5 0 R")}, rawObj{5, grayImg("")})
	two := rawPDF(rawObj{1, rawCatalog}, rawObj{2, imgPages(box, 3)}, rawObj{3, imgPage("/Im0 5 0 R /Im1 6 0 R")}, rawObj{5, grayImg("")}, rawObj{6, grayImg("")})
	openValidatedFile(t, ins, "tab", one)
	if entries, err := ins.GetImageIndex("tab"); err != nil || len(entries) != 1 {
		t.Fatalf("first document: %d entries, err %v", len(entries), err)
	}
	openValidatedFile(t, ins, "tab", two)
	if entries, err := ins.GetImageIndex("tab"); err != nil || len(entries) != 2 {
		t.Errorf("re-opened document: %d entries, err %v; want 2", len(entries), err)
	}
}

func TestImageIndexUnknownTab(t *testing.T) {
	if _, err := NewInspector().GetImageIndex("missing"); !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("err = %v, want ErrDocumentNotFound", err)
	}
}

func TestImageIndexConcurrentWithPageIndexObjectIndexAndImageData(t *testing.T) {
	ins := NewInspector()
	openValidatedFile(t, ins, "tab", rawPDF(sharedImageObjs()...))
	t.Cleanup(func() { _ = ins.Close("tab") })
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(5)
		go func() {
			defer wg.Done()
			if _, err := ins.GetImageIndex("tab"); err != nil {
				t.Errorf("GetImageIndex: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := ins.GetImagePageGroups("tab"); err != nil {
				t.Errorf("GetImagePageGroups: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := ins.GetPageIndex("tab"); err != nil {
				t.Errorf("GetPageIndex: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := ins.GetObjectIndex("tab"); err != nil {
				t.Errorf("GetObjectIndex: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := ins.GetImageData(context.Background(), "tab", "obj:0:5"); err != nil {
				t.Errorf("GetImageData: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestImageIndexFactsMatchGetImageData(t *testing.T) {
	docs := map[string][]byte{
		"shared": rawPDF(sharedImageObjs()...),
		"nested": rawPDF(nestedImageObjs()...),
		"flags": rawPDF(
			rawObj{1, rawCatalog},
			rawObj{2, imgPages(box, 3)},
			rawObj{3, imgPage("/Mask 5 0 R /Soft 6 0 R /Inv 8 0 R /Odd 9 0 R /Part 10 0 R")},
			rawObj{5, imgStream("/Type /XObject /Subtype /Image /Width 8 /Height 1 /ImageMask true /BitsPerComponent 1", "\xaa")},
			rawObj{6, grayImg("/SMask 7 0 R")},
			rawObj{7, grayImg("")},
			rawObj{8, grayImg("/Decode [1 0]")},
			rawObj{9, grayImg("/Decode [0 1 0]")},
			rawObj{10, rgbImg("/Decode [0 1 1 0 0 1]")},
		),
	}
	matches, _ := filepath.Glob(filepath.Join("..", "..", "testdata", "*.pdf"))
	for _, m := range matches {
		b, err := os.ReadFile(m)
		if err != nil {
			t.Fatal(err)
		}
		docs[filepath.Base(m)] = b
	}
	compared := 0
	for name, pdf := range docs {
		ins := NewInspector()
		path := filepath.Join(t.TempDir(), "doc.pdf")
		if err := os.WriteFile(path, pdf, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := ins.Open("tab", path); err != nil {
			continue
		}
		entries, err := ins.GetImageIndex("tab")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, e := range entries {
			if e.NodeID == "" {
				continue
			}
			d, err := ins.GetImageData(context.Background(), "tab", e.NodeID)
			if err != nil {
				t.Fatalf("%s %s: %v", name, e.NodeID, err)
			}
			got := []any{e.Width, e.Height, e.BitsPerComponent, e.ColorSpace, strings.Join(e.Filters, ","), e.ImageMask, e.SMask, e.Decode, e.AdobeMarker, e.AdobeTransform, e.SampleInterpretation, e.EstimatedBytes}
			want := []any{d.Width, d.Height, d.BitsPerComponent, d.ColorSpace, d.Filter, d.ImageMask, d.SMask, d.Decode, d.AdobeMarker, d.AdobeTransform, d.SampleInterpretation, d.DecodedBytes}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s %s: index facts %v, GetImageData %v", name, e.NodeID, got, want)
			}
			compared++
		}
		_ = ins.Close("tab")
	}
	// shared 2, nested 1, flags 5 (object 7 is only an /SMask target), and the
	// one image in testdata/image-xobject.pdf.
	if compared != 9 {
		t.Errorf("compared %d images, want 9", compared)
	}
}

func TestImageIndexReferencesDifferingOnlyInGenerationAreOneEntry(t *testing.T) {
	ins, _ := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3, 4)},
		rawObj{3, imgPage("/Im0 5 0 R")},
		rawObj{4, imgPage("/Im0 5 1 R")},
		rawObj{5, grayImg("")},
	))
	entries, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].PageCount != 2 {
		t.Fatalf("entries %+v, want one entry for object 5 on two pages", entries)
	}
	if pages, err := ins.GetImagePages("raw", 5); err != nil || !reflect.DeepEqual(pageNums(pages), []int{1, 2}) {
		t.Errorf("GetImagePages = %v, %v; want [1 2]", pages, err)
	}
}

func TestImageIndexNonFiniteDecodeIsRejectedAndEncodes(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3)},
		rawObj{3, imgPage("/Im0 5 0 R /Im1 6 0 R")},
		rawObj{5, grayImg("/Decode [0 1]")},
		rawObj{6, grayImg("/Decode [0 1]")},
	))
	// pdfcpu's parser refuses out-of-range reals, so the values are set afterwards.
	setDecode := func(num int, v float64) {
		doc.PDFContext.XRefTable.Table[num].Object.(pdfcpu_types.StreamDict).Dict["Decode"].(pdfcpu_types.Array)[1] = pdfcpu_types.Float(v)
	}
	setDecode(5, math.NaN())
	setDecode(6, math.Inf(1))

	entries, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	for _, num := range []int{5, 6} {
		e := imageEntryFor(t, entries, num)
		if e.Decode != nil || !e.DecodeNonDefault || e.SampleInterpretation != verdictUnknownDecode ||
			!strings.Contains(e.Warning, "/Decode entry 1 is not a finite number") || e.Err != "" {
			t.Errorf("object %d: entry %+v, want the array rejected with a warning", num, *e)
		}
	}
	if _, err := json.Marshal(entries); err != nil {
		t.Errorf("the index must encode as JSON: %v", err)
	}
	// The preview rejects the same array and reports the same verdict.
	for _, num := range []int{5, 6} {
		e := imageEntryFor(t, entries, num)
		d, err := ins.GetImageData(context.Background(), "raw", e.NodeID)
		if err != nil {
			t.Fatal(err)
		}
		if d.Decode != nil || d.SampleInterpretation != e.SampleInterpretation || !strings.Contains(d.Warning, "/Decode entry 1 is not a finite number") {
			t.Errorf("object %d: GetImageData decode %v verdict %q warning %q, want the index's rejection", num, d.Decode, d.SampleInterpretation, d.Warning)
		}
		if _, err := json.Marshal(d); err != nil {
			t.Errorf("object %d: the image data must encode as JSON: %v", num, err)
		}
	}
	desc, err := ins.DescribeImage("raw", "obj:0:5")
	if err != nil || desc.Warning != "" {
		t.Errorf("DescribeImage = %+v (%v), want no warning", desc, err)
	}
}

func TestImageIndexDoesNotDecodeImageStreams(t *testing.T) {
	// The stream is not valid zlib, so any decode of it fails; the index
	// still lists it from the dictionary alone.
	ins, _ := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3)},
		rawObj{3, imgPage("/Im0 5 0 R")},
		rawObj{5, imgStream("/Type /XObject /Subtype /Image /Width 4000 /Height 3000 /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode", "this is not zlib data")},
	))
	if d, err := ins.GetImageData(context.Background(), "raw", "obj:0:5"); err == nil && d.Error == "" {
		t.Fatal("GetImageData decoded an invalid Flate stream; the fixture no longer proves the index skips decoding")
	}
	entries, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	e := imageEntryFor(t, entries, 5)
	if e.Err != "" || e.Warning != "" {
		t.Errorf("entry error %q warning %q; an undecodable stream must not affect a decode-free read", e.Err, e.Warning)
	}
	if e.Width != 4000 || e.Height != 3000 || e.EstimatedBytes != 4000*3000*3 || !reflect.DeepEqual(e.Filters, []string{"FlateDecode"}) {
		t.Errorf("entry %+v, want 4000x3000 FlateDecode estimated at %d bytes", *e, 4000*3000*3)
	}
}

// damagedObject replaces object num in doc with an object-stream member whose
// decode returns decodeErr, or panics with it when panics is set, the way a
// corrupt compressed object fails to resolve.
func damagedObject(t *testing.T, doc *DocumentState, num int, decodeErr error, panics bool) {
	t.Helper()
	osd := pdfcpu_types.NewObjectStreamDict()
	osd.Content = []byte("x")
	lazy := pdfcpu_types.NewLazyObjectStreamObject(osd, 0, -1, func(context.Context, string) (pdfcpu_types.Object, error) {
		if panics {
			panic(decodeErr.Error())
		}
		return nil, decodeErr
	})
	entry, ok := doc.PDFContext.Find(num)
	if !ok {
		t.Fatalf("object %d is not in the xref table", num)
	}
	entry.Object = lazy
}

func TestImageIndexEntryThatPanicsOnResolveIsAnErrorRowAndTheWalkGoesOn(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3, 4)},
		rawObj{3, imgPage("/Bad 9 0 R /Im0 5 0 R")},
		rawObj{4, imgPage("/Im0 6 0 R")},
		rawObj{5, grayImg("")},
		rawObj{6, grayImg("")},
		rawObj{9, "<< >>"},
	))
	damagedObject(t, doc, 9, errors.New("object stream damaged"), true)

	entries, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	if got := entryObjNums(entries); !reflect.DeepEqual(got, []int{5, 6, 0}) {
		t.Fatalf("entries %v, want both images and then one error row", got)
	}
	want := "/XObject entry /Bad (9 0 R) on page 1 could not be read: pdf parsing panic: object stream damaged"
	if entries[2].Err != want {
		t.Errorf("error row %q, want %q", entries[2].Err, want)
	}
	groups, err := ins.GetImagePageGroups("raw")
	if err != nil {
		t.Fatal(err)
	}
	if !groups[0].Incomplete || groups[1].Incomplete {
		t.Errorf("incomplete flags %v %v, want only page 1 incomplete", groups[0].Incomplete, groups[1].Incomplete)
	}
	if len(groups[0].Images) != 1 || groups[0].Images[0].ObjNum != 5 {
		t.Errorf("page 1 images %+v, want the image after the unreadable entry", groups[0].Images)
	}
}

func TestImageIndexPageResourcesThatPanicOnResolveAreAnErrorRowAndLaterPagesAreWalked(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3, 4)},
		rawObj{3, "<< /Type /Page /Parent 2 0 R " + box + " /Resources << /XObject 9 0 R >> >>"},
		rawObj{4, imgPage("/Im0 5 0 R")},
		rawObj{5, grayImg("")},
		rawObj{9, "<< >>"},
	))
	damagedObject(t, doc, 9, errors.New("object stream damaged"), true)

	entries, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	if got := entryObjNums(entries); !reflect.DeepEqual(got, []int{5, 0}) {
		t.Fatalf("entries %v, want the page 2 image and then one error row", got)
	}
	want := "the resources of page 1 could not be read: pdf parsing panic: object stream damaged"
	if entries[1].Err != want {
		t.Errorf("error row %q, want %q", entries[1].Err, want)
	}
	groups, err := ins.GetImagePageGroups("raw")
	if err != nil {
		t.Fatal(err)
	}
	if !groups[0].Incomplete || len(groups[0].Images) != 0 || groups[1].Incomplete || len(groups[1].Images) != 1 {
		t.Errorf("groups %+v, want page 1 incomplete and empty, page 2 complete with its image", groups)
	}
}

func TestImageIndexBudgetSpentInsideAFormSkipsTheRestOfThePage(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3)},
		rawObj{3, imgPage("/A 6 0 R /B 12 0 R")},
		rawObj{6, formXObj("/X 10 0 R /Y 11 0 R")},
		rawObj{10, grayImg("")}, rawObj{11, grayImg("")}, rawObj{12, grayImg("")},
	))
	pt, _ := doc.pageTree()
	w := newImageWalker(doc.PDFContext)
	w.budget = 2
	if _, err := doc.imageIndex.get(func() (*imageTree, error) { return w.build(pt) }); err != nil {
		t.Fatal(err)
	}
	entries, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	if got := entryObjNums(entries); !reflect.DeepEqual(got, []int{10, 0}) {
		t.Fatalf("entries %v, want the one image reached inside the form, then the error row; /B is never examined", got)
	}
	if want := "image walk stopped after 2 resource entries at page 1"; entries[1].Err != want {
		t.Errorf("error row %q, want %q", entries[1].Err, want)
	}
}

func TestReadImageDictFactsUnresolvableEntriesAreWarningsInReadOrder(t *testing.T) {
	_, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("")},
		rawObj{9, "<< >>"},
	))
	damagedObject(t, doc, 9, errors.New("object stream damaged"), false)
	bad := pdfcpu_types.IndirectRef{ObjectNumber: 9}
	sd := pdfcpu_types.StreamDict{Dict: pdfcpu_types.Dict{
		"Subtype":    pdfcpu_types.Name("Image"),
		"Width":      bad,
		"ImageMask":  bad,
		"ColorSpace": bad,
	}}

	f := readImageDictFacts(doc.PDFContext.XRefTable, &sd)
	want := "Width metadata: object stream damaged; ImageMask metadata: object stream damaged; ColorSpace metadata: object stream damaged"
	if f.warning != want {
		t.Errorf("warning %q, want %q", f.warning, want)
	}
	if f.width != 0 || f.height != 0 || f.bitsPerComponent != 8 || f.imageMask || f.colorSpace != "" {
		t.Errorf("facts %+v, want zero geometry, the default 8 bits, no mask and no colour space", f)
	}
	if f.estimatedBytes != 0 {
		t.Errorf("estimatedBytes %d, want 0 with no geometry", f.estimatedBytes)
	}
}

func TestReadImageDictMetadataSkipsTheDecodeAndMarkerReads(t *testing.T) {
	_, doc := openUnvalidated(t, rawPDF(rawObj{1, rawCatalog}, rawObj{2, imgPages("")}))
	sd := pdfcpu_types.StreamDict{
		Dict: pdfcpu_types.Dict{
			"Subtype":          pdfcpu_types.Name("Image"),
			"Width":            pdfcpu_types.Integer(4),
			"Height":           pdfcpu_types.Integer(2),
			"ColorSpace":       pdfcpu_types.Name("DeviceGray"),
			"BitsPerComponent": pdfcpu_types.Integer(8),
			"Decode":           pdfcpu_types.Array{pdfcpu_types.Integer(1), pdfcpu_types.Integer(0)},
			"SMask":            pdfcpu_types.IndirectRef{ObjectNumber: 7},
		},
		FilterPipeline: []pdfcpu_types.PDFFilter{{Name: "DCTDecode"}},
		Raw:            []byte{0xFF, 0xD8, 0xFF, 0xDA},
	}
	m := readImageDictMetadata(doc.PDFContext.XRefTable, &sd)
	if m.adobeMarker != "" || m.sampleInterpretation != "" || m.decode != nil || m.decodeNonDefault || m.smask != nil {
		t.Errorf("metadata %+v, want no APP14 scan, verdict, /Decode or /SMask read", m)
	}
	f := readImageDictFacts(doc.PDFContext.XRefTable, &sd)
	if f.width != m.width || f.height != m.height || f.colorSpace != m.colorSpace || f.estimatedBytes != m.estimatedBytes || f.warning != m.warning {
		t.Errorf("facts %+v and metadata %+v disagree on the shared fields", f, m)
	}
	if f.adobeMarker == "" || f.sampleInterpretation == "" || f.decode == nil || f.smask == nil {
		t.Errorf("facts %+v, want the APP14 outcome, verdict, /Decode and /SMask", f)
	}
}

func TestReadImageDictFactsPanickingEntriesAreWarnings(t *testing.T) {
	_, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("")},
		rawObj{9, "<< >>"},
	))
	damagedObject(t, doc, 9, errors.New("boom"), true)
	bad := pdfcpu_types.IndirectRef{ObjectNumber: 9}
	sd := pdfcpu_types.StreamDict{Dict: pdfcpu_types.Dict{
		"Subtype":    pdfcpu_types.Name("Image"),
		"Width":      pdfcpu_types.Integer(4),
		"Height":     pdfcpu_types.Integer(2),
		"ColorSpace": bad,
	}}

	f := readImageDictFacts(doc.PDFContext.XRefTable, &sd)
	if want := "ColorSpace metadata: pdf parsing panic: boom"; f.warning != want {
		t.Errorf("warning %q, want %q", f.warning, want)
	}
	if f.width != 4 || f.height != 2 {
		t.Errorf("geometry %dx%d, want 4x2 read before the failing entry", f.width, f.height)
	}
}

func TestImageIndexResultsAreCopies(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3)},
		rawObj{3, imgPage("/Fm 6 0 R /Im0 5 0 R")},
		rawObj{5, grayImg("/Decode [1 0] /SMask 7 0 R")},
		rawObj{6, formXObj("/Im1 8 0 R")},
		rawObj{7, grayImg("")},
		rawObj{8, grayImg("")},
	))
	entries, _ := ins.GetImageIndex("raw")
	wantEntries, _ := json.Marshal(entries)
	groups, _ := ins.GetImagePageGroups("raw")
	wantGroups, _ := json.Marshal(groups)
	pages, _ := ins.GetImagePages("raw", 5)

	e := imageEntryFor(t, entries, 5)
	e.Width = 99
	e.Filters = append(e.Filters, "X")
	e.FirstPages[0] = 99
	e.FirstPageNodeIDs[0] = "X"
	e.Decode[0] = 99
	*e.SMask = "99 0 R"
	entries[0] = nil
	groups[0].Images[0].Path[0] = "X"
	groups[0].Images[1].ObjNum = 99
	groups[0].Incomplete = true
	pages[0].PageNum = 99

	if got, _ := ins.GetImageIndex("raw"); !reflect.DeepEqual(mustJSON(t, got), wantEntries) {
		t.Errorf("index after mutating a result:\n%s\nwant\n%s", mustJSON(t, got), wantEntries)
	}
	if got, _ := ins.GetImagePageGroups("raw"); !reflect.DeepEqual(mustJSON(t, got), wantGroups) {
		t.Errorf("groups after mutating a result:\n%s\nwant\n%s", mustJSON(t, got), wantGroups)
	}
	if got, _ := ins.GetImagePages("raw", 5); !reflect.DeepEqual(got, []ImagePageRef{{PageNum: 1, NodeID: "obj:0:3"}}) {
		t.Errorf("pages after mutating a result %v, want [1]", got)
	}
	if !doc.imageIndex.isBuilt() {
		t.Fatal("the image index was not cached")
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// buildWithBudget builds the image index of doc with a walker limited to
// budget entries and returns the walker.
func buildWithBudget(t *testing.T, doc *DocumentState, budget int) *imageWalker {
	t.Helper()
	pt, _ := doc.pageTree()
	w := newImageWalker(doc.PDFContext)
	w.budget = budget
	if _, err := doc.imageIndex.get(func() (*imageTree, error) { return w.build(pt) }); err != nil {
		t.Fatal(err)
	}
	return w
}

func TestImageIndexSharedResourcesAreWalkedAndChargedOnce(t *testing.T) {
	shapes := map[string]string{
		"inherited indirect /Resources": "/Resources 9 0 R",
		"inherited direct /Resources":   "/Resources << /XObject << /A 10 0 R /B 11 0 R /C 12 0 R >> >>",
		"indirect /XObject dictionary":  "/Resources << /XObject 9 0 R >>",
	}
	for name, attr := range shapes {
		t.Run(name, func(t *testing.T) {
			objs := []rawObj{{1, rawCatalog}, {9, "<< /A 10 0 R /B 11 0 R /C 12 0 R >>"}, {10, grayImg("")}, {11, grayImg("")}, {12, grayImg("")}}
			if attr == "/Resources 9 0 R" {
				objs[1] = rawObj{9, "<< /XObject << /A 10 0 R /B 11 0 R /C 12 0 R >> >>"}
			}
			kids := make([]int, 50)
			for i := range kids {
				kids[i] = 100 + i
				objs = append(objs, rawObj{100 + i, "<< /Type /Page /Parent 2 0 R " + box + " >>"})
			}
			objs = append(objs, rawObj{2, imgPages(attr, kids...)})
			ins, doc := openUnvalidated(t, rawPDF(objs...))
			w := buildWithBudget(t, doc, 3)
			if w.examined != 3 || w.stopped {
				t.Errorf("examined %d entries, stopped %v; want the shared dictionary charged once", w.examined, w.stopped)
			}
			entries, _ := ins.GetImageIndex("raw")
			if got := entryObjNums(entries); !reflect.DeepEqual(got, []int{10, 11, 12}) {
				t.Fatalf("entries %v, want [10 11 12] and no error row", got)
			}
			for _, e := range entries {
				if e.PageCount != 50 {
					t.Errorf("object %d on %d pages, want 50", e.ObjNum, e.PageCount)
				}
			}
			groups, _ := ins.GetImagePageGroups("raw")
			want := []ImagePageUse{{ObjNum: 10, Path: []string{"A"}}, {ObjNum: 11, Path: []string{"B"}}, {ObjNum: 12, Path: []string{"C"}}}
			for _, g := range groups {
				if g.Incomplete || !reflect.DeepEqual(g.Images, want) {
					t.Fatalf("page %d: %+v, want %+v and complete", g.PageNum, g, want)
				}
			}
		})
	}
}

func TestImageIndexSharedFormIsWalkedAndChargedOnce(t *testing.T) {
	objs := []rawObj{{1, rawCatalog}, {6, formXObj("/A 10 0 R /B 11 0 R /C 12 0 R")}, {10, grayImg("")}, {11, grayImg("")}, {12, grayImg("")}}
	kids := make([]int, 20)
	for i := range kids {
		kids[i] = 100 + i
		objs = append(objs, rawObj{100 + i, imgPage("/Fm 6 0 R")})
	}
	objs = append(objs, rawObj{2, imgPages("", kids...)})
	ins, doc := openUnvalidated(t, rawPDF(objs...))
	// One entry per page plus the form's three, once.
	w := buildWithBudget(t, doc, 20+3)
	if w.examined != 23 || w.stopped {
		t.Errorf("examined %d entries, stopped %v; want 23 and the walk finished", w.examined, w.stopped)
	}
	groups, _ := ins.GetImagePageGroups("raw")
	want := []ImagePageUse{{ObjNum: 10, Path: []string{"Fm", "A"}}, {ObjNum: 11, Path: []string{"Fm", "B"}}, {ObjNum: 12, Path: []string{"Fm", "C"}}}
	for _, g := range groups {
		if g.Incomplete || !reflect.DeepEqual(g.Images, want) {
			t.Fatalf("page %d: %+v, want %+v and complete", g.PageNum, g, want)
		}
	}
}

func TestImageIndexFormSharedAtTwoDepthsIsWalkedAndChargedOnce(t *testing.T) {
	// S is reached inside A at depth 1, then straight from the page at depth 0.
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3)},
		rawObj{3, imgPage("/A 10 0 R /S 12 0 R")},
		rawObj{10, formXObj("/S 12 0 R")},
		rawObj{12, formXObj("/Im0 20 0 R")},
		rawObj{20, grayImg("")},
	))
	w := buildWithBudget(t, doc, 100)
	// The page's two entries, A's one and S's one, once.
	if w.examined != 4 || w.stopped {
		t.Errorf("examined %d entries, stopped %v; want 4 and S charged once", w.examined, w.stopped)
	}
	groups, _ := ins.GetImagePageGroups("raw")
	want := []ImagePageGroup{{PageNum: 1, NodeID: "obj:0:3", Images: []ImagePageUse{{ObjNum: 20, Path: []string{"A", "S", "Im0"}}}}}
	if !reflect.DeepEqual(groups, want) {
		t.Errorf("groups %+v, want %+v", groups, want)
	}
}

func TestImageIndexSharedFormIsWalkedAgainWhereItsDepthPassesTheCap(t *testing.T) {
	// Page 1 enters the 30-form chain at 10 directly; page 2 enters it under
	// five wrapper forms, which puts its last form past the cap.
	objs := []rawObj{{1, rawCatalog}, {2, imgPages("", 3, 4)}, {3, imgPage("/C 10 0 R")}, {4, imgPage("/W 50 0 R")}, {6, grayImg("")}}
	for i := range 30 {
		x := fmt.Sprintf("/C %d 0 R", 11+i)
		if i == 29 {
			x = "/Im0 6 0 R"
		}
		objs = append(objs, rawObj{10 + i, formXObj(x)})
	}
	for i := range 5 {
		x := fmt.Sprintf("/W %d 0 R", 51+i)
		if i == 4 {
			x = "/C 10 0 R"
		}
		objs = append(objs, rawObj{50 + i, formXObj(x)})
	}
	ins, _ := openUnvalidated(t, rawPDF(objs...))
	groups, err := ins.GetImagePageGroups("raw")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || groups[0].Incomplete || len(groups[0].Images) != 1 || !groups[1].Incomplete || len(groups[1].Images) != 0 {
		t.Errorf("groups %+v, want page 1 complete with image 6 and page 2 incomplete with none", groups)
	}
	entries, _ := ins.GetImageIndex("raw")
	if got := entryObjNums(entries); !reflect.DeepEqual(got, []int{6, 0}) || !strings.Contains(entries[1].Err, "on page 2") {
		t.Errorf("entries %v, want image 6 then the nesting cap's error row for page 2", got)
	}
}

func TestImageIndexFormWalkedInsideACycleIsNotReusedOutsideIt(t *testing.T) {
	// On page 1, G is walked inside F and skips its /Back to F. On page 2, G
	// is reached at the same depth through X, where /Back leads to F's image.
	ins, _ := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3, 4)},
		rawObj{3, imgPage("/F 6 0 R")},
		rawObj{4, imgPage("/X 9 0 R")},
		rawObj{6, formXObj("/G 7 0 R /ImA 10 0 R")},
		rawObj{7, formXObj("/Back 6 0 R /ImB 11 0 R")},
		rawObj{9, formXObj("/G 7 0 R")},
		rawObj{10, grayImg("")},
		rawObj{11, grayImg("")},
	))
	groups, err := ins.GetImagePageGroups("raw")
	if err != nil {
		t.Fatal(err)
	}
	want := []ImagePageGroup{
		{PageNum: 1, NodeID: "obj:0:3", Images: []ImagePageUse{{ObjNum: 11, Path: []string{"F", "G", "ImB"}}, {ObjNum: 10, Path: []string{"F", "ImA"}}}},
		{PageNum: 2, NodeID: "obj:0:4", Images: []ImagePageUse{{ObjNum: 10, Path: []string{"X", "G", "Back", "ImA"}}, {ObjNum: 11, Path: []string{"X", "G", "ImB"}}}},
	}
	if !reflect.DeepEqual(groups, want) {
		t.Errorf("groups %+v, want %+v", groups, want)
	}
}

func TestImageIndexBrokenSharedResourcesWriteOneErrorRow(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("/Resources << /XObject 9 0 R >>", 3, 4, 5)},
		rawObj{3, "<< /Type /Page /Parent 2 0 R " + box + " >>"},
		rawObj{4, "<< /Type /Page /Parent 2 0 R " + box + " >>"},
		rawObj{5, "<< /Type /Page /Parent 2 0 R " + box + " /Resources << /XObject << /Im0 8 0 R /Bad 7 0 R >> >> >>"},
		rawObj{7, "<< >>"},
		rawObj{8, grayImg("")},
		rawObj{9, "<< >>"},
	))
	damagedObject(t, doc, 9, errors.New("object stream damaged"), true)
	damagedObject(t, doc, 7, errors.New("object stream damaged"), true)

	entries, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	if got := entryObjNums(entries); !reflect.DeepEqual(got, []int{8, 0, 0}) {
		t.Fatalf("entries %v, want image 8, one row for the shared resources and one for page 3's entry", got)
	}
	if want := "the resources of pages 1-2 could not be read: pdf parsing panic: object stream damaged"; entries[1].Err != want {
		t.Errorf("error row %q, want %q", entries[1].Err, want)
	}
	groups, _ := ins.GetImagePageGroups("raw")
	if len(groups) != 3 {
		t.Fatalf("groups %+v, want all three pages listed as incomplete", groups)
	}
	for _, g := range groups {
		if !g.Incomplete {
			t.Errorf("page %d complete, want every page sharing the broken resources and page 3 incomplete", g.PageNum)
		}
	}
	if b, _ := json.Marshal(groups[0]); !strings.Contains(string(b), `"images":[]`) {
		t.Errorf("an incomplete page with no images marshals as %s, want images []", b)
	}
}

// wrappedSharedFormObjs has n pages (objs 100..), each naming its own wrapper
// form (objs 200..), and every wrapper names the one shared form 6 holding n
// images (objs 10..).
func wrappedSharedFormObjs(n int) []rawObj {
	shared := make([]string, n)
	objs := []rawObj{{1, rawCatalog}}
	kids := make([]int, n)
	for i := range n {
		shared[i] = fmt.Sprintf("/Im%d %d 0 R", i, 10+i)
		kids[i] = 100 + i
		objs = append(objs,
			rawObj{10 + i, grayImg("")},
			rawObj{100 + i, imgPage(fmt.Sprintf("/W %d 0 R", 200+i))},
			rawObj{200 + i, formXObj("/S 6 0 R")},
		)
	}
	return append(objs, rawObj{6, formXObj(strings.Join(shared, " "))}, rawObj{2, imgPages("", kids...)})
}

func TestImageIndexUseLimitStopsTheWalk(t *testing.T) {
	// Each page retains 10 uses merged into its wrapper, 10 merged from the
	// wrapper into the page and 10 copied into its group, so pages 1 and 2
	// retain 60.
	cases := []struct {
		name  string
		limit int
	}{
		{"inside a form merge", 65},
		{"copying into a page group", 80},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ins, doc := openUnvalidated(t, rawPDF(wrappedSharedFormObjs(10)...))
			pt, _ := doc.pageTree()
			w := newImageWalker(doc.PDFContext)
			w.useLimit = c.limit
			if _, err := doc.imageIndex.get(func() (*imageTree, error) { return w.build(pt) }); err != nil {
				t.Fatal(err)
			}
			if !w.stopped || w.uses > c.limit {
				t.Errorf("stopped %v with %d uses, want the walk stopped within %d", w.stopped, w.uses, c.limit)
			}
			entries, _ := ins.GetImageIndex("raw")
			last := entries[len(entries)-1]
			want := fmt.Sprintf("image walk stopped at the limit of %d image uses at page 3; pages 4 to 10 were not walked", c.limit)
			if last.NodeID != "" || last.Err != want {
				t.Errorf("last row %+v, want error %q", *last, want)
			}
			for _, e := range entries[:len(entries)-1] {
				if e.NodeID == "" || e.PageCount != 2 {
					t.Errorf("entry %+v, want an image on pages 1 and 2 only", *e)
				}
			}
			groups, _ := ins.GetImagePageGroups("raw")
			if len(groups) != 3 {
				t.Fatalf("%d groups, want pages 1 to 3 and none for the pages after the stop", len(groups))
			}
			for _, g := range groups {
				wantIncomplete := g.PageNum == 3
				if g.Incomplete != wantIncomplete || (wantIncomplete && len(g.Images) != 0) || (!wantIncomplete && len(g.Images) != 10) {
					t.Errorf("page %d: incomplete %v with %d images", g.PageNum, g.Incomplete, len(g.Images))
				}
			}
		})
	}
}

func TestImageIndexPageThatFailedToReadIsAnErrorRowAndStillWalked(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3, 4)},
		rawObj{3, imgPage("/Im0 5 0 R")},
		rawObj{4, imgPage("/Im0 6 0 R")},
		rawObj{5, grayImg("")},
		rawObj{6, grayImg("")},
	))
	pw := newPageWalker(doc.PDFContext)
	pw.walk()
	pw.leaves[0].err = errors.New("page 1 could not be read: pdf parsing panic: boom")
	if _, err := doc.pageIndex.get(func() (*pageTree, error) {
		return &pageTree{entries: pw.entries, leaves: pw.leaves}, nil
	}); err != nil {
		t.Fatal(err)
	}
	entries, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	if got := entryObjNums(entries); !reflect.DeepEqual(got, []int{5, 6, 0}) {
		t.Fatalf("entries %v, want both images then one error row", got)
	}
	if want := "page 1 could not be read: pdf parsing panic: boom"; entries[2].Err != want {
		t.Errorf("error row %q, want %q", entries[2].Err, want)
	}
	groups, _ := ins.GetImagePageGroups("raw")
	if !groups[0].Incomplete || len(groups[0].Images) != 1 || groups[1].Incomplete {
		t.Errorf("groups %+v, want page 1 incomplete with its image and page 2 complete", groups)
	}
}

func TestImageIndexCopiesUnnumberedPageTreeErrorRows(t *testing.T) {
	ins, _ := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 20 0 R null] /Count 2 >>"},
		rawObj{3, imgPage("/Im0 5 0 R")},
		rawObj{5, grayImg("")},
	))
	pages, err := ins.GetPageIndex("raw")
	if err != nil || len(pages) != 3 {
		t.Fatalf("page index %+v (%v), want the page and two unnumbered rows", pages, err)
	}
	entries, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	if got := entryObjNums(entries); !reflect.DeepEqual(got, []int{5, 0, 0}) {
		t.Fatalf("entries %v, want the image then one row per page-tree problem", got)
	}
	want := []string{
		"page tree (20 0 R): " + pages[1].Err,
		"page tree: /Kids entry 2 is null",
	}
	for i, w := range want {
		if e := entries[1+i]; e.NodeID != "" || e.Err != w {
			t.Errorf("row %d %+v, want error %q", 1+i, *e, w)
		}
	}
}

func TestImageIndexResourcesThatFailToResolveAreErrorRows(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("/Resources << /XObject 9 0 R >>", 3, 4, 5)},
		rawObj{3, "<< /Type /Page /Parent 2 0 R " + box + " >>"},
		rawObj{4, "<< /Type /Page /Parent 2 0 R " + box + " >>"},
		rawObj{5, imgPage("/Fm 7 0 R /Im0 6 0 R /Gone 30 0 R")},
		rawObj{6, grayImg("")},
		rawObj{7, imgStream("/Type /XObject /Subtype /Form /BBox [0 0 10 10] /Resources 8 0 R", "q Q")},
		rawObj{8, "<< >>"},
		rawObj{9, "<< >>"},
	))
	damagedObject(t, doc, 9, errors.New("object stream damaged"), false)
	damagedObject(t, doc, 8, errors.New("object stream damaged"), false)

	entries, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	if got := entryObjNums(entries); !reflect.DeepEqual(got, []int{6, 0, 0}) {
		t.Fatalf("entries %v, want image 6, one row for the shared /XObject and one for the form's /Resources", got)
	}
	want := []string{
		"the resources of pages 1-2 could not be read: /XObject: object stream damaged",
		"/XObject entry /Fm (7 0 R) on page 3 could not be read: /Resources: object stream damaged",
	}
	for i, w := range want {
		if entries[1+i].Err != w {
			t.Errorf("row %d %q, want %q", 1+i, entries[1+i].Err, w)
		}
	}
	groups, _ := ins.GetImagePageGroups("raw")
	for _, g := range groups {
		if !g.Incomplete {
			t.Errorf("page %d complete, want every page incomplete", g.PageNum)
		}
	}
	if len(groups[2].Images) != 1 || groups[2].Images[0].ObjNum != 6 {
		t.Errorf("page 3 images %+v, want image 6", groups[2].Images)
	}
}

func TestImageIndexXObjectThatFailsToResolveIsAnErrorRow(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3, 4)},
		rawObj{3, imgPage("/Bad 9 0 R /Im0 5 0 R")},
		rawObj{4, imgPage("/Gone 30 0 R /Im0 6 0 R")},
		rawObj{5, grayImg("")},
		rawObj{6, grayImg("")},
		rawObj{9, "<< >>"},
	))
	damagedObject(t, doc, 9, errors.New("object stream damaged"), false)

	entries, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	if got := entryObjNums(entries); !reflect.DeepEqual(got, []int{5, 6, 0}) {
		t.Fatalf("entries %v, want both images and one error row; the dangling /Gone is skipped", got)
	}
	if want := "/XObject entry /Bad (9 0 R) on page 1 could not be read: object stream damaged"; entries[2].Err != want {
		t.Errorf("error row %q, want %q", entries[2].Err, want)
	}
	groups, _ := ins.GetImagePageGroups("raw")
	if !groups[0].Incomplete || len(groups[0].Images) != 1 || groups[1].Incomplete {
		t.Errorf("groups %+v, want page 1 incomplete with its image and page 2 complete", groups)
	}
}

func TestImagePageGroupsLeaveOutAnUnreadablePageAfterTheStop(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3, 4, 5)},
		rawObj{3, imgPage("/A 10 0 R /B 11 0 R")},
		rawObj{4, imgPage("/A 12 0 R /B 13 0 R")},
		rawObj{5, imgPage("/A 14 0 R")},
		rawObj{10, grayImg("")}, rawObj{11, grayImg("")}, rawObj{12, grayImg("")},
		rawObj{13, grayImg("")}, rawObj{14, grayImg("")},
	))
	pw := newPageWalker(doc.PDFContext)
	pw.walk()
	pw.leaves[2].err = errors.New("page 3 could not be read: pdf parsing panic: boom")
	pt := &pageTree{entries: pw.entries, leaves: pw.leaves}
	w := newImageWalker(doc.PDFContext)
	w.budget = 3
	if _, err := doc.imageIndex.get(func() (*imageTree, error) { return w.build(pt) }); err != nil {
		t.Fatal(err)
	}
	entries, _ := ins.GetImageIndex("raw")
	var errs []string
	for _, e := range entries {
		if e.NodeID == "" {
			errs = append(errs, e.Err)
		}
	}
	want := []string{
		"image walk stopped after 3 resource entries at page 2; page 3 was not walked",
		"page 3 could not be read: pdf parsing panic: boom",
	}
	if !reflect.DeepEqual(errs, want) {
		t.Errorf("error rows %q, want %q", errs, want)
	}
	groups, _ := ins.GetImagePageGroups("raw")
	var nums []int
	for _, g := range groups {
		nums = append(nums, g.PageNum)
	}
	if !reflect.DeepEqual(nums, []int{1, 2}) {
		t.Errorf("group pages %v, want [1 2] and no group for the unreadable page after the stop", nums)
	}
}

// formChain returns n chained forms numbered from base, the last listing
// image 5.
func formChain(base, n int) []rawObj {
	objs := make([]rawObj, n)
	for i := range n {
		x := fmt.Sprintf("/Fm %d 0 R", base+i+1)
		if i == n-1 {
			x = "/Im 5 0 R"
		}
		objs[i] = rawObj{base + i, formXObj(x)}
	}
	return objs
}

func TestImageIndexNestingCapWritesOneRowPerCappedFormNamingItsPages(t *testing.T) {
	objs := []rawObj{
		{1, rawCatalog},
		{2, imgPages("", 3, 4, 6, 7)},
		{3, imgPage("/Fm 100 0 R")},
		{4, imgPage("/Fm 200 0 R")},
		{5, grayImg("")},
		{6, imgPage("/Fm 200 0 R")},
		{7, imgPage("/Fm 200 0 R")},
	}
	objs = append(objs, formChain(100, maxFormWalkDepth+2)...)
	objs = append(objs, formChain(200, maxFormWalkDepth+2)...)
	ins, _ := openUnvalidated(t, rawPDF(objs...))
	entries, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	var errs []string
	for _, e := range entries {
		if e.NodeID == "" {
			errs = append(errs, e.Err)
		}
	}
	want := []string{
		fmt.Sprintf("Form XObject nesting deeper than 32 at %d 0 R on page 1; deeper forms were not walked", 100+maxFormWalkDepth),
		fmt.Sprintf("Form XObject nesting deeper than 32 at %d 0 R on pages 2-4; deeper forms were not walked", 200+maxFormWalkDepth),
	}
	if !reflect.DeepEqual(errs, want) {
		t.Errorf("error rows %q, want %q", errs, want)
	}
	groups, _ := ins.GetImagePageGroups("raw")
	if len(groups) != 4 {
		t.Fatalf("groups %+v, want all four pages incomplete", groups)
	}
	for _, g := range groups {
		if !g.Incomplete {
			t.Errorf("page %d complete, want it incomplete", g.PageNum)
		}
	}
}

func TestImageIndexFailingObjectRowNamesEveryPageItCutShort(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3, 4, 5, 6)},
		rawObj{3, imgPage("/Fm 7 0 R /Bad 9 0 R")},
		rawObj{4, imgPage("/Fm 7 0 R")},
		rawObj{5, imgPage("/Im0 8 0 R")},
		rawObj{6, imgPage("/Other 7 0 R /Broken 9 0 R")},
		rawObj{7, imgStream("/Type /XObject /Subtype /Form /BBox [0 0 10 10] /Resources 10 0 R", "q Q")},
		rawObj{8, grayImg("")},
		rawObj{9, "<< >>"},
		rawObj{10, "<< >>"},
	))
	damagedObject(t, doc, 9, errors.New("object stream damaged"), false)
	damagedObject(t, doc, 10, errors.New("object stream damaged"), false)

	entries, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	var errs []string
	for _, e := range entries {
		if e.NodeID == "" {
			errs = append(errs, e.Err)
		}
	}
	want := []string{
		"/XObject entry /Bad (9 0 R) on pages 1, 4 could not be read: object stream damaged",
		"/XObject entry /Fm (7 0 R) on pages 1-2, 4 could not be read: /Resources: object stream damaged",
	}
	if !reflect.DeepEqual(errs, want) {
		t.Errorf("error rows %q, want %q", errs, want)
	}
	groups, _ := ins.GetImagePageGroups("raw")
	var incomplete []int
	for _, g := range groups {
		if g.Incomplete {
			incomplete = append(incomplete, g.PageNum)
		}
	}
	if !reflect.DeepEqual(incomplete, []int{1, 2, 4}) {
		t.Errorf("incomplete pages %v, want [1 2 4]", incomplete)
	}
}

func TestPageRangeList(t *testing.T) {
	cases := []struct {
		ranges [][2]int
		want   string
	}{
		{[][2]int{{3, 3}}, "page 3"},
		{[][2]int{{2, 50}}, "pages 2-50"},
		{[][2]int{{1, 1}, {4, 6}, {9, 9}}, "pages 1, 4-6, 9"},
	}
	for _, c := range cases {
		if got := pageRangeList(c.ranges); got != c.want {
			t.Errorf("pageRangeList(%v) = %q, want %q", c.ranges, got, c.want)
		}
	}
}

func TestImageIndexBuildLetsOtherCallsRunBetweenPages(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(sharedImageObjs()...))
	w := newImageWalker(doc.PDFContext)
	w.afterPage = func(page int) {
		if page != 1 {
			return
		}
		done := make(chan error, 1)
		go func() {
			_, err := ins.GetChildren("raw", "root")
			done <- err
		}()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("GetChildren: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("GetChildren did not complete while the image walk was between pages")
		}
	}
	if _, err := doc.imageIndex.get(func() (*imageTree, error) { return doc.buildImageTree(w) }); err != nil {
		t.Fatal(err)
	}
}

func TestImageIndexConcurrentCallerWaitsOnTheBuildInProgress(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(sharedImageObjs()...))
	w := newImageWalker(doc.PDFContext)
	// A width no other build would report marks entries from this build.
	w.readFacts = func(xrt *pdfcpu_model.XRefTable, sd *pdfcpu_types.StreamDict) imageDictFacts {
		f := readImageDictFacts(xrt, sd)
		f.width = 777
		return f
	}
	type result struct {
		entries []*ImageIndexEntry
		err     error
	}
	got := make(chan result, 1)
	w.afterPage = func(page int) {
		switch page {
		case 1:
			go func() {
				entries, err := ins.GetImageIndex("raw")
				got <- result{entries, err}
			}()
		case 20:
			select {
			case <-got:
				t.Error("GetImageIndex returned before the build in progress finished")
			case <-time.After(50 * time.Millisecond):
			}
		}
	}
	if _, err := doc.imageIndex.get(func() (*imageTree, error) { return doc.buildImageTree(w) }); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-got:
		if r.err != nil || len(r.entries) == 0 || r.entries[0].Width != 777 {
			t.Errorf("GetImageIndex %+v, err %v; want the entries of the build in progress", r.entries, r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("GetImageIndex did not return after the build finished")
	}
}

func TestImageIndexCloseDuringTheBuildStopsIt(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(sharedImageObjs()...))
	w := newImageWalker(doc.PDFContext)
	walked := 0
	w.afterPage = func(page int) {
		walked++
		if page == 1 {
			if err := ins.Close("raw"); err != nil {
				t.Errorf("Close: %v", err)
			}
		}
	}
	_, err := doc.imageIndex.get(func() (*imageTree, error) { return doc.buildImageTree(w) })
	if !errors.Is(err, context.Canceled) {
		t.Errorf("build err %v, want context.Canceled", err)
	}
	if walked != 1 {
		t.Errorf("walked %d pages, want the walk to stop after page 1", walked)
	}
	if doc.imageIndex.isBuilt() {
		t.Error("a build stopped by a close was cached")
	}
}

func TestImageIndexReadsOfABuiltIndexDoNotWaitForPdfMu(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(sharedImageObjs()...))
	if _, err := ins.GetImageIndex("raw"); err != nil {
		t.Fatal(err)
	}
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()
	done := make(chan error, 1)
	go func() {
		if _, err := ins.GetImagePages("raw", 5); err != nil {
			done <- err
			return
		}
		if _, err := ins.GetImagePageGroups("raw"); err != nil {
			done <- err
			return
		}
		_, err := ins.GetImageIndex("raw")
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reads of the built index waited for pdfMu")
	}
}

func TestImageIndexFormLadderOverASelfReferencingFormIsWalkedInLinearWork(t *testing.T) {
	// A_k reaches A_k+1 through both B_k and C_k; the bottom form Z names
	// itself and the image.
	const levels = 14
	a := func(k int) int { return 100 + 3*k }
	objs := []rawObj{{1, rawCatalog}, {2, imgPages("", 3)}, {3, imgPage(fmt.Sprintf("/A %d 0 R", a(0)))}, {6, grayImg("")}}
	for k := range levels {
		objs = append(objs,
			rawObj{a(k), formXObj(fmt.Sprintf("/B %d 0 R /C %d 0 R", a(k)+1, a(k)+2))},
			rawObj{a(k) + 1, formXObj(fmt.Sprintf("/A %d 0 R", a(k+1)))},
			rawObj{a(k) + 2, formXObj(fmt.Sprintf("/A %d 0 R", a(k+1)))},
		)
	}
	objs = append(objs, rawObj{a(levels), formXObj(fmt.Sprintf("/Im 6 0 R /Self %d 0 R", a(levels)))})
	ins, doc := openUnvalidated(t, rawPDF(objs...))
	w := buildWithBudget(t, doc, maxImageWalkEntries)
	// One page entry, two per A_k, one per B_k and C_k, and Z's two.
	if want := 1 + 4*levels + 2; w.examined != want {
		t.Errorf("examined %d entries, want %d", w.examined, want)
	}
	groups, _ := ins.GetImagePageGroups("raw")
	path := []string{}
	for range levels {
		path = append(path, "A", "B")
	}
	path = append(path, "A", "Im")
	want := []ImagePageGroup{{PageNum: 1, NodeID: "obj:0:3", Images: []ImagePageUse{{ObjNum: 6, Path: path}}}}
	if !reflect.DeepEqual(groups, want) {
		t.Errorf("groups %+v, want %+v", groups, want)
	}
}

func TestImageIndexFormInACycleIsNotReusedWhereAFormOfTheCycleIsAbove(t *testing.T) {
	// F and G name each other and the image is in G. Page 1 reaches F
	// through X, page 2 reaches G straight from its resources.
	ins, _ := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("", 3, 4)},
		rawObj{3, imgPage("/X 9 0 R")},
		rawObj{4, imgPage("/G 7 0 R")},
		rawObj{6, formXObj("/G 7 0 R")},
		rawObj{7, formXObj("/F 6 0 R /Im 10 0 R")},
		rawObj{9, formXObj("/F 6 0 R")},
		rawObj{10, grayImg("")},
	))
	groups, err := ins.GetImagePageGroups("raw")
	if err != nil {
		t.Fatal(err)
	}
	want := []ImagePageGroup{
		{PageNum: 1, NodeID: "obj:0:3", Images: []ImagePageUse{{ObjNum: 10, Path: []string{"X", "F", "G", "Im"}}}},
		{PageNum: 2, NodeID: "obj:0:4", Images: []ImagePageUse{{ObjNum: 10, Path: []string{"G", "Im"}}}},
	}
	if !reflect.DeepEqual(groups, want) {
		t.Errorf("groups %+v, want %+v", groups, want)
	}
}

func TestImageIndexCappedFormWalkIsNotReusedWhereItsCycleIsAbove(t *testing.T) {
	// Page 1 reaches H at depth 22 under a chain of 22 forms; H leads to X and
	// the ten-form chain Y back to H, which passes the nesting cap. Page 2
	// reaches X at depth 11 and H through Y at depth 22 again, where H's
	// /X closes a cycle and nothing passes the cap.
	const h, x, y, w1, w2 = 50, 51, 60, 100, 200
	objs := []rawObj{
		{1, rawCatalog},
		{2, imgPages("", 3, 4)},
		{3, imgPage(fmt.Sprintf("/W %d 0 R", w1))},
		{4, imgPage(fmt.Sprintf("/W %d 0 R", w2))},
		{6, grayImg("")},
		{h, formXObj(fmt.Sprintf("/X %d 0 R", x))},
		{x, formXObj(fmt.Sprintf("/ImX 6 0 R /Y %d 0 R", y))},
	}
	chain := func(base, n, end int, key string) {
		for i := range n {
			next := base + i + 1
			k := "/W"
			if i == n-1 {
				next, k = end, key
			}
			objs = append(objs, rawObj{base + i, formXObj(fmt.Sprintf("%s %d 0 R", k, next))})
		}
	}
	chain(y, 10, h, "/H")
	chain(w1, 22, h, "/H")
	chain(w2, 11, x, "/X")
	ins, _ := openUnvalidated(t, rawPDF(objs...))
	groups, err := ins.GetImagePageGroups("raw")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 || !groups[0].Incomplete || groups[1].Incomplete {
		t.Fatalf("groups %+v, want page 1 incomplete at the cap and page 2 complete", groups)
	}
	path := slices.Repeat([]string{"W"}, 11)
	path = append(path, "X", "ImX")
	if want := []ImagePageUse{{ObjNum: 6, Path: path}}; !reflect.DeepEqual(groups[1].Images, want) {
		t.Errorf("page 2 images %+v, want %+v", groups[1].Images, want)
	}
	entries, _ := ins.GetImageIndex("raw")
	for _, e := range entries {
		if e.NodeID == "" && !strings.HasSuffix(e.Err, "on page 1; deeper forms were not walked") {
			t.Errorf("error row %q, want only page 1 named", e.Err)
		}
	}
}

func TestImageIndexResourcesEntryThatFailsToResolveIsAnErrorRowAndInheritedResourcesAreWalked(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, imgPages("/Resources << /XObject << /Im0 5 0 R >> >>", 3, 4, 10)},
		rawObj{3, "<< /Type /Page /Parent 2 0 R " + box + " /Resources 9 0 R >>"},
		rawObj{4, "<< /Type /Pages /Parent 2 0 R /Kids [6 0 R 7 0 R] /Count 2 /Resources 8 0 R >>"},
		rawObj{5, grayImg("")},
		rawObj{6, "<< /Type /Page /Parent 4 0 R " + box + " >>"},
		rawObj{7, "<< /Type /Page /Parent 4 0 R " + box + " >>"},
		rawObj{8, "<< >>"},
		rawObj{9, "<< >>"},
		rawObj{10, imgPage("/Im1 11 0 R")},
		rawObj{11, grayImg("")},
	))
	damagedObject(t, doc, 8, errors.New("object stream damaged"), false)
	damagedObject(t, doc, 9, errors.New("object stream damaged"), false)

	entries, err := ins.GetImageIndex("raw")
	if err != nil {
		t.Fatal(err)
	}
	if got := entryObjNums(entries); !reflect.DeepEqual(got, []int{5, 11, 0, 0}) {
		t.Fatalf("entries %v, want both images and one row per failing /Resources entry", got)
	}
	want := []string{
		"/Resources of 3 0 R for page 1 could not be read: object stream damaged",
		"/Resources of 4 0 R for pages 2-3 could not be read: object stream damaged",
	}
	for i, w := range want {
		if entries[2+i].Err != w {
			t.Errorf("row %d %q, want %q", 2+i, entries[2+i].Err, w)
		}
	}
	groups, _ := ins.GetImagePageGroups("raw")
	if len(groups) != 4 {
		t.Fatalf("groups %+v, want four", groups)
	}
	for _, g := range groups[:3] {
		if !g.Incomplete || len(g.Images) != 1 || g.Images[0].ObjNum != 5 {
			t.Errorf("page %d: %+v, want incomplete with the inherited image 5", g.PageNum, g)
		}
	}
	if g := groups[3]; g.Incomplete || len(g.Images) != 1 || g.Images[0].ObjNum != 11 {
		t.Errorf("page 4: %+v, want complete with its own image 11", g)
	}
	pages, _ := ins.GetPageIndex("raw")
	if want := "/Resources resolves to nothing; inherited value used"; pages[0].Err != want {
		t.Errorf("page 1 row error %q, want %q", pages[0].Err, want)
	}
}

func TestImageWalkPageLockIsReleasedWhenTheWalkPanics(t *testing.T) {
	var mu sync.Mutex
	w := newImageWalker(nil)
	w.pdfMu = &mu
	func() {
		defer func() { _ = recover() }()
		w.walkLeafLocked(nil, 1, "")
	}()
	if !mu.TryLock() {
		t.Fatal("pdfMu is still held after a panic in the page walk")
	}
	mu.Unlock()
}
