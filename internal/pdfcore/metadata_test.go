// Story 13-2 RED-PHASE co-located unit tests for the document metadata view
// (AC 3, 8).
//
// Exercises the NEW pdfcore surface:
//
//	(ins *Inspector) GetDocumentMetadata(tabID string) (*DocumentMetadata, error)
//	type DocumentMetadata{ Info map[string]string; XMP string; Warning string }
//
// The exact field set of DocumentMetadata is the dev's to finalize; these tests
// pin the contract the story names: XMP packet bytes passed VERBATIM, /Info
// fields surfaced, missing = empty (not error), undecodable /Metadata = empty
// XMP + warning (never an error that fails the view).
//
// RED state: the package will not compile until metadata.go lands
// GetDocumentMetadata + DocumentMetadata. Naming: 13.2-UNIT-NNN [Px].
package pdfcore

import (
	"strconv"
	"strings"
	"testing"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// assembleWithInfo builds a PDF whose trailer carries BOTH /Root and /Info, so
// the /Info dictionary path of GetDocumentMetadata can be exercised. objs[i] is
// object number i+1. rootNum / infoNum are 1-based object numbers (infoNum may
// be 0 to omit /Info from the trailer).
func assembleWithInfo(objs []string, rootNum, infoNum int) []byte {
	body := "%PDF-1.7\n"
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = len(body)
		body += o
	}
	xrefOff := len(body)
	size := len(objs) + 1
	xref := "xref\n0 " + strconv.Itoa(size) + "\n0000000000 65535 f \n"
	for _, off := range offsets {
		xref += pad10(off) + " 00000 n \n"
	}
	trailer := "trailer\n<< /Size " + strconv.Itoa(size) + " /Root " + strconv.Itoa(rootNum) + " 0 R"
	if infoNum > 0 {
		trailer += " /Info " + strconv.Itoa(infoNum) + " 0 R"
	}
	trailer += " >>\nstartxref\n" + strconv.Itoa(xrefOff) + "\n%%EOF\n"
	return []byte(body + xref + trailer)
}

// xmpPacket returns a minimal but well-formed XMP packet body.
const xmpPacket = "<?xpacket begin=\"\" id=\"W5M0MpCehiHzreSzNTczkc9d\"?>" +
	"<x:xmpmeta xmlns:x=\"adobe:ns:meta/\"><rdf:RDF>marker-VERBATIM-XMP</rdf:RDF></x:xmpmeta>" +
	"<?xpacket end=\"w\"?>"

// metadataPDF builds a doc with catalog /Metadata (object 4, uncompressed XML
// stream) and an /Info dict (object 5) carrying Title/Author/Producer.
func metadataPDF() []byte {
	meta := "4 0 obj\n<< /Type /Metadata /Subtype /XML /Length " + strconv.Itoa(len(xmpPacket)) + " >>\n" +
		"stream\n" + xmpPacket + "\nendstream\nendobj\n"
	info := "5 0 obj\n<< /Title (Invoice 2024-001) /Author (ACME GmbH) /Producer (pdfdebug-test) >>\nendobj\n"
	return assembleWithInfo([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Metadata 4 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		meta,
		info,
	}, 1, 5)
}

// ---------------------------------------------------------------------------
// 13.2-UNIT-020 [P0] AC3: the XMP /Metadata stream is returned.
// ---------------------------------------------------------------------------

func TestGetDocumentMetadata_ReturnsXMP(t *testing.T) {
	ins, tabID := writeTempPDF(t, "metadata.pdf", metadataPDF())

	md, err := ins.GetDocumentMetadata(tabID)
	if err != nil {
		t.Fatalf("[P0] 13.2-UNIT-020: GetDocumentMetadata error: %v", err)
	}
	if md == nil || md.XMP == "" {
		t.Fatalf("[P0] 13.2-UNIT-020: expected non-empty XMP, got %+v", md)
	}
}

// ---------------------------------------------------------------------------
// 13.2-UNIT-021 [P0] AC3: the XMP bytes are passed VERBATIM -- no Go-side XML
// parse/mutation. The exact marker survives byte-for-byte and the packet is not
// re-escaped or re-serialized.
// ---------------------------------------------------------------------------

func TestGetDocumentMetadata_XMPVerbatim(t *testing.T) {
	ins, tabID := writeTempPDF(t, "metadata.pdf", metadataPDF())

	md, err := ins.GetDocumentMetadata(tabID)
	if err != nil {
		t.Fatalf("[P0] 13.2-UNIT-021: error: %v", err)
	}
	if md.XMP != xmpPacket {
		t.Errorf("[P0] 13.2-UNIT-021: XMP not verbatim\n got: %q\nwant: %q", md.XMP, xmpPacket)
	}
}

// ---------------------------------------------------------------------------
// 13.2-UNIT-022 [P0] AC3: /Info fields are surfaced when present.
// ---------------------------------------------------------------------------

