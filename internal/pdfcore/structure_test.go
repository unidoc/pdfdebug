package pdfcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

// openStructureFixture opens a committed testdata file under tab "s".
func openStructureFixture(t *testing.T, name string) *Inspector {
	t.Helper()
	ins := NewInspector()
	if _, err := ins.Open("s", filepath.Join(testdataDir(t), name)); err != nil {
		t.Fatalf("Open %s: %v", name, err)
	}
	t.Cleanup(func() { _ = ins.Close("s") })
	return ins
}

func getStructure(t *testing.T, ins *Inspector, tabID string) *StructureReport {
	t.Helper()
	r, err := ins.GetStructure(tabID)
	if err != nil {
		t.Fatalf("GetStructure: %v", err)
	}
	if r == nil {
		t.Fatal("GetStructure returned a nil report")
	}
	return r
}

// rawStructure runs GetStructure on an in-memory document read without
// pdfcpu validation.
func rawStructure(t *testing.T, pdf []byte) *StructureReport {
	t.Helper()
	ins, _ := openUnvalidated(t, pdf)
	return getStructure(t, ins, "raw")
}

// eachKid calls fn on every kid of kids and of their elements, depth first.
func eachKid(kids []*StructKid, fn func(k *StructKid)) {
	for _, k := range kids {
		fn(k)
		if k.Element != nil {
			eachKid(k.Element.Kids, fn)
		}
	}
}

// findElement returns the first element whose /S is s.
func findElement(r *StructureReport, s string) *StructElem {
	var found *StructElem
	eachKid(r.Elements, func(k *StructKid) {
		if found == nil && k.Element != nil && k.Element.S == s {
			found = k.Element
		}
	})
	return found
}

func mustElement(t *testing.T, r *StructureReport, s string) *StructElem {
	t.Helper()
	e := findElement(r, s)
	if e == nil {
		t.Fatalf("no element with /S %s in the report", s)
	}
	return e
}

// kidsWithError returns every kid whose Kind is "error".
func kidsWithError(r *StructureReport) []*StructKid {
	var out []*StructKid
	eachKid(r.Elements, func(k *StructKid) {
		if k.Kind == "error" {
			out = append(out, k)
		}
	})
	return out
}

// kidHasError reports whether a kid, or the element it carries, has an error.
func kidHasError(k *StructKid) bool {
	return k.Kind == "error" || k.Err != "" || (k.Element != nil && k.Element.Err != "")
}

func kindsOf(kids []*StructKid) []string {
	out := make([]string, len(kids))
	for i, k := range kids {
		out[i] = k.Kind
		if k.Element != nil {
			out[i] += ":" + k.Element.S
		}
	}
	return out
}

func alignmentRow(t *testing.T, p *StructPage, mcid int) MCIDAlignment {
	t.Helper()
	for _, a := range p.Alignment {
		if a.MCID == mcid {
			return a
		}
	}
	t.Fatalf("page %d has no alignment row for MCID %d: %+v", p.PageNum, mcid, p.Alignment)
	return MCIDAlignment{}
}

func alignmentMCIDs(p *StructPage) []int {
	out := make([]int, len(p.Alignment))
	for i, a := range p.Alignment {
		out[i] = a.MCID
	}
	return out
}

func mustPage(t *testing.T, r *StructureReport, n int) *StructPage {
	t.Helper()
	for _, p := range r.Pages {
		if p.PageNum == n {
			return p
		}
	}
	t.Fatalf("page %d missing from the report (%d pages)", n, len(r.Pages))
	return nil
}

func intPtrIs(p *int, v int) bool { return p != nil && *p == v }

// ---------------------------------------------------------------------------
// Clean tagged fixture
// ---------------------------------------------------------------------------

