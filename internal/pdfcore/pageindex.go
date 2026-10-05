package pdfcore

import (
	"fmt"
	"math"
	"reflect"
	"strings"

	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// GetPageIndex returns the page index for the document in tabID: one entry per
// page leaf of the page tree in document order, plus an unnumbered entry
// (PageNum 0, Err set) for every page-tree entry that is not a page. Built by a
// single depth-first walk from the catalog's /Pages on first call and cached on
// the per-tab DocumentState; a re-Open under the same tabID replaces the
// DocumentState, and with it the cache. A malformed page tree, including a
// node pdfcpu fails to read, yields rows carrying Err, not a failed call. A
// document with no pages returns a non-nil empty slice.
func (ins *Inspector) GetPageIndex(tabID string) ([]*PageIndexEntry, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	// Serialize pdfcpu access. Outer lock; the pageIndex cache mutex (inner)
	// guards the cached walk.
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	t, err := doc.pageTree()
	if err != nil {
		return nil, err
	}
	return t.entries, nil
}

// pageTree is the result of one page-tree walk: the index rows, and every
// numbered page in document order, so leaves[n-1] is page n. err is set only
// when the walk failed outside any page-tree node (each node is read under its
// own guard); leaves then holds the pages reached before the failure.
type pageTree struct {
	entries []*PageIndexEntry
	leaves  []*pageLeaf
	err     error
}

// pageTree returns the cached walk of d's page tree, building it on first
// use. A walk that failed part way is cached too, with its error, so the
// failure is paid once per document: the build itself never returns an error
// (lazyCache would not cache one), the failure travels in the value. The
// returned tree is never nil. Callers hold pdfMu.
func (d *DocumentState) pageTree() (*pageTree, error) {
	t, _ := d.pageIndex.get(func() (*pageTree, error) {
		w := newPageWalker(d.PDFContext)
		t := &pageTree{}
		if err := safeCall(func() error {
			w.walk()
			return nil
		}); err != nil {
			t.err = wrapPDFError(err)
		}
		t.entries, t.leaves = w.entries, w.leaves
		return t, nil
	})
	return t, t.err
}

// findPage returns page pageNum as GetPageIndex numbers it, so page-number
// lookups agree with `dump pages` and with viewers (a /Type /Page carrying
// /Kids is still a page). Nil when no page has that number. After a walk that
// failed part way, a page it reached still resolves and a later number returns
// the walk's error. Callers hold pdfMu.
func (d *DocumentState) findPage(pageNum int) (*pageLeaf, error) {
	if pageNum < 1 {
		return nil, nil
	}
	t, err := d.pageTree()
	if pageNum <= len(t.leaves) {
		leaf := t.leaves[pageNum-1]
		if leaf.err != nil {
			return nil, leaf.err
		}
		return leaf, nil
	}
	return nil, err
}

// maxPageTreeDepth caps how many intermediate nodes one descent path may
// hold. A deeper subtree becomes one unnumbered error row, so a direct-dict
// cycle or a pathological chain cannot exhaust the goroutine stack, which
// safeCall cannot recover.
const maxPageTreeDepth = 1024

// pageAttrs holds the resolved value of each inheritable page attribute (ISO
// 32000-1 7.7.3.4) in effect at a node; nil when neither the node nor an
// ancestor declares it. unresolvedAt names, per attribute in inheritableAttrs
// order, the nearest node whose own entry resolves to nothing (a missing or
// null object) when no nearer node supplies a value; "" otherwise.
// resourcesErr is set when such a node's own /Resources entry failed to
// resolve with an error, as opposed to resolving to null or dangling.
type pageAttrs struct {
	resources    pdfcpu_types.Object
	mediaBox     pdfcpu_types.Object
	cropBox      pdfcpu_types.Object
	rotate       pdfcpu_types.Object
	unresolvedAt [4]string
	resourcesErr *attrError
}

// attrError is a page-tree node's attribute entry that failed to resolve:
// the node's label for messages, a key identifying the node (its object
// number, or the identity of a direct dictionary), and the resolve error.
type attrError struct {
	node string
	key  pageResKey
	err  error
}

// Positions of the inheritable attributes in inheritableAttrs and
// pageAttrs.unresolvedAt.
const (
	attrResources = iota
	attrMediaBox
	attrCropBox
	attrRotate
)

// inheritableAttrs gives each inheritable attribute's key and Inherited bit,
// indexed by the attr constants.
var inheritableAttrs = [4]struct {
	key string
	bit uint8
}{
	attrResources: {"Resources", InheritedResources},
	attrMediaBox:  {"MediaBox", InheritedMediaBox},
	attrCropBox:   {"CropBox", InheritedCropBox},
	attrRotate:    {"Rotate", InheritedRotate},
}

// pageWalker carries the state of one page-tree walk.
type pageWalker struct {
	ctx     *pdfcpu_model.Context
	entries []*PageIndexEntry
	// onPath holds the object numbers of the intermediate nodes on the current
	// descent path; a kid naming one of them is a cycle.
	onPath map[int]bool
	// walked holds every intermediate node already descended into, so a
	// subtree shared by two parents is walked once.
	walked map[int]bool
	// firstPage maps a page object number to the page number it first got, so
	// a page listed under two parents is numbered again with an error.
	firstPage map[int]int
	next      int
	// depth counts the intermediate nodes on the current descent path.
	depth int
	// leaves holds every numbered page in document order.
	leaves []*pageLeaf
}

// pageLeaf is one numbered page from the page-tree walk: its dictionary, its
// reference (nil for a direct dictionary) and each inheritable attribute in
// effect - the page's own value, else the nearest ancestor's, else nil.
type pageLeaf struct {
	dict  pdfcpu_types.Dict
	ref   *pdfcpu_types.IndirectRef
	attrs pageAttrs
	// err is set when pdfcpu failed while reading the page; attrs is then
	// incomplete and lookups report err.
	err error
}

func newPageWalker(ctx *pdfcpu_model.Context) *pageWalker {
	return &pageWalker{
		ctx:       ctx,
		entries:   []*PageIndexEntry{},
		onPath:    map[int]bool{},
		walked:    map[int]bool{},
		firstPage: map[int]int{},
	}
}

// walk visits the page tree from the catalog's /Pages, filling w.entries and
// w.leaves. No /Pages, or a /Pages that is not a dictionary, yields neither.
func (w *pageWalker) walk() {
	ctx := w.ctx
	if ctx == nil || ctx.XRefTable == nil {
		return
	}
	catalog, err := ctx.Catalog()
	if err != nil || catalog == nil {
		return
	}
	rootObj, found := catalog.Find("Pages")
	if !found || rootObj == nil {
		return
	}
	var ref *pdfcpu_types.IndirectRef
	if r, ok := rootObj.(pdfcpu_types.IndirectRef); ok {
		ref = &r
	}
	root, ok := derefIn(w.ctx, rootObj).(pdfcpu_types.Dict)
	if !ok {
		return
	}
	guard(func() { w.visitNode(root, ref, pageAttrs{}) }, func(msg string) {
		w.entries = append(w.entries, errorRow(ref, "the page tree could not be read: "+msg))
	})
}

// guard runs fn and, when pdfcpu panics inside it, reports the panic through
// fail and returns, so one unreadable node does not end the page-tree walk.
// Go runtime errors are not recovered (see safeCall).
func guard(fn func(), fail func(msg string)) {
	if err := safeCall(func() error {
		fn()
		return nil
	}); err != nil {
		fail(err.Error())
	}
}

// derefIn resolves o in ctx, returning nil for a dangling reference or a
// resolve error.
func derefIn(ctx *pdfcpu_model.Context, o pdfcpu_types.Object) pdfcpu_types.Object {
	v, err := ctx.Dereference(o)
	if err != nil {
		return nil
	}
	return v
}

// visitNode classifies a page-tree node and either descends into it or emits
// its row, the way viewers number pages. First match wins: /Type /Page is a
// page leaf even with /Kids; a node with /Kids is an intermediate; /Type
// /Pages without /Kids is an unnumbered error row; anything else is a page
// leaf.
func (w *pageWalker) visitNode(d pdfcpu_types.Dict, ref *pdfcpu_types.IndirectRef, inherited pageAttrs) {
	typ := nameOf(derefIn(w.ctx, d["Type"]))
	if typ == "Page" {
		w.visitLeaf(d, ref, inherited)
		return
	}
	if kids, hasKids := d.Find("Kids"); hasKids {
		w.visitIntermediate(d, ref, kids, inherited)
		return
	}
	if typ == "Pages" {
		w.entries = append(w.entries, errorRow(ref, "/Pages node has no /Kids"))
		return
	}
	w.visitLeaf(d, ref, inherited)
}

// visitIntermediate pushes the node's inheritable attributes and visits each
// /Kids entry in order.
func (w *pageWalker) visitIntermediate(d pdfcpu_types.Dict, ref *pdfcpu_types.IndirectRef, kidsObj pdfcpu_types.Object, inherited pageAttrs) {
	if w.depth >= maxPageTreeDepth {
		w.entries = append(w.entries, errorRow(ref, fmt.Sprintf("page tree deeper than %d levels; subtree not walked", maxPageTreeDepth)))
		return
	}
	w.depth++
	defer func() { w.depth-- }()

	if ref != nil {
		num := ref.ObjectNumber.Value()
		if w.walked[num] {
			w.entries = append(w.entries, errorRow(ref, "page tree node also reached from another parent"))
			return
		}
		w.walked[num] = true
		w.onPath[num] = true
		defer delete(w.onPath, num)
	}

	kids, ok := derefIn(w.ctx, kidsObj).(pdfcpu_types.Array)
	if !ok {
		w.entries = append(w.entries, errorRow(ref, fmt.Sprintf("/Kids is not an array: %s", typeLabel(derefIn(w.ctx, kidsObj)))))
		return
	}

	frame, _, _ := effectiveAttrs(w.ctx, d, ref, inherited)
	for i, kid := range kids {
		guard(func() { w.visitKid(i, kid, frame) }, func(msg string) {
			var kidRef *pdfcpu_types.IndirectRef
			if r, ok := kid.(pdfcpu_types.IndirectRef); ok {
				kidRef = &r
			}
			w.entries = append(w.entries, errorRow(kidRef, fmt.Sprintf("/Kids entry %d could not be read: %s", i, msg)))
		})
	}
}

// visitKid resolves one /Kids entry. A null, a non-dictionary, a dangling
// reference or a reference to a node on the current path becomes an unnumbered
// error row; a dictionary is classified by visitNode.
func (w *pageWalker) visitKid(index int, kid pdfcpu_types.Object, inherited pageAttrs) {
	if kid == nil {
		w.entries = append(w.entries, errorRow(nil, fmt.Sprintf("/Kids entry %d is null", index)))
		return
	}
	ref, isRef := kid.(pdfcpu_types.IndirectRef)
	if !isRef {
		if d, ok := kid.(pdfcpu_types.Dict); ok {
			w.visitNode(d, nil, inherited)
			return
		}
		w.entries = append(w.entries, errorRow(nil, fmt.Sprintf("/Kids entry %d is not a dictionary: %s", index, typeLabel(kid))))
		return
	}
	if w.onPath[ref.ObjectNumber.Value()] {
		w.entries = append(w.entries, errorRow(&ref, fmt.Sprintf("/Kids entry %d (%s) points at an ancestor: page tree cycle", index, ref.String())))
		return
	}
	resolved := derefIn(w.ctx, ref)
	if resolved == nil {
		w.entries = append(w.entries, errorRow(&ref, fmt.Sprintf("/Kids entry %d (%s) is a dangling reference", index, ref.String())))
		return
	}
	d, ok := resolved.(pdfcpu_types.Dict)
	if !ok {
		w.entries = append(w.entries, errorRow(&ref, fmt.Sprintf("/Kids entry %d (%s) is not a dictionary: %s", index, ref.String(), typeLabel(resolved))))
		return
	}
	w.visitNode(d, &ref, inherited)
}

// visitLeaf numbers a page leaf and records it in w.leaves together with its
// row, so a page resolves by number exactly when the index lists it. Problems
// on the page set Err without unnumbering it; a pdfcpu panic while reading the
// page keeps its number and marks both the row and the leaf.
func (w *pageWalker) visitLeaf(d pdfcpu_types.Dict, ref *pdfcpu_types.IndirectRef, inherited pageAttrs) {
	w.next++
	e := &PageIndexEntry{PageNum: w.next}
	if ref != nil {
		setRef(e, ref)
	}
	leaf := &pageLeaf{dict: d, ref: ref}
	guard(func() { w.fillLeaf(d, ref, inherited, e, leaf) }, func(msg string) {
		e.Err = "page could not be read: " + msg
		leaf.err = fmt.Errorf("page %d could not be read: %s", e.PageNum, msg)
	})
	w.leaves = append(w.leaves, leaf)
	w.entries = append(w.entries, e)
}

// fillLeaf reads a page's attributes into its row e and its leaf.
func (w *pageWalker) fillLeaf(d pdfcpu_types.Dict, ref *pdfcpu_types.IndirectRef, inherited pageAttrs, e *PageIndexEntry, leaf *pageLeaf) {
	eff, inheritedBits, ownUnresolved := effectiveAttrs(w.ctx, d, ref, inherited)
	leaf.attrs = eff
	e.Inherited = inheritedBits

	var errs []string
	if ref != nil {
		num := ref.ObjectNumber.Value()
		if first, seen := w.firstPage[num]; seen {
			errs = append(errs, fmt.Sprintf("page object also listed as page %d", first))
		} else {
			w.firstPage[num] = e.PageNum
		}
	} else {
		errs = append(errs, "page is a direct dictionary, not an indirect reference")
	}

	switch t := derefIn(w.ctx, d["Type"]); {
	case t == nil:
		errs = append(errs, "page has no /Type")
	case nameOf(t) != "Page":
		errs = append(errs, fmt.Sprintf("page has /Type %s, want /Page", typeValueLabel(t)))
	}
	if _, ok := d.Find("Kids"); ok {
		errs = append(errs, "page has a /Kids entry; its kids are not walked")
	}

	for i, a := range inheritableAttrs {
		at := eff.unresolvedAt[i]
		if at == "" {
			continue
		}
		msg := fmt.Sprintf("/%s on %s resolves to nothing", a.key, at)
		if ownUnresolved&a.bit != 0 {
			msg = fmt.Sprintf("/%s resolves to nothing", a.key)
		}
		if inheritedBits&a.bit != 0 {
			msg += "; inherited value used"
		}
		errs = append(errs, msg)
	}

	if eff.mediaBox == nil {
		// An entry that resolves to nothing has already been named above.
		if eff.unresolvedAt[attrMediaBox] == "" {
			errs = append(errs, "no /MediaBox, own or inherited")
		}
	} else if box, ok := readRect(w.ctx, eff.mediaBox); ok {
		e.MediaBox = box
	} else {
		errs = append(errs, "/MediaBox is not an array of four numbers")
	}

	if eff.rotate != nil {
		n, integer, ok := readRotation(w.ctx, eff.rotate)
		switch {
		case !ok:
			errs = append(errs, "/Rotate is not a number")
		case !integer:
			e.Rotate = n
			errs = append(errs, fmt.Sprintf("/Rotate is not an integer; read as %d", n))
		default:
			e.Rotate = n
		}
	}

	if annots, ok := derefIn(w.ctx, ownEntry(d, "Annots")).(pdfcpu_types.Array); ok {
		e.AnnotCount = len(annots)
	}

	errs = append(errs, w.contents(ownEntry(d, "Contents"), e)...)

	e.Err = strings.Join(errs, "; ")
}

// ownEntry returns d's own value for key, nil when absent.
func ownEntry(d pdfcpu_types.Dict, key string) pdfcpu_types.Object {
	v, ok := d.Find(key)
	if !ok {
		return nil
	}
	return v
}

// effectiveAttrs applies page-attribute inheritance at node d (reference ref,
// nil for a direct dictionary): each attribute is the node's own value, else
// the nearest ancestor's, else nil, stored resolved (readers still dereference
// elements inside an array). An own entry that resolves to nothing (a missing
// or null object, which ISO 32000-1 7.3.10 reads as null) counts as absent, so
// the ancestor's value applies; the node is recorded in unresolvedAt so a page
// row can name it. A /Resources entry whose resolve returned an error is also
// recorded in resourcesErr, which a nearer node's resolved /Resources clears.
// bits reports which attributes came from an ancestor, and
// ownUnresolved which of d's own entries resolved to nothing.
func effectiveAttrs(ctx *pdfcpu_model.Context, d pdfcpu_types.Dict, ref *pdfcpu_types.IndirectRef, inherited pageAttrs) (eff pageAttrs, bits, ownUnresolved uint8) {
	eff.unresolvedAt = inherited.unresolvedAt
	eff.resourcesErr = inherited.resourcesErr
	pick := func(i int, ancestor pdfcpu_types.Object) pdfcpu_types.Object {
		a := inheritableAttrs[i]
		if v := ownEntry(d, a.key); v != nil {
			r, err := ctx.Dereference(v)
			if err == nil && r != nil {
				eff.unresolvedAt[i] = ""
				if i == attrResources {
					eff.resourcesErr = nil
				}
				return r
			}
			if err != nil && i == attrResources {
				key := pageResKey{dict: reflect.ValueOf(d).Pointer()}
				if ref != nil {
					key = pageResKey{num: ref.ObjectNumber.Value()}
				}
				eff.resourcesErr = &attrError{node: nodeLabel(ref), key: key, err: err}
			}
			ownUnresolved |= a.bit
			eff.unresolvedAt[i] = nodeLabel(ref)
		}
		if ancestor != nil {
			bits |= a.bit
		}
		return ancestor
	}
	eff.resources = pick(attrResources, inherited.resources)
	eff.mediaBox = pick(attrMediaBox, inherited.mediaBox)
	eff.cropBox = pick(attrCropBox, inherited.cropBox)
	eff.rotate = pick(attrRotate, inherited.rotate)
	return eff, bits, ownUnresolved
}

// nodeLabel names a page-tree node for a message: its reference, or "a direct
// dictionary" when it has none.
func nodeLabel(ref *pdfcpu_types.IndirectRef) string {
	if ref == nil {
		return "a direct dictionary"
	}
	return refString(*ref)
}

// readRotation reads /Rotate. An integer is taken as is; a real is rounded to
// the nearest integer and reported as not an integer. ok is false for anything
// that is not a number.
func readRotation(ctx *pdfcpu_model.Context, o pdfcpu_types.Object) (n int, integer, ok bool) {
	switch v := derefIn(ctx, o).(type) {
	case pdfcpu_types.Integer:
		return v.Value(), true, true
	case pdfcpu_types.Float:
		f := v.Value()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return 0, false, false
		}
		return int(math.Round(f)), false, true
	}
	return 0, false, false
}

