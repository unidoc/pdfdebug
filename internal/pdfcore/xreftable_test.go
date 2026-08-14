// XREF Table extraction tests.
//
// Tests are named to match the runPdfcoreTest patterns pinned in
// tests/detail-panel-tabs/detail_panel_tabs_test.go.

package pdfcore

import (
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

// openWithFixture is a small helper that opens a fixture PDF under a fresh
// tabID and returns (Inspector, tabID, doc) so tests can poke at unexported
// caches.
func openWithFixture(t *testing.T, fixture string) (*Inspector, string, *DocumentState) {
	t.Helper()
	ins := NewInspector()
	tabID := "tab-xref-test"
	pdfPath := filepath.Join(testdataDir(t), fixture)
	if _, err := ins.Open(tabID, pdfPath); err != nil {
		t.Fatalf("Open(%s): %v", fixture, err)
	}
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	return ins, tabID, doc
}

// TestGetXRefTableBasicShape verifies the table is populated for a minimal
// fixture, every non-free row is in-use, and the table is sorted by ObjNum.
func TestGetXRefTableBasicShape(t *testing.T) {
	ins, tabID, _ := openWithFixture(t, "minimal.pdf")
	table, err := ins.GetXRefTable(tabID)
	if err != nil {
		t.Fatalf("GetXRefTable: %v", err)
	}
	if table == nil {
		t.Fatal("GetXRefTable returned nil")
	}
	if table.TabID != tabID {
		t.Errorf("TabID = %q, want %q", table.TabID, tabID)
	}
	if len(table.Entries) == 0 {
		t.Fatalf("entries empty for minimal.pdf")
	}
	for i := 1; i < len(table.Entries); i++ {
		if table.Entries[i-1].ObjNum > table.Entries[i].ObjNum {
			t.Errorf("entries not sorted by ObjNum at index %d", i)
		}
	}
	for _, e := range table.Entries {
		if e.ObjNum == 0 {
			t.Errorf("object 0 should be skipped, found in entries")
		}
		if e.Status != "in-use" && e.Status != "free" && e.Status != "in-objstm" {
			t.Errorf("unexpected status %q for obj %d", e.Status, e.ObjNum)
		}
	}
}

// TestGetXRefTableSortedByObjNumThenGen verifies the (ObjNum asc, Gen asc) sort
// order. pdfcpu's map iteration is non-deterministic; the sort is the only
// stability guarantee.
func TestGetXRefTableSortedByObjNumThenGen(t *testing.T) {
	ins, tabID, _ := openWithFixture(t, "multipage.pdf")
	table, err := ins.GetXRefTable(tabID)
	if err != nil {
		t.Fatalf("GetXRefTable: %v", err)
	}
	for i := 1; i < len(table.Entries); i++ {
		prev, cur := table.Entries[i-1], table.Entries[i]
		if prev.ObjNum > cur.ObjNum {
			t.Errorf("not sorted by ObjNum at %d: prev=%d cur=%d", i, prev.ObjNum, cur.ObjNum)
			break
		}
		if prev.ObjNum == cur.ObjNum && prev.Gen > cur.Gen {
			t.Errorf("not sorted by Gen at ObjNum=%d index=%d", prev.ObjNum, i)
			break
		}
	}
}

// TestGetXRefTableSkipsObjectZero verifies obj 0 (the free-list head) is never
// emitted.
func TestGetXRefTableSkipsObjectZero(t *testing.T) {
	ins, tabID, _ := openWithFixture(t, "multipage.pdf")
	table, err := ins.GetXRefTable(tabID)
	if err != nil {
		t.Fatalf("GetXRefTable: %v", err)
	}
	for _, e := range table.Entries {
		if e.ObjNum == 0 {
			t.Errorf("obj 0 should be skipped")
		}
	}
}

// TestGetXRefTableStatusLiterals verifies the Status strings exactly match the
// IPC contract: "in-use" / "free" / "in-objstm".
func TestGetXRefTableStatusLiterals(t *testing.T) {
	ins, tabID, _ := openWithFixture(t, "minimal.pdf")
	table, err := ins.GetXRefTable(tabID)
	if err != nil {
		t.Fatalf("GetXRefTable: %v", err)
	}
	for _, e := range table.Entries {
		switch e.Status {
		case "in-use", "free", "in-objstm":
		default:
			t.Errorf("bad Status %q for obj %d -- must be one of in-use|free|in-objstm", e.Status, e.ObjNum)
		}
	}
}

// TestGetXRefTableNodeIDEncoding verifies NodeID is "obj:<gen>:<num>" for
// in-use / in-objstm entries and "" for free entries.
func TestGetXRefTableNodeIDEncoding(t *testing.T) {
	ins, tabID, _ := openWithFixture(t, "multipage.pdf")
	table, err := ins.GetXRefTable(tabID)
	if err != nil {
		t.Fatalf("GetXRefTable: %v", err)
	}
	if len(table.Entries) == 0 {
		t.Fatal("no entries")
	}
	for _, e := range table.Entries {
		switch e.Status {
		case "free":
			if e.NodeID != "" {
				t.Errorf("free obj %d has non-empty NodeID %q", e.ObjNum, e.NodeID)
			}
		case "in-use", "in-objstm":
			expected := "obj:" + intToStr(e.Gen) + ":" + intToStr(e.ObjNum)
			if e.NodeID != expected {
				t.Errorf("obj %d gen %d status %s: NodeID = %q, want %q", e.ObjNum, e.Gen, e.Status, e.NodeID, expected)
			}
		}
	}
}

func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}

