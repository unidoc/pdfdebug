package pdfcore

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// fontFlagNames maps 1-indexed bit positions to PDF 1.7 spec 9.8.2 Table 123
// flag names. Bits 5 and 8-16 are reserved and intentionally absent. Bits
// outside this map are ignored even if set (regression guard for malformed
// FontDescriptor /Flags integers).
var fontFlagNames = map[int]string{
	1:  "FixedPitch",
	2:  "Serif",
	3:  "Symbolic",
	4:  "Script",
	6:  "Nonsymbolic",
	7:  "Italic",
	17: "AllCap",
	18: "SmallCap",
	19: "ForceBold",
}

// fontFlagOrder fixes deterministic emission order so the frontend pills
// render in a stable sequence. Matches the bit-number order in fontFlagNames.
var fontFlagOrder = []int{1, 2, 3, 4, 6, 7, 17, 18, 19}

// GetFontDetail resolves a tree node ID to a /Type /Font dict and extracts the
// consolidated FontDetail payload: metadata, encoding (named or Differences),
// ToUnicode mappings, FontDescriptor summary, embedded badge state, and the
// descendant CIDFont chain for composite (/Subtype /Type0) fonts.
//
// Returns ErrNotAFont when the resolved dict does not carry /Type /Font; the
// frontend uses this sentinel to silently fall back to the generic DictView
// for the /Resources /Font resource-map iconHint false positive (story 9-9
// Task 1.3).
func (ins *Inspector) GetFontDetail(tabID, nodeID string) (*FontDetail, error) {
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
	// Serialize pdfcpu access. Font detail extraction walks
	// /FontDescriptor, /Encoding, /DescendantFonts via Dereference.
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
		return nil, ErrNotAFont
	}

	// Verify /Type /Font. Without this, the iconHint='font' false positive on
	// the /Resources /Font resource map (a dict of font-name -> Font dict)
	// would feed the resource map into the font extractor and produce garbage.
	if !isFontDict(d) {
		return nil, ErrNotAFont
	}

	detail := buildFontDetailFromDict(doc, nodeID, d)
	return detail, nil
}

// isFontDict reports whether d has /Type /Font.
func isFontDict(d pdfcpu_types.Dict) bool {
	typeVal, ok := d["Type"]
	if !ok {
		return false
	}
	name, ok := typeVal.(pdfcpu_types.Name)
	if !ok {
		return false
	}
	return string(name) == "Font"
}

// maxDescendantDepth caps recursion through /DescendantFonts. Per PDF spec a
// composite Type0 font has exactly one CIDFont descendant (CIDFontType0/2,
// never Type0) so depth 1 is the legitimate maximum. A higher cap absorbs
// malformed input without permitting cycles.
const maxDescendantDepth = 2

// buildFontDetailFromDict drives the full extraction pipeline for one Font
// dict (which may itself be a descendant CIDFont). Every pdfcpu call inside
// is wrapped in safeCall.
func buildFontDetailFromDict(doc *DocumentState, nodeID string, d pdfcpu_types.Dict) *FontDetail {
	return buildFontDetailFromDictDepth(doc, nodeID, d, 0)
}

// buildFontDetailFromDictDepth is the recursion-aware worker. depth tracks
// /DescendantFonts traversal and short-circuits the descendant call past
// maxDescendantDepth to prevent cycles from blowing the stack.
func buildFontDetailFromDictDepth(doc *DocumentState, nodeID string, d pdfcpu_types.Dict, depth int) *FontDetail {
	detail := &FontDetail{
		NodeID:            nodeID,
		ObjectRef:         objectRefFromNodeID(nodeID),
		Differences:       []EncodingDifference{},
		ToUnicodeMappings: []ToUnicodeMapping{},
	}

	if name, ok := nameField(d, "Subtype"); ok {
		detail.Subtype = name
	}
	if name, ok := nameField(d, "BaseFont"); ok {
		detail.BaseFont = "/" + name
	}
	if i, ok := intField(d, "FirstChar"); ok {
		detail.FirstChar = i
	}
	if i, ok := intField(d, "LastChar"); ok {
		detail.LastChar = i
	}

	// Encoding: name (e.g. /WinAnsiEncoding) OR dict with /BaseEncoding and
	// /Differences. Absent /Encoding -> "Built-in encoding" sentinel handled
	// frontend-side by checking encodingName=="" && differences.length==0.
	populateEncoding(doc, d, detail)

	// ToUnicode CMap parsing. ToUnicodeError captures partial-success state.
	populateToUnicode(doc, d, detail)

	// FontDescriptor (own or descendant for Type0).
	if fd := resolveFontDescriptor(doc, d); fd != nil {
		detail.FontDescriptor = fd
	}

	// CIDFont-specific fields: CIDSystemInfo, CIDToGIDMap mode, DW.
	// Populated when this dict is itself a CIDFont (i.e. a Type0 parent's
	// descendant), so the FontPreview's Descendant Font section can render
	// them. No-op for Type1 / TrueType / Type0 parents -- those dicts
	// shouldn't carry these keys.
	if detail.Subtype == "CIDFontType0" || detail.Subtype == "CIDFontType2" {
		detail.CIDSystemInfo = resolveCIDSystemInfo(doc, d)
		detail.CIDToGIDMap = resolveCIDToGIDMap(doc, d)
		if dw, ok := intField(d, "DW"); ok {
			detail.DefaultWidth = dw
		}
	}

	// Descendant CIDFont for composite fonts. depth cap prevents runaway
	// recursion if a malformed PDF links descendants in a cycle.
	if detail.Subtype == "Type0" && depth < maxDescendantDepth {
		if desc := resolveDescendant(doc, d, depth+1); desc != nil {
			detail.Descendant = desc
		}
	}

	// Assembled per-code mapping table + health signals. Assembled
	// from the Differences / ToUnicodeMappings already populated above -- no
	// re-parsing. Must run after populateEncoding/populateToUnicode.
	assembleMapping(detail)

	// Embedded badge state: read from the FontDescriptor that actually carries
	// the FontFile. For Type0 that lives on the descendant; for everything else
	// it lives on the font's own FontDescriptor.
	descriptorForBadge := detail.FontDescriptor
	if detail.Subtype == "Type0" && detail.Descendant != nil && detail.Descendant.FontDescriptor != nil {
		descriptorForBadge = detail.Descendant.FontDescriptor
	}
	if descriptorForBadge != nil && descriptorForBadge.FontFileFormat != "" {
		detail.Embedded = true
	}

	return detail
}

