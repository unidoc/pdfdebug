package pdfcore

// TreeNode represents a single node in the PDF object tree shown in the UI.
type TreeNode struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	RawKey      string `json:"rawKey"`
	NodeType    string `json:"nodeType"`
	ValueType   string `json:"valueType"`
	HasChildren bool   `json:"hasChildren"`
	ChildCount  int    `json:"childCount"`
	IconHint    string `json:"iconHint"`
	Error       string `json:"error"`
	// ObjectRef is "<num> <gen> R" when the node corresponds to an indirect
	// object, "" otherwise. Powers the inline [N G R] suffix on tree rows
	// (Story 9-8 AC1).
	ObjectRef string `json:"objectRef"`
	// TypeName is the literal /Type value of the resolved dict (e.g. "Pages",
	// "Page", "Font"), "" when the dict has no /Type entry or the node is not
	// a dict. The frontend dedups this against the semantic label (AC2).
	TypeName string `json:"typeName"`
}

// ObjectIndexEntry is one row in the per-document object index produced by
// Inspector.GetObjectIndex. It backs the Cmd+K command palette (Story 9-8).
type ObjectIndexEntry struct {
	ObjNum    int    `json:"objNum"`
	Gen       int    `json:"gen"`
	TypeName  string `json:"typeName"`  // "" if no /Type key
	Free      bool   `json:"free"`      // true if xref entry marked free
	Reachable bool   `json:"reachable"` // true if reachable from catalog root
	NodeID    string `json:"nodeId"`    // "obj:<gen>:<num>" for reachable; "" for free/orphan
}

// ObjectDetail holds the full inspection data for a single PDF object.
type ObjectDetail struct {
	NodeID      string          `json:"nodeId"`
	ObjectRef   string          `json:"objectRef"`
	Type        string          `json:"type"`
	Properties  []PropertyEntry `json:"properties"`
	Elements    []ValueEntry    `json:"elements"`
	ScalarValue *ValueEntry     `json:"scalarValue"`
	StreamInfo  *StreamInfo     `json:"streamInfo"`
}

// PropertyEntry is a key-value pair from a PDF dictionary.
type PropertyEntry struct {
	Key   string     `json:"key"`
	Value ValueEntry `json:"value"`
}

// ValueEntry describes a single PDF value with its type and display form.
type ValueEntry struct {
	Type      string `json:"type"`
	Display   string `json:"display"`
	Raw       string `json:"raw"`
	RefTarget string `json:"refTarget"`
}

// ContentStreamData holds raw and tokenized content stream data for a page.
type ContentStreamData struct {
	NodeID    string          `json:"nodeId"`
	Raw       string          `json:"raw"`
	Tokenized []Token         `json:"tokenized"`
	Formatted []FormattedLine `json:"formatted"`
	Error     string          `json:"error"`
}

// FormattedLine is one logical PDF operation in a content stream: zero or more
// operand tokens followed by their operator, plus the indent depth and the
// source-byte-line range the operation came from. Story 9-6 introduced this
// shape so the frontend can render formatted view as a flat row sequence
// without re-deriving operator boundaries or indent client-side.
type FormattedLine struct {
	Tokens       []Token `json:"tokens"`
	Indent       int     `json:"indent"`
	Operator     string  `json:"operator"`
	SrcLineStart int     `json:"srcLineStart"`
	SrcLineEnd   int     `json:"srcLineEnd"`
}

// ImageData holds extracted image data and metadata for frontend display.
type ImageData struct {
	NodeID           string `json:"nodeId"`
	ObjectRef        string `json:"objectRef"`
	MimeType         string `json:"mimeType"`
	Base64           string `json:"base64"`
	Width            int    `json:"width"`
	Height           int    `json:"height"`
	ColorSpace       string `json:"colorSpace"`
	BitsPerComponent int    `json:"bitsPerComponent"`
	Filter           string `json:"filter"`
	Warning          string `json:"warning"`
	Error            string `json:"error"`
}

// StreamInfo describes the length and filter pipeline of a PDF stream.
type StreamInfo struct {
	Length  int64    `json:"length"`
	Filters []string `json:"filters"`
}

// Token is a single lexical token from a content stream.
type Token struct {
	Type  string `json:"type"`
	Value string `json:"value"`
	Line  int    `json:"line"`
	Col   int    `json:"col"`
}

