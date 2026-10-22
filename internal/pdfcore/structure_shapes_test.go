package pdfcore

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

// Structure-walk branches for malformed entries, read from in-memory
// documents through openUnvalidated.

// problemsContain reports whether some problem has where as its location
// prefix and msg in its message.
func problemsContain(r *StructureReport, where, msg string) bool {
	for _, p := range r.Problems {
		if strings.HasPrefix(p.Where, where) && strings.Contains(p.Message, msg) {
			return true
		}
	}
	return false
}

// onlyKid returns the single kid of the single element under the root.
func onlyKid(t *testing.T, r *StructureReport) *StructKid {
	t.Helper()
	if len(r.Elements) != 1 || r.Elements[0].Element == nil || len(r.Elements[0].Element.Kids) != 1 {
		t.Fatalf("root kids = %v, want one element with one kid", kindsOf(r.Elements))
	}
	return r.Elements[0].Element.Kids[0]
}

func TestGetStructure_CleanFileActualTextValue(t *testing.T) {
	r := getStructure(t, openStructureFixture(t, "structure.pdf"), "s")
	p := mustElement(t, r, "P")
	if p.ActualText == nil || *p.ActualText != "Hello world" {
		t.Errorf("P ActualText = %v, want \"Hello world\" decoded from UTF-16BE", p.ActualText)
	}
	if p.Alt != nil || p.E != nil {
		t.Errorf("P Alt %v E %v, want both absent (nil)", p.Alt, p.E)
	}
}

func TestGetStructure_MalformedRootEntriesAreProblems(t *testing.T) {
	cases := []struct {
		name string
		root string
		msg  string
	}{
		{"RoleMap not a dictionary", "<< /Type /StructTreeRoot /RoleMap 7 >>", "/RoleMap is an integer, not a dictionary"},
		{"RoleMap value not a name", "<< /Type /StructTreeRoot /RoleMap << /X 7 >> >>", "/RoleMap /X maps to an integer, not a name"},
		{"ParentTreeNextKey not an integer", "<< /Type /StructTreeRoot /ParentTreeNextKey (x) >>", "/ParentTreeNextKey is a string, not an integer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := rawStructure(t, structTreePDF(tc.root))
			if !problemsContain(r, "StructTreeRoot 5 0 R", tc.msg) {
				t.Errorf("Problems = %+v, want one at StructTreeRoot 5 0 R with %q", r.Problems, tc.msg)
			}
			if r.ParentTreeNextKey != nil {
				t.Errorf("ParentTreeNextKey = %v, want nil", *r.ParentTreeNextKey)
			}
		})
	}
}

func TestGetStructure_RoleMapValueNotANameIsListedButNotFollowed(t *testing.T) {
	r := rawStructure(t, structTreePDF("<< /Type /StructTreeRoot /RoleMap << /X 7 >> /K 10 0 R >>",
		rawObj{10, "<< /Type /StructElem /S /X >>"},
	))
	if !slices.Equal(r.RoleMap, []RoleMapEntry{{From: "X", To: ""}}) {
		t.Errorf("RoleMap = %+v, want [{X \"\"}]", r.RoleMap)
	}
	if e := mustElement(t, r, "X"); e.MappedS != "" || e.Err != "" {
		t.Errorf("element X MappedS %q Err %q, want unmapped with no error", e.MappedS, e.Err)
	}
}

func TestGetStructure_StructTreeRootNotADictionary(t *testing.T) {
	r := rawStructure(t, structTreePDF("42"))
	if !r.HasStructTree || r.RootRef != "5 0 R" {
		t.Errorf("HasStructTree %v RootRef %q, want true and 5 0 R", r.HasStructTree, r.RootRef)
	}
	if !problemsContain(r, "StructTreeRoot 5 0 R", "/StructTreeRoot is an integer, not a dictionary") {
		t.Errorf("Problems = %+v, want the root type named", r.Problems)
	}
	if r.Elements == nil || len(r.Elements) != 0 || len(r.Pages) != 1 {
		t.Errorf("Elements %v, %d pages; want an empty non-nil slice and page 1 still listed", r.Elements, len(r.Pages))
	}
}

