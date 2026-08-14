package pdfcore

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Co-located unit tests for the structural rule engine. Fixtures are
// hand-assembled via the shared assemblexref helper and opened through the same
// Inspector.Open path the app uses. Each rule is exercised positive + negative.

// problemByRule returns the first problem with the given ruleId, or nil.
func problemByRule(ps []Problem, ruleID string) *Problem {
	for i := range ps {
		if ps[i].RuleID == ruleID {
			return &ps[i]
		}
	}
	return nil
}

// countByRule returns how many problems carry the given ruleId.
func countByRule(ps []Problem, ruleID string) int {
	n := 0
	for i := range ps {
		if ps[i].RuleID == ruleID {
			n++
		}
	}
	return n
}

// assembleWithTrailer mirrors assemblexref but injects extra trailer entries
// (e.g. "/Info 5 0 R ") so a fixture can carry an /Info dict. parts[0] is the
// header; parts[1:] are object bodies numbered 1..N in order.
func assembleWithTrailer(trailerExtra string, parts ...string) []byte {
	header := parts[0]
	objs := parts[1:]
	body := header
	offsets := make([]int, len(objs))
	cur := len(header)
	for i, o := range objs {
		offsets[i] = cur
		body += o
		cur += len(o)
	}
	xrefOffset := len(body)
	xref := fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, off := range offsets {
		xref += fmt.Sprintf("%010d 00000 n \n", off)
	}
	trailer := fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R %s>>\nstartxref\n%d\n%%%%EOF\n",
		len(objs)+1, trailerExtra, xrefOffset)
	return []byte(body + xref + trailer)
}

// wrapXMP wraps property elements in a minimal but valid XMP packet carrying the
// dc:, pdf:, and xmp: namespaces used by the /Info<->XMP consistency check.
func wrapXMP(inner string) string {
	return `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?>` +
		`<x:xmpmeta xmlns:x="adobe:ns:meta/"><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">` +
		`<rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/" ` +
		`xmlns:pdf="http://ns.adobe.com/pdf/1.3/" xmlns:xmp="http://ns.adobe.com/xap/1.0/">` +
		inner +
		`</rdf:Description></rdf:RDF></x:xmpmeta><?xpacket end="w"?>`
}

// xmpInfoPDF builds a minimal PDF whose catalog carries an unfiltered /Metadata
// XMP packet (xmp) and whose trailer /Info dict body is infoBody, so the
// /Info<->XMP consistency check has both sides to compare.
func xmpInfoPDF(xmp, infoBody string) []byte {
	meta := fmt.Sprintf("4 0 obj\n<< /Type /Metadata /Subtype /XML /Length %d >>\nstream\n%s\nendstream\nendobj\n\n", len(xmp), xmp)
	info := fmt.Sprintf("5 0 obj\n<< %s >>\nendobj\n\n", infoBody)
	return assembleWithTrailer("/Info 5 0 R ",
		"%PDF-1.4\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Metadata 4 0 R >>\nendobj\n\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n",
		meta,
		info,
	)
}

func TestValidate_UnknownProfileErrors(t *testing.T) {
	ins, tabID := writeTempPDF(t, "min.pdf", assemblexref(
		"%PDF-1.4\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n",
	))
	if _, err := ins.Validate(tabID, "bogus-profile"); !errors.Is(err, ErrUnknownProfile) {
		t.Fatalf("Validate(unknown profile): want ErrUnknownProfile, got %v", err)
	}
}

func TestValidate_FontEmbedding(t *testing.T) {
	ins, tabID := writeTempPDF(t, "nonembed.pdf", assemblexref(
		"%PDF-1.4\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> >>\nendobj\n\n",
		"4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n\n",
	))
	res, err := ins.Validate(tabID, ProfilePDFA1B)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	p := problemByRule(res.Problems, "font-embedding")
	if p == nil {
		t.Fatalf("font-embedding rule did not fire on a non-embedded font:\n%+v", res.Problems)
	}
	if p.Severity != "error" {
		t.Errorf("font-embedding severity = %q, want error", p.Severity)
	}
	if p.ObjRef != "4 0 R" || p.ObjNodeID != "obj:0:4" {
		t.Errorf("font-embedding objRef/objNodeId = %q/%q, want \"4 0 R\"/obj:0:4", p.ObjRef, p.ObjNodeID)
	}
	if !strings.Contains(strings.ToLower(p.Message), "embed") {
		t.Errorf("font-embedding message %q must mention embedding", p.Message)
	}
	if p.SpecRef == "" {
		t.Errorf("font-embedding specRef is empty")
	}
}

