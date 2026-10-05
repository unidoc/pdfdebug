package pdfcore

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"strconv"
	"strings"

	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// maxImageFirstPages caps ImageIndexEntry.FirstPages, so an image used on
// every page of a long document does not carry every page number.
const maxImageFirstPages = 16

// maxImageWalkEntries bounds the /XObject entries one image walk examines
// across the whole document, so a crafted resource graph cannot make the walk
// quadratic or exponential.
const maxImageWalkEntries = 1_000_000

// maxImageWalkUses bounds the image uses one image walk retains across the
// whole document: those merged up from forms and those copied into page
// groups, memoised walks included. Shared forms nested under many wrappers
// stay cheap in entries but multiply uses, so entries alone do not bound them.
const maxImageWalkUses = 4_000_000

// GetImageIndex returns the image index for the document in tabID: one entry
// per image XObject referenced from page resources (directly, through
// inherited /Resources, or through nested Form XObjects), deduplicated by
// object reference and ordered by first-use page then object number, followed
// by error rows (NodeID ""): one per failing object or form skipped at the
// nesting cap, naming the pages it affected, one where a budget stopped the
// walk, naming the pages left unwalked, and one per page-tree problem. "Used on"
// means declared in the page's resources; content streams are not parsed, so
// inline images are not listed. Built on first call from the cached page-tree
// walk, decode-free, and cached on the per-tab DocumentState; a re-Open under
// the same tabID replaces the DocumentState, and with it the cache. A document
// with no images returns a non-nil empty slice. The entries are copies, so a
// caller may change them without touching the cache.
func (ins *Inspector) GetImageIndex(tabID string) ([]*ImageIndexEntry, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	// Serialize pdfcpu access. Outer lock; the pageIndex and imageIndex cache
	// mutexes (inner) guard the cached walks.
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	entries := doc.imageTree().entries
	out := make([]*ImageIndexEntry, len(entries))
	for i, e := range entries {
		out[i] = copyImageIndexEntry(e)
	}
	return out, nil
}

// GetImagePages returns every page, ascending, whose resources reference the
// image with object number objNum, each with its /Page node id. It reads the
// cached image walk, so it is the full list ImageIndexEntry.FirstPages caps.
// An object number the index does not list is an error naming it.
func (ins *Inspector) GetImagePages(tabID string, objNum int) ([]ImagePageRef, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	t := doc.imageTree()
	pages, ok := t.pages[objNum]
	if !ok {
		return nil, fmt.Errorf("object %d is not an image in the image index", objNum)
	}
	out := make([]ImagePageRef, len(pages))
	for i, n := range pages {
		out[i] = ImagePageRef{PageNum: n, NodeID: t.pageNodeIDs[n-1]}
	}
	return out, nil
}

// GetImagePageGroups returns one group per numbered page that uses an image or
// that the walk did not finish, in document order, each listing the images
// reached from that page's resources in walk order with the resource-name path
// to each. A page with no images that was walked to the end is left out, and
// so is every page after the one the walk stopped at. An image reachable twice
// within one page is listed once, under the first path found. Read from the
// cached image walk; the groups are copies.
func (ins *Inspector) GetImagePageGroups(tabID string) ([]ImagePageGroup, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	groups := doc.imageTree().groups
	out := make([]ImagePageGroup, len(groups))
	for i, g := range groups {
		out[i] = g
		out[i].Images = make([]ImagePageUse, len(g.Images))
		for j, u := range g.Images {
			out[i].Images[j] = ImagePageUse{ObjNum: u.ObjNum, Gen: u.Gen, Path: slices.Clone(u.Path)}
		}
	}
	return out, nil
}

// copyImageIndexEntry returns a copy of e that shares no slice or pointer
// with it.
func copyImageIndexEntry(e *ImageIndexEntry) *ImageIndexEntry {
	c := *e
	c.Filters = slices.Clone(e.Filters)
	c.FirstPages = slices.Clone(e.FirstPages)
	c.FirstPageNodeIDs = slices.Clone(e.FirstPageNodeIDs)
	c.Decode = slices.Clone(e.Decode)
	if e.SMask != nil {
		s := *e.SMask
		c.SMask = &s
	}
	if e.AdobeTransform != nil {
		t := *e.AdobeTransform
		c.AdobeTransform = &t
	}
	return &c
}

