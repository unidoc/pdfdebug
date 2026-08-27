// Co-located unit tests for embedded-file enumeration and
// extraction.
//
// These exercise the NEW pdfcore surface:
//
//	(ins *Inspector) GetEmbeddedFiles(tabID string) (*EmbeddedFileList, error)
//	(ins *Inspector) GetEmbeddedFileBytes(tabID, nodeID string) ([]byte, error)
//	types EmbeddedFile, EmbeddedFileList
//
// Fixtures are hand-rolled raw PDF bytes assembled via the existing assemblexref
// helper (resolve_ref_atdd_test.go) and opened through writeTempPDF, which uses
// the SAME Inspector.Open path the app uses (pdfcpu_api.ReadContextFile under
// default validation). The embedded-file Subtype MUST be a Name with #2F
// hex-escapes (/text#2Fxml) or pdfcpu's default validator rejects the document.
package pdfcore

import (
	"bytes"
	"compress/zlib"
	"errors"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// --- fixture builders -------------------------------------------------------

// embeddedStreamObj returns an /EmbeddedFile stream object body carrying payload
// as its (unfiltered) stream bytes plus a /Params dict with /Size, /CheckSum,
// /ModDate so the Params surfacing can be asserted.
func embeddedStreamObj(num int, payload string) string {
	return strconv.Itoa(num) + " 0 obj\n" +
		"<< /Type /EmbeddedFile /Subtype /text#2Fxml /Length " + strconv.Itoa(len(payload)) +
		" /Params << /Size " + strconv.Itoa(len(payload)) +
		" /CheckSum (deadbeefcafef00d0011223344556677) /ModDate (D:20240101000000Z) >> >>\n" +
		"stream\n" + payload + "\nendstream\nendobj\n"
}

// filespecObj returns a /Filespec dict object body pointing /EF /F and /UF at
// the given EmbeddedFile stream object number, with the given /AFRelationship.
func filespecObj(num, efNum int, displayName, afRel string) string {
	return strconv.Itoa(num) + " 0 obj\n" +
		"<< /Type /Filespec /F (" + displayName + ") /UF (" + displayName + ") " +
		"/AFRelationship /" + afRel + " " +
		"/EF << /F " + strconv.Itoa(efNum) + " 0 R /UF " + strconv.Itoa(efNum) + " 0 R >> >>\nendobj\n"
}

// zugferdSharedFilespec builds a ZUGFeRD/Factur-X-style PDF where ONE /Filespec
// (object 6) is reachable from BOTH the catalog /AF array and the
// /Names/EmbeddedFiles name tree (object 7). it must appear ONCE after dedupe
// by the /Filespec indirect ref. The XML stream is object 4.
//
// Object map: 1 Catalog, 2 Pages, 3 Page, 4 EmbeddedFile(XML), 5 null,
// 6 Filespec, 7 EmbeddedFiles name-tree node.
func zugferdSharedFilespec(xml string) []byte {
	return assemblexref(
		"%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AF [6 0 R] /Names << /EmbeddedFiles 7 0 R >> >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		embeddedStreamObj(4, xml),
		"5 0 obj\nnull\nendobj\n",
		filespecObj(6, 4, "factur-x.xml", "Data"),
		"7 0 obj\n<< /Names [(factur-x.xml) 6 0 R] >>\nendobj\n",
	)
}

// ---------------------------------------------------------------------------
// Enumeration finds the embedded file from BOTH the catalog /AF array and
// the /Names/EmbeddedFiles name tree.
// ---------------------------------------------------------------------------

func TestGetEmbeddedFiles_FindsFromBothSources(t *testing.T) {
	ins, tabID := writeTempPDF(t, "zugferd.pdf", zugferdSharedFilespec("<x>hi</x>"))

	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("GetEmbeddedFiles error: %v", err)
	}
	if list == nil || len(list.Files) == 0 {
		t.Fatalf("expected at least one embedded file, got %+v", list)
	}
}