func TestValidate_FontEmbeddedPasses(t *testing.T) {
	// TrueType font with a FontFile2-bearing descriptor is embedded -> no hit.
	ins, tabID := writeTempPDF(t, "embed.pdf", assemblexref(
		"%PDF-1.4\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> >>\nendobj\n\n",
		"4 0 obj\n<< /Type /Font /Subtype /TrueType /BaseFont /MyFont /FontDescriptor 5 0 R >>\nendobj\n\n",
		"5 0 obj\n<< /Type /FontDescriptor /FontName /MyFont /FontFile2 6 0 R >>\nendobj\n\n",
		"6 0 obj\n<< /Length 4 /Length1 4 >>\nstream\nTTF!\nendstream\nendobj\n\n",
	))
	res, err := ins.Validate(tabID, ProfilePDFA1B)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if p := problemByRule(res.Problems, "font-embedding"); p != nil {
		t.Errorf("font-embedding fired on an embedded font: %+v", p)
	}
}

func TestValidate_OutputIntentGatedOnDeviceColor(t *testing.T) {
	// Device RGB fill (rg) with no /OutputIntents -> output-intent error.
	withColor := assemblexref(
		"%PDF-1.4\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>\nendobj\n\n",
		"4 0 obj\n<< /Length 21 >>\nstream\n1 0 0 rg 0 0 10 10 re f\nendstream\nendobj\n\n",
	)
	ins, tabID := writeTempPDF(t, "color.pdf", withColor)
	res, err := ins.Validate(tabID, ProfilePDFA1B)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if problemByRule(res.Problems, "output-intent") == nil {
		t.Errorf("output-intent rule must fire when device color is used without an OutputIntent:\n%+v", res.Problems)
	}

	// No color operators -> the rule must NOT fire (matches veraPDF).
	noColor := assemblexref(
		"%PDF-1.4\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>\nendobj\n\n",
		"4 0 obj\n<< /Length 33 >>\nstream\nBT /F1 24 Tf 72 720 Td (Hi) Tj ET\nendstream\nendobj\n\n",
	)
	ins2, tabID2 := writeTempPDF(t, "nocolor.pdf", noColor)
	res2, err := ins2.Validate(tabID2, ProfilePDFA1B)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if p := problemByRule(res2.Problems, "output-intent"); p != nil {
		t.Errorf("output-intent must not fire without device color: %+v", p)
	}
}

func TestValidate_NoJSLaunch(t *testing.T) {
	ins, tabID := writeTempPDF(t, "js.pdf", assemblexref(
		"%PDF-1.4\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /OpenAction 4 0 R >>\nendobj\n\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n",
		"4 0 obj\n<< /Type /Action /S /JavaScript /JS (app.alert\\(1\\);) >>\nendobj\n\n",
	))
	res, err := ins.Validate(tabID, ProfilePDFA1B)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	p := problemByRule(res.Problems, "no-js-launch")
	if p == nil {
		t.Fatalf("no-js-launch rule did not fire on a JavaScript action:\n%+v", res.Problems)
	}
	if p.ObjNodeID != "obj:0:4" {
		t.Errorf("no-js-launch objNodeId = %q, want obj:0:4", p.ObjNodeID)
	}
}

func TestValidate_XMPAndIDMissing(t *testing.T) {
	ins, tabID := writeTempPDF(t, "bare.pdf", assemblexref(
		"%PDF-1.4\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n",
	))
	res, err := ins.Validate(tabID, ProfilePDFA1B)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if problemByRule(res.Problems, "xmp-metadata") == nil {
		t.Errorf("xmp-metadata rule must fire when no /Metadata packet is present")
	}
	if problemByRule(res.Problems, "document-id") == nil {
		t.Errorf("document-id rule must fire when the trailer has no /ID")
	}
	// Every PDF/A-1b problem is error severity and carries the profile tag.
	for _, p := range res.Problems {
		if p.Severity != "error" {
			t.Errorf("PDF/A-1b problem %q severity = %q, want error", p.RuleID, p.Severity)
		}
		if p.Profile != ProfilePDFA1B {
			t.Errorf("problem %q profile = %q, want %q", p.RuleID, p.Profile, ProfilePDFA1B)
		}
	}
	if res.Disclaimer == "" || !strings.Contains(res.Disclaimer, "structural checks only") {
		t.Errorf("result disclaimer missing the not-authoritative note: %q", res.Disclaimer)
	}
}

