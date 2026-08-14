package pdfcore

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	pdfcpu_api "github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// DocumentState holds the parsed pdfcpu context and metadata for one open PDF.
//
// Lock ordering:
//
//   - pdfMu is the OUTER lock for every pdfcpu-touching Inspector method.
//     pdfcpu's XRefTable is not concurrent-read-safe (Dereference mutates the
//     object-stream resolution cache), so every method that calls into pdfcpu
//     acquires pdfMu for the duration of the pdfcpu call sequence.
//
//   - pdfMu MUST be acquired BEFORE any per-feature mutex (streamMu,
//     objectIndexMu, xrefTableMu) when the feature path calls into pdfcpu.
//
//   - plainTextMu and plainTextCancelMu are DISJOINT from pdfMu; the
//     plaintext path does not call into pdfcpu inside its critical section.
//
//   - ins.mu (Inspector.documents map) -> plainTextCancelMu is an edge:
//     Inspector.Open invokes closeDocLocked on the prior entry while holding
//     ins.mu, and closeDocLocked acquires plainTextCancelMu. No path acquires
//     ins.mu while holding plainTextCancelMu, so this edge is acyclic.
type DocumentState struct {
	FilePath   string
	PDFContext *pdfcpu_model.Context
	PageCount  int
	// FileSize is the byte size of FilePath captured ONCE at Open via os.Stat
	// (Story 10.6). Surfaced by Inspector.GetPlainTextSize directly (no
	// re-stat) and threaded into readPlainText for the buffer pre-size. If the
	// underlying file is moved or deleted after Open, this value is unchanged.
	FileSize int64

	// pdfMu serializes pdfcpu access for this DocumentState. Acquired by every
	// Inspector.GetX method that calls into pdfcpu, immediately after
	// GetDocument returns. Methods that do NOT call pdfcpu (GetPlainText,
	// GetPlainTextSize, CancelPlainText, Open, Close, GetDocument) are exempt.
	// See lock-ordering note on the struct doc comment.
	pdfMu sync.Mutex

	// revBuildOnce gates the lazy reverse-refs build. The reverse-ref index is
	// no longer built at Open time; the first GetReverseRefs call triggers
	// buildReverseRefsOnce which runs the BFS under pdfMu inside the Once's
	// inner function.
	revBuildOnce sync.Once

	streamMu    sync.Mutex
	streamCache map[string]*ContentStreamData

	// reverseRefs maps (objNum, gen) -> inbound dict-graph references. Built
	// lazily on the first GetReverseRefs call via revBuildOnce;
	// Open does not build it. The trailer's /Root pointer is NOT recorded as
	// a reverse ref (the catalog is treated as having no
	// incoming edges by construction). Nil means the build has not yet run OR
	// the build failed -- check revRefsBuildFailed to distinguish.
	reverseRefs        map[[2]int][]ReverseRef
	revRefsBuildFailed bool

	// objectIndex caches the per-tab GetObjectIndex result. Lazy on first call;
	// invalidated implicitly when the DocumentState pointer is replaced by a
	// re-Open under the same tabID.
	objectIndexMu    sync.Mutex
	objectIndexCache []*ObjectIndexEntry

	// xrefTableCache caches the per-tab GetXRefTable result. Lazy on first
	// call; invalidated implicitly when the DocumentState pointer is replaced
	// by a re-Open under the same tabID.
	xrefTableMu    sync.Mutex
	xrefTableCache *XRefTable

	// plainTextCache caches the per-tab GetPlainText result. Lazy on first
	// call; mutex coverage includes the I/O so concurrent callers share one
	// disk read. Story 9-11 Task 2.9 (cap removed in Story 10-1).
	plainTextMu    sync.Mutex
	plainTextCache *PlainTextDocument

	// plainTextLoadCancel is the cancel func for the in-flight GetPlainText
	// chunked read. Nil when no load is in flight. Guarded by
	// plainTextCancelMu (NOT plainTextMu) so CancelPlainText can preempt a
	// read without contending against the mutex the read itself holds for the
	// entire I/O.
	//
	// plainTextClosed is set by Inspector.Close under plainTextCancelMu so a
	// GetPlainText goroutine that has acquired the cancel mutex AFTER Close
	// already ran can observe the close and bail out instead of leaking the
	// read to natural completion (would defeat the "one chunk-read cycle"
	// guarantee in the race where Close fires between GetDocument and the
	// cancel-func registration).
	plainTextCancelMu   sync.Mutex
	plainTextLoadCancel context.CancelFunc
	plainTextClosed     bool
}

