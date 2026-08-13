// Story 13-6 RED-PHASE unit tests for the path-aligned structural diff
// engine.
//
// These are co-located pdfcore UNIT tests (the project's precedent for keystone
// pdfcore logic; see resolve_ref_atdd_test.go for 11-5). They assert the
// EXPECTED post-implementation behavior of the NEW diff engine that Task 1.2
// will land in internal/pdfcore/diff.go:
//
//	func (ins *Inspector) DiffDocuments(leftTabID, rightTabID string) (*DiffResult, error)
//
//	type DiffResult struct { Root *DiffNode; Summary DiffSummary }
//	type DiffNode struct {
//	    Path         string      // structural path, catalog-rooted (e.g. "/Root/Pages")
//	    Status       string      // "added" | "removed" | "changed" | "unchanged"
//	    Kind         string      // "dict" | "array" | "stream" | "scalar" | "ref"
//	    ChangedKeys  []string    // for a changed dict: keys added/removed/modified
//	    LeftSummary  string      // short repr of the left value ("" for added)
//	    RightSummary string      // short repr of the right value ("" for removed)
//	    Children     []*DiffNode // recursive, modeled on ResolvedNode (NOT the flat TreeNode)
//	}
//	type DiffSummary struct {
//	    Added, Removed, Changed          int
//	    PageCountLeft, PageCountRight    int
//	    VersionChanged                   bool  // catalog /Version
//	    EncryptionChanged                bool  // read from DocumentState/trailer, NOT the catalog walk
//	    InfoChanged                      bool  // /Info dictionary
//	    XMPChanged                       bool  // XMP metadata packet
//	}
//
// RED: diff.go does not exist yet, so this file (and the rest of the pdfcore
// test package) fails to COMPILE until Task 1.2 lands the engine. That compile
// failure IS the red state; the Dev step turns it green. `go build ./...`
// (the CLI/app build) is unaffected -- _test.go files are excluded from builds.
//
// Naming: [Px] per the story Testing Requirements.
// Run: cd code && go test -run TestDiff ./internal/pdfcore/...
//
// Status literals are asserted as strings (not exported const names) to keep the
// contract on OBSERVABLE behavior rather than internal identifiers.

package pdfcore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pdfcpu_api "github.com/pdfcpu/pdfcpu/pkg/api"
)

// --- diff fixture assembler (custom /Root so numbering can be permuted) -------

// assembleDiffPDF stitches a 1.7 header, object bodies (object i+1 at position
// i), an xref table, and a trailer whose /Root points at rootNum. Reuses the
// package-level itoa/pad10 (resolve_ref_atdd_test.go).
func assembleDiffPDF(rootNum int, objs ...string) []byte {
	header := "%PDF-1.7\n"
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
	trailer := "trailer\n<< /Size " + itoa(len(objs)+1) + " /Root " + itoa(rootNum) +
		" 0 R >>\nstartxref\n" + itoa(xrefOffset) + "\n%%EOF\n"
	return []byte(body + xref + trailer)
}

// diffOnePage is the baseline: catalog(1) -> pages(2) -> page(3), natural order.
func diffOnePage() []byte {
	return assembleDiffPDF(1,
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
	)
}

// diffOnePageRenumbered is STRUCTURALLY IDENTICAL to diffOnePage but with the
// object numbers permuted: page(1), pages(2), catalog(3), /Root 3 0 R. This is
// the ALIGNMENT GUARDRAIL fixture -- a path-aligned diff must report ZERO delta;
// an object-number-aligned diff would (wrongly) report everything changed.
func diffOnePageRenumbered() []byte {
	return assembleDiffPDF(3,
		"1 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [1 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
	)
}

// diffThreeLevelNatural is a THREE-level page tree (catalog -> PagesRoot ->
// PagesMid -> PagesLeaf -> Page) in natural numbering. The intermediate Pages
// nodes carry a direct /Parent indirect ref, which is the shape that exposes
// object-number leakage at back-edge cut points.
func diffThreeLevelNatural() []byte {
	return assembleDiffPDF(1,
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Pages /Parent 2 0 R /Kids [4 0 R] /Count 1 >>\nendobj\n",
		"4 0 obj\n<< /Type /Pages /Parent 3 0 R /Kids [5 0 R] /Count 1 >>\nendobj\n",
		"5 0 obj\n<< /Type /Page /Parent 4 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
	)
}

// diffThreeLevelRenumbered is STRUCTURALLY IDENTICAL to diffThreeLevelNatural
// with the object numbers permuted asymmetrically so the /Parent refs on the
// intermediate Pages nodes carry DIFFERENT object numbers than the natural
// version. A path-aligned diff must report ZERO delta; a diff that embeds object
// numbers in its cut-point value summaries would wrongly flag the /Parent
// back-edge targets as changed.
func diffThreeLevelRenumbered() []byte {
	return assembleDiffPDF(5,
		"1 0 obj\n<< /Type /Pages /Parent 4 0 R /Kids [2 0 R] /Count 1 >>\nendobj\n", // PagesMid
		"2 0 obj\n<< /Type /Pages /Parent 1 0 R /Kids [3 0 R] /Count 1 >>\nendobj\n", // PagesLeaf
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n", // Page
		"4 0 obj\n<< /Type /Pages /Kids [1 0 R] /Count 1 >>\nendobj\n",              // PagesRoot
		"5 0 obj\n<< /Type /Catalog /Pages 4 0 R >>\nendobj\n",                      // Catalog
	)
}