func TestValidate_PDFUAUntaggedWarnings(t *testing.T) {
	ins, tabID := writeTempPDF(t, "untagged.pdf", assemblexref(
		"%PDF-1.4\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n",
	))
	res, err := ins.Validate(tabID, ProfilePDFUA1Structural)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if res.Summary.Errors != 0 {
		t.Errorf("PDF/UA structural profile must emit zero errors, got %d", res.Summary.Errors)
	}
	if res.Summary.Warnings < 3 {
		t.Errorf("untagged doc should raise 3 PDF/UA warnings, got %d", res.Summary.Warnings)
	}
	for _, id := range []string{"marked", "struct-tree-root", "lang"} {
		p := problemByRule(res.Problems, id)
		if p == nil {
			t.Errorf("PDF/UA rule %q did not fire on an untagged doc", id)
			continue
		}
		if p.Severity != "warning" {
			t.Errorf("PDF/UA rule %q severity = %q, want warning", id, p.Severity)
		}
	}
	// The missing /Lang problem is document-level (no object ref).
	if lang := problemByRule(res.Problems, "lang"); lang != nil && lang.ObjRef != "" {
		t.Errorf("missing-/Lang is document-level; objRef must be empty, got %q", lang.ObjRef)
	}
}

func TestValidate_PDFUATaggedClean(t *testing.T) {
	ins, tabID := writeTempPDF(t, "tagged.pdf", assemblexref(
		"%PDF-1.4\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 4 0 R /Lang (en-US) >>\nendobj\n\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n",
		"4 0 obj\n<< /Type /StructTreeRoot >>\nendobj\n\n",
	))
	res, err := ins.Validate(tabID, ProfilePDFUA1Structural)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(res.Problems) != 0 {
		t.Errorf("tagged doc must yield zero PDF/UA structural problems, got %+v", res.Problems)
	}
}

func TestValidate_EncryptedResultHelper(t *testing.T) {
	res := EncryptedResult(ProfilePDFA1B)
	if res.Summary.Errors != 1 || len(res.Problems) != 1 {
		t.Fatalf("EncryptedResult must carry exactly one error problem, got %+v", res)
	}
	p := res.Problems[0]
	if p.Severity != "error" || !strings.Contains(strings.ToLower(p.Message), "encrypt") {
		t.Errorf("EncryptedResult problem = %+v, want an error mentioning encryption", p)
	}
}

// --- /Info<->XMP consistency: false-positive vectors (code-review rounds 1-3) --
//
// Each test proves the consistency check does NOT emit a spurious xmp-metadata
// error when /Info and XMP encode the SAME text via different but legal
// serializations. A spurious error gates exit 1 and breaks the oracle
// "no false errors" guarantee, so these lock the hardening in the "never a
// false error" direction the story mandates.

// xmpConsistencyHasFalseMismatch runs pdfa-1b and returns the xmp-metadata
// problem if one fired (a mismatch), or nil.
func xmpConsistencyProblem(t *testing.T, xmp, infoBody string) *Problem {
	t.Helper()
	ins, tabID := writeTempPDF(t, "xmpinfo.pdf", xmpInfoPDF(xmp, infoBody))
	res, err := ins.Validate(tabID, ProfilePDFA1B)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return problemByRule(res.Problems, "xmp-metadata")
}

// Round 1: UTF-16BE /Info value decoded via StringOrHexLiteral. /Info /Title is
// a hex literal UTF-16BE-with-BOM "Cafe" (with an accented e); XMP dc:title is
// the same text in UTF-8. Byte comparison would false-mismatch; decoding must
// make them equal.
func TestValidate_XMPInfo_UTF16BENoFalseMismatch(t *testing.T) {
	xmp := wrapXMP(`<dc:title><rdf:Alt><rdf:li xml:lang="x-default">Café</rdf:li></rdf:Alt></dc:title>`)
	// UTF-16BE BOM FEFF + C a f é(00E9).
	if p := xmpConsistencyProblem(t, xmp, "/Title <FEFF00430061006600E9>"); p != nil {
		t.Errorf("UTF-16BE /Info decoded to the same text as UTF-8 XMP; must not false-mismatch: %q", p.Message)
	}
}

