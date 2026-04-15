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
	NodeID    string  `json:"nodeId"`
	Raw       string  `json:"raw"`
	Tokenized []Token `json:"tokenized"`
	Error     string  `json:"error"`
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

// DocumentInfo summarizes an opened PDF document for the frontend.
type DocumentInfo struct {
	TabID     string `json:"tabId"`
	FileName  string `json:"fileName"`
	FilePath  string `json:"filePath"`
	PageCount int    `json:"pageCount"`
	FileSize  int64  `json:"fileSize"`
	Error     string `json:"error"`
}