// inheritedPageAttrs resolves a found page's effective attributes into
// pdfcpu's InheritedPageAttrs shape: the /Resources dictionary, the /MediaBox
// and /CropBox rectangles (nil when absent), and /Rotate (a real rounded to the
// nearest integer). Rectangles and /Rotate are read exactly as GetPageIndex
// reads them. A present but malformed attribute is an error. Callers hold
// pdfMu and wrap the call in safeCall.
func inheritedPageAttrs(ctx *pdfcpu_model.Context, leaf *pageLeaf) (*pdfcpu_model.InheritedPageAttrs, error) {
	out := &pdfcpu_model.InheritedPageAttrs{}
	if r := derefIn(ctx, leaf.attrs.resources); r != nil {
		d, ok := r.(pdfcpu_types.Dict)
		if !ok {
			return nil, fmt.Errorf("/Resources is not a dictionary")
		}
		out.Resources = d
	}
	box := func(name string, o pdfcpu_types.Object) (*pdfcpu_types.Rectangle, error) {
		if o == nil {
			return nil, nil
		}
		v, ok := readRect(ctx, o)
		if !ok {
			return nil, fmt.Errorf("/%s is not an array of four numbers", name)
		}
		return pdfcpu_types.NewRectangle(v[0], v[1], v[2], v[3]), nil
	}
	var err error
	if out.MediaBox, err = box("MediaBox", leaf.attrs.mediaBox); err != nil {
		return nil, err
	}
	if out.CropBox, err = box("CropBox", leaf.attrs.cropBox); err != nil {
		return nil, err
	}
	if leaf.attrs.rotate != nil {
		n, _, ok := readRotation(ctx, leaf.attrs.rotate)
		if !ok {
			return nil, fmt.Errorf("/Rotate is not a number")
		}
		out.Rotate = n
	}
	return out, nil
}