// Round 1: the five named XML entities. XMP "AT&amp;T" must compare equal to
// /Info "AT&T".
func TestValidate_XMPInfo_NamedEntityNoFalseMismatch(t *testing.T) {
	xmp := wrapXMP(`<pdf:Producer>AT&amp;T &lt;Corp&gt;</pdf:Producer>`)
	if p := xmpConsistencyProblem(t, xmp, "/Producer (AT&T <Corp>)"); p != nil {
		t.Errorf("named XML entities must be unescaped before comparison; must not false-mismatch: %q", p.Message)
	}
}

// Round 3: numeric XML character references (decimal &#233; and hex &#xE9;) must
// decode to the same accented text as the UTF-16BE /Info value.
func TestValidate_XMPInfo_NumericEntityNoFalseMismatch(t *testing.T) {
	// /Info /Title = UTF-16BE "Cafe"+accented-e.
	const infoTitle = "/Title <FEFF00430061006600E9>"
	decimal := wrapXMP(`<dc:title><rdf:Alt><rdf:li>Caf&#233;</rdf:li></rdf:Alt></dc:title>`)
	if p := xmpConsistencyProblem(t, decimal, infoTitle); p != nil {
		t.Errorf("decimal numeric char ref must decode; must not false-mismatch: %q", p.Message)
	}
	hexRef := wrapXMP(`<dc:title><rdf:Alt><rdf:li>Caf&#xE9;</rdf:li></rdf:Alt></dc:title>`)
	if p := xmpConsistencyProblem(t, hexRef, infoTitle); p != nil {
		t.Errorf("hex numeric char ref must decode; must not false-mismatch: %q", p.Message)
	}
}

// Round 1: a multi-value dc:creator rdf:Seq is ambiguous against a single /Info
// string, so it is skipped (never a mismatch).
func TestValidate_XMPInfo_MultiValueSeqSkipped(t *testing.T) {
	xmp := wrapXMP(`<dc:creator><rdf:Seq><rdf:li>Alice</rdf:li><rdf:li>Bob</rdf:li></rdf:Seq></dc:creator>`)
	if p := xmpConsistencyProblem(t, xmp, "/Author (Alice)"); p != nil {
		t.Errorf("multi-value rdf:Seq is ambiguous and must be skipped, not compared: %q", p.Message)
	}
}

// Round 2: date fields (CreationDate/ModDate) are EXCLUDED from the comparison
// because /Info PDF-date syntax never equals XMP ISO-8601 syntax. Both present,
// different format -> must not mismatch.
func TestValidate_XMPInfo_DateFieldsExcluded(t *testing.T) {
	xmp := wrapXMP(`<xmp:CreateDate>2024-01-01T12:00:00Z</xmp:CreateDate><xmp:ModifyDate>2024-01-02T12:00:00Z</xmp:ModifyDate>`)
	info := "/CreationDate (D:20240101120000Z) /ModDate (D:20240102120000Z)"
	if p := xmpConsistencyProblem(t, xmp, info); p != nil {
		t.Errorf("date fields are excluded from /Info<->XMP comparison; must not fire: %q", p.Message)
	}
}

// Round 2: pretty-printed multi-line XMP whitespace must be normalized before
// comparison so it does not false-mismatch the single-line /Info value.
func TestValidate_XMPInfo_WhitespaceNormalized(t *testing.T) {
	xmp := wrapXMP("<pdf:Producer>Acme\n            Corp</pdf:Producer>")
	if p := xmpConsistencyProblem(t, xmp, "/Producer (Acme Corp)"); p != nil {
		t.Errorf("internal whitespace must be normalized; must not false-mismatch: %q", p.Message)
	}
}

// Positive control: a genuine value difference MUST still fire, proving the
// hardening above did not disable the check outright.
func TestValidate_XMPInfo_GenuineMismatchStillFires(t *testing.T) {
	xmp := wrapXMP(`<pdf:Producer>Different Corp</pdf:Producer>`)
	p := xmpConsistencyProblem(t, xmp, "/Producer (Acme Corp)")
	if p == nil {
		t.Fatalf("a genuine /Info<->XMP value difference must still be reported")
	}
	if p.Severity != "error" || !strings.Contains(p.Message, "Producer") {
		t.Errorf("genuine mismatch problem = %+v, want an error naming Producer", p)
	}
}

// --- ProfileGates: fail-closed gate foundation (review #2) --------------------

