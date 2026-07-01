package pdfcore

import pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

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

// ResolveOpts configures Inspector.ResolveRef. MaxDepth is the number of
// indirect-reference levels to follow inline below the addressed object:
//   - 0 resolves the addressed object only; indirect refs found inside it stay
//     as unfollowed child markers (Truncated=true, not recursed into).
//   - N follows up to N levels of indirect refs.
//
// A negative MaxDepth is clamped to 0 (resolve the addressed object only). A
// separate internal ceiling (maxResolveDepth) caps the effective depth so a
// caller passing a huge N cannot exhaust the stack.
type ResolveOpts struct {
	MaxDepth int
}

// ResolvedNode is the dedicated result tree of Inspector.ResolveRef. It is a
// deliberate alternative to TreeNode/ObjectDetail (which are display-oriented:
// string Display fields, IObject placeholders) because consumers like Story
// 11-6 need to read the raw pdfcpu value to classify /Subtype, read /MediaBox
// arrays, and walk /Resources sub-dicts. Value carries that raw object for Go
// callers; it is excluded from JSON (the GUI/CLI read the typed fields).
//
// The JSON shape (objectRef, cyclic, truncated, children, ...) is a stable
// contract for 11-6 and the GUI, pinned by a test.
type ResolvedNode struct {
	// Value is the raw resolved pdfcpu object (a Dict, StreamDict, Array, scalar,
	// or - for an unfollowed ref marker - the IndirectRef itself). Excluded from
	// JSON; Go callers (Story 11-6) read it to classify type and named entries.
	Value pdfcpu_types.Object `json:"-"`
	// Key is the dict key ("Subtype") or array index ("[0]") this node occupies
	// in its parent; "" for the root.
	Key string `json:"key,omitempty"`
	// ObjectRef is "<num> <gen> R" when this node came from (or marks) an
	// indirect object, "" for a direct value.
	ObjectRef string `json:"objectRef"`
	// NodeType classifies Value: "dict" | "stream" | "array" | "ref" (unfollowed
	// marker) | "scalar".
	NodeType string `json:"nodeType"`
	// Children are the resolved entries of a dict, stream-dict, or array, in
	// deterministic (sorted-key / array-index) order. Nil for scalars and for
	// unfollowed ref markers.
	Children []*ResolvedNode `json:"children,omitempty"`
	// Truncated is true when this node is an indirect ref left UNFOLLOWED because
	// the MaxDepth/internal ceiling was hit (the depth-cap marker, AC3).
	Truncated bool `json:"truncated"`
	// Cyclic is true when this node is an indirect ref that re-enters an object
	// already on the current resolution path (the cycle back-edge marker, AC3).
	Cyclic bool `json:"cyclic"`
}

// XObjectInfo describes one entry in a /Resources/XObject sub-dictionary: the
// resource name (e.g. "Fm0"), the resolved indirect-object reference, the node
// ID of the resolved XObject stream, and its /Subtype ("Image", "Form", or the
// raw value when neither). Backs the --ops Do classification and the
// --xobject NAME stream resolution (Story 11-5 items 2/3).
type XObjectInfo struct {
	Name      string `json:"name"`
	ObjectRef string `json:"objectRef"` // "<num> <gen> R"
	NodeID    string `json:"nodeId"`    // "obj:<gen>:<num>"
	Subtype   string `json:"subtype"`   // "Image" | "Form" | raw /Subtype | ""
}

// PageRenderOpts configures Inspector.PageRenderInfo. It controls the optional
// recursive Form-XObject walk (off by default - a complex catalog page can have
// deep form nesting, so recursion is opt-in and depth-bounded).
//
// FormsRecursive enables the walk; FormsDepth caps how many form-nesting LEVELS
// are followed. FormsDepth is a THIRD recursion axis, distinct from the
// keystone's internal ref-ceiling (maxResolveDepth): a form tree deeper than the
// ref-ceiling still truncates per-ref via ResolveRef.
type PageRenderOpts struct {
	FormsRecursive bool
	FormsDepth     int
}