// diffAddedMetadata is diffOnePage with an extra /Metadata 4 0 R on the catalog
// and a new object 4 -> one object ADDED, reachable at /Root/Metadata. Object 4
// is a real /Metadata XML stream (pdfcpu eagerly dereferences /Metadata as a
// stream at read time, so a plain dict there makes the whole file unparseable).
func diffAddedMetadata() []byte {
	return assembleDiffPDF(1,
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Metadata 4 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		"4 0 obj\n<< /Type /Metadata /Subtype /XML /Length 4 >>\nstream\n<x/>\nendstream\nendobj\n",
	)
}

// diffChangedMediaBox is diffOnePage with the page /MediaBox modified in place
// (A4 instead of US-Letter) -> one CHANGED dict, changed key "MediaBox".
func diffChangedMediaBox() []byte {
	return assembleDiffPDF(1,
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] >>\nendobj\n",
	)
}

// diffChangedVersion is diffOnePage with a catalog /Version override.
func diffChangedVersion() []byte {
	return assembleDiffPDF(1,
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Version /1.5 >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
	)
}

// diffTwoPage is a two-page document (page count 2) for the summary page-count
// delta assertion.
func diffTwoPage() []byte {
	return assembleDiffPDF(1,
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		"4 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
	)
}

// diffRetargetGoTo is diffOnePage with a catalog /OpenAction indirect ref (obj
// 4) pointing at a /GoTo action. The RIGHT side of the retarget pair.
func diffRetargetGoTo() []byte {
	return assembleDiffPDF(1,
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /OpenAction 4 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		"4 0 obj\n<< /Type /Action /S /GoTo >>\nendobj\n",
	)
}

// diffRetargetNamed is STRUCTURALLY renumbered relative to diffRetargetGoTo AND
// its catalog /OpenAction indirect ref is RETARGETED to a DIFFERENT action
// object (/S /Named with an extra /N key), which also carries a different object
// number. A path-aligned diff must report the change by the TARGET content
// (/OpenAction: /S /GoTo -> /Named, /N added), NEVER as a raw object-number
// change - the object numbers differ on BOTH the action object AND the ref, yet
// only the genuine content retarget must surface.
func diffRetargetNamed() []byte {
	return assembleDiffPDF(3,
		"1 0 obj\n<< /Type /Action /S /Named /N /NextPage >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [4 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Catalog /Pages 2 0 R /OpenAction 1 0 R >>\nendobj\n",
		"4 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
	)
}

// --- two-document opener ------------------------------------------------------

// openTwoForDiff writes both fixtures, verifies pdfcpu accepts each (guards an
// eternally-red suite), opens BOTH into ONE Inspector under tabs "left"/"right"
// (the diff is a read-only walk over two DocumentStates in one Inspector), and
// returns the inspector + the two tab ids.
func openTwoForDiff(t *testing.T, leftName string, left []byte, rightName string, right []byte) (*Inspector, string, string) {
	t.Helper()
	dir := t.TempDir()
	lp := filepath.Join(dir, leftName)
	rp := filepath.Join(dir, rightName)
	if err := os.WriteFile(lp, left, 0o644); err != nil {
		t.Fatalf("write left fixture: %v", err)
	}
	if err := os.WriteFile(rp, right, 0o644); err != nil {
		t.Fatalf("write right fixture: %v", err)
	}
	if _, err := pdfcpu_api.ReadContextFile(lp); err != nil {
		t.Fatalf("left fixture %s rejected by pdfcpu: %v", leftName, err)
	}
	if _, err := pdfcpu_api.ReadContextFile(rp); err != nil {
		t.Fatalf("right fixture %s rejected by pdfcpu: %v", rightName, err)
	}
	ins := NewInspector()
	if _, err := ins.Open("left", lp); err != nil {
		t.Fatalf("open left: %v", err)
	}
	if _, err := ins.Open("right", rp); err != nil {
		t.Fatalf("open right: %v", err)
	}
	t.Cleanup(func() {
		_ = ins.Close("left")
		_ = ins.Close("right")
	})
	return ins, "left", "right"
}

// collectDiffNodes flattens the DiffNode tree in pre-order for whole-tree
// assertions that do not want to couple to the exact Path string format.
func collectDiffNodes(root *DiffNode) []*DiffNode {
	if root == nil {
		return nil
	}
	out := []*DiffNode{root}
	for _, c := range root.Children {
		out = append(out, collectDiffNodes(c)...)
	}
	return out
}

