package pdfcore

import (
	"fmt"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// infoFields is the ordered set of /Info dictionary keys surfaced by
// GetDocumentMetadata. Only keys present in the document appear in the result
// Info map.
var infoFields = []string{
	"Title", "Author", "Subject", "Keywords",
	"Creator", "Producer", "CreationDate", "ModDate",
}

// DocumentMetadata is the document-level metadata view: the catalog /Metadata
// XMP packet (passed through VERBATIM after any /Filter decompression) and the
// trailer /Info dictionary fields. A document with neither is a normal empty
// result, not an error. A /Metadata stream whose /Filter decode fails yields an
// empty XMP plus a Warning (never an error that fails the view).
type DocumentMetadata struct {
	// Info holds the present /Info fields (Title..ModDate). Non-nil; empty when
	// the document has no /Info dict.
	Info map[string]string `json:"info"`
	// XMP is the verbatim XMP packet from the catalog /Metadata stream. Empty
	// when absent or undecodable.
	XMP string `json:"xmp"`
	// Warning is a non-fatal note (e.g. the /Metadata stream could not be
	// decoded). Empty on the normal path.
	Warning string `json:"warning,omitempty"`
}

// GetDocumentMetadata returns the document's XMP packet (catalog /Metadata
// stream, decompressed per its /Filter then passed through verbatim) and its
// /Info dictionary fields. Missing /Metadata or /Info is a normal empty result.
// A /Metadata stream that fails to decode surfaces as empty XMP + a warning,
// never an error (AC3/AC8). Runs under doc.pdfMu + safeCall.
func (ins *Inspector) GetDocumentMetadata(tabID string) (*DocumentMetadata, error) {
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		return nil, err
	}
	// AC3: serialize pdfcpu access for the catalog read, /Info deref, and the
	// /Metadata stream decode.
	doc.pdfMu.Lock()
	defer doc.pdfMu.Unlock()

	md := &DocumentMetadata{Info: map[string]string{}}
	err = safeCall(func() error {
		collectInfoFields(doc, md)
		collectXMP(doc, md)
		return nil
	})
	if err != nil {
		return nil, wrapPDFError(err)
	}
	return md, nil
}

// collectInfoFields populates md.Info from the trailer /Info dict. Caller MUST
// hold doc.pdfMu and run inside safeCall.
func collectInfoFields(doc *DocumentState, md *DocumentMetadata) {
	infoRef := doc.PDFContext.XRefTable.Info
	if infoRef == nil {
		return
	}
	infoObj, err := doc.PDFContext.Dereference(*infoRef)
	if err != nil || infoObj == nil {
		return
	}
	info := asDict(infoObj)
	if info == nil {
		return
	}
	for _, key := range infoFields {
		if v := stringValue(info[key]); v != "" {
			md.Info[key] = v
		}
	}
}

// collectXMP populates md.XMP (or md.Warning) from the catalog /Metadata
// stream. Caller MUST hold doc.pdfMu and run inside safeCall.
func collectXMP(doc *DocumentState, md *DocumentMetadata) {
	cat, err := doc.PDFContext.Catalog()
	if err != nil {
		return
	}
	metaObj, ok := cat["Metadata"]
	if !ok {
		return
	}
	resolved := dereference(doc, metaObj)
	sd, ok := resolved.(pdfcpu_types.StreamDict)
	if !ok {
		return
	}
	md.XMP, md.Warning = decodeXMPStream(sd)
}

// decodeXMPStream decompresses an XMP /Metadata stream per its /Filter (when
// present) and returns the verbatim XML bytes as a string. A decode failure
// returns ("", warning) per AC8 - empty XMP plus a non-fatal warning, never an
// error. Separated from collectXMP so the decode/classify branch is unit
// testable on a bare StreamDict (pdfcpu strictly decodes /Metadata at file-read
// time, so the undecodable path is unreachable through Inspector.Open).
func decodeXMPStream(sd pdfcpu_types.StreamDict) (xmp, warning string) {
	if len(sd.FilterPipeline) > 0 {
		decErr := safeCall(func() error {
			return sd.Decode()
		})
		if decErr != nil {
			// AC8: undecodable /Metadata degrades to empty XMP + warning.
			return "", fmt.Sprintf("metadata stream could not be decoded: %v", decErr)
		}
		return string(sd.Content), ""
	}
	// Unfiltered: the raw bytes ARE the XMP packet.
	if sd.Raw != nil {
		return string(sd.Raw), ""
	}
	return string(sd.Content), ""
}
