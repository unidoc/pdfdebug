package pdfcore

import (
	"fmt"
	"slices"

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

// GetImageIndex returns the image index for the document in tabID: one entry
// per image XObject referenced from page resources (directly, through
// inherited /Resources, or through nested Form XObjects), deduplicated by
// object reference and ordered by first-use page then object number, followed
// by error rows (NodeID "") for every point where the walk stopped. "Used on"
// means declared in the page's resources; content streams are not parsed, so
// inline images are not listed. Built on first call from the cached page-tree
// walk, decode-free, and cached on the per-tab DocumentState; a re-Open under
// the same tabID replaces the DocumentState, and with it the cache. A document
// with no images returns a non-nil empty slice.
func (ins *Inspector) GetImageIndex(tabID string) ([]*ImageIndexEntry, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	// Serialize pdfcpu access. Outer lock; the pageIndex and imageIndex cache
	// mutexes (inner) guard the cached walks.
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	return doc.imageTree().entries, nil
}

// GetImagePages returns every page number, ascending, whose resources
// reference the image with object number objNum. It reads the cached image
// walk, so it is the full list ImageIndexEntry.FirstPages caps. An object
// number the index does not list is an error naming it.
func (ins *Inspector) GetImagePages(tabID string, objNum int) ([]int, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	pages, ok := doc.imageTree().pages[objNum]
	if !ok {
		return nil, fmt.Errorf("object %d is not an image in the image index", objNum)
	}
	return pages, nil
}

// GetImagePageGroups returns one group per numbered page in document order,
// each listing the images reached from that page's resources in walk order
// with the resource-name path to each. An image reachable twice within one
// page is listed once, under the first path found. Read from the cached
// image walk.
func (ins *Inspector) GetImagePageGroups(tabID string) ([]ImagePageGroup, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	return doc.imageTree().groups, nil
}

// imageTree is the result of one image walk: the index rows, the full page
// list per image object number, and the per-page groups.
type imageTree struct {
	entries []*ImageIndexEntry
	pages   map[int][]int
	groups  []ImagePageGroup
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

// imageWalker carries the state of one image walk.
type imageWalker struct {
	ctx *pdfcpu_model.Context
	// budget is the number of /XObject entries the walk may examine.
	budget   int
	examined int
	stopped  bool
	// readFacts reads one image dictionary; a field so tests can make it fail.
	readFacts func(*pdfcpu_model.XRefTable, *pdfcpu_types.StreamDict) imageDictFacts
	// records is keyed by object number alone, as pdfcpu resolves references.
	records map[int]*imageRecord
	errRows []*ImageIndexEntry
	// depthCapped is set once the form nesting cap has written its error row.
	depthCapped bool
}

func newImageWalker(ctx *pdfcpu_model.Context) *imageWalker {
	return &imageWalker{
		ctx:       ctx,
		budget:    maxImageWalkEntries,
		readFacts: readImageDictFacts,
		records:   map[int]*imageRecord{},
	}
}

// build walks every numbered page of pt and assembles the index, the page
// lists and the groups. A page-tree walk that failed part way still yields
// the pages it reached, plus an error row naming the failure.
func (w *imageWalker) build(pt *pageTree) *imageTree {
	groups := make([]ImagePageGroup, 0, len(pt.leaves))
	for i, leaf := range pt.leaves {
		g := ImagePageGroup{PageNum: i + 1, Images: []ImagePageUse{}}
		if w.stopped {
			g.Incomplete = true
		} else if w.ctx != nil {
			guard(func() {
				w.walkResources(leaf.attrs.resources, &g, nil, 0, map[int]bool{})
			}, func(msg string) {
				g.Incomplete = true
				w.addErrorRow(fmt.Sprintf("the resources of page %d could not be read: %s", g.PageNum, msg))
			})
		}
		groups = append(groups, g)
	}
	if pt.err != nil {
		w.addErrorRow(fmt.Sprintf("the page tree could not be read past page %d: %v", len(pt.leaves), pt.err))
	}

	t := &imageTree{entries: make([]*ImageIndexEntry, 0, len(w.records)+len(w.errRows)), pages: map[int][]int{}, groups: groups}
	for _, r := range w.records {
		e := r.entry
		e.FirstPage = r.pages[0]
		e.PageCount = len(r.pages)
		e.FirstPages = slices.Clone(r.pages[:min(len(r.pages), maxImageFirstPages)])
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

// walkResources visits the /XObject entries of one resources dictionary in
// sorted key order: images are recorded for g's page, forms are descended
// into through their own /Resources. visited holds the object number of every
// image and form already reached on this page, so a diamond or a cycle is
// walked once. depth
// counts the forms entered above res.
func (w *imageWalker) walkResources(res pdfcpu_types.Object, g *ImagePageGroup, path []string, depth int, visited map[int]bool) {
	resDict, ok := derefIn(w.ctx, res).(pdfcpu_types.Dict)
	if !ok {
		return
	}
	xobjects, ok := derefIn(w.ctx, ownEntry(resDict, "XObject")).(pdfcpu_types.Dict)
	if !ok {
		return
	}
	for _, key := range sortedKeys(xobjects) {
		if w.stopped {
			return
		}
		if w.examined >= w.budget {
			w.stopped = true
			g.Incomplete = true
			w.addErrorRow(fmt.Sprintf("image walk stopped after %d resource entries at page %d; later pages were not walked", w.examined, g.PageNum))
			return
		}
		w.examined++

		// pdfcpu cannot hold a direct stream, so a direct value is not an image.
		ref, ok := xobjects[key].(pdfcpu_types.IndirectRef)
		if !ok {
			continue
		}
		if visited[ref.ObjectNumber.Value()] {
			continue
		}
		guard(func() {
			w.visitXObject(ref, key, g, path, depth, visited)
		}, func(msg string) {
			g.Incomplete = true
			w.addErrorRow(fmt.Sprintf("/XObject entry /%s (%s) on page %d could not be read: %s", key, refString(ref), g.PageNum, msg))
		})
	}
}

// visitXObject classifies one referenced XObject. /Subtype is read as a
// direct name, the way image extraction reads it, so every listed image
// previews; anything that is not an image or a form is skipped.
func (w *imageWalker) visitXObject(ref pdfcpu_types.IndirectRef, key string, g *ImagePageGroup, path []string, depth int, visited map[int]bool) {
	sd, ok := derefIn(w.ctx, ref).(pdfcpu_types.StreamDict)
	if !ok {
		return
	}
	subtype, _ := sd.Find("Subtype")
	num, gen := ref.ObjectNumber.Value(), ref.GenerationNumber.Value()
	switch nameOf(subtype) {
	case "Image":
		visited[num] = true
		g.Images = append(g.Images, ImagePageUse{ObjNum: num, Gen: gen, Path: append(slices.Clone(path), key)})
		w.recordUse(ref, sd, g.PageNum)
	case "Form":
		if depth >= maxFormWalkDepth {
			g.Incomplete = true
			if !w.depthCapped {
				w.depthCapped = true
				w.addErrorRow(fmt.Sprintf("Form XObject nesting deeper than %d at %s on page %d; deeper forms were not walked", maxFormWalkDepth, refString(ref), g.PageNum))
			}
			return
		}
		visited[num] = true
		// A form's own /Resources only; a form without one contributes nothing.
		w.walkResources(ownEntry(sd.Dict, "Resources"), g, append(slices.Clone(path), key), depth+1, visited)
	}
}

// recordUse notes that page references the image ref, reading the image's
// facts the first time it is seen anywhere. A facts read that panics keeps
// the entry with Err set.
func (w *imageWalker) recordUse(ref pdfcpu_types.IndirectRef, sd pdfcpu_types.StreamDict, page int) {
	num, gen := ref.ObjectNumber.Value(), ref.GenerationNumber.Value()
	r := w.records[num]
	if r == nil {
		e := &ImageIndexEntry{ObjNum: num, Gen: gen, NodeID: nodeIDForRef(ref), Filters: []string{}, FirstPages: []int{}}
		guard(func() {
			setImageFacts(e, w.readFacts(w.ctx.XRefTable, &sd))
		}, func(msg string) {
			e.Err = "image dictionary could not be read: " + msg
		})
		r = &imageRecord{entry: e}
		w.records[num] = r
	}
	if n := len(r.pages); n == 0 || r.pages[n-1] != page {
		r.pages = append(r.pages, page)
	}
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

// addErrorRow appends an error row carrying msg.
func (w *imageWalker) addErrorRow(msg string) {
	w.errRows = append(w.errRows, &ImageIndexEntry{Filters: []string{}, FirstPages: []int{}, Err: msg})
}