// ReverseRef describes one inbound dict-graph edge pointing at an object:
// which indirect object contains the reference, where inside it the reference
// lives (dict-key/array-index path), and the parent's /Type if present.
// Used to power the right-panel "Referenced by" section (Story 9-10).
//
// The ParentType *string pointer (NOT string) is load-bearing: nil means
// "key absent" so the frontend can omit the column, while a non-nil pointer
// to "" means "key present with empty value" -- a distinction a non-pointer
// type would erase.
type ReverseRef struct {
	ParentNodeID string  `json:"parentNodeId"`         // obj:<gen>:<num> for NAVIGATE_TO_REF
	ParentRef    string  `json:"parentRef"`            // "<num> <gen> R"
	ParentType   *string `json:"parentType,omitempty"` // /Type value when present; nil when key absent
	Path         string  `json:"path"`                 // e.g. "/Kids[3]" or "/Resources /Font /F1"
	ParentPath   string  `json:"parentPath"`           // root-relative path to ParentRef (BFS discovery), "" for the catalog
}

// FontDetail is the consolidated font inspection payload returned by
// Inspector.GetFontDetail. Mirrors the metadata + encoding + ToUnicode +
// FontDescriptor structure that PDF debuggers (iText RUPS, PDFBox) surface
// for /Type /Font dicts. Story 9-9.
//
// Descendant is non-nil only for composite (/Subtype /Type0) fonts and
// recursively carries the same shape for the descendant CIDFont. ToUnicodeError
// is the partial-success signal: when the CMap stream exists but the bfchar /
// bfrange scanner returns an error, ToUnicodeMappings is empty and the field
// holds the parse error so the frontend can show a warning instead of blanking
// the panel (AC9a).
type FontDetail struct {
	NodeID            string               `json:"nodeId"`
	ObjectRef         string               `json:"objectRef"`
	Subtype           string               `json:"subtype"`
	BaseFont          string               `json:"baseFont"`
	FirstChar         int                  `json:"firstChar"`
	LastChar          int                  `json:"lastChar"`
	EncodingName      string               `json:"encodingName"`
	BaseEncoding      string               `json:"baseEncoding"`
	Differences       []EncodingDifference `json:"differences"`
	ToUnicodeMappings []ToUnicodeMapping   `json:"toUnicodeMappings"`
	ToUnicodeError    string               `json:"toUnicodeError"`
	Embedded          bool                 `json:"embedded"`
	FontDescriptor    *FontDescriptorInfo  `json:"fontDescriptor"`
	Descendant        *FontDetail          `json:"descendant"`
	// CIDSystemInfo / CIDToGIDMap / DefaultWidth populate only for CIDFont
	// descendants (Subtype CIDFontType0 or CIDFontType2) per AC7. Zero values
	// on non-CID fonts are inert; the frontend renders rows conditionally.
	CIDSystemInfo *CIDSystemInfo `json:"cidSystemInfo"`
	CIDToGIDMap   string         `json:"cidToGIDMap"`
	DefaultWidth  int            `json:"defaultWidth"`
}

// CIDSystemInfo carries the /Registry /Ordering /Supplement triplet from a
// CIDFont's /CIDSystemInfo dict. AC7 requires these surfaced in the
// "Descendant Font" section so users can identify the character collection.
type CIDSystemInfo struct {
	Registry   string `json:"registry"`
	Ordering   string `json:"ordering"`
	Supplement int    `json:"supplement"`
}

// EncodingDifference is one entry in an /Encoding /Differences table: a
// character code mapped to a glyph name (e.g. 32 -> "/space").
type EncodingDifference struct {
	Code      int    `json:"code"`
	GlyphName string `json:"glyphName"`
}

// ToUnicodeMapping is one row in a font's /ToUnicode CMap: character code
// mapped to its Unicode form (U+XXXX, possibly multi-codepoint for ligatures)
// plus the literal glyph string suitable for display. Glyph is blanked for
// codepoints in C0/C1 control, surrogate, or PUA ranges per AC5.
type ToUnicodeMapping struct {
	Code    int    `json:"code"`
	Unicode string `json:"unicode"`
	Glyph   string `json:"glyph"`
}

// FontDescriptorInfo summarizes a /FontDescriptor dict: name, decoded /Flags
// bits, the standard metric fields, and which of /FontFile, /FontFile2, or
// /FontFile3 carries the embedded font program (with a Subtype-derived
// FontFileFormat string for /FontFile3 dispatch).
type FontDescriptorInfo struct {
	NodeID         string    `json:"nodeId"`
	ObjectRef      string    `json:"objectRef"`
	FontName       string    `json:"fontName"`
	Flags          int       `json:"flags"`
	FlagNames      []string  `json:"flagNames"`
	ItalicAngle    float64   `json:"italicAngle"`
	Ascent         float64   `json:"ascent"`
	Descent        float64   `json:"descent"`
	CapHeight      float64   `json:"capHeight"`
	StemV          float64   `json:"stemV"`
	FontBBox       []float64 `json:"fontBBox"`
	FontFileFormat string    `json:"fontFileFormat"`
	FontFileSize   int       `json:"fontFileSize"`
}