// ProfileGates is the pdfcore decision the CLI uses to fail closed when a gating
// rule degrades to info. pdfa-1b has error rules (gates); pdfua-1-structural is
// all warnings (never gates); an unknown profile matches nothing.
func TestProfileGates(t *testing.T) {
	if !ProfileGates(ProfilePDFA1B) {
		t.Errorf("ProfileGates(%q) = false, want true (has error rules)", ProfilePDFA1B)
	}
	if ProfileGates(ProfilePDFUA1Structural) {
		t.Errorf("ProfileGates(%q) = true, want false (all warnings)", ProfilePDFUA1Structural)
	}
	if ProfileGates("bogus-profile") {
		t.Errorf("ProfileGates(unknown) = true, want false")
	}
}

// --- Type0 / CIDFont double-report skip (review #1) --------------------------

// A non-embedded Type0 composite font must be reported ONCE (on the Type0
// parent), not twice (Type0 + its descendant CIDFont, which is /Type /Font too).
func TestValidate_Type0NotDoubleReported(t *testing.T) {
	ins, tabID := writeTempPDF(t, "type0.pdf", assemblexref(
		"%PDF-1.4\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> >>\nendobj\n\n",
		"4 0 obj\n<< /Type /Font /Subtype /Type0 /BaseFont /MyCID /Encoding /Identity-H /DescendantFonts [5 0 R] >>\nendobj\n\n",
		"5 0 obj\n<< /Type /Font /Subtype /CIDFontType2 /BaseFont /MyCID /FontDescriptor 6 0 R >>\nendobj\n\n",
		"6 0 obj\n<< /Type /FontDescriptor /FontName /MyCID >>\nendobj\n\n",
	))
	res, err := ins.Validate(tabID, ProfilePDFA1B)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if n := countByRule(res.Problems, "font-embedding"); n != 1 {
		t.Fatalf("non-embedded Type0 font must be reported once, got %d:\n%+v", n, res.Problems)
	}
	if p := problemByRule(res.Problems, "font-embedding"); p.ObjNodeID != "obj:0:4" {
		t.Errorf("Type0 font-embedding must anchor on the Type0 parent obj:0:4, got %q", p.ObjNodeID)
	}
}

// --- Indirect vs direct /OpenAction (review #1 + #2) -------------------------

// An INDIRECT catalog /OpenAction points at an object forEachObject already
// visits; the catalog scan must skip it so the action is reported exactly once.
func TestValidate_IndirectOpenActionNotDoubleReported(t *testing.T) {
	ins, tabID := writeTempPDF(t, "indirect-oa.pdf", assemblexref(
		"%PDF-1.4\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /OpenAction 4 0 R >>\nendobj\n\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n",
		"4 0 obj\n<< /Type /Action /S /JavaScript /JS (app.alert\\(1\\);) >>\nendobj\n\n",
	))
	res, err := ins.Validate(tabID, ProfilePDFA1B)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if n := countByRule(res.Problems, "no-js-launch"); n != 1 {
		t.Fatalf("indirect /OpenAction action must be reported once, got %d:\n%+v", n, res.Problems)
	}
	if p := problemByRule(res.Problems, "no-js-launch"); p.ObjNodeID != "obj:0:4" {
		t.Errorf("indirect /OpenAction must anchor on the indirect object obj:0:4, got %q", p.ObjNodeID)
	}
}

// A DIRECT (inline) catalog /OpenAction is never visited by forEachObject, so
// the catalog scan must catch it (document-level, no object ref).
func TestValidate_DirectOpenActionReported(t *testing.T) {
	ins, tabID := writeTempPDF(t, "direct-oa.pdf", assemblexref(
		"%PDF-1.4\n",
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /OpenAction << /S /Launch /F (evil.exe) >> >>\nendobj\n\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n\n",
	))
	res, err := ins.Validate(tabID, ProfilePDFA1B)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	p := problemByRule(res.Problems, "no-js-launch")
	if p == nil {
		t.Fatalf("direct (inline) catalog /OpenAction Launch action must be reported:\n%+v", res.Problems)
	}
	if p.ObjRef != "" {
		t.Errorf("direct /OpenAction problem is document-level; objRef must be empty, got %q", p.ObjRef)
	}
	if !strings.Contains(p.Message, "OpenAction") {
		t.Errorf("direct /OpenAction message should name /OpenAction, got %q", p.Message)
	}
}