// containsStr reports whether ss contains s.
func containsStr(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// diffDeepChain builds a single-page PDF with a LINEAR chain of nested dict
// refs hanging off the catalog's /Deep key: obj4 -> obj5 -> ... each
// << /L (n+1) 0 R >>, terminating in a << /V value >> leaf. With chainLen well
// above maxResolveDepth (32) the diff's depth cap cuts the chain before it
// reaches the leaf, so a change in the leaf's /V is HIDDEN behind the shallow
// summary at the cut (Story 14.3 #2). Mirrors testdata/correctness/
// deep-change-{a,b}.pdf without touching disk.
func diffDeepChain(chainLen int, leafValue string) []byte {
	objs := []string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Deep 4 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
	}
	base := 4 // first chain object number
	for i := 0; i < chainLen; i++ {
		num := base + i
		objs = append(objs, fmt.Sprintf("%d 0 obj\n<< /L %d 0 R >>\nendobj\n", num, num+1))
	}
	objs = append(objs, fmt.Sprintf("%d 0 obj\n<< /V %s >>\nendobj\n", base+chainLen, leafValue))
	return assembleDiffPDF(1, objs...)
}

// diffSharedDeepAndShallow builds a single-page PDF where ONE object is
// reachable two ways: directly off the catalog's /Shallow key (depth 1, fully
// walked) AND at the bottom of a linear /Deep chain long enough that the ref to
// it lands past maxResolveDepth (depth-capped). It is the topology that made the
// depth-cap over-count TruncatedSubtrees before reconcileTruncation: the shared
// pair is fully accounted for on the shallow path, so the capped encounter hides
// nothing. Catalog keys sort alphabetically, so /Deep is walked BEFORE /Shallow
// (deep-first order) -- the case a mere check-reorder would not fix.
func diffSharedDeepAndShallow(chainLen int, sharedValue string) []byte {
	shared := 4 + chainLen // shared object number, immediately after the chain
	objs := []string{
		fmt.Sprintf("1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Shallow %d 0 R /Deep 4 0 R >>\nendobj\n", shared),
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
	}
	base := 4
	for i := 0; i < chainLen; i++ {
		num := base + i
		// The last chain object (num == shared-1) points its /L at the shared obj.
		objs = append(objs, fmt.Sprintf("%d 0 obj\n<< /L %d 0 R >>\nendobj\n", num, num+1))
	}
	objs = append(objs, fmt.Sprintf("%d 0 obj\n<< /V %s >>\nendobj\n", shared, sharedValue))
	return assembleDiffPDF(1, objs...)
}

// ---------------------------------------------------------------------------
// Regression (Story 14.3 code review): a shared object reachable both shallow
// (fully walked) and past the depth cap must NOT be counted as a truncated
// subtree. Before reconcileTruncation the capped encounter over-counted
// TruncatedSubtrees, flipping an IDENTICAL pair to a false "not identical" /
// exit 1 -- the same false-positive class the spec's adversarial review
// flagged. Exercises the deep-first DFS order.
// ---------------------------------------------------------------------------

func TestDiff_SharedRefWalkedElsewhereNotTruncated(t *testing.T) {
	// Chain length == maxResolveDepth lands the shared ref EXACTLY at the first
	// cap (diffChild depth 32), so the shared object is the only capped node and
	// no deep-only intermediate is cut. Identical inputs: the sole "truncation"
	// is that shared object seen past the cap on the /Deep path, which is fully
	// walked on the /Shallow path.
	pdf := diffSharedDeepAndShallow(maxResolveDepth, "111")
	ins, l, r := openTwoForDiff(t, "sharedA.pdf", pdf, "sharedB.pdf", pdf)

	res, err := ins.DiffDocuments(l, r)
	if err != nil {
		t.Fatalf("DiffDocuments returned error: %v", err)
	}
	if res.Summary.TruncatedSubtrees != 0 {
		t.Errorf("TruncatedSubtrees = %d, want 0 (the shared object is fully walked on the shallow path)", res.Summary.TruncatedSubtrees)
	}
	if res.Summary.Added != 0 || res.Summary.Removed != 0 || res.Summary.Changed != 0 {
		t.Errorf("identical inputs reported deltas Added=%d Removed=%d Changed=%d, want all 0", res.Summary.Added, res.Summary.Removed, res.Summary.Changed)
	}
	// No node may still carry the Truncated mark after reconciliation.
	for _, n := range collectDiffNodes(res.Root) {
		if n.Truncated {
			t.Errorf("node %q still marked Truncated after reconciliation", n.Path)
		}
	}
}

// TestReconcileTruncation_EntryNotCompletion pins the honesty guarantee behind
// the entry-populated enteredPairs set (PR review, diff.go:reconcileTruncation):
// enteredPairs records a pair on diff ENTRY, not on completion of a full walk,
// so a pair entered near the depth cap whose own subtree is then cut is in the
// set exactly like a fully-walked pair. Clearing a redundant past-the-cap
// encounter of such a pair must NOT zero TruncatedSubtrees, because the deeper,
// genuinely-unresolved pair retains its own mark. Asserted directly on
// reconcileTruncation + countDelta so it does not depend on a brittle
// depth-33 PDF fixture.
func TestReconcileTruncation_EntryNotCompletion(t *testing.T) {
	// childP: a redundant past-cap encounter of pair "P" that WAS entered
	// elsewhere -> in enteredPairs -> its mark clears.
	// childQ: pair "Q" that was NEVER entered elsewhere (genuinely unresolved,
	// e.g. a child of P cut when P's own entry walk hit the cap) -> survives.
	childP := &DiffNode{Path: "/Root/P", Status: "unchanged", Kind: "ref", Truncated: true, capRefPair: "P"}
	childQ := &DiffNode{Path: "/Root/Q", Status: "unchanged", Kind: "ref", Truncated: true, capRefPair: "Q"}
	root := &DiffNode{Path: "/Root", Status: "unchanged", Kind: "dict", Children: []*DiffNode{childP, childQ}}

	enteredPairs := map[string]bool{"P": true} // P entered elsewhere; Q was not

	reconcileTruncation(root, enteredPairs)

	if childP.Truncated {
		t.Errorf("childP: a pair entered elsewhere should have its redundant mark cleared")
	}
	if !childQ.Truncated {
		t.Errorf("childQ: a pair never entered elsewhere is genuinely unresolved and must stay truncated")
	}
	var s DiffSummary
	countDelta(root, &s)
	if s.TruncatedSubtrees != 1 {
		t.Errorf("TruncatedSubtrees = %d, want 1: clearing an entered-but-not-fully-walked pair must not zero the count while a genuinely-unresolved pair remains", s.TruncatedSubtrees)
	}
}