// assembleMapping builds detail.MappingRows (the per-code JOIN of Differences
// and ToUnicodeMappings over the union of declared codes) and detail.Health
// (the coverage/health signals), from the already-populated parser outputs.
// Pure assembly: it re-parses nothing.
//
// MappingRows is always a non-nil slice and Health is always populated, even
// on a degraded font (malformed ToUnicode), so the frontend never nil-derefs.
func assembleMapping(detail *FontDetail) {
	// Index the two code-keyed sources.
	glyphByCode := make(map[int]string, len(detail.Differences))
	for _, diff := range detail.Differences {
		glyphByCode[diff.Code] = diff.GlyphName
	}
	tuByCode := make(map[int]ToUnicodeMapping, len(detail.ToUnicodeMappings))
	for _, m := range detail.ToUnicodeMappings {
		tuByCode[m.Code] = m
	}

	// Union of declared codes, sorted ascending for deterministic output.
	codeSet := make(map[int]struct{}, len(glyphByCode)+len(tuByCode))
	for c := range glyphByCode {
		codeSet[c] = struct{}{}
	}
	for c := range tuByCode {
		codeSet[c] = struct{}{}
	}
	codes := make([]int, 0, len(codeSet))
	for c := range codeSet {
		codes = append(codes, c)
	}
	sort.Ints(codes)

	rows := make([]FontMappingRow, 0, len(codes))
	for _, c := range codes {
		row := FontMappingRow{
			Code:    c,
			CodeHex: fmt.Sprintf("0x%X", c),
		}
		if g, ok := glyphByCode[c]; ok {
			row.GlyphName = g
		}
		if m, ok := tuByCode[c]; ok {
			row.Unicode = m.Unicode
			row.UnicodeText = m.Glyph
		}
		rows = append(rows, row)
	}
	detail.MappingRows = rows

	// Health signals. A ToUnicode that is absent OR present-but-unparseable
	// yields no code-to-Unicode coverage -- both are "missing" for diagnosis.
	toUnicodeMissing := len(detail.ToUnicodeMappings) == 0
	// Codes declared in /Differences but absent from /ToUnicode: extraction
	// fails for each. Emitted in ascending code order.
	encodingWithout := []int{}
	for _, c := range codes {
		if _, hasGlyph := glyphByCode[c]; !hasGlyph {
			continue
		}
		if _, hasTU := tuByCode[c]; !hasTU {
			encodingWithout = append(encodingWithout, c)
		}
	}
	detail.Health = &FontHealth{
		DeclaredCodeCount:             len(codes),
		ToUnicodeMissing:              toUnicodeMissing,
		IdentityWithoutToUnicode:      isIdentityEncoding(detail.EncodingName) && toUnicodeMissing,
		EncodingWithoutToUnicodeCodes: encodingWithout,
	}
}

// isIdentityEncoding reports whether the named encoding is an Identity CMap
// (Identity-H / Identity-V), the composite-font encoding that maps codes
// straight to CIDs with no Unicode semantics. The leading '/' is tolerated.
func isIdentityEncoding(name string) bool {
	n := strings.TrimPrefix(name, "/")
	return n == "Identity-H" || n == "Identity-V"
}

// nameField returns the string form of d[key] when it is a Name; the bool is
// false otherwise. Names are returned WITHOUT the leading '/' so callers can
// decide whether to prepend it for display.
func nameField(d pdfcpu_types.Dict, key string) (string, bool) {
	v, ok := d[key]
	if !ok {
		return "", false
	}
	n, ok := v.(pdfcpu_types.Name)
	if !ok {
		return "", false
	}
	return string(n), true
}

