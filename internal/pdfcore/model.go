package pdfcore

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

type ObjectDetail struct {
	NodeID      string          `json:"nodeId"`
	ObjectRef   string          `json:"objectRef"`
	Type        string          `json:"type"`
	Properties  []PropertyEntry `json:"properties"`
	Elements    []ValueEntry    `json:"elements"`
	ScalarValue *ValueEntry     `json:"scalarValue"`
	StreamInfo  *StreamInfo     `json:"streamInfo"`
}

type PropertyEntry struct {
	Key   string     `json:"key"`
	Value ValueEntry `json:"value"`
}

type ValueEntry struct {
	Type      string `json:"type"`
	Display   string `json:"display"`
	Raw       string `json:"raw"`
	RefTarget string `json:"refTarget"`
}

type ContentStreamData struct {
	NodeID    string  `json:"nodeId"`
	Raw       string  `json:"raw"`
	Tokenized []Token `json:"tokenized"`
	Error     string  `json:"error"`
}

type StreamInfo struct {
	Length  int64    `json:"length"`
	Filters []string `json:"filters"`
}

type Token struct {
	Type  string `json:"type"`
	Value string `json:"value"`
	Line  int    `json:"line"`
	Col   int    `json:"col"`
}

type DocumentInfo struct {
	TabID     string `json:"tabId"`
	FileName  string `json:"fileName"`
	FilePath  string `json:"filePath"`
	PageCount int    `json:"pageCount"`
	FileSize  int64  `json:"fileSize"`
	Error     string `json:"error"`
}