// ---------------------------------------------------------------------------
// Story 14.3: a deep chain diffed past the maxResolveDepth cap marks the cut
// node DiffNode.Truncated and tallies it in DiffSummary.TruncatedSubtrees,
// and does so only at the depth-cap arm (a self-diff of a SHALLOW graph
// counts zero). This is the co-located pdfcore logic assertion the ATDD step
// deferred until the production types existed.
// ---------------------------------------------------------------------------

func TestDiff_DepthCapMarksTruncatedSubtree(t *testing.T) {
	// A chain far deeper than maxResolveDepth (32); the two sides differ only in
	// the leaf /V, well below the cut, so the difference is hidden.
	left := diffDeepChain(45, "111")
	right := diffDeepChain(45, "222")
	ins, l, r := openTwoForDiff(t, "deepA.pdf", left, "deepB.pdf", right)

	res, err := ins.DiffDocuments(l, r)
	if err != nil {
		t.Fatalf("DiffDocuments returned error: %v", err)
	}

	if res.Summary.TruncatedSubtrees < 1 {
		t.Errorf("TruncatedSubtrees = %d, want >= 1 (the deep chain is cut at the depth cap)", res.Summary.TruncatedSubtrees)
	}

	nodes := collectDiffNodes(res.Root)
	truncated := 0
	for _, n := range nodes {
		if n.Truncated {
			truncated++
			// A truncated node is the depth-capped ref, compared by shallow
			// summary, so it carries Kind "ref" and no children.
			if n.Kind != "ref" {
				t.Errorf("truncated node %q kind = %q, want \"ref\"", n.Path, n.Kind)
			}
			if len(n.Children) != 0 {
				t.Errorf("truncated node %q has %d children, want 0 (subtree not walked)", n.Path, len(n.Children))
			}
		}
	}
	if truncated != res.Summary.TruncatedSubtrees {
		t.Errorf("%d nodes carry Truncated but summary counts %d", truncated, res.Summary.TruncatedSubtrees)
	}

	// Guardrail (the "only the depth cap is truncation" rule): a SHALLOW graph
	// diffed against itself must not mark anything -- else every real PDF's
	// back-edges/dedup would flip to a false non-identical.
	shallow := diffOnePage()
	ins2, l2, r2 := openTwoForDiff(t, "s1.pdf", shallow, "s2.pdf", shallow)
	res2, err := ins2.DiffDocuments(l2, r2)
	if err != nil {
		t.Fatalf("shallow self-diff returned error: %v", err)
	}
	if res2.Summary.TruncatedSubtrees != 0 {
		t.Errorf("shallow self-diff TruncatedSubtrees = %d, want 0 (cycles/dedup are not truncation)", res2.Summary.TruncatedSubtrees)
	}
}

// ---------------------------------------------------------------------------
// A document diffed against ITSELF (same bytes, two tabs) yields an
// all-unchanged tree and a zero-delta summary.
// ---------------------------------------------------------------------------

func TestDiff_SelfIsZeroDelta(t *testing.T) {
	pdf := diffOnePage()
	ins, l, r := openTwoForDiff(t, "a.pdf", pdf, "b.pdf", pdf)

	res, err := ins.DiffDocuments(l, r)
	if err != nil {
		t.Fatalf("DiffDocuments returned error: %v", err)
	}
	if res.Root == nil {
		t.Fatalf("result Root is nil")
	}
	if res.Root.Status != "unchanged" {
		t.Errorf("Root.Status = %q, want unchanged", res.Root.Status)
	}
	if res.Summary.Added != 0 || res.Summary.Removed != 0 || res.Summary.Changed != 0 {
		t.Errorf("self-diff summary not zero: +%d -%d ~%d",
			res.Summary.Added, res.Summary.Removed, res.Summary.Changed)
	}
	for _, n := range collectDiffNodes(res.Root) {
		if n.Status != "unchanged" {
			t.Errorf("node %q has status %q in a self-diff (want all unchanged)", n.Path, n.Status)
		}
	}
}

// ---------------------------------------------------------------------------
// The ALIGNMENT GUARDRAIL. A renumbered-but- structurally-identical pair
// must produce ZERO delta. If this shows a large delta the diff is aligning
// by object number, not by structural path.
// ---------------------------------------------------------------------------

