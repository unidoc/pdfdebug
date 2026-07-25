package pdfcore

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// diffLockOrderMu serializes the two-document pdfMu ACQUISITION in
// DiffDocuments. Holding it only across the pair of Lock calls (released once
// both are held) makes an AB-BA deadlock impossible without needing a total
// order over DocumentState pointers - two concurrent diffs with swapped
// arguments, or two tabs on the same file, can never interleave their
// acquisitions.
var diffLockOrderMu sync.Mutex

// DiffResult is the top-level result of Inspector.DiffDocuments: the recursive
// path-aligned delta tree rooted at the catalog, plus the document-level
// summary. JSON envelope (camelCase): {"summary": {...}, "root": DiffNode}.
type DiffResult struct {
	Root    *DiffNode   `json:"root"`
	Summary DiffSummary `json:"summary"`
}

// DiffNode is one node in the structural delta tree. It is aligned by
// STRUCTURAL PATH (catalog-rooted), NOT by object number, so a renumbered but
// structurally identical document yields an all-unchanged tree. The recursive
// Children shape is modeled on ResolvedNode (model.go), NOT the flat/lazy
// TreeNode.
type DiffNode struct {
	// Path is the catalog-rooted structural path of this node, e.g.
	// "/Root/Pages/Kids[0]/MediaBox[3]". Dict keys append "/Key"; array
	// elements append "[i]".
	Path string `json:"path"`
	// Status is one of "added" (present only on the right), "removed" (present
	// only on the left), "changed" (present on both but differing), or
	// "unchanged".
	Status string `json:"status"`
	// Kind classifies the value: "dict" | "array" | "stream" | "scalar" |
	// "ref". A ref kind marks a node left unfollowed at the depth cap.
	Kind string `json:"kind"`
	// ChangedKeys names the dict keys that were added/removed/modified on a
	// changed dict. Empty for arrays, scalars, and unchanged dicts.
	ChangedKeys []string `json:"changedKeys,omitempty"`
	// LeftSummary is a short repr of the left value; "" when the node is added
	// (nothing on the left).
	LeftSummary string `json:"leftSummary"`
	// RightSummary is a short repr of the right value; "" when the node is
	// removed (nothing on the right).
	RightSummary string `json:"rightSummary"`
	// Children are the recursively-diffed dict entries / array elements, in
	// deterministic (sorted-key / array-index) order. Nil for scalar leaves,
	// single-sided (added/removed) nodes, and depth-capped refs.
	Children []*DiffNode `json:"children,omitempty"`
	// Truncated is true ONLY when this node is a ref left unwalked at the
	// maxResolveDepth depth cap and compared by shallow summary (Story 14.3
	// AC1). It is NOT set for back-edge (cycle) cuts or the visitedPairs
	// cross-path dedup, both of which hide nothing (the target is fully
	// accounted for elsewhere). A truncated node's shallow summaries can match
	// while a deeper difference is hidden, so the run must not be called
	// identical; see DiffSummary.TruncatedSubtrees.
	Truncated bool `json:"truncated,omitempty"`
	// capRefPair is the "numL:genL|numR:genR" key of a depth-capped ref pair,
	// recorded so reconcileTruncation can clear Truncated when the SAME pair is
	// fully walked on another (shallower) path. Unexported: internal to the
	// walk, never marshaled across the IPC boundary.
	capRefPair string
}

// DiffSummary is the document-level "what changed at a glance" tally. The
// added/removed/changed counts are DELTA-POINT counts: added/removed subtrees
// (always leaf nodes) plus changed scalar leaves. Intermediate changed
// containers (the catalog, /Pages, ...) are the ROUTE to a change, not a change
// themselves, so they are NOT counted - this keeps the count object/value
// level and matches the GUI's navigable-change count (DiffView collectChanges).
// The remaining fields are high-signal facts. EncryptionChanged is read from
// each DocumentState's trailer /Encrypt (NOT the catalog walk, which never
// reaches it); PageCount is read from each DocumentState.
type DiffSummary struct {
	Added             int  `json:"added"`
	Removed           int  `json:"removed"`
	Changed           int  `json:"changed"`
	PageCountLeft     int  `json:"pageCountLeft"`
	PageCountRight    int  `json:"pageCountRight"`
	VersionChanged    bool `json:"versionChanged"`    // catalog /Version
	EncryptionChanged bool `json:"encryptionChanged"` // trailer /Encrypt presence
	InfoChanged       bool `json:"infoChanged"`       // /Info dictionary fields
	XMPChanged        bool `json:"xmpChanged"`        // catalog /Metadata XMP packet
	// TruncatedSubtrees counts the nodes cut at the maxResolveDepth depth cap
	// (DiffNode.Truncated) - subtrees compared only by shallow summary, whose
	// deeper contents were not walked (Story 14.3 AC2). When > 0 the walk was
	// bounded, so the pair CANNOT be certified identical: the CLI withholds
	// exit 0 and the "structurally identical" claim. Not omitempty so the
	// honest count is always present in the JSON contract.
	TruncatedSubtrees int `json:"truncatedSubtrees"`
}

