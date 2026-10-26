package pdfcore

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// maxStructTreeDepth caps how many elements one descent path of the
// structure tree may hold. A deeper element becomes one error kid, so a
// pathological chain cannot exhaust the goroutine stack.
const maxStructTreeDepth = 256

// noPageMsg is the error on an MCID or MCR kid with no page to attribute it to.
const noPageMsg = "no page: the nearest /Pg (on the kid, its element or an ancestor) is absent or not a page; MCID not attributed to a page"

// rootMCIDMsg is the error on an MCID or MCR kid listed directly in the
// root's /K, where no element owns it.
const rootMCIDMsg = "MCID directly under the structure tree root, with no element to own it; not aligned"

// formXObjectNote is the note on a ParentTree array value no page's
// /StructParents names: a Form XObject with marked content has its own
// /StructParents and an array value (ISO 32000-1 14.7.4.4).
const formXObjectNote = "array value no page's /StructParents names; may belong to a Form XObject's /StructParents"

// directElement stands for a direct-dictionary element where a ref is shown.
const directElement = "(direct)"

// errStructureCanceled is returned when the document is closed during
// GetStructure. It satisfies errors.Is(err, context.Canceled).
var errStructureCanceled = fmt.Errorf("structure walk canceled: %w", context.Canceled)

// GetStructure returns the tagged-structure report for the document in tabID:
// the structure tree from the catalog's /StructTreeRoot and, for every
// numbered page, its ParentTree array, the marked content of its content
// streams and the per-MCID alignment of the two with the tree. Anything
// malformed in the document is a finding in the report; the call fails only
// for an unknown tab, a close during the walk (errors.Is context.Canceled) or
// a pdfcpu failure outside every per-node guard. Not cached. Every content
// stream of every page is read: from the stream cache when GetContentStream
// already decoded it, else decoded here without writing the cache.
func (ins *Inspector) GetStructure(tabID string) (*StructureReport, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	return doc.buildStructure(newStructureWalker(doc.PDFContext))
}

// structureWalker carries the state of one GetStructure walk.
type structureWalker struct {
	ctx *pdfcpu_model.Context
	// beforeLock, when set, runs just before pdfMu is taken: with 0 before
	// the tree walk, then with each page number. Tests use it to close the
	// document at a known point.
	beforeLock func(page int)

	report *StructureReport
	// pageNums maps a page object number to the first page number it got in
	// the page-tree walk.
	pageNums map[int]int
	roleMap  map[string]string
	// visited holds the object numbers of every element and indirect /K
	// array already walked.
	visited map[int]bool
	// claims lists, per page number and MCID, the elements whose /K names
	// it, in walk order.
	claims map[int]map[int][]string
	// parentTree holds the ParentTree entries by key.
	parentTree map[int]pdfcpu_types.Object
	// streamUses counts, per content stream node ID, the page scans still to
	// come that read it.
	streamUses map[string]int
	// streamMemo holds the decoded tokens of a content stream that a later
	// page scan still reads, so a shared stream is decoded once per walk.
	// An entry is dropped after its last use.
	streamMemo map[string]decodedStream
	// decodes counts content stream decodes.
	decodes int
}

// decodedStream is one content stream's tokens, or the error that stopped
// its decode.
type decodedStream struct {
	tokens []Token
	err    string
}

func newStructureWalker(ctx *pdfcpu_model.Context) *structureWalker {
	return &structureWalker{
		ctx: ctx,
		report: &StructureReport{
			RoleMap:         []RoleMapEntry{},
			Elements:        []*StructKid{},
			Pages:           []*StructPage{},
			OtherParentTree: []ParentTreeEntry{},
			Problems:        []StructureProblem{},
		},
		pageNums:   map[int]int{},
		roleMap:    map[string]string{},
		visited:    map[int]bool{},
		claims:     map[int]map[int][]string{},
		parentTree: map[int]pdfcpu_types.Object{},
		streamUses: map[string]int{},
		streamMemo: map[string]decodedStream{},
	}
}