func TestGetStructure_CleanFileTree(t *testing.T) {
	r := getStructure(t, openStructureFixture(t, "structure.pdf"), "s")

	if !r.HasStructTree {
		t.Error("HasStructTree = false, want true")
	}
	if r.Marked == nil || !*r.Marked {
		t.Errorf("Marked = %v, want true", r.Marked)
	}
	if r.DocLang != "en-US" {
		t.Errorf("DocLang = %q, want en-US", r.DocLang)
	}
	if !intPtrIs(r.ParentTreeNextKey, 3) {
		t.Errorf("ParentTreeNextKey = %v, want 3", r.ParentTreeNextKey)
	}
	if !slices.Equal(r.RoleMap, []RoleMapEntry{{From: "Heading1", To: "H1"}}) {
		t.Errorf("RoleMap = %+v, want [{Heading1 H1}]", r.RoleMap)
	}
	if len(r.Problems) != 0 {
		t.Errorf("Problems = %+v, want none", r.Problems)
	}
	if !r.HasParentTree || r.ParentTreeRef == "" {
		t.Errorf("HasParentTree = %v, ParentTreeRef = %q; want true and a ref", r.HasParentTree, r.ParentTreeRef)
	}
	if r.RootRef == "" {
		t.Error("RootRef is empty for an indirect /StructTreeRoot")
	}

	if len(r.Elements) != 1 || r.Elements[0].Kind != "element" || r.Elements[0].Element.S != "Document" {
		t.Fatalf("top-level kids = %v, want one Document element", kindsOf(r.Elements))
	}
	doc := r.Elements[0].Element
	if got, want := kindsOf(doc.Kids), []string{"element:Heading1", "element:Figure", "element:Sect", "element:Link"}; !slices.Equal(got, want) {
		t.Fatalf("Document kids = %v, want %v", got, want)
	}

	h1 := doc.Kids[0].Element
	if h1.MappedS != "H1" || h1.Page != 1 || h1.PageInherited || h1.Lang != "en" || h1.Ref == "" {
		t.Errorf("Heading1 = %+v, want MappedS H1, own page 1, Lang en, a ref", *h1)
	}
	if len(h1.Kids) != 1 || h1.Kids[0].Kind != "mcid" || !intPtrIs(h1.Kids[0].MCID, 0) || h1.Kids[0].Page != 1 {
		t.Errorf("Heading1 kids = %+v, want one mcid kid 0 on page 1", h1.Kids)
	}

	fig := doc.Kids[1].Element
	if fig.Page != 1 || fig.Alt == nil || *fig.Alt != "Company logo" {
		t.Errorf("Figure page %d, Alt %v; want page 1, Alt \"Company logo\"", fig.Page, fig.Alt)
	}
	if len(fig.Kids) != 1 || fig.Kids[0].Kind != "mcid" || !intPtrIs(fig.Kids[0].MCID, 1) {
		t.Errorf("Figure kids = %+v, want one mcid kid 1", fig.Kids)
	}

	sect := doc.Kids[2].Element
	if sect.Page != 2 {
		t.Errorf("Sect page = %d, want 2", sect.Page)
	}
	if got := kindsOf(sect.Kids); !slices.Equal(got, []string{"element:P"}) {
		t.Fatalf("Sect kids = %v, want [element:P]", got)
	}
	p := sect.Kids[0].Element
	if p.Page != 2 || !p.PageInherited || p.PgRef != "" {
		t.Errorf("P page %d inherited %v PgRef %q; want page 2 inherited from Sect, no own /Pg", p.Page, p.PageInherited, p.PgRef)
	}
	if p.ActualText == nil || *p.ActualText == "" {
		t.Errorf("P ActualText = %v, want the decoded text", p.ActualText)
	} else if at := *p.ActualText; !utf8.ValidString(at) || strings.ContainsRune(at, 0) || strings.ContainsRune(at, 0xFEFF) {
		t.Errorf("P ActualText = %q, want UTF-16BE decoded (no NUL, no BOM)", at)
	}
	if len(p.Kids) != 1 || p.Kids[0].Kind != "mcr" || !intPtrIs(p.Kids[0].MCID, 0) || p.Kids[0].Page != 2 || p.Kids[0].StmRef != "" {
		t.Errorf("P kids = %+v, want one mcr kid MCID 0 on page 2 with no /Stm", p.Kids)
	}

	link := doc.Kids[3].Element
	if link.Page != 2 {
		t.Errorf("Link page = %d, want 2", link.Page)
	}
	if got := kindsOf(link.Kids); !slices.Equal(got, []string{"mcid", "objr"}) {
		t.Fatalf("Link kids = %v, want [mcid objr]", got)
	}
	if !intPtrIs(link.Kids[0].MCID, 1) || link.Kids[0].Page != 2 {
		t.Errorf("Link mcid kid = %+v, want MCID 1 on page 2", *link.Kids[0])
	}
	if link.Kids[1].ObjType != "Annot/Link" || link.Kids[1].ObjRef == "" {
		t.Errorf("Link objr kid ObjType %q ObjRef %q, want Annot/Link and a ref", link.Kids[1].ObjType, link.Kids[1].ObjRef)
	}

	want := StructureCounts{Elements: 6, MCIDKids: 3, MCRKids: 1, OBJRKids: 1, ErrorKids: 0, ContentMCIDs: 4}
	if r.Counts != want {
		t.Errorf("Counts = %+v, want %+v", r.Counts, want)
	}

	eachKid(r.Elements, func(k *StructKid) {
		if kidHasError(k) {
			t.Errorf("unexpected error on a clean file: kid %+v element %+v", *k, k.Element)
		}
	})
}