// intField returns d[key] coerced to int when it is an Integer; false otherwise.
// No IndirectRef dereferencing here -- font dicts overwhelmingly inline these
// simple scalar fields; the few that don't will register as "missing" and the
// frontend handles 0 defaults gracefully.
func intField(d pdfcpu_types.Dict, key string) (int, bool) {
	v, ok := d[key]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case pdfcpu_types.Integer:
		return int(x), true
	case pdfcpu_types.Float:
		return int(x), true
	}
	return 0, false
}

// floatField returns d[key] as float64 when it is Integer or Float.
func floatField(d pdfcpu_types.Dict, key string) (float64, bool) {
	v, ok := d[key]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case pdfcpu_types.Integer:
		return float64(x), true
	case pdfcpu_types.Float:
		return float64(x), true
	}
	return 0, false
}

// populateEncoding fills detail.EncodingName / BaseEncoding / Differences from
// the font dict's /Encoding entry. Branches on whether /Encoding is a Name,
// a Dict, or absent; absent is the "Built-in encoding" case handled frontend-side.
func populateEncoding(doc *DocumentState, d pdfcpu_types.Dict, detail *FontDetail) {
	encObj, ok := d["Encoding"]
	if !ok {
		return
	}
	// Dereference IndirectRef encoding (rare but legal).
	encObj = dereferenceIfRef(doc, encObj)

	switch v := encObj.(type) {
	case pdfcpu_types.Name:
		detail.EncodingName = "/" + string(v)
	case pdfcpu_types.Dict:
		if base, ok := nameField(v, "BaseEncoding"); ok {
			detail.BaseEncoding = "/" + base
		}
		if diffsObj, ok := v["Differences"]; ok {
			diffsObj = dereferenceIfRef(doc, diffsObj)
			if arr, ok := diffsObj.(pdfcpu_types.Array); ok {
				detail.Differences = parseDifferences(arr)
			}
		}
	}
}

// parseDifferences walks a PDF /Differences array per spec section 9.6.6.1:
// integers reset the running code; names append at the current code, then
// the code increments. Bad entries are skipped silently (no panic; the
// frontend tolerates partial tables).
func parseDifferences(arr pdfcpu_types.Array) []EncodingDifference {
	out := []EncodingDifference{}
	currentCode := -1
	for _, elem := range arr {
		switch v := elem.(type) {
		case pdfcpu_types.Integer:
			currentCode = int(v)
			// Silently skip out-of-range codes. Malformed
			// /Differences arrays in the wild carry negatives or values >255
			// (typo / merge-conflict residue). With no guard, the subsequent
			// Name entries appended at those codes pollute the encoding table.
			if currentCode < 0 || currentCode > 255 {
				continue
			}
		case pdfcpu_types.Name:
			if currentCode < 0 || currentCode > 255 {
				continue
			}
			out = append(out, EncodingDifference{
				Code:      currentCode,
				GlyphName: "/" + string(v),
			})
			currentCode++
		}
	}
	return out
}

// populateToUnicode extracts the bfchar/bfrange contents of a /ToUnicode CMap
// stream. On parse error, ToUnicodeError is populated and ToUnicodeMappings is
// left empty; the section still renders (partial-success contract).
func populateToUnicode(doc *DocumentState, d pdfcpu_types.Dict, detail *FontDetail) {
	tuObj, ok := d["ToUnicode"]
	if !ok {
		return
	}
	tuObj = dereferenceIfRef(doc, tuObj)
	sd, ok := tuObj.(pdfcpu_types.StreamDict)
	if !ok {
		// Anything other than a stream is unusable; record nothing and let
		// the frontend show "no ToUnicode" implicitly.
		return
	}

	var content []byte
	err := safeCall(func() error {
		if e := sd.Decode(); e != nil {
			return e
		}
		content = sd.Content
		return nil
	})
	if err != nil {
		detail.ToUnicodeError = fmt.Sprintf("CMap decode failed: %v", err)
		return
	}
	if len(content) == 0 {
		return
	}

	mappings, parseErr := parseToUnicodeCMap(content)
	if parseErr != nil {
		detail.ToUnicodeError = parseErr.Error()
		return
	}
	detail.ToUnicodeMappings = mappings
}