// ---------------------------------------------------------------------------
// A /Filespec reachable from BOTH /AF and the name tree (same indirect object)
// is merged and de-duplicated -> appears exactly ONCE.
// ---------------------------------------------------------------------------

func TestGetEmbeddedFiles_DedupesSharedFilespec(t *testing.T) {
	ins, tabID := writeTempPDF(t, "zugferd.pdf", zugferdSharedFilespec("<x>hi</x>"))

	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("GetEmbeddedFiles error: %v", err)
	}
	// Object 6 is the single shared /Filespec; it must collapse to one entry.
	count := 0
	for _, f := range list.Files {
		if f.Name == "factur-x.xml" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("shared /Filespec must dedupe to 1 entry, got %d:\n%+v", count, list.Files)
	}
}

// ---------------------------------------------------------------------------
// A DIRECT (non-indirect, inline) /Filespec is kept as a distinct entry with
// an empty filespec ref -- never silently dropped.
// ---------------------------------------------------------------------------

func TestGetEmbeddedFiles_DirectFilespecKeptWithEmptyRef(t *testing.T) {
	// /AF holds an inline (direct) /Filespec dict rather than an indirect ref.
	content := assemblexref(
		"%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AF [ "+
			"<< /Type /Filespec /F (inline.xml) /UF (inline.xml) /AFRelationship /Data "+
			"/EF << /F 4 0 R >> >> ] >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		embeddedStreamObj(4, "<x>inline</x>"),
	)
	ins, tabID := writeTempPDF(t, "inline-filespec.pdf", content)

	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("GetEmbeddedFiles error: %v", err)
	}
	var found *EmbeddedFile
	for i := range list.Files {
		if list.Files[i].Name == "inline.xml" {
			found = &list.Files[i]
		}
	}
	if found == nil {
		t.Fatalf("direct (inline) /Filespec was dropped; expected a distinct entry:\n%+v", list.Files)
	}
	if found.FilespecRef != "" {
		t.Errorf("direct /Filespec must carry an EMPTY filespec ref, got %q", found.FilespecRef)
	}
}

// ---------------------------------------------------------------------------
// Each entry carries the discriminating fields -- display name (/UF
// preferred), AFRelationship, Subtype MIME, decoded size, and the
// EmbeddedFile stream ref.
// ---------------------------------------------------------------------------

func TestGetEmbeddedFiles_EntryCarriesDiscriminatingFields(t *testing.T) {
	xml := "<?xml version=\"1.0\"?><Invoice/>"
	ins, tabID := writeTempPDF(t, "zugferd.pdf", zugferdSharedFilespec(xml))

	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("GetEmbeddedFiles error: %v", err)
	}
	if len(list.Files) == 0 {
		t.Fatalf("GetEmbeddedFiles returned no entries")
	}
	f := list.Files[0]
	if f.Name != "factur-x.xml" {
		t.Errorf("Name = %q, want factur-x.xml (/UF preferred)", f.Name)
	}
	if f.AFRelationship != "Data" {
		t.Errorf("AFRelationship = %q, want Data (the ZUGFeRD discriminator)", f.AFRelationship)
	}
	// /text#2Fxml decodes to the MIME text/xml.
	if f.Subtype != "text/xml" {
		t.Errorf("Subtype = %q, want text/xml (the #2F-escaped Name decoded)", f.Subtype)
	}
	if f.Size != int64(len(xml)) {
		t.Errorf("decoded Size = %d, want %d", f.Size, len(xml))
	}
	if f.EmbeddedFileRef == "" {
		t.Errorf("EmbeddedFileRef must be set (object 4)")
	}
}

// ---------------------------------------------------------------------------
// /Params CheckSum/ModDate/Size are surfaced when present.
// ---------------------------------------------------------------------------