func TestGetStructure_CleanFilePagesAndAlignment(t *testing.T) {
	r := getStructure(t, openStructureFixture(t, "structure.pdf"), "s")
	if len(r.Pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(r.Pages))
	}
	h1 := mustElement(t, r, "Heading1")
	fig := mustElement(t, r, "Figure")
	para := mustElement(t, r, "P")
	link := mustElement(t, r, "Link")

	p1 := mustPage(t, r, 1)
	if !intPtrIs(p1.StructParents, 0) || p1.ParentTreeErr != "" || p1.Err != "" {
		t.Errorf("page 1 StructParents %v ParentTreeErr %q Err %q; want 0, no errors", p1.StructParents, p1.ParentTreeErr, p1.Err)
	}
	if len(p1.Streams) != 1 {
		t.Fatalf("page 1 has %d streams, want 1", len(p1.Streams))
	}
	if s := p1.Streams[0]; s.Opens != 2 || s.Closes != 2 || s.DepthAtEnd != 0 || s.MinDepth != 0 || s.Err != "" || s.Ref == "" {
		t.Errorf("page 1 stream = %+v, want 2 opens, 2 closes, depth 0 at end, min depth 0, a ref", s)
	}
	if len(p1.ContentMCIDs) != 2 {
		t.Fatalf("page 1 ContentMCIDs = %+v, want 2", p1.ContentMCIDs)
	}
	for i, want := range []struct {
		mcid int
		tag  string
	}{{0, "Heading1"}, {1, "P"}} {
		c := p1.ContentMCIDs[i]
		if c.MCID != want.mcid || c.Tag != want.tag || c.Depth != 1 || c.ParentMCID != nil || c.Stream != 0 || c.Err != "" {
			t.Errorf("page 1 content MCID %d = %+v, want MCID %d tag %s depth 1 no parent stream 0", i, c, want.mcid, want.tag)
		}
	}
	if got := alignmentMCIDs(p1); !slices.Equal(got, []int{0, 1}) {
		t.Errorf("page 1 alignment MCIDs = %v, want [0 1]", got)
	}
	for mcid, el := range map[int]*StructElem{0: h1, 1: fig} {
		a := alignmentRow(t, p1, mcid)
		if !a.Consistent || a.ContentCount != 1 || a.ParentTreeElement != el.Ref || !slices.Equal(a.TreeElements, []string{el.Ref}) {
			t.Errorf("page 1 MCID %d row = %+v, want consistent and naming %s (%s)", mcid, a, el.Ref, el.S)
		}
	}
	if a := alignmentRow(t, p1, 0); a.ParentTreeS != "Heading1" {
		t.Errorf("page 1 MCID 0 ParentTreeS = %q, want Heading1", a.ParentTreeS)
	}
	if len(p1.ParentTree) != 2 || p1.ParentTree[0].ElementRef != h1.Ref || p1.ParentTree[1].ElementRef != fig.Ref {
		t.Errorf("page 1 ParentTree = %+v, want slots 0 -> %s, 1 -> %s", p1.ParentTree, h1.Ref, fig.Ref)
	}

	p2 := mustPage(t, r, 2)
	if !intPtrIs(p2.StructParents, 1) || p2.ParentTreeErr != "" || p2.Err != "" {
		t.Errorf("page 2 StructParents %v ParentTreeErr %q Err %q; want 1, no errors", p2.StructParents, p2.ParentTreeErr, p2.Err)
	}
	if len(p2.Streams) != 2 {
		t.Fatalf("page 2 has %d streams, want 2", len(p2.Streams))
	}
	var named *ContentMCID
	for i := range p2.ContentMCIDs {
		if p2.ContentMCIDs[i].MCID == 1 {
			named = &p2.ContentMCIDs[i]
		}
	}
	if named == nil || named.Stream != 1 || named.Err != "" {
		t.Errorf("page 2 MCID 1 from /MC0 = %+v, want it recorded from stream 1 with no error", named)
	}
	if got := alignmentMCIDs(p2); !slices.Equal(got, []int{0, 1}) {
		t.Errorf("page 2 alignment MCIDs = %v, want [0 1]", got)
	}
	for mcid, el := range map[int]*StructElem{0: para, 1: link} {
		a := alignmentRow(t, p2, mcid)
		if !a.Consistent || a.ParentTreeElement != el.Ref || !slices.Equal(a.TreeElements, []string{el.Ref}) {
			t.Errorf("page 2 MCID %d row = %+v, want consistent and naming %s (%s)", mcid, a, el.Ref, el.S)
		}
	}

	if want := []ParentTreeEntry{{Key: 2, ElementRef: link.Ref, S: "Link"}}; !slices.Equal(r.OtherParentTree, want) {
		t.Errorf("OtherParentTree = %+v, want %+v", r.OtherParentTree, want)
	}
	if len(r.Problems) != 0 {
		t.Errorf("the two-level ParentTree produced problems: %+v", r.Problems)
	}
}

func TestGetStructure_CleanFileDoesNotFillStreamCache(t *testing.T) {
	ins := openStructureFixture(t, "structure.pdf")
	getStructure(t, ins, "s")
	doc, err := ins.GetDocument("s")
	if err != nil {
		t.Fatal(err)
	}
	doc.streamMu.Lock()
	n := len(doc.streamCache)
	doc.streamMu.Unlock()
	if n != 0 {
		t.Errorf("streamCache holds %d entries after GetStructure, want 0", n)
	}
}

// ---------------------------------------------------------------------------
// Broken fixture: each planted finding in its place
// ---------------------------------------------------------------------------

