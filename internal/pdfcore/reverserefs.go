package pdfcore

import (
	"cmp"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// revBfsEntry is one indirect object pending descent. parentRef / parentNodeID
// / parentType identify the indirect object whose contents we are scanning;
// any IndirectRef found inside (including through inline dict/array nesting)
// records an inbound edge attributed to that indirect identity.
type revBfsEntry struct {
	num          int
	gen          int
	obj          pdfcpu_types.Object
	parentRef    string  // "<num> <gen> R"; "" for the special catalog source
	parentNodeID string  // "obj:<gen>:<num>"; "root" for catalog scan
	parentType   *string // /Type of this indirect object, if any
}

// buildReverseRefs walks the dict-graph from /Root via BFS, recording one
// ReverseRef per outbound IndirectRef. The visited set is keyed by (num, gen)
// so each indirect object is descended into at most once -- cycles like the
// page tree's /Parent loop terminate naturally. Cycle protection lives ONLY
// in the visited set; a depth cap would silently exclude deeply nested
// indirect objects and mis-label them as orphans.
//
// The trailer's /Root pointer is NOT recorded -- the catalog is the BFS source
// and is treated as having no incoming edges by construction (see AC10).
// However, edges OUT of the catalog into its children (e.g. /Pages 2 0 R)
// ARE recorded because the catalog is itself an indirect object in real PDFs.
func buildReverseRefs(doc *DocumentState, out map[[2]int][]ReverseRef) {
	if doc == nil || doc.PDFContext == nil {
		return
	}

	rootDict, err := doc.PDFContext.Catalog()
	if err != nil || rootDict == nil {
		return
	}

	// The catalog scan uses parentRef="" so the special-cased child-edge
	// recorder fires for refs found directly under /Root, attributing them
	// to the catalog as an indirect object (looked up via the trailer's
	// /Root pointer).
	queue := []revBfsEntry{{
		num:          -1,
		gen:          -1,
		obj:          rootDict,
		parentRef:    "",
		parentNodeID: "root",
		parentType:   dictTypeName(rootDict),
	}}

	visited := map[[2]int]bool{}
	// Mark the catalog's indirect identity as visited up front so a stray
	// indirect ref TO the catalog (e.g. /OpenAction or a JS action pointing
	// back at /Root) does NOT trigger a second descent. Without this, the
	// catalog's children would receive duplicate reverse-ref entries: one
	// recorded via recordCatalogChildEdge during the initial scan, and one
	// recorded via the standard case during the re-scan. The inbound edge to
	// the catalog itself from such a ref is still recorded -- only the
	// re-descent is suppressed.
	if rootRef, ok := catalogIndirectRef(doc); ok {
		visited[[2]int{rootRef.ObjectNumber.Value(), rootRef.GenerationNumber.Value()}] = true
	}

	for len(queue) > 0 {
		entry := queue[0]
		queue = queue[1:]
		scanContainerForRefs(doc, entry.obj, entry.parentNodeID, entry.parentRef, entry.parentType, "", out, &queue, visited)
	}

	// Stable per-key ordering: ParentRef asc (numeric on num then gen), then
	// Path asc, then ParentNodeID asc. Two refs from the same parent into
	// different slots of /Kids must have deterministic display order.
	for k, list := range out {
		slices.SortFunc(list, func(a, b ReverseRef) int {
			if c := refLess(a.ParentRef, b.ParentRef); c != 0 {
				return c
			}
			if c := cmp.Compare(a.Path, b.Path); c != 0 {
				return c
			}
			return cmp.Compare(a.ParentNodeID, b.ParentNodeID)
		})
		out[k] = list
	}
}

// scanContainerForRefs walks one container (dict / array / stream-dict) and
// records every IndirectRef it finds. Inline containers descend without
// recording an edge; pathPrefix is extended to keep the inline location.
func scanContainerForRefs(
	doc *DocumentState,
	obj pdfcpu_types.Object,
	parentNodeID, parentRef string,
	parentType *string,
	pathPrefix string,
	out map[[2]int][]ReverseRef,
	queue *[]revBfsEntry,
	visited map[[2]int]bool,
) {
	switch v := obj.(type) {
	case pdfcpu_types.Dict:
		// Sorted-key iteration for deterministic descend order; Go map
		// iteration is random and would break stable output.
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			subPath := joinPath(pathPrefix, "/"+k)
			handleValueForRefs(doc, v[k], parentNodeID, parentRef, parentType, subPath, out, queue, visited)
		}
	case pdfcpu_types.StreamDict:
		scanContainerForRefs(doc, v.Dict, parentNodeID, parentRef, parentType, pathPrefix, out, queue, visited)
	case pdfcpu_types.ObjectStreamDict:
		scanContainerForRefs(doc, v.StreamDict.Dict, parentNodeID, parentRef, parentType, pathPrefix, out, queue, visited)
	case pdfcpu_types.XRefStreamDict:
		scanContainerForRefs(doc, v.StreamDict.Dict, parentNodeID, parentRef, parentType, pathPrefix, out, queue, visited)
	case pdfcpu_types.Array:
		for i, elem := range v {
			subPath := joinPath(pathPrefix, fmt.Sprintf("[%d]", i))
			handleValueForRefs(doc, elem, parentNodeID, parentRef, parentType, subPath, out, queue, visited)
		}
	}
}