// buildStructure runs w over d. Phase 1 holds pdfMu once to read the page
// walk, the structure tree, the ParentTree and every page's /StructParents.
// Phase 2 takes pdfMu per page to decode and scan its content streams, and
// checks for a close of d before every page.
func (d *DocumentState) buildStructure(w *structureWalker) (*StructureReport, error) {
	if w.beforeLock != nil {
		w.beforeLock(0)
	}
	if docClosed(d) {
		return nil, errStructureCanceled
	}
	leaves, err := d.structurePhase1(w)
	if err != nil {
		return nil, err
	}
	for i, leaf := range leaves {
		page := i + 1
		if w.beforeLock != nil {
			w.beforeLock(page)
		}
		if docClosed(d) {
			return nil, errStructureCanceled
		}
		sp := w.report.Pages[i]
		d.scanPageLocked(w, leaf, sp)
		sp.Alignment = w.align(sp)
	}
	return w.report, nil
}

// structurePhase1 reads everything but the content streams under one pdfMu
// hold and returns the numbered pages.
func (d *DocumentState) structurePhase1(w *structureWalker) ([]*pageLeaf, error) {
	d.pdfMu.Lock()
	defer d.pdfMu.Unlock()
	var leaves []*pageLeaf
	err := safeCall(func() error {
		pt, ptErr := d.pageTree()
		leaves = pt.leaves
		w.readTree(leaves)
		w.countStreamUses(leaves)
		switch {
		case ptErr != nil:
			msg := fmt.Sprintf("page-tree walk stopped after %d pages: %v; later pages not listed", len(leaves), ptErr)
			w.problem("page tree", msg)
			w.report.pagesIncomplete = msg
		case pt.skipped != "":
			msg := "page-tree subtree not walked: " + pt.skipped + "; its pages not listed"
			w.problem("page tree", msg)
			w.report.pagesIncomplete = msg
		}
		return nil
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}
	return leaves, nil
}

// scanPageLocked decodes and scans one page's content streams with pdfMu
// held.
func (d *DocumentState) scanPageLocked(w *structureWalker, leaf *pageLeaf, sp *StructPage) {
	d.pdfMu.Lock()
	defer d.pdfMu.Unlock()
	guard(func() { w.scanPage(d, leaf, sp) }, func(msg string) {
		sp.Err = joinErr(sp.Err, "page could not be read: "+msg)
	})
}

// countStreamUses counts how many page scans read each content stream.
func (w *structureWalker) countStreamUses(leaves []*pageLeaf) {
	for _, leaf := range leaves {
		if leaf.err != nil {
			continue
		}
		refs, _ := contentStreamRefs(w.ctx, leaf.dict)
		for _, ref := range refs {
			w.streamUses[nodeIDForRef(ref)]++
		}
	}
}

// problem records a finding with no row of its own.
func (w *structureWalker) problem(where, msg string) {
	w.report.Problems = append(w.report.Problems, StructureProblem{Where: where, Message: msg})
}

// deref resolves o, returning nil for a dangling reference.
func (w *structureWalker) deref(o pdfcpu_types.Object) pdfcpu_types.Object {
	return derefIn(w.ctx, o)
}

// readTree fills the report from the catalog, the structure tree root and the
// page leaves.
func (w *structureWalker) readTree(leaves []*pageLeaf) {
	for i, leaf := range leaves {
		if leaf.ref != nil {
			n := leaf.ref.ObjectNumber.Value()
			if _, seen := w.pageNums[n]; !seen {
				w.pageNums[n] = i + 1
			}
		}
	}

	catalog, err := w.ctx.Catalog()
	if err != nil || catalog == nil {
		w.problem("catalog", "the catalog could not be read")
	} else {
		w.readCatalog(catalog)
	}

	// named maps each /StructParents key to the first page that has it.
	named := map[int]int{}
	for i, leaf := range leaves {
		sp := &StructPage{
			PageNum:      i + 1,
			ParentTree:   []ParentTreeSlot{},
			Streams:      []MarkedStream{},
			ContentMCIDs: []ContentMCID{},
			Alignment:    []MCIDAlignment{},
		}
		if leaf.ref != nil {
			sp.Ref = refString(*leaf.ref)
			sp.nodeID = nodeIDForRef(*leaf.ref)
		}
		w.report.Pages = append(w.report.Pages, sp)
		if leaf.err != nil {
			sp.Err = leaf.err.Error()
			continue
		}
		guard(func() { w.readPageParentTree(leaf, sp) }, func(msg string) {
			sp.ParentTreeErr = "the ParentTree entry could not be read: " + msg
		})
		if sp.StructParents != nil {
			key := *sp.StructParents
			if first, dup := named[key]; dup {
				w.problem(fmt.Sprintf("page %d", sp.PageNum), fmt.Sprintf("/StructParents %d is also the key of page %d; both pages bind to the same ParentTree entry", key, first))
			} else {
				named[key] = sp.PageNum
			}
		}
	}
	w.readOtherParentTree(named)
}

// readCatalog reads the document-level entries and walks the structure tree.
func (w *structureWalker) readCatalog(catalog pdfcpu_types.Dict) {
	r := w.report
	if o, found := catalog.Find("Lang"); found {
		v := w.deref(o)
		s, ok := decodeTextStringOK(v)
		if !ok || !utf8.ValidString(s) {
			s = textStringOrRaw(v)
			w.problem("catalog", fmt.Sprintf("/Lang is %s that does not decode as a text string", objKind(v)))
		}
		r.DocLang = s
	}
	if o, found := catalog.Find("MarkInfo"); found {
		mi, ok := w.deref(o).(pdfcpu_types.Dict)
		if !ok {
			w.problem("catalog", fmt.Sprintf("/MarkInfo is %s, not a dictionary", objKind(w.deref(o))))
		} else if mo, found := mi.Find("Marked"); found {
			if b, ok := w.deref(mo).(pdfcpu_types.Boolean); ok {
				v := b.Value()
				r.Marked = &v
			} else {
				w.problem("catalog", fmt.Sprintf("/MarkInfo /Marked is %s, not a boolean", objKind(w.deref(mo))))
			}
		}
	}

	rootObj, found := catalog.Find("StructTreeRoot")
	if !found || rootObj == nil {
		return
	}
	// A reference to a missing object is null (ISO 32000-1 7.3.10), so the
	// root is absent.
	if ref, ok := rootObj.(pdfcpu_types.IndirectRef); ok && w.deref(rootObj) == nil {
		w.problem("catalog", fmt.Sprintf("/StructTreeRoot %s does not resolve to an object; treated as absent", refString(ref)))
		return
	}
	r.HasStructTree = true
	where := "StructTreeRoot"
	if ref, ok := rootObj.(pdfcpu_types.IndirectRef); ok {
		r.RootRef = refString(ref)
		where += " " + r.RootRef
	}
	root, ok := w.deref(rootObj).(pdfcpu_types.Dict)
	if !ok {
		msg := "/StructTreeRoot is " + objKind(w.deref(rootObj)) + ", not a dictionary"
		w.problem(where, msg)
		r.rootNotDict = msg
		return
	}
	guard(func() { w.readRoot(root, where) }, func(msg string) {
		msg = "the structure tree root could not be read: " + msg
		w.problem(where, msg)
		r.rootReadFailed = msg
	})
}

// readRoot reads /RoleMap, /ParentTreeNextKey and /ParentTree, then walks
// the root's /K.
func (w *structureWalker) readRoot(root pdfcpu_types.Dict, where string) {
	r := w.report
	if o, found := root.Find("RoleMap"); found {
		if rm, ok := w.deref(o).(pdfcpu_types.Dict); ok {
			keys := make([]string, 0, len(rm))
			for k := range rm {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				v := w.deref(rm[k])
				to := nameOf(v)
				if _, isName := v.(pdfcpu_types.Name); !isName {
					w.problem(where, fmt.Sprintf("/RoleMap /%s maps to %s, not a name", k, objKind(v)))
				} else {
					w.roleMap[k] = to
				}
				r.RoleMap = append(r.RoleMap, RoleMapEntry{From: k, To: to})
			}
		} else {
			w.problem(where, fmt.Sprintf("/RoleMap is %s, not a dictionary", objKind(w.deref(o))))
		}
	}
	if o, found := root.Find("ParentTreeNextKey"); found {
		if n, ok := w.deref(o).(pdfcpu_types.Integer); ok {
			v := int(n)
			r.ParentTreeNextKey = &v
		} else {
			w.problem(where, fmt.Sprintf("/ParentTreeNextKey is %s, not an integer", objKind(w.deref(o))))
		}
	}
	if o, found := root.Find("ParentTree"); found && o != nil {
		r.HasParentTree = true
		ptWhere := "ParentTree"
		if ref, ok := o.(pdfcpu_types.IndirectRef); ok {
			r.ParentTreeRef = refString(ref)
			ptWhere += " " + r.ParentTreeRef
		}
		entries, problems, incomplete := walkNumberTree(w.ctx, o, ptWhere)
		w.parentTree = entries
		r.Problems = append(r.Problems, problems...)
		r.parentTreeIncomplete = incomplete
	}
	r.Elements = w.walkK(root["K"], "", 0, 0)
}

// walkK returns the kids of one /K value. owner is the claiming element's ref
// ("" for the root), page its effective page, depth its depth (0 for the
// root).
func (w *structureWalker) walkK(k pdfcpu_types.Object, owner string, page, depth int) []*StructKid {
	kids := []*StructKid{}
	if k == nil {
		return kids
	}
	items := []pdfcpu_types.Object{k}
	if arr, ok := k.(pdfcpu_types.Array); ok {
		items = arr
	} else if ref, ok := k.(pdfcpu_types.IndirectRef); ok {
		if arr, ok := w.deref(ref).(pdfcpu_types.Array); ok {
			// An indirect /K array shared by several elements is walked once,
			// so its kids cannot be listed once per element that names it.
			n := ref.ObjectNumber.Value()
			if w.visited[n] {
				return append(kids, w.errorKid(fmt.Sprintf("/K array %s already listed; not walked again", refString(ref))))
			}
			w.visited[n] = true
			items = arr
		}
	}
	for i, item := range items {
		if item == nil {
			continue
		}
		kid := w.kid(item, owner, page, depth)
		kid.index = i
		kids = append(kids, kid)
	}
	return kids
}

// kid classifies one /K entry.
func (w *structureWalker) kid(raw pdfcpu_types.Object, owner string, page, depth int) *StructKid {
	c := &w.report.Counts
	var ref *pdfcpu_types.IndirectRef
	if r, ok := raw.(pdfcpu_types.IndirectRef); ok {
		ref = &r
	}
	obj := w.deref(raw)
	if obj == nil && ref != nil {
		return w.errorKid(refString(*ref) + " does not resolve to an object")
	}
	switch v := obj.(type) {
	case pdfcpu_types.Integer:
		if v < 0 {
			return w.errorKid(fmt.Sprintf("MCID %d is negative", v))
		}
		c.MCIDKids++
		mcid := int(v)
		w.claim(owner, page, mcid)
		k := &StructKid{Kind: "mcid", MCID: &mcid, Page: page, PageInherited: page != 0}
		switch {
		case owner == "":
			k.Err = rootMCIDMsg
		case page == 0:
			k.Err = noPageMsg
		}
		return k
	case pdfcpu_types.Dict:
		switch nameOf(w.deref(v["Type"])) {
		case "MCR":
			c.MCRKids++
			return w.mcrKid(v, owner, page)
		case "OBJR":
			c.OBJRKids++
			return w.objrKid(v, page)
		}
		return w.elementKid(v, ref, page, depth+1)
	}
	return w.errorKid(fmt.Sprintf("kid is %s, not an element, MCID, MCR or OBJR", objKind(obj)))
}

// errorKid returns a kind "error" kid carrying msg.
func (w *structureWalker) errorKid(msg string) *StructKid {
	w.report.Counts.ErrorKids++
	return &StructKid{Kind: "error", Err: msg}
}

// claim records that owner's /K names mcid on page.
func (w *structureWalker) claim(owner string, page, mcid int) {
	if owner == "" || page == 0 {
		return
	}
	byMCID := w.claims[page]
	if byMCID == nil {
		byMCID = map[int][]string{}
		w.claims[page] = byMCID
	}
	byMCID[mcid] = append(byMCID[mcid], owner)
}

// kidPage returns the page of an MCR or OBJR kid: its own /Pg when present,
// else the owning element's page. errMsg is set when an own /Pg is not a page.
func (w *structureWalker) kidPage(d pdfcpu_types.Dict, page int) (int, bool, string) {
	pg, found := d.Find("Pg")
	if !found {
		return page, page != 0, ""
	}
	n, _, msg := w.pageOf(pg)
	return n, false, msg
}

// mcrKid reads a marked-content reference dictionary.
func (w *structureWalker) mcrKid(d pdfcpu_types.Dict, owner string, page int) *StructKid {
	k := &StructKid{Kind: "mcr"}
	var errs []string
	var msg string
	k.Page, k.PageInherited, msg = w.kidPage(d, page)
	errs = appendErr(errs, msg)
	if stm, found := d.Find("Stm"); found {
		ref, isRef := stm.(pdfcpu_types.IndirectRef)
		switch {
		case !isRef:
			errs = append(errs, fmt.Sprintf("/Stm is %s, not a reference to a stream", objKind(stm)))
			k.StmRef = directElement
		default:
			k.StmRef = refString(ref)
			if _, isStream := w.deref(ref).(pdfcpu_types.StreamDict); !isStream {
				errs = append(errs, fmt.Sprintf("/Stm %s is %s, not a stream", k.StmRef, objKind(w.deref(ref))))
			}
		}
	}
	mcidObj, found := d.Find("MCID")
	switch n, isInt := w.deref(mcidObj).(pdfcpu_types.Integer); {
	case !found:
		errs = append(errs, "MCR has no /MCID")
	case !isInt:
		errs = append(errs, fmt.Sprintf("MCR /MCID is %s, not an integer", objKind(w.deref(mcidObj))))
	case n < 0:
		errs = append(errs, fmt.Sprintf("MCR /MCID %d is negative", n))
	default:
		mcid := int(n)
		k.MCID = &mcid
		switch {
		case k.StmRef != "":
		case owner == "":
			errs = append(errs, rootMCIDMsg)
		default:
			w.claim(owner, k.Page, mcid)
			if k.Page == 0 && msg == "" {
				errs = append(errs, noPageMsg)
			}
		}
	}
	k.Err = strings.Join(errs, "; ")
	return k
}

// objrKid reads an object reference dictionary.
func (w *structureWalker) objrKid(d pdfcpu_types.Dict, page int) *StructKid {
	k := &StructKid{Kind: "objr"}
	var errs []string
	var msg string
	k.Page, k.PageInherited, msg = w.kidPage(d, page)
	errs = appendErr(errs, msg)
	objRaw, found := d.Find("Obj")
	ref, isRef := objRaw.(pdfcpu_types.IndirectRef)
	switch {
	case !found:
		errs = append(errs, "OBJR has no /Obj")
	case !isRef:
		errs = append(errs, fmt.Sprintf("OBJR /Obj is %s, not a reference", objKind(objRaw)))
	default:
		k.ObjRef = refString(ref)
		target := w.deref(ref)
		var td pdfcpu_types.Dict
		switch t := target.(type) {
		case pdfcpu_types.Dict:
			td = t
		case pdfcpu_types.StreamDict:
			td = t.Dict
		case nil:
			errs = append(errs, fmt.Sprintf("OBJR /Obj %s does not resolve to an object", k.ObjRef))
		default:
			errs = append(errs, fmt.Sprintf("OBJR /Obj %s is %s, not a dictionary or stream", k.ObjRef, objKind(target)))
		}
		if td != nil {
			var parts []string
			for _, key := range []string{"Type", "Subtype"} {
				if s := nameOf(w.deref(td[key])); s != "" {
					parts = append(parts, s)
				}
			}
			k.ObjType = strings.Join(parts, "/")
		}
	}
	k.Err = strings.Join(errs, "; ")
	return k
}

// elementKid walks one structure element at depth (1 = a kid of the root).
// parentPage is the effective page of the nearest ancestor.
func (w *structureWalker) elementKid(d pdfcpu_types.Dict, ref *pdfcpu_types.IndirectRef, parentPage, depth int) *StructKid {
	if depth > maxStructTreeDepth {
		return w.errorKid(fmt.Sprintf("structure tree deeper than %d levels; subtree not walked", maxStructTreeDepth))
	}
	e := &StructElem{Kids: []*StructKid{}}
	id := directElement
	if ref != nil {
		n := ref.ObjectNumber.Value()
		if w.visited[n] {
			return w.errorKid(fmt.Sprintf("element %s already listed; not walked again", refString(*ref)))
		}
		w.visited[n] = true
		e.Ref = refString(*ref)
		e.NodeID = nodeIDForRef(*ref)
		id = e.Ref
	}
	w.report.Counts.Elements++
	guard(func() { w.readElement(d, e, id, parentPage, depth) }, func(msg string) {
		e.readErr = msg
		e.Err = joinErr(e.Err, "element could not be read: "+msg)
	})
	return &StructKid{Kind: "element", Element: e, Page: e.Page, PageInherited: e.PageInherited}
}

// readElement fills e from d and walks its kids.
func (w *structureWalker) readElement(d pdfcpu_types.Dict, e *StructElem, id string, parentPage, depth int) {
	var errs []string
	sObj, hasS := d.Find("S")
	sv := w.deref(sObj)
	switch s, isName := sv.(pdfcpu_types.Name); {
	case !hasS || sv == nil:
		e.sErr = "no /S"
	case !isName:
		e.sErr = fmt.Sprintf("/S is %s, not a name", objKind(sv))
	case s.Value() == "":
		e.sErr = "/S is an empty name"
	default:
		e.S = s.Value()
	}
	errs = appendErr(errs, e.sErr)
	if typ := nameOf(w.deref(d["Type"])); typ != "" && typ != "StructElem" {
		errs = append(errs, fmt.Sprintf("/Type /%s is not StructElem", typ))
	}
	if e.S != "" {
		mapped, msg := w.mapRole(e.S)
		e.MappedS = mapped
		errs = appendErr(errs, msg)
	}

	if pg, found := d.Find("Pg"); found {
		var msg string
		e.Page, e.PgRef, msg = w.pageOf(pg)
		errs = appendErr(errs, msg)
	} else {
		e.Page = parentPage
		e.PageInherited = parentPage != 0
	}

	for _, f := range []struct {
		key string
		dst **string
	}{{"Alt", &e.Alt}, {"ActualText", &e.ActualText}, {"E", &e.E}} {
		if o, found := d.Find(f.key); found {
			s, msg := w.textField(f.key, o)
			*f.dst = &s
			errs = appendErr(errs, msg)
		}
	}
	for _, f := range []struct {
		key string
		dst *string
	}{{"T", &e.Title}, {"Lang", &e.Lang}} {
		if o, found := d.Find(f.key); found {
			s, msg := w.textField(f.key, o)
			*f.dst = s
			errs = appendErr(errs, msg)
		}
	}
	e.Err = strings.Join(errs, "; ")

	e.Kids = w.walkK(d["K"], id, e.Page, depth)
}

// mapRole follows s through /RoleMap until a name with no mapping. A cycle or
// a mapping to the empty name stops the walk and is returned as msg with
// mapped "".
func (w *structureWalker) mapRole(s string) (mapped, msg string) {
	seen := map[string]bool{s: true}
	path := []string{s}
	cur := s
	for {
		to, ok := w.roleMap[cur]
		if !ok {
			break
		}
		if to == "" {
			return "", fmt.Sprintf("/RoleMap maps /%s to an empty name", cur)
		}
		path = append(path, to)
		if seen[to] {
			return "", "/RoleMap cycle: " + strings.Join(path, " -> ")
		}
		seen[to] = true
		cur = to
	}
	if cur == s {
		return "", ""
	}
	return cur, ""
}

// pageOf resolves a /Pg value to a page number. pgRef is its "N G R" (or
// "(direct)"); msg is set when it does not name a numbered page.
func (w *structureWalker) pageOf(pg pdfcpu_types.Object) (page int, pgRef, msg string) {
	ref, ok := pg.(pdfcpu_types.IndirectRef)
	if !ok {
		return 0, directElement, fmt.Sprintf("/Pg is %s, not a reference to a page", objKind(pg))
	}
	pgRef = refString(ref)
	if n, found := w.pageNums[ref.ObjectNumber.Value()]; found {
		return n, pgRef, ""
	}
	target := w.deref(ref)
	if target == nil {
		return 0, pgRef, fmt.Sprintf("/Pg %s does not resolve to an object", pgRef)
	}
	return 0, pgRef, fmt.Sprintf("/Pg %s is %s that is not a page in the page tree", pgRef, objKind(target))
}

// textField decodes a text-string entry; a value that does not decode is
// shown raw with msg set.
func (w *structureWalker) textField(key string, o pdfcpu_types.Object) (string, string) {
	v := w.deref(o)
	if s, ok := decodeTextStringOK(v); ok && utf8.ValidString(s) {
		return s, ""
	}
	return textStringOrRaw(v), fmt.Sprintf("/%s is %s that does not decode as a text string", key, objKind(v))
}

// readPageParentTree reads a page's /StructParents and its ParentTree array.
func (w *structureWalker) readPageParentTree(leaf *pageLeaf, sp *StructPage) {
	o, found := leaf.dict.Find("StructParents")
	if !found {
		return
	}
	n, ok := w.deref(o).(pdfcpu_types.Integer)
	if !ok {
		sp.ParentTreeErr = fmt.Sprintf("/StructParents is %s, not an integer", objKind(w.deref(o)))
		return
	}
	key := int(n)
	sp.StructParents = &key
	if !w.report.HasParentTree {
		sp.ParentTreeErr = fmt.Sprintf("/StructParents %d but the document has no ParentTree", key)
		return
	}
	v, ok := w.parentTree[key]
	if !ok {
		sp.ParentTreeErr = fmt.Sprintf("ParentTree has no key %d", key)
		return
	}
	arr, ok := w.deref(v).(pdfcpu_types.Array)
	if !ok {
		sp.ParentTreeErr = fmt.Sprintf("ParentTree key %d is %s, not an array", key, objKind(w.deref(v)))
		return
	}
	for i, slot := range arr {
		s := ParentTreeSlot{MCID: i}
		if slot != nil {
			s.ElementRef, s.S, s.Err = w.elementRefOf(slot)
		}
		sp.ParentTree = append(sp.ParentTree, s)
	}
}

// elementRefOf reads a ParentTree value that should be a reference to an
// element: its ref, its /S and an error when it is not one.
func (w *structureWalker) elementRefOf(o pdfcpu_types.Object) (ref, s, msg string) {
	r, ok := o.(pdfcpu_types.IndirectRef)
	if !ok {
		return "", "", fmt.Sprintf("value is %s, not a reference to an element", objKind(o))
	}
	ref = refString(r)
	target := w.deref(r)
	d, ok := target.(pdfcpu_types.Dict)
	if !ok {
		if target == nil {
			return ref, "", ref + " does not resolve to an object"
		}
		return ref, "", fmt.Sprintf("%s is %s, not an element", ref, objKind(target))
	}
	return ref, nameOf(w.deref(d["S"])), ""
}

// readOtherParentTree lists the ParentTree keys no page names.
func (w *structureWalker) readOtherParentTree(named map[int]int) {
	keys := make([]int, 0, len(w.parentTree))
	for k := range w.parentTree {
		if _, isNamed := named[k]; !isNamed {
			keys = append(keys, k)
		}
	}
	sort.Ints(keys)
	for _, k := range keys {
		e := ParentTreeEntry{Key: k}
		v := w.parentTree[k]
		if _, isArr := w.deref(v).(pdfcpu_types.Array); isArr {
			e.Note = formXObjectNote
		} else {
			e.ElementRef, e.S, e.Err = w.elementRefOf(v)
		}
		w.report.OtherParentTree = append(w.report.OtherParentTree, e)
	}
}

// scanPage decodes the page's content streams in /Contents order and scans
// their marked content. A stream that a later page also reads is decoded once
// and its tokens kept until that page's scan. Callers hold pdfMu.
func (w *structureWalker) scanPage(d *DocumentState, leaf *pageLeaf, sp *StructPage) {
	if leaf.err != nil {
		return
	}
	refs, err := contentStreamRefs(w.ctx, leaf.dict)
	if err != nil {
		sp.Err = joinErr(sp.Err, err.Error())
		return
	}
	props := w.propertyLookup(leaf)
	st := &mcState{}
	for i, ref := range refs {
		ms := MarkedStream{Ref: refString(ref), nodeID: nodeIDForRef(ref)}
		ds := w.decodeStream(d, nodeIDForRef(ref))
		ms.Err = ds.err
		if ms.Err != "" {
			ms.DepthAtEnd, ms.MinDepth = len(st.stack), len(st.stack)
			// The stream's content is unknown, so operands read before it
			// do not carry past it.
			st.clearOperands()
		} else {
			rows := scanMarkedContent(ds.tokens, i, props, st, &ms)
			sp.ContentMCIDs = append(sp.ContentMCIDs, rows...)
		}
		sp.Streams = append(sp.Streams, ms)
	}
	for _, c := range sp.ContentMCIDs {
		if c.MCID >= 0 {
			w.report.Counts.ContentMCIDs++
		}
	}
}

// decodeStream returns the tokens of the content stream id for one use by a
// page scan: from streamMemo when an earlier page decoded it, else from an
// entry GetContentStream already put in d's stream cache, else by decoding
// it. The stream cache is read, never written. Callers hold pdfMu, so
// streamMu is taken inside it, the order GetContentStream uses.
func (w *structureWalker) decodeStream(d *DocumentState, id string) decodedStream {
	ds, memo := w.streamMemo[id]
	if !memo {
		d.streamMu.Lock()
		cached, hit := d.streamCache[id]
		d.streamMu.Unlock()
		memo = hit
		if hit {
			ds = decodedStream{tokens: cached.Tokenized, err: cached.Error}
		}
	}
	if !memo {
		w.decodes++
		raw, decodeErr, err := d.decodeContentStreamLocked(id)
		switch {
		case err != nil:
			ds.err = err.Error()
		case decodeErr != "":
			ds.err = decodeErr
		default:
			ds.tokens = tokenizeContentStream(raw)
		}
	}
	w.streamUses[id]--
	if w.streamUses[id] > 0 {
		w.streamMemo[id] = ds
	} else {
		delete(w.streamMemo, id)
		delete(w.streamUses, id)
	}
	return ds
}

// propertyLookup returns the named-property-list lookup for a page: its
// effective /Resources /Properties, each level dereferenced once here.
// Callers hold pdfMu, and so does every call of the returned lookup.
func (w *structureWalker) propertyLookup(leaf *pageLeaf) propertyLookup {
	var props pdfcpu_types.Dict
	if res, ok := w.deref(leaf.attrs.resources).(pdfcpu_types.Dict); ok {
		props, _ = w.deref(res["Properties"]).(pdfcpu_types.Dict)
	}
	return func(name string) (pdfcpu_types.Object, error) {
		entry, found := props.Find(name)
		if !found {
			return nil, fmt.Errorf("property list /%s not found in /Resources /Properties", name)
		}
		pl, ok := w.deref(entry).(pdfcpu_types.Dict)
		if !ok {
			return nil, fmt.Errorf("property list /%s is %s, not a dictionary", name, objKind(w.deref(entry)))
		}
		mcid, found := pl.Find("MCID")
		if !found {
			return nil, nil
		}
		v := w.deref(mcid)
		if v == nil {
			return nil, fmt.Errorf("/MCID in property list /%s does not resolve to an object", name)
		}
		return v, nil
	}
}

// align builds a page's per-MCID alignment rows in ascending MCID order.
func (w *structureWalker) align(sp *StructPage) []MCIDAlignment {
	rows := map[int]*MCIDAlignment{}
	row := func(mcid int) *MCIDAlignment {
		if a, ok := rows[mcid]; ok {
			return a
		}
		a := &MCIDAlignment{MCID: mcid, TreeElements: []string{}}
		rows[mcid] = a
		return a
	}
	for _, c := range sp.ContentMCIDs {
		if c.MCID >= 0 {
			row(c.MCID).ContentCount++
		}
	}
	for _, s := range sp.ParentTree {
		if s.ElementRef == "" && s.Err == "" {
			continue
		}
		a := row(s.MCID)
		if s.Err == "" {
			a.ParentTreeElement, a.ParentTreeS = s.ElementRef, s.S
		}
	}
	for mcid, owners := range w.claims[sp.PageNum] {
		row(mcid).TreeElements = append([]string{}, owners...)
	}
	out := make([]MCIDAlignment, 0, len(rows))
	for _, a := range rows {
		a.Consistent = a.ContentCount == 1 && a.ParentTreeElement != "" &&
			len(a.TreeElements) == 1 && a.TreeElements[0] == a.ParentTreeElement
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MCID < out[j].MCID })
	return out
}

// appendErr appends msg to errs when it is not empty.
func appendErr(errs []string, msg string) []string {
	if msg == "" {
		return errs
	}
	return append(errs, msg)
}

// joinErr joins two error texts with "; ", skipping an empty one.
func joinErr(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}