// resolveFontDescriptor follows the /FontDescriptor entry, dereferences it,
// and reads the decoded payload. Returns nil when missing or unresolvable.
func resolveFontDescriptor(doc *DocumentState, d pdfcpu_types.Dict) *FontDescriptorInfo {
	fdObj, ok := d["FontDescriptor"]
	if !ok {
		return nil
	}
	nodeID := ""
	objectRef := ""
	if ref, isRef := fdObj.(pdfcpu_types.IndirectRef); isRef {
		num := ref.ObjectNumber.Value()
		gen := ref.GenerationNumber.Value()
		nodeID = fmt.Sprintf("obj:%d:%d", gen, num)
		objectRef = fmt.Sprintf("%d %d R", num, gen)
	}
	fdObj = dereferenceIfRef(doc, fdObj)
	fd, ok := fdObj.(pdfcpu_types.Dict)
	if !ok {
		return nil
	}

	info := &FontDescriptorInfo{
		NodeID:    nodeID,
		ObjectRef: objectRef,
		FontBBox:  []float64{},
		FlagNames: []string{},
	}
	if name, ok := nameField(fd, "FontName"); ok {
		info.FontName = name
	}
	if i, ok := intField(fd, "Flags"); ok {
		info.Flags = i
		info.FlagNames = decodeFontFlags(i)
	}
	if f, ok := floatField(fd, "ItalicAngle"); ok {
		info.ItalicAngle = f
	}
	if f, ok := floatField(fd, "Ascent"); ok {
		info.Ascent = f
	}
	if f, ok := floatField(fd, "Descent"); ok {
		info.Descent = f
	}
	if f, ok := floatField(fd, "CapHeight"); ok {
		info.CapHeight = f
	}
	if f, ok := floatField(fd, "StemV"); ok {
		info.StemV = f
	}
	if v, ok := fd["FontBBox"]; ok {
		if arr, ok := dereferenceIfRef(doc, v).(pdfcpu_types.Array); ok {
			info.FontBBox = arrayToFloats(arr)
		}
	}

	// FontFile / FontFile2 / FontFile3 detection. The descriptor carries at
	// most one of these; FontFile3 additionally carries a /Subtype that names
	// the actual font program format (OpenType, Type1C, CIDFontType0C).
	format, size := detectFontFile(doc, fd)
	info.FontFileFormat = format
	info.FontFileSize = size

	return info
}

// decodeFontFlags decodes a PDF /Flags integer into the set of human-readable
// flag names. Bits 5 and 8-16 are reserved per spec and never produce a flag
// name (the regression guard the frontend test for "Reserved5" anchors).
func decodeFontFlags(flags int) []string {
	if flags == 0 {
		return []string{}
	}
	out := []string{}
	for _, bit := range fontFlagOrder {
		if (flags>>(bit-1))&1 == 1 {
			if name, ok := fontFlagNames[bit]; ok {
				out = append(out, name)
			}
		}
	}
	return out
}

// detectFontFile returns the format string and decoded byte-length for the
// embedded font program referenced by /FontFile, /FontFile2, or /FontFile3.
// Returns ("", 0) when none is present (unembedded font).
//
// FontFile (Type1), FontFile2 (TrueType), FontFile3 (other -- the Subtype on
// the stream dict names the actual format: OpenType, Type1C, CIDFontType0C).
func detectFontFile(doc *DocumentState, fd pdfcpu_types.Dict) (string, int) {
	for _, key := range []string{"FontFile", "FontFile2", "FontFile3"} {
		obj, ok := fd[key]
		if !ok {
			continue
		}
		obj = dereferenceIfRef(doc, obj)
		sd, ok := obj.(pdfcpu_types.StreamDict)
		if !ok {
			continue
		}
		size := streamDecodedSize(sd)
		switch key {
		case "FontFile":
			return "Type1", size
		case "FontFile2":
			return "TrueType", size
		case "FontFile3":
			if name, ok := nameField(sd.Dict, "Subtype"); ok {
				return name, size
			}
			return "FontFile3", size
		}
	}
	return "", 0
}

// streamDecodedSize returns the byte length of the decoded stream content.
// Best-effort: a decode failure returns 0 rather than failing the whole
// FontDescriptor extraction.
func streamDecodedSize(sd pdfcpu_types.StreamDict) int {
	var size int
	_ = safeCall(func() error {
		if e := sd.Decode(); e != nil {
			return e
		}
		size = len(sd.Content)
		return nil
	})
	return size
}

// resolveDescendant returns the populated FontDetail for the first entry of
// /DescendantFonts. PDF spec section 9.7.6.2: composite fonts have exactly
// one descendant (the spec says "shall be a one-element array"). depth is
// the current /DescendantFonts traversal depth and is forwarded so the
// recursion cap in buildFontDetailFromDictDepth applies.
func resolveDescendant(doc *DocumentState, d pdfcpu_types.Dict, depth int) *FontDetail {
	descObj, ok := d["DescendantFonts"]
	if !ok {
		return nil
	}
	descObj = dereferenceIfRef(doc, descObj)
	arr, ok := descObj.(pdfcpu_types.Array)
	if !ok || len(arr) == 0 {
		return nil
	}
	first := arr[0]
	nodeID := ""
	if ref, isRef := first.(pdfcpu_types.IndirectRef); isRef {
		num := ref.ObjectNumber.Value()
		gen := ref.GenerationNumber.Value()
		nodeID = fmt.Sprintf("obj:%d:%d", gen, num)
	}
	first = dereferenceIfRef(doc, first)
	descDict, ok := first.(pdfcpu_types.Dict)
	if !ok {
		return nil
	}
	// Descendant CIDFont dicts carry /Type /Font; reuse the same extractor.
	if !isFontDict(descDict) {
		return nil
	}
	return buildFontDetailFromDictDepth(doc, nodeID, descDict, depth)
}