func TestGetDocumentMetadata_SurfacesInfoFields(t *testing.T) {
	ins, tabID := writeTempPDF(t, "metadata.pdf", metadataPDF())

	md, err := ins.GetDocumentMetadata(tabID)
	if err != nil {
		t.Fatalf("[P0] 13.2-UNIT-022: error: %v", err)
	}
	if got := md.Info["Title"]; got != "Invoice 2024-001" {
		t.Errorf("[P0] 13.2-UNIT-022: Info[Title] = %q, want %q", got, "Invoice 2024-001")
	}
	if got := md.Info["Author"]; got != "ACME GmbH" {
		t.Errorf("[P0] 13.2-UNIT-022: Info[Author] = %q, want %q", got, "ACME GmbH")
	}
	if got := md.Info["Producer"]; got != "pdfdebug-test" {
		t.Errorf("[P0] 13.2-UNIT-022: Info[Producer] = %q, want %q", got, "pdfdebug-test")
	}
}

// ---------------------------------------------------------------------------
// 13.2-UNIT-023 [P1] AC3: missing /Metadata AND /Info is a normal empty result,
// not an error.
// ---------------------------------------------------------------------------

func TestGetDocumentMetadata_MissingIsEmptyNotError(t *testing.T) {
	content := assembleWithInfo([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
	}, 1, 0)
	ins, tabID := writeTempPDF(t, "no-metadata.pdf", content)

	md, err := ins.GetDocumentMetadata(tabID)
	if err != nil {
		t.Fatalf("[P1] 13.2-UNIT-023: missing metadata must NOT error, got %v", err)
	}
	if md == nil {
		t.Fatalf("[P1] 13.2-UNIT-023: expected a non-nil empty result")
	}
	if md.XMP != "" {
		t.Errorf("[P1] 13.2-UNIT-023: expected empty XMP, got %q", md.XMP)
	}
	if len(md.Info) != 0 {
		t.Errorf("[P1] 13.2-UNIT-023: expected empty Info, got %+v", md.Info)
	}
}

// ---------------------------------------------------------------------------
// 13.2-UNIT-024 [P0] AC3/8: a /Metadata stream whose /Filter decode fails
// surfaces as an EMPTY-XMP result PLUS a warning, never an error that fails the
// view.
//
// NOTE: pdfcpu STRICTLY decodes every stream's /Filter at file-read time
// (ReadContextFile rejects a /FlateDecode /Metadata over garbage bytes with
// "zlib: invalid header"), so an undecodable /Metadata is unreachable through
// Inspector.Open with a hand-rolled fixture - the file would never open. The
// AC8 contract is therefore asserted directly against decodeXMPStream, the pure
// decode/classify branch collectXMP delegates to: a /FlateDecode StreamDict
// over non-zlib bytes whose Decode() fails must yield ("", warning), never an
// error. (Original ATDD fixture revised during the green phase: it could not
// parse under pdfcpu.)
// ---------------------------------------------------------------------------

func TestGetDocumentMetadata_UndecodableMetadataWarnsNotErrors(t *testing.T) {
	garbage := []byte("this-is-not-valid-flate-data")
	length := int64(len(garbage))
	sd := pdfcpu_types.StreamDict{
		Dict: pdfcpu_types.Dict{
			"Type":    pdfcpu_types.Name("Metadata"),
			"Subtype": pdfcpu_types.Name("XML"),
			"Filter":  pdfcpu_types.Name("FlateDecode"),
		},
		Raw:            garbage,
		StreamLength:   &length,
		FilterPipeline: []pdfcpu_types.PDFFilter{{Name: "FlateDecode"}},
	}

	xmp, warning := decodeXMPStream(sd)
	if xmp != "" {
		t.Errorf("[P0] 13.2-UNIT-024: undecodable /Metadata must yield empty XMP, got %q", xmp)
	}
	if strings.TrimSpace(warning) == "" {
		t.Errorf("[P0] 13.2-UNIT-024: undecodable /Metadata must surface a warning")
	}
}

// ---------------------------------------------------------------------------
// 13.2-UNIT-025 [P1] AC3: an UNFILTERED /Metadata stream (no /Filter) passes
// its Raw bytes through VERBATIM as the XMP packet with NO warning -- the happy
// counterpart to UNIT-024's undecodable path (metadata.go:119-123).
// ---------------------------------------------------------------------------

func TestDecodeXMPStream_UnfilteredVerbatimNoWarning(t *testing.T) {
	raw := []byte(xmpPacket)
	length := int64(len(raw))
	sd := pdfcpu_types.StreamDict{
		Dict: pdfcpu_types.Dict{
			"Type":    pdfcpu_types.Name("Metadata"),
			"Subtype": pdfcpu_types.Name("XML"),
		},
		Raw:          raw,
		StreamLength: &length,
		// No FilterPipeline -> the raw bytes ARE the packet.
	}

	xmp, warning := decodeXMPStream(sd)
	if xmp != xmpPacket {
		t.Errorf("[P1] 13.2-UNIT-025: unfiltered XMP not verbatim\n got: %q\nwant: %q", xmp, xmpPacket)
	}
	if warning != "" {
		t.Errorf("[P1] 13.2-UNIT-025: unfiltered XMP must NOT warn, got %q", warning)
	}
}
