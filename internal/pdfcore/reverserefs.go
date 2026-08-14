package pdfcore

import (
	"cmp"
	"fmt"
	"log"
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
	num            int
	gen            int
	obj            pdfcpu_types.Object
	parentRef      string  // "<num> <gen> R"; "" for the special catalog source
	parentNodeID   string  // "obj:<gen>:<num>"; "root" for catalog scan
	parentType     *string // /Type of this indirect object, if any
	pathFromRoot   string  // canonical BFS path from /Root to this object; "" for the catalog
}

// buildReverseRefs walks the dict-graph from /Root via BFS, recording one
// ReverseRef per outbound IndirectRef. The visited set is keyed by (num, gen)
// so each indirect object is descended into at most once -- cycles like the
// page tree's /Parent loop terminate naturally. Cycle protection lives ONLY
// in the visited set; a depth cap would silently exclude deeply nested
// indirect objects and mis-label them as orphans.
//
// The trailer's /Root pointer is NOT recorded -- the catalog is the BFS source
// and is treated as having no incoming edges by construction. However, edges
// OUT of the catalog into its children (e.g. /Pages 2 0 R) ARE recorded
// because the catalog is itself an indirect object in real PDFs.
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
		pathFromRoot: "",
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
		scanContainerForRefs(doc, entry.obj, entry.parentNodeID, entry.parentRef, entry.parentType, entry.pathFromRoot, "", out, &queue, visited)
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

	// Label fallback: when a parent's /Type is absent (e.g. the Outlines dict,
	// where /Type is optional per the PDF spec), derive a label from the
	// canonical inbound dict-entry key that led BFS to that parent. Without
	// this the UI row shows only the bare "1 0 R" with no semantic hint of
	// what 1 0 R actually is.
	applyParentLabelFallback(out)
}

// applyParentLabelFallback fills ReverseRef.ParentType for parents whose /Type
// was nil by using the last dict-key segment of the parent's own first inbound
// path. For the /Outlines dict (typically /Type omitted) reached from the
// catalog via /Outlines, this surfaces "Outlines" in the type column.
func applyParentLabelFallback(out map[[2]int][]ReverseRef) {
	labelByObj := map[[2]int]string{}
	for objKey, refs := range out {
		if len(refs) == 0 {
			continue
		}
		if label := lastDictKey(refs[0].Path); label != "" {
			labelByObj[objKey] = label
		}
	}
	for _, refs := range out {
		for i := range refs {
			if refs[i].ParentType != nil {
				continue
			}
			n, g, ok := parseObjGenR(refs[i].ParentRef)
			if !ok {
				continue
			}
			if label, ok := labelByObj[[2]int{n, g}]; ok {
				lbl := label
				refs[i].ParentType = &lbl
			}
		}
	}
}

// lastDictKey returns the last "/Key" segment of a BFS path (e.g. "/A /B [3]"
// -> "B"). Returns "" when the path has no dict-key segment.
func lastDictKey(path string) string {
	segs := strings.Fields(path)
	for i := len(segs) - 1; i >= 0; i-- {
		if k, ok := strings.CutPrefix(segs[i], "/"); ok {
			return k
		}
	}
	return ""
}

// scanContainerForRefs walks one container (dict / array / stream-dict) and
// records every IndirectRef it finds. Inline containers descend without
// recording an edge; pathPrefix is extended to keep the inline location.
// parentPathFromRoot is the canonical root-relative path of the indirect
// object whose contents we are scanning; it is forwarded onto each ReverseRef
// recorded so the UI can show the parent's structural position.
func scanContainerForRefs(
	doc *DocumentState,
	obj pdfcpu_types.Object,
	parentNodeID, parentRef string,
	parentType *string,
	parentPathFromRoot string,
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
			handleValueForRefs(doc, v[k], parentNodeID, parentRef, parentType, parentPathFromRoot, subPath, out, queue, visited)
		}
	case pdfcpu_types.StreamDict:
		scanContainerForRefs(doc, v.Dict, parentNodeID, parentRef, parentType, parentPathFromRoot, pathPrefix, out, queue, visited)
	case pdfcpu_types.ObjectStreamDict:
		scanContainerForRefs(doc, v.StreamDict.Dict, parentNodeID, parentRef, parentType, parentPathFromRoot, pathPrefix, out, queue, visited)
	case pdfcpu_types.XRefStreamDict:
		scanContainerForRefs(doc, v.StreamDict.Dict, parentNodeID, parentRef, parentType, parentPathFromRoot, pathPrefix, out, queue, visited)
	case pdfcpu_types.Array:
		for i, elem := range v {
			subPath := joinPath(pathPrefix, fmt.Sprintf("[%d]", i))
			handleValueForRefs(doc, elem, parentNodeID, parentRef, parentType, parentPathFromRoot, subPath, out, queue, visited)
		}
	}
}

