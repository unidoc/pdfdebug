package pdfcore

import (
	"cmp"
	"fmt"
	"log"
	"slices"
)

// GetXRefTable returns the cross-reference table view for the document in
// tabID. Lazy-built on first call, cached on the per-tab DocumentState; the
// xref does not change for an opened document, so subsequent calls return the
// cached pointer. Story 9-11.
//
// The cache-check, build, and cache-store all happen under xrefTableMu so two
// concurrent callers share one build pass (the second blocks, then sees the
// cache and returns immediately). pdfcpu's XRefTable.Table is a map keyed by
// object number, so iteration order is non-deterministic; sorting on egress
// (ObjNum asc, Gen asc) is mandatory.
func (ins *Inspector) GetXRefTable(tabID string) (*XRefTable, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	// Serialize pdfcpu access. Outer lock; xrefTableMu (inner) guards the
	// cache. buildXRefTable iterates pdfcpu's XRefTable.Table which is not
	// concurrent-read-safe.
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	doc.xrefTableMu.Lock()
	defer doc.xrefTableMu.Unlock()
	if doc.xrefTableCache != nil {
		return doc.xrefTableCache, nil
	}

	var table *XRefTable
	err = safeCall(func() error {
		table = buildXRefTable(doc, tabID)
		return nil
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}
	doc.xrefTableCache = table
	return table, nil
}

// buildXRefTable iterates pdfcpu's XRefTable.Table and produces the IPC-stable
// XRefTable payload sorted by (ObjNum asc, Gen asc). Object 0 (the free-list
// head) is skipped because it isn't a real object. Nil entries are skipped
// defensively (pdfcpu can leave map slots unpopulated on partial parses --
// precedent: objectindex.go:56-58).
func buildXRefTable(doc *DocumentState, tabID string) *XRefTable {
	out := &XRefTable{TabID: tabID, Entries: []XRefEntry{}}
	if doc == nil || doc.PDFContext == nil || doc.PDFContext.XRefTable == nil {
		return out
	}
	xrt := doc.PDFContext.XRefTable

	entries := make([]XRefEntry, 0, len(xrt.Table))
	for objNum, entry := range xrt.Table {
		if entry == nil {
			continue
		}
		if objNum == 0 {
			// Object 0 is the free-list head by construction -- skip.
			continue
		}

		gen := 0
		if entry.Generation != nil {
			gen = *entry.Generation
		}

		var status string
		var offset int64 = -1
		var hostObjStm int
		var nodeID string

		switch {
		case entry.Free:
			status = "free"
			nodeID = ""
		case entry.Compressed:
			status = "in-objstm"
			if entry.ObjectStream != nil {
				hostObjStm = *entry.ObjectStream
			}
			// Compressed objects use gen=0 per ISO 32000-1 §7.5.8.1; we honour
			// the parsed gen field, but the navigation target uses the parsed
			// value as-is (best effort if a malformed file disagrees).
			nodeID = fmt.Sprintf("obj:%d:%d", gen, objNum)
		default:
			status = "in-use"
			if entry.Offset != nil {
				offset = *entry.Offset
			} else {
				// Malformed in-use entry with nil Offset: log, emit with
				// sentinel -1 so the frontend renders "-" rather than crash.
				log.Printf("pdfcore: xref entry %d has nil Offset; emitting with sentinel", objNum)
			}
			nodeID = fmt.Sprintf("obj:%d:%d", gen, objNum)
		}

		entries = append(entries, XRefEntry{
			ObjNum:     objNum,
			Gen:        gen,
			Status:     status,
			Offset:     offset,
			HostObjStm: hostObjStm,
			NodeID:     nodeID,
		})
	}

	slices.SortFunc(entries, func(a, b XRefEntry) int {
		if c := cmp.Compare(a.ObjNum, b.ObjNum); c != 0 {
			return c
		}
		return cmp.Compare(a.Gen, b.Gen)
	})
	out.Entries = entries
	return out
}
