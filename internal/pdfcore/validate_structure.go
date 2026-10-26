package pdfcore

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	pdfcpu_model "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// --- PDF/UA-1 structure report rule checks -----------------------------------

// buildValidateStructure builds the structure report for a validate run. Any
// panic in the walk, a runtime.Error included, becomes the returned error, so
// the structure rules degrade instead of the run failing. buildStructure
// releases pdfMu through defer, so the lock is free after a recovered panic.
func buildValidateStructure(doc *DocumentState, newWalker func(*pdfcpu_model.Context) *structureWalker) (rep *StructureReport, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			rep = nil
			err = fmt.Errorf("%v", rec)
		}
	}()
	return doc.buildStructure(newWalker(doc.PDFContext))
}

// profileReadsStructure reports whether any rule of profile reads the
// structure report.
func profileReadsStructure(profile string) bool {
	for _, r := range ruleRegistry {
		if r.profile == profile && r.checkStructure != nil {
			return true
		}
	}
	return false
}

// asciiText returns s unchanged when it is printable ASCII, else quoted with
// strconv.QuoteToASCII, so text taken from the file keeps messages ASCII.
func asciiText(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return strconv.QuoteToASCII(s)
		}
	}
	return s
}

// objHit is a finding on the object with reference ref ("N G R") and tree
// node nodeID, or a document-level finding when nodeID is empty.
func objHit(ref, nodeID, msg string) ruleHit {
	if nodeID == "" {
		return ruleHit{message: msg}
	}
	return ruleHit{message: msg, objRef: ref, objNodeID: nodeID}
}

// pagesComplete returns an error when the page-tree walk did not list every
// page, so a content rule is not evaluated instead of passing on the pages it
// saw.
func pagesComplete(rep *StructureReport) error {
	if rep.pagesIncomplete != "" {
		return fmt.Errorf("could not read every page: %s", asciiText(rep.pagesIncomplete))
	}
	return nil
}

// firstUndecodedStream returns the index of the page's first content stream
// that failed to decode, or len(sp.Streams) when every stream decoded. The
// scan stack is unknown from that stream on.
func firstUndecodedStream(sp *StructPage) int {
	for i, ms := range sp.Streams {
		if ms.Err != "" {
			return i
		}
	}
	return len(sp.Streams)
}

// mcidLoc renders the location of a content row as "stream R line L col C".
func mcidLoc(sp *StructPage, c ContentMCID) string {
	ref := "?"
	if c.Stream >= 0 && c.Stream < len(sp.Streams) {
		ref = sp.Streams[c.Stream].Ref
	}
	return fmt.Sprintf("stream %s line %d col %d", ref, c.Line, c.Col)
}

// mcidOccurrences groups a page's content rows with a readable MCID by MCID
// and returns the MCIDs in order of first occurrence with their locations in
// content order.
func mcidOccurrences(sp *StructPage) ([]int, map[int][]string) {
	locs := map[int][]string{}
	var mcids []int
	for _, c := range sp.ContentMCIDs {
		if c.MCID < 0 {
			continue
		}
		if _, seen := locs[c.MCID]; !seen {
			mcids = append(mcids, c.MCID)
		}
		locs[c.MCID] = append(locs[c.MCID], mcidLoc(sp, c))
	}
	return mcids, locs
}

