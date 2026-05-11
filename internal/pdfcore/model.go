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