// PageRenderInfo is the assembled per-page rendering picture (Story 11-6): page
// geometry (resolved incl. /Pages inheritance), every ExtGState's
// blend/alpha/SMask, every XObject classified (Form vs Image with colorspace
// family), and - when requested - each Form XObject walked recursively against
// ITS OWN /Resources. The recursive walk enumerates the Form XObjects declared
// in each form's /Resources/XObject (a superset of the forms its content
// actually Does); it does not parse the content stream.
//
// EXPERIMENTAL CONTRACT (caution 1): the field set is derived from a single
// transcript. It is NOT a frozen contract. Story 11-5's low-level flags
// (dump stream --xobject/--ref/--ops, dump tree/dump object --resolve) remain
// the escape hatch for anything this omits. STRUCTURAL ONLY (caution 2): every
// field is file-resident structure (names, refs, function types, profile
// sizes); NO rendering computation (color conversion, tint-transform
// evaluation, SMask compositing) is performed - that is the renderer's job.
//
// Lives in pdfcore so the GUI can later render the same struct as a panel.
type PageRenderInfo struct {
	Page       int                 `json:"page"`
	PageRef    string              `json:"pageRef"`         // "<num> <gen> R"
	MediaBox   []float64           `json:"mediaBox"`        // [llx lly urx ury], inherited from /Pages when absent on the page
	CropBox    []float64           `json:"cropBox"`         // [llx lly urx ury], inherited; nil when absent
	Rotate     int                 `json:"rotate"`          // inherited; 0 when absent
	ExtGStates []ExtGStateInfo     `json:"extGStates"`      // sorted by Name
	XObjects   []XObjectRenderInfo `json:"xobjects"`        // sorted by Name
	Patterns   []PatternInfo       `json:"patterns"`        // structural only: name+ref+patternType
	Shadings   []ShadingInfo       `json:"shadings"`        // structural only: name+ref+shadingType
	Forms      []FormRenderInfo    `json:"forms,omitempty"` // populated only with FormsRecursive
}

// ExtGStateInfo summarizes one /Resources/ExtGState entry: its resource name,
// resolved ref, and the structural transparency parameters BM (blend-mode
// name), ca (non-stroking alpha), CA (stroking alpha), and SMask. No blend MATH
// is performed (AC2/AC7) - values are read verbatim from the file.
type ExtGStateInfo struct {
	Name string `json:"name"`
	Ref  string `json:"ref"` // "<num> <gen> R", "" for a direct (inline) ExtGState
	// BM is the blend-mode name ("Normal", "Multiply", ...). A /BM array (the
	// rarely-used multi-mode form) renders as its first name.
	BM string `json:"BM,omitempty"`
	// CA is the non-stroking alpha (/ca); Ca is the stroking alpha (/CA). Pointers
	// so an absent entry is distinguishable from an explicit 0.0 (fully
	// transparent) - load-bearing for a transparency investigation.
	CA *float64 `json:"ca,omitempty"`
	Ca *float64 `json:"CA,omitempty"`
	// SMask is the resolved soft-mask: the literal string "None" (the /None value),
	// nil (no /SMask key), or a *SMaskDescriptor object (the resolved soft-mask
	// dict, exposing /S, /G, etc.) when a soft-mask dict is present. Typed as any
	// so the resolved descriptor serializes inline as the SMask value (AC2),
	// rather than a placeholder string with the descriptor on a side field.
	SMask any `json:"SMask,omitempty"`
}

// SMaskDescriptor is the resolved soft-mask dict of an ExtGState /SMask (AC2):
// the masking subtype /S (Alpha or Luminosity), the masked group /G ref, and
// the backdrop /BC array length. STRUCTURAL ONLY: the mask is NOT composited.
type SMaskDescriptor struct {
	S      string `json:"S,omitempty"`      // "Alpha" | "Luminosity"
	GRef   string `json:"gRef,omitempty"`   // ref to the transparency group XObject /G
	HasTR  bool   `json:"hasTR,omitempty"`  // a /TR transfer function is present (not evaluated)
	BCSize int    `json:"bcSize,omitempty"` // /BC backdrop color component count
}