func TestDiff_RenumberedIdenticalIsZeroDelta(t *testing.T) {
	ins, l, r := openTwoForDiff(t, "natural.pdf", diffOnePage(), "renumbered.pdf", diffOnePageRenumbered())

	res, err := ins.DiffDocuments(l, r)
	if err != nil {
		t.Fatalf("DiffDocuments returned error: %v", err)
	}
	if res.Summary.Added != 0 || res.Summary.Removed != 0 || res.Summary.Changed != 0 {
		t.Errorf("path-alignment failed -- renumbered-but-identical pair shows delta +%d -%d ~%d (must be 0/0/0)",
			res.Summary.Added, res.Summary.Removed, res.Summary.Changed)
	}
	if res.Root.Status != "unchanged" {
		t.Errorf("Root.Status = %q, want unchanged for a renumbered-identical pair", res.Root.Status)
	}
}

// ---------------------------------------------------------------------------
// The alignment guardrail at CUT points. A THREE-level page tree renumbered
// asymmetrically must produce ZERO delta. The /Parent back-edges are cut and
// compared by value summary; if that summary embedded object numbers, the
// renumbered intermediate Pages nodes (different /Parent numbers) would wrongly
// show as changed. Regression guard for the number-leak.
// ---------------------------------------------------------------------------

func TestDiff_MultiLevelRenumberedIsZeroDelta(t *testing.T) {
	ins, l, r := openTwoForDiff(t, "natural3.pdf", diffThreeLevelNatural(), "renum3.pdf", diffThreeLevelRenumbered())

	res, err := ins.DiffDocuments(l, r)
	if err != nil {
		t.Fatalf("DiffDocuments returned error: %v", err)
	}
	if res.Summary.Added != 0 || res.Summary.Removed != 0 || res.Summary.Changed != 0 {
		t.Errorf("multi-level renumbered-identical pair shows delta +%d -%d ~%d (must be 0/0/0); cut-point summaries are leaking object numbers",
			res.Summary.Added, res.Summary.Removed, res.Summary.Changed)
	}
	if res.Root.Status != "unchanged" {
		t.Errorf("Root.Status = %q, want unchanged", res.Root.Status)
	}
}

// ---------------------------------------------------------------------------
// An object present only on the right is reported as ADDED (both in the tree
// and the summary count).
// ---------------------------------------------------------------------------

func TestDiff_AddedObject(t *testing.T) {
	ins, l, r := openTwoForDiff(t, "base.pdf", diffOnePage(), "withmeta.pdf", diffAddedMetadata())

	res, err := ins.DiffDocuments(l, r)
	if err != nil {
		t.Fatalf("DiffDocuments returned error: %v", err)
	}
	if res.Summary.Added < 1 {
		t.Errorf("summary.Added = %d, want >=1", res.Summary.Added)
	}
	var found *DiffNode
	for _, n := range collectDiffNodes(res.Root) {
		if n.Status == "added" {
			found = n
			break
		}
	}
	if found == nil {
		t.Fatalf("no node with status \"added\" in the tree")
	}
	if found.LeftSummary != "" {
		t.Errorf("added node LeftSummary = %q, want empty (nothing on the left)", found.LeftSummary)
	}
	if found.RightSummary == "" {
		t.Errorf("added node RightSummary is empty, want the right value repr")
	}
}

// ---------------------------------------------------------------------------
// The mirror of 003 -- an object present only on the left (right is the
// baseline) is reported as REMOVED.
// ---------------------------------------------------------------------------

func TestDiff_RemovedObject(t *testing.T) {
	ins, l, r := openTwoForDiff(t, "withmeta.pdf", diffAddedMetadata(), "base.pdf", diffOnePage())

	res, err := ins.DiffDocuments(l, r)
	if err != nil {
		t.Fatalf("DiffDocuments returned error: %v", err)
	}
	if res.Summary.Removed < 1 {
		t.Errorf("summary.Removed = %d, want >=1", res.Summary.Removed)
	}
	var found *DiffNode
	for _, n := range collectDiffNodes(res.Root) {
		if n.Status == "removed" {
			found = n
			break
		}
	}
	if found == nil {
		t.Fatalf("no node with status \"removed\" in the tree")
	}
	if found.RightSummary != "" {
		t.Errorf("removed node RightSummary = %q, want empty (nothing on the right)", found.RightSummary)
	}
}

// ---------------------------------------------------------------------------
// A modified dictionary is reported as CHANGED and its
// ChangedKeys names the modified key ("MediaBox").
// ---------------------------------------------------------------------------

func TestDiff_ChangedDictReportsChangedKey(t *testing.T) {
	ins, l, r := openTwoForDiff(t, "letter.pdf", diffOnePage(), "a4.pdf", diffChangedMediaBox())

	res, err := ins.DiffDocuments(l, r)
	if err != nil {
		t.Fatalf("DiffDocuments returned error: %v", err)
	}
	if res.Summary.Changed < 1 {
		t.Errorf("summary.Changed = %d, want >=1", res.Summary.Changed)
	}
	var pageNode *DiffNode
	for _, n := range collectDiffNodes(res.Root) {
		if n.Status == "changed" && containsStr(n.ChangedKeys, "MediaBox") {
			pageNode = n
			break
		}
	}
	if pageNode == nil {
		t.Fatalf("no changed dict reports ChangedKeys containing \"MediaBox\"")
	}
}

