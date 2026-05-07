package pdfcore

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// GetTreeRoot returns the top-level catalog node for the document in tabID.
func (ins *Inspector) GetTreeRoot(tabID string) (*TreeNode, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}

	var rootDict pdfcpu_types.Dict
	err = safeCall(func() error {
		var e error
		rootDict, e = doc.PDFContext.Catalog()
		return e
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}

	if rootDict == nil {
		return nil, wrapPDFError(fmt.Errorf("catalog dictionary is nil"))
	}

	return &TreeNode{
		ID:          "root",
		Label:       "Catalog",
		RawKey:      "",
		NodeType:    "dict",
		ValueType:   "",
		HasChildren: len(rootDict) > 0,
		ChildCount:  len(rootDict),
		IconHint:    "catalog",
	}, nil
}

// GetChildren returns the immediate child tree nodes for a given node ID.
func (ins *Inspector) GetChildren(tabID string, nodeID string) ([]*TreeNode, error) {
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
		return nil, wrapPDFError(fmt.Errorf("failed to resolve node %q: %w", nodeID, err))
	}

	return buildChildren(doc, nodeID, obj), nil
}

// maxRefDepth guards against circular IndirectRef chains in malformed PDFs.
const maxRefDepth = 32

func buildChildren(doc *DocumentState, parentID string, obj pdfcpu_types.Object) []*TreeNode {
	return buildChildrenDepth(doc, parentID, obj, 0)
}

func buildChildrenDepth(doc *DocumentState, parentID string, obj pdfcpu_types.Object, depth int) []*TreeNode {
	switch v := obj.(type) {
	case pdfcpu_types.Dict:
		return buildDictChildren(doc, parentID, v)
	case pdfcpu_types.StreamDict:
		return buildDictChildren(doc, parentID, v.Dict)
	case pdfcpu_types.ObjectStreamDict:
		return buildDictChildren(doc, parentID, v.StreamDict.Dict)
	case pdfcpu_types.XRefStreamDict:
		return buildDictChildren(doc, parentID, v.StreamDict.Dict)
	case pdfcpu_types.Array:
		return buildArrayChildren(doc, parentID, v)
	case pdfcpu_types.IndirectRef:
		if depth >= maxRefDepth {
			return []*TreeNode{makeErrorNode(fmt.Sprintf("error:%s:depth", parentID), "", fmt.Errorf("circular reference detected (depth %d)", depth))}
		}
		var resolved pdfcpu_types.Object
		err := safeCall(func() error {
			var e error
			resolved, e = doc.PDFContext.Dereference(v)
			return e
		})
		if err != nil {
			return []*TreeNode{makeErrorNode(fmt.Sprintf("error:%s:deref", parentID), "", err)}
		}
		if resolved == nil {
			return []*TreeNode{makeErrorNode(fmt.Sprintf("error:%s:null", parentID), "", fmt.Errorf("dangling indirect reference obj:%d:%d", v.ObjectNumber.Value(), v.GenerationNumber.Value()))}
		}
		return buildChildrenDepth(doc, parentID, resolved, depth+1)
	default:
		return []*TreeNode{}
	}
}

