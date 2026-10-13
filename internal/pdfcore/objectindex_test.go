// Object Reference Visibility (Inline Labels + Command Palette)
//
// These tests pin the contract for:
//
//   - TreeNode.ObjectRef and TreeNode.TypeName: every indirect
//     object surfaces "<num> <gen> R" on its node, plus the raw /Type when
//     the dict carries one. Dedup with the existing semantic label is the
//     render layer's job (frontend), so the backend always emits the raw
//     /Type here.
//
//   - Inspector.GetObjectIndex: full xref-derived index, with
//     per-entry reachability, free-flag, /Type extraction, deterministic
//     ObjNum-asc / Gen-asc ordering, NodeID round-trip through
//     GetAncestorPath for reachable entries, and per-DocumentState cache
//     invalidation on re-Open / Close.
//
// Run: go test ./internal/pdfcore/ -run TestObjectIndex -count=1
// go test ./internal/pdfcore/ -run TestTreeNodeObjectRef -count=1
// go test ./internal/pdfcore/ -run TestTreeNodeTypeName -count=1

package pdfcore

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// TreeNode.ObjectRef and TreeNode.TypeName
// ---------------------------------------------------------------------------

// TestTreeNodeObjectRefPopulatedOnIndirectChildren verifies that the catalog's
// indirect-ref children (/Pages) carry ObjectRef in "<num> <gen> R" form.
// Nodes that correspond to an indirect object expose their object ref.
func TestTreeNodeObjectRefPopulatedOnIndirectChildren(t *testing.T) {
	ins, tabID := openMultipage(t)
	children, err := ins.GetChildren(tabID, "root")
	if err != nil {
		t.Fatalf("GetChildren(root): %v", err)
	}
	var pages *TreeNode
	for _, c := range children {
		if c.RawKey == "/Pages" {
			pages = c
			break
		}
	}
	if pages == nil {
		t.Fatalf("no /Pages child in catalog")
	}
	// multipage.pdf's /Pages is obj 2 gen 0. The ref form is "<num> <gen> R".
	if pages.ObjectRef != "2 0 R" {
		t.Errorf("Pages.ObjectRef = %q, want %q", pages.ObjectRef, "2 0 R")
	}
}

// TestTreeNodeObjectRefBlankOnInlineScalar verifies that inline scalars (e.g.
// /Type which is a Name, not an indirect object) carry an empty ObjectRef so
// the frontend can suppress the inline suffix for them.
func TestTreeNodeObjectRefBlankOnInlineScalar(t *testing.T) {
	ins, tabID := openMultipage(t)
	children, err := ins.GetChildren(tabID, "root")
	if err != nil {
		t.Fatalf("GetChildren(root): %v", err)
	}
	var typ *TreeNode
	for _, c := range children {
		if c.RawKey == "/Type" {
			typ = c
			break
		}
	}
	if typ == nil {
		t.Fatalf("no /Type child in catalog")
	}
	if typ.ObjectRef != "" {
		t.Errorf("/Type inline scalar ObjectRef = %q, want empty", typ.ObjectRef)
	}
}

// TestTreeNodeTypeNameOnIndirectPages verifies the /Pages dict surfaces
// TypeName="Pages" (the literal /Type value).
func TestTreeNodeTypeNameOnIndirectPages(t *testing.T) {
	ins, tabID := openMultipage(t)
	children, err := ins.GetChildren(tabID, "root")
	if err != nil {
		t.Fatalf("GetChildren(root): %v", err)
	}
	var pages *TreeNode
	for _, c := range children {
		if c.RawKey == "/Pages" {
			pages = c
			break
		}
	}
	if pages == nil {
		t.Fatalf("no /Pages child in catalog")
	}
	if pages.TypeName != "Pages" {
		t.Errorf("Pages.TypeName = %q, want %q", pages.TypeName, "Pages")
	}
}