func TestGetStructure_CatalogLangThatDoesNotDecodeIsAProblem(t *testing.T) {
	all := []rawObj{{1, "<< /Type /Catalog /Pages 2 0 R /Lang 5 >>"}}
	all = append(all, structPagesObjs(1, nil, nil)...)
	r := rawStructure(t, rawPDF(all...))
	if !problemsContain(r, "catalog", "/Lang is an integer that does not decode as a text string") {
		t.Errorf("Problems = %+v, want the /Lang problem", r.Problems)
	}
	if r.HasStructTree {
		t.Error("HasStructTree = true with no /StructTreeRoot")
	}
}

func TestGetStructure_MCRKidShapes(t *testing.T) {
	cases := []struct {
		name   string
		mcr    string
		stmRef string
		mcid   int // -1 for no MCID
		err    string
	}{
		{"direct /Stm", "<< /Type /MCR /MCID 0 /Stm << /X 1 >> >>", "(direct)", 0, "/Stm is a dictionary, not a reference to a stream"},
		{"/Stm to a non-stream", "<< /Type /MCR /MCID 0 /Stm 3 0 R >>", "3 0 R", 0, "/Stm 3 0 R is a dictionary, not a stream"},
		{"/Stm to a stream", "<< /Type /MCR /MCID 0 /Stm 9001 0 R >>", "9001 0 R", 0, ""},
		{"/MCID not an integer", "<< /Type /MCR /MCID (x) >>", "", -1, "MCR /MCID is a string, not an integer"},
		{"negative /MCID", "<< /Type /MCR /MCID -1 >>", "", -1, "MCR /MCID -1 is negative"},
		{"own /Pg not a page", "<< /Type /MCR /MCID 0 /Pg 5 0 R >>", "", 0, "/Pg 5 0 R is a dictionary that is not a page"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := rawStructure(t, structTreePDF("<< /Type /StructTreeRoot /K 10 0 R >>",
				rawObj{10, "<< /Type /StructElem /S /P /Pg 3 0 R /K " + tc.mcr + " >>"},
			))
			k := onlyKid(t, r)
			if k.Kind != "mcr" || k.StmRef != tc.stmRef {
				t.Errorf("kid = %+v, want mcr with StmRef %q", *k, tc.stmRef)
			}
			if (tc.mcid < 0 && k.MCID != nil) || (tc.mcid >= 0 && !intPtrIs(k.MCID, tc.mcid)) {
				t.Errorf("MCID = %v, want %d (-1 for none)", k.MCID, tc.mcid)
			}
			if tc.err == "" && k.Err != "" || !strings.Contains(k.Err, tc.err) {
				t.Errorf("Err = %q, want %q", k.Err, tc.err)
			}
			if tc.stmRef != "" && len(mustPage(t, r, 1).Alignment) != 0 {
				t.Errorf("an MCR with /Stm is aligned on page 1: %+v", mustPage(t, r, 1).Alignment)
			}
		})
	}
}

func TestGetStructure_OBJRKidShapes(t *testing.T) {
	form := rawObj{30, "<< /Type /XObject /Subtype /Form /BBox [0 0 1 1] /Length 0 >>\nstream\n\nendstream"}
	subtypeOnly := rawObj{31, "<< /Subtype /Widget >>"}
	cases := []struct {
		name    string
		objr    string
		objRef  string
		objType string
		err     string
	}{
		{"no /Obj", "<< /Type /OBJR >>", "", "", "OBJR has no /Obj"},
		{"direct /Obj", "<< /Type /OBJR /Obj << /Type /Annot >> >>", "", "", "OBJR /Obj is a dictionary, not a reference"},
		{"/Obj dangling", "<< /Type /OBJR /Obj 99 0 R >>", "99 0 R", "", "OBJR /Obj 99 0 R does not resolve to an object"},
		{"/Obj to a stream", "<< /Type /OBJR /Obj 30 0 R >>", "30 0 R", "XObject/Form", ""},
		{"/Obj with only /Subtype", "<< /Type /OBJR /Obj 31 0 R >>", "31 0 R", "Widget", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := rawStructure(t, structTreePDF("<< /Type /StructTreeRoot /K 10 0 R >>",
				rawObj{10, "<< /Type /StructElem /S /Link /Pg 3 0 R /K " + tc.objr + " >>"}, form, subtypeOnly,
			))
			k := onlyKid(t, r)
			if k.Kind != "objr" || k.ObjRef != tc.objRef || k.ObjType != tc.objType || k.Page != 1 {
				t.Errorf("kid = %+v, want objr ObjRef %q ObjType %q on page 1", *k, tc.objRef, tc.objType)
			}
			if tc.err == "" && k.Err != "" || !strings.Contains(k.Err, tc.err) {
				t.Errorf("Err = %q, want %q", k.Err, tc.err)
			}
		})
	}
}