// dereferenceIfRef returns the resolved object when v is an IndirectRef,
// otherwise v unchanged. Errors return v as-is; the caller's type assertion
// will then fail safely.
func dereferenceIfRef(doc *DocumentState, v pdfcpu_types.Object) pdfcpu_types.Object {
	ref, ok := v.(pdfcpu_types.IndirectRef)
	if !ok {
		return v
	}
	if doc == nil || doc.PDFContext == nil {
		return v
	}
	var out pdfcpu_types.Object
	err := safeCall(func() error {
		resolved, e := doc.PDFContext.Dereference(ref)
		if e != nil {
			return e
		}
		out = resolved
		return nil
	})
	if err != nil || out == nil {
		return v
	}
	return out
}

// resolveCIDSystemInfo extracts a CIDFont's /CIDSystemInfo dict into the
// IPC struct. Returns nil when the field is absent or unresolvable so the
// frontend can omit the row.
func resolveCIDSystemInfo(doc *DocumentState, d pdfcpu_types.Dict) *CIDSystemInfo {
	v, ok := d["CIDSystemInfo"]
	if !ok {
		return nil
	}
	v = dereferenceIfRef(doc, v)
	cd, ok := v.(pdfcpu_types.Dict)
	if !ok {
		return nil
	}
	info := &CIDSystemInfo{}
	// /Registry and /Ordering can be string literals (parenthesized) or hex
	// strings; pdfcpu surfaces both as StringLiteral / HexLiteral.
	if s, ok := stringField(cd, "Registry"); ok {
		info.Registry = s
	}
	if s, ok := stringField(cd, "Ordering"); ok {
		info.Ordering = s
	}
	if i, ok := intField(cd, "Supplement"); ok {
		info.Supplement = i
	}
	return info
}

// resolveCIDToGIDMap returns a display string for /CIDToGIDMap: "Identity"
// for the name form, "Stream (<size> bytes)" when the value is a stream, or
// "" when the field is absent.
func resolveCIDToGIDMap(doc *DocumentState, d pdfcpu_types.Dict) string {
	v, ok := d["CIDToGIDMap"]
	if !ok {
		return ""
	}
	v = dereferenceIfRef(doc, v)
	switch x := v.(type) {
	case pdfcpu_types.Name:
		return string(x)
	case pdfcpu_types.StreamDict:
		size := streamDecodedSize(x)
		return fmt.Sprintf("Stream (%d bytes)", size)
	}
	return ""
}

// stringField returns d[key] coerced to a Go string when it is a
// StringLiteral or HexLiteral, "" otherwise.
func stringField(d pdfcpu_types.Dict, key string) (string, bool) {
	v, ok := d[key]
	if !ok {
		return "", false
	}
	switch x := v.(type) {
	case pdfcpu_types.StringLiteral:
		return string(x), true
	case pdfcpu_types.HexLiteral:
		return string(x), true
	}
	return "", false
}

// arrayToFloats coerces a pdfcpu Array of numeric elements to []float64,
// dropping non-numeric entries silently. Used for /FontBBox.
func arrayToFloats(arr pdfcpu_types.Array) []float64 {
	out := make([]float64, 0, len(arr))
	for _, elem := range arr {
		switch v := elem.(type) {
		case pdfcpu_types.Integer:
			out = append(out, float64(v))
		case pdfcpu_types.Float:
			out = append(out, float64(v))
		}
	}
	return out
}

// --- ToUnicode CMap parsing ---

// parseToUnicodeCMap is a hand-rolled scanner for the bfchar/bfrange subset of
// Adobe's CMap syntax used by PDF ToUnicode streams. pdfcpu v0.12.0 does not
// expose a public CMap reader; the internal helper at pkg/pdfcpu/font is
// unexported and only counts used GIDs (not generic code-to-Unicode mapping).
//
// Returns parsed mappings or an error describing where the scan failed. On
// error, the caller surfaces it via ToUnicodeError and renders other sections
// normally (partial-success contract).
func parseToUnicodeCMap(content []byte) ([]ToUnicodeMapping, error) {
	out := []ToUnicodeMapping{}
	s := string(content)

	// bfchar blocks
	bfcharIdx := 0
	for {
		begin := indexFromAt(s, "beginbfchar", bfcharIdx)
		if begin < 0 {
			break
		}
		end := indexFromAt(s, "endbfchar", begin)
		if end < 0 {
			return nil, fmt.Errorf("bfchar: missing endbfchar after offset %d", begin)
		}
		section := s[begin+len("beginbfchar") : end]
		entries, err := parseBfchar(section)
		if err != nil {
			return nil, fmt.Errorf("bfchar: %w", err)
		}
		out = append(out, entries...)
		if len(out) > maxCMapMappings {
			return nil, fmt.Errorf("CMap mappings exceed cap %d", maxCMapMappings)
		}
		bfcharIdx = end + len("endbfchar")
	}

	// bfrange blocks
	bfrangeIdx := 0
	for {
		begin := indexFromAt(s, "beginbfrange", bfrangeIdx)
		if begin < 0 {
			break
		}
		end := indexFromAt(s, "endbfrange", begin)
		if end < 0 {
			return nil, fmt.Errorf("bfrange: missing endbfrange after offset %d", begin)
		}
		section := s[begin+len("beginbfrange") : end]
		entries, err := parseBfrange(section)
		if err != nil {
			return nil, fmt.Errorf("bfrange: %w", err)
		}
		out = append(out, entries...)
		if len(out) > maxCMapMappings {
			return nil, fmt.Errorf("CMap mappings exceed cap %d", maxCMapMappings)
		}
		bfrangeIdx = end + len("endbfrange")
	}

	return out, nil
}