// imageTree is the result of one image walk: the index rows, the full page
// list per image object number, the per-page groups, and the /Page node id of
// each numbered page (index pageNum - 1; "" for a direct dictionary).
type imageTree struct {
	entries     []*ImageIndexEntry
	pages       map[int][]int
	groups      []ImagePageGroup
	pageNodeIDs []string
}

// imageTree returns the cached image walk of d, building it on first use.
// The page walk is fetched before the image cache is entered, so the image
// cache mutex is never held while waiting for the page cache mutex. The build
// never fails; failures are error rows. Callers hold pdfMu.
func (d *DocumentState) imageTree() *imageTree {
	pt, _ := d.pageTree()
	t, _ := d.imageIndex.get(func() (*imageTree, error) {
		return newImageWalker(d.PDFContext).build(pt), nil
	})
	return t
}

// imageRecord is one deduplicated image and the pages referencing it, in
// ascending order.
type imageRecord struct {
	entry *ImageIndexEntry
	pages []int
}

// resWalk is the walk of one resources dictionary: the images it reaches in
// walk order, each once, with paths relative to that dictionary, whether
// the walk was cut short, and the ids of the walk issues that cut it (an id
// may repeat). height is the number of Form XObject levels the walk
// entered below the dictionary, capped is set when the nesting cap stopped a
// form in it, and cyclic when it skipped a form already being walked above.
type resWalk struct {
	images     []ImagePageUse
	incomplete bool
	issues     []int
	height     int
	capped     bool
	cyclic     bool
}

// pageResKey identifies a page's /XObject dictionary for the walk memo: the
// object number of an indirect one, else the identity of a direct one. The
// page walk stores /Resources resolved, so pages sharing a resources
// dictionary, inherited or by reference, share the /XObject value too.
type pageResKey struct {
	num  int
	dict uintptr
}

// Kinds of walk issue: unreadable resources shared by pages, unreadable
// resources of one page, an unreadable /XObject entry or form /Resources, and
// a form not walked because of the nesting cap.
const (
	issueSharedResources = iota
	issuePageResources
	issueXObject
	issueDepth
)

// issueKey identifies the failing object behind a walk issue: res for shared
// page resources, else num (the page for issuePageResources, the object number
// otherwise).
type issueKey struct {
	kind int
	res  pageResKey
	num  int
}

// walkIssue is one failing object, its error row, and the pages whose walk it
// cut short as ascending inclusive ranges. build writes the row's message as
// prefix, the page list, suffix.
type walkIssue struct {
	row            *ImageIndexEntry
	prefix, suffix string
	firstPage      int
	ranges         [][2]int
}

// formWalk is a finished Form XObject walk and the depth it was entered at.
type formWalk struct {
	rw    resWalk
	depth int
}

// imageWalker carries the state of one image walk.
type imageWalker struct {
	ctx *pdfcpu_model.Context
	// budget is the number of /XObject entries the walk may examine.
	budget   int
	examined int
	// useLimit is the number of image uses the walk may retain; uses counts
	// them.
	useLimit int
	uses     int
	stopped  bool
	// readFacts reads one image dictionary; a field so tests can make it fail.
	readFacts func(*pdfcpu_model.XRefTable, *pdfcpu_types.StreamDict) imageDictFacts
	// records is keyed by object number alone, as pdfcpu resolves references.
	records map[int]*imageRecord
	errRows []*ImageIndexEntry
	// stopRow is the error row written when the walk stopped, at page
	// stopPage; build names the pages left unwalked in it.
	stopRow  *ImageIndexEntry
	stopPage int
	// issues holds one entry per failing object, indexed by the ids resWalk
	// carries; issueIDs finds them by key.
	issues   []*walkIssue
	issueIDs map[issueKey]int
	// pageMemo and formMemo hold finished walks, so resources shared by many
	// pages or forms are walked, charged to the budget and reported once.
	// formMemo is keyed by object number.
	pageMemo map[pageResKey]resWalk
	formMemo map[int]formWalk
}

func newImageWalker(ctx *pdfcpu_model.Context) *imageWalker {
	return &imageWalker{
		ctx:       ctx,
		budget:    maxImageWalkEntries,
		useLimit:  maxImageWalkUses,
		readFacts: readImageDictFacts,
		records:   map[int]*imageRecord{},
		pageMemo:  map[pageResKey]resWalk{},
		formMemo:  map[int]formWalk{},
		issueIDs:  map[issueKey]int{},
	}
}