func buildDictChildren(doc *DocumentState, parentID string, d pdfcpu_types.Dict) []*TreeNode {
	// Sort keys for deterministic child order across runs
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	nodes := make([]*TreeNode, 0, len(d))
	for _, bareKey := range keys {
		val := d[bareKey]
		var node *TreeNode
		err := safeCall(func() error {
			node = buildChildFromDictEntry(doc, parentID, bareKey, val)
			return nil
		})
		if err != nil {
			node = makeErrorNode(fmt.Sprintf("dict:%s:%s", parentID, bareKey), "/"+bareKey, err)
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func buildChildFromDictEntry(doc *DocumentState, parentID, bareKey string, val pdfcpu_types.Object) *TreeNode {
	if ref, ok := val.(pdfcpu_types.IndirectRef); ok {
		id := fmt.Sprintf("obj:%d:%d", ref.GenerationNumber.Value(), ref.ObjectNumber.Value())
		resolved, label, nodeType, valueType, hasChildren, childCount := resolveRefInfo(doc, ref, bareKey)
		if label == "" {
			label = semanticLabel(bareKey, resolved)
		}
		return &TreeNode{
			ID:          id,
			Label:       label,
			RawKey:      "/" + bareKey,
			NodeType:    nodeType,
			ValueType:   valueType,
			HasChildren: hasChildren,
			ChildCount:  childCount,
			IconHint:    iconHint(bareKey, nodeType, resolved),
		}
	}

	id := fmt.Sprintf("dict:%s:%s", parentID, bareKey)
	return buildTreeNode(id, "/"+bareKey, bareKey, val)
}

func resolveRefInfo(doc *DocumentState, ref pdfcpu_types.IndirectRef, bareKey string) (pdfcpu_types.Object, string, string, string, bool, int) {
	var resolved pdfcpu_types.Object
	err := safeCall(func() error {
		var e error
		resolved, e = doc.PDFContext.Dereference(ref)
		return e
	})
	if err != nil {
		return nil, "", "ref", "reference", true, -1
	}
	if resolved == nil {
		return nil, "", "ref", "reference", true, -1
	}

	nodeType, valueType, hasChildren, childCount := classifyObject(resolved)
	label := semanticLabel(bareKey, resolved)
	return resolved, label, nodeType, valueType, hasChildren, childCount
}

func buildArrayChildren(doc *DocumentState, parentID string, arr pdfcpu_types.Array) []*TreeNode {
	nodes := make([]*TreeNode, 0, len(arr))
	for i, elem := range arr {
		var node *TreeNode
		err := safeCall(func() error {
			if ref, ok := elem.(pdfcpu_types.IndirectRef); ok {
				id := fmt.Sprintf("obj:%d:%d", ref.GenerationNumber.Value(), ref.ObjectNumber.Value())
				resolved, label, nodeType, valueType, hasChildren, childCount := resolveRefInfo(doc, ref, "")
				if label == "" {
					label = fmt.Sprintf("[%d]", i)
				}
				node = &TreeNode{
					ID:          id,
					Label:       label,
					RawKey:      fmt.Sprintf("[%d]", i),
					NodeType:    nodeType,
					ValueType:   valueType,
					HasChildren: hasChildren,
					ChildCount:  childCount,
					IconHint:    iconHint("", nodeType, resolved),
				}
			} else {
				id := fmt.Sprintf("arr:%s:%d", parentID, i)
				node = buildTreeNode(id, fmt.Sprintf("[%d]", i), "", elem)
			}
			return nil
		})
		if err != nil {
			node = makeErrorNode(fmt.Sprintf("arr:%s:%d", parentID, i), fmt.Sprintf("[%d]", i), err)
		}
		nodes = append(nodes, node)
	}
	return nodes
}

func buildTreeNode(id, rawKey, bareKey string, obj pdfcpu_types.Object) *TreeNode {
	nodeType, valueType, hasChildren, childCount := classifyObject(obj)
	label := semanticLabel(bareKey, obj)
	if label == "" {
		label = rawKey
	}
	return &TreeNode{
		ID:          id,
		Label:       label,
		RawKey:      rawKey,
		NodeType:    nodeType,
		ValueType:   valueType,
		HasChildren: hasChildren,
		ChildCount:  childCount,
		IconHint:    iconHint(bareKey, nodeType, obj),
	}
}

func classifyObject(obj pdfcpu_types.Object) (nodeType, valueType string, hasChildren bool, childCount int) {
	switch v := obj.(type) {
	case pdfcpu_types.Dict:
		return "dict", "", true, len(v)
	case pdfcpu_types.StreamDict:
		return "stream", "", true, len(v.Dict)
	case pdfcpu_types.ObjectStreamDict:
		return "stream", "", true, len(v.StreamDict.Dict)
	case pdfcpu_types.XRefStreamDict:
		return "stream", "", true, len(v.StreamDict.Dict)
	case pdfcpu_types.Array:
		return "array", "", true, len(v)
	case pdfcpu_types.IndirectRef:
		return "ref", "reference", true, -1
	case pdfcpu_types.Name:
		return "scalar", "name", false, 0
	case pdfcpu_types.StringLiteral:
		return "scalar", "string", false, 0
	case pdfcpu_types.HexLiteral:
		return "scalar", "string", false, 0
	case pdfcpu_types.Integer:
		return "scalar", "number", false, 0
	case pdfcpu_types.Float:
		return "scalar", "number", false, 0
	case pdfcpu_types.Boolean:
		return "scalar", "boolean", false, 0
	case nil:
		return "scalar", "null", false, 0
	default:
		_ = v
		return "scalar", "", false, 0
	}
}

func semanticLabel(bareKey string, obj pdfcpu_types.Object) string {
	switch bareKey {
	case "Pages":
		return "Pages"
	case "Page":
		return "Page"
	case "Resources":
		return "Resources"
	case "Contents":
		return "Contents"
	case "MediaBox", "CropBox", "BleedBox", "TrimBox", "ArtBox":
		return bareKey
	case "Font":
		return fontLabel(obj)
	case "XObject":
		return "XObject"
	}

	// Check resolved dict for /Type
	if d := asDict(obj); d != nil {
		if typeVal, ok := d["Type"]; ok {
			if name, ok := typeVal.(pdfcpu_types.Name); ok {
				switch string(name) {
				case "Pages":
					return "Pages"
				case "Page":
					return "Page"
				case "Font":
					return fontLabel(obj)
				case "Catalog":
					return "Catalog"
				}
			}
		}
		if subtype, ok := d["Subtype"]; ok {
			if name, ok := subtype.(pdfcpu_types.Name); ok {
				switch string(name) {
				case "Image":
					return "Image"
				case "Form":
					return "Form XObject"
				}
			}
		}
	}

	if bareKey != "" {
		return bareKey
	}
	// Only produce a display label for scalars. Container types (dict, array,
	// stream) return "" so buildTreeNode falls back to rawKey (e.g. "[0]").
	switch obj.(type) {
	case pdfcpu_types.Dict, pdfcpu_types.StreamDict,
		pdfcpu_types.ObjectStreamDict, pdfcpu_types.XRefStreamDict,
		pdfcpu_types.Array:
		return ""
	}
	return scalarDisplay(obj)
}

func fontLabel(obj pdfcpu_types.Object) string {
	if d := asDict(obj); d != nil {
		if bf, ok := d["BaseFont"]; ok {
			if name, ok := bf.(pdfcpu_types.Name); ok {
				return "Font: " + string(name)
			}
		}
	}
	return "Font"
}

func scalarDisplay(obj pdfcpu_types.Object) string {
	switch v := obj.(type) {
	case pdfcpu_types.Name:
		return "/" + string(v)
	case pdfcpu_types.StringLiteral:
		return "(" + string(v) + ")"
	case pdfcpu_types.HexLiteral:
		return "<" + string(v) + ">"
	case pdfcpu_types.Integer:
		return strconv.Itoa(int(v))
	case pdfcpu_types.Float:
		return strconv.FormatFloat(float64(v), 'f', -1, 64)
	case pdfcpu_types.Boolean:
		if bool(v) {
			return "true"
		}
		return "false"
	case nil:
		return "null"
	default:
		return "Unknown"
	}
}

func iconHint(bareKey, nodeType string, obj pdfcpu_types.Object) string {
	switch bareKey {
	case "Pages":
		return "pages"
	case "Page":
		return "page"
	case "Font":
		return "font"
	case "Contents":
		return "stream"
	}

	if d := asDict(obj); d != nil {
		if typeVal, ok := d["Type"]; ok {
			if name, ok := typeVal.(pdfcpu_types.Name); ok {
				switch string(name) {
				case "Pages":
					return "pages"
				case "Page":
					return "page"
				case "Font":
					return "font"
				}
			}
		}
		if subtype, ok := d["Subtype"]; ok {
			if name, ok := subtype.(pdfcpu_types.Name); ok {
				if string(name) == "Image" {
					return "image"
				}
			}
		}
	}

	if nodeType == "stream" {
		return "stream"
	}

	return "default"
}

func asDict(obj pdfcpu_types.Object) pdfcpu_types.Dict {
	switch v := obj.(type) {
	case pdfcpu_types.Dict:
		return v
	case pdfcpu_types.StreamDict:
		return v.Dict
	case pdfcpu_types.ObjectStreamDict:
		return v.StreamDict.Dict
	case pdfcpu_types.XRefStreamDict:
		return v.StreamDict.Dict
	default:
		return nil
	}
}

func parseNodeID(nodeID string) (kind string, parentID string, lastPart string) {
	if nodeID == "root" {
		return "root", "", ""
	}

	idx := strings.Index(nodeID, ":")
	if idx < 0 {
		return nodeID, "", ""
	}
	kind = nodeID[:idx]
	rest := nodeID[idx+1:]

	switch kind {
	case "obj":
		// obj:{gen}:{num} - exactly 2 parts
		parts := strings.SplitN(rest, ":", 2)
		if len(parts) == 2 {
			return kind, parts[0], parts[1]
		}
		return kind, rest, ""
	case "dict", "arr":
		// last segment is the key/index, everything before is parentID
		lastColon := strings.LastIndex(rest, ":")
		if lastColon < 0 {
			return kind, "", rest
		}
		return kind, rest[:lastColon], rest[lastColon+1:]
	default:
		return kind, "", rest
	}
}

// maxNodeIDDepth guards against pathological node ID nesting.
const maxNodeIDDepth = 64

func resolveNodeObject(doc *DocumentState, nodeID string) (pdfcpu_types.Object, error) {
	return resolveNodeObjectDepth(doc, nodeID, 0)
}

func resolveNodeObjectDepth(doc *DocumentState, nodeID string, depth int) (pdfcpu_types.Object, error) {
	if depth > maxNodeIDDepth {
		return nil, fmt.Errorf("node ID nesting too deep (>%d) for %q", maxNodeIDDepth, nodeID)
	}

	kind, parentID, lastPart := parseNodeID(nodeID)

	switch kind {
	case "root":
		rootDict, err := doc.PDFContext.Catalog()
		if err != nil {
			return nil, fmt.Errorf("failed to get catalog: %w", err)
		}
		return rootDict, nil

	case "obj":
		gen, err := strconv.Atoi(parentID)
		if err != nil {
			return nil, fmt.Errorf("invalid gen number in node ID %q: %w", nodeID, err)
		}
		num, err := strconv.Atoi(lastPart)
		if err != nil {
			return nil, fmt.Errorf("invalid obj number in node ID %q: %w", nodeID, err)
		}
		ref := pdfcpu_types.IndirectRef{
			ObjectNumber:     pdfcpu_types.Integer(num),
			GenerationNumber: pdfcpu_types.Integer(gen),
		}
		resolved, err := doc.PDFContext.Dereference(ref)
		if err != nil {
			return nil, fmt.Errorf("failed to dereference obj %d gen %d: %w", num, gen, err)
		}
		return resolved, nil

	case "dict":
		parent, err := resolveNodeObjectDepth(doc, parentID, depth+1)
		if err != nil {
			return nil, err
		}
		d := asDict(parent)
		if d == nil {
			return nil, fmt.Errorf("parent %q is not a dict", parentID)
		}
		val, ok := d[lastPart]
		if !ok {
			return nil, fmt.Errorf("key %q not found in dict %q", lastPart, parentID)
		}
		// If the value is an IndirectRef, dereference it
		if ref, ok := val.(pdfcpu_types.IndirectRef); ok {
			resolved, err := doc.PDFContext.Dereference(ref)
			if err != nil {
				return nil, fmt.Errorf("failed to dereference dict entry %q: %w", lastPart, err)
			}
			return resolved, nil
		}
		return val, nil

	case "arr":
		parent, err := resolveNodeObjectDepth(doc, parentID, depth+1)
		if err != nil {
			return nil, err
		}
		arr, ok := parent.(pdfcpu_types.Array)
		if !ok {
			return nil, fmt.Errorf("parent %q is not an array", parentID)
		}
		idx, err := strconv.Atoi(lastPart)
		if err != nil {
			return nil, fmt.Errorf("invalid array index in node ID %q: %w", nodeID, err)
		}
		if idx < 0 || idx >= len(arr) {
			return nil, fmt.Errorf("array index %d out of range (len=%d)", idx, len(arr))
		}
		elem := arr[idx]
		if ref, ok := elem.(pdfcpu_types.IndirectRef); ok {
			resolved, err := doc.PDFContext.Dereference(ref)
			if err != nil {
				return nil, fmt.Errorf("failed to dereference array element %d: %w", idx, err)
			}
			return resolved, nil
		}
		return elem, nil

	default:
		return nil, fmt.Errorf("unknown node ID kind %q in %q", kind, nodeID)
	}
}

func makeErrorNode(id, rawKey string, err error) *TreeNode {
	return &TreeNode{
		ID:       id,
		Label:    "Error: " + err.Error(),
		RawKey:   rawKey,
		NodeType: "scalar",
		Error:    err.Error(),
		IconHint: "default",
	}
}