func TestGetEmbeddedFiles_SurfacesParams(t *testing.T) {
	ins, tabID := writeTempPDF(t, "zugferd.pdf", zugferdSharedFilespec("<x/>"))

	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("GetEmbeddedFiles error: %v", err)
	}
	f := list.Files[0]
	if f.CheckSum == "" {
		t.Errorf("Params /CheckSum not surfaced")
	}
	if f.ModDate == "" {
		t.Errorf("Params /ModDate not surfaced")
	}
}

// ---------------------------------------------------------------------------
// A document with NO embedded files returns an empty list and NO error
// (normal empty state).
// ---------------------------------------------------------------------------

func TestGetEmbeddedFiles_NoneIsEmptyNotError(t *testing.T) {
	content := assemblexref(
		"%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
	)
	ins, tabID := writeTempPDF(t, "no-attachments.pdf", content)

	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("expected nil error for no-attachments doc, got %v", err)
	}
	if list == nil {
		t.Fatalf("expected non-nil empty list")
	}
	if len(list.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(list.Files))
	}
}

// ---------------------------------------------------------------------------
// GetEmbeddedFileBytes returns the decoded bytes of one embedded file,
// addressed by the obj:G:N nodeID of its /EmbeddedFile stream (same nodeID
// convention as GetImageData). Round-trip.
// ---------------------------------------------------------------------------

func TestGetEmbeddedFileBytes_RoundTrip(t *testing.T) {
	payload := "<?xml version=\"1.0\"?><CrossIndustryInvoice>data</CrossIndustryInvoice>"
	ins, tabID := writeTempPDF(t, "zugferd.pdf", zugferdSharedFilespec(payload))

	// Object 4 is the /EmbeddedFile stream -> nodeID obj:0:4.
	data, err := ins.GetEmbeddedFileBytes(tabID, "obj:0:4")
	if err != nil {
		t.Fatalf("GetEmbeddedFileBytes error: %v", err)
	}
	if string(data) != payload {
		t.Errorf("round-trip mismatch\n got: %q\nwant: %q", string(data), payload)
	}
}

// ---------------------------------------------------------------------------
// A stream whose DECODED size exceeds the image.go ceiling discipline
// (maxImageBytes order of magnitude, NOT the 4 GiB plaintext cap) returns
// ErrUnsupportedPDF rather than OOMing.
// ---------------------------------------------------------------------------