// Inspector manages open PDF documents keyed by tab ID. All methods are
// safe for concurrent use; the internal mutex serializes document map access.
type Inspector struct {
	mu        sync.Mutex
	documents map[string]*DocumentState
}

// NewInspector creates an Inspector with an empty document map.
func NewInspector() *Inspector {
	return &Inspector{
		documents: make(map[string]*DocumentState),
	}
}

// Open parses a PDF file and registers it under the given tab ID. Returns
// document metadata including page count and file size.
func (ins *Inspector) Open(tabID, filePath string) (*DocumentInfo, error) {
	if tabID == "" {
		return nil, fmt.Errorf("%w: empty tab ID", ErrDocumentNotFound)
	}
	if filePath == "" {
		return nil, fmt.Errorf("%w: empty file path", ErrDocumentNotFound)
	}

	fi, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %v", ErrDocumentNotFound, err)
		}
		return nil, err
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("%w: path is a directory", ErrDocumentNotFound)
	}
	fileSize := fi.Size()

	var ctx *pdfcpu_model.Context
	err = safeCall(func() error {
		var e error
		ctx, e = pdfcpu_api.ReadContextFile(filePath)
		return e
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}

	var warning string
	pageCount := 0
	err = safeCall(func() error {
		return ctx.XRefTable.EnsurePageCount()
	})
	if err != nil {
		warning = "This PDF has structural errors. Some objects may display incorrectly."
	} else {
		pageCount = ctx.PageCount
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	doc := &DocumentState{
		FilePath:   absPath,
		PDFContext: ctx,
		PageCount:  pageCount,

		FileSize: fileSize,

		streamCache: make(map[string]*ContentStreamData),
	}

	// The reverse-refs build is deferred to the first GetReverseRefs call via
	// revBuildOnce; Open does not touch the index. The build-failure log line
	// lives in buildReverseRefsOnce.

	ins.mu.Lock()
	// If a document is already registered under this tabID, release the
	// prior DocumentState's per-doc resources (cancel an in-flight plaintext
	// load, flag plainTextClosed) BEFORE inserting the new entry. Without
	// this, a tabID collision would leak the prior cancel func and let the
	// prior plaintext read complete naturally. closeDocLocked does NOT
	// acquire ins.mu (we hold it); calling the public Close would
	// self-deadlock since Go mutexes are not reentrant.
	if prior, ok := ins.documents[tabID]; ok && prior != nil {
		closeDocLocked(prior)
	}
	ins.documents[tabID] = doc
	ins.mu.Unlock()

	return &DocumentInfo{
		TabID:     tabID,
		FileName:  filepath.Base(filePath),
		FilePath:  absPath,
		PageCount: pageCount,
		FileSize:  fileSize,
		Error:     warning,
	}, nil
}

// Close removes the document associated with tabID from the inspector. If a
// GetPlainText load is in flight for this tab, the cancel func is invoked
// before the document is dropped so the read goroutine returns
// context.Canceled within one chunk-read cycle and releases its file handle.
// Story 10-1.
//
// Critical: the cancel invocation acquires plainTextCancelMu only, NEVER
// plainTextMu (which the read goroutine holds for the entire I/O). Holding
// plainTextMu here would deadlock against the active read.
func (ins *Inspector) Close(tabID string) error {
	ins.mu.Lock()
	doc, ok := ins.documents[tabID]
	if !ok {
		ins.mu.Unlock()
		return fmt.Errorf("%w: tab %q", ErrDocumentNotFound, tabID)
	}
	delete(ins.documents, tabID)
	ins.mu.Unlock()

	// Release per-doc resources outside ins.mu so a slow cancel callback
	// cannot block other Inspector calls. The same helper is invoked by
	// Inspector.Open against a prior entry on tabID collision, but in that
	// path the caller holds ins.mu and must not drop it (would race with the
	// concurrent map insert).
	closeDocLocked(doc)
	return nil
}

// closeDocLocked releases per-DocumentState resources: it sets plainTextClosed
// and invokes plainTextLoadCancel if registered. It acquires ONLY
// plainTextCancelMu; the caller controls whether ins.mu is held.
//
// Naming: "Locked" follows the Go convention "the caller is responsible for
// the higher-level lock". Inspector.Close holds ins.mu briefly to delete the
// map entry, drops it, then calls closeDocLocked. Inspector.Open holds ins.mu
// across the closeDocLocked + map-insert pair to keep the lifecycle atomic.
// Story 10-5.
func closeDocLocked(doc *DocumentState) {
	if doc == nil {
		return
	}
	doc.plainTextCancelMu.Lock()
	doc.plainTextClosed = true
	cancel := doc.plainTextLoadCancel
	doc.plainTextCancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// GetDocument returns the DocumentState for a tab, or ErrDocumentNotFound.
func (ins *Inspector) GetDocument(tabID string) (*DocumentState, error) {
	ins.mu.Lock()
	defer ins.mu.Unlock()
	doc, ok := ins.documents[tabID]
	if !ok {
		return nil, fmt.Errorf("%w: tab %q", ErrDocumentNotFound, tabID)
	}
	return doc, nil
}

// GetObjectDetail resolves a node ID to its full object representation,
// including properties for dicts, elements for arrays, or a scalar value.
func (ins *Inspector) GetObjectDetail(tabID, nodeID string) (*ObjectDetail, error) {
	if nodeID == "" {
		return nil, fmt.Errorf("%w: empty node ID", ErrDocumentNotFound)
	}

	if strings.HasPrefix(nodeID, "error:") {
		return &ObjectDetail{
			NodeID: nodeID,
			Type:   "scalar",
			ScalarValue: &ValueEntry{
				Type:    "string",
				Display: "Parse error on this object",
				Raw:     nodeID,
			},
		}, nil
	}

	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	// Serialize pdfcpu access for the duration of the resolve + property
	// extraction. pdfcpu's Dereference is not concurrent-read-safe.
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	var obj pdfcpu_types.Object
	err = safeCall(func() error {
		var e error
		obj, e = resolveNodeObject(doc, nodeID)
		return e
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}

	if obj == nil {
		return &ObjectDetail{
			NodeID:    nodeID,
			ObjectRef: objectRefFromNodeID(nodeID),
			Type:      "scalar",
			ScalarValue: &ValueEntry{
				Type:    "null",
				Display: "null",
				Raw:     "null",
			},
		}, nil
	}

	detail := &ObjectDetail{
		NodeID:    nodeID,
		ObjectRef: objectRefFromNodeID(nodeID),
	}

	// Stream types must be matched before Dict because StreamDict embeds Dict.
	switch v := obj.(type) {
	case pdfcpu_types.StreamDict:
		detail.Type = "stream"
		detail.Properties = buildPropertyEntries(v.Dict)
		detail.StreamInfo = extractStreamInfo(doc, obj)
	case pdfcpu_types.ObjectStreamDict:
		detail.Type = "stream"
		detail.Properties = buildPropertyEntries(v.StreamDict.Dict)
		detail.StreamInfo = extractStreamInfo(doc, obj)
	case pdfcpu_types.XRefStreamDict:
		detail.Type = "stream"
		detail.Properties = buildPropertyEntries(v.StreamDict.Dict)
		detail.StreamInfo = extractStreamInfo(doc, obj)
	case pdfcpu_types.Dict:
		detail.Type = "dict"
		detail.Properties = buildPropertyEntries(v)
	case pdfcpu_types.Array:
		detail.Type = "array"
		detail.Elements = buildArrayEntries(v)
	default:
		detail.Type = "scalar"
		ve := valueEntryFromObject(obj)
		detail.ScalarValue = &ve
	}

	return detail, nil
}

func valueEntryFromObject(obj pdfcpu_types.Object) ValueEntry {
	switch v := obj.(type) {
	case pdfcpu_types.Name:
		d := "/" + string(v)
		return ValueEntry{Type: "name", Display: d, Raw: d}
	case pdfcpu_types.StringLiteral:
		d := "(" + string(v) + ")"
		return ValueEntry{Type: "string", Display: d, Raw: d}
	case pdfcpu_types.HexLiteral:
		d := "<" + string(v) + ">"
		return ValueEntry{Type: "string", Display: d, Raw: d}
	case pdfcpu_types.Integer:
		d := strconv.Itoa(int(v))
		return ValueEntry{Type: "number", Display: d, Raw: d}
	case pdfcpu_types.Float:
		d := strconv.FormatFloat(float64(v), 'f', -1, 64)
		return ValueEntry{Type: "number", Display: d, Raw: d}
	case pdfcpu_types.Boolean:
		d := "false"
		if bool(v) {
			d = "true"
		}
		return ValueEntry{Type: "boolean", Display: d, Raw: d}
	case pdfcpu_types.IndirectRef:
		num := v.ObjectNumber.Value()
		gen := v.GenerationNumber.Value()
		d := fmt.Sprintf("%d %d R", num, gen)
		ref := fmt.Sprintf("obj:%d:%d", gen, num)
		return ValueEntry{Type: "reference", Display: d, Raw: d, RefTarget: ref}
	case pdfcpu_types.Dict:
		return ValueEntry{Type: "dict", Display: "<< ... >>", Raw: "<< ... >>"}
	case pdfcpu_types.Array:
		return ValueEntry{Type: "array", Display: "[...]", Raw: "[...]"}
	case nil:
		return ValueEntry{Type: "null", Display: "null", Raw: "null"}
	default:
		return ValueEntry{Type: "unknown", Display: "Unknown", Raw: "Unknown"}
	}
}

// extractStreamInfo distills a StreamInfo IPC payload from a pdfcpu stream
// object. Length resolution order:
//
//  1. sd.StreamLength populated by pdfcpu's reader -- use directly.
//  2. sd.StreamLength nil AND sd.Dict["Length"] is an Integer -- use directly.
//  3. sd.StreamLength nil AND sd.Dict["Length"] is an IndirectRef -- resolve
//     via doc.PDFContext.Dereference (caller MUST hold doc.pdfMu; the only
//     call site is GetObjectDetail, which acquires pdfMu before invoking).
//
// The prior single-arg signature could not reach pdfcpu's
// Dereference so streams with an indirect /Length whose StreamLength field was
// left nil (a corner the spec permits and pdfcpu's older reads occasionally
// expose) reported Length=0 in the inspector UI.
func extractStreamInfo(doc *DocumentState, obj pdfcpu_types.Object) *StreamInfo {
	var sd pdfcpu_types.StreamDict
	switch v := obj.(type) {
	case pdfcpu_types.StreamDict:
		sd = v
	case pdfcpu_types.ObjectStreamDict:
		sd = v.StreamDict
	case pdfcpu_types.XRefStreamDict:
		sd = v.StreamDict
	default:
		return nil
	}

	info := &StreamInfo{
		Length:  0,
		Filters: []string{},
	}

	if sd.StreamLength != nil {
		info.Length = *sd.StreamLength
	} else if lenObj, ok := sd.Dict["Length"]; ok {
		// Fallback: pdfcpu sometimes leaves StreamLength nil even when the
		// dict carries a usable /Length. Honor the Integer form directly; for
		// the IndirectRef form, dereference via doc.PDFContext.
		switch lv := lenObj.(type) {
		case pdfcpu_types.Integer:
			info.Length = int64(lv)
		case pdfcpu_types.IndirectRef:
			if doc != nil && doc.PDFContext != nil {
				// Wrap Dereference in safeCall: pdfcpu can panic with a string
				// on malformed indirect /Length targets. extractStreamInfo runs
				// OUTSIDE the safeCall in GetObjectDetail, so an uncaught panic
				// here would propagate past the pdfservice boundary (which only
				// catches runtime.Error, not string panics).
				var resolved pdfcpu_types.Object
				_ = safeCall(func() error {
					var derefErr error
					resolved, derefErr = doc.PDFContext.Dereference(lv)
					return derefErr
				})
				if n, ok := resolved.(pdfcpu_types.Integer); ok {
					info.Length = int64(n)
				}
			}
		}
	}

	for _, f := range sd.FilterPipeline {
		info.Filters = append(info.Filters, f.Name)
	}

	return info
}

func buildPropertyEntries(d pdfcpu_types.Dict) []PropertyEntry {
	entries := make([]PropertyEntry, 0, len(d))
	for key, val := range d {
		entries = append(entries, PropertyEntry{
			Key:   "/" + key,
			Value: valueEntryFromObject(val),
		})
	}
	slices.SortFunc(entries, func(a, b PropertyEntry) int {
		return cmp.Compare(a.Key, b.Key)
	})
	return entries
}

func buildArrayEntries(arr pdfcpu_types.Array) []ValueEntry {
	entries := make([]ValueEntry, 0, len(arr))
	for _, elem := range arr {
		entries = append(entries, valueEntryFromObject(elem))
	}
	return entries
}

// GetAncestorPath returns the sequence of node IDs from the catalog root down
// to nodeID, used by the frontend to expand the tree to a given object.
func (ins *Inspector) GetAncestorPath(tabID, nodeID string) ([]string, error) {
	if nodeID == "" {
		return nil, fmt.Errorf("%w: empty node ID", ErrDocumentNotFound)
	}
	if strings.HasPrefix(nodeID, "error:") {
		return nil, fmt.Errorf("cannot resolve ancestor path for error node %q", nodeID)
	}
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	// Serialize pdfcpu access for the recursive walk. The recursive
	// helper getAncestorPathDepth calls into pdfcpu (findPathToObject ->
	// Dereference); locking once here covers the whole walk.
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()
	return ins.getAncestorPathDepth(doc, tabID, nodeID, 0)
}

func (ins *Inspector) getAncestorPathDepth(doc *DocumentState, tabID, nodeID string, depth int) ([]string, error) {
	if depth > maxNodeIDDepth {
		return nil, fmt.Errorf("ancestor path nesting too deep (>%d) for %q", maxNodeIDDepth, nodeID)
	}
	if nodeID == "root" {
		return []string{"root"}, nil
	}
	kind, parentID, _ := parseNodeID(nodeID)
	switch kind {
	case "obj":
		var path []string
		err := safeCall(func() error {
			var e error
			path, e = findPathToObject(doc, nodeID)
			return e
		})
		if err != nil {
			return nil, wrapPDFError(err)
		}
		return path, nil
	case "dict", "arr":
		parentPath, err := ins.getAncestorPathDepth(doc, tabID, parentID, depth+1)
		if err != nil {
			return nil, err
		}
		return append(parentPath, nodeID), nil
	default:
		return nil, fmt.Errorf("unknown node ID kind %q", kind)
	}
}

// findPathToObject performs a BFS from the catalog root to locate targetNodeID
// in the PDF object graph, returning the path of node IDs from root to target.
func findPathToObject(doc *DocumentState, targetNodeID string) ([]string, error) {
	type queueEntry struct {
		nodeID string
		obj    pdfcpu_types.Object
		path   []string
		depth  int
	}

	rootDict, err := doc.PDFContext.Catalog()
	if err != nil {
		return nil, fmt.Errorf("failed to get catalog: %w", err)
	}

	visited := map[string]bool{"root": true}
	queue := []queueEntry{{nodeID: "root", obj: rootDict, path: []string{"root"}, depth: 0}}

	// No depth cap. The visited-set already prevents cycles;
	// the prior depth guard made findPathToObject fail to surface
	// legitimate-but-deep paths in PDFs with page-tree chains over 32 levels.
	for len(queue) > 0 {
		entry := queue[0]
		queue = queue[1:]

		switch v := entry.obj.(type) {
		case pdfcpu_types.Dict:
			for _, val := range v {
				ref, ok := val.(pdfcpu_types.IndirectRef)
				if !ok {
					// Descend into inline dicts/arrays to find nested IndirectRefs
					if isContainer(val) {
						queue = append(queue, queueEntry{
							nodeID: entry.nodeID,
							obj:    val,
							path:   entry.path,
							depth:  entry.depth + 1,
						})
					}
					continue
				}
				childID := fmt.Sprintf("obj:%d:%d", ref.GenerationNumber.Value(), ref.ObjectNumber.Value())
				if childID == targetNodeID {
					return append(entry.path, targetNodeID), nil
				}
				if visited[childID] {
					continue
				}
				visited[childID] = true
				resolved, resolveErr := doc.PDFContext.Dereference(ref)
				if resolveErr != nil || resolved == nil {
					continue
				}
				if isContainer(resolved) {
					queue = append(queue, queueEntry{
						nodeID: childID,
						obj:    resolved,
						path:   append(append([]string{}, entry.path...), childID),
						depth:  entry.depth + 1,
					})
				}
			}
		case pdfcpu_types.Array:
			for _, elem := range v {
				ref, ok := elem.(pdfcpu_types.IndirectRef)
				if !ok {
					// Descend into inline dicts/arrays to find nested IndirectRefs
					if isContainer(elem) {
						queue = append(queue, queueEntry{
							nodeID: entry.nodeID,
							obj:    elem,
							path:   entry.path,
							depth:  entry.depth + 1,
						})
					}
					continue
				}
				childID := fmt.Sprintf("obj:%d:%d", ref.GenerationNumber.Value(), ref.ObjectNumber.Value())
				if childID == targetNodeID {
					return append(entry.path, targetNodeID), nil
				}
				if visited[childID] {
					continue
				}
				visited[childID] = true
				resolved, resolveErr := doc.PDFContext.Dereference(ref)
				if resolveErr != nil || resolved == nil {
					continue
				}
				if isContainer(resolved) {
					queue = append(queue, queueEntry{
						nodeID: childID,
						obj:    resolved,
						path:   append(append([]string{}, entry.path...), childID),
						depth:  entry.depth + 1,
					})
				}
			}
		case pdfcpu_types.StreamDict:
			queue = append(queue, queueEntry{nodeID: entry.nodeID, obj: v.Dict, path: entry.path, depth: entry.depth + 1})
		case pdfcpu_types.ObjectStreamDict:
			queue = append(queue, queueEntry{nodeID: entry.nodeID, obj: v.StreamDict.Dict, path: entry.path, depth: entry.depth + 1})
		case pdfcpu_types.XRefStreamDict:
			queue = append(queue, queueEntry{nodeID: entry.nodeID, obj: v.StreamDict.Dict, path: entry.path, depth: entry.depth + 1})
		}
	}

	return nil, fmt.Errorf("object %s not found in PDF object graph", targetNodeID)
}

func isContainer(obj pdfcpu_types.Object) bool {
	switch obj.(type) {
	case pdfcpu_types.Dict, pdfcpu_types.Array,
		pdfcpu_types.StreamDict, pdfcpu_types.ObjectStreamDict, pdfcpu_types.XRefStreamDict:
		return true
	default:
		return false
	}
}

func objectRefFromNodeID(nodeID string) string {
	if !strings.HasPrefix(nodeID, "obj:") {
		return ""
	}
	kind, genStr, numStr := parseNodeID(nodeID)
	if kind != "obj" {
		return ""
	}
	// genStr is the generation number, numStr is the object number
	return numStr + " " + genStr + " R"
}