// XObjectRenderInfo classifies one /Resources/XObject entry (AC3). Form
// XObjects carry BBox/Matrix/Group; Image XObjects carry Width/Height/
// ColorSpace. Colorspace is CLASSIFIED, not evaluated (AC7).
type XObjectRenderInfo struct {
	Name    string `json:"name"`
	Ref     string `json:"ref"`     // "<num> <gen> R"
	Subtype string `json:"subtype"` // "Form" | "Image" | raw /Subtype
	// Form fields.
	BBox   []float64  `json:"bbox,omitempty"`
	Matrix []float64  `json:"matrix,omitempty"`
	Group  *GroupInfo `json:"group,omitempty"`
	// Image fields.
	Width      int             `json:"width,omitempty"`
	Height     int             `json:"height,omitempty"`
	ColorSpace *ColorSpaceInfo `json:"colorSpace,omitempty"`
}

// GroupInfo is a Form XObject's /Group (transparency group) attributes (AC3):
// /S (group subtype, "Transparency"), /CS (group colorspace family), /I
// (isolated), /K (knockout). STRUCTURAL ONLY.
type GroupInfo struct {
	S  string `json:"S,omitempty"`
	CS string `json:"CS,omitempty"` // group colorspace family (classified)
	// I (isolated) and K (knockout) default to false when /I or /K is absent (PDF
	// spec). They are NOT omitempty: a resolved group must report both flags so a
	// consumer can distinguish "knockout false" from "field missing" (AC3).
	I bool `json:"I"`
	K bool `json:"K"`
}

// ColorSpaceInfo is the classified (NOT evaluated) colorspace of an Image
// XObject or a /Group /CS (AC3/AC7). Family is the colorspace family name
// (DeviceRGB, ICCBased, Separation, ...). For ICCBased: N (component count) and
// ICCProfileSize (the profile stream's byte length) are surfaced so the user
// runs any color math themselves. For Separation/DeviceN: TintTransformType is
// the tint-transform FUNCTION TYPE (structure, not evaluation) and AltFamily is
// the alternate colorspace family. For Indexed: HiVal is the palette's hival.
type ColorSpaceInfo struct {
	Family            string `json:"family"`
	N                 int    `json:"n,omitempty"`                 // ICCBased component count
	ICCProfileSize    int    `json:"iccProfileSize,omitempty"`    // ICCBased profile stream byte length
	AltFamily         string `json:"altFamily,omitempty"`         // Separation/DeviceN/ICCBased alternate family
	TintTransformType int    `json:"tintTransformType,omitempty"` // Separation/DeviceN tint-transform /FunctionType
	HiVal             int    `json:"hiVal,omitempty"`             // Indexed palette hival
}

// PatternInfo is a STRUCTURAL-ONLY /Resources/Pattern entry (AC1/AC7): resource
// name, resolved ref, and the /PatternType integer (1 = tiling, 2 = shading).
// No tiling-content walk, no shading-function evaluation. Appears only in the
// full object - there is no --section patterns.
type PatternInfo struct {
	Name        string `json:"name"`
	Ref         string `json:"ref"`
	PatternType int    `json:"patternType,omitempty"`
}

// ShadingInfo is a STRUCTURAL-ONLY /Resources/Shading entry (AC1/AC7): resource
// name, resolved ref, and the /ShadingType integer. No shading-function
// evaluation. Appears only in the full object - there is no --section shadings.
type ShadingInfo struct {
	Name        string `json:"name"`
	Ref         string `json:"ref"`
	ShadingType int    `json:"shadingType,omitempty"`
}