// handleValueForRefs records one inbound edge for IndirectRef values and
// descends inline containers without recording. New indirect targets are
// queued for further scanning.
func handleValueForRefs(
	doc *DocumentState,
	val pdfcpu_types.Object,
	parentNodeID, parentRef string,
	parentType *string,
	path string,
	out map[[2]int][]ReverseRef,
	queue *[]revBfsEntry,
	visited map[[2]int]bool,
) {
	if ref, ok := val.(pdfcpu_types.IndirectRef); ok {
		num := ref.ObjectNumber.Value()
		gen := ref.GenerationNumber.Value()

		switch {
		case parentRef != "":
			// Standard case: record an edge from the parent indirect object.
			rr := ReverseRef{
				ParentNodeID: parentNodeID,
				ParentRef:    parentRef,
				ParentType:   parentType,
				Path:         path,
			}
			out[[2]int{num, gen}] = append(out[[2]int{num, gen}], rr)
		case parentNodeID == "root":
			// Edges out of the catalog (whose indirect identity lives in the
			// trailer) are recorded so /Pages etc. show the catalog under
			// "Referenced by". The trailer's pointer AT the catalog itself
			// remains excluded by construction.
			recordCatalogChildEdge(doc, num, gen, path, out, parentType)
		}

		if visited[[2]int{num, gen}] {
			return
		}
		visited[[2]int{num, gen}] = true

		var resolved pdfcpu_types.Object
		err := safeCall(func() error {
			var e error
			resolved, e = doc.PDFContext.Dereference(ref)
			return e
		})
		if err != nil || resolved == nil {
			return
		}

		*queue = append(*queue, revBfsEntry{
			num:          num,
			gen:          gen,
			obj:          resolved,
			parentRef:    fmt.Sprintf("%d %d R", num, gen),
			parentNodeID: fmt.Sprintf("obj:%d:%d", gen, num),
			parentType:   dictTypeName(resolved),
		})
		return
	}

	// Inline container: descend without recording an edge.
	if isContainer(val) {
		scanContainerForRefs(doc, val, parentNodeID, parentRef, parentType, path, out, queue, visited)
	}
}

// recordCatalogChildEdge records an inbound edge from the catalog (its actual
// indirect identity from the trailer's /Root pointer) into one of its
// children. Without this, /Pages would appear as an orphan in documents whose
// catalog points at it directly.
func recordCatalogChildEdge(
	doc *DocumentState,
	num, gen int,
	path string,
	out map[[2]int][]ReverseRef,
	parentType *string,
) {
	rootRef, ok := catalogIndirectRef(doc)
	if !ok {
		return
	}
	rNum := rootRef.ObjectNumber.Value()
	rGen := rootRef.GenerationNumber.Value()
	rr := ReverseRef{
		ParentNodeID: fmt.Sprintf("obj:%d:%d", rGen, rNum),
		ParentRef:    fmt.Sprintf("%d %d R", rNum, rGen),
		ParentType:   parentType,
		Path:         path,
	}
	out[[2]int{num, gen}] = append(out[[2]int{num, gen}], rr)
}