func TestGetStructure_ElementEntryShapes(t *testing.T) {
	cases := []struct {
		name  string
		elem  string
		check func(t *testing.T, e *StructElem)
		err   string
	}{
		{"/S not a name", "<< /Type /StructElem /S (P) >>", func(t *testing.T, e *StructElem) {
			if e.S != "" {
				t.Errorf("S = %q, want empty", e.S)
			}
		}, "/S is a string, not a name"},
		{"direct /Pg", "<< /Type /StructElem /S /P /Pg << /Type /Page >> >>", func(t *testing.T, e *StructElem) {
			if e.Page != 0 || e.PgRef != "(direct)" {
				t.Errorf("Page %d PgRef %q, want 0 and (direct)", e.Page, e.PgRef)
			}
		}, "/Pg is a dictionary, not a reference to a page"},
		{"dangling /Pg", "<< /Type /StructElem /S /P /Pg 99 0 R >>", func(t *testing.T, e *StructElem) {
			if e.Page != 0 || e.PgRef != "99 0 R" {
				t.Errorf("Page %d PgRef %q, want 0 and 99 0 R", e.Page, e.PgRef)
			}
		}, "/Pg 99 0 R does not resolve to an object"},
		{"/Alt not a text string", "<< /Type /StructElem /S /Figure /Alt 5 >>", func(t *testing.T, e *StructElem) {
			if e.Alt == nil {
				t.Error("Alt = nil, want the raw value shown")
			}
		}, "/Alt is an integer that does not decode as a text string"},
		{"empty /E and /T and /Lang read", "<< /Type /StructElem /S /Span /E () /T (Intro) /Lang (fr) >>", func(t *testing.T, e *StructElem) {
			if e.E == nil || *e.E != "" || e.Title != "Intro" || e.Lang != "fr" {
				t.Errorf("E %v T %q Lang %q, want present-but-empty E, T Intro, Lang fr", e.E, e.Title, e.Lang)
			}
			if e.Alt != nil || e.ActualText != nil {
				t.Errorf("Alt %v ActualText %v, want nil when absent", e.Alt, e.ActualText)
			}
		}, ""},
		{"/Type other than StructElem with an /S", "<< /Type /Foo /S /P /Pg 3 0 R /K 0 >>", func(t *testing.T, e *StructElem) {
			if e.S != "P" {
				t.Errorf("S = %q, want P", e.S)
			}
			if len(e.Kids) != 1 || e.Kids[0].Kind != "mcid" {
				t.Errorf("kids = %v, want the element walked with its one MCID kid", kindsOf(e.Kids))
			}
		}, "/Type /Foo is not StructElem"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := rawStructure(t, structTreePDF("<< /Type /StructTreeRoot /K 10 0 R >>", rawObj{10, tc.elem}))
			if len(r.Elements) != 1 || r.Elements[0].Element == nil {
				t.Fatalf("root kids = %v, want one element", kindsOf(r.Elements))
			}
			e := r.Elements[0].Element
			tc.check(t, e)
			if tc.err == "" && e.Err != "" || !strings.Contains(e.Err, tc.err) {
				t.Errorf("Err = %q, want %q", e.Err, tc.err)
			}
		})
	}
}

// pageParentTreePDF is a one-page tagged document whose page carries
// pageExtra and whose root carries rootExtra; element 10 is a /P on page 1
// claiming MCID 4, element 11 a /Span with no page.
func pageParentTreePDF(rootExtra, pageExtra string, objs ...rawObj) []byte {
	all := []rawObj{
		{1, "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 5 0 R >>"},
		{5, "<< /Type /StructTreeRoot /K [10 0 R 11 0 R] " + rootExtra + " >>"},
		{10, "<< /Type /StructElem /S /P /Pg 3 0 R /K 4 >>"},
		{11, "<< /Type /StructElem /S /Span >>"},
	}
	all = append(all, structPagesObjs(1, nil, []string{pageExtra})...)
	return rawPDF(append(all, objs...)...)
}

