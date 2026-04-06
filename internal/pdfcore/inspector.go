package pdfcore

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	pdfcpu_api "github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

type DocumentState struct {
	FilePath   string
	PDFContext *pdfcpu_model.Context
	PageCount  int
}

type Inspector struct {
	mu        sync.Mutex
	documents map[string]*DocumentState
}

func NewInspector() *Inspector {
	return &Inspector{
		documents: make(map[string]*DocumentState),
	}
}

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

	ins.mu.Lock()
	ins.documents[tabID] = &DocumentState{
		FilePath:   absPath,
		PDFContext: ctx,
		PageCount:  pageCount,
	}
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

func (ins *Inspector) Close(tabID string) error {
	ins.mu.Lock()
	defer ins.mu.Unlock()
	if _, ok := ins.documents[tabID]; !ok {
		return fmt.Errorf("%w: tab %q", ErrDocumentNotFound, tabID)
	}
	delete(ins.documents, tabID)
	return nil
}

func (ins *Inspector) GetDocument(tabID string) (*DocumentState, error) {
	ins.mu.Lock()
	defer ins.mu.Unlock()
	doc, ok := ins.documents[tabID]
	if !ok {
		return nil, fmt.Errorf("%w: tab %q", ErrDocumentNotFound, tabID)
	}
	return doc, nil
}

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

	switch v := obj.(type) {
	case pdfcpu_types.StreamDict:
		detail.Type = "stream"
		detail.Properties = buildPropertyEntries(v.Dict)
		detail.StreamInfo = extractStreamInfo(obj)
	case pdfcpu_types.ObjectStreamDict:
		detail.Type = "stream"
		detail.Properties = buildPropertyEntries(v.StreamDict.Dict)
		detail.StreamInfo = extractStreamInfo(obj)
	case pdfcpu_types.XRefStreamDict:
		detail.Type = "stream"
		detail.Properties = buildPropertyEntries(v.StreamDict.Dict)
		detail.StreamInfo = extractStreamInfo(obj)
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

func extractStreamInfo(obj pdfcpu_types.Object) *StreamInfo {
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
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
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

	for len(queue) > 0 {
		entry := queue[0]
		queue = queue[1:]

		if entry.depth >= maxRefDepth {
			continue
		}

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
	kind, parentID, lastPart := parseNodeID(nodeID)
	if kind != "obj" {
		return ""
	}
	// parentID is gen, lastPart is num
	return lastPart + " " + parentID + " R"
}