// build walks every numbered page of pt and assembles the index, the page
// lists and the groups. Every unnumbered page-tree error row becomes an
// error row, and a page pdfcpu failed to read writes its error and is marked
// incomplete, though its resources are still walked. Every other failure
// writes one error row per failing object, naming every page it cut short. A
// page-tree walk that failed part way still yields the pages it reached, plus
// an error row naming the failure. Once the walk stops, later pages are not
// walked and get no group; the stop's error row names them.
func (w *imageWalker) build(pt *pageTree) *imageTree {
	for _, e := range pt.entries {
		if e.PageNum != 0 || e.Err == "" {
			continue
		}
		if e.NodeID != "" {
			w.addErrorRow(fmt.Sprintf("page tree (%d %d R): %s", e.ObjNum, e.Gen, e.Err))
		} else {
			w.addErrorRow("page tree: " + e.Err)
		}
	}
	groups := make([]ImagePageGroup, 0, len(pt.leaves))
	pageNodeIDs := make([]string, len(pt.leaves))
	for i, leaf := range pt.leaves {
		if leaf.ref != nil {
			pageNodeIDs[i] = nodeIDForRef(*leaf.ref)
		}
		g := ImagePageGroup{PageNum: i + 1, NodeID: pageNodeIDs[i], Images: []ImagePageUse{}}
		if leaf.err != nil {
			w.addErrorRow(leaf.err.Error())
			g.Incomplete = true
		}
		if !w.stopped && w.ctx != nil {
			rw := w.walkPage(leaf.attrs.resources, g.PageNum)
			g.Incomplete = g.Incomplete || rw.incomplete
			if w.chargeUses(len(rw.images), g.PageNum) {
				g.Images = append(g.Images, rw.images...)
				for _, u := range rw.images {
					if r := w.records[u.ObjNum]; r != nil {
						r.pages = append(r.pages, g.PageNum)
					}
				}
				for _, id := range rw.issues {
					w.noteIssuePage(id, g.PageNum)
				}
			} else {
				g.Incomplete = true
			}
		}
		if w.stopped && g.PageNum > w.stopPage {
			continue
		}
		if len(g.Images) > 0 || g.Incomplete {
			groups = append(groups, g)
		}
	}
	for _, is := range w.issues {
		if len(is.ranges) == 0 {
			is.ranges = [][2]int{{is.firstPage, is.firstPage}}
		}
		is.row.Err = is.prefix + pageRangeList(is.ranges) + is.suffix
	}
	if w.stopRow != nil {
		switch first, last := w.stopPage+1, len(pt.leaves); {
		case first == last:
			w.stopRow.Err += fmt.Sprintf("; page %d was not walked", first)
		case first < last:
			w.stopRow.Err += fmt.Sprintf("; pages %d to %d were not walked", first, last)
		}
	}
	if pt.err != nil {
		w.addErrorRow(fmt.Sprintf("the page tree could not be read past page %d: %v", len(pt.leaves), pt.err))
	}

	t := &imageTree{entries: make([]*ImageIndexEntry, 0, len(w.records)+len(w.errRows)), pages: map[int][]int{}, groups: groups, pageNodeIDs: pageNodeIDs}
	for _, r := range w.records {
		if len(r.pages) == 0 {
			continue
		}
		e := r.entry
		e.FirstPage = r.pages[0]
		e.PageCount = len(r.pages)
		e.FirstPages = slices.Clone(r.pages[:min(len(r.pages), maxImageFirstPages)])
		e.FirstPageNodeIDs = make([]string, len(e.FirstPages))
		for i, n := range e.FirstPages {
			e.FirstPageNodeIDs[i] = pageNodeIDs[n-1]
		}
		t.pages[e.ObjNum] = r.pages
		t.entries = append(t.entries, e)
	}
	slices.SortFunc(t.entries, func(a, b *ImageIndexEntry) int {
		if a.FirstPage != b.FirstPage {
			return a.FirstPage - b.FirstPage
		}
		if a.ObjNum != b.ObjNum {
			return a.ObjNum - b.ObjNum
		}
		return a.Gen - b.Gen
	})
	t.entries = append(t.entries, w.errRows...)
	return t
}