func TestGetStructure_BrokenFileReportsEachFinding(t *testing.T) {
	r := getStructure(t, openStructureFixture(t, "structure-broken.pdf"), "s")

	t.Run("cycle in a /K array", func(t *testing.T) {
		var cycle *StructKid
		for _, k := range kidsWithError(r) {
			if strings.Contains(k.Err, "already listed") {
				cycle = k
			}
		}
		if cycle == nil {
			t.Fatalf("no \"already listed\" error kid; error kids: %+v", kidsWithError(r))
		}
		seen := map[string]int{}
		eachKid(r.Elements, func(k *StructKid) {
			if k.Element != nil && k.Element.Ref != "" {
				seen[k.Element.Ref]++
			}
		})
		for ref, n := range seen {
			if n != 1 {
				t.Errorf("element %s listed %d times, want once", ref, n)
			}
		}
	})

	t.Run("/Pg naming a non-page", func(t *testing.T) {
		var found bool
		eachKid(r.Elements, func(k *StructKid) {
			if e := k.Element; e != nil && e.PgRef != "" && e.Page == 0 && e.Err != "" {
				found = true
			}
		})
		if !found {
			t.Error("no element with PgRef set, Page 0 and an error")
		}
	})

	t.Run("missing /S", func(t *testing.T) {
		var found bool
		eachKid(r.Elements, func(k *StructKid) {
			if e := k.Element; e != nil && e.S == "" && e.Err != "" {
				found = true
			}
		})
		if !found {
			t.Error("no element with an empty S and an error")
		}
	})

	p1 := mustPage(t, r, 1)
	t.Run("page 1 alignment", func(t *testing.T) {
		if a := alignmentRow(t, p1, 0); a.ContentCount != 2 || a.Consistent {
			t.Errorf("MCID 0 row = %+v, want ContentCount 2 and not consistent", a)
		}
		if a := alignmentRow(t, p1, 2); a.ParentTreeElement != "" || a.Consistent {
			t.Errorf("MCID 2 row = %+v, want no ParentTree element and not consistent", a)
		}
		a := alignmentRow(t, p1, 1)
		if a.ContentCount != 0 || a.ParentTreeElement == "" || slices.Contains(a.TreeElements, a.ParentTreeElement) || a.Consistent {
			t.Errorf("MCID 1 row = %+v, want no content, a ParentTree element that no tree claimant matches, not consistent", a)
		}
	})

	t.Run("page 1 marked-content balance", func(t *testing.T) {
		if len(p1.Streams) != 2 {
			t.Fatalf("page 1 has %d streams, want 2", len(p1.Streams))
		}
		if p1.Streams[0].MinDepth != -1 {
			t.Errorf("first stream MinDepth = %d, want -1 (EMC with nothing open)", p1.Streams[0].MinDepth)
		}
		if p1.Streams[1].DepthAtEnd != 1 {
			t.Errorf("last stream DepthAtEnd = %d, want 1 (BDC with no EMC)", p1.Streams[1].DepthAtEnd)
		}
	})

	t.Run("named property list missing", func(t *testing.T) {
		var found bool
		for _, p := range r.Pages {
			for _, c := range p.ContentMCIDs {
				if c.Err != "" && strings.Contains(c.Err, "MC9") {
					found = true
				}
			}
		}
		if !found {
			t.Error("no content MCID with an error naming /MC9")
		}
	})

	t.Run("page 2 /StructParents names a missing key", func(t *testing.T) {
		p2 := mustPage(t, r, 2)
		if !intPtrIs(p2.StructParents, 5) || !strings.Contains(p2.ParentTreeErr, "5") {
			t.Errorf("page 2 StructParents %v ParentTreeErr %q, want 5 and an error naming key 5", p2.StructParents, p2.ParentTreeErr)
		}
	})

	t.Run("key outside /Limits", func(t *testing.T) {
		var found bool
		for _, p := range r.Problems {
			if strings.Contains(p.Message, "key 1") && strings.Contains(p.Message, "/Limits") {
				found = true
			}
		}
		if !found {
			t.Errorf("no problem for key 1 outside /Limits [0 0]: %+v", r.Problems)
		}
	})

	t.Run("everything else still reported", func(t *testing.T) {
		if len(r.Pages) != 2 {
			t.Errorf("got %d pages, want 2", len(r.Pages))
		}
		if r.Counts.Elements < 3 {
			t.Errorf("Counts.Elements = %d, want the other elements walked", r.Counts.Elements)
		}
		if r.Counts.ErrorKids < 1 {
			t.Errorf("Counts.ErrorKids = %d, want the cycle counted", r.Counts.ErrorKids)
		}
	})
}

// ---------------------------------------------------------------------------
// Untagged and empty roots
// ---------------------------------------------------------------------------

// requireJSONArrays marshals v and requires every named key to be a JSON
// array, never null.
func requireJSONArrays(t *testing.T, where string, v any, keys ...string) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		raw, ok := m[k]
		if !ok {
			t.Errorf("%s: JSON has no %q key", where, k)
			continue
		}
		if !strings.HasPrefix(string(raw), "[") {
			t.Errorf("%s: %q = %s, want an array", where, k, raw)
		}
	}
}