// checkMarkedContentBalance flags, per page, unreadable page content, content
// streams that fail to decode, EMC operators with no open sequence and
// sequences still open at the end of the page's /Contents, which is read as
// one stream (ISO 32000-1 14.6.1). From a stream that failed to decode on, the
// scan stack is unknown, so later streams of that page are not judged for
// balance and no open-at-end finding is made for it.
func checkMarkedContentBalance(_ *DocumentState, rep *StructureReport) ([]ruleHit, error) {
	if err := pagesComplete(rep); err != nil {
		return nil, err
	}
	var hits []ruleHit
	for _, sp := range rep.Pages {
		if sp.Err != "" {
			hits = append(hits, objHit(sp.Ref, sp.nodeID, fmt.Sprintf("page %d content could not be read (%s); marked content not checked", sp.PageNum, asciiText(sp.Err))))
		}
		bad := firstUndecodedStream(sp)
		for i, ms := range sp.Streams {
			switch {
			case ms.Err != "":
				hits = append(hits, objHit(ms.Ref, ms.nodeID, fmt.Sprintf("content stream %s of page %d could not be decoded (%s); its marked content was not checked", ms.Ref, sp.PageNum, asciiText(ms.Err))))
			case i < bad && ms.MinDepth < 0:
				hits = append(hits, objHit(sp.Ref, sp.nodeID, fmt.Sprintf("page %d: EMC with no open marked-content sequence (in content stream %s)", sp.PageNum, ms.Ref)))
			}
		}
		if sp.Err == "" && bad == len(sp.Streams) && len(sp.Streams) > 0 {
			if open := sp.Streams[len(sp.Streams)-1].DepthAtEnd; open > 0 {
				hits = append(hits, objHit(sp.Ref, sp.nodeID, fmt.Sprintf("page %d: %d marked-content sequence(s) still open at the end of the page's content", sp.PageNum, open)))
			}
		}
	}
	return hits, nil
}

// checkMCIDUnique flags, per page, every MCID that marks more than one BDC in
// the page's content, listing every occurrence (ISO 32000-1 14.7.4.2).
func checkMCIDUnique(_ *DocumentState, rep *StructureReport) ([]ruleHit, error) {
	if err := pagesComplete(rep); err != nil {
		return nil, err
	}
	var hits []ruleHit
	for _, sp := range rep.Pages {
		mcids, locs := mcidOccurrences(sp)
		for _, m := range mcids {
			if l := locs[m]; len(l) > 1 {
				hits = append(hits, objHit(sp.Ref, sp.nodeID, fmt.Sprintf("page %d: MCID %d marks %d BDC sequences: %s", sp.PageNum, m, len(l), strings.Join(l, ", "))))
			}
		}
	}
	return hits, nil
}

// checkMCIDInParentTree flags, per page, content MCIDs that do not reach a
// structure element through the page's /StructParents and its ParentTree
// array (ISO 32000-1 14.7.4.4), and content whose MCIDs could not be checked:
// page content that could not be read, a content stream that failed to
// decode, and every BDC whose MCID could not be read. Per page, the content
// that could not be read comes first, then the MCIDs in content order, each
// at its first occurrence. A page-wide cause (no /StructParents, a ParentTree
// entry that is missing or unreadable) is one finding naming every MCID of the
// page in ascending order. A structure tree root whose read failed part way,
// or ParentTree entries that were not read, leave the rule not evaluated.
func checkMCIDInParentTree(_ *DocumentState, rep *StructureReport) ([]ruleHit, error) {
	if err := pagesComplete(rep); err != nil {
		return nil, err
	}
	if rep.rootReadFailed != "" {
		return nil, fmt.Errorf("structure tree root could not be read, so the ParentTree is unknown: %s", asciiText(rep.rootReadFailed))
	}
	if rep.parentTreeIncomplete != "" {
		return nil, fmt.Errorf("the ParentTree could not be read in full, so its entries are unknown: %s", asciiText(rep.parentTreeIncomplete))
	}
	var hits []ruleHit
	for _, sp := range rep.Pages {
		if sp.Err != "" {
			hits = append(hits, objHit(sp.Ref, sp.nodeID, fmt.Sprintf("page %d content could not be read (%s); its MCIDs were not checked against the ParentTree", sp.PageNum, asciiText(sp.Err))))
		}
		for _, ms := range sp.Streams {
			if ms.Err != "" {
				hits = append(hits, objHit(ms.Ref, ms.nodeID, fmt.Sprintf("content stream %s of page %d could not be decoded (%s); its MCIDs were not checked against the ParentTree", ms.Ref, sp.PageNum, asciiText(ms.Err))))
			}
		}
		mcids, locs := mcidOccurrences(sp)
		cause := ""
		switch {
		case sp.ParentTreeErr != "":
			cause = asciiText(sp.ParentTreeErr)
		case sp.StructParents == nil:
			cause = "page has no /StructParents"
		}
		if len(mcids) > 0 && cause != "" {
			sorted := slices.Sorted(slices.Values(mcids))
			list := make([]string, len(sorted))
			for i, m := range sorted {
				list[i] = strconv.Itoa(m)
			}
			hits = append(hits, objHit(sp.Ref, sp.nodeID, fmt.Sprintf("page %d: MCIDs %s not reachable: %s", sp.PageNum, strings.Join(list, ", "), cause)))
		}
		reported := map[int]bool{}
		for _, c := range sp.ContentMCIDs {
			if c.MCID < 0 {
				hits = append(hits, objHit(sp.Ref, sp.nodeID, fmt.Sprintf("page %d: BDC at %s has no readable MCID (%s); not checked against the ParentTree", sp.PageNum, mcidLoc(sp, c), asciiText(c.Err))))
				continue
			}
			m := c.MCID
			if cause != "" || reported[m] {
				continue
			}
			reported[m] = true
			var slot ParentTreeSlot
			if m < len(sp.ParentTree) {
				slot = sp.ParentTree[m]
			}
			if slot.ElementRef != "" && slot.Err == "" {
				continue
			}
			msg := fmt.Sprintf("page %d: MCID %d (%s) has no structure element in ParentTree key %d", sp.PageNum, m, strings.Join(locs[m], ", "), *sp.StructParents)
			if slot.Err != "" {
				msg += ": " + asciiText(slot.Err)
			}
			hits = append(hits, objHit(sp.Ref, sp.nodeID, msg))
		}
	}
	return hits, nil
}