// readRect reads a rectangle of four finite numbers, dereferencing the array and
// each element when indirect. A NaN or infinite element fails, since it cannot
// be encoded as JSON.
func readRect(ctx *pdfcpu_model.Context, o pdfcpu_types.Object) ([4]float64, bool) {
	var box [4]float64
	arr, ok := derefIn(ctx, o).(pdfcpu_types.Array)
	if !ok || len(arr) != 4 {
		return box, false
	}
	for i, el := range arr {
		switch v := derefIn(ctx, el).(type) {
		case pdfcpu_types.Integer:
			box[i] = float64(v.Value())
		case pdfcpu_types.Float:
			f := v.Value()
			if math.IsNaN(f) || math.IsInf(f, 0) {
				return [4]float64{}, false
			}
			box[i] = f
		default:
			return [4]float64{}, false
		}
	}
	return box, true
}

// contents sets ContentNodeID and ContentLen from a page's /Contents and
// returns the problems found. ContentLen is the sum of the /Length entries,
// read from the stream dictionaries; no stream is decoded. It is 0 without
// /Contents and -1 when any stream's /Length is unreadable, the sum overflows
// int64, or /Contents is malformed.
func (w *pageWalker) contents(obj pdfcpu_types.Object, e *PageIndexEntry) []string {
	if obj == nil {
		return nil
	}
	var elems pdfcpu_types.Array
	switch v := obj.(type) {
	case pdfcpu_types.IndirectRef:
		switch r := derefIn(w.ctx, v).(type) {
		case pdfcpu_types.StreamDict:
			e.ContentNodeID = nodeIDForRef(v)
			e.ContentLen = w.streamLength(r)
			return nil
		case pdfcpu_types.Array:
			elems = r
		default:
			e.ContentLen = -1
			return []string{fmt.Sprintf("/Contents %s is not a stream: %s", v.String(), typeLabel(r))}
		}
	case pdfcpu_types.Array:
		elems = v
	default:
		e.ContentLen = -1
		return []string{fmt.Sprintf("/Contents is not a stream reference or array: %s", typeLabel(v))}
	}

	var errs []string
	var total int64
	lengthOK := true
	for i, el := range elems {
		if el == nil {
			continue
		}
		ref, ok := el.(pdfcpu_types.IndirectRef)
		if !ok {
			errs = append(errs, fmt.Sprintf("/Contents element %d is not an indirect reference: %s", i, typeLabel(el)))
			continue
		}
		sd, ok := derefIn(w.ctx, ref).(pdfcpu_types.StreamDict)
		if !ok {
			errs = append(errs, fmt.Sprintf("/Contents element %d (%s) is not a stream", i, ref.String()))
			continue
		}
		if e.ContentNodeID == "" {
			e.ContentNodeID = nodeIDForRef(ref)
		}
		if n := w.streamLength(sd); n >= 0 && total <= math.MaxInt64-n {
			total += n
		} else {
			lengthOK = false
		}
	}
	if len(errs) > 0 || !lengthOK {
		e.ContentLen = -1
	} else {
		e.ContentLen = total
	}
	return errs
}

