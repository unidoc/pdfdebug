// Story 11-5 acceptance test for the keystone primitive ResolveRef.
//
// AC3: pdfcore.ResolveRef(tabID, nodeID, {MaxDepth}) is a bounded,
// cycle-guarded resolver. These assertions pin the cycle/depth/MaxDepth-0
// guards and the ResolvedNode JSON shape contract for Story 11-6 and the GUI.
//
// Run: cd code && go test -run TestResolveRef ./internal/pdfcore/...

package pdfcore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	pdfcpu_api "github.com/pdfcpu/pdfcpu/pkg/api"
)

// writeTempPDF writes raw PDF bytes to a temp file, verifies pdfcpu can read
// it, opens it in a fresh Inspector, and returns the inspector + tabID. The
// file and document are cleaned up via t.Cleanup.
func writeTempPDF(t *testing.T, name string, content []byte) (*Inspector, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write temp pdf: %v", err)
	}
	// Sanity: pdfcpu must accept the fixture, else the test is moot.
	if _, err := pdfcpu_api.ReadContextFile(path); err != nil {
		t.Fatalf("fixture %s rejected by pdfcpu: %v", name, err)
	}
	ins := NewInspector()
	tabID := "atdd-11-5"
	if _, err := ins.Open(tabID, path); err != nil {
		t.Fatalf("open temp pdf: %v", err)
	}
	t.Cleanup(func() { _ = ins.Close(tabID) })
	return ins, tabID
}

// cyclePDF builds a PDF whose object graph contains a deliberate A->B->A cycle
// reachable from the catalog. Object 4 (A) has /Next 5 0 R, object 5 (B) has
// /Next 4 0 R. The page tree is minimal and valid so pdfcpu accepts the file.
func cyclePDF() []byte {
	pdf := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R /CycleRoot 4 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n"
	obj4 := "4 0 obj\n<< /Name (A) /Next 5 0 R >>\nendobj\n\n"
	obj5 := "5 0 obj\n<< /Name (B) /Next 4 0 R >>\nendobj\n\n"
	return assemblexref(pdf, obj1, obj2, obj3, obj4, obj5)
}

// selfRefPDF builds a PDF with a self-referential object (A -> A): object 4
// has /Self 4 0 R.
func selfRefPDF() []byte {
	pdf := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R /SelfRoot 4 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n"
	obj4 := "4 0 obj\n<< /Name (A) /Self 4 0 R >>\nendobj\n\n"
	return assemblexref(pdf, obj1, obj2, obj3, obj4)
}

// linearChainPDF builds a PDF with a known-deep linear ref chain
// 4 -> 5 -> 6 -> 7 -> 8 (each /Next points at the following object, the last
// has no /Next). Used to assert Truncated at the depth cap and full
// resolution below it.
func linearChainPDF() []byte {
	pdf := "%PDF-1.4\n"
	obj1 := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R /ChainRoot 4 0 R >>\nendobj\n\n"
	obj2 := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n"
	obj3 := "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n"
	obj4 := "4 0 obj\n<< /Depth 0 /Next 5 0 R >>\nendobj\n\n"
	obj5 := "5 0 obj\n<< /Depth 1 /Next 6 0 R >>\nendobj\n\n"
	obj6 := "6 0 obj\n<< /Depth 2 /Next 7 0 R >>\nendobj\n\n"
	obj7 := "7 0 obj\n<< /Depth 3 /Next 8 0 R >>\nendobj\n\n"
	obj8 := "8 0 obj\n<< /Depth 4 >>\nendobj\n\n"
	return assemblexref(pdf, obj1, obj2, obj3, obj4, obj5, obj6, obj7, obj8)
}