func requireReportArrays(t *testing.T, r *StructureReport) {
	t.Helper()
	requireJSONArrays(t, "report", r, "roleMap", "elements", "pages", "otherParentTree", "problems")
	for _, p := range r.Pages {
		requireJSONArrays(t, fmt.Sprintf("page %d", p.PageNum), p, "parentTree", "streams", "contentMcids", "alignment")
	}
}

func TestGetStructure_UntaggedFile(t *testing.T) {
	r := getStructure(t, openStructureFixture(t, "untagged.pdf"), "s")
	if r.HasStructTree {
		t.Error("HasStructTree = true for a file with no /StructTreeRoot")
	}
	if len(r.Elements) != 0 {
		t.Errorf("Elements = %v, want empty", kindsOf(r.Elements))
	}
	if len(r.Pages) == 0 {
		t.Error("no pages listed")
	}
	requireReportArrays(t, r)
}

func TestGetStructure_EmptyRoot(t *testing.T) {
	r := getStructure(t, openStructureFixture(t, "tagged.pdf"), "s")
	if !r.HasStructTree || r.HasParentTree {
		t.Errorf("HasStructTree %v HasParentTree %v, want true / false", r.HasStructTree, r.HasParentTree)
	}
	if len(r.Elements) != 0 || len(r.Problems) != 0 {
		t.Errorf("Elements %v Problems %+v, want both empty (absent /K and /ParentTree are not problems)", kindsOf(r.Elements), r.Problems)
	}
	requireReportArrays(t, r)
}

func TestGetStructure_UnknownTab(t *testing.T) {
	_, err := NewInspector().GetStructure("missing")
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("err = %v, want ErrDocumentNotFound", err)
	}
}

// ---------------------------------------------------------------------------
// Caps, cycles and shapes pdfcpu rejects at open (in-memory, unvalidated)
// ---------------------------------------------------------------------------

// chainBottom follows the only kid of each element from the top and returns
// the element at the end of the chain and its depth (1 = top).
func chainBottom(t *testing.T, r *StructureReport) (*StructElem, int) {
	t.Helper()
	if len(r.Elements) != 1 || r.Elements[0].Element == nil {
		t.Fatalf("top-level kids = %v, want one element", kindsOf(r.Elements))
	}
	e, depth := r.Elements[0].Element, 1
	for len(e.Kids) == 1 && e.Kids[0].Element != nil {
		e, depth = e.Kids[0].Element, depth+1
	}
	return e, depth
}

func TestGetStructure_ChainPastDepthCapGivesOneErrorKid(t *testing.T) {
	r := rawStructure(t, structChainPDF(257))
	bottom, depth := chainBottom(t, r)
	if depth != 256 {
		t.Errorf("walked %d levels, want 256", depth)
	}
	if len(bottom.Kids) != 1 || bottom.Kids[0].Kind != "error" ||
		!strings.Contains(bottom.Kids[0].Err, "structure tree deeper than 256 levels; subtree not walked") {
		t.Errorf("kids of the 256th element = %+v, want one error kid with the depth message", bottom.Kids)
	}
	if got := len(kidsWithError(r)); got != 1 {
		t.Errorf("got %d error kids, want 1", got)
	}
	if r.Counts.Elements != 256 || r.Counts.ErrorKids != 1 {
		t.Errorf("Counts = %+v, want 256 elements and 1 error kid", r.Counts)
	}
}

func TestGetStructure_ChainAtDepthCapHasNoError(t *testing.T) {
	r := rawStructure(t, structChainPDF(256))
	bottom, depth := chainBottom(t, r)
	if depth != 256 || len(bottom.Kids) != 0 {
		t.Errorf("walked %d levels, bottom kids %+v; want 256 levels and no kid below", depth, bottom.Kids)
	}
	if n := len(kidsWithError(r)); n != 0 || r.Counts.ErrorKids != 0 {
		t.Errorf("got %d error kids (Counts.ErrorKids %d), want 0", n, r.Counts.ErrorKids)
	}
}

func TestGetStructure_SharedChildListedOnce(t *testing.T) {
	r := rawStructure(t, structSharedChildPDF(1000))
	spans, listed := 0, 0
	eachKid(r.Elements, func(k *StructKid) {
		if k.Element != nil && k.Element.S == "Span" {
			spans++
		}
		if k.Kind == "error" && strings.Contains(k.Err, "5000 0 R already listed; not walked again") {
			listed++
		}
	})
	if spans != 1 || listed != 999 {
		t.Errorf("shared Span listed %d times with %d already-listed kids, want 1 and 999", spans, listed)
	}
	if r.Counts.Elements != 1001 || r.Counts.ErrorKids != 999 {
		t.Errorf("Counts = %+v, want 1001 elements and 999 error kids", r.Counts)
	}
}