// walkPage walks the resources of page, or returns the memoised walk of
// resources an earlier page shares. A failure to read them is one walk issue
// per resources object and marks every page sharing them incomplete.
func (w *imageWalker) walkPage(res pdfcpu_types.Object, page int) resWalk {
	key, shared := pageResourcesKey(res)
	if shared {
		if rw, ok := w.pageMemo[key]; ok {
			return rw
		}
	}
	var rw resWalk
	fail := func(msg string) {
		k := issueKey{kind: issuePageResources, num: page}
		if shared {
			k = issueKey{kind: issueSharedResources, res: key}
		}
		rw = resWalk{incomplete: true, issues: []int{w.issue(k, page, "the resources of ", " could not be read: "+msg)}}
	}
	guard(func() {
		var err error
		if rw, _, err = w.walkResources(res, page, 0, map[int]int{}); err != nil {
			fail(err.Error())
		}
	}, fail)
	// A walk the budget stopped is partial; no later page is walked anyway.
	if shared && !w.stopped {
		w.pageMemo[key] = rw
	}
	return rw
}

// pageResourcesKey returns the memo key for a page's resolved /Resources
// value, read without resolving anything, and false when it has no /XObject
// dictionary to share.
func pageResourcesKey(res pdfcpu_types.Object) (pageResKey, bool) {
	d, ok := res.(pdfcpu_types.Dict)
	if !ok {
		return pageResKey{}, false
	}
	switch x := ownEntry(d, "XObject").(type) {
	case pdfcpu_types.IndirectRef:
		return pageResKey{num: x.ObjectNumber.Value()}, true
	case pdfcpu_types.Dict:
		// The page tree holds the dictionary for the document's lifetime, so
		// its address is not reused while the walk runs.
		return pageResKey{dict: reflect.ValueOf(x).Pointer()}, true
	}
	return pageResKey{}, false
}

// walkResources walks the /XObject entries of one resources dictionary in
// sorted key order: images are listed, forms are walked through their own
// /Resources and their images merged in. Each object is reached once per
// walk, so a diamond is walked once. stack maps each form being walked above
// res to its position; a form on it is a cycle and is skipped. depth counts
// the forms entered above res. low is the lowest stack position a skipped
// cycle reached, math.MaxInt when none: the walk depends on those forms
// being above it. err is set when the resources dictionary or its /XObject
// dictionary fails to resolve; a missing, null or dangling one, or one that
// is not a dictionary, contributes nothing.
func (w *imageWalker) walkResources(res pdfcpu_types.Object, page, depth int, stack map[int]int) (rw resWalk, low int, err error) {
	low = math.MaxInt
	resObj, err := w.ctx.Dereference(res)
	if err != nil {
		return rw, low, err
	}
	resDict, ok := resObj.(pdfcpu_types.Dict)
	if !ok {
		return rw, low, nil
	}
	xobjObj, err := w.ctx.Dereference(ownEntry(resDict, "XObject"))
	if err != nil {
		return rw, low, fmt.Errorf("/XObject: %w", err)
	}
	xobjects, ok := xobjObj.(pdfcpu_types.Dict)
	if !ok {
		return rw, low, nil
	}
	seen := map[int]bool{}
	for _, key := range sortedKeys(xobjects) {
		if w.stopped {
			return rw, low, nil
		}
		if w.examined >= w.budget {
			rw.incomplete = true
			w.stop(fmt.Sprintf("image walk stopped after %d resource entries at page %d", w.examined, page), page)
			return rw, low, nil
		}
		w.examined++

		// pdfcpu cannot hold a direct stream, so a direct value is not an image.
		ref, ok := xobjects[key].(pdfcpu_types.IndirectRef)
		if !ok {
			continue
		}
		num := ref.ObjectNumber.Value()
		if seen[num] {
			continue
		}
		if pos, onStack := stack[num]; onStack {
			low = min(low, pos)
			rw.cyclic = true
			continue
		}
		fail := func(msg string) {
			rw.incomplete = true
			rw.issues = append(rw.issues, w.issue(issueKey{kind: issueXObject, num: num}, page,
				fmt.Sprintf("/XObject entry /%s (%s) on ", key, refString(ref)), " could not be read: "+msg))
		}
		guard(func() {
			if err := w.visitXObject(ref, key, page, depth, stack, seen, &rw, &low); err != nil {
				fail(err.Error())
			}
		}, fail)
	}
	return rw, low, nil
}