// FontResourceMap is the payload returned by Inspector.GetFontResourceMap
// when the iconHint='font' heuristic resolves to a /Resources /Font dict
// rather than a /Type /Font dict. Each entry summarizes one font referenced
// by the resource map; entries are sorted alphabetically by Name so the
// frontend table renders in stable order.
type FontResourceMap struct {
	NodeID  string            `json:"nodeId"`
	Entries []FontRosterEntry `json:"entries"`
}

// FontView is the unified payload returned by Inspector.GetFontView. It
// disambiguates server-side whether a node resolves to a /Type /Font dict
// (Kind=="detail"), a /Resources /Font roster (Kind=="roster"), or neither
// (Kind=="neither"), so the frontend issues exactly one call per click and
// "this isn't a font" never produces an error log on the Wails binding layer.
type FontView struct {
	Kind   string           `json:"kind"`
	Detail *FontDetail      `json:"detail"`
	Roster *FontResourceMap `json:"roster"`
}

// FontRosterEntry is one row in a /Resources /Font roster: the resource-map
// key (e.g. "F1"), the resolved /Type /Font dict's node ID, and a thin
// summary suitable for the FontRosterPreview table. Unresolved is set when
// the resolved value is not a Font dict; the entry still renders (Name +
// red pill) so users can see the resource-map shape, but it isn't clickable.
type FontRosterEntry struct {
	Name            string `json:"name"`
	NodeID          string `json:"nodeId"`
	ObjectRef       string `json:"objectRef"`
	BaseFont        string `json:"baseFont"`
	Subtype         string `json:"subtype"`
	EncodingSummary string `json:"encodingSummary"`
	Embedded        bool   `json:"embedded"`
	Unresolved      bool   `json:"unresolved"`
}

// XRefTable is the document-level cross-reference table payload returned by
// Inspector.GetXRefTable. Story 9-11.
type XRefTable struct {
	TabID   string      `json:"tabId"`
	Entries []XRefEntry `json:"entries"` // sorted by ObjNum asc, then Gen asc
}

// XRefEntry is one row in the XRefTable view: object number, generation,
// status, and the on-disk byte offset (for in-use entries) or the host
// object stream number (for compressed entries). The frontend renders free
// entries as non-clickable. Status is the load-bearing IPC contract: "in-use"
// / "free" / "in-objstm" -- frontend pills key off these literals. Story 9-11.
type XRefEntry struct {
	ObjNum     int    `json:"objNum"`
	Gen        int    `json:"gen"`
	Status     string `json:"status"`     // "in-use" | "free" | "in-objstm"
	Offset     int64  `json:"offset"`     // -1 when Status != "in-use" (caller renders "-")
	HostObjStm int    `json:"hostObjStm"` // host /ObjStm object number when Status == "in-objstm"; 0 otherwise (caller renders "-")
	NodeID     string `json:"nodeID"`     // "obj:<gen>:<num>" for in-use and in-objstm; "" for free
}

// PlainTextDocument is the document-level Latin-1-decoded bytes payload
// returned by Inspector.GetPlainText. Latin-1 is a deliberate choice over
// UTF-8 because UTF-8 decode would inject replacement characters for valid
// PDF byte sequences inside stream contents; Latin-1 is lossless byte-for-byte
// (every byte maps to a Unicode codepoint U+0000-U+00FF). Story 9-11.
type PlainTextDocument struct {
	TabID      string `json:"tabId"`
	Content    string `json:"content"`    // Latin-1-decoded bytes; may be truncated
	TotalBytes int64  `json:"totalBytes"` // file size on disk in bytes
	Truncated  bool   `json:"truncated"`  // true when Content is only the first CapBytes bytes
	CapBytes   int64  `json:"capBytes"`   // the cap that was applied; echoed so the frontend can format the banner
}

// DocumentInfo summarizes an opened PDF document for the frontend.
type DocumentInfo struct {
	TabID     string `json:"tabId"`
	FileName  string `json:"fileName"`
	FilePath  string `json:"filePath"`
	PageCount int    `json:"pageCount"`
	FileSize  int64  `json:"fileSize"`
	Error     string `json:"error"`
}