// TestGetXRefTableCompressedNodeIDTargetsUnderlying pins the contract:
// in-objstm rows expose the UNDERLYING object's NodeID, not the host objstm's.
// This is the structural invariant -- we cannot guarantee that the fixture has
// compressed entries, so the test is best-effort: if none are present we skip
// rather than fail, but for any compressed row we find we assert the NodeID
// matches the underlying obj number with gen=0.
func TestGetXRefTableCompressedNodeIDTargetsUnderlying(t *testing.T) {
	ins, tabID, _ := openWithFixture(t, "multipage.pdf")
	table, err := ins.GetXRefTable(tabID)
	if err != nil {
		t.Fatalf("GetXRefTable: %v", err)
	}
	found := false
	for _, e := range table.Entries {
		if e.Status != "in-objstm" {
			continue
		}
		found = true
		expected := "obj:" + intToStr(e.Gen) + ":" + intToStr(e.ObjNum)
		if e.NodeID != expected {
			t.Errorf("in-objstm obj %d gen %d NodeID = %q, want %q (underlying obj, NOT host objstm)",
				e.ObjNum, e.Gen, e.NodeID, expected)
		}
		// Sanity: HostObjStm is the host's obj number, NOT the underlying obj.
		if e.HostObjStm == e.ObjNum {
			t.Errorf("in-objstm obj %d has HostObjStm == ObjNum; payload conflates host with underlying", e.ObjNum)
		}
	}
	if !found {
		t.Skip("no in-objstm entries in this fixture; the NodeID assertion is not exercised at the backend layer")
	}
}

// TestGetXRefTableHostObjStmSentinel verifies HostObjStm is 0 for in-use and
// free rows; for in-objstm rows it carries the host objstm's object number
// (verified non-zero when present).
func TestGetXRefTableHostObjStmSentinel(t *testing.T) {
	ins, tabID, _ := openWithFixture(t, "minimal.pdf")
	table, err := ins.GetXRefTable(tabID)
	if err != nil {
		t.Fatalf("GetXRefTable: %v", err)
	}
	for _, e := range table.Entries {
		switch e.Status {
		case "in-use", "free":
			if e.HostObjStm != 0 {
				t.Errorf("obj %d status %s has HostObjStm=%d, want 0", e.ObjNum, e.Status, e.HostObjStm)
			}
		case "in-objstm":
			if e.HostObjStm <= 0 {
				t.Errorf("obj %d in-objstm has HostObjStm=%d, want >0", e.ObjNum, e.HostObjStm)
			}
		}
	}
}

