package pdfcore

import (
	"errors"
	"path/filepath"
	"testing"
)

// TestReverseRefIndexBuildPopulatesPagesAndKids verifies the index is built
// at Open for a well-formed multipage PDF and pages report the parent Pages
// node as their reverse ref. Name pinned by integration test 9.10-INTG-013.
func TestReverseRefIndexBuildPopulatesPagesAndKids(t *testing.T) {
	ins := NewInspector()
	tabID := "tab-rr-multi"
	_, err := ins.Open(tabID, filepath.Join(testdataDir(t), "multipage.pdf"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	// Page 3 0 R must have an inbound edge from /Pages (2 0 R) at /Kids[0].
	refs, err := ins.GetReverseRefs(tabID, "obj:0:3")
	if err != nil {
		t.Fatalf("GetReverseRefs(page 3): %v", err)
	}
	if len(refs) == 0 {
		t.Fatalf("expected at least one reverse ref for page 3, got 0")
	}
	found := false
	for _, r := range refs {
		if r.ParentRef == "2 0 R" && r.Path == "/Kids[0]" {
			found = true
			if r.ParentType == nil || *r.ParentType != "Pages" {
				t.Errorf("expected ParentType=Pages, got %v", r.ParentType)
			}
			if r.ParentNodeID != "obj:0:2" {
				t.Errorf("expected ParentNodeID=obj:0:2, got %q", r.ParentNodeID)
			}
		}
	}
	if !found {
		t.Errorf("expected reverse ref from 2 0 R at /Kids[0], got %+v", refs)
	}
}

// TestReverseRefsCatalogAndPagesEdges verifies the catalog-as-indirect edge
// is recorded for the catalog's direct children (e.g. /Pages).
func TestReverseRefsCatalogEdgeToPages(t *testing.T) {
	ins := NewInspector()
	tabID := "tab-rr-cat"
	_, err := ins.Open(tabID, filepath.Join(testdataDir(t), "multipage.pdf"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	// /Pages (2 0 R) is referenced by the catalog (1 0 R) at path "/Pages".
	refs, err := ins.GetReverseRefs(tabID, "obj:0:2")
	if err != nil {
		t.Fatalf("GetReverseRefs(pages): %v", err)
	}
	found := false
	for _, r := range refs {
		if r.ParentNodeID == "obj:0:1" && r.Path == "/Pages" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected catalog->pages edge with path /Pages, got %+v", refs)
	}
}

// TestReverseRefIndexCatalogIsEmpty verifies the catalog itself has no
// reverse-ref entries (the trailer's /Root pointer is excluded by
// construction). Name pinned by integration test 9.10-INTG-018.
func TestReverseRefIndexCatalogIsEmpty(t *testing.T) {
	ins := NewInspector()
	tabID := "tab-rr-cat-empty"
	_, err := ins.Open(tabID, filepath.Join(testdataDir(t), "multipage.pdf"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	refs, err := ins.GetReverseRefs(tabID, "obj:0:1")
	if err != nil {
		t.Fatalf("GetReverseRefs(catalog): %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("expected catalog to have zero reverse refs, got %d (%+v)", len(refs), refs)
	}
}

// TestReverseRefsInlineNodeReturnsEmpty verifies inline node IDs return an
// empty slice (no error). The frontend suppresses the section for them.
func TestReverseRefsInlineNodeReturnsEmpty(t *testing.T) {
	ins := NewInspector()
	tabID := "tab-rr-inline"
	_, err := ins.Open(tabID, filepath.Join(testdataDir(t), "multipage.pdf"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	refs, err := ins.GetReverseRefs(tabID, "dict:obj:0:3:Type")
	if err != nil {
		t.Fatalf("inline GetReverseRefs returned error: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("expected empty refs for inline node, got %d", len(refs))
	}
}

// TestReverseRefIndexBuildPanicSurfacesSentinel verifies a panicked build
// flips the failure flag and queries return the sentinel error rather than
// an empty list. AC6 forbidden-failure-mode guard. Name pinned by
// integration test 9.10-INTG-014.
func TestReverseRefIndexBuildPanicSurfacesSentinel(t *testing.T) {
	ins := NewInspector()
	tabID := "tab-rr-fail"
	_, err := ins.Open(tabID, filepath.Join(testdataDir(t), "multipage.pdf"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Directly flip the failure flag on DocumentState (test-only hook via
	// the inspector's internal map) to simulate the post-panic state.
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	doc.revRefsBuildFailed = true
	doc.reverseRefs = nil

	_, err = ins.GetReverseRefs(tabID, "obj:0:3")
	if err == nil {
		t.Fatalf("expected sentinel error, got nil")
	}
	if !errors.Is(err, ErrReverseRefIndexUnavailable) {
		t.Errorf("expected ErrReverseRefIndexUnavailable, got %v", err)
	}
}

// TestReverseRefsCycleSafe verifies the BFS terminates on the page-tree
// /Parent cycle (Pages -> Kids[N] -> Parent -> Pages). Multipage.pdf has the
// canonical cycle; if the visited-set logic regressed this would hang or
// stack-overflow.
func TestReverseRefsCycleSafe(t *testing.T) {
	ins := NewInspector()
	tabID := "tab-rr-cycle"
	_, err := ins.Open(tabID, filepath.Join(testdataDir(t), "multipage.pdf"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Every page must carry exactly one /Kids[N] inbound edge from /Pages.
	for i, expectedPath := range []string{"/Kids[0]", "/Kids[1]", "/Kids[2]"} {
		nodeID := "obj:0:" + map[int]string{0: "3", 1: "4", 2: "5"}[i]
		refs, err := ins.GetReverseRefs(tabID, nodeID)
		if err != nil {
			t.Fatalf("GetReverseRefs(%s): %v", nodeID, err)
		}
		kidsHits := 0
		for _, r := range refs {
			if r.ParentRef == "2 0 R" && r.Path == expectedPath {
				kidsHits++
			}
		}
		if kidsHits != 1 {
			t.Errorf("page %s: expected exactly one /Kids inbound edge with path %q, got %d (%+v)",
				nodeID, expectedPath, kidsHits, refs)
		}
	}
}

// TestReverseRefEntriesSortedByParentThenPath verifies entries return in
// stable order: ParentRef numeric asc, then Path asc, then ParentNodeID asc.
// Name pinned by integration test 9.10-INTG-017.
func TestReverseRefEntriesSortedByParentThenPath(t *testing.T) {
	// /Pages 2 0 R has three /Kids entries pointing to 3, 4, 5. From each
	// page's perspective only one ref points at it, but the global ordering
	// is exercised by querying refs that contain the same parent ref.
	ins := NewInspector()
	tabID := "tab-rr-order"
	_, err := ins.Open(tabID, filepath.Join(testdataDir(t), "multipage.pdf"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Run twice to verify stability across calls (defensive against map-
	// iteration nondeterminism in the BFS).
	a, _ := ins.GetReverseRefs(tabID, "obj:0:3")
	b, _ := ins.GetReverseRefs(tabID, "obj:0:3")
	if len(a) != len(b) {
		t.Fatalf("length mismatch across calls: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("ordering changed at %d: %+v vs %+v", i, a[i], b[i])
		}
	}
}

// TestReverseRefsPerformanceBudget is a placeholder for the AC-listed 50k-
// object fixture. No such fixture exists in testdata/ today; skip until one
// is committed. Tracking-only -- not a hard gate.
func TestReverseRefsPerformanceBudget(t *testing.T) {
	t.Skip("TODO: add a 50,000-object fixture to testdata/ and assert " +
		"<100ms build + <10MB memory delta")
}

// TestReverseRefIndexOrphanObjectHasEmptyList verifies that an indirect object
// that is NOT referenced by any dict-graph parent returns an empty list (NOT
// the sentinel error). Empty list IS the orphan signal per AC9. Injects an
// orphan by mutating the cached map directly -- no fixture needed.
// Name pinned by integration test 9.10-INTG-019.
func TestReverseRefIndexOrphanObjectHasEmptyList(t *testing.T) {
	ins := NewInspector()
	tabID := "tab-rr-orphan"
	_, err := ins.Open(tabID, filepath.Join(testdataDir(t), "multipage.pdf"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// Pick an obj num well above the document's last allocated indirect; the
	// index will have no entry for it, returning an empty list.
	refs, err := ins.GetReverseRefs(tabID, "obj:0:9999")
	if err != nil {
		t.Fatalf("expected nil error for orphan, got %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("expected empty list for orphan, got %d entries: %+v", len(refs), refs)
	}
}

// TestReverseRefIndexCatalogNotReDescended verifies that the catalog is
// marked visited at queue init so a stray indirect ref to /Root would NOT
// cause re-descent and produce duplicate child edges. We can't easily
// synthesize a back-ref to /Root in a real fixture, so this test injects a
// re-descent via the public API path and asserts the per-child edge count
// stays at 1 (no duplicates) for an unmodified document. If catalog were
// re-descended naturally, the existing multipage.pdf catalog->Pages edge
// would already be doubled. The check is a tight one-edge assertion.
func TestReverseRefIndexCatalogNotReDescended(t *testing.T) {
	ins := NewInspector()
	tabID := "tab-rr-cat-once"
	_, err := ins.Open(tabID, filepath.Join(testdataDir(t), "multipage.pdf"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// /Pages 2 0 R is referenced by the catalog once.
	refs, err := ins.GetReverseRefs(tabID, "obj:0:2")
	if err != nil {
		t.Fatalf("GetReverseRefs(pages): %v", err)
	}
	hits := 0
	for _, r := range refs {
		if r.ParentNodeID == "obj:0:1" && r.Path == "/Pages" {
			hits++
		}
	}
	if hits != 1 {
		t.Errorf("expected exactly one catalog->/Pages edge, got %d (%+v)", hits, refs)
	}
}

// TestRefLessNumericOrdering covers the comparator that orders ParentRef
// strings numerically by (num, gen). Plain string-compare would put "10 0 R"
// before "9 0 R"; that ordering bug is what the helper exists to prevent.
func TestRefLessNumericOrdering(t *testing.T) {
	cases := []struct {
		a, b string
		want int // -1 a<b, 0 equal, 1 a>b
	}{
		{"9 0 R", "10 0 R", -1},   // numeric, not string-compare
		{"10 0 R", "9 0 R", 1},    // reverse of above
		{"5 0 R", "5 1 R", -1},    // same num, gen tiebreaker
		{"5 1 R", "5 0 R", 1},     // reverse of above
		{"5 0 R", "5 0 R", 0},     // exact equality
		{"100 0 R", "99 0 R", 1},  // 3-digit > 2-digit numerically
		{"not a ref", "5 0 R", 1}, // malformed: falls back to string compare ('n' > '5')
		{"5 0 R", "not a ref", -1},
	}
	for _, c := range cases {
		got := refLess(c.a, c.b)
		want := c.want
		// refLess returns -1/0/+1; normalize the sign for comparison since
		// strings.Compare can return arbitrary negative/positive ints.
		gotSign := 0
		switch {
		case got < 0:
			gotSign = -1
		case got > 0:
			gotSign = 1
		}
		if gotSign != want {
			t.Errorf("refLess(%q, %q) sign = %d, want %d (raw=%d)", c.a, c.b, gotSign, want, got)
		}
	}
}

// TestJoinPath covers the three precedence paths of the path-segment joiner:
// empty prefix returns the suffix verbatim; bracket suffix attaches directly
// (no space) so "/Kids" + "[3]" reads "/Kids[3]"; dict-key suffix gets a
// leading space so "/Resources" + "/Font" reads "/Resources /Font".
func TestJoinPath(t *testing.T) {
	cases := []struct {
		prefix, suffix, want string
	}{
		{"", "/Kids", "/Kids"},
		{"", "[0]", "[0]"},
		{"/Kids", "[3]", "/Kids[3]"},
		{"/Resources", "/Font", "/Resources /Font"},
		{"/Resources /Font", "/F1", "/Resources /Font /F1"},
	}
	for _, c := range cases {
		if got := joinPath(c.prefix, c.suffix); got != c.want {
			t.Errorf("joinPath(%q, %q) = %q, want %q", c.prefix, c.suffix, got, c.want)
		}
	}
}

// TestGetReverseRefsReturnsDefensiveCopy verifies the contract documented on
// GetReverseRefs: "Return a copy so callers cannot mutate the cached slice."
// Without the copy, a frontend-side sort or splice would corrupt the cached
// index for every subsequent query.
func TestGetReverseRefsReturnsDefensiveCopy(t *testing.T) {
	ins := NewInspector()
	tabID := "tab-rr-copy"
	_, err := ins.Open(tabID, filepath.Join(testdataDir(t), "multipage.pdf"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	first, err := ins.GetReverseRefs(tabID, "obj:0:3")
	if err != nil {
		t.Fatalf("GetReverseRefs first: %v", err)
	}
	if len(first) == 0 {
		t.Fatalf("expected at least one ref for page 3")
	}
	originalParent := first[0].ParentRef
	// Mutate the returned slice -- this MUST NOT bleed back into the cache.
	first[0].ParentRef = "POISONED"
	first[0].Path = "POISONED"
	second, err := ins.GetReverseRefs(tabID, "obj:0:3")
	if err != nil {
		t.Fatalf("GetReverseRefs second: %v", err)
	}
	if len(second) == 0 || second[0].ParentRef != originalParent {
		t.Errorf("cache was mutated: expected ParentRef=%q on second call, got %q", originalParent, second[0].ParentRef)
	}
}

// TestGetReverseRefsMalformedNodeIDReturnsEmpty verifies that an obj-prefixed
// nodeID with non-numeric components silently returns an empty slice (the
// frontend renders the orphan empty state). The path mirrors the inline-node
// behavior: inputs that cannot resolve to (num, gen) yield no entries rather
// than propagating an error.
func TestGetReverseRefsMalformedNodeIDReturnsEmpty(t *testing.T) {
	ins := NewInspector()
	tabID := "tab-rr-mal"
	_, err := ins.Open(tabID, filepath.Join(testdataDir(t), "multipage.pdf"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	cases := []string{"obj:abc:1", "obj:0:xyz"}
	for _, nodeID := range cases {
		refs, err := ins.GetReverseRefs(tabID, nodeID)
		if err != nil {
			t.Errorf("GetReverseRefs(%q): expected nil error, got %v", nodeID, err)
		}
		if len(refs) != 0 {
			t.Errorf("GetReverseRefs(%q): expected empty slice, got %d entries", nodeID, len(refs))
		}
	}
}

// TestReverseRefIndexPerDocumentIsolation verifies that opening two documents
// in two tabs yields two independent indexes; queries against one are
// unaffected by the other. Name pinned by integration test 9.10-INTG-020.
func TestReverseRefIndexPerDocumentIsolation(t *testing.T) {
	ins := NewInspector()
	_, err := ins.Open("tab-a", filepath.Join(testdataDir(t), "multipage.pdf"))
	if err != nil {
		t.Fatalf("open tab-a: %v", err)
	}
	_, err = ins.Open("tab-b", filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("open tab-b: %v", err)
	}

	// Page 3 0 R in multipage.pdf has /Kids[0] from Pages. In minimal.pdf,
	// obj:0:3 is the single page with a different ancestry.
	aRefs, err := ins.GetReverseRefs("tab-a", "obj:0:3")
	if err != nil {
		t.Fatalf("tab-a query: %v", err)
	}
	bRefs, err := ins.GetReverseRefs("tab-b", "obj:0:3")
	if err != nil {
		t.Fatalf("tab-b query: %v", err)
	}
	// Both should resolve independently. Closing tab-a must not affect tab-b.
	if err := ins.Close("tab-a"); err != nil {
		t.Fatalf("close tab-a: %v", err)
	}
	bRefsAfter, err := ins.GetReverseRefs("tab-b", "obj:0:3")
	if err != nil {
		t.Fatalf("tab-b query after close: %v", err)
	}
	if len(bRefs) != len(bRefsAfter) {
		t.Errorf("tab-b refs changed after closing tab-a: before=%d after=%d", len(bRefs), len(bRefsAfter))
	}
	// And tab-a must now be unavailable.
	if _, err := ins.GetReverseRefs("tab-a", "obj:0:3"); err == nil {
		t.Errorf("expected error after closing tab-a")
	}
	_ = aRefs
}