// ---------------------------------------------------------------------------
// A modified scalar LEAF reports distinct left vs right value summaries (the
// actual value change, not just "changed").
// ---------------------------------------------------------------------------

func TestDiff_ChangedScalarLeafReportsBothValues(t *testing.T) {
	ins, l, r := openTwoForDiff(t, "letter.pdf", diffOnePage(), "a4.pdf", diffChangedMediaBox())

	res, err := ins.DiffDocuments(l, r)
	if err != nil {
		t.Fatalf("DiffDocuments returned error: %v", err)
	}
	var leaf *DiffNode
	for _, n := range collectDiffNodes(res.Root) {
		if n.Status == "changed" && len(n.Children) == 0 && n.LeftSummary != "" && n.RightSummary != "" {
			leaf = n
			break
		}
	}
	if leaf == nil {
		t.Fatalf("no changed scalar leaf carries both a LeftSummary and a RightSummary")
	}
	if leaf.LeftSummary == leaf.RightSummary {
		t.Errorf("changed leaf has identical summaries %q; a value change must differ", leaf.LeftSummary)
	}
}

// ---------------------------------------------------------------------------
// The document summary reports the page-count change.
// ---------------------------------------------------------------------------

func TestDiff_SummaryPageCountChange(t *testing.T) {
	ins, l, r := openTwoForDiff(t, "one.pdf", diffOnePage(), "two.pdf", diffTwoPage())

	res, err := ins.DiffDocuments(l, r)
	if err != nil {
		t.Fatalf("DiffDocuments returned error: %v", err)
	}
	if res.Summary.PageCountLeft != 1 {
		t.Errorf("summary.PageCountLeft = %d, want 1", res.Summary.PageCountLeft)
	}
	if res.Summary.PageCountRight != 2 {
		t.Errorf("summary.PageCountRight = %d, want 2", res.Summary.PageCountRight)
	}
}

// ---------------------------------------------------------------------------
// A catalog /Version change is surfaced in the summary.
// ---------------------------------------------------------------------------

func TestDiff_SummaryVersionChange(t *testing.T) {
	ins, l, r := openTwoForDiff(t, "v17.pdf", diffOnePage(), "v15.pdf", diffChangedVersion())

	res, err := ins.DiffDocuments(l, r)
	if err != nil {
		t.Fatalf("DiffDocuments returned error: %v", err)
	}
	if !res.Summary.VersionChanged {
		t.Errorf("summary.VersionChanged = false, want true (catalog /Version differs)")
	}
}

// ---------------------------------------------------------------------------
// Contract shape: the summary carries the enumerated high-signal fields
// (encryption/info/xmp read outside the catalog walk). A self-diff must leave
// every one of them false/zero. This also PINS the field set at compile time
// so the CLI/GUI can rely on it.
// ---------------------------------------------------------------------------

func TestDiff_SummaryShapeStableOnSelfDiff(t *testing.T) {
	pdf := diffOnePage()
	ins, l, r := openTwoForDiff(t, "a.pdf", pdf, "b.pdf", pdf)

	res, err := ins.DiffDocuments(l, r)
	if err != nil {
		t.Fatalf("DiffDocuments returned error: %v", err)
	}
	s := res.Summary
	if s.VersionChanged || s.EncryptionChanged || s.InfoChanged || s.XMPChanged {
		t.Errorf("self-diff flags must all be false, got version=%v enc=%v info=%v xmp=%v",
			s.VersionChanged, s.EncryptionChanged, s.InfoChanged, s.XMPChanged)
	}
	if s.PageCountLeft != s.PageCountRight {
		t.Errorf("self-diff page counts differ: %d vs %d", s.PageCountLeft, s.PageCountRight)
	}
}

// ---------------------------------------------------------------------------
// A cyclic graph diffed against itself TERMINATES (depth-bounded by
// maxResolveDepth) and does not hang or stack-overflow. cyclePDF is the
// A->B->A fixture from resolve_ref_atdd_test.go.
// ---------------------------------------------------------------------------

func TestDiff_CyclicGraphTerminates(t *testing.T) {
	pdf := cyclePDF()
	ins, l, r := openTwoForDiff(t, "cyc1.pdf", pdf, "cyc2.pdf", pdf)

	res, err := ins.DiffDocuments(l, r)
	if err != nil {
		t.Fatalf("DiffDocuments returned error on a cyclic graph: %v", err)
	}
	if res.Root == nil {
		t.Fatalf("result Root is nil")
	}
	if res.Summary.Added != 0 || res.Summary.Removed != 0 || res.Summary.Changed != 0 {
		t.Errorf("cyclic self-diff should be zero delta, got +%d -%d ~%d",
			res.Summary.Added, res.Summary.Removed, res.Summary.Changed)
	}
}

// ---------------------------------------------------------------------------
// A RETARGETED indirect ref (same structural path, the ref points to a
// DIFFERENT object with different content AND a different object number) is
// reported as a change by TARGET CONTENT, not object number. This is the
// enumerated "retargeted ref" fixture, distinct from the renumbered-identical
// guardrail: here the target genuinely differs, so the diff must show a delta
// at /Root/OpenAction carrying the /S value change.
// ---------------------------------------------------------------------------