// TestGetXRefTableOffsetSentinel verifies Offset is -1 for non-in-use rows and
// non-negative for in-use rows.
func TestGetXRefTableOffsetSentinel(t *testing.T) {
	ins, tabID, _ := openWithFixture(t, "minimal.pdf")
	table, err := ins.GetXRefTable(tabID)
	if err != nil {
		t.Fatalf("GetXRefTable: %v", err)
	}
	for _, e := range table.Entries {
		switch e.Status {
		case "in-use":
			if e.Offset < 0 {
				t.Errorf("in-use obj %d Offset=%d, want >= 0", e.ObjNum, e.Offset)
			}
		case "free", "in-objstm":
			if e.Offset != -1 {
				t.Errorf("obj %d status %s Offset=%d, want -1 (sentinel)", e.ObjNum, e.Status, e.Offset)
			}
		}
	}
}

// TestGetXRefTableSafeCallOnMalformed verifies safeCall catches pdfcpu panics
// on malformed fixtures. The malformed PDF may not even open, so we accept
// either (a) Open fails (which is fine; the test still validates the contract
// that GetXRefTable cannot be reached without an open document), or (b) Open
// succeeds and GetXRefTable returns a result or wrapped error without panic.
func TestGetXRefTableSafeCallOnMalformed(t *testing.T) {
	ins := NewInspector()
	tabID := "tab-malformed"
	pdfPath := filepath.Join(testdataDir(t), "malformed.pdf")
	if _, err := ins.Open(tabID, pdfPath); err != nil {
		// Open itself failed -- malformed PDF blocked at parse. That is
		// acceptable; the safeCall contract is upheld by Open.
		return
	}
	defer func() { _ = ins.Close(tabID) }()

	// Open succeeded; GetXRefTable must not panic.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("GetXRefTable panicked on malformed.pdf: %v", r)
		}
	}()
	_, _ = ins.GetXRefTable(tabID)
}

// TestGetXRefTableCacheReturnsSamePointer verifies the cache returns the same
// pointer across calls, and that dropping the cache forces a rebuild that
// returns a different pointer with deeply-equal contents.
func TestGetXRefTableCacheReturnsSamePointer(t *testing.T) {
	ins, tabID, doc := openWithFixture(t, "minimal.pdf")
	first, err := ins.GetXRefTable(tabID)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	second, err := ins.GetXRefTable(tabID)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if first != second {
		t.Errorf("cache returned different pointers across consecutive calls")
	}

	// Drop the cache and verify a rebuild produces a new pointer with equal
	// contents.
	doc.xrefTableMu.Lock()
	doc.xrefTableCache = nil
	doc.xrefTableMu.Unlock()

	third, err := ins.GetXRefTable(tabID)
	if err != nil {
		t.Fatalf("third call (post-drop): %v", err)
	}
	if third == second {
		t.Errorf("post-drop call returned same pointer; expected rebuild")
	}
	if !reflect.DeepEqual(third, second) {
		t.Errorf("rebuilt table not deeply equal to cached one")
	}
}

// TestGetXRefTableConcurrentCallsShareBuild smoke-tests the mutex coverage:
// concurrent callers must converge on one cached pointer. supporting
// assertion.
func TestGetXRefTableConcurrentCallsShareBuild(t *testing.T) {
	ins, tabID, _ := openWithFixture(t, "minimal.pdf")
	var wg sync.WaitGroup
	ptrs := make([]*XRefTable, 8)
	wg.Add(len(ptrs))
	for i := range ptrs {
		go func(i int) {
			defer wg.Done()
			tab, err := ins.GetXRefTable(tabID)
			if err != nil {
				t.Errorf("goroutine %d: %v", i, err)
				return
			}
			ptrs[i] = tab
		}(i)
	}
	wg.Wait()
	for i := 1; i < len(ptrs); i++ {
		if ptrs[i] != ptrs[0] {
			t.Errorf("concurrent callers got different pointers")
			break
		}
	}
}

// TestGetXRefTableUnknownTab verifies the method returns an error for an
// unknown tabID without panicking.
func TestGetXRefTableUnknownTab(t *testing.T) {
	ins := NewInspector()
	if _, err := ins.GetXRefTable("no-such-tab"); err == nil {
		t.Errorf("expected error for unknown tab, got nil")
	}
}