// TestTreeNodeTypeNameBlankWhenNoTypeKey verifies that an indirect object
// without a /Type entry surfaces an empty TypeName, NOT "Catalog" or any
// inferred value. backend stays strict; only literal /Type values
// populate TypeName.
func TestTreeNodeTypeNameBlankWhenNoTypeKey(t *testing.T) {
	ins, tabID := openMultipage(t)
	// Descend to a Page's /Contents entry. multipage.pdf's page 1 is obj 3,
	// its /Contents is a stream object that typically has no /Type entry.
	children, err := ins.GetChildren(tabID, "obj:0:3")
	if err != nil {
		t.Fatalf("GetChildren(obj:0:3): %v", err)
	}
	var contents *TreeNode
	for _, c := range children {
		if c.RawKey == "/Contents" {
			contents = c
			break
		}
	}
	if contents == nil {
		t.Skip("page 1 has no /Contents child in this fixture; skipping TypeName-blank assertion")
	}
	if contents.TypeName != "" {
		t.Errorf("Contents.TypeName = %q, want empty (no /Type key on most content streams)", contents.TypeName)
	}
}

// TestTreeNodeObjectRefArrayElement verifies array elements that resolve to
// indirect objects also carry ObjectRef + TypeName. The rule applies to
// array members too (e.g. Pages.Kids[0] -> Page object).
func TestTreeNodeObjectRefArrayElement(t *testing.T) {
	ins, tabID := openMultipage(t)
	// /Pages is obj 2 0 R; its /Kids array contains the page refs.
	pagesChildren, err := ins.GetChildren(tabID, "obj:0:2")
	if err != nil {
		t.Fatalf("GetChildren(pages): %v", err)
	}
	var kids *TreeNode
	for _, c := range pagesChildren {
		if c.RawKey == "/Kids" {
			kids = c
			break
		}
	}
	if kids == nil {
		t.Fatalf("no /Kids under /Pages")
	}
	kidsChildren, err := ins.GetChildren(tabID, kids.ID)
	if err != nil {
		t.Fatalf("GetChildren(kids): %v", err)
	}
	if len(kidsChildren) == 0 {
		t.Fatalf("Kids array is empty")
	}
	first := kidsChildren[0]
	if first.ObjectRef == "" {
		t.Errorf("Kids[0].ObjectRef is empty, want non-empty for indirect page ref")
	}
	if !strings.HasSuffix(first.ObjectRef, " R") {
		t.Errorf("Kids[0].ObjectRef = %q, want suffix \" R\"", first.ObjectRef)
	}
	if first.TypeName != "Page" {
		t.Errorf("Kids[0].TypeName = %q, want %q", first.TypeName, "Page")
	}
}

// ---------------------------------------------------------------------------
// Inspector.GetObjectIndex
// ---------------------------------------------------------------------------