func TestGetStructure_SharedIndirectKArrayWalkedOnce(t *testing.T) {
	r := rawStructure(t, structTreePDF("<< /Type /StructTreeRoot /K [10 0 R 11 0 R 12 0 R] >>",
		rawObj{7, "[0 1 << /Type /MCR /MCID 2 >>]"},
		rawObj{10, "<< /Type /StructElem /S /P /Pg 3 0 R /K 7 0 R >>"},
		rawObj{11, "<< /Type /StructElem /S /P /Pg 3 0 R /K 7 0 R >>"},
		rawObj{12, "<< /Type /StructElem /S /P /Pg 3 0 R /K 7 0 R >>"},
	))
	if len(r.Elements) != 3 {
		t.Fatalf("root kids = %v, want 3 elements", kindsOf(r.Elements))
	}
	if got := kindsOf(r.Elements[0].Element.Kids); !slices.Equal(got, []string{"mcid", "mcid", "mcr"}) {
		t.Errorf("first element kids = %v, want the shared array walked once", got)
	}
	for _, k := range r.Elements[1:] {
		kids := k.Element.Kids
		if len(kids) != 1 || kids[0].Kind != "error" || !strings.Contains(kids[0].Err, "/K array 7 0 R already listed; not walked again") {
			t.Errorf("element %s kids = %+v, want one error kid naming 7 0 R as already listed", k.Element.Ref, kids)
		}
	}
	if c := r.Counts; c.MCIDKids != 2 || c.MCRKids != 1 || c.ErrorKids != 2 {
		t.Errorf("Counts = %+v, want 2 MCID kids, 1 MCR kid, 2 error kids", c)
	}
	for _, a := range mustPage(t, r, 1).Alignment {
		if !slices.Equal(a.TreeElements, []string{"10 0 R"}) {
			t.Errorf("MCID %d tree elements = %v, want only 10 0 R", a.MCID, a.TreeElements)
		}
	}
}

func TestGetStructure_SingleObjectKBackReference(t *testing.T) {
	r := rawStructure(t, structSingleBackRefPDF())
	errs := kidsWithError(r)
	if len(errs) != 1 || !strings.Contains(errs[0].Err, "element 10 0 R already listed; not walked again") {
		t.Errorf("error kids = %+v, want one naming 10 0 R as already listed", errs)
	}
	if r.Counts.Elements != 2 {
		t.Errorf("Counts.Elements = %d, want 2", r.Counts.Elements)
	}
}

func TestGetStructure_KidShapesRejectedByPdfcpuAreErrorKids(t *testing.T) {
	r := rawStructure(t, structRejectedKidsPDF())
	div := mustElement(t, r, "Div")
	if len(div.Kids) != 9 {
		t.Fatalf("Div kids = %v, want 9 (the direct null skipped)", kindsOf(div.Kids))
	}
	for i, what := range []string{"string", "name", "real", "dangling ref"} {
		if div.Kids[i].Kind != "error" || div.Kids[i].Err == "" {
			t.Errorf("kid %d (%s) = %+v, want an error kid", i, what, *div.Kids[i])
		}
	}
	if !strings.Contains(div.Kids[3].Err, "99 0 R") {
		t.Errorf("dangling kid error %q does not name 99 0 R", div.Kids[3].Err)
	}
	if !kidHasError(div.Kids[4]) {
		t.Errorf("/Type /Foo with no /S kid = %+v, want an error on it", *div.Kids[4])
	}
	if !kidHasError(div.Kids[5]) {
		t.Errorf("MCR with no /MCID kid = %+v, want an error on it", *div.Kids[5])
	}
	if div.Kids[6].Kind != "error" || div.Kids[6].Err == "" {
		t.Errorf("negative MCID kid = %+v, want an error kid", *div.Kids[6])
	}
	if k := div.Kids[7]; k.Kind != "mcid" || !intPtrIs(k.MCID, 0) || k.Page != 1 {
		t.Errorf("kid 7 = %+v, want mcid 0 on page 1 after the bad kids", *k)
	}
	if k := div.Kids[8]; k.Element == nil || k.Element.S != "P" || len(k.Element.Kids) != 1 || !intPtrIs(k.Element.Kids[0].MCID, 1) {
		t.Errorf("kid 8 = %+v, want the P element with MCID 1", *k)
	}
}

func TestGetStructure_RoleMapAndSValues(t *testing.T) {
	r := rawStructure(t, structRoleMapPDF())
	want := []RoleMapEntry{{"A", "B"}, {"B", "A"}, {"Heading1", "H1"}}
	if !slices.Equal(r.RoleMap, want) {
		t.Errorf("RoleMap = %+v, want %+v in key order", r.RoleMap, want)
	}
	if len(r.Elements) != 4 {
		t.Fatalf("top-level kids = %v, want 4 elements", kindsOf(r.Elements))
	}
	if e := r.Elements[0].Element; e == nil || e.S != "A" || e.Err == "" {
		t.Errorf("element with a RoleMap cycle = %+v, want S A and an error", e)
	}
	if e := r.Elements[1].Element; e == nil || e.MappedS != "H1" || e.Err != "" {
		t.Errorf("Heading1 element = %+v, want MappedS H1 and no error", e)
	}
	if e := r.Elements[2].Element; e == nil || e.S != "" || !strings.Contains(e.Err, "/S is an empty name") {
		t.Errorf("empty /S element = %+v, want S \"\" and \"/S is an empty name\"", e)
	}
	if e := r.Elements[3].Element; e == nil || e.S != "" || e.Err == "" || e.Title != "untitled" {
		t.Errorf("element with no /S = %+v, want S \"\", an error and Title untitled", e)
	}
}