// indexFromAt returns strings.Index(s[from:], substr) + from, or -1.
// Indistinguishable from strings.Index when from==0; centralized so the bfchar
// / bfrange scan loops above stay concise.
func indexFromAt(s, substr string, from int) int {
	if from < 0 || from > len(s) {
		return -1
	}
	i := strings.Index(s[from:], substr)
	if i < 0 {
		return -1
	}
	return i + from
}

// parseBfchar walks `<src> <dst>` pairs inside a beginbfchar/endbfchar block.
// Both tokens are hex strings (e.g. "<0041> <0041>"). dst can be a multi-rune
// UTF-16 sequence that decodes to one (surrogate-paired) or multiple Unicode
// codepoints.
func parseBfchar(section string) ([]ToUnicodeMapping, error) {
	out := []ToUnicodeMapping{}
	tokens := tokenizeCMapSection(section)
	for i := 0; i+1 < len(tokens); i += 2 {
		srcHex := tokens[i]
		dstHex := tokens[i+1]
		// dst can be `<...>` (hex literal) or `[<...> <...>]` (array of hex
		// literals). The array form is rare in bfchar; treat the first inner
		// hex as the mapping value.
		dstHex = unwrapArrayHex(dstHex)
		code, err := parseHexCode(srcHex)
		if err != nil {
			return nil, err
		}
		unicode, glyph := decodeHexUnicode(dstHex)
		out = append(out, ToUnicodeMapping{
			Code:    code,
			Unicode: unicode,
			Glyph:   glyph,
		})
	}
	return out, nil
}

// maxBfrangeSpan caps a single bfrange's (high - low + 1) expansion. PDF
// character codes are at most 4 bytes (0..0xFFFFFFFF) so a malformed CMap
// could request billions of entries; this guard rejects that as a parse error
// so the UI shows a warning instead of allocating until OOM.
const maxBfrangeSpan = 65536

// maxCMapMappings caps the total mappings emitted across all bfchar and
// bfrange blocks in a single CMap. Bounds memory for malicious / malformed
// CMaps that ship millions of bfchar entries (maxBfrangeSpan only caps a
// single range; nothing else caps the count of blocks or bfchar pairs).
const maxCMapMappings = 1 << 17 // 131072

// parseBfrange handles both bfrange forms:
//   <low> <high> <unicode-base>   -- sequential codepoints starting at base
//   <low> <high> [ <u1> <u2> ... ] -- explicit per-code mapping list
func parseBfrange(section string) ([]ToUnicodeMapping, error) {
	out := []ToUnicodeMapping{}
	tokens := tokenizeCMapSection(section)
	for i := 0; i+2 < len(tokens); i += 3 {
		lowHex := tokens[i]
		highHex := tokens[i+1]
		third := tokens[i+2]

		low, err := parseHexCode(lowHex)
		if err != nil {
			return nil, err
		}
		high, err := parseHexCode(highHex)
		if err != nil {
			return nil, err
		}
		if high < low {
			return nil, fmt.Errorf("range high %d < low %d", high, low)
		}
		if high-low+1 > maxBfrangeSpan {
			return nil, fmt.Errorf("range span %d exceeds cap %d", high-low+1, maxBfrangeSpan)
		}

		if strings.HasPrefix(third, "[") {
			// Array form: split into hex literals.
			items := splitArrayHex(third)
			for k := low; k <= high; k++ {
				idx := k - low
				if idx >= len(items) {
					break
				}
				unicode, glyph := decodeHexUnicode(items[idx])
				out = append(out, ToUnicodeMapping{
					Code:    k,
					Unicode: unicode,
					Glyph:   glyph,
				})
			}
			continue
		}

		// Sequential form: third is the base codepoint sequence; each code in
		// [low, high] maps to base + (k - low) at the LAST hex word's offset
		// (PDF spec section 9.10.3). Multi-codepoint base sequences (ligatures
		// in a range, rare in practice) are not incremented across codes; only
		// the trailing codepoint advances.
		base, err := parseHexBytes(third)
		if err != nil {
			return nil, err
		}
		// Split base into UTF-16 code units (2 bytes each). The trailing unit
		// is the one that advances per code; the leading units stay constant.
		if len(base)%2 != 0 {
			return nil, fmt.Errorf("base hex length %d not multiple of 2", len(base))
		}
		units := make([]uint16, len(base)/2)
		for j := range units {
			units[j] = uint16(base[2*j])<<8 | uint16(base[2*j+1])
		}
		if len(units) == 0 {
			continue
		}
		for k := low; k <= high; k++ {
			delta := k - low
			// Propagate the trailing-unit overflow into higher
			// UTF-16 units (carry). PDF spec 9.10.3 increments the trailing
			// 16-bit code unit; when (unit + delta) > 0xFFFF the excess
			// MUST carry into the next-higher unit, not be silently
			// dropped. If propagation overflows the LEADING unit (no higher
			// unit to absorb the carry), stop the loop -- no wraparound, no
			// error -- and return what was emitted.
			advanced := make([]uint16, len(units))
			copy(advanced, units)
			tail := int(advanced[len(advanced)-1]) + delta
			advanced[len(advanced)-1] = uint16(tail & 0xFFFF)
			carry := tail >> 16
			for j := len(advanced) - 2; j >= 0 && carry > 0; j-- {
				sum := int(advanced[j]) + carry
				advanced[j] = uint16(sum & 0xFFFF)
				carry = sum >> 16
			}
			if carry > 0 {
				// Carry propagated past the leading unit -- there is no
				// higher unit to receive it. Stop emitting for this range.
				break
			}
			unicode, glyph := decodeUnits(advanced)
			out = append(out, ToUnicodeMapping{
				Code:    k,
				Unicode: unicode,
				Glyph:   glyph,
			})
		}
	}
	return out, nil
}