// handleValueForRefs records one inbound edge for IndirectRef values and
// descends inline containers without recording. New indirect targets are
// queued for further scanning. parentPathFromRoot identifies the canonical
// BFS path of the parent indirect object.
func handleValueForRefs(
	doc *DocumentState,
	val pdfcpu_types.Object,
	parentNodeID, parentRef string,
	parentType *string,
	parentPathFromRoot string,
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
				ParentPath:   parentPathFromRoot,
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
			pathFromRoot: joinPath(parentPathFromRoot, path),
		})
		return
	}

	// Inline container: descend without recording an edge.
	if isContainer(val) {
		scanContainerForRefs(doc, val, parentNodeID, parentRef, parentType, parentPathFromRoot, path, out, queue, visited)
	}
}

// recordCatalogChildEdge records an inbound edge from the catalog (its actual
// indirect identity from the trailer's /Root pointer) into one of its
// children. Without this, /Pages would appear as an orphan in documents whose
// catalog points at it directly. The catalog's own canonical path is "" (root).
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
		ParentPath:   "",
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

// buildReverseRefsOnce runs the reverse-refs BFS exactly once per
// DocumentState via doc.revBuildOnce. The build is deferred to the first
// GetReverseRefs call so Open stays responsive on large PDFs.
//
// Locking: acquires doc.pdfMu for the duration of the BFS because the walk
// calls doc.PDFContext.Dereference, which is pdfcpu state. The Once's inner
// function is invoked at most once; concurrent first-time callers serialize
// on the Once's internal mutex and then on pdfMu.
//
// On panic during BFS, safeCall captures the panic; the inner function flags
// revRefsBuildFailed = true so subsequent callers receive
// ErrReverseRefIndexUnavailable without re-running the build. The
// build-failure log line moved here from Inspector.Open.
func buildReverseRefsOnce(doc *DocumentState) {
	if doc == nil {
		return
	}
	doc.revBuildOnce.Do(func() {
		// Defense in depth: sync.Once.Do marks the function "done" even if
		// the inner func panics. If anything inside the Do panics OUTSIDE of
		// safeCall (e.g. a future refactor moves a pdfcpu call out of the
		// safeCall, or log.Printf panics on an exhausted writer), the Once
		// would never re-run and GetReverseRefs would observe nil reverseRefs
		// + revRefsBuildFailed=false -- the forbidden silent-empty-list mode.
		// Flag the build as failed before the panic propagates so subsequent
		// callers receive ErrReverseRefIndexUnavailable.
		defer func() {
			if r := recover(); r != nil {
				doc.reverseRefs = nil
				doc.revRefsBuildFailed = true
				panic(r)
			}
		}()
		doc.pdfMu.Lock()
		defer doc.pdfMu.Unlock()
		revMap := map[[2]int][]ReverseRef{}
		buildErr := safeCall(func() error {
			buildReverseRefs(doc, revMap)
			return nil
		})
		if buildErr != nil {
			log.Printf("pdfcore: reverse-ref index build failed for %s: %v", doc.FilePath, buildErr)
			doc.reverseRefs = nil
			doc.revRefsBuildFailed = true
			return
		}
		doc.reverseRefs = revMap
	})
}

// GetReverseRefs returns the inbound dict-graph references for the indirect
// object identified by nodeID. Inline-node IDs (dict:* / arr:*) return an
// empty slice with no error -- the frontend suppresses the section for those.
// Returns ErrReverseRefIndexUnavailable when the index could not be built
// (panic during BFS); empty list on every object would be the forbidden
// silent failure mode.
//
// Story 10-5: the index is built lazily on the first call via
// buildReverseRefsOnce; subsequent calls skip the build and read the cached
// state.
func (ins *Inspector) GetReverseRefs(tabID, nodeID string) ([]ReverseRef, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	// Trigger the lazy build on first call. The helper acquires pdfMu
	// internally for the BFS (Go mutexes are not reentrant, so we MUST NOT
	// hold pdfMu here); on success doc.reverseRefs is populated, on failure
	// doc.revRefsBuildFailed is set. Concurrent first-time callers serialize
	// on revBuildOnce's internal mutex.
	buildReverseRefsOnce(doc)

	// Serialize pdfcpu access while we read doc.reverseRefs. The Once
	// guarantees the build has completed (or failed) by this point.
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

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