// streamLength returns a stream's /Length entry, dereferenced when indirect,
// or -1 when it is missing, not an integer, or negative.
func (w *pageWalker) streamLength(sd pdfcpu_types.StreamDict) int64 {
	v, ok := sd.Find("Length")
	if !ok {
		return -1
	}
	n, ok := derefIn(w.ctx, v).(pdfcpu_types.Integer)
	if !ok || n.Value() < 0 {
		return -1
	}
	return int64(n.Value())
}

// errorRow builds an unnumbered row, carrying ref when there is one.
func errorRow(ref *pdfcpu_types.IndirectRef, msg string) *PageIndexEntry {
	e := &PageIndexEntry{Err: msg}
	if ref != nil {
		setRef(e, ref)
	}
	return e
}

// setRef fills an entry's object number, generation and node id from ref.
func setRef(e *PageIndexEntry, ref *pdfcpu_types.IndirectRef) {
	e.ObjNum = ref.ObjectNumber.Value()
	e.Gen = ref.GenerationNumber.Value()
	e.NodeID = nodeIDForRef(*ref)
}

// nodeIDForRef encodes ref as a tree node id, generation first.
func nodeIDForRef(ref pdfcpu_types.IndirectRef) string {
	return fmt.Sprintf("obj:%d:%d", ref.GenerationNumber.Value(), ref.ObjectNumber.Value())
}