// visitXObject classifies one referenced XObject and adds what it reaches to
// rw. /Subtype is read as a direct name, the way image extraction reads it,
// so every listed image previews; anything that is not an image or a form is
// skipped, as is a dangling reference. It returns the error when the XObject
// fails to resolve.
func (w *imageWalker) visitXObject(ref pdfcpu_types.IndirectRef, key string, page, depth int, stack map[int]int, seen map[int]bool, rw *resWalk, low *int) error {
	obj, err := w.ctx.Dereference(ref)
	if err != nil {
		return err
	}
	sd, ok := obj.(pdfcpu_types.StreamDict)
	if !ok {
		return nil
	}
	subtype, _ := sd.Find("Subtype")
	num, gen := ref.ObjectNumber.Value(), ref.GenerationNumber.Value()
	switch nameOf(subtype) {
	case "Image":
		seen[num] = true
		rw.images = append(rw.images, ImagePageUse{ObjNum: num, Gen: gen, Path: []string{key}})
		w.recordImage(ref, sd)
	case "Form":
		if depth >= maxFormWalkDepth {
			rw.incomplete = true
			rw.capped = true
			rw.issues = append(rw.issues, w.issue(issueKey{kind: issueDepth, num: num}, page,
				fmt.Sprintf("Form XObject nesting deeper than %d at %s on ", maxFormWalkDepth, refString(ref)), "; deeper forms were not walked"))
			return nil
		}
		seen[num] = true
		child := w.walkForm(ref, sd, key, page, depth, stack, low)
		rw.incomplete = rw.incomplete || child.incomplete
		rw.capped = rw.capped || child.capped
		rw.cyclic = rw.cyclic || child.cyclic
		rw.height = max(rw.height, child.height+1)
		for _, id := range child.issues {
			if !w.chargeUses(1, page) {
				rw.incomplete = true
				return nil
			}
			rw.issues = append(rw.issues, id)
		}
		for _, u := range child.images {
			if seen[u.ObjNum] {
				continue
			}
			if !w.chargeUses(1, page) {
				rw.incomplete = true
				return nil
			}
			seen[u.ObjNum] = true
			rw.images = append(rw.images, ImagePageUse{ObjNum: u.ObjNum, Gen: u.Gen, Path: append([]string{key}, u.Path...)})
		}
	}
	return nil
}

// chargeUses counts n more retained image uses and reports whether they fit
// under the use limit. When they do not, nothing is counted and the walk
// stops with an error row, unless it has already stopped.
func (w *imageWalker) chargeUses(n, page int) bool {
	if w.uses+n > w.useLimit {
		if !w.stopped {
			w.stop(fmt.Sprintf("image walk stopped at the limit of %d image uses at page %d", w.useLimit, page), page)
		}
		return false
	}
	w.uses += n
	return true
}

// stop ends the walk at page with an error row carrying msg.
func (w *imageWalker) stop(msg string, page int) {
	w.stopped = true
	w.stopPage = page
	w.addErrorRow(msg)
	w.stopRow = w.errRows[len(w.errRows)-1]
}

// walkForm walks a form's own /Resources (a form without one contributes
// nothing), or returns its memoised walk. A memoised walk that skipped no
// cycle and was not cut by the nesting cap is reused at any depth its height
// still fits under the cap; one the cap cut is reused only at the depth it was
// walked at. One that skipped a cycle depends on the forms being walked above
// it, so it is reused only when the form is again entered straight from page
// resources. A walk that skipped a cycle back to a form above this one is not
// memoised at all; low carries the cycle up. A form whose resources cannot be
// read is one walk issue, and the failure is memoised.
func (w *imageWalker) walkForm(ref pdfcpu_types.IndirectRef, sd pdfcpu_types.StreamDict, key string, page, depth int, stack map[int]int, low *int) resWalk {
	num := ref.ObjectNumber.Value()
	memo, memoised := w.formMemo[num]
	if memoised {
		switch {
		case depth == 0 && memo.depth == 0:
			// No form above in either walk, so the context is the same.
			return memo.rw
		case memo.rw.cyclic:
		case memo.rw.capped:
			if memo.depth == depth {
				return memo.rw
			}
		case depth+memo.rw.height < maxFormWalkDepth:
			// The form's resources are walked at depth+1, so the deepest
			// form in them is checked against the cap at depth+height.
			return memo.rw
		}
	}
	pos := len(stack)
	stack[num] = pos
	var rw resWalk
	childLow := math.MaxInt
	fail := func(msg string) {
		rw = resWalk{incomplete: true, issues: []int{w.issue(issueKey{kind: issueXObject, num: num}, page,
			fmt.Sprintf("/XObject entry /%s (%s) on ", key, refString(ref)), " could not be read: "+msg)}}
	}
	guard(func() {
		var err error
		if rw, childLow, err = w.walkResources(ownEntry(sd.Dict, "Resources"), page, depth+1, stack); err != nil {
			fail("/Resources: " + err.Error())
		}
	}, fail)
	delete(stack, num)
	if childLow < pos {
		*low = min(*low, childLow)
	} else if !w.stopped && (!memoised || depth <= memo.depth) {
		w.formMemo[num] = formWalk{rw: rw, depth: depth}
	}
	return rw
}