// assemblexref concatenates the PDF header + object bodies and appends a
// correct classic xref table + trailer (Root = 1 0 R). objs[0] is the header.
func assemblexref(parts ...string) []byte {
	header := parts[0]
	objs := parts[1:]
	body := header
	offsets := make([]int, len(objs))
	cur := len(header)
	for i, o := range objs {
		offsets[i] = cur
		body += o
		cur += len(o)
	}
	xrefOffset := len(body)
	xref := "xref\n0 " + itoa(len(objs)+1) + "\n0000000000 65535 f \n"
	for _, off := range offsets {
		xref += pad10(off) + " 00000 n \n"
	}
	trailer := "trailer\n<< /Size " + itoa(len(objs)+1) + " /Root 1 0 R >>\nstartxref\n" + itoa(xrefOffset) + "\n%%EOF\n"
	return []byte(body + xref + trailer)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func pad10(n int) string {
	s := itoa(n)
	for len(s) < 10 {
		s = "0" + s
	}
	return s
}

// ---------------------------------------------------------------------------
// 11.5-UNIT-AC3-001 [P0]: A->B->A cycle terminates and marks the back-edge
// Cyclic rather than looping or stack-overflowing.
// ---------------------------------------------------------------------------

func TestResolveRef_CycleTerminatesAndMarksCyclic(t *testing.T) {
	ins, tabID := writeTempPDF(t, "cycle.pdf", cyclePDF())

	// MaxDepth high enough to traverse the whole graph if there were no guard;
	// the visited set (objNum:gen) must break A->B->A.
	node, err := ins.ResolveRef(tabID, "obj:0:4", ResolveOpts{MaxDepth: 16})
	if err != nil {
		t.Fatalf("ResolveRef returned error: %v", err)
	}
	if node == nil {
		t.Fatal("ResolveRef returned nil for obj 4")
	}

	// Some node in the resolved graph must carry Cyclic=true (the back-edge that
	// re-enters object 4). Walk children and assert at least one Cyclic marker.
	if !anyCyclic(node) {
		t.Errorf("expected a Cyclic marker on the A->B->A back-edge; found none in resolved graph")
	}
}

// ---------------------------------------------------------------------------
// 11.5-UNIT-AC3-002 [P0]: A->A self-reference terminates and marks Cyclic.
// ---------------------------------------------------------------------------

func TestResolveRef_SelfReferenceTerminates(t *testing.T) {
	ins, tabID := writeTempPDF(t, "selfref.pdf", selfRefPDF())

	node, err := ins.ResolveRef(tabID, "obj:0:4", ResolveOpts{MaxDepth: 16})
	if err != nil {
		t.Fatalf("ResolveRef returned error: %v", err)
	}
	if node == nil {
		t.Fatal("ResolveRef returned nil for self-ref obj 4")
	}
	if !anyCyclic(node) {
		t.Errorf("expected a Cyclic marker on the A->A self-edge; found none")
	}
}

// ---------------------------------------------------------------------------
// 11.5-UNIT-AC3-003 [P0]: A known-deep linear chain is resolved fully below
// the cap and marked Truncated at the cap.
// ---------------------------------------------------------------------------

func TestResolveRef_DeepChainTruncatedAtCap(t *testing.T) {
	ins, tabID := writeTempPDF(t, "chain.pdf", linearChainPDF())

	// MaxDepth=2 should follow obj4 -> obj5 -> obj6, then leave obj6's /Next
	// (to obj7) unresolved and marked Truncated.
	node, err := ins.ResolveRef(tabID, "obj:0:4", ResolveOpts{MaxDepth: 2})
	if err != nil {
		t.Fatalf("ResolveRef returned error: %v", err)
	}
	if node == nil {
		t.Fatal("ResolveRef returned nil")
	}

	depth := chainDepthResolved(node)
	if depth < 2 {
		t.Errorf("chain resolved to depth %d, want at least 2 below the cap", depth)
	}
	if !anyTruncated(node) {
		t.Errorf("expected a Truncated marker at the MaxDepth=2 cap; found none")
	}
}

// ---------------------------------------------------------------------------
// 11.5-UNIT-AC3-004 [P0]: MaxDepth=0 resolves the addressed object only and
// leaves its child refs UNFOLLOWED (no resolved children for indirect refs).
// ---------------------------------------------------------------------------

func TestResolveRef_MaxDepthZeroLeavesChildRefsUnfollowed(t *testing.T) {
	ins, tabID := writeTempPDF(t, "chain0.pdf", linearChainPDF())

	node, err := ins.ResolveRef(tabID, "obj:0:4", ResolveOpts{MaxDepth: 0})
	if err != nil {
		t.Fatalf("ResolveRef returned error: %v", err)
	}
	if node == nil {
		t.Fatal("ResolveRef returned nil")
	}

	// At MaxDepth=0 the /Next ref inside object 4 must NOT have been followed:
	// no resolved child carrying object 5's ObjectRef should appear.
	if chainDepthResolved(node) != 0 {
		t.Errorf("MaxDepth=0 followed a child ref (resolved depth > 0); child refs must stay unfollowed")
	}
}

// ---------------------------------------------------------------------------
// 11.5-UNIT-AC3-005 [P1]: Negative MaxDepth is rejected (error) OR clamped to
// 0 (resolve addressed object only, no children followed). The story leaves
// the choice to Dev; this test accepts either contract but FORBIDS following
// child refs on a negative input.
// ---------------------------------------------------------------------------

func TestResolveRef_NegativeMaxDepthRejectedOrClamped(t *testing.T) {
	ins, tabID := writeTempPDF(t, "chainneg.pdf", linearChainPDF())

	node, err := ins.ResolveRef(tabID, "obj:0:4", ResolveOpts{MaxDepth: -1})
	if err != nil {
		// Rejected: acceptable contract.
		return
	}
	// Clamped-to-0 contract: must behave exactly like MaxDepth=0.
	if node == nil {
		t.Fatal("negative MaxDepth neither errored nor returned a node")
	}
	if chainDepthResolved(node) != 0 {
		t.Errorf("negative MaxDepth was neither rejected nor clamped to 0 (resolved child refs)")
	}
}

// ---------------------------------------------------------------------------
// 11.5-UNIT-AC3-006 [P1]: ResolvedNode JSON shape is a stable contract for
// 11-6 and the GUI: it must marshal and expose objectRef, cyclic, and
// truncated keys (the load-bearing AC3 markers).
// ---------------------------------------------------------------------------

func TestResolveRef_JSONShapeContract(t *testing.T) {
	ins, tabID := writeTempPDF(t, "chainjson.pdf", linearChainPDF())

	node, err := ins.ResolveRef(tabID, "obj:0:4", ResolveOpts{MaxDepth: 1})
	if err != nil {
		t.Fatalf("ResolveRef error: %v", err)
	}
	b, err := json.Marshal(node)
	if err != nil {
		t.Fatalf("ResolvedNode does not marshal to JSON: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("ResolvedNode JSON is not an object: %v\nraw: %s", err, b)
	}
	for _, key := range []string{"objectRef", "cyclic", "truncated"} {
		if _, ok := m[key]; !ok {
			t.Errorf("ResolvedNode JSON missing contract key %q (keys present: %v)", key, keysOf(m))
		}
	}
}

// --- resolved-graph walkers (operate on the dedicated ResolvedNode type) ----

// anyCyclic reports whether any node in the resolved graph is flagged Cyclic.
func anyCyclic(n *ResolvedNode) bool {
	if n == nil {
		return false
	}
	if n.Cyclic {
		return true
	}
	for _, c := range n.Children {
		if anyCyclic(c) {
			return true
		}
	}
	return false
}

// anyTruncated reports whether any node in the resolved graph is flagged
// Truncated (depth-cap hit).
func anyTruncated(n *ResolvedNode) bool {
	if n == nil {
		return false
	}
	if n.Truncated {
		return true
	}
	for _, c := range n.Children {
		if anyTruncated(c) {
			return true
		}
	}
	return false
}

// chainDepthResolved counts how many levels of the /Next ref chain were
// actually followed (resolved children that themselves came from an indirect
// ref). It is the longest resolved-child path length from n.
func chainDepthResolved(n *ResolvedNode) int {
	if n == nil || len(n.Children) == 0 {
		return 0
	}
	best := 0
	for _, c := range n.Children {
		// Only count children that were themselves resolved from an indirect
		// ref (carry an ObjectRef) and are not mere unfollowed ref markers.
		if c != nil && c.ObjectRef != "" && !c.Truncated && !c.Cyclic {
			if d := 1 + chainDepthResolved(c); d > best {
				best = d
			}
		}
	}
	return best
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