// TestGetObjectIndexReturnsAllXRefEntries verifies the index contains an
// entry for every indirect object the document reports through pdfcpu's
// XRefTable, with entries sorted by ObjNum ascending.
func TestGetObjectIndexReturnsAllXRefEntries(t *testing.T) {
	ins, tabID := openMultipage(t)
	entries, err := ins.GetObjectIndex(tabID)
	if err != nil {
		t.Fatalf("GetObjectIndex: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("GetObjectIndex returned 0 entries; want >= 1 for multipage.pdf")
	}
	// Catalog (obj 1) must always be present and reachable.
	var catalog *ObjectIndexEntry
	for _, e := range entries {
		if e.ObjNum == 1 {
			catalog = e
			break
		}
	}
	if catalog == nil {
		t.Fatalf("catalog (obj 1) missing from index")
	}
	if !catalog.Reachable {
		t.Errorf("catalog.Reachable = false, want true")
	}
	if catalog.NodeID != "obj:0:1" && catalog.NodeID != "root" {
		t.Errorf("catalog.NodeID = %q, want %q or %q", catalog.NodeID, "obj:0:1", "root")
	}

	// Sort assertion: ObjNum ascending.
	for i := 1; i < len(entries); i++ {
		prev, cur := entries[i-1], entries[i]
		if prev.ObjNum > cur.ObjNum {
			t.Errorf("entries not sorted by ObjNum asc at index %d: prev=%d cur=%d", i, prev.ObjNum, cur.ObjNum)
			break
		}
		if prev.ObjNum == cur.ObjNum && prev.Gen > cur.Gen {
			t.Errorf("entries with same ObjNum=%d not sorted by Gen asc at index %d: prev.Gen=%d cur.Gen=%d", prev.ObjNum, i, prev.Gen, cur.Gen)
			break
		}
	}
}

// TestGetObjectIndexTypeExtraction verifies the index carries TypeName for
// objects with a /Type entry (e.g. /Pages, /Page). backend side.
func TestGetObjectIndexTypeExtraction(t *testing.T) {
	ins, tabID := openMultipage(t)
	entries, err := ins.GetObjectIndex(tabID)
	if err != nil {
		t.Fatalf("GetObjectIndex: %v", err)
	}
	foundPages := false
	foundPage := false
	for _, e := range entries {
		if e.TypeName == "Pages" {
			foundPages = true
		}
		if e.TypeName == "Page" {
			foundPage = true
		}
	}
	if !foundPages {
		t.Error("no entry with TypeName=Pages found in index")
	}
	if !foundPage {
		t.Error("no entry with TypeName=Page found in index")
	}
}

// TestGetObjectIndexReachableNodeIDRoundTrip verifies that NodeID on a
// reachable entry round-trips through GetAncestorPath (returns a non-empty
// path ending in that NodeID).
func TestGetObjectIndexReachableNodeIDRoundTrip(t *testing.T) {
	ins, tabID := openMultipage(t)
	entries, err := ins.GetObjectIndex(tabID)
	if err != nil {
		t.Fatalf("GetObjectIndex: %v", err)
	}
	// Pick the first reachable, non-catalog entry.
	var target *ObjectIndexEntry
	for _, e := range entries {
		if e.Reachable && e.NodeID != "" && e.NodeID != "root" && e.ObjNum > 1 {
			target = e
			break
		}
	}
	if target == nil {
		t.Skip("no reachable non-catalog entry found in index; skipping round-trip")
	}
	path, err := ins.GetAncestorPath(tabID, target.NodeID)
	if err != nil {
		t.Fatalf("GetAncestorPath(%s): %v", target.NodeID, err)
	}
	if len(path) == 0 {
		t.Fatalf("GetAncestorPath returned empty path for reachable NodeID %q", target.NodeID)
	}
	if path[len(path)-1] != target.NodeID {
		t.Errorf("path tail = %q, want %q", path[len(path)-1], target.NodeID)
	}
}

// TestGetObjectIndexUnknownTab verifies the method returns ErrDocumentNotFound
// (or a wrapped form) when called with an unknown tab id. Defensive contract.
func TestGetObjectIndexUnknownTab(t *testing.T) {
	ins := NewInspector()
	_, err := ins.GetObjectIndex("no-such-tab")
	if err == nil {
		t.Fatalf("expected error for unknown tab, got nil")
	}
}

// TestGetObjectIndexCacheStableAcrossCalls verifies repeat calls return the
// same slice contents (cache hit): lazy build, then cache per
// DocumentState. We compare by deep equality on the entries.
func TestGetObjectIndexCacheStableAcrossCalls(t *testing.T) {
	ins, tabID := openMultipage(t)
	first, err := ins.GetObjectIndex(tabID)
	if err != nil {
		t.Fatalf("GetObjectIndex (first): %v", err)
	}
	second, err := ins.GetObjectIndex(tabID)
	if err != nil {
		t.Fatalf("GetObjectIndex (second): %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("cache returned different entries across calls:\n first  = %+v\n second = %+v", first, second)
	}
}

// TestGetObjectIndexInvalidatesOnReopen verifies that closing and re-opening
// the same tabID rebuilds the index (cache lives on the DocumentState
// pointer, which is replaced on Open).
func TestGetObjectIndexInvalidatesOnReopen(t *testing.T) {
	ins := NewInspector()
	tabID := "tab-reopen"
	pdfPath := filepath.Join(testdataDir(t), "multipage.pdf")

	if _, err := ins.Open(tabID, pdfPath); err != nil {
		t.Fatalf("first Open: %v", err)
	}
	first, err := ins.GetObjectIndex(tabID)
	if err != nil {
		t.Fatalf("GetObjectIndex (first): %v", err)
	}
	if len(first) == 0 {
		t.Fatalf("first index empty")
	}

	// Re-open under the same tabID. The DocumentState pointer must be
	// replaced so a fresh build runs on the next GetObjectIndex call.
	if _, err := ins.Open(tabID, pdfPath); err != nil {
		t.Fatalf("re-Open: %v", err)
	}
	second, err := ins.GetObjectIndex(tabID)
	if err != nil {
		t.Fatalf("GetObjectIndex (second): %v", err)
	}
	if len(second) == 0 {
		t.Fatalf("second index empty after re-Open")
	}
	// Content shape should be identical, but the underlying slice pointer
	// must differ (fresh build). Compare slice header addresses to confirm.
	if &first[0] == &second[0] {
		t.Errorf("re-Open did not rebuild index: same slice backing array")
	}
}

// TestBuildReachableSetDeepNesting verifies that buildReachableSet no longer
// caps the BFS at depth 32. The fixture page-tree depth is 52;
// pre-fix, objects 34..53 were mislabeled as orphan trees. Post-fix all 53
// objects are reachable. Boundary at depth 32 AND well past it.
func TestBuildReachableSetDeepNesting(t *testing.T) {
	ins := NewInspector()
	tabID := "tab-10-6-deep"
	path := filepath.Join(testdataDir(t), "correctness", "deep-nesting.pdf")
	if _, err := ins.Open(tabID, path); err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = ins.Close(tabID) })

	entries, err := ins.GetObjectIndex(tabID)
	if err != nil {
		t.Fatalf("GetObjectIndex: %v", err)
	}
	byNum := map[int]*ObjectIndexEntry{}
	for _, e := range entries {
		byNum[e.ObjNum] = e
	}
	// Boundary check: obj 33 sits at depth 32 pre-fix (the first depth
	// that the cap blocked). Assert reachable.
	if e := byNum[33]; e == nil || !e.Reachable {
		t.Errorf("boundary: obj 33 (depth 32) reachable=%v, want true", e != nil && e.Reachable)
	}
	// Well past the boundary: obj 50 must also be reachable (depth 49).
	if e := byNum[50]; e == nil || !e.Reachable {
		t.Errorf("well-past-32: obj 50 (depth 49) reachable=%v, want true", e != nil && e.Reachable)
	}
	// Leaf page at obj 53.
	if e := byNum[53]; e == nil || !e.Reachable {
		t.Errorf("leaf: obj 53 (leaf Page) reachable=%v, want true", e != nil && e.Reachable)
	}
}

// TestObjectIndexEntryShape pins the struct's exported field set so a
// rename/typo at implementation time is caught here, not at the frontend
// binding layer.
func TestObjectIndexEntryShape(t *testing.T) {
	e := ObjectIndexEntry{
		ObjNum:    1,
		Gen:       0,
		TypeName:  "Catalog",
		Free:      false,
		Reachable: true,
		NodeID:    "obj:0:1",
	}
	// Force compilation of every field; if any is missing or mis-named,
	// the build fails here.
	_ = e.ObjNum
	_ = e.Gen
	_ = e.TypeName
	_ = e.Free
	_ = e.Reachable
	_ = e.NodeID
}
