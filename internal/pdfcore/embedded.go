package pdfcore

import (
	"errors"
	"fmt"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// EmbeddedFile is one attachment surfaced from a PDF's catalog /AF
// (associated files) array or its /Names/EmbeddedFiles name tree. The
// /Filespec indirect-object reference is the stable identity of an attachment;
// a filespec reachable from both sources appears once (deduped by FilespecRef).
// Direct (inline) filespecs carry an empty FilespecRef and are never deduped.
type EmbeddedFile struct {
	// Name is the display name: /UF preferred, else /F.
	Name string `json:"name"`
	// FilespecRef is the "N G R" object ref of the /Filespec dict. Empty for a
	// direct (inline, non-indirect) filespec.
	FilespecRef string `json:"filespecRef"`
	// EmbeddedFileRef is the "N G R" object ref of the /EmbeddedFile stream.
	// Empty when the filespec has no resolvable /EF /F (or /UF) stream.
	EmbeddedFileRef string `json:"embeddedFileRef"`
	// EmbeddedFileNodeID is the obj:G:N tree-node id of the /EmbeddedFile
	// stream, the address GetEmbeddedFileBytes and the GUI "Save..." path use.
	// Empty when there is no resolvable stream.
	EmbeddedFileNodeID string `json:"embeddedFileNodeId"`
	// AFRelationship is the /AFRelationship name (e.g. Source, Data,
	// Alternative) - the ZUGFeRD/Factur-X discriminator. Empty when absent.
	AFRelationship string `json:"afRelationship"`
	// Subtype is the decoded /Subtype MIME of the /EmbeddedFile (e.g.
	// text/xml). Empty when absent.
	Subtype string `json:"subtype"`
	// Size is the decoded byte size of the /EmbeddedFile stream (the /Params
	// /Size when present, else the resolved stream length). Zero when unknown.
	Size int64 `json:"size"`
	// CheckSum is the /Params /CheckSum when present.
	CheckSum string `json:"checkSum,omitempty"`
	// ModDate is the /Params /ModDate when present.
	ModDate string `json:"modDate,omitempty"`
	// Warning is a per-entry degradation note (e.g. missing /EF) so a broken
	// attachment is listed rather than dropped or crashing the whole view.
	Warning string `json:"warning,omitempty"`
}

// EmbeddedFileList is the document-level enumeration of embedded/associated
// files. Files is non-nil but may be empty (a document with no attachments is
// a normal empty result, not an error).
type EmbeddedFileList struct {
	Files []EmbeddedFile `json:"files"`
}

// GetEmbeddedFiles enumerates every embedded file reachable from the catalog
// /AF array and the /Names/EmbeddedFiles name tree, merged and de-duplicated by
// the /Filespec indirect-object reference. Direct (inline) filespecs are kept
// as distinct entries with an empty FilespecRef. A document with no embedded
// files returns a non-nil empty list and no error. A broken name tree or a
// filespec without /EF degrades per-entry rather than failing the whole view.
// The whole sequence runs under doc.pdfMu and inside safeCall.
func (ins *Inspector) GetEmbeddedFiles(tabID string) (*EmbeddedFileList, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	// Serialize pdfcpu access. The walk dereferences /AF, the name tree, and
	// each /Filespec's /EF stream, all of which touch pdfcpu's XRefTable.
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	list := &EmbeddedFileList{Files: []EmbeddedFile{}}
	err = safeCall(func() error {
		list.Files = collectEmbeddedFiles(doc)
		return nil
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}
	return list, nil
}

// collectEmbeddedFiles walks both sources and returns the merged, deduped
// entries. Caller MUST hold doc.pdfMu and run inside safeCall.
func collectEmbeddedFiles(doc *DocumentState) []EmbeddedFile {
	files := []EmbeddedFile{}
	// seen tracks /Filespec indirect refs already emitted so a filespec reached
	// from both /AF and the name tree collapses to one entry. Direct filespecs
	// (empty ref) are never deduped.
	seen := map[string]bool{}

	add := func(fsObj pdfcpu_types.Object, fsRef string) {
		if fsRef != "" {
			if seen[fsRef] {
				return
			}
			seen[fsRef] = true
		}
		d := asDict(fsObj)
		if d == nil {
			return
		}
		ef := embeddedFileFromFilespec(doc, d, fsRef)
		files = append(files, ef)
	}

	cat, err := doc.PDFContext.Catalog()
	if err != nil {
		return files
	}

	// Source 1: catalog /AF associated-files array. Each element is normally an
	// indirect ref to a /Filespec, but a direct (inline) filespec dict is legal.
	if afObj, ok := cat["AF"]; ok {
		afArr := dereferenceArray(doc, afObj)
		for _, elem := range afArr {
			if ref, ok := elem.(pdfcpu_types.IndirectRef); ok {
				resolved, derefErr := doc.PDFContext.Dereference(ref)
				if derefErr != nil || resolved == nil {
					continue
				}
				add(resolved, refString(ref))
			} else if d := asDict(elem); d != nil {
				// Direct (inline) filespec: kept with an empty ref, never deduped.
				add(elem, "")
			}
		}
	}

	// Source 2: /Names/EmbeddedFiles name tree. A broken tree degrades to no
	// entries from this source; /AF entries above are unaffected.
	if namesObj, ok := cat["Names"]; ok {
		namesDict := asDict(dereference(doc, namesObj))
		if namesDict != nil {
			if efObj, ok := namesDict["EmbeddedFiles"]; ok {
				walkNameTree(doc, efObj, 0, func(resolved pdfcpu_types.Object, fsRef string) {
					add(resolved, fsRef)
				})
			}
		}
	}

	return files
}

// embeddedFileFromFilespec distills one EmbeddedFile from a resolved /Filespec
// dict. fsRef is the filespec's "N G R" ref (empty for a direct filespec).
func embeddedFileFromFilespec(doc *DocumentState, fs pdfcpu_types.Dict, fsRef string) EmbeddedFile {
	ef := EmbeddedFile{FilespecRef: fsRef}

	// Display name: /UF preferred, else /F. Both are text strings. An
	// undecodable /UF stays present as raw bytes rather than deferring to /F.
	if uf := textStringOrRaw(fs["UF"]); uf != "" {
		ef.Name = uf
	} else if f := textStringOrRaw(fs["F"]); f != "" {
		ef.Name = f
	}

	if rel, ok := fs["AFRelationship"].(pdfcpu_types.Name); ok {
		ef.AFRelationship = rel.Value()
	}

	// Resolve the /EF /F (or /UF) embedded-file stream ref.
	efRef, found := embeddedFileStreamRef(doc, fs)
	if !found {
		ef.Warning = "filespec has no /EF embedded-file stream"
		return ef
	}
	ef.EmbeddedFileRef = refString(efRef)
	ef.EmbeddedFileNodeID = nodeIDFromRef(efRef)

	// Resolve the stream dict for /Subtype, /Params, and size.
	streamObj, derefErr := doc.PDFContext.Dereference(efRef)
	if derefErr != nil || streamObj == nil {
		ef.Warning = "embedded-file stream could not be resolved"
		return ef
	}
	sd, ok := streamObj.(pdfcpu_types.StreamDict)
	if !ok {
		ef.Warning = "embedded-file target is not a stream"
		return ef
	}

	if subObj, ok := sd.Dict["Subtype"]; ok {
		if name, ok := subObj.(pdfcpu_types.Name); ok {
			// Decode #2F-style escapes (e.g. /text#2Fxml -> text/xml). DecodeName
			// is a no-op on names without escapes.
			if decoded, decErr := pdfcpu_types.DecodeName(name.String()); decErr == nil {
				ef.Subtype = decoded
			} else {
				ef.Subtype = name.Value()
			}
		}
	}

	// /Params: /Size (preferred for decoded size), /CheckSum, /ModDate.
	if paramsObj, ok := sd.Dict["Params"]; ok {
		if params := asDict(dereference(doc, paramsObj)); params != nil {
			// Only trust a non-negative /Size; a negative value in a malformed
			// PDF would otherwise propagate to the UI and skip the length fallback.
			if sz, ok := params["Size"].(pdfcpu_types.Integer); ok && int64(sz) >= 0 {
				ef.Size = int64(sz)
			}
			ef.CheckSum = stringValue(params["CheckSum"])
			ef.ModDate = stringValue(params["ModDate"])
		}
	}
	// Fall back to the resolved stream length when /Params /Size is absent.
	if ef.Size == 0 {
		if sd.StreamLength != nil {
			ef.Size = *sd.StreamLength
		} else if lenObj, ok := sd.Dict["Length"]; ok {
			if n, ok := lenObj.(pdfcpu_types.Integer); ok && int64(n) >= 0 {
				ef.Size = int64(n)
			}
		}
	}

	return ef
}

// embeddedFileStreamRef resolves the /EF /F (or /UF) indirect ref of an
// embedded-file stream from a /Filespec dict, returning ok=false when the
// filespec has no resolvable /EF stream ref.
func embeddedFileStreamRef(doc *DocumentState, fs pdfcpu_types.Dict) (pdfcpu_types.IndirectRef, bool) {
	efObj, ok := fs["EF"]
	if !ok {
		return pdfcpu_types.IndirectRef{}, false
	}
	efDict := asDict(dereference(doc, efObj))
	if efDict == nil {
		return pdfcpu_types.IndirectRef{}, false
	}
	// /F preferred, then /UF; both point at the same stream in practice.
	for _, key := range []string{"F", "UF"} {
		if v, ok := efDict[key]; ok {
			if ref, ok := v.(pdfcpu_types.IndirectRef); ok {
				return ref, true
			}
		}
	}
	return pdfcpu_types.IndirectRef{}, false
}

// walkNameTree traverses a PDF name-tree node (/Kids intermediate nodes or
// /Names leaf pairs), invoking visit for each /Filespec value. visit receives
// the resolved filespec object and its "N G R" ref ("" for a direct, inline
// filespec - rare but legal, kept). A malformed node (unresolvable ref, wrong
// shape) degrades silently so the rest of the enumeration proceeds. depth
// guards against pathological nesting.
func walkNameTree(doc *DocumentState, node pdfcpu_types.Object, depth int, visit func(resolved pdfcpu_types.Object, fsRef string)) {
	if depth > maxNodeIDDepth {
		return
	}
	d := asDict(dereference(doc, node))
	if d == nil {
		return
	}

	// Leaf: /Names is a flat [key value key value ...] array; values at odd
	// indices are filespec refs (normally indirect, but a direct filespec dict
	// is legal and must be kept with an empty ref, never dropped).
	if namesObj, ok := d["Names"]; ok {
		arr := dereferenceArray(doc, namesObj)
		for i := 1; i < len(arr); i += 2 {
			if ref, ok := arr[i].(pdfcpu_types.IndirectRef); ok {
				resolved, derefErr := doc.PDFContext.Dereference(ref)
				if derefErr != nil || resolved == nil {
					continue
				}
				visit(resolved, refString(ref))
			} else if dd := asDict(arr[i]); dd != nil {
				visit(arr[i], "")
			}
		}
	}

	// Intermediate: /Kids is an array of child name-tree node refs.
	if kidsObj, ok := d["Kids"]; ok {
		for _, kid := range dereferenceArray(doc, kidsObj) {
			walkNameTree(doc, kid, depth+1, visit)
		}
	}
}

// --- small object helpers ---------------------------------------------------

// dereference resolves an indirect ref to its target, returning the object
// unchanged when it is not an indirect ref. Errors yield nil.
func dereference(doc *DocumentState, obj pdfcpu_types.Object) pdfcpu_types.Object {
	ref, ok := obj.(pdfcpu_types.IndirectRef)
	if !ok {
		return obj
	}
	resolved, err := doc.PDFContext.Dereference(ref)
	if err != nil {
		return nil
	}
	return resolved
}

// dereferenceArray resolves obj to an Array (following one indirect ref),
// returning nil when it is not an array.
func dereferenceArray(doc *DocumentState, obj pdfcpu_types.Object) pdfcpu_types.Array {
	arr, ok := dereference(doc, obj).(pdfcpu_types.Array)
	if !ok {
		return nil
	}
	return arr
}

// stringValue renders a PDF string-like object as its raw bytes - the escaped
// body for a StringLiteral, the hex digit text for a HexLiteral - and "" for
// any other type.
//
// Used for two kinds of field. Decoding would corrupt the first: filespec
// /Params /CheckSum, signature /Contents and /Cert, trailer /ID. The second is
// only unwired: /Params /ModDate and the /Info date keys (legally text strings),
// catalog /Lang, and the signature /T, /Name, /Reason, /Location, /ContactInfo.
//
// Text fields use textStringOrRaw, which also falls back to this, so it must
// not be re-pointed at the decoder.
func stringValue(obj pdfcpu_types.Object) string {
	switch v := obj.(type) {
	case pdfcpu_types.StringLiteral:
		return string(v)
	case pdfcpu_types.HexLiteral:
		return string(v)
	default:
		return ""
	}
}

// refString renders an IndirectRef as the canonical "N G R" form.
func refString(ref pdfcpu_types.IndirectRef) string {
	return fmt.Sprintf("%d %d R", ref.ObjectNumber.Value(), ref.GenerationNumber.Value())
}

// nodeIDFromRef renders an IndirectRef as the obj:G:N tree-node id form.
func nodeIDFromRef(ref pdfcpu_types.IndirectRef) string {
	return fmt.Sprintf("obj:%d:%d", ref.GenerationNumber.Value(), ref.ObjectNumber.Value())
}

// GetEmbeddedFileBytes returns the decoded (decompressed) bytes of one embedded
// file, addressed by the obj:G:N nodeID of its /EmbeddedFile stream (the same
// nodeID convention as GetImageData). A stream whose decoded size exceeds the
// maxImageBytes ceiling returns ErrUnsupportedPDF rather than OOMing. A nodeID
// that does not resolve to a stream returns an error (never a panic). Runs
// under doc.pdfMu + safeCall.
func (ins *Inspector) GetEmbeddedFileBytes(tabID, nodeID string) ([]byte, error) {
	if nodeID == "" {
		return nil, fmt.Errorf("%w: empty node ID", ErrDocumentNotFound)
	}

	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	// Serialize pdfcpu access for resolve + decode.
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

	sd, ok := obj.(pdfcpu_types.StreamDict)
	if !ok {
		return nil, fmt.Errorf("%w: node %q is not an embedded-file stream", ErrUnsupportedPDF, nodeID)
	}

	// Pre-decode ceiling guard against the raw payload (unfiltered streams have
	// their decoded length on disk). Per image.go's maxImageBytes discipline.
	if len(sd.Raw) > maxImageBytes {
		return nil, fmt.Errorf("%w: embedded file exceeds the %d MB extraction ceiling", ErrUnsupportedPDF, maxImageBytes/(1024*1024))
	}

	var out []byte
	if len(sd.FilterPipeline) > 0 {
		out, err = decodeBounded(&sd, maxImageBytes)
		if err != nil {
			if errors.Is(err, ErrUnsupportedPDF) {
				return nil, fmt.Errorf("%w: embedded file exceeds the %d MB extraction ceiling", ErrUnsupportedPDF, maxImageBytes/(1024*1024))
			}
			return nil, fmt.Errorf("%w: failed to decode embedded file: %v", ErrUnsupportedPDF, err)
		}
	} else {
		// Unfiltered: raw bytes ARE the decoded payload.
		out = sd.Raw
		if out == nil {
			out = sd.Content
		}
		// Both nil means the loader never populated the stream body; surface an
		// error rather than a silent 0-byte payload that a redirect/save would
		// capture as a successful-but-empty extraction.
		if out == nil {
			return nil, fmt.Errorf("%w: embedded-file stream %q has no loaded content", ErrUnsupportedPDF, nodeID)
		}
	}

	// The delivered payload never exceeds the ceiling, whichever branch produced
	// it. decodeBounded already measured the filtered branch against the same
	// limit, so this is the unfiltered branch's guard and the filtered branch's
	// backstop - keep it even if that looks redundant.
	if len(out) > maxImageBytes {
		return nil, fmt.Errorf("%w: embedded file exceeds the %d MB extraction ceiling", ErrUnsupportedPDF, maxImageBytes/(1024*1024))
	}

	return out, nil
}