func TestGetStructure_DirectElementNeverConsistent(t *testing.T) {
	r := rawStructure(t, structDirectElementPDF())
	if len(r.Elements) != 1 || r.Elements[0].Element == nil || r.Elements[0].Element.Ref != "" {
		t.Fatalf("top-level kids = %+v, want one direct element with no ref", r.Elements)
	}
	a := alignmentRow(t, mustPage(t, r, 1), 0)
	if a.ContentCount != 1 || a.ParentTreeElement != "" || !slices.Equal(a.TreeElements, []string{"(direct)"}) || a.Consistent {
		t.Errorf("MCID 0 row = %+v, want content 1, null slot, tree [(direct)], not consistent", a)
	}
}

// ---------------------------------------------------------------------------
// Locking and close
// ---------------------------------------------------------------------------

func threePagePDF() []byte {
	all := []rawObj{{1, rawCatalog}}
	all = append(all, structPagesObjs(3, []string{
		"/P <</MCID 0>> BDC EMC", "/P <</MCID 0>> BDC EMC", "/P <</MCID 0>> BDC EMC",
	}, nil)...)
	return rawPDF(all...)
}

func TestGetStructure_ConcurrentWithOtherReads(t *testing.T) {
	ins, _ := openUnvalidated(t, threePagePDF())
	var wg sync.WaitGroup
	errs := make(chan error, 30)
	for i := 0; i < 10; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			if _, err := ins.GetStructure("raw"); err != nil {
				errs <- fmt.Errorf("GetStructure: %w", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := ins.GetContentStream("raw", "obj:0:9001"); err != nil {
				errs <- fmt.Errorf("GetContentStream: %w", err)
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := ins.GetPageIndex("raw"); err != nil {
				errs <- fmt.Errorf("GetPageIndex: %w", err)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestGetStructure_CloseDuringTheWalkCancels(t *testing.T) {
	ins, doc := openUnvalidated(t, threePagePDF())
	w := newStructureWalker(doc.PDFContext)
	var calls []int
	w.beforeLock = func(page int) {
		calls = append(calls, page)
		if !doc.pdfMu.TryLock() {
			t.Errorf("pdfMu held when beforeLock(%d) ran", page)
		} else {
			doc.pdfMu.Unlock()
		}
		if page == 2 {
			if err := ins.Close("raw"); err != nil {
				t.Errorf("Close: %v", err)
			}
		}
	}
	_, err := doc.buildStructure(w)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want one satisfying errors.Is(err, context.Canceled)", err)
	}
	if len(calls) == 0 || calls[0] != 0 {
		t.Errorf("beforeLock calls = %v, want 0 first (before phase 1)", calls)
	}
	if slices.Contains(calls, 3) {
		t.Errorf("beforeLock calls = %v, want the walk stopped before page 3", calls)
	}
}

func TestGetStructure_BuildWithoutCloseVisitsEveryPage(t *testing.T) {
	_, doc := openUnvalidated(t, threePagePDF())
	w := newStructureWalker(doc.PDFContext)
	var calls []int
	w.beforeLock = func(page int) { calls = append(calls, page) }
	r, err := doc.buildStructure(w)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(calls, []int{0, 1, 2, 3}) {
		t.Errorf("beforeLock calls = %v, want [0 1 2 3]", calls)
	}
	if len(r.Pages) != 3 || r.HasStructTree {
		t.Errorf("got %d pages, HasStructTree %v; want 3 pages and no tree", len(r.Pages), r.HasStructTree)
	}
	for _, p := range r.Pages {
		if len(p.ContentMCIDs) != 1 || p.ContentMCIDs[0].MCID != 0 {
			t.Errorf("page %d ContentMCIDs = %+v, want MCID 0 with no structure tree", p.PageNum, p.ContentMCIDs)
		}
	}
}

func TestGetStructure_MCIDKidWithNoPageCarriesError(t *testing.T) {
	r := rawStructure(t, structTreePDF("<< /Type /StructTreeRoot /K 10 0 R >>",
		rawObj{10, "<< /Type /StructElem /S /P /K [0 << /Type /MCR /MCID 1 >>] >>"},
	))
	p := mustElement(t, r, "P")
	if len(p.Kids) != 2 {
		t.Fatalf("P kids = %v, want 2", kindsOf(p.Kids))
	}
	for _, k := range p.Kids {
		if k.Page != 0 || !strings.Contains(k.Err, "not attributed to a page") {
			t.Errorf("kid %+v, want page 0 and an error saying the MCID is not attributed to a page", *k)
		}
	}
}

func TestGetStructure_MCIDKidUnderNonPagePgSaysPgIsNotAPage(t *testing.T) {
	r := rawStructure(t, structTreePDF("<< /Type /StructTreeRoot /K [10 0 R] >>",
		rawObj{10, "<< /Type /StructElem /S /P /Pg 11 0 R /K 0 >>"},
		rawObj{11, "<< /Foo 1 >>"},
	))
	p := mustElement(t, r, "P")
	if len(p.Kids) != 1 {
		t.Fatalf("P kids = %v, want 1", kindsOf(p.Kids))
	}
	if k := p.Kids[0]; k.Page != 0 || !strings.Contains(k.Err, "not a page") || !strings.Contains(k.Err, "not attributed to a page") {
		t.Errorf("kid %+v, want page 0 and an error saying the nearest /Pg is not a page", *k)
	}
}

func TestGetStructure_MCIDKidsUnderRootCarryError(t *testing.T) {
	r := rawStructure(t, structTreePDF("<< /Type /StructTreeRoot /K [0 << /Type /MCR /MCID 1 /Pg 3 0 R >>] >>"))
	if len(r.Elements) != 2 {
		t.Fatalf("root kids = %v, want 2", kindsOf(r.Elements))
	}
	for _, k := range r.Elements {
		if !strings.Contains(k.Err, "structure tree root") {
			t.Errorf("kid %+v, want an error saying no element owns the MCID", *k)
		}
	}
	if rows := mustPage(t, r, 1).Alignment; len(rows) != 0 {
		t.Errorf("page 1 alignment = %+v, want none", rows)
	}
}

func TestGetStructure_OBJRToNonDictionaryCarriesError(t *testing.T) {
	r := rawStructure(t, structTreePDF("<< /Type /StructTreeRoot /K 10 0 R >>",
		rawObj{10, "<< /Type /StructElem /S /Link /Pg 3 0 R /K [<< /Type /OBJR /Obj 11 0 R >>] >>"},
		rawObj{11, "42"},
	))
	link := mustElement(t, r, "Link")
	if len(link.Kids) != 1 || link.Kids[0].Kind != "objr" || !strings.Contains(link.Kids[0].Err, "not a dictionary or stream") {
		t.Fatalf("Link kids = %+v, want one objr kid whose error says /Obj is not a dictionary or stream", link.Kids)
	}
}

func TestGetStructure_RoleMapToEmptyNameCarriesError(t *testing.T) {
	r := rawStructure(t, structTreePDF("<< /Type /StructTreeRoot /RoleMap << /Foo / >> /K 10 0 R >>",
		rawObj{10, "<< /Type /StructElem /S /Foo >>"},
	))
	e := mustElement(t, r, "Foo")
	if e.MappedS != "" || !strings.Contains(e.Err, "empty name") {
		t.Errorf("Foo MappedS %q, Err %q; want no mapping and an error naming the empty name", e.MappedS, e.Err)
	}
}

func TestGetStructure_MalformedMarkInfoIsAProblem(t *testing.T) {
	for _, tc := range []struct{ name, markInfo, want string }{
		{"Marked not a boolean", "<< /Marked /true >>", "/Marked is a name"},
		{"MarkInfo not a dictionary", "[true]", "/MarkInfo is an array"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			objs := []rawObj{{1, "<< /Type /Catalog /Pages 2 0 R /MarkInfo " + tc.markInfo + " >>"}}
			r := rawStructure(t, rawPDF(append(objs, structPagesObjs(1, nil, nil)...)...))
			if r.Marked != nil {
				t.Errorf("Marked = %v, want nil", *r.Marked)
			}
			var found bool
			for _, p := range r.Problems {
				found = found || (p.Where == "catalog" && strings.Contains(p.Message, tc.want))
			}
			if !found {
				t.Errorf("Problems = %+v, want a catalog problem containing %q", r.Problems, tc.want)
			}
		})
	}
}

func TestGetStructure_PageTreeWalkStoppedEarlyIsAProblem(t *testing.T) {
	ins, doc := openUnvalidated(t, rawPDF(append([]rawObj{{1, rawCatalog}}, structPagesObjs(2, nil, nil)...)...))
	pw := newPageWalker(doc.PDFContext)
	pw.walk()
	if _, err := doc.pageIndex.get(func() (*pageTree, error) {
		return &pageTree{entries: pw.entries[:1], leaves: pw.leaves[:1], err: errors.New("pdf parsing panic: boom")}, nil
	}); err != nil {
		t.Fatal(err)
	}
	r := getStructure(t, ins, "raw")
	if len(r.Pages) != 1 {
		t.Fatalf("got %d pages, want the 1 page reached before the failure", len(r.Pages))
	}
	if len(r.Problems) != 1 || r.Problems[0].Where != "page tree" || !strings.Contains(r.Problems[0].Message, "boom") {
		t.Errorf("Problems = %+v, want one page tree problem carrying the walk error", r.Problems)
	}
}