func TestGetEmbeddedFileBytes_OverCeilingReturnsUnsupported(t *testing.T) {
	// Build a payload strictly larger than maxImageBytes (50 MB). The stream is
	// unfiltered, so /Length is the decoded length and the raw payload above the
	// ceiling trips the guard.
	big := strings.Repeat("A", maxImageBytes+1)
	content := assemblexref(
		"%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AF [6 0 R] >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		embeddedStreamObj(4, big),
		"5 0 obj\nnull\nendobj\n",
		filespecObj(6, 4, "big.bin", "Data"),
	)
	ins, tabID := writeTempPDF(t, "over-ceiling.pdf", content)

	_, err := ins.GetEmbeddedFileBytes(tabID, "obj:0:4")
	if !errors.Is(err, ErrUnsupportedPDF) {
		t.Errorf("over-ceiling payload must return ErrUnsupportedPDF, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// An empty, malformed, or non-stream nodeID returns an error (or a non-crashing
// sentinel), never a panic.
// ---------------------------------------------------------------------------

func TestGetEmbeddedFileBytes_BadNodeIDErrors(t *testing.T) {
	ins, tabID := writeTempPDF(t, "zugferd.pdf", zugferdSharedFilespec("<x/>"))

	cases := []string{"", "not-a-node", "obj:0:2" /* /Pages, not a stream */}
	for _, nid := range cases {
		nid := nid
		t.Run(nid, func(t *testing.T) {
			_, err := ins.GetEmbeddedFileBytes(tabID, nid)
			if err == nil {
				t.Errorf("nodeID %q must yield an error, got nil", nid)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// A /Filespec with no /EF (or no /EmbeddedFile) produces a per-entry
// empty/warning state, NOT a crash or a failed whole-document load.
// ---------------------------------------------------------------------------

func TestGetEmbeddedFiles_FilespecWithoutEFDegradesPerEntry(t *testing.T) {
	content := assemblexref(
		"%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AF [6 0 R] >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		"4 0 obj\nnull\nendobj\n",
		"5 0 obj\nnull\nendobj\n",
		// Filespec with NO /EF entry at all.
		"6 0 obj\n<< /Type /Filespec /F (orphan.dat) /UF (orphan.dat) /AFRelationship /Unspecified >>\nendobj\n",
	)
	ins, tabID := writeTempPDF(t, "no-ef.pdf", content)

	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("missing /EF must NOT fail the whole view, got error: %v", err)
	}
	// The entry is still listed (so the user sees it), but with no stream ref.
	var found *EmbeddedFile
	for i := range list.Files {
		if list.Files[i].Name == "orphan.dat" {
			found = &list.Files[i]
		}
	}
	if found == nil {
		t.Fatalf("filespec without /EF should still appear as an entry:\n%+v", list.Files)
	}
	if found.EmbeddedFileRef != "" {
		t.Errorf("entry without /EF must carry an empty EmbeddedFileRef, got %q", found.EmbeddedFileRef)
	}
}

// ---------------------------------------------------------------------------
// A malformed /Names/EmbeddedFiles tree degrades (the /AF source still yields
// its entries) and the whole-document view does not fail.
// ---------------------------------------------------------------------------

func TestGetEmbeddedFiles_BrokenNameTreeDegrades(t *testing.T) {
	content := assemblexref(
		"%PDF-1.7\n",
		// Name tree points at object 9, which does not exist (broken).
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AF [6 0 R] /Names << /EmbeddedFiles 9 0 R >> >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		embeddedStreamObj(4, "<x/>"),
		"5 0 obj\nnull\nendobj\n",
		filespecObj(6, 4, "from-af.xml", "Data"),
	)
	ins, tabID := writeTempPDF(t, "broken-nametree.pdf", content)

	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("broken name tree must NOT fail the view, got error: %v", err)
	}
	if len(list.Files) == 0 {
		t.Errorf("AF source should still yield its entry despite the broken name tree")
	}
}

// --- coverage-expansion tests (testarch-automate, 2026-06-28) ---------------
// These fill source branches the green-phase suite left untested. They target
// the LOWEST viable layer (co-located pdfcore unit), per the automate directive.

// embeddedStreamNoParams returns an /EmbeddedFile stream WITHOUT a /Params dict,
// so embeddedFileFromFilespec must fall back to the resolved stream length for
// the decoded size (embedded.go:204-213).
func embeddedStreamNoParams(num int, payload string) string {
	return strconv.Itoa(num) + " 0 obj\n" +
		"<< /Type /EmbeddedFile /Subtype /text#2Fxml /Length " + strconv.Itoa(len(payload)) + " >>\n" +
		"stream\n" + payload + "\nendstream\nendobj\n"
}

// ---------------------------------------------------------------------------
// When /Params /Size is absent, the decoded size falls back to the resolved
// stream length (/Length), not zero.
// ---------------------------------------------------------------------------

func TestGetEmbeddedFiles_SizeFallsBackToStreamLength(t *testing.T) {
	payload := "<x>no-params-size</x>"
	content := assemblexref(
		"%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AF [6 0 R] >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		embeddedStreamNoParams(4, payload),
		"5 0 obj\nnull\nendobj\n",
		filespecObj(6, 4, "no-params.xml", "Data"),
	)
	ins, tabID := writeTempPDF(t, "no-params.pdf", content)

	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("GetEmbeddedFiles error: %v", err)
	}
	if len(list.Files) == 0 {
		t.Fatalf("GetEmbeddedFiles returned no entries")
	}
	f := list.Files[0]
	if f.Size != int64(len(payload)) {
		t.Errorf("Size = %d, want %d (length fallback when /Params /Size absent)", f.Size, len(payload))
	}
	if f.CheckSum != "" || f.ModDate != "" {
		t.Errorf("absent /Params must leave CheckSum/ModDate empty, got %q/%q", f.CheckSum, f.ModDate)
	}
}

// ---------------------------------------------------------------------------
// The name-tree walk recurses through /Kids intermediate nodes (not just a
// flat /Names leaf). The filespec lives in a child node reached via /Kids.
// ---------------------------------------------------------------------------

func TestGetEmbeddedFiles_WalksNameTreeKids(t *testing.T) {
	// Object 7 is an intermediate node (/Kids [8 0 R]); object 8 is the leaf
	// (/Names [...]) carrying the filespec 6. Only a /Kids-recursing walk finds it.
	content := assemblexref(
		"%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Names << /EmbeddedFiles 7 0 R >> >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		embeddedStreamObj(4, "<x>via-kids</x>"),
		"5 0 obj\nnull\nendobj\n",
		filespecObj(6, 4, "via-kids.xml", "Data"),
		// Intermediate /Kids node + leaf with /Limits, the shape pdfcpu's name-tree
		// validator requires to traverse a non-flat tree.
		"7 0 obj\n<< /Kids [8 0 R] >>\nendobj\n",
		"8 0 obj\n<< /Limits [(via-kids.xml) (via-kids.xml)] /Names [(via-kids.xml) 6 0 R] >>\nendobj\n",
	)
	ins, tabID := writeTempPDF(t, "nametree-kids.pdf", content)

	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("GetEmbeddedFiles error: %v", err)
	}
	var found bool
	for _, f := range list.Files {
		if f.Name == "via-kids.xml" {
			found = true
		}
	}
	if !found {
		t.Errorf("filespec under a /Kids intermediate node was not found:\n%+v", list.Files)
	}
}

// ---------------------------------------------------------------------------
// A DIRECT (inline, non-indirect) /Filespec inside the /Names/EmbeddedFiles
// NAME TREE is kept with an empty filespec ref -- never silently dropped (the
// name-tree analogue of the direct-/Filespec array case, guarding the patch at
// embedded.go:268-270).
// ---------------------------------------------------------------------------

func TestGetEmbeddedFiles_DirectFilespecInNameTreeKept(t *testing.T) {
	// The name-tree leaf pairs a name with an INLINE filespec dict (not a ref).
	content := assemblexref(
		"%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Names << /EmbeddedFiles 7 0 R >> >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		embeddedStreamObj(4, "<x>inline-in-tree</x>"),
		"5 0 obj\nnull\nendobj\n",
		"6 0 obj\nnull\nendobj\n",
		"7 0 obj\n<< /Names [(inline-tree.xml) "+
			"<< /Type /Filespec /F (inline-tree.xml) /UF (inline-tree.xml) /AFRelationship /Data "+
			"/EF << /F 4 0 R >> >> ] >>\nendobj\n",
	)
	ins, tabID := writeTempPDF(t, "inline-nametree.pdf", content)

	list, err := ins.GetEmbeddedFiles(tabID)
	if err != nil {
		t.Fatalf("GetEmbeddedFiles error: %v", err)
	}
	var found *EmbeddedFile
	for i := range list.Files {
		if list.Files[i].Name == "inline-tree.xml" {
			found = &list.Files[i]
		}
	}
	if found == nil {
		t.Fatalf("direct filespec in the name tree was dropped:\n%+v", list.Files)
	}
	if found.FilespecRef != "" {
		t.Errorf("direct filespec must carry an EMPTY ref, got %q", found.FilespecRef)
	}
}

// flateEmbeddedStreamObj returns an /EmbeddedFile object whose payload is
// FlateDecode-compressed, so the stream's raw size stays far below the
// extraction ceiling while its decoded size sits far above it.
func flateEmbeddedStreamObj(num int, raw []byte, decodedSize int) string {
	return strconv.Itoa(num) + " 0 obj\n" +
		"<< /Type /EmbeddedFile /Subtype /application#2Foctet-stream /Filter /FlateDecode" +
		" /Length " + strconv.Itoa(len(raw)) +
		" /Params << /Size " + strconv.Itoa(decodedSize) + " >> >>\n" +
		"stream\n" + string(raw) + "\nendstream\nendobj\n"
}

// The attachment path rejects a bomb DURING inflation rather than after it.
// Allocation is the only observable difference: both a bounded and an unbounded
// decode refuse this stream and print the same message, so output cannot show
// which one ran. The assertion is therefore on how much was allocated getting
// there, which is why this lives at the unit layer rather than in the CLI suite.
//
// TotalAlloc is cumulative and GC-immune, so this test runs serially and takes
// both readings with nothing but the guarded call between them.
func TestGetEmbeddedFileBytes_BombRejectedWithoutFullInflation(t *testing.T) {
	const inflatedSize = 400 * 1024 * 1024
	// The threshold MUST sit below inflatedSize, or the assertion does not say
	// what it claims: a regression that allocated the payload exactly once - a
	// buffer pre-sized from /Length, say - would land near 400 MB and pass, and
	// the test would only still work by accident of pdfcpu's buffer doubling.
	//
	// Measured on this fixture: the bounded path allocates ~134 MB (TotalAlloc is
	// CUMULATIVE, and pdfcpu's buffer doubles its way to the 50 MB ceiling, so
	// ~130 MB total for a ~64 MB peak); the unbounded path allocates ~1,074 MB.
	// 256 MB gives ~1.9x clearance above the legitimate figure and sits 1.6x
	// below the payload size.
	const allocCeiling = uint64(256 * 1024 * 1024)

	// Race instrumentation roughly doubles the measured allocation, which no
	// threshold below the payload size can accommodate. Skip rather than widen
	// the threshold past the point where it proves anything.
	if raceEnabled {
		t.Skip("allocation magnitude is not measurable under race instrumentation")
	}

	var compressed bytes.Buffer
	zw := zlib.NewWriter(&compressed)
	block := make([]byte, 64*1024)
	for written := 0; written < inflatedSize; written += len(block) {
		if _, err := zw.Write(block); err != nil {
			t.Fatalf("zlib write: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}
	if compressed.Len() > maxImageBytes {
		t.Fatalf("fixture defeats the test: %d compressed bytes exceed the ceiling, so the raw pre-guard rejects first", compressed.Len())
	}

	content := assemblexref(
		"%PDF-1.7\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AF [6 0 R] >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		flateEmbeddedStreamObj(4, compressed.Bytes(), 1024),
		"5 0 obj\nnull\nendobj\n",
		filespecObj(6, 4, "bomb.bin", "Data"),
	)
	ins, tabID := writeTempPDF(t, "attachment-bomb.pdf", content)

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := ins.GetEmbeddedFileBytes(tabID, "obj:0:4")
	runtime.ReadMemStats(&after)

	// Assert the WORDING, not just the sentinel. GetEmbeddedFileBytes wraps every
	// decode failure as ErrUnsupportedPDF, so a broken fixture - a bad /Length, a
	// wrong object number - would fail early, allocate almost nothing, and pass
	// this test while proving nothing at all.
	if err == nil || !strings.Contains(err.Error(), "extraction ceiling") {
		t.Fatalf("expected the extraction-ceiling rejection, got %v", err)
	}
	if delta := after.TotalAlloc - before.TotalAlloc; delta >= allocCeiling {
		t.Errorf("allocated %d bytes rejecting a %d-byte inflation; want under %d",
			delta, inflatedSize, allocCeiling)
	}
}