func TestDiff_RetargetedRefReportsTargetChange(t *testing.T) {
	ins, l, r := openTwoForDiff(t, "goto.pdf", diffRetargetGoTo(), "named.pdf", diffRetargetNamed())

	res, err := ins.DiffDocuments(l, r)
	if err != nil {
		t.Fatalf("DiffDocuments returned error: %v", err)
	}
	if res.Summary.Added == 0 && res.Summary.Changed == 0 {
		t.Fatalf("retargeted ref produced no delta (+%d ~%d); the change was missed",
			res.Summary.Added, res.Summary.Changed)
	}

	var openAction *DiffNode
	for _, n := range collectDiffNodes(res.Root) {
		if n.Path == "/Root/OpenAction" {
			openAction = n
			break
		}
	}
	if openAction == nil {
		t.Fatalf("no /Root/OpenAction node in the tree (ref not dereferenced by path)")
	}
	if openAction.Status != "changed" {
		t.Fatalf("Root/OpenAction status = %q, want changed", openAction.Status)
	}
	// The retarget must be reported by the TARGET's content: a changed /S leaf
	// carrying the actual name values (/GoTo -> /Named), never a bare "N G R".
	var sLeaf *DiffNode
	for _, n := range collectDiffNodes(openAction) {
		if n.Path == "/Root/OpenAction/S" {
			sLeaf = n
			break
		}
	}
	if sLeaf == nil {
		t.Fatalf("no /Root/OpenAction/S leaf; retarget not compared by target content")
	}
	if sLeaf.Status != "changed" {
		t.Errorf("Root/OpenAction/S status = %q, want changed", sLeaf.Status)
	}
	if !strings.Contains(sLeaf.LeftSummary, "GoTo") || !strings.Contains(sLeaf.RightSummary, "Named") {
		t.Errorf("S change reported as %q -> %q, want the target names (GoTo -> Named); a raw object number here means alignment leaked the ref target",
			sLeaf.LeftSummary, sLeaf.RightSummary)
	}
}

// ---------------------------------------------------------------------------
// DiffDocuments with an unknown tab id returns an error (not a panic), so a
// parse/load failure never crashes the session.
// ---------------------------------------------------------------------------

func TestDiff_UnknownTabReturnsError(t *testing.T) {
	ins := NewInspector()
	if _, err := ins.DiffDocuments("nope-left", "nope-right"); err == nil {
		t.Errorf("DiffDocuments on unknown tabs returned nil error, want an error")
	}
}

// diffDiamondLadder builds catalog(1) -> Pages(2) -> Page(3) plus a DIAMOND
// LADDER hung off the catalog via /Ladder: ladder node k carries TWO indirect
// refs (/A and /B) both pointing at node k+1, so the number of distinct
// root->leaf PATHS doubles at every level (2^levels). A path walk WITHOUT
// cross-path dedup visits ~2^levels nodes (exponential); the global
// visitedPairs dedup makes it linear because the (k+1,k+1) pair is diffed once.
func diffDiamondLadder(levels int) []byte {
	objs := []string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Ladder 4 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
	}
	base := 4 // first ladder object number
	for i := 0; i < levels; i++ {
		num := base + i
		next := num + 1
		objs = append(objs, fmt.Sprintf("%d 0 obj\n<< /Node %d /A %d 0 R /B %d 0 R >>\nendobj\n", num, i, next, next))
	}
	objs = append(objs, fmt.Sprintf("%d 0 obj\n<< /Leaf true >>\nendobj\n", base+levels))
	return assembleDiffPDF(1, objs...)
}

// ---------------------------------------------------------------------------
// A shared subgraph reachable by exponentially many structural paths (a diamond
// "ladder") is diffed in LINEAR time via the global cross-path
// (left,right)-pair dedup. Without it the walk is O(2^levels) and hangs. Guards
// the shared-/Resources re-walk and the crafted-diamond DoS.
// ---------------------------------------------------------------------------

