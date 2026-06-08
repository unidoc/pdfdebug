package pdfcore

import (
	"fmt"
	"slices"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// maxResolveDepth is the hard internal ceiling on ref-following depth for
// ResolveRef. It is a DIFFERENT axis from maxNodeIDDepth (which counts node-ID
// structural nesting in parseNodeID): maxResolveDepth counts how many indirect
// references ResolveRef follows inline. It caps ResolveOpts.MaxDepth so a
// caller passing a huge N cannot exhaust the stack on a deep (or adversarial)
// ref chain.
const maxResolveDepth = 32

// ResolveRef resolves the object addressed by nodeID and follows the indirect
// references found inside it inline, up to opts.MaxDepth levels deep, returning
// a ResolvedNode tree. It is the keystone primitive behind --ops Do
// classification, --xobject/--ref stream resolution, --resolve, and Story 11-6.
//
// MaxDepth semantics (ResolveOpts): 0 resolves the addressed object only and
// leaves its child refs as unfollowed markers (Truncated=true); N follows up to
// N ref levels. A negative MaxDepth is clamped to 0. The effective depth is
// additionally capped at maxResolveDepth.
//
// Cycle guarding: a visited set keyed "objNum:gen" tracks the objects on the
// CURRENT resolution path. A ref that re-enters an object already on the path
// is emitted as a Cyclic marker (not recursed into), so an A->B->A or A->A
// graph terminates. The set is path-scoped (entries are popped on unwind) so a
// diamond (two siblings referencing the same object) is NOT mislabeled cyclic.
//
// Locking + safety: acquires doc.pdfMu for the whole walk (pdfcpu is not
// concurrent-read-safe) and wraps every Dereference in safeCall (pdfcpu can
// panic on malformed refs - the target population).
func (ins *Inspector) ResolveRef(tabID, nodeID string, opts ResolveOpts) (*ResolvedNode, error) {
	if nodeID == "" {
		return nil, fmt.Errorf("%w: empty node ID", ErrDocumentNotFound)
	}

	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	maxDepth := opts.MaxDepth
	if maxDepth < 0 {
		maxDepth = 0 // clamp negative to "addressed object only"
	}
	if maxDepth > maxResolveDepth {
		maxDepth = maxResolveDepth
	}

	// Resolve the addressed object itself (the root). resolveNodeObject already
	// dereferences the single addressed leaf, so the root carries the resolved
	// value, not a ref marker.
	var obj pdfcpu_types.Object
	err = safeCall(func() error {
		var e error
		obj, e = resolveNodeObject(doc, nodeID)
		return e
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}

	visited := map[string]bool{}
	if ref := objNodeIDKey(nodeID); ref != "" {
		visited[ref] = true
	}

	root := &ResolvedNode{
		ObjectRef: objectRefFromNodeID(nodeID),
		NodeType:  resolvedNodeType(obj),
		Value:     obj,
	}
	root.Children = resolveChildren(doc, obj, 0, maxDepth, visited)
	return root, nil
}

// objNodeIDKey returns the "objNum:gen" cycle-guard key for an "obj:gen:num"
// node ID, or "" for non-indirect node IDs (root, dict:, arr:). The addressed
// object is seeded into the visited set so a ref that points back at it is
// flagged cyclic.
func objNodeIDKey(nodeID string) string {
	kind, gen, num := parseNodeID(nodeID)
	if kind != "obj" {
		return ""
	}
	return num + ":" + gen
}

// resolveChildren resolves the dict-entry / array-element children of obj. For
// each child that is an indirect ref it either follows it (within depth +
// cycle budget) or emits an unfollowed marker. depth is the number of ref
// levels already followed below the addressed object.
func resolveChildren(doc *DocumentState, obj pdfcpu_types.Object, depth, maxDepth int, visited map[string]bool) []*ResolvedNode {
	if d := asDict(obj); d != nil {
		keys := make([]string, 0, len(d))
		for k := range d {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		children := make([]*ResolvedNode, 0, len(d))
		for _, k := range keys {
			children = append(children, resolveValue(doc, k, d[k], depth, maxDepth, visited))
		}
		return children
	}

	if arr, ok := obj.(pdfcpu_types.Array); ok {
		children := make([]*ResolvedNode, 0, len(arr))
		for i, elem := range arr {
			children = append(children, resolveValue(doc, fmt.Sprintf("[%d]", i), elem, depth, maxDepth, visited))
		}
		return children
	}

	return nil
}

// resolveValue builds a ResolvedNode for one dict-entry/array-element value
// under key. Indirect refs are followed (depth < maxDepth, not on path) or
// marked Truncated/Cyclic. Direct containers recurse at the same depth (no ref
// was crossed); direct scalars are leaves.
func resolveValue(doc *DocumentState, key string, val pdfcpu_types.Object, depth, maxDepth int, visited map[string]bool) *ResolvedNode {
	if ref, ok := val.(pdfcpu_types.IndirectRef); ok {
		num := ref.ObjectNumber.Value()
		gen := ref.GenerationNumber.Value()
		refStr := fmt.Sprintf("%d %d R", num, gen)
		visitedKey := fmt.Sprintf("%d:%d", num, gen)

		// Cycle: re-entering an object already on the current path.
		if visited[visitedKey] {
			return &ResolvedNode{Key: key, ObjectRef: refStr, NodeType: "ref", Cyclic: true, Value: ref}
		}
		// Depth cap: leave the ref unfollowed.
		if depth >= maxDepth {
			return &ResolvedNode{Key: key, ObjectRef: refStr, NodeType: "ref", Truncated: true, Value: ref}
		}

		var resolved pdfcpu_types.Object
		err := safeCall(func() error {
			var e error
			resolved, e = doc.PDFContext.Dereference(ref)
			return e
		})
		if err != nil || resolved == nil {
			// Dangling/broken ref: surface as an unfollowed ref marker rather
			// than crashing or dropping the child.
			return &ResolvedNode{Key: key, ObjectRef: refStr, NodeType: "ref", Truncated: true, Value: ref}
		}

		visited[visitedKey] = true
		node := &ResolvedNode{
			Key:       key,
			ObjectRef: refStr,
			NodeType:  resolvedNodeType(resolved),
			Value:     resolved,
		}
		node.Children = resolveChildren(doc, resolved, depth+1, maxDepth, visited)
		delete(visited, visitedKey) // pop on unwind: path-scoped, not global
		return node
	}

	// Direct value: containers recurse at the same depth (no ref crossed).
	node := &ResolvedNode{
		Key:      key,
		NodeType: resolvedNodeType(val),
		Value:    val,
	}
	switch val.(type) {
	case pdfcpu_types.Dict, pdfcpu_types.StreamDict, pdfcpu_types.ObjectStreamDict,
		pdfcpu_types.XRefStreamDict, pdfcpu_types.Array:
		node.Children = resolveChildren(doc, val, depth, maxDepth, visited)
	}
	return node
}

// GetXObjectResources resolves the /Resources/XObject sub-dictionary of the
// container addressed by nodeID (a page dict or a form-XObject dict) into a map
// of resource name -> XObjectInfo (resolved object ref, node ID, and /Subtype).
// It is the lookup behind --ops Do classification and --xobject NAME stream
// resolution: a Do operand or an --xobject name keys into the returned map.
//
// Returns an empty (non-nil) map when the container has no /Resources/XObject.
// Each entry's indirect ref is dereferenced under safeCall to read /Subtype;
// an entry whose value is not an indirect ref or whose target cannot be read is
// still included (with whatever ObjectRef/Subtype could be determined) rather
// than dropped, so callers can decide how to render it.
func (ins *Inspector) GetXObjectResources(tabID, nodeID string) (map[string]XObjectInfo, error) {
	if nodeID == "" {
		return nil, fmt.Errorf("%w: empty node ID", ErrDocumentNotFound)
	}

	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	var container pdfcpu_types.Object
	err = safeCall(func() error {
		var e error
		container, e = resolveNodeObject(doc, nodeID)
		return e
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}

	out := map[string]XObjectInfo{}
	d := asDict(container)
	if d == nil {
		return out, nil
	}

	xobjDict := derefDictEntry(doc, d, "Resources")
	if xobjDict == nil {
		return out, nil
	}
	resDict := asDict(xobjDict)
	if resDict == nil {
		return out, nil
	}
	xobj := asDict(derefDictEntry(doc, resDict, "XObject"))
	if xobj == nil {
		return out, nil
	}

	for name, val := range xobj {
		info := XObjectInfo{Name: name}
		ref, ok := val.(pdfcpu_types.IndirectRef)
		if !ok {
			out[name] = info
			continue
		}
		num := ref.ObjectNumber.Value()
		gen := ref.GenerationNumber.Value()
		info.ObjectRef = fmt.Sprintf("%d %d R", num, gen)
		info.NodeID = fmt.Sprintf("obj:%d:%d", gen, num)

		var resolved pdfcpu_types.Object
		derefErr := safeCall(func() error {
			var e error
			resolved, e = doc.PDFContext.Dereference(ref)
			return e
		})
		if derefErr == nil && resolved != nil {
			if rd := asDict(resolved); rd != nil {
				if st, ok := rd["Subtype"].(pdfcpu_types.Name); ok {
					info.Subtype = string(st)
				}
			}
		}
		out[name] = info
	}
	return out, nil
}

// derefDictEntry returns d[key], dereferencing it once if it is an indirect
// ref. Returns nil when the key is absent or the deref fails. Caller must hold
// doc.pdfMu.
func derefDictEntry(doc *DocumentState, d pdfcpu_types.Dict, key string) pdfcpu_types.Object {
	val, ok := d[key]
	if !ok {
		return nil
	}
	ref, ok := val.(pdfcpu_types.IndirectRef)
	if !ok {
		return val
	}
	var resolved pdfcpu_types.Object
	err := safeCall(func() error {
		var e error
		resolved, e = doc.PDFContext.Dereference(ref)
		return e
	})
	if err != nil {
		return nil
	}
	return resolved
}

// resolvedNodeType classifies a resolved object for ResolvedNode.NodeType.
func resolvedNodeType(obj pdfcpu_types.Object) string {
	switch obj.(type) {
	case pdfcpu_types.Dict:
		return "dict"
	case pdfcpu_types.StreamDict, pdfcpu_types.ObjectStreamDict, pdfcpu_types.XRefStreamDict:
		return "stream"
	case pdfcpu_types.Array:
		return "array"
	case pdfcpu_types.IndirectRef:
		return "ref"
	default:
		return "scalar"
	}
}