// diffContext carries the two documents through the recursive walk so the
// per-side dereference always targets the correct DocumentState.
type diffContext struct {
	left  *DocumentState
	right *DocumentState
	// visitedPairs records the (left,right) indirect-ref pairs already fully
	// diffed, keyed "numL:genL|numR:genR". It is a GLOBAL (never-cleared)
	// cross-path dedup: a subgraph shared by many referrers (e.g. one /Resources
	// referenced by every page) is walked once, and a crafted diamond graph
	// cannot blow up exponentially. Distinct from the path-scoped visited sets,
	// which cut only back-edges (cycles) on the current ancestor path.
	visitedPairs map[string]bool
}

// DiffDocuments computes the path-aligned structural delta between two already
// open documents (two tab IDs in ONE Inspector). It walks both object graphs
// from the catalog, aligning by structural path rather than object number, so a
// regenerated/renumbered file does not appear as "everything changed". The walk
// is bounded two ways: a per-side path-scoped visited set cuts back-edges (e.g.
// a page's /Parent) so cycles do not re-walk, and maxResolveDepth caps depth as
// a backstop; a cut/capped ref is compared by shallow value summary rather than
// recursed. Every pdfcpu access is wrapped in safeCall. It is a read-only walk
// over both
// DocumentStates; neither document is modified.
//
// Returns an error (never a panic) when either tab is unknown or a catalog read
// fails, so a parse/load failure never crashes the session.
func (ins *Inspector) DiffDocuments(leftTabID, rightTabID string) (*DiffResult, error) {
	leftDoc, err := ins.GetDocument(leftTabID)
	if err != nil {
		return nil, err
	}
	rightDoc, err := ins.GetDocument(rightTabID)
	if err != nil {
		return nil, err
	}

	// Lock both documents' pdfMu for the whole walk (pdfcpu is not
	// concurrent-read-safe). The package-level ordering mutex serializes the
	// acquisition so swapped-argument concurrent diffs cannot deadlock; lock
	// once when both tabs resolve to the same DocumentState.
	diffLockOrderMu.Lock()
	leftDoc.pdfMu.Lock()
	if rightDoc != leftDoc {
		rightDoc.pdfMu.Lock()
	}
	diffLockOrderMu.Unlock()
	defer leftDoc.pdfMu.Unlock()
	if rightDoc != leftDoc {
		defer rightDoc.pdfMu.Unlock()
	}

	var leftCat, rightCat pdfcpu_types.Dict
	err = safeCall(func() error {
		var e error
		leftCat, e = leftDoc.PDFContext.Catalog()
		return e
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}
	err = safeCall(func() error {
		var e error
		rightCat, e = rightDoc.PDFContext.Catalog()
		return e
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}

	dc := &diffContext{left: leftDoc, right: rightDoc, visitedPairs: map[string]bool{}}

	// Seed each side's path-scoped visited set with its catalog object so a ref
	// back to the catalog is caught as a back-edge (like resolve.go's guard).
	leftVisited := map[string]bool{}
	rightVisited := map[string]bool{}
	if ref, ok := catalogIndirectRef(leftDoc); ok {
		leftVisited[refVisitKey(ref)] = true
	}
	if ref, ok := catalogIndirectRef(rightDoc); ok {
		rightVisited[refVisitKey(ref)] = true
	}

	var root *DiffNode
	err = safeCall(func() error {
		root = dc.diffPresent("/Root", leftCat, rightCat, 0, leftVisited, rightVisited)
		return nil
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}

	// Clear depth-cap marks on pairs that were fully walked elsewhere before
	// tallying, so a shared object reached both shallow and past the cap is not
	// counted as truncated (would falsely withhold "identical"). Runs once with
	// the complete visitedPairs set, so it is DFS-order independent.
	reconcileTruncation(root, dc.visitedPairs)

	summary := DiffSummary{}
	countDelta(root, &summary)
	dc.fillSummary(&summary, leftCat, rightCat)
	return &DiffResult{Root: root, Summary: summary}, nil
}

// diffPresent diffs two values that are BOTH present at path (already
// dereferenced to their resolved form). depth is the total recursion depth
// (ref follows plus direct dict/array nesting) below the catalog;
// leftVisited/rightVisited are the per-side path-scoped visited sets used to cut
// back-edges (e.g. /Parent).
func (dc *diffContext) diffPresent(path string, left, right pdfcpu_types.Object, depth int, leftVisited, rightVisited map[string]bool) *DiffNode {
	lk := resolvedNodeType(left)
	rk := resolvedNodeType(right)

	// Kind mismatch (e.g. a dict replaced by an array): report as a changed
	// leaf carrying both summaries rather than trying to align dissimilar
	// shapes.
	if lk != rk {
		return &DiffNode{
			Path:         path,
			Status:       "changed",
			Kind:         rk,
			LeftSummary:  diffSummarize(left),
			RightSummary: diffSummarize(right),
		}
	}

	switch lk {
	case "dict", "stream":
		return dc.diffDict(path, asDict(left), asDict(right), lk, depth, leftVisited, rightVisited)
	case "array":
		la, _ := left.(pdfcpu_types.Array)
		ra, _ := right.(pdfcpu_types.Array)
		return dc.diffArray(path, la, ra, depth, leftVisited, rightVisited)
	default: // scalar (and unresolved ref summaries)
		return scalarLeaf(path, lk, left, right)
	}
}

// scalarLeaf builds a leaf DiffNode comparing two resolved values by their
// shallow summary: unchanged when the summaries match, changed otherwise. Used
// for scalar leaves and for cut/depth-capped refs.
func scalarLeaf(path, kind string, left, right pdfcpu_types.Object) *DiffNode {
	ls := diffSummarize(left)
	rs := diffSummarize(right)
	status := "unchanged"
	if ls != rs {
		status = "changed"
	}
	return &DiffNode{Path: path, Status: status, Kind: kind, LeftSummary: ls, RightSummary: rs}
}

// diffDict diffs two dictionaries (or stream dicts) over the union of their
// keys. A key present on only one side is added/removed; a key on both is
// recursed. ChangedKeys names every key whose child is not unchanged.
func (dc *diffContext) diffDict(path string, ld, rd pdfcpu_types.Dict, kind string, depth int, leftVisited, rightVisited map[string]bool) *DiffNode {
	node := &DiffNode{Path: path, Kind: kind}

	keySet := map[string]bool{}
	for k := range ld {
		keySet[k] = true
	}
	for k := range rd {
		keySet[k] = true
	}
	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	var changedKeys []string
	for _, k := range keys {
		lv, lok := ld[k]
		rv, rok := rd[k]
		childPath := path + "/" + k
		var child *DiffNode
		switch {
		case lok && rok:
			child = dc.diffChild(childPath, lv, rv, depth, leftVisited, rightVisited)
		case lok:
			child = dc.singleSided(childPath, dc.left, lv, "removed")
		default:
			child = dc.singleSided(childPath, dc.right, rv, "added")
		}
		node.Children = append(node.Children, child)
		if child.Status != "unchanged" {
			changedKeys = append(changedKeys, k)
		}
	}

	node.ChangedKeys = changedKeys
	if len(changedKeys) > 0 {
		node.Status = "changed"
	} else {
		node.Status = "unchanged"
	}
	return node
}

// diffArray diffs two arrays index-by-index. Trailing elements on the longer
// side are added/removed. The array node is changed when any element differs.
func (dc *diffContext) diffArray(path string, la, ra pdfcpu_types.Array, depth int, leftVisited, rightVisited map[string]bool) *DiffNode {
	node := &DiffNode{Path: path, Kind: "array"}
	n := len(la)
	if len(ra) > n {
		n = len(ra)
	}
	changed := false
	for i := 0; i < n; i++ {
		childPath := fmt.Sprintf("%s[%d]", path, i)
		var child *DiffNode
		switch {
		case i < len(la) && i < len(ra):
			child = dc.diffChild(childPath, la[i], ra[i], depth, leftVisited, rightVisited)
		case i < len(la):
			child = dc.singleSided(childPath, dc.left, la[i], "removed")
		default:
			child = dc.singleSided(childPath, dc.right, ra[i], "added")
		}
		node.Children = append(node.Children, child)
		if child.Status != "unchanged" {
			changed = true
		}
	}
	if changed {
		node.Status = "changed"
	} else {
		node.Status = "unchanged"
	}
	return node
}

// diffChild diffs one dict-entry/array-element value present on both sides,
// dereferencing indirect refs (so alignment is by target structure, never
// object number). Every level costs one depth unit (see the nextDepth note
// below). Following a ref pushes the target onto the per-side path-scoped
// visited set so a back-edge (e.g. a page's /Parent pointing at an ancestor
// already on the path) is CUT rather than re-walked - without this, every real
// page's /Parent explodes the diff up to the depth cap. A both-sides ref pair
// already diffed on another path is cut via the global visitedPairs dedup. A
// cut or depth-capped ref is compared by shallow value summary only, then the
// path-scoped entry is popped on unwind (so a diamond is not mislabeled).
func (dc *diffContext) diffChild(path string, lv, rv pdfcpu_types.Object, depth int, leftVisited, rightVisited map[string]bool) *DiffNode {
	lKey := refVisitKeyOf(lv)
	rKey := refVisitKeyOf(rv)
	leftCycle := lKey != "" && leftVisited[lKey]
	rightCycle := rKey != "" && rightVisited[rKey]

	// depth bounds TOTAL recursion (indirect-ref follows AND direct dict/array
	// nesting), not just ref levels. A PDF can nest DIRECT arrays/dicts
	// arbitrarily deep (e.g. [[[[...]]]]) and pdfcpu will parse it; counting only
	// ref-follows would let such a structure recurse until the goroutine stack
	// overflows - a fatal crash that neither safeCall (it re-panics on
	// runtime.Error) nor recoverRuntimePanic can catch. Incrementing every level
	// caps the recursion at maxResolveDepth for direct nesting too.
	nextDepth := depth + 1
	lres := dereferenceIfRef(dc.left, lv)
	rres := dereferenceIfRef(dc.right, rv)

	// Back-edge (cycle): a followed ref re-enters an object already on the
	// current path (e.g. a page's /Parent). The target is fully accounted for
	// by its first visit, so the shallow-summary comparison hides nothing - this
	// is NOT truncation and must not be marked (marking it would flip every real
	// multi-page PDF to a false non-identical; Story 14.3 "Only the depth cap is
	// truncation").
	if leftCycle || rightCycle {
		return scalarLeaf(path, "ref", lres, rres)
	}
	// Depth cap: the subtree below maxResolveDepth is ABANDONED unwalked, so the
	// shallow summary can hide a deeper difference. Mark it truncated (AC1) so
	// the run cannot claim "identical" while a difference was left unexplored.
	if nextDepth > maxResolveDepth {
		leaf := scalarLeaf(path, "ref", lres, rres)
		leaf.Truncated = true
		// Record the ref-pair (both sides indirect) so reconcileTruncation can
		// clear this mark if the SAME pair is fully walked on a shallower path:
		// a pair accounted for elsewhere hides nothing here. Without this a
		// shared object reachable both shallow and past the cap over-counts
		// TruncatedSubtrees and flips an identical pair to a false exit 1
		// (the visitedPairs dedup below cannot catch it - it runs after this
		// return, and the deep leg may be walked before the shallow one).
		if lKey != "" && rKey != "" {
			leaf.capRefPair = lKey + "|" + rKey
		}
		return leaf
	}

	// Global cross-path dedup: an indirect-ref pair present on BOTH sides and
	// already fully diffed elsewhere is compared by shallow summary rather than
	// re-walked. Sharing/re-convergence in a PDF happens only through indirect
	// refs (a direct value has a single parent), so this precisely targets
	// shared subgraphs: without it, a /Resources shared by N pages is re-diffed
	// N times - O(N x subtree) - and a diamond "ladder" is exponential even
	// under the depth cap (2^depth paths). Keyed on the PAIR so the same left
	// object aligned against two different right objects is still diffed at both.
	if lKey != "" && rKey != "" {
		pairKey := lKey + "|" + rKey
		if dc.visitedPairs[pairKey] {
			return scalarLeaf(path, "ref", lres, rres)
		}
		dc.visitedPairs[pairKey] = true
	}

	// Push the followed targets onto the path, recurse, then pop on unwind.
	if lKey != "" {
		leftVisited[lKey] = true
		defer delete(leftVisited, lKey)
	}
	if rKey != "" {
		rightVisited[rKey] = true
		defer delete(rightVisited, rKey)
	}
	return dc.diffPresent(path, lres, rres, nextDepth, leftVisited, rightVisited)
}

// refVisitKey returns the "num:gen" path-visited key for an indirect ref.
func refVisitKey(ref pdfcpu_types.IndirectRef) string {
	return fmt.Sprintf("%d:%d", ref.ObjectNumber.Value(), ref.GenerationNumber.Value())
}

// refVisitKeyOf returns the visited key for obj when it is an indirect ref, ""
// otherwise.
func refVisitKeyOf(obj pdfcpu_types.Object) string {
	if ref, ok := obj.(pdfcpu_types.IndirectRef); ok {
		return refVisitKey(ref)
	}
	return ""
}

// singleSided builds an added/removed leaf node for a value present on only one
// side. The present side carries a summary; the absent side is "". The value is
// dereferenced so the summary reflects the target object, not a bare ref.
func (dc *diffContext) singleSided(path string, doc *DocumentState, val pdfcpu_types.Object, status string) *DiffNode {
	resolved := dereferenceIfRef(doc, val)
	summary := diffSummarize(resolved)
	node := &DiffNode{Path: path, Status: status, Kind: resolvedNodeType(resolved)}
	if status == "added" {
		node.RightSummary = summary
	} else {
		node.LeftSummary = summary
	}
	return node
}

// reconcileTruncation clears the depth-cap Truncated mark on any node whose
// ref-pair was fully walked elsewhere in the graph (its capRefPair is in
// walkedPairs). A pair diffed on a shallow path is fully accounted for, so a
// second encounter past the depth cap hides nothing; marking it would
// over-count DiffSummary.TruncatedSubtrees and wrongly withhold the "identical"
// verdict (Story 14.3). It runs once after the walk, when walkedPairs is
// complete, so it corrects BOTH DFS orders (shallow-first: the deep leg is
// cleared here; deep-first: the deep leg is marked during the walk, then
// cleared here once the shallow leg has populated walkedPairs). Depth-cap cuts
// with no ref-pair (deep DIRECT nesting, or a ref aligned against a non-ref)
// carry no capRefPair and are left marked - they are genuinely unresolved.
func reconcileTruncation(n *DiffNode, walkedPairs map[string]bool) {
	if n == nil {
		return
	}
	if n.Truncated && n.capRefPair != "" && walkedPairs[n.capRefPair] {
		n.Truncated = false
	}
	for _, c := range n.Children {
		reconcileTruncation(c, walkedPairs)
	}
}

// countDelta tallies the delta POINTS over the whole tree: added/removed
// subtrees (always leaves) and changed scalar leaves. A changed CONTAINER (dict
// or array with children) is only the route to a deeper change, so counting it
// would inflate the tally (a single /MediaBox edit would count the catalog,
// /Pages, the page dict, the array, and the leaf). Counting only leaf-level
// deltas keeps Changed object/value level and equal to the GUI's navigable
// count.
func countDelta(n *DiffNode, s *DiffSummary) {
	if n == nil {
		return
	}
	// A depth-capped node is counted regardless of its shallow-summary status:
	// its subtree was not walked, so a matching summary does not prove equality
	// (Story 14.3 AC2).
	if n.Truncated {
		s.TruncatedSubtrees++
	}
	isLeaf := len(n.Children) == 0
	switch n.Status {
	case "added":
		s.Added++
	case "removed":
		s.Removed++
	case "changed":
		if isLeaf {
			s.Changed++
		}
	}
	for _, c := range n.Children {
		countDelta(c, s)
	}
}

// fillSummary populates the document-level high-signal facts. Page count and
// encryption come from each DocumentState (encryption lives in the trailer
// /Encrypt, off the catalog walk); /Version from the catalog; /Info and XMP via
// the existing metadata collectors.
func (dc *diffContext) fillSummary(s *DiffSummary, leftCat, rightCat pdfcpu_types.Dict) {
	s.PageCountLeft = dc.left.PageCount
	s.PageCountRight = dc.right.PageCount
	s.VersionChanged = catalogVersion(leftCat) != catalogVersion(rightCat)
	s.EncryptionChanged = isEncrypted(dc.left) != isEncrypted(dc.right)

	lmd := &DocumentMetadata{Info: map[string]string{}}
	rmd := &DocumentMetadata{Info: map[string]string{}}
	_ = safeCall(func() error {
		collectInfoFields(dc.left, lmd)
		collectXMP(dc.left, lmd)
		return nil
	})
	_ = safeCall(func() error {
		collectInfoFields(dc.right, rmd)
		collectXMP(dc.right, rmd)
		return nil
	})
	s.InfoChanged = !infoEqual(lmd.Info, rmd.Info)
	s.XMPChanged = lmd.XMP != rmd.XMP
}

// catalogVersion returns the catalog /Version name value ("" when absent).
func catalogVersion(cat pdfcpu_types.Dict) string {
	if cat == nil {
		return ""
	}
	if v, ok := cat["Version"].(pdfcpu_types.Name); ok {
		return string(v)
	}
	return ""
}

// isEncrypted reports whether the document carries a trailer /Encrypt entry.
func isEncrypted(doc *DocumentState) bool {
	return doc.PDFContext != nil && doc.PDFContext.XRefTable != nil && doc.PDFContext.XRefTable.Encrypt != nil
}

// infoEqual reports whether two /Info field maps carry the same key/value set.
func infoEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// diffSummarize renders a short, deterministic repr of an object for the
// left/right summary fields and for cut/depth-capped/scalar equality
// comparison. Containers are summarized SHALLOWLY (one level) so a summary
// comparison at a cut point is stable and bounded. Indirect refs render as a
// number-independent "<ref>" token, NOT "N G R" - object numbers are not stable
// across files, so embedding them would make a renumbered-but-identical pair
// compare as changed at cut points (defeating the path-alignment guarantee).
func diffSummarize(obj pdfcpu_types.Object) string {
	switch v := obj.(type) {
	case pdfcpu_types.Dict:
		return dictSummary(v)
	case pdfcpu_types.StreamDict:
		return dictSummary(v.Dict) + " stream"
	case pdfcpu_types.ObjectStreamDict:
		return dictSummary(v.StreamDict.Dict) + " stream"
	case pdfcpu_types.XRefStreamDict:
		return dictSummary(v.StreamDict.Dict) + " stream"
	case pdfcpu_types.Array:
		return arraySummary(v)
	case pdfcpu_types.IndirectRef:
		return "<ref>"
	default:
		return scalarDisplay(obj)
	}
}

// dictSummary renders a shallow "<< /K v ... >>" repr with sorted keys.
func dictSummary(d pdfcpu_types.Dict) string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var b strings.Builder
	b.WriteString("<<")
	for _, k := range keys {
		b.WriteString(" /")
		b.WriteString(k)
		b.WriteString(" ")
		b.WriteString(shallowValue(d[k]))
	}
	b.WriteString(" >>")
	return b.String()
}

// arraySummary renders a shallow "[ e1 e2 ... ]" repr.
func arraySummary(a pdfcpu_types.Array) string {
	var b strings.Builder
	b.WriteString("[")
	for _, e := range a {
		b.WriteString(" ")
		b.WriteString(shallowValue(e))
	}
	b.WriteString(" ]")
	return b.String()
}

// shallowValue renders one nested value for a container summary WITHOUT
// recursing into further containers (they collapse to "<<...>>" / "[...]"),
// keeping summaries bounded and comparison cheap. Indirect refs render as a
// number-independent "<ref>" token (see diffSummarize) so a cut-point summary
// comparison stays renumber-invariant.
func shallowValue(obj pdfcpu_types.Object) string {
	switch obj.(type) {
	case pdfcpu_types.Dict, pdfcpu_types.StreamDict, pdfcpu_types.ObjectStreamDict, pdfcpu_types.XRefStreamDict:
		return "<<...>>"
	case pdfcpu_types.Array:
		return "[...]"
	case pdfcpu_types.IndirectRef:
		return "<ref>"
	default:
		return scalarDisplay(obj)
	}
}
