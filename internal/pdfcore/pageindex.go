package pdfcore

import (
	"fmt"
	"math"
	"strings"

	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// GetPageIndex returns the page index for the document in tabID: one entry per
// page leaf of the page tree in document order, plus an unnumbered entry
// (PageNum 0, Err set) for every page-tree entry that is not a page. Built by a
// single depth-first walk from the catalog's /Pages on first call and cached on
// the per-tab DocumentState; a re-Open under the same tabID replaces the
// DocumentState, and with it the cache. A malformed page tree yields rows
// carrying Err, never a failed call. A document with no pages returns a non-nil empty slice.
func (ins *Inspector) GetPageIndex(tabID string) ([]*PageIndexEntry, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	// Serialize pdfcpu access. Outer lock; the pageIndex cache mutex (inner)
	// guards the cached slice.
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	return doc.pageIndex.get(func() ([]*PageIndexEntry, error) {
		var entries []*PageIndexEntry
		err := safeCall(func() error {
			entries = buildPageIndex(doc.PDFContext)
			return nil
		})
		if err != nil {
			return nil, wrapPDFError(err)
		}
		return entries, nil
	})
}

// maxPageTreeDepth caps how many intermediate nodes one descent path may
// hold. A deeper subtree becomes one unnumbered error row, so a direct-dict
// cycle or a pathological chain cannot exhaust the goroutine stack, which
// safeCall cannot recover.
const maxPageTreeDepth = 1024

// pageAttrs holds the nearest-ancestor value of each inheritable page
// attribute (ISO 32000-1 7.7.3.4); nil when no ancestor declares it.
type pageAttrs struct {
	resources pdfcpu_types.Object
	mediaBox  pdfcpu_types.Object
	cropBox   pdfcpu_types.Object
	rotate    pdfcpu_types.Object
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
	// want, when positive, is the page number findPage is looking for; the
	// walk records that page in found and stops descending.
	want  int
	found *pageLeaf
}

// pageLeaf is one numbered page found by findPage: its dictionary, its
// reference (nil for a direct dictionary) and each inheritable attribute in
// effect - the page's own value, else the nearest ancestor's, else nil.
type pageLeaf struct {
	dict  pdfcpu_types.Dict
	ref   *pdfcpu_types.IndirectRef
	attrs pageAttrs
}

// findPage returns the page numbered pageNum by the same walk and numbering
// as GetPageIndex, so page-number lookups agree with `dump pages` and with
// viewers (a /Type /Page carrying /Kids is still a page). Nil when no page has
// that number. Callers hold pdfMu and wrap the call in safeCall.
func findPage(ctx *pdfcpu_model.Context, pageNum int) *pageLeaf {
	if pageNum < 1 {
		return nil
	}
	w := newPageWalker(ctx)
	w.want = pageNum
	w.walk()
	return w.found
}

// buildPageIndex walks the page tree of ctx from the catalog's /Pages. No
// /Pages, or a /Pages that is not a dictionary, yields an empty slice.
func buildPageIndex(ctx *pdfcpu_model.Context) []*PageIndexEntry {
	w := newPageWalker(ctx)
	return w.walk()
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

// walk visits the page tree from the catalog's /Pages and returns the rows.
func (w *pageWalker) walk() []*PageIndexEntry {
	ctx := w.ctx
	if ctx == nil || ctx.XRefTable == nil {
		return w.entries
	}
	catalog, err := ctx.Catalog()
	if err != nil || catalog == nil {
		return w.entries
	}
	rootObj, found := catalog.Find("Pages")
	if !found || rootObj == nil {
		return w.entries
	}
	var ref *pdfcpu_types.IndirectRef
	if r, ok := rootObj.(pdfcpu_types.IndirectRef); ok {
		ref = &r
	}
	root, ok := w.deref(rootObj).(pdfcpu_types.Dict)
	if !ok {
		return w.entries
	}
	w.visitNode(root, ref, pageAttrs{})
	return w.entries
}

// deref resolves o, returning nil for a dangling reference or a resolve error.
func (w *pageWalker) deref(o pdfcpu_types.Object) pdfcpu_types.Object {
	v, err := w.ctx.Dereference(o)
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
	typ := nameOf(w.deref(d["Type"]))
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

	kids, ok := w.deref(kidsObj).(pdfcpu_types.Array)
	if !ok {
		w.entries = append(w.entries, errorRow(ref, fmt.Sprintf("/Kids is not an array: %s", typeLabel(w.deref(kidsObj)))))
		return
	}

	frame := inherited
	if v, ok := d.Find("Resources"); ok && v != nil {
		frame.resources = v
	}
	if v, ok := d.Find("MediaBox"); ok && v != nil {
		frame.mediaBox = v
	}
	if v, ok := d.Find("CropBox"); ok && v != nil {
		frame.cropBox = v
	}
	if v, ok := d.Find("Rotate"); ok && v != nil {
		frame.rotate = v
	}

	for i, kid := range kids {
		if w.found != nil {
			return
		}
		w.visitKid(i, kid, frame)
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
	resolved := w.deref(ref)
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

// visitLeaf numbers a page leaf and records its attributes. Problems on the
// page set Err without unnumbering it. When findPage is looking for a page,
// leaves before it are only counted and the wanted one is captured.
func (w *pageWalker) visitLeaf(d pdfcpu_types.Dict, ref *pdfcpu_types.IndirectRef, inherited pageAttrs) {
	w.next++
	eff := effectiveAttrs(d, inherited)
	if w.want > 0 {
		if w.next == w.want && w.found == nil {
			w.found = &pageLeaf{dict: d, ref: ref, attrs: eff}
		}
		return
	}

	e := &PageIndexEntry{PageNum: w.next}
	var errs []string
	if ref != nil {
		setRef(e, ref)
		num := ref.ObjectNumber.Value()
		if first, seen := w.firstPage[num]; seen {
			errs = append(errs, fmt.Sprintf("page object also listed as page %d", first))
		} else {
			w.firstPage[num] = e.PageNum
		}
	} else {
		errs = append(errs, "page is a direct dictionary, not an indirect reference")
	}

	switch t := w.deref(d["Type"]); {
	case t == nil:
		errs = append(errs, "page has no /Type")
	case nameOf(t) != "Page":
		errs = append(errs, fmt.Sprintf("page has /Type %s, want /Page", typeValueLabel(t)))
	}
	if _, ok := d.Find("Kids"); ok {
		errs = append(errs, "page has a /Kids entry; its kids are not walked")
	}

	// An attribute is inherited when the page has none of its own and an
	// ancestor supplies one.
	for _, a := range []struct {
		key      string
		ancestor pdfcpu_types.Object
		bit      uint8
	}{
		{"Resources", inherited.resources, InheritedResources},
		{"MediaBox", inherited.mediaBox, InheritedMediaBox},
		{"CropBox", inherited.cropBox, InheritedCropBox},
		{"Rotate", inherited.rotate, InheritedRotate},
	} {
		if ownEntry(d, a.key) == nil && a.ancestor != nil {
			e.Inherited |= a.bit
		}
	}

	if eff.mediaBox == nil {
		errs = append(errs, "no /MediaBox, own or inherited")
	} else if box, ok := w.rect(eff.mediaBox); ok {
		e.MediaBox = box
	} else {
		errs = append(errs, "/MediaBox is not an array of four numbers")
	}

	if eff.rotate != nil {
		n, integer, ok := w.rotation(eff.rotate)
		switch {
		case !ok:
			errs = append(errs, "/Rotate is not an integer")
		case !integer:
			e.Rotate = n
			errs = append(errs, fmt.Sprintf("/Rotate is not an integer; read as %d", n))
		default:
			e.Rotate = n
		}
	}

	if annots, ok := w.deref(ownEntry(d, "Annots")).(pdfcpu_types.Array); ok {
		e.AnnotCount = len(annots)
	}

	errs = append(errs, w.contents(ownEntry(d, "Contents"), e)...)

	e.Err = strings.Join(errs, "; ")
	w.entries = append(w.entries, e)
}

// ownEntry returns d's own value for key, nil when absent.
func ownEntry(d pdfcpu_types.Dict, key string) pdfcpu_types.Object {
	v, ok := d.Find(key)
	if !ok {
		return nil
	}
	return v
}

// effectiveAttrs applies page-attribute inheritance: each attribute is the
// page's own value, else the nearest ancestor's, else nil.
func effectiveAttrs(d pdfcpu_types.Dict, inherited pageAttrs) pageAttrs {
	pick := func(key string, ancestor pdfcpu_types.Object) pdfcpu_types.Object {
		if v := ownEntry(d, key); v != nil {
			return v
		}
		return ancestor
	}
	return pageAttrs{
		resources: pick("Resources", inherited.resources),
		mediaBox:  pick("MediaBox", inherited.mediaBox),
		cropBox:   pick("CropBox", inherited.cropBox),
		rotate:    pick("Rotate", inherited.rotate),
	}
}

// rotation reads /Rotate. An integer is taken as is; a real is rounded to the
// nearest integer and reported as not an integer. ok is false for anything
// that is not a number.
func (w *pageWalker) rotation(o pdfcpu_types.Object) (n int, integer, ok bool) {
	switch v := w.deref(o).(type) {
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
	w := &pageWalker{ctx: ctx}
	out := &pdfcpu_model.InheritedPageAttrs{}
	// A /Resources that resolves to nothing (a dangling reference) is absent.
	if r := w.deref(leaf.attrs.resources); r != nil {
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
		v, ok := w.rect(o)
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
		n, _, ok := w.rotation(leaf.attrs.rotate)
		if !ok {
			return nil, fmt.Errorf("/Rotate is not a number")
		}
		out.Rotate = n
	}
	return out, nil
}

// rect reads a rectangle of four finite numbers, dereferencing the array and
// each element when indirect. A NaN or infinite element fails, since it cannot
// be encoded as JSON.
func (w *pageWalker) rect(o pdfcpu_types.Object) ([4]float64, bool) {
	var box [4]float64
	arr, ok := w.deref(o).(pdfcpu_types.Array)
	if !ok || len(arr) != 4 {
		return box, false
	}
	for i, el := range arr {
		switch v := w.deref(el).(type) {
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
		switch r := w.deref(v).(type) {
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
		sd, ok := w.deref(ref).(pdfcpu_types.StreamDict)
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
	n, ok := w.deref(v).(pdfcpu_types.Integer)
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
