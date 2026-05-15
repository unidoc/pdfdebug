package pdfcore

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// GetFontResourceMap resolves nodeID to a dict and returns a FontResourceMap
// summarizing every entry that points at a /Type /Font dict. Used when the
// iconHint='font' heuristic fires on the /Resources /Font resource map (a
// dict of font-name -> Font dict reference), which is NOT itself a font.
//
// Non-font entries are kept with Unresolved=true so the rendered table still
// reflects the resource-map shape. Returns ErrNotAFont when ZERO entries
// resolve to a Font dict, signalling the frontend to fall back to the generic
// DictView (the dict isn't a font resource map either).
//
// Deprecated: callers in the running app go through GetFontView which never
// surfaces ErrNotAFont. This method is retained for the unit tests that pin
// its sentinel contract.
func (ins *Inspector) GetFontResourceMap(tabID, nodeID string) (*FontResourceMap, error) {
	if nodeID == "" {
		return nil, fmt.Errorf("%w: empty node ID", ErrDocumentNotFound)
	}
	if strings.HasPrefix(nodeID, "error:") {
		return nil, fmt.Errorf("%w: error node", ErrNotAFont)
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

	d := asDict(obj)
	if d == nil {
		return nil, ErrNotAFont
	}

	roster := buildFontRoster(doc, nodeID, d)
	if len(roster.Entries) == 0 || !rosterHasResolved(roster) {
		return nil, ErrNotAFont
	}
	return roster, nil
}

// buildFontRoster walks every entry of d and emits a FontResourceMap row per
// entry. Rows whose value does not resolve to a /Type /Font dict are kept with
// Unresolved=true so the eventual table preserves the resource-map shape.
// Entries are sorted by Name for deterministic emission.
//
// The returned roster is always non-nil; callers decide whether a zero-resolved
// roster should be surfaced (GetFontView -> Kind:"neither") or rejected
// (GetFontResourceMap -> ErrNotAFont).
func buildFontRoster(doc *DocumentState, nodeID string, d pdfcpu_types.Dict) *FontResourceMap {
	entries := make([]FontRosterEntry, 0, len(d))
	for key, val := range d {
		entry := FontRosterEntry{Name: key}

		// Capture the indirect-ref form before dereferencing so we can render
		// the per-row "N G R" badge even when the resolved dict is unresolved.
		if ref, isRef := val.(pdfcpu_types.IndirectRef); isRef {
			num := ref.ObjectNumber.Value()
			gen := ref.GenerationNumber.Value()
			entry.NodeID = fmt.Sprintf("obj:%d:%d", gen, num)
			entry.ObjectRef = fmt.Sprintf("%d %d R", num, gen)
		}

		resolved := dereferenceIfRef(doc, val)
		rd := asDict(resolved)
		if rd == nil || !isFontDict(rd) {
			entry.Unresolved = true
			entries = append(entries, entry)
			continue
		}

		populateRosterEntry(doc, rd, &entry)
		entries = append(entries, entry)
	}

	slices.SortFunc(entries, func(a, b FontRosterEntry) int {
		return cmp.Compare(a.Name, b.Name)
	})

	return &FontResourceMap{
		NodeID:  nodeID,
		Entries: entries,
	}
}

// rosterHasResolved reports whether roster contains at least one entry whose
// value resolved to a /Type /Font dict (i.e. not Unresolved). Used to detect
// the "this dict isn't a font resource map" case.
func rosterHasResolved(roster *FontResourceMap) bool {
	for i := range roster.Entries {
		if !roster.Entries[i].Unresolved {
			return true
		}
	}
	return false
}

// populateRosterEntry fills the per-row summary fields from a /Type /Font dict.
// Embedded detection mirrors buildFontDetailFromDict: for Type0 the FontFile
// lives on the descendant CIDFont's FontDescriptor, otherwise on the font's
// own FontDescriptor.
func populateRosterEntry(doc *DocumentState, d pdfcpu_types.Dict, entry *FontRosterEntry) {
	if name, ok := nameField(d, "BaseFont"); ok {
		entry.BaseFont = "/" + name
	}
	if name, ok := nameField(d, "Subtype"); ok {
		entry.Subtype = name
	}
	entry.EncodingSummary = encodingSummaryForDict(doc, d)
	entry.Embedded = fontHasEmbeddedProgram(doc, d, 0)
}

// encodingSummaryForDict returns a short label for /Encoding suitable for a
// table column: the named encoding (e.g. "/WinAnsiEncoding"), "Differences"
// when /Encoding is a dict with a /Differences array, or "Built-in" when the
// key is absent.
func encodingSummaryForDict(doc *DocumentState, d pdfcpu_types.Dict) string {
	v, ok := d["Encoding"]
	if !ok {
		return "Built-in"
	}
	v = dereferenceIfRef(doc, v)
	switch x := v.(type) {
	case pdfcpu_types.Name:
		return "/" + string(x)
	case pdfcpu_types.Dict:
		if _, hasDiffs := x["Differences"]; hasDiffs {
			return "Differences"
		}
		if base, ok := nameField(x, "BaseEncoding"); ok {
			return "/" + base
		}
		return "Dict"
	}
	return "Built-in"
}

// fontHasEmbeddedProgram reports whether the font dict carries an embedded
// font program. For Type0 fonts the FontFile lives on the descendant's
// FontDescriptor; the depth cap mirrors buildFontDetailFromDictDepth to
// guard against /DescendantFonts cycles.
func fontHasEmbeddedProgram(doc *DocumentState, d pdfcpu_types.Dict, depth int) bool {
	subtype, _ := nameField(d, "Subtype")
	if subtype == "Type0" && depth < maxDescendantDepth {
		if descObj, ok := d["DescendantFonts"]; ok {
			descObj = dereferenceIfRef(doc, descObj)
			if arr, ok := descObj.(pdfcpu_types.Array); ok && len(arr) > 0 {
				first := dereferenceIfRef(doc, arr[0])
				if dd, ok := first.(pdfcpu_types.Dict); ok && isFontDict(dd) {
					return fontHasEmbeddedProgram(doc, dd, depth+1)
				}
			}
		}
		return false
	}
	fdObj, ok := d["FontDescriptor"]
	if !ok {
		return false
	}
	fdObj = dereferenceIfRef(doc, fdObj)
	fd, ok := fdObj.(pdfcpu_types.Dict)
	if !ok {
		return false
	}
	format, _ := detectFontFile(doc, fd)
	return format != ""
}