func TestGetStructure_PageStructParentsErrors(t *testing.T) {
	cases := []struct {
		name      string
		rootExtra string
		pageExtra string
		objs      []rawObj
		err       string
	}{
		{"not an integer", "/ParentTree 6 0 R", "/StructParents (x)", []rawObj{{6, "<< /Nums [] >>"}}, "/StructParents is a string, not an integer"},
		{"no ParentTree", "", "/StructParents 0", nil, "/StructParents 0 but the document has no ParentTree"},
		{"value not an array", "/ParentTree 6 0 R", "/StructParents 0", []rawObj{{6, "<< /Nums [0 5] >>"}}, "ParentTree key 0 is an integer, not an array"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := rawStructure(t, pageParentTreePDF(tc.rootExtra, tc.pageExtra, tc.objs...))
			p := mustPage(t, r, 1)
			if p.ParentTreeErr != tc.err {
				t.Errorf("ParentTreeErr = %q, want %q", p.ParentTreeErr, tc.err)
			}
			if p.ParentTree == nil || len(p.ParentTree) != 0 {
				t.Errorf("ParentTree = %v, want an empty non-nil slice", p.ParentTree)
			}
		})
	}
}

func TestGetStructure_ParentTreeSlotShapes(t *testing.T) {
	r := rawStructure(t, pageParentTreePDF("/ParentTree 6 0 R", "/StructParents 0",
		rawObj{6, "<< /Nums [0 [5 99 0 R 9001 0 R null 10 0 R]] >>"},
	))
	p := mustPage(t, r, 1)
	want := []ParentTreeSlot{
		{MCID: 0, Err: "value is an integer, not a reference to an element"},
		{MCID: 1, ElementRef: "99 0 R", Err: "99 0 R does not resolve to an object"},
		{MCID: 2, ElementRef: "9001 0 R", Err: "9001 0 R is a stream, not an element"},
		{MCID: 3},
		{MCID: 4, ElementRef: "10 0 R", S: "P"},
	}
	if !slices.Equal(p.ParentTree, want) {
		t.Errorf("ParentTree = %+v\nwant %+v", p.ParentTree, want)
	}
	if got := alignmentMCIDs(p); !slices.Equal(got, []int{0, 1, 2, 4}) {
		t.Errorf("alignment MCIDs = %v, want [0 1 2 4] (error slots kept, null slot dropped)", got)
	}
	for _, mcid := range []int{0, 1, 2} {
		if a := alignmentRow(t, p, mcid); a.ParentTreeElement != "" || a.Consistent {
			t.Errorf("MCID %d row = %+v, want no ParentTree element and not consistent", mcid, a)
		}
	}
	if a := alignmentRow(t, p, 4); a.ParentTreeElement != "10 0 R" || a.ContentCount != 0 || a.Consistent {
		t.Errorf("MCID 4 row = %+v, want ParentTree 10 0 R, no content, not consistent", a)
	}
}

func TestGetStructure_OtherParentTreeValueShapes(t *testing.T) {
	r := rawStructure(t, pageParentTreePDF("/ParentTree 6 0 R", "/StructParents 0",
		rawObj{6, "<< /Nums [0 [] 7 [10 0 R] 8 11 0 R 9 5] >>"},
	))
	want := []ParentTreeEntry{
		{Key: 7, Err: "value is an array, but no page's /StructParents names this key"},
		{Key: 8, ElementRef: "11 0 R", S: "Span"},
		{Key: 9, Err: "value is an integer, not a reference to an element"},
	}
	if !slices.Equal(r.OtherParentTree, want) {
		t.Errorf("OtherParentTree = %+v\nwant %+v", r.OtherParentTree, want)
	}
}