func TestDiff_SharedSubgraphDedupTerminates(t *testing.T) {
	pdf := diffDiamondLadder(30) // 2^30 paths without dedup
	ins, l, r := openTwoForDiff(t, "ladderA.pdf", pdf, "ladderB.pdf", pdf)

	done := make(chan *DiffResult, 1)
	errc := make(chan error, 1)
	go func() {
		res, err := ins.DiffDocuments(l, r)
		if err != nil {
			errc <- err
			return
		}
		done <- res
	}()

	select {
	case err := <-errc:
		t.Fatalf("DiffDocuments error on a shared-subgraph graph: %v", err)
	case res := <-done:
		if res.Summary.Added != 0 || res.Summary.Removed != 0 || res.Summary.Changed != 0 {
			t.Errorf("shared-subgraph self-diff should be zero delta, got +%d -%d ~%d",
				res.Summary.Added, res.Summary.Removed, res.Summary.Changed)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("DiffDocuments did not terminate within 10s on a diamond ladder; cross-path dedup regressed (exponential re-walk)")
	}
}

// diffInfoProducer is diffOnePage with a trailer /Info dict (obj 4) carrying the
// given /Producer. The /Info object is referenced ONLY from the trailer, never
// from the catalog, so it is OFF the catalog-rooted walk. Two documents that
// differ only in /Producer therefore have an identical node graph (zero
// added/removed/changed) yet must still report InfoChanged. Uses
// assembleWithTrailer (validate_test.go, same package) to inject the trailer
// /Info entry that assembleDiffPDF cannot.
func diffInfoProducer(producer string) []byte {
	return assembleWithTrailer("/Info 4 0 R ",
		"%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		fmt.Sprintf("4 0 obj\n<< /Producer (%s) >>\nendobj\n", producer),
	)
}

// ---------------------------------------------------------------------------
// A trailer /Info-only difference is surfaced as InfoChanged WITHOUT bumping the
// node counts, proving /Info is read from the DocumentState trailer (off the
// catalog walk) as requires. This is the exact fact that makes the CLI's
// exit-1-on-flags-only-change contract necessary (the counts stay 0/0/0). The
// /Producer strings are the SAME length so the only possible source of a delta
// is the /Info comparison, not an object size change.
// ---------------------------------------------------------------------------

func TestDiff_SummaryInfoChange(t *testing.T) {
	ins, l, r := openTwoForDiff(t, "prodA.pdf", diffInfoProducer("AlphaLib"), "prodB.pdf", diffInfoProducer("BetaXLib"))

	res, err := ins.DiffDocuments(l, r)
	if err != nil {
		t.Fatalf("DiffDocuments returned error: %v", err)
	}
	if !res.Summary.InfoChanged {
		t.Errorf("summary.InfoChanged = false, want true (trailer /Info /Producer differs)")
	}
	if res.Summary.Added != 0 || res.Summary.Removed != 0 || res.Summary.Changed != 0 {
		t.Errorf("Info lives OFF the catalog walk, so node counts must stay 0/0/0, got +%d -%d ~%d",
			res.Summary.Added, res.Summary.Removed, res.Summary.Changed)
	}
}

// diffXMP is diffOnePage with an UNFILTERED catalog /Metadata XML stream (obj 4)
// carrying the given packet. Callers must pass equal-length packets so the stream
// DICT (/Length included) is byte-identical: the diff does not compare stream
// bodies (Out of Scope: "not a byte diff"), so a same-length body change yields
// a zero node delta, and XMPChanged is the ONLY signal - proving XMP is read via
// the metadata collector, not the object walk.
func diffXMP(packet string) []byte {
	meta := fmt.Sprintf("4 0 obj\n<< /Type /Metadata /Subtype /XML /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(packet), packet)
	return assembleDiffPDF(1,
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Metadata 4 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		meta,
	)
}

// ---------------------------------------------------------------------------
// A catalog /Metadata XMP change (equal-length packet, so the stream dict is
// identical and the object walk sees no delta) is surfaced as XMPChanged. Guards
// the XMP fact AND the Out-of-Scope "stream bytes are not diffed" boundary in one
// shot: node counts stay 0/0/0, XMPChanged is true.
// ---------------------------------------------------------------------------

func TestDiff_SummaryXMPChange(t *testing.T) {
	ins, l, r := openTwoForDiff(t, "xmpA.pdf", diffXMP("<x>AAAA</x>"), "xmpB.pdf", diffXMP("<x>BBBB</x>"))

	res, err := ins.DiffDocuments(l, r)
	if err != nil {
		t.Fatalf("DiffDocuments returned error: %v", err)
	}
	if !res.Summary.XMPChanged {
		t.Errorf("summary.XMPChanged = false, want true (catalog /Metadata XMP packet differs)")
	}
	if res.Summary.Added != 0 || res.Summary.Removed != 0 || res.Summary.Changed != 0 {
		t.Errorf("equal-length XMP means the stream dict is identical and bodies are not diffed, so node counts must stay 0/0/0, got +%d -%d ~%d",
			res.Summary.Added, res.Summary.Removed, res.Summary.Changed)
	}
}

// ---------------------------------------------------------------------------
// A KEY added to a dict present on BOTH sides is reported as an added CHILD node
// under that dict AND named in the parent's ChangedKeys - the "which keys were
// added/removed/modified" granularity, distinct from a wholly-added object.
// The right catalog gains a /Metadata key vs the left; the shared /Root dict
// must report it.
// ---------------------------------------------------------------------------

func TestDiff_ChangedDictReportsAddedKey(t *testing.T) {
	ins, l, r := openTwoForDiff(t, "base.pdf", diffOnePage(), "withmeta.pdf", diffAddedMetadata())

	res, err := ins.DiffDocuments(l, r)
	if err != nil {
		t.Fatalf("DiffDocuments returned error: %v", err)
	}
	if res.Root.Status != "changed" {
		t.Fatalf("Root status = %q, want changed (a key was added to the catalog)", res.Root.Status)
	}
	if !containsStr(res.Root.ChangedKeys, "Metadata") {
		t.Errorf("Root.ChangedKeys = %v, want it to name the added key \"Metadata\"", res.Root.ChangedKeys)
	}
	var meta *DiffNode
	for _, n := range collectDiffNodes(res.Root) {
		if n.Path == "/Root/Metadata" {
			meta = n
			break
		}
	}
	if meta == nil {
		t.Fatalf("no /Root/Metadata node; an added key must appear as an added child")
	}
	if meta.Status != "added" {
		t.Errorf("Root/Metadata status = %q, want added", meta.Status)
	}
}
