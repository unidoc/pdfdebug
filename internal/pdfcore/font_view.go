package pdfcore

import (
	"strings"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// FontView Kind constants. Strings are the JSON wire form consumed by the
// frontend; keep them in sync with FontView.Kind on the TS side.
const (
	FontViewKindDetail  = "detail"
	FontViewKindRoster  = "roster"
	FontViewKindNeither = "neither"
)

// GetFontView is the unified font-inspection endpoint. It resolves nodeID and
// returns a single FontView describing whether the node is a /Type /Font dict
// (Kind=="detail"), a /Resources /Font roster (Kind=="roster"), or neither
// (Kind=="neither"). The "neither" case is NOT an error -- it is the normal
// negative outcome for the iconHint='font' false positive, so a click on a
// /Resources /Font node no longer produces a Wails ERR log.
//
// Real failures (unknown tab, malformed PDF, pdfcpu panics caught by safeCall)
// still return Go errors. ErrNotAFont is intentionally never returned here.
func (ins *Inspector) GetFontView(tabID, nodeID string) (*FontView, error) {
	if nodeID == "" || strings.HasPrefix(nodeID, "error:") {
		return &FontView{Kind: FontViewKindNeither}, nil
	}

	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	// AC1: serialize pdfcpu access. buildFontDetailFromDict and
	// buildFontRoster both dereference indirect refs.
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	var obj pdfcpu_types.Object
	err = safeCall(func() error {
		var e error
		obj, e = resolveNodeObject(doc, nodeID)
		return e
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}

	d := asDict(obj)
	if d == nil {
		return &FontView{Kind: FontViewKindNeither}, nil
	}

	if isFontDict(d) {
		return &FontView{
			Kind:   FontViewKindDetail,
			Detail: buildFontDetailFromDict(doc, nodeID, d),
		}, nil
	}

	roster := buildFontRoster(doc, nodeID, d)
	if !rosterHasResolved(roster) {
		return &FontView{Kind: FontViewKindNeither}, nil
	}
	return &FontView{
		Kind:   FontViewKindRoster,
		Roster: roster,
	}, nil
}