// FormRenderInfo is one node in the recursive Form-XObject walk (AC4): the
// resource name + ref the form was reached through, the form's OWN classified
// /Resources (the "does the inner Fm0 live in the page's or the outer form's
// resources" gotcha - it lives in the form's own resources), and the Form
// XObjects declared in that /Resources/XObject (recursing). The walk reads the
// resource dict, not the content stream.
//
// Truncated marks a form left UNWALKED because FormsDepth was reached. Cyclic
// marks a form that re-enters a form already on the current walk path (the
// self-referential-form guard, AC4) - the walk terminates rather than looping.
type FormRenderInfo struct {
	Name       string              `json:"name"`
	Ref        string              `json:"ref"`
	ExtGStates []ExtGStateInfo     `json:"extGStates"`
	XObjects   []XObjectRenderInfo `json:"xobjects"`
	Patterns   []PatternInfo       `json:"patterns"`
	Shadings   []ShadingInfo       `json:"shadings"`
	Forms      []FormRenderInfo    `json:"forms,omitempty"`
	Truncated  bool                `json:"truncated,omitempty"`
	Cyclic     bool                `json:"cyclic,omitempty"`
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
	// MappingRows is the assembled per-code mapping table (Story 13.3 AC1): the
	// JOIN of Differences (glyph name) and ToUnicodeMappings (unicode + literal
	// text) over the union of declared codes. Assembled, never re-parsed.
	MappingRows []FontMappingRow `json:"mappingRows"`
	// Health carries the coverage/health diagnostic signals (Story 13.3 AC2).
	// Always populated, even on a malformed ToUnicode (the signals reflect
	// whatever parsed).
	Health *FontHealth `json:"health"`
}

// FontMappingRow is one assembled row in the per-code font mapping table
// (Story 13.3 AC1). It is the JOIN of an /Encoding /Differences entry (GlyphName)
// and a /ToUnicode CMap entry (Unicode, UnicodeText) keyed by character code.
// Either side may be empty when a code is declared in only one of the two
// sources. There is no single existing type spanning these fields, so this row
// type is defined explicitly.
type FontMappingRow struct {
	Code        int    `json:"code"`        // character code
	CodeHex     string `json:"codeHex"`     // "0x41" form
	GlyphName   string `json:"glyphName"`   // from /Differences, "" if none
	Unicode     string `json:"unicode"`     // "U+XXXX" from ToUnicode, "" if none
	UnicodeText string `json:"unicodeText"` // literal glyph string, "" if none
}

// FontHealth carries the coverage/health diagnostic signals for a font
// (Story 13.3 AC2). These surface the classic text-extraction failure modes
// explicitly rather than leaving the user to infer them.
type FontHealth struct {
	// DeclaredCodeCount is the count of distinct declared codes (the union of
	// Differences and ToUnicode codes) -- the "complete" denominator.
	DeclaredCodeCount int `json:"declaredCodeCount"`
	// ToUnicodeMissing is true when the font has no usable /ToUnicode CMap:
	// absent entirely, or present but unparseable (ToUnicodeError set). In both
	// cases extraction has no code-to-Unicode coverage.
	ToUnicodeMissing bool `json:"toUnicodeMissing"`
	// IdentityWithoutToUnicode flags an Identity (Identity-H/V) encoding with no
	// usable ToUnicode -- the classic "copy yields gibberish" case.
	IdentityWithoutToUnicode bool `json:"identityWithoutToUnicode"`
	// EncodingWithoutToUnicodeCodes lists codes present in /Differences but
	// absent from /ToUnicode -- extraction will fail for each.
	EncodingWithoutToUnicodeCodes []int `json:"encodingWithoutToUnicodeCodes"`
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
// (every byte maps to a Unicode codepoint U+0000-U+00FF). Story 9-11; the
// Truncated and CapBytes fields were removed in Story 10-1 alongside the
// single uncapped lazy-load contract.
type PlainTextDocument struct {
	TabID      string `json:"tabId"`
	Content    string `json:"content"`    // Latin-1-decoded full-file bytes
	TotalBytes int64  `json:"totalBytes"` // file size on disk in bytes
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