// tokenizeCMapSection splits a CMap block body into hex/array tokens. PostScript
// comments (lines starting with %), whitespace, and stray newlines are skipped.
// A token is either `<...>` or `[...]` (balanced).
func tokenizeCMapSection(section string) []string {
	tokens := []string{}
	i := 0
	for i < len(section) {
		c := section[i]
		// Skip PostScript comment to end-of-line.
		if c == '%' {
			for i < len(section) && section[i] != '\n' && section[i] != '\r' {
				i++
			}
			continue
		}
		if c == '<' {
			// Hex literal.
			end := strings.IndexByte(section[i:], '>')
			if end < 0 {
				return tokens
			}
			tokens = append(tokens, section[i:i+end+1])
			i += end + 1
			continue
		}
		if c == '[' {
			// Balanced bracket scan -- nested arrays are not legal in bfchar /
			// bfrange (per spec) but be defensive in case of weird inputs.
			depth := 1
			j := i + 1
			for j < len(section) && depth > 0 {
				switch section[j] {
				case '[':
					depth++
				case ']':
					depth--
				}
				j++
			}
			tokens = append(tokens, section[i:j])
			i = j
			continue
		}
		i++
	}
	return tokens
}

// unwrapArrayHex returns the first inner hex literal from `[<...> <...>]` form,
// or s unchanged when it's already a `<...>` hex literal.
func unwrapArrayHex(s string) string {
	if !strings.HasPrefix(s, "[") {
		return s
	}
	items := splitArrayHex(s)
	if len(items) == 0 {
		return s
	}
	return items[0]
}

// splitArrayHex returns the `<...>` hex literals from a bracket-array form.
func splitArrayHex(s string) []string {
	inner := strings.TrimPrefix(s, "[")
	inner = strings.TrimSuffix(inner, "]")
	out := []string{}
	i := 0
	for i < len(inner) {
		if inner[i] != '<' {
			i++
			continue
		}
		end := strings.IndexByte(inner[i:], '>')
		if end < 0 {
			break
		}
		out = append(out, inner[i:i+end+1])
		i += end + 1
	}
	return out
}

// parseHexCode parses a `<...>` hex literal into its integer code. The hex
// can be any byte length (1..4 bytes in PDF ToUnicode use); we interpret as
// big-endian.
func parseHexCode(hex string) (int, error) {
	bytes, err := parseHexBytes(hex)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, b := range bytes {
		n = (n << 8) | int(b)
	}
	return n, nil
}

// parseHexBytes strips the `<...>` and decodes the hex digits to bytes. Odd
// digit counts get a trailing zero per PDF spec section 7.3.4.3.
func parseHexBytes(hex string) ([]byte, error) {
	if len(hex) < 2 || hex[0] != '<' || hex[len(hex)-1] != '>' {
		return nil, fmt.Errorf("not a hex literal: %q", hex)
	}
	inner := hex[1 : len(hex)-1]
	// Drop whitespace inside the hex literal.
	clean := make([]byte, 0, len(inner))
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			continue
		}
		clean = append(clean, c)
	}
	if len(clean)%2 == 1 {
		clean = append(clean, '0')
	}
	bytes := make([]byte, len(clean)/2)
	for i := 0; i < len(bytes); i++ {
		hi, ok1 := hexNibble(clean[2*i])
		lo, ok2 := hexNibble(clean[2*i+1])
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("invalid hex char in %q", inner)
		}
		bytes[i] = (hi << 4) | lo
	}
	return bytes, nil
}