// recordImage notes the image ref, reading its facts the first time it is
// seen anywhere. A facts read that panics keeps the entry with Err set. The
// pages using it are added once each page's walk is done.
func (w *imageWalker) recordImage(ref pdfcpu_types.IndirectRef, sd pdfcpu_types.StreamDict) {
	num, gen := ref.ObjectNumber.Value(), ref.GenerationNumber.Value()
	if w.records[num] != nil {
		return
	}
	e := &ImageIndexEntry{ObjNum: num, Gen: gen, NodeID: nodeIDForRef(ref), Filters: []string{}, FirstPages: []int{}, FirstPageNodeIDs: []string{}}
	guard(func() {
		setImageFacts(e, w.readFacts(w.ctx.XRefTable, &sd))
	}, func(msg string) {
		e.Err = "image dictionary could not be read: " + msg
	})
	w.records[num] = &imageRecord{entry: e}
}

// setImageFacts copies the dictionary facts into e. Warning joins the
// metadata-read warnings with a rejected /Decode.
func setImageFacts(e *ImageIndexEntry, f imageDictFacts) {
	e.Width = f.width
	e.Height = f.height
	e.BitsPerComponent = f.bitsPerComponent
	e.ColorSpace = f.colorSpace
	e.Filters = f.filters
	e.ImageMask = f.imageMask
	e.SMask = f.smask
	e.Decode = f.decode
	e.DecodeNonDefault = f.decodeNonDefault
	e.SampleInterpretation = f.sampleInterpretation
	e.AdobeMarker = f.adobeMarker
	e.AdobeTransform = f.adobeTransform
	e.EstimatedBytes = f.estimatedBytes
	e.Warning = f.warning
	if f.decodeErr != nil {
		e.Warning = appendWarning(e.Warning, fmt.Sprintf("decode array metadata: %v", f.decodeErr))
	}
}

// issue returns the id of the walk issue for k, first found on page, adding
// it and its error row when k is new. The row's message is prefix, the pages
// the issue cut short, suffix; build writes it once every page is walked.
func (w *imageWalker) issue(k issueKey, page int, prefix, suffix string) int {
	if id, ok := w.issueIDs[k]; ok {
		return id
	}
	w.addErrorRow(prefix + "page " + strconv.Itoa(page) + suffix)
	id := len(w.issues)
	w.issues = append(w.issues, &walkIssue{row: w.errRows[len(w.errRows)-1], prefix: prefix, suffix: suffix, firstPage: page})
	w.issueIDs[k] = id
	return id
}

// noteIssuePage adds page to the pages issue id cut short. Pages arrive in
// ascending order, so a page next to the last range extends it; a new range
// is charged as one image use, and is dropped when the use limit stops the
// walk, whose own row then names the page.
func (w *imageWalker) noteIssuePage(id, page int) {
	is := w.issues[id]
	if n := len(is.ranges); n > 0 && is.ranges[n-1][1] >= page-1 {
		is.ranges[n-1][1] = max(is.ranges[n-1][1], page)
		return
	}
	if w.chargeUses(1, page) {
		is.ranges = append(is.ranges, [2]int{page, page})
	}
}

// pageRangeList renders ascending inclusive page ranges as "page 3" or
// "pages 2-4, 7".
func pageRangeList(ranges [][2]int) string {
	if len(ranges) == 1 && ranges[0][0] == ranges[0][1] {
		return "page " + strconv.Itoa(ranges[0][0])
	}
	var b strings.Builder
	b.WriteString("pages ")
	for i, r := range ranges {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.Itoa(r[0]))
		if r[1] != r[0] {
			b.WriteString("-" + strconv.Itoa(r[1]))
		}
	}
	return b.String()
}

// addErrorRow appends an error row carrying msg.
func (w *imageWalker) addErrorRow(msg string) {
	w.errRows = append(w.errRows, &ImageIndexEntry{Filters: []string{}, FirstPages: []int{}, FirstPageNodeIDs: []string{}, Err: msg})
}