// checkMCIDNotNested flags every BDC carrying an MCID that opens inside
// another open sequence carrying an MCID (ISO 32000-1 14.7.4.1). BMC and
// MCID-less BDC sequences may wrap one. Rows in or after a page's first
// content stream that failed to decode are not judged, since their enclosing
// sequences are unknown.
func checkMCIDNotNested(_ *DocumentState, rep *StructureReport) ([]ruleHit, error) {
	if err := pagesComplete(rep); err != nil {
		return nil, err
	}
	var hits []ruleHit
	for _, sp := range rep.Pages {
		bad := firstUndecodedStream(sp)
		for _, c := range sp.ContentMCIDs {
			if c.MCID < 0 || c.ParentMCID == nil || c.Stream >= bad {
				continue
			}
			hits = append(hits, objHit(sp.Ref, sp.nodeID, fmt.Sprintf("page %d: MCID %d (%s) is nested inside MCID %d", sp.PageNum, c.MCID, mcidLoc(sp, c), *c.ParentMCID)))
		}
	}
	return hits, nil
}

// checkStructElemType flags every structure element reached from
// /StructTreeRoot /K whose /S is absent, empty or not a name (ISO 32000-1
// 14.7.2), and every element whose read failed before its /S was read, as not
// checked. A structure tree root that is not a dictionary or could not be read
// leaves the rule not evaluated; an absent root, a dangling reference
// included, is struct-tree-root's finding, not this rule's.
func checkStructElemType(_ *DocumentState, rep *StructureReport) ([]ruleHit, error) {
	if msg := cmp.Or(rep.rootNotDict, rep.rootReadFailed); msg != "" {
		return nil, fmt.Errorf("structure tree root could not be read: %s", asciiText(msg))
	}
	var hits []ruleHit
	var walk func(kids []*StructKid, owner string)
	walk = func(kids []*StructKid, owner string) {
		for _, k := range kids {
			if k.Kind != "element" || k.Element == nil {
				continue
			}
			e := k.Element
			at := fmt.Sprintf("/K index %d of %s", k.index, owner)
			name := "direct structure element at " + at
			if e.Ref != "" {
				name = "structure element " + e.Ref
			}
			switch {
			case e.sErr != "":
				hits = append(hits, objHit(e.Ref, e.NodeID, fmt.Sprintf("%s has no usable /S (%s)", name, e.sErr)))
			case e.S == "" && e.readErr != "":
				hits = append(hits, objHit(e.Ref, e.NodeID, fmt.Sprintf("%s could not be read (%s); its /S was not checked", name, asciiText(e.readErr))))
			}
			next := e.Ref
			if next == "" {
				next = at
			}
			walk(e.Kids, next)
		}
	}
	walk(rep.Elements, "the structure tree root")
	return hits, nil
}