// nameOf returns the value of a name object, or "" for anything else.
func nameOf(o pdfcpu_types.Object) string {
	if n, ok := o.(pdfcpu_types.Name); ok {
		return n.Value()
	}
	return ""
}

// typeValueLabel renders a /Type value for an error message: "/Name" for a
// name, the object kind otherwise.
func typeValueLabel(o pdfcpu_types.Object) string {
	if n, ok := o.(pdfcpu_types.Name); ok {
		return "/" + n.Value()
	}
	return typeLabel(o)
}

// typeLabel names the kind of a PDF object for an error message.
func typeLabel(o pdfcpu_types.Object) string {
	switch o.(type) {
	case nil:
		return "null"
	case pdfcpu_types.Dict:
		return "dictionary"
	case pdfcpu_types.StreamDict:
		return "stream"
	case pdfcpu_types.Array:
		return "array"
	case pdfcpu_types.Integer:
		return "integer"
	case pdfcpu_types.Float:
		return "real"
	case pdfcpu_types.Name:
		return "name"
	case pdfcpu_types.StringLiteral, pdfcpu_types.HexLiteral:
		return "string"
	case pdfcpu_types.Boolean:
		return "boolean"
	case pdfcpu_types.IndirectRef:
		return "reference"
	default:
		return fmt.Sprintf("%T", o)
	}
}
