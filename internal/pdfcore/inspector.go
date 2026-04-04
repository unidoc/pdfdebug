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

	err = safeCall(func() error {
		return ctx.XRefTable.EnsurePageCount()
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	ins.mu.Lock()
	ins.documents[tabID] = &DocumentState{
		FilePath:   absPath,
		PDFContext: ctx,
		PageCount:  ctx.PageCount,
	}
	ins.mu.Unlock()

	return &DocumentInfo{
		TabID:     tabID,
		FileName:  filepath.Base(filePath),
		FilePath:  absPath,
		PageCount: ctx.PageCount,
		FileSize:  fileSize,
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
