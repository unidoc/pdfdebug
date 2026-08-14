package pdfcore

import (
	"cmp"
	"fmt"
	"slices"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// GetObjectIndex returns the full xref-derived object index for the document
// in tabID. Lazy-built on first call, cached on the per-tab DocumentState so
// re-Open under the same tabID transparently invalidates (the DocumentState
// pointer is replaced).
//
// pdfcpu's XRefTable.Table is map[int]*XRefTableEntry keyed by object number,
// so only one entry per ObjNum is enumerable here -- the multi-generation
// clause is treated as best-effort and is a no-op at this layer.
func (ins *Inspector) GetObjectIndex(tabID string) ([]*ObjectIndexEntry, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	// Serialize pdfcpu access. Outer lock; objectIndexMu (inner) guards the
	// cache. buildObjectIndex walks XRefTable.Table and dereferences
	// indirect refs to compute the reachable set.
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	doc.objectIndexMu.Lock()
	defer doc.objectIndexMu.Unlock()
	if doc.objectIndexCache != nil {
		return doc.objectIndexCache, nil
	}

	var entries []*ObjectIndexEntry
	err = safeCall(func() error {
		entries = buildObjectIndex(doc)
		return nil
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}
	doc.objectIndexCache = entries
	return entries, nil
}

// buildObjectIndex walks pdfcpu's XRefTable, computes a reachable-set via BFS
// from the catalog, and emits one entry per xref slot. Sorted ObjNum asc,
// then Gen asc.
func buildObjectIndex(doc *DocumentState) []*ObjectIndexEntry {
	if doc == nil || doc.PDFContext == nil || doc.PDFContext.XRefTable == nil {
		return []*ObjectIndexEntry{}
	}
	xrt := doc.PDFContext.XRefTable

	reachable := buildReachableSet(doc)

	entries := make([]*ObjectIndexEntry, 0, len(xrt.Table))
	for objNum, entry := range xrt.Table {
		if entry == nil {
			continue
		}
		gen := 0
		if entry.Generation != nil {
			gen = *entry.Generation
		}
		// Object 0 is the head of the free list by construction -- it isn't a
		// real object and would confuse palette users. Skip it.
		if objNum == 0 {
			continue
		}
		typeName := ""
		if !entry.Free {
			typeName = extractTypeName(entry.Object)
		}
		isReachable := reachable[objNum]
		nodeID := ""
		if isReachable && !entry.Free {
			nodeID = fmt.Sprintf("obj:%d:%d", gen, objNum)
		}
		entries = append(entries, &ObjectIndexEntry{
			ObjNum:    objNum,
			Gen:       gen,
			TypeName:  typeName,
			Free:      entry.Free,
			Reachable: isReachable && !entry.Free,
			NodeID:    nodeID,
		})
	}

	slices.SortFunc(entries, func(a, b *ObjectIndexEntry) int {
		if c := cmp.Compare(a.ObjNum, b.ObjNum); c != 0 {
			return c
		}
		return cmp.Compare(a.Gen, b.Gen)
	})
	return entries
}

// buildReachableSet returns the set of object numbers reachable from the
// catalog via the dict-graph. Used to mark orphan objects (Free=false,
// Reachable=false) so the palette can render them as non-navigable rows.
func buildReachableSet(doc *DocumentState) map[int]bool {
	reachable := map[int]bool{}
	if doc == nil || doc.PDFContext == nil {
		return reachable
	}
	rootDict, err := doc.PDFContext.Catalog()
	if err != nil || rootDict == nil {
		return reachable
	}

	// Include the catalog itself if the trailer carries an indirect /Root.
	if rootRef, ok := catalogIndirectRef(doc); ok {
		reachable[rootRef.ObjectNumber.Value()] = true
	}

	// No depth cap. The visited-set (the `reachable` map plus the
	// in-progress check inside queueRefs) already prevents cycles; the prior
	// depth guard mislabeled legitimate-but-deep PDFs (page-tree chains over 32
	// levels deep) as orphan trees.
	queue := []reachEntry{{obj: rootDict, depth: 0}}
	for len(queue) > 0 {
		head := queue[0]
		queue = queue[1:]
		switch v := head.obj.(type) {
		case pdfcpu_types.Dict:
			for _, val := range v {
				queueRefs(doc, val, &queue, reachable, head.depth+1)
			}
		case pdfcpu_types.StreamDict:
			queue = append(queue, reachEntry{obj: v.Dict, depth: head.depth + 1})
		case pdfcpu_types.ObjectStreamDict:
			queue = append(queue, reachEntry{obj: v.StreamDict.Dict, depth: head.depth + 1})
		case pdfcpu_types.XRefStreamDict:
			queue = append(queue, reachEntry{obj: v.StreamDict.Dict, depth: head.depth + 1})
		case pdfcpu_types.Array:
			for _, elem := range v {
				queueRefs(doc, elem, &queue, reachable, head.depth+1)
			}
		}
	}
	return reachable
}

// reachEntry is one pending BFS frame in buildReachableSet.
type reachEntry struct {
	obj   pdfcpu_types.Object
	depth int
}

// queueRefs marks indirect targets reachable and queues them for descent.
// Inline containers descend without marking. The function is intentionally
// unexported and only used by buildReachableSet.
func queueRefs(
	doc *DocumentState,
	val pdfcpu_types.Object,
	queue *[]reachEntry,
	reachable map[int]bool,
	depth int,
) {
	if ref, ok := val.(pdfcpu_types.IndirectRef); ok {
		num := ref.ObjectNumber.Value()
		if reachable[num] {
			return
		}
		reachable[num] = true
		resolved, err := doc.PDFContext.Dereference(ref)
		if err != nil || resolved == nil {
			return
		}
		*queue = append(*queue, reachEntry{obj: resolved, depth: depth})
		return
	}
	if isContainer(val) {
		*queue = append(*queue, reachEntry{obj: val, depth: depth})
	}
}