// hexNibble decodes one hex digit to its 0-15 value.
func hexNibble(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// decodeHexUnicode turns a `<...>` UTF-16BE hex literal into (display, glyph).
// display is the space-separated U+XXXX form (one per codepoint after surrogate
// pairing). glyph is the literal string suitable for the visual glyph cell,
// with C0/C1/PUA/unpaired-surrogate codepoints blanked.
func decodeHexUnicode(hex string) (string, string) {
	bytes, err := parseHexBytes(hex)
	if err != nil || len(bytes) == 0 {
		return "", ""
	}
	// Odd-byte hex is a single-byte source/dest written as <NN> rather than
	// the spec's <00NN>. PDF 7.3.4.3 right-pads odd-nibble hex with a low-zero
	// nibble, but for UTF-16BE ToUnicode values that produces wrong codepoints
	// (e.g. <41> -> U+4100 instead of U+0041). Left-pad instead so the byte
	// occupies the low half of the resulting code unit.
	if len(bytes)%2 == 1 {
		padded := make([]byte, len(bytes)+1)
		copy(padded[1:], bytes)
		bytes = padded
	}
	units := make([]uint16, len(bytes)/2)
	for i := range units {
		units[i] = uint16(bytes[2*i])<<8 | uint16(bytes[2*i+1])
	}
	return decodeUnits(units)
}

// decodeUnits decodes a UTF-16BE code-unit sequence into (display, glyph).
// Surrogate pairs collapse into a single non-BMP codepoint via utf16.Decode.
// Unpaired surrogates remain as U+D8XX entries with a blank glyph cell.
func decodeUnits(units []uint16) (string, string) {
	if len(units) == 0 {
		return "", ""
	}

	// Manually walk the unit slice so unpaired surrogates remain visible in
	// the U+ display (utf16.Decode would convert them to U+FFFD silently).
	displays := []string{}
	glyphRunes := []rune{}
	i := 0
	for i < len(units) {
		u := units[i]
		if u >= 0xD800 && u <= 0xDBFF {
			// High surrogate: needs a matching low surrogate.
			if i+1 < len(units) {
				lo := units[i+1]
				if lo >= 0xDC00 && lo <= 0xDFFF {
					r := utf16.Decode([]uint16{u, lo})
					if len(r) == 1 {
						cp := r[0]
						displays = append(displays, formatCodepoint(int(cp)))
						glyphRunes = append(glyphRunes, glyphRuneForCodepoint(cp))
						i += 2
						continue
					}
				}
			}
			// Unpaired high surrogate.
			displays = append(displays, formatCodepoint(int(u)))
			glyphRunes = append(glyphRunes, 0) // blanked
			i++
			continue
		}
		if u >= 0xDC00 && u <= 0xDFFF {
			// Unpaired low surrogate.
			displays = append(displays, formatCodepoint(int(u)))
			glyphRunes = append(glyphRunes, 0)
			i++
			continue
		}
		displays = append(displays, formatCodepoint(int(u)))
		glyphRunes = append(glyphRunes, glyphRuneForCodepoint(rune(u)))
		i++
	}

	display := strings.Join(displays, " ")
	glyph := buildGlyphString(glyphRunes)
	return display, glyph
}

// formatCodepoint renders an integer codepoint as "U+XXXX" (minimum 4 hex
// digits; non-BMP codepoints render with their natural width since %04X is a
// minimum-width directive).
func formatCodepoint(cp int) string {
	return fmt.Sprintf("U+%04X", cp)
}

// glyphRuneForCodepoint returns the rune to render in the literal-glyph cell,
// or 0 for codepoints that must be blanked (C0/C1 control, surrogate halves,
// PUA). Returning 0 lets the caller drop the rune from the joined glyph string.
func glyphRuneForCodepoint(cp rune) rune {
	if cp < 0x20 {
		return 0 // C0 control
	}
	if cp >= 0x7F && cp <= 0xA0 {
		return 0 // DEL + C1 control + NBSP boundary
	}
	if cp >= 0xD800 && cp <= 0xDFFF {
		return 0 // surrogate
	}
	if cp >= 0xE000 && cp <= 0xF8FF {
		return 0 // BMP PUA
	}
	if cp >= 0xF0000 && cp <= 0xFFFFD {
		return 0 // Supplementary PUA-A
	}
	if cp >= 0x100000 && cp <= 0x10FFFD {
		return 0 // Supplementary PUA-B
	}
	if !utf8.ValidRune(cp) {
		return 0
	}
	return cp
}

// buildGlyphString concatenates the non-zero runes from runes into a string.
// All-zero input returns the empty string -- the frontend can choose to render
// the U+25CC dotted circle placeholder or leave the cell blank.
func buildGlyphString(runes []rune) string {
	out := []rune{}
	for _, r := range runes {
		if r == 0 {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return ""
	}
	return string(out)
}