// catalogIndirectRef returns the IndirectRef for the document catalog by
// reading the trailer's /Root entry. Returns ok=false if not available.
func catalogIndirectRef(doc *DocumentState) (pdfcpu_types.IndirectRef, bool) {
	if doc.PDFContext == nil || doc.PDFContext.XRefTable == nil {
		return pdfcpu_types.IndirectRef{}, false
	}
	root := doc.PDFContext.XRefTable.Root
	if root == nil {
		return pdfcpu_types.IndirectRef{}, false
	}
	return *root, true
}

// dictTypeName returns the /Type value of the object's dict (if any) as
// *string. Nil means "key absent" so the frontend can omit the column;
// callers must NOT collapse nil to "" because empty-string Names exist.
func dictTypeName(obj pdfcpu_types.Object) *string {
	d := asDict(obj)
	if d == nil {
		return nil
	}
	v, ok := d["Type"]
	if !ok {
		return nil
	}
	name, ok := v.(pdfcpu_types.Name)
	if !ok {
		return nil
	}
	s := string(name)
	return &s
}

// joinPath concatenates path segments. Array indices `[N]` attach directly to
// the previous segment without a separator; dict keys get a leading space when
// extending a non-empty prefix.
func joinPath(prefix, suffix string) string {
	if prefix == "" {
		return suffix
	}
	if strings.HasPrefix(suffix, "[") {
		return prefix + suffix
	}
	return prefix + " " + suffix
}

// refLess compares "N G R" forms numerically by N then G so that "10 0 R"
// sorts after "9 0 R" rather than before it.
func refLess(a, b string) int {
	an, ag, aok := parseObjGenR(a)
	bn, bg, bok := parseObjGenR(b)
	if !aok || !bok {
		return strings.Compare(a, b)
	}
	if an != bn {
		return cmp.Compare(an, bn)
	}
	return cmp.Compare(ag, bg)
}

// parseObjGenR parses a "<num> <gen> R" form into ints.
func parseObjGenR(s string) (num, gen int, ok bool) {
	parts := strings.Fields(s)
	if len(parts) != 3 || parts[2] != "R" {
		return 0, 0, false
	}
	n, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	g, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return n, g, true
}

// GetReverseRefs returns the inbound dict-graph references for the indirect
// object identified by nodeID. Inline-node IDs (dict:* / arr:*) return an
// empty slice with no error -- the frontend suppresses the section for those.
// Returns ErrReverseRefIndexUnavailable when the index could not be built at
// document open (panic during BFS); empty list on every object would be the
// forbidden silent failure mode.
func (ins *Inspector) GetReverseRefs(tabID, nodeID string) ([]ReverseRef, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	if doc.revRefsBuildFailed {
		return nil, ErrReverseRefIndexUnavailable
	}
	if !strings.HasPrefix(nodeID, "obj:") {
		return []ReverseRef{}, nil
	}
	kind, parentID, lastPart := parseNodeID(nodeID)
	if kind != "obj" {
		return []ReverseRef{}, nil
	}
	gen, err := strconv.Atoi(parentID)
	if err != nil {
		return []ReverseRef{}, nil
	}
	num, err := strconv.Atoi(lastPart)
	if err != nil {
		return []ReverseRef{}, nil
	}
	list, ok := doc.reverseRefs[[2]int{num, gen}]
	if !ok {
		return []ReverseRef{}, nil
	}
	// Return a copy so callers cannot mutate the cached slice.
	cp := make([]ReverseRef, len(list))
	copy(cp, list)
	return cp, nil
}