func TestGetStructure_ContentsShapes(t *testing.T) {
	cases := []struct {
		name     string
		contents string
		pageErr  string
		streams  int
	}{
		{"array element not a reference", "[5]", "contents array element 0 is not an indirect reference", 0},
		{"not a reference or array", "5", "unexpected Contents type", 0},
		{"dangling stream ref", "[99 0 R 20 0 R]", "", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := rawStructure(t, rawPDF(
				rawObj{1, rawCatalog},
				rawObj{2, "<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>"},
				rawObj{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents " + tc.contents + " >>"},
				rawObj{4, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 21 0 R >>"},
				rawObj{20, rawStream("/P <</MCID 0>> BDC EMC")},
				rawObj{21, rawStream("/P <</MCID 3>> BDC EMC")},
			))
			p1 := mustPage(t, r, 1)
			if !strings.Contains(p1.Err, tc.pageErr) || (tc.pageErr == "") != (p1.Err == "") {
				t.Errorf("page 1 Err = %q, want %q", p1.Err, tc.pageErr)
			}
			if len(p1.Streams) != tc.streams {
				t.Fatalf("page 1 streams = %+v, want %d", p1.Streams, tc.streams)
			}
			if tc.streams == 2 {
				if p1.Streams[0].Ref != "99 0 R" || p1.Streams[0].Err == "" {
					t.Errorf("stream 0 = %+v, want 99 0 R with an error", p1.Streams[0])
				}
				if p1.Streams[1].Err != "" || len(p1.ContentMCIDs) != 1 {
					t.Errorf("stream 1 = %+v, MCIDs %+v; want the next stream scanned", p1.Streams[1], p1.ContentMCIDs)
				}
			}
			p2 := mustPage(t, r, 2)
			if len(p2.ContentMCIDs) != 1 || p2.ContentMCIDs[0].MCID != 3 {
				t.Errorf("page 2 ContentMCIDs = %+v, want MCID 3 (page 2 still scanned)", p2.ContentMCIDs)
			}
		})
	}
}

func TestGetStructure_NamedPropertyListLookup(t *testing.T) {
	cases := []struct {
		name      string
		pagesRes  string
		resources string
		mcid      int // -1 for an error row, -2 for no row
		err       string
	}{
		{"no /Resources", "", "", -1, "property list /MC0 not found in /Resources /Properties"},
		{"no /Properties", "", "<< /Font << >> >>", -1, "property list /MC0 not found"},
		{"name not in /Properties", "", "<< /Properties << /MC1 << /MCID 1 >> >> >>", -1, "property list /MC0 not found"},
		{"entry not a dictionary", "", "<< /Properties << /MC0 5 >> >>", -1, "property list /MC0 is an integer, not a dictionary"},
		{"entry without /MCID", "", "<< /Properties << /MC0 << /X 1 >> >> >>", -2, ""},
		{"indirect /Properties and entry", "", "<< /Properties 40 0 R >>", 6, ""},
		{"inherited from /Pages", "/Resources << /Properties << /MC0 << /MCID 8 >> >> >>", "", 8, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := ""
			if tc.resources != "" {
				res = "/Resources " + tc.resources
			}
			r := rawStructure(t, rawPDF(
				rawObj{1, rawCatalog},
				rawObj{2, "<< /Type /Pages /Kids [3 0 R] /Count 1 " + tc.pagesRes + " >>"},
				rawObj{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 20 0 R " + res + " >>"},
				rawObj{20, rawStream("/Span /MC0 BDC EMC")},
				rawObj{40, "<< /MC0 41 0 R >>"},
				rawObj{41, "<< /MCID 6 >>"},
			))
			p := mustPage(t, r, 1)
			switch {
			case tc.mcid == -2:
				if len(p.ContentMCIDs) != 0 {
					t.Errorf("ContentMCIDs = %+v, want none", p.ContentMCIDs)
				}
			case len(p.ContentMCIDs) != 1:
				t.Errorf("ContentMCIDs = %+v, want one row", p.ContentMCIDs)
			default:
				c := p.ContentMCIDs[0]
				if c.MCID != tc.mcid || c.Tag != "Span" || !strings.Contains(c.Err, tc.err) || (tc.err == "") != (c.Err == "") {
					t.Errorf("row = %+v, want MCID %d tag Span error %q", c, tc.mcid, tc.err)
				}
			}
		})
	}
}

func TestGetStructure_PagesSharingAStructParentsKeyAreAProblem(t *testing.T) {
	all := []rawObj{
		{1, "<< /Type /Catalog /Pages 2 0 R /StructTreeRoot 5 0 R >>"},
		{5, "<< /Type /StructTreeRoot /K [10 0 R] /ParentTree 6 0 R >>"},
		{6, "<< /Nums [0 [10 0 R]] >>"},
		{10, "<< /Type /StructElem /S /P /Pg 3 0 R /K 0 >>"},
	}
	all = append(all, structPagesObjs(3, nil, []string{"/StructParents 0", "/StructParents 1", "/StructParents 0"})...)
	r := rawStructure(t, rawPDF(all...))
	var found []StructureProblem
	for _, p := range r.Problems {
		if strings.Contains(p.Message, "/StructParents 0") {
			found = append(found, p)
		}
	}
	if len(found) != 1 || found[0].Where != "page 3" || !strings.Contains(found[0].Message, "page 1") {
		t.Errorf("problems = %+v, want one at page 3 naming key 0 and page 1", r.Problems)
	}
	for _, p := range r.Problems {
		if strings.Contains(p.Message, "/StructParents 1") {
			t.Errorf("unshared key reported: %+v", p)
		}
	}
	if len(r.OtherParentTree) != 0 {
		t.Errorf("OtherParentTree = %+v, want none: key 0 is named", r.OtherParentTree)
	}
}

func TestGetStructure_SharedContentStreamIsDecodedOnce(t *testing.T) {
	_, doc := openUnvalidated(t, rawPDF(
		rawObj{1, rawCatalog},
		rawObj{2, "<< /Type /Pages /Kids [3 0 R 4 0 R 7 0 R] /Count 3 >>"},
		rawObj{3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 20 0 R >>"},
		rawObj{4, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents [20 0 R 21 0 R] >>"},
		rawObj{7, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents [21 0 R 20 0 R] >>"},
		rawObj{20, rawStream("/P <</MCID 0>> BDC EMC")},
		rawObj{21, rawStream("/P <</MCID 1>> BDC EMC")},
	))
	w := newStructureWalker(doc.PDFContext)
	r, err := doc.buildStructure(w)
	if err != nil {
		t.Fatalf("buildStructure: %v", err)
	}
	if w.decodes != 2 {
		t.Errorf("decodes = %d, want 2 (one per distinct stream)", w.decodes)
	}
	if len(w.streamMemo) != 0 || len(w.streamUses) != 0 {
		t.Errorf("memo %d entries, uses %d entries after the walk, want both empty", len(w.streamMemo), len(w.streamUses))
	}
	want := [][]string{{"20 0 R"}, {"20 0 R", "21 0 R"}, {"21 0 R", "20 0 R"}}
	for i, p := range r.Pages {
		var refs []string
		for _, s := range p.Streams {
			if s.Err != "" {
				t.Errorf("page %d stream %s: %s", p.PageNum, s.Ref, s.Err)
			}
			refs = append(refs, s.Ref)
		}
		if !slices.Equal(refs, want[i]) || len(p.ContentMCIDs) != len(want[i]) {
			t.Errorf("page %d streams %v with %d MCID rows, want %v with one row each", p.PageNum, refs, len(p.ContentMCIDs), want[i])
		}
	}
}

func TestGetStructure_CloseBeforeTreeWalkCancels(t *testing.T) {
	ins, doc := openUnvalidated(t, threePagePDF())
	w := newStructureWalker(doc.PDFContext)
	var calls []int
	w.beforeLock = func(page int) {
		calls = append(calls, page)
		if page == 0 {
			if err := ins.Close("raw"); err != nil {
				t.Errorf("Close: %v", err)
			}
		}
	}
	r, err := doc.buildStructure(w)
	if !errors.Is(err, context.Canceled) || r != nil {
		t.Errorf("report %v err %v, want nil and an error satisfying errors.Is(err, context.Canceled)", r, err)
	}
	if !slices.Equal(calls, []int{0}) {
		t.Errorf("beforeLock calls = %v, want [0]", calls)
	}
}

func TestJoinErr(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"", "b", "b"},
		{"a", "b", "a; b"},
		{"", "", ""},
	}
	for _, tc := range cases {
		if got := joinErr(tc.a, tc.b); got != tc.want {
			t.Errorf("joinErr(%q, %q) = %q, want %q", tc.a, tc.b, got, tc.want)
		}
	}
}
