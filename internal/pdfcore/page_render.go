package pdfcore

import (
	"fmt"
	"slices"

	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// maxFormWalkDepth is the hard internal ceiling on the recursive Form-XObject
// walk, independent of the caller's PageRenderOpts.FormsDepth. It is a backstop
// against an adversarial form tree that is deep but not (yet) cyclic; the
// per-path visited set (onPath) is the primary cycle guard.
const maxFormWalkDepth = 32

// PageRenderInfo assembles the complete per-page rendering picture (Story 11-6):
// resolved geometry (with /Pages inheritance), every ExtGState's blend/alpha/
// SMask, every XObject classified (Form vs Image + colorspace family), and -
// when opts.FormsRecursive is set - each Form XObject walked recursively against
// ITS OWN /Resources.
//
// The recursive walk enumerates the Form XObjects in each form's OWN
// /Resources/XObject dict (the set a form's content stream may Do, since any
// `Do /Name` target must be declared in the current /Resources) - it does NOT
// parse the content stream itself. This can over-report a form that is declared
// in /Resources but never actually invoked by a Do; that is a benign superset.
//
// It is an assembled view built ON TOP of the 11-5 ResolveRef keystone /
// GetXObjectResources: it composes those for per-level dict/ref resolution and
// adds the NEW semantic readers (geometry inheritance, ExtGState/SMask resolver,
// colorspace classifier, recursive form walk). STRUCTURAL ONLY - no rendering
// computation (caution 2).
//
// pageNum is 1-based. An out-of-range / non-positive page returns an error
// whose message contains "not found" (the CLI maps that to exit 2, mirroring
// GetPageNode). A page with no /Resources yields empty (non-nil) arrays, not an
// error.
//
// Locking: acquires doc.pdfMu for the whole walk (pdfcpu is not
// concurrent-read-safe); every Dereference goes through safeCall.
func (ins *Inspector) PageRenderInfo(tabID string, pageNum int, opts PageRenderOpts) (*PageRenderInfo, error) {
	if pageNum < 1 {
		return nil, fmt.Errorf("page %d not found (pages are 1-based)", pageNum)
	}

	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	// PageDict resolves the page dict AND its inherited attributes (MediaBox,
	// CropBox, Rotate, Resources) walking up /Pages ancestors - the classic
	// inheritance gotcha is handled by pdfcpu here (AC1).
	var pageDict pdfcpu_types.Dict
	var indRef *pdfcpu_types.IndirectRef
	var inh *pdfcpu_model.InheritedPageAttrs
	err = safeCall(func() error {
		var e error
		pageDict, indRef, inh, e = doc.PDFContext.PageDict(pageNum, false)
		return e
	})
	if err != nil {
		// pdfcpu's "page not found" surfaces here for out-of-range pages; keep the
		// "not found" wording so the CLI exit-code mapping (AC6) is stable.
		return nil, fmt.Errorf("page %d not found: %w", pageNum, err)
	}
	if pageDict == nil || indRef == nil {
		return nil, fmt.Errorf("page %d not found", pageNum)
	}

	info := &PageRenderInfo{
		Page:    pageNum,
		PageRef: fmt.Sprintf("%d %d R", indRef.ObjectNumber.Value(), indRef.GenerationNumber.Value()),
		Rotate:  inh.Rotate,
	}
	info.MediaBox = rectToSlice(inh.MediaBox)
	info.CropBox = rectToSlice(inh.CropBox)

	// Resolve the page's /Resources. PageDict already resolved the inherited
	// /Resources dict; classify its ExtGState/XObject/Pattern/Shading sub-dicts.
	res := classifyResources(doc, inh.Resources)
	info.ExtGStates = res.extGStates
	info.XObjects = res.xobjects
	info.Patterns = res.patterns
	info.Shadings = res.shadings

	// Optional recursive form walk (AC4). Seed the path-visited set with the page
	// object so a form that references back to the page (degenerate) terminates.
	if opts.FormsRecursive {
		depth := opts.FormsDepth
		if depth < 0 {
			depth = 0
		}
		if depth > maxFormWalkDepth {
			depth = maxFormWalkDepth
		}
		onPath := map[string]bool{}
		if k := refKey(indRef.ObjectNumber.Value(), indRef.GenerationNumber.Value()); k != "" {
			onPath[k] = true
		}
		info.Forms = walkForms(doc, res.formRefs, depth, onPath)
	}

	return info, nil
}

// resourceSummary is the classified content of one /Resources dict: the four
// emitted resource sections plus formRefs (the Form XObjects to recurse into).
type resourceSummary struct {
	extGStates []ExtGStateInfo
	xobjects   []XObjectRenderInfo
	patterns   []PatternInfo
	shadings   []ShadingInfo
	formRefs   []formRef // Form XObjects, for the recursive walk
}

// formRef names a Form XObject reachable from a /Resources dict: the resource
// name it is bound to, its resolved indirect ref string, and the object/
// generation numbers used to re-resolve and cycle-guard it during the recursive
// walk.
type formRef struct {
	name string
	ref  string
	num  int
	gen  int
}

// classifyResources reads a (resolved) /Resources dict and classifies its
// ExtGState / XObject / Pattern / Shading sub-dictionaries into the emitted
// shapes. A nil resources dict yields empty (non-nil) slices (AC6: an absent
// resource is a valid empty result, not an error). Caller must hold doc.pdfMu.
func classifyResources(doc *DocumentState, resources pdfcpu_types.Dict) resourceSummary {
	out := resourceSummary{
		extGStates: []ExtGStateInfo{},
		xobjects:   []XObjectRenderInfo{},
		patterns:   []PatternInfo{},
		shadings:   []ShadingInfo{},
	}
	if resources == nil {
		return out
	}

	if egDict := asDict(derefDictEntry(doc, resources, "ExtGState")); egDict != nil {
		for _, name := range sortedKeys(egDict) {
			out.extGStates = append(out.extGStates, classifyExtGState(doc, name, egDict[name]))
		}
	}

	if xoDict := asDict(derefDictEntry(doc, resources, "XObject")); xoDict != nil {
		for _, name := range sortedKeys(xoDict) {
			xi, fr := classifyXObject(doc, name, xoDict[name])
			out.xobjects = append(out.xobjects, xi)
			if fr != nil {
				out.formRefs = append(out.formRefs, *fr)
			}
		}
	}

	if patDict := asDict(derefDictEntry(doc, resources, "Pattern")); patDict != nil {
		for _, name := range sortedKeys(patDict) {
			out.patterns = append(out.patterns, classifyPattern(doc, name, patDict[name]))
		}
	}

	if shDict := asDict(derefDictEntry(doc, resources, "Shading")); shDict != nil {
		for _, name := range sortedKeys(shDict) {
			out.shadings = append(out.shadings, classifyShading(doc, name, shDict[name]))
		}
	}

	return out
}

// classifyExtGState resolves one /Resources/ExtGState entry into an
// ExtGStateInfo: BM (blend mode), ca/CA (alphas), and SMask (None vs a resolved
// soft-mask descriptor). STRUCTURAL ONLY (AC2/AC7). Caller holds doc.pdfMu.
func classifyExtGState(doc *DocumentState, name string, val pdfcpu_types.Object) ExtGStateInfo {
	info := ExtGStateInfo{Name: name}
	resolved := val
	if ref, ok := val.(pdfcpu_types.IndirectRef); ok {
		info.Ref = refStr(ref.ObjectNumber.Value(), ref.GenerationNumber.Value())
		resolved = derefObject(doc, ref)
	}
	d := asDict(resolved)
	if d == nil {
		return info
	}

	if bm, ok := d["BM"]; ok {
		info.BM = blendModeName(bm)
	}
	if ca, ok := numberValue(d["ca"]); ok {
		info.CA = &ca
	}
	if cA, ok := numberValue(d["CA"]); ok {
		info.Ca = &cA
	}

	if smObj, ok := d["SMask"]; ok {
		info.SMask = classifySMask(doc, smObj)
	}
	return info
}

// blendModeName returns the blend-mode name from a /BM value: a Name renders
// verbatim; the rarely-used /BM array (mode fallback list) renders as its first
// name.
func blendModeName(bm pdfcpu_types.Object) string {
	switch v := bm.(type) {
	case pdfcpu_types.Name:
		return string(v)
	case pdfcpu_types.Array:
		if len(v) > 0 {
			if n, ok := v[0].(pdfcpu_types.Name); ok {
				return string(n)
			}
		}
	}
	return ""
}

// classifySMask resolves an ExtGState /SMask value: the literal /None yields the
// string "None"; a soft-mask dict yields a resolved *SMaskDescriptor (emitted
// inline as the SMask value, AC2). STRUCTURAL ONLY - the mask is described,
// never composited (AC2/AC7). Returns nil for an unresolvable value. Caller
// holds doc.pdfMu.
func classifySMask(doc *DocumentState, sm pdfcpu_types.Object) any {
	resolved := sm
	if ref, ok := sm.(pdfcpu_types.IndirectRef); ok {
		resolved = derefObject(doc, ref)
	}
	if n, ok := resolved.(pdfcpu_types.Name); ok {
		return string(n) // typically /None
	}
	d := asDict(resolved)
	if d == nil {
		return nil
	}
	desc := &SMaskDescriptor{}
	if s, ok := d["S"].(pdfcpu_types.Name); ok {
		desc.S = string(s)
	}
	if g, ok := d["G"].(pdfcpu_types.IndirectRef); ok {
		desc.GRef = refStr(g.ObjectNumber.Value(), g.GenerationNumber.Value())
	}
	if _, ok := d["TR"]; ok {
		desc.HasTR = true
	}
	if bc, ok := d["BC"].(pdfcpu_types.Array); ok {
		desc.BCSize = len(bc)
	}
	return desc
}

// classifyXObject classifies one /Resources/XObject entry (AC3). Returns the
// XObjectRenderInfo and, for a Form XObject, a non-nil formRef so the caller can
// recurse into the form's own resources. Caller holds doc.pdfMu.
func classifyXObject(doc *DocumentState, name string, val pdfcpu_types.Object) (XObjectRenderInfo, *formRef) {
	xi := XObjectRenderInfo{Name: name}
	resolved := val
	var num, gen int
	var haveRef bool
	if ref, ok := val.(pdfcpu_types.IndirectRef); ok {
		num = ref.ObjectNumber.Value()
		gen = ref.GenerationNumber.Value()
		haveRef = true
		xi.Ref = refStr(num, gen)
		resolved = derefObject(doc, ref)
	}
	d := asDict(resolved)
	if d == nil {
		return xi, nil
	}
	if st, ok := d["Subtype"].(pdfcpu_types.Name); ok {
		xi.Subtype = string(st)
	}

	switch xi.Subtype {
	case "Form":
		xi.BBox = numberArray(doc, d["BBox"])
		xi.Matrix = numberArray(doc, d["Matrix"])
		xi.Group = classifyGroup(doc, d["Group"])
		if haveRef {
			return xi, &formRef{
				name: name,
				ref:  xi.Ref,
				num:  num,
				gen:  gen,
			}
		}
	case "Image":
		xi.Width = intValue(doc, d["Width"])
		xi.Height = intValue(doc, d["Height"])
		xi.ColorSpace = classifyColorSpace(doc, d["ColorSpace"])
	}
	return xi, nil
}

// classifyGroup reads a Form XObject's /Group (transparency group) attributes
// (AC3): /S, /CS (classified family), /I, /K. STRUCTURAL ONLY. Caller holds
// doc.pdfMu.
func classifyGroup(doc *DocumentState, val pdfcpu_types.Object) *GroupInfo {
	resolved := val
	if ref, ok := val.(pdfcpu_types.IndirectRef); ok {
		resolved = derefObject(doc, ref)
	}
	d := asDict(resolved)
	if d == nil {
		return nil
	}
	g := &GroupInfo{}
	if s, ok := d["S"].(pdfcpu_types.Name); ok {
		g.S = string(s)
	}
	if cs := classifyColorSpace(doc, d["CS"]); cs != nil {
		g.CS = cs.Family
	}
	if i, ok := d["I"].(pdfcpu_types.Boolean); ok {
		g.I = bool(i)
	}
	if k, ok := d["K"].(pdfcpu_types.Boolean); ok {
		g.K = bool(k)
	}
	return g
}

// maxColorSpaceDepth caps colorspace-alternate recursion (Separation/DeviceN
// /Alternate, ICCBased /Alternate). A crafted file can chain alternates through
// indirect refs back onto themselves; pdfcpu's Dereference resolves one level
// and does NOT break such a cycle, so without this ceiling the classifier would
// recurse until the goroutine stack overflows (a fatal, unrecoverable crash the
// execPageDump recover() cannot catch).
const maxColorSpaceDepth = 16

// classifyColorSpace classifies (does NOT evaluate, AC7) a colorspace value into
// a ColorSpaceInfo. Reuses the Name-or-array-head deref pattern from image.go.
// Families: Device{RGB,Gray,CMYK}, CalRGB/CalGray/Lab, ICCBased (with /N +
// profile size), Indexed (with hival), Separation/DeviceN (with alternate
// family + tint-transform function type), Pattern. Caller holds doc.pdfMu.
func classifyColorSpace(doc *DocumentState, val pdfcpu_types.Object) *ColorSpaceInfo {
	return classifyColorSpaceDepth(doc, val, maxColorSpaceDepth)
}

// classifyColorSpaceDepth is classifyColorSpace with an explicit recursion
// budget. depth <= 0 stops the alternate-colorspace descent so a cyclic
// /Alternate chain terminates instead of overflowing the stack.
func classifyColorSpaceDepth(doc *DocumentState, val pdfcpu_types.Object, depth int) *ColorSpaceInfo {
	resolved := val
	if ref, ok := val.(pdfcpu_types.IndirectRef); ok {
		resolved = derefObject(doc, ref)
	}
	if resolved == nil {
		return nil
	}

	switch cs := resolved.(type) {
	case pdfcpu_types.Name:
		return &ColorSpaceInfo{Family: string(cs)}
	case pdfcpu_types.Array:
		if len(cs) == 0 {
			return nil
		}
		family, ok := cs[0].(pdfcpu_types.Name)
		if !ok {
			return nil
		}
		info := &ColorSpaceInfo{Family: string(family)}
		switch string(family) {
		case "ICCBased":
			classifyICCBased(doc, cs, info, depth)
		case "Indexed":
			// [/Indexed base hival lookup]; hival is element 2.
			if len(cs) >= 3 {
				info.HiVal = intValue(doc, cs[2])
			}
		case "Separation":
			// [/Separation name alternate tintTransform]
			if len(cs) >= 3 && depth > 0 {
				if alt := classifyColorSpaceDepth(doc, cs[2], depth-1); alt != nil {
					info.AltFamily = alt.Family
				}
			}
			if len(cs) >= 4 {
				info.TintTransformType = functionType(doc, cs[3])
			}
		case "DeviceN":
			// [/DeviceN names alternate tintTransform ...]
			if len(cs) >= 3 && depth > 0 {
				if alt := classifyColorSpaceDepth(doc, cs[2], depth-1); alt != nil {
					info.AltFamily = alt.Family
				}
			}
			if len(cs) >= 4 {
				info.TintTransformType = functionType(doc, cs[3])
			}
		}
		return info
	}
	return nil
}

// classifyICCBased fills the /N component count and the ICC profile stream's
// byte size for an [/ICCBased streamRef] colorspace array. The alternate space
// (/Alternate) family is surfaced when present, passing depth-1 so a cyclic
// alternate chain terminates. STRUCTURAL ONLY - the profile bytes are NOT
// parsed. Caller holds doc.pdfMu.
func classifyICCBased(doc *DocumentState, cs pdfcpu_types.Array, info *ColorSpaceInfo, depth int) {
	if len(cs) < 2 {
		return
	}
	ref, ok := cs[1].(pdfcpu_types.IndirectRef)
	if !ok {
		return
	}
	sd := derefStream(doc, ref)
	if sd == nil {
		return
	}
	if n, ok := numberValue(sd.Dict["N"]); ok {
		info.N = int(n)
	}
	if alt, ok := sd.Dict["Alternate"]; ok && depth > 0 {
		if a := classifyColorSpaceDepth(doc, alt, depth-1); a != nil {
			info.AltFamily = a.Family
		}
	}
	// Profile size: prefer the decoded length, else the /Length entry.
	if sd.Content != nil {
		info.ICCProfileSize = len(sd.Content)
	} else if sd.Raw != nil {
		info.ICCProfileSize = len(sd.Raw)
	} else if l, ok := numberValue(sd.Dict["Length"]); ok {
		info.ICCProfileSize = int(l)
	}
}

// functionType returns a PDF function's /FunctionType integer (0 sampled, 2
// exponential, 3 stitching, 4 PostScript). It reads STRUCTURE only - the
// function is never evaluated (AC7). Caller holds doc.pdfMu.
func functionType(doc *DocumentState, val pdfcpu_types.Object) int {
	resolved := val
	if ref, ok := val.(pdfcpu_types.IndirectRef); ok {
		resolved = derefObject(doc, ref)
	}
	d := asDict(resolved)
	if d == nil {
		return 0
	}
	if ft, ok := numberValue(d["FunctionType"]); ok {
		return int(ft)
	}
	return 0
}

// classifyPattern reads a /Resources/Pattern entry: name, ref, /PatternType.
// STRUCTURAL ONLY - no tiling-content walk, no shading-function evaluation
// (AC7). Caller holds doc.pdfMu.
func classifyPattern(doc *DocumentState, name string, val pdfcpu_types.Object) PatternInfo {
	pi := PatternInfo{Name: name}
	resolved := val
	if ref, ok := val.(pdfcpu_types.IndirectRef); ok {
		pi.Ref = refStr(ref.ObjectNumber.Value(), ref.GenerationNumber.Value())
		resolved = derefObject(doc, ref)
	}
	// A shading pattern is a dict; a tiling pattern is a stream (dict accessible
	// via asDict). Both carry /PatternType.
	if d := asDict(resolved); d != nil {
		if pt, ok := numberValue(d["PatternType"]); ok {
			pi.PatternType = int(pt)
		}
	}
	return pi
}

// classifyShading reads a /Resources/Shading entry: name, ref, /ShadingType.
// STRUCTURAL ONLY - no shading-function evaluation (AC7). Caller holds
// doc.pdfMu.
func classifyShading(doc *DocumentState, name string, val pdfcpu_types.Object) ShadingInfo {
	si := ShadingInfo{Name: name}
	resolved := val
	if ref, ok := val.(pdfcpu_types.IndirectRef); ok {
		si.Ref = refStr(ref.ObjectNumber.Value(), ref.GenerationNumber.Value())
		resolved = derefObject(doc, ref)
	}
	if d := asDict(resolved); d != nil {
		if st, ok := numberValue(d["ShadingType"]); ok {
			si.ShadingType = int(st)
		}
	}
	return si
}

// walkForms recursively walks a slice of Form XObjects, resolving each one's OWN
// /Resources (the inner-Fm0-resources gotcha, AC4) and recursing into the Form
// XObjects declared in that /Resources/XObject dict (the forms its content may
// Do; the walk reads the resource dict, not the content stream). onPath is the
// per-path form-object-ref visited set that terminates a self-referential form
// chain (the 11-5 ResolveRef cycle guard covers dict/array ref chains, NOT this
// form-XObject recursion - it is the NEW guard the story calls for). Caller
// holds doc.pdfMu.
//
// depth is the number of form LEVELS to fully expand (classify resources + walk
// nested forms), where the page's direct forms are level 1. A form reached at
// depth <= 1 is LISTED (name + ref) but its /Resources are NOT classified and
// its nested forms are NOT walked - it is marked Truncated. This keeps a form
// below the --forms-depth cap entirely out of the picture (AC4-004: the deeper
// form's ref must not appear at all), while a form AT the cap is fully expanded
// (its own resources, including the names of the forms it calls, are surfaced -
// AC4-002, the own-resources gotcha).
func walkForms(doc *DocumentState, forms []formRef, depth int, onPath map[string]bool) []FormRenderInfo {
	if len(forms) == 0 || depth <= 0 {
		return nil
	}
	out := make([]FormRenderInfo, 0, len(forms))
	for _, fr := range forms {
		node := FormRenderInfo{Name: fr.name, Ref: fr.ref}
		key := refKey(fr.num, fr.gen)

		// Cycle: this form is already on the current walk path (self-reference or a
		// form chain that loops back). Mark and DO NOT recurse.
		if key != "" && onPath[key] {
			node.Cyclic = true
			out = append(out, node)
			continue
		}

		// At the last allowed level: list the form but do NOT classify its resources
		// (which would surface deeper forms' refs) and do NOT recurse. Truncated.
		if depth <= 1 {
			node.Truncated = true
			out = append(out, node)
			continue
		}

		// Resolve this form's OWN /Resources (NOT the page's / outer form's).
		formDict := asDict(derefObject(doc, pdfcpu_types.IndirectRef{
			ObjectNumber:     pdfcpu_types.Integer(fr.num),
			GenerationNumber: pdfcpu_types.Integer(fr.gen),
		}))
		if formDict != nil {
			res := classifyResources(doc, asDict(derefDictEntry(doc, formDict, "Resources")))
			node.ExtGStates = res.extGStates
			node.XObjects = res.xobjects
			node.Patterns = res.patterns
			node.Shadings = res.shadings

			if key != "" {
				onPath[key] = true
			}
			node.Forms = walkForms(doc, res.formRefs, depth-1, onPath)
			if key != "" {
				delete(onPath, key) // pop on unwind: path-scoped, not global
			}
		}
		out = append(out, node)
	}
	return out
}

// --- small helpers ---

// rectToSlice flattens a pdfcpu Rectangle into [llx lly urx ury], or nil when
// the rectangle is absent (an inherited box not present anywhere yields nil).
func rectToSlice(r *pdfcpu_types.Rectangle) []float64 {
	if r == nil {
		return nil
	}
	return []float64{r.LL.X, r.LL.Y, r.UR.X, r.UR.Y}
}

// sortedKeys returns a dict's keys in deterministic sorted order so emitted
// arrays are stable across runs.
func sortedKeys(d pdfcpu_types.Dict) []string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// refStr formats an indirect reference as "<num> <gen> R".
func refStr(num, gen int) string {
	return fmt.Sprintf("%d %d R", num, gen)
}

// refKey is the "num:gen" cycle-guard key.
func refKey(num, gen int) string {
	return fmt.Sprintf("%d:%d", num, gen)
}

// derefObject dereferences an indirect ref under safeCall, returning nil on
// failure (a dangling ref must not crash the assembled view). Caller holds
// doc.pdfMu.
func derefObject(doc *DocumentState, ref pdfcpu_types.IndirectRef) pdfcpu_types.Object {
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

// derefStream dereferences an indirect ref expected to be a StreamDict (the ICC
// profile stream). Returns nil when it is not a stream. Caller holds doc.pdfMu.
func derefStream(doc *DocumentState, ref pdfcpu_types.IndirectRef) *pdfcpu_types.StreamDict {
	resolved := derefObject(doc, ref)
	if sd, ok := resolved.(pdfcpu_types.StreamDict); ok {
		return &sd
	}
	return nil
}

// numberValue extracts a float64 from a PDF Integer or Float, dereferencing is
// NOT done here (callers pass already-direct values or accept a miss). Returns
// (0, false) for any other type so an absent /ca is distinguishable from 0.0.
func numberValue(obj pdfcpu_types.Object) (float64, bool) {
	switch v := obj.(type) {
	case pdfcpu_types.Integer:
		return float64(v.Value()), true
	case pdfcpu_types.Float:
		return v.Value(), true
	}
	return 0, false
}

// intValue extracts an int from a PDF Integer/Float, dereferencing an indirect
// ref once (Width/Height/hival may be indirect). Returns 0 on a miss. Caller
// holds doc.pdfMu.
func intValue(doc *DocumentState, obj pdfcpu_types.Object) int {
	resolved := obj
	if ref, ok := obj.(pdfcpu_types.IndirectRef); ok {
		resolved = derefObject(doc, ref)
	}
	if n, ok := numberValue(resolved); ok {
		return int(n)
	}
	return 0
}

// numberArray flattens a PDF numeric array (e.g. /BBox, /Matrix) into a
// []float64, dereferencing the array itself once if indirect. Returns nil for a
// non-array. Caller holds doc.pdfMu.
func numberArray(doc *DocumentState, obj pdfcpu_types.Object) []float64 {
	resolved := obj
	if ref, ok := obj.(pdfcpu_types.IndirectRef); ok {
		resolved = derefObject(doc, ref)
	}
	arr, ok := resolved.(pdfcpu_types.Array)
	if !ok {
		return nil
	}
	out := make([]float64, 0, len(arr))
	for _, e := range arr {
		if n, ok := numberValue(e); ok {
			out = append(out, n)
		}
	}
	return out
}
