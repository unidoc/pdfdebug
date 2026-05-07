package pdfcore

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// 3.1-UNIT-001 [P0]: GetContentStream returns decoded plain text for a page's
// Contents node using testdata/content-stream.pdf.
// AC#1: Given a tree node ID corresponding to a page's Contents entry,
//       When GetContentStream(tabID, nodeID) is called,
//       Then it returns ContentStreamData with decoded plain text in Raw,
//       And NodeID is populated correctly.
// ---------------------------------------------------------------------------

func TestGetContentStreamValid(t *testing.T) {
	ins, tabID := openContentStream(t)

	streamNodeID := findStreamNode(t, ins, tabID, "root", 0)
	if streamNodeID == "" {
		t.Fatal("no stream node found in content-stream.pdf")
	}

	result, err := ins.GetContentStream(tabID, streamNodeID)
	if err != nil {
		t.Fatalf("GetContentStream returned error: %v", err)
	}
	if result == nil {
		t.Fatal("GetContentStream returned nil")
	}
	if result.Error != "" {
		t.Fatalf("ContentStreamData.Error = %q, want empty", result.Error)
	}
	if result.NodeID != streamNodeID {
		t.Errorf("NodeID = %q, want %q", result.NodeID, streamNodeID)
	}
	if result.Raw == "" {
		t.Fatal("Raw is empty, want decoded content stream text")
	}

	// content-stream.pdf contains: BT /F1 12 Tf 100 700 Td (Hello World) Tj ET
	if !strings.Contains(result.Raw, "BT") {
		t.Errorf("Raw does not contain 'BT': %q", result.Raw)
	}
	if !strings.Contains(result.Raw, "Tj") {
		t.Errorf("Raw does not contain 'Tj': %q", result.Raw)
	}
	if !strings.Contains(result.Raw, "ET") {
		t.Errorf("Raw does not contain 'ET': %q", result.Raw)
	}

	// 3.3-UNIT-007 [P1] / Task 2.3: Verify tokenizer integration in GetContentStream.
	// Tokenized field must be non-nil, non-empty, and contain at least BT and ET operators.
	if result.Tokenized == nil {
		t.Fatal("Tokenized is nil, want non-nil after tokenizer integration")
	}
	if len(result.Tokenized) == 0 {
		t.Fatal("Tokenized is empty, want at least one token")
	}
	if result.Tokenized[0].Type != "operator" {
		t.Errorf("Tokenized[0].Type = %q, want \"operator\"", result.Tokenized[0].Type)
	}
	if result.Tokenized[0].Value != "BT" {
		t.Errorf("Tokenized[0].Value = %q, want \"BT\"", result.Tokenized[0].Value)
	}
	lastTok := result.Tokenized[len(result.Tokenized)-1]
	if lastTok.Type != "operator" {
		t.Errorf("last token Type = %q, want \"operator\"", lastTok.Type)
	}
	if lastTok.Value != "ET" {
		t.Errorf("last token Value = %q, want \"ET\"", lastTok.Value)
	}
}

// ---------------------------------------------------------------------------
// 3.1-UNIT-005 [P0]: GetContentStream with non-stream nodeID returns error
// in ContentStreamData.Error field, not a Go error.
// AC#2: Given a node that is not a stream, When GetContentStream is called,
//       Then ContentStreamData.Error is populated, Raw is empty, no Go error.
// ---------------------------------------------------------------------------

func TestGetContentStreamNonStream(t *testing.T) {
	ins, tabID := openMinimal(t)

	// "root" is the catalog dict, not a stream
	result, err := ins.GetContentStream(tabID, "root")
	if err != nil {
		t.Fatalf("GetContentStream returned Go error: %v (want struct-level error)", err)
	}
	if result == nil {
		t.Fatal("GetContentStream returned nil")
	}
	if result.Error == "" {
		t.Fatal("ContentStreamData.Error is empty, want non-empty for non-stream node")
	}
	if result.Raw != "" {
		t.Errorf("Raw = %q, want empty for non-stream node", result.Raw)
	}
}

// ---------------------------------------------------------------------------
// 3.1-UNIT-004 [P0]: GetContentStream with invalid tabID returns
// ErrDocumentNotFound as a Go error.
// ---------------------------------------------------------------------------

func TestGetContentStreamUnknownTab(t *testing.T) {
	ins := NewInspector()
	_, err := ins.GetContentStream("nonexistent-tab", "root")
	if err == nil {
		t.Fatal("expected Go error for unknown tabID, got nil")
	}
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 3.1-UNIT-005b [P0]: GetContentStream with empty nodeID returns Go error.
// ---------------------------------------------------------------------------

func TestGetContentStreamEmptyNodeID(t *testing.T) {
	ins, tabID := openMinimal(t)
	_, err := ins.GetContentStream(tabID, "")
	if err == nil {
		t.Fatal("expected Go error for empty nodeID, got nil")
	}
}

// ---------------------------------------------------------------------------
// 3.1-UNIT-006 [P0]: Decoded content stream is cached per-document; second
// call returns same result without re-decoding.
// AC#3: Caching requirement.
// ---------------------------------------------------------------------------

func TestGetContentStreamCached(t *testing.T) {
	ins, tabID := openContentStream(t)

	streamNodeID := findStreamNode(t, ins, tabID, "root", 0)
	if streamNodeID == "" {
		t.Fatal("no stream node found in content-stream.pdf")
	}

	// First call
	result1, err := ins.GetContentStream(tabID, streamNodeID)
	if err != nil {
		t.Fatalf("first GetContentStream failed: %v", err)
	}
	if result1.Error != "" {
		t.Fatalf("first call Error = %q", result1.Error)
	}

	// Second call
	result2, err := ins.GetContentStream(tabID, streamNodeID)
	if err != nil {
		t.Fatalf("second GetContentStream failed: %v", err)
	}

	// Both calls must return identical data
	if result1.Raw != result2.Raw {
		t.Errorf("cached result differs: first=%q, second=%q", result1.Raw, result2.Raw)
	}
	if result1.NodeID != result2.NodeID {
		t.Errorf("cached NodeID differs: first=%q, second=%q", result1.NodeID, result2.NodeID)
	}

	// Verify cache is populated by checking DocumentState directly
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}
	doc.streamMu.Lock()
	defer doc.streamMu.Unlock()
	if doc.streamCache == nil {
		t.Fatal("streamCache is nil after GetContentStream call")
	}
	cached, ok := doc.streamCache[streamNodeID]
	if !ok {
		t.Fatalf("streamCache does not contain key %q", streamNodeID)
	}
	if cached.Raw != result1.Raw {
		t.Errorf("cached Raw = %q, want %q", cached.Raw, result1.Raw)
	}
}

// ---------------------------------------------------------------------------
// 3.1-UNIT-006b [P0]: GetContentStream with error-prefixed nodeID returns
// ContentStreamData.Error, no panic, no Go error.
// AC#2: Graceful degradation for error nodes.
// ---------------------------------------------------------------------------

func TestGetContentStreamErrorNode(t *testing.T) {
	ins, tabID := openMinimal(t)

	testCases := []string{
		"error:obj:0:5:deref",
		"error:something",
	}
	for _, nodeID := range testCases {
		t.Run(nodeID, func(t *testing.T) {
			result, err := ins.GetContentStream(tabID, nodeID)
			if err != nil {
				t.Fatalf("GetContentStream returned Go error: %v (want struct-level error)", err)
			}
			if result == nil {
				t.Fatal("GetContentStream returned nil")
			}
			if result.Error == "" {
				t.Fatal("ContentStreamData.Error is empty, want non-empty for error node")
			}
			if result.NodeID != nodeID {
				t.Errorf("NodeID = %q, want %q", result.NodeID, nodeID)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 3.1-UNIT-009 [P1]: ContentStreamData.NodeID field is populated correctly.
// ---------------------------------------------------------------------------

func TestGetContentStreamNodeIDPopulated(t *testing.T) {
	ins, tabID := openContentStream(t)

	streamNodeID := findStreamNode(t, ins, tabID, "root", 0)
	if streamNodeID == "" {
		t.Fatal("no stream node found in content-stream.pdf")
	}

	result, err := ins.GetContentStream(tabID, streamNodeID)
	if err != nil {
		t.Fatalf("GetContentStream returned error: %v", err)
	}
	if result.NodeID != streamNodeID {
		t.Errorf("NodeID = %q, want %q", result.NodeID, streamNodeID)
	}
}

// ---------------------------------------------------------------------------
// 3.1-UNIT-011 [P2]: Empty content stream (zero-length) returns empty Raw
// string and no error.
// Edge case: page with no drawing commands.
// ---------------------------------------------------------------------------

func TestGetContentStreamEmpty(t *testing.T) {
	ins := NewInspector()
	tabID := "test-empty-stream"
	_, err := ins.Open(tabID, filepath.Join(testdataDir(t), "empty-stream.pdf"))
	if err != nil {
		t.Fatalf("failed to open empty-stream.pdf: %v", err)
	}

	streamNodeID := findStreamNode(t, ins, tabID, "root", 0)
	if streamNodeID == "" {
		t.Fatal("no stream node found in empty-stream.pdf")
	}

	result, err := ins.GetContentStream(tabID, streamNodeID)
	if err != nil {
		t.Fatalf("GetContentStream returned error: %v", err)
	}
	if result == nil {
		t.Fatal("GetContentStream returned nil")
	}
	if result.Error != "" {
		t.Fatalf("ContentStreamData.Error = %q, want empty", result.Error)
	}
	if result.Raw != "" {
		t.Errorf("Raw = %q, want empty for zero-length stream", result.Raw)
	}
	if result.NodeID != streamNodeID {
		t.Errorf("NodeID = %q, want %q", result.NodeID, streamNodeID)
	}
}

// ---------------------------------------------------------------------------
// 3.1-UNIT-012 [P2]: Cache is cleared when document is closed. After
// Inspector.Close(tabID), GetDocument returns ErrDocumentNotFound, confirming
// the DocumentState (and its streamCache) has been removed.
// ---------------------------------------------------------------------------

func TestGetContentStreamCacheClearedOnClose(t *testing.T) {
	ins, tabID := openContentStream(t)

	streamNodeID := findStreamNode(t, ins, tabID, "root", 0)
	if streamNodeID == "" {
		t.Fatal("no stream node found in content-stream.pdf")
	}

	// Populate the cache.
	_, err := ins.GetContentStream(tabID, streamNodeID)
	if err != nil {
		t.Fatalf("GetContentStream failed: %v", err)
	}

	// Verify cache is populated.
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}
	doc.streamMu.Lock()
	if len(doc.streamCache) == 0 {
		doc.streamMu.Unlock()
		t.Fatal("streamCache is empty before close, expected at least one entry")
	}
	doc.streamMu.Unlock()

	// Close the document.
	if err := ins.Close(tabID); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// DocumentState (and its streamCache) should no longer be reachable.
	_, err = ins.GetDocument(tabID)
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound after close, got: %v", err)
	}

	// GetContentStream should also fail.
	_, err = ins.GetContentStream(tabID, streamNodeID)
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound from GetContentStream after close, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 5.3-UNIT: GetPageContentStreamNodeID resolves a page number to the node ID
// of its content stream.
// ---------------------------------------------------------------------------

func TestGetPageContentStreamNodeID_ContentStreamPDF(t *testing.T) {
	ins, tabID := openContentStream(t)
	nodeID, err := ins.GetPageContentStreamNodeID(tabID, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nodeID == "" {
		t.Fatal("expected non-empty node ID for content-stream.pdf page 1")
	}
	if !strings.HasPrefix(nodeID, "obj:") {
		t.Errorf("node ID should start with 'obj:', got: %s", nodeID)
	}
	parts := strings.SplitN(nodeID, ":", 3)
	if len(parts) != 3 {
		t.Fatalf("node ID should have 3 colon-separated parts, got: %s", nodeID)
	}
}

func TestGetPageContentStreamNodeID_OutOfRange(t *testing.T) {
	ins, tabID := openContentStream(t)
	_, err := ins.GetPageContentStreamNodeID(tabID, 999)
	if err == nil {
		t.Fatal("expected error for out-of-range page, got nil")
	}
}

func TestGetPageContentStreamNodeID_EmptyStream(t *testing.T) {
	ins := NewInspector()
	tabID := "test-empty"
	_, err := ins.Open(tabID, filepath.Join(testdataDir(t), "empty-stream.pdf"))
	if err != nil {
		t.Fatalf("failed to open empty-stream.pdf: %v", err)
	}
	nodeID, err := ins.GetPageContentStreamNodeID(tabID, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(nodeID, "obj:") {
		t.Errorf("expected valid node ID starting with 'obj:', got: %q", nodeID)
	}
}

func TestGetPageContentStreamNodeID_UnknownTab(t *testing.T) {
	ins := NewInspector()
	_, err := ins.GetPageContentStreamNodeID("nonexistent", 1)
	if err == nil {
		t.Fatal("expected error for unknown tab, got nil")
	}
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got: %v", err)
	}
}

// 5.3-PDFCORE-005 [P1]: GetPageContentStreamNodeID returns empty string (no
// error) for a page with no Contents entry. Uses minimal.pdf whose page 1 has
// no Contents key in its page dict.
// AC#4: page with no content stream returns non-fatal empty result.
func TestGetPageContentStreamNodeID_NoContentsEntry(t *testing.T) {
	ins, tabID := openMinimal(t)
	nodeID, err := ins.GetPageContentStreamNodeID(tabID, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nodeID != "" {
		t.Errorf("expected empty node ID for page with no Contents entry, got: %q", nodeID)
	}
}

// ---------------------------------------------------------------------------
// 3.3-UNIT-001 [P0]: Tokenizer produces correct Token structs for a reference
// content stream line: "BT /F1 12 Tf (Hello) Tj ET".
// AC#1: tokenizeContentStream produces Token structs with type, value, line, col.
// ---------------------------------------------------------------------------

func TestTokenizeContentStream(t *testing.T) {
	t.Run("basic operators", func(t *testing.T) {
		tokens := tokenizeContentStream("BT ET")
		if len(tokens) != 2 {
			t.Fatalf("got %d tokens, want 2", len(tokens))
		}
		if tokens[0].Type != "operator" || tokens[0].Value != "BT" {
			t.Errorf("token[0] = %+v, want operator(BT)", tokens[0])
		}
		if tokens[1].Type != "operator" || tokens[1].Value != "ET" {
			t.Errorf("token[1] = %+v, want operator(ET)", tokens[1])
		}
		if tokens[0].Line != 1 || tokens[1].Line != 1 {
			t.Errorf("Line values = %d, %d, want 1, 1", tokens[0].Line, tokens[1].Line)
		}
	})

	t.Run("numbers and operator", func(t *testing.T) {
		tokens := tokenizeContentStream("100 700 Td")
		if len(tokens) != 3 {
			t.Fatalf("got %d tokens, want 3", len(tokens))
		}
		if tokens[0].Type != "number" || tokens[0].Value != "100" {
			t.Errorf("token[0] = %+v, want number(100)", tokens[0])
		}
		if tokens[1].Type != "number" || tokens[1].Value != "700" {
			t.Errorf("token[1] = %+v, want number(700)", tokens[1])
		}
		if tokens[2].Type != "operator" || tokens[2].Value != "Td" {
			t.Errorf("token[2] = %+v, want operator(Td)", tokens[2])
		}
	})

	t.Run("string literal", func(t *testing.T) {
		tokens := tokenizeContentStream("(Hello World) Tj")
		if len(tokens) != 2 {
			t.Fatalf("got %d tokens, want 2", len(tokens))
		}
		if tokens[0].Type != "string" || tokens[0].Value != "(Hello World)" {
			t.Errorf("token[0] = %+v, want string((Hello World))", tokens[0])
		}
		if tokens[1].Type != "operator" || tokens[1].Value != "Tj" {
			t.Errorf("token[1] = %+v, want operator(Tj)", tokens[1])
		}
	})

	t.Run("name object", func(t *testing.T) {
		tokens := tokenizeContentStream("/F1 12 Tf")
		if len(tokens) != 3 {
			t.Fatalf("got %d tokens, want 3", len(tokens))
		}
		if tokens[0].Type != "name" || tokens[0].Value != "/F1" {
			t.Errorf("token[0] = %+v, want name(/F1)", tokens[0])
		}
		if tokens[1].Type != "number" || tokens[1].Value != "12" {
			t.Errorf("token[1] = %+v, want number(12)", tokens[1])
		}
		if tokens[2].Type != "operator" || tokens[2].Value != "Tf" {
			t.Errorf("token[2] = %+v, want operator(Tf)", tokens[2])
		}
	})

	t.Run("multi-line with correct Line values", func(t *testing.T) {
		tokens := tokenizeContentStream("BT\n/F1 12 Tf\nET")
		if len(tokens) != 5 {
			t.Fatalf("got %d tokens, want 5", len(tokens))
		}
		wantLines := []int{1, 2, 2, 2, 3}
		for i, wl := range wantLines {
			if tokens[i].Line != wl {
				t.Errorf("token[%d].Line = %d, want %d (token=%+v)", i, tokens[i].Line, wl, tokens[i])
			}
		}
	})

	t.Run("comment", func(t *testing.T) {
		tokens := tokenizeContentStream("% this is a comment\nBT")
		if len(tokens) != 2 {
			t.Fatalf("got %d tokens, want 2", len(tokens))
		}
		if tokens[0].Type != "comment" || tokens[0].Value != "% this is a comment" {
			t.Errorf("token[0] = %+v, want comment(%% this is a comment)", tokens[0])
		}
		if tokens[1].Type != "operator" || tokens[1].Value != "BT" {
			t.Errorf("token[1] = %+v, want operator(BT)", tokens[1])
		}
	})

	t.Run("hex string", func(t *testing.T) {
		tokens := tokenizeContentStream("<48656C6C6F> Tj")
		if len(tokens) != 2 {
			t.Fatalf("got %d tokens, want 2", len(tokens))
		}
		if tokens[0].Type != "string" || tokens[0].Value != "<48656C6C6F>" {
			t.Errorf("token[0] = %+v, want string(<48656C6C6F>)", tokens[0])
		}
		if tokens[1].Type != "operator" || tokens[1].Value != "Tj" {
			t.Errorf("token[1] = %+v, want operator(Tj)", tokens[1])
		}
	})

	t.Run("nested parens", func(t *testing.T) {
		tokens := tokenizeContentStream("(Hello (nested) World) Tj")
		if len(tokens) != 2 {
			t.Fatalf("got %d tokens, want 2", len(tokens))
		}
		if tokens[0].Type != "string" || tokens[0].Value != "(Hello (nested) World)" {
			t.Errorf("token[0] = %+v, want string((Hello (nested) World))", tokens[0])
		}
	})

	t.Run("negative numbers", func(t *testing.T) {
		tokens := tokenizeContentStream("-100 200 Td")
		if len(tokens) != 3 {
			t.Fatalf("got %d tokens, want 3", len(tokens))
		}
		if tokens[0].Type != "number" || tokens[0].Value != "-100" {
			t.Errorf("token[0] = %+v, want number(-100)", tokens[0])
		}
		if tokens[1].Type != "number" || tokens[1].Value != "200" {
			t.Errorf("token[1] = %+v, want number(200)", tokens[1])
		}
	})

	t.Run("real numbers", func(t *testing.T) {
		tokens := tokenizeContentStream("1.5 0.75 Td")
		if len(tokens) != 3 {
			t.Fatalf("got %d tokens, want 3", len(tokens))
		}
		if tokens[0].Type != "number" || tokens[0].Value != "1.5" {
			t.Errorf("token[0] = %+v, want number(1.5)", tokens[0])
		}
		if tokens[1].Type != "number" || tokens[1].Value != "0.75" {
			t.Errorf("token[1] = %+v, want number(0.75)", tokens[1])
		}
	})

	t.Run("array brackets", func(t *testing.T) {
		tokens := tokenizeContentStream("[3 1] 0 d")
		if len(tokens) != 6 {
			t.Fatalf("got %d tokens, want 6", len(tokens))
		}
		if tokens[0].Type != "operator" || tokens[0].Value != "[" {
			t.Errorf("token[0] = %+v, want operator([)", tokens[0])
		}
		if tokens[1].Type != "number" || tokens[1].Value != "3" {
			t.Errorf("token[1] = %+v, want number(3)", tokens[1])
		}
		if tokens[2].Type != "number" || tokens[2].Value != "1" {
			t.Errorf("token[2] = %+v, want number(1)", tokens[2])
		}
		if tokens[3].Type != "operator" || tokens[3].Value != "]" {
			t.Errorf("token[3] = %+v, want operator(])", tokens[3])
		}
		if tokens[4].Type != "number" || tokens[4].Value != "0" {
			t.Errorf("token[4] = %+v, want number(0)", tokens[4])
		}
	})

	t.Run("empty input", func(t *testing.T) {
		tokens := tokenizeContentStream("")
		if len(tokens) != 0 {
			t.Fatalf("got %d tokens, want 0", len(tokens))
		}
	})

	t.Run("full content stream fixture", func(t *testing.T) {
		tokens := tokenizeContentStream("BT /F1 12 Tf 100 700 Td (Hello World) Tj ET")
		if len(tokens) != 10 {
			t.Fatalf("got %d tokens, want 10", len(tokens))
		}
		// First token is operator BT
		if tokens[0].Type != "operator" || tokens[0].Value != "BT" {
			t.Errorf("first token = %+v, want operator(BT)", tokens[0])
		}
		// Last token is operator ET
		if tokens[9].Type != "operator" || tokens[9].Value != "ET" {
			t.Errorf("last token = %+v, want operator(ET)", tokens[9])
		}
		// Verify all expected types
		wantTypes := []string{"operator", "name", "number", "operator", "number", "number", "operator", "string", "operator", "operator"}
		for i, wt := range wantTypes {
			if tokens[i].Type != wt {
				t.Errorf("token[%d].Type = %q, want %q (value=%q)", i, tokens[i].Type, wt, tokens[i].Value)
			}
		}
	})

	t.Run("leading dot real number", func(t *testing.T) {
		tokens := tokenizeContentStream(".75 Td")
		if len(tokens) != 2 {
			t.Fatalf("got %d tokens, want 2", len(tokens))
		}
		if tokens[0].Type != "number" || tokens[0].Value != ".75" {
			t.Errorf("token[0] = %+v, want number(.75)", tokens[0])
		}
	})

	t.Run("negative leading dot real number", func(t *testing.T) {
		tokens := tokenizeContentStream("-.75 Td")
		if len(tokens) != 2 {
			t.Fatalf("got %d tokens, want 2", len(tokens))
		}
		if tokens[0].Type != "number" || tokens[0].Value != "-.75" {
			t.Errorf("token[0] = %+v, want number(-.75)", tokens[0])
		}
	})

	t.Run("inline image BI ID EI", func(t *testing.T) {
		input := "BI /W 10 /H 10 /CS /G /BPC 8 ID \x00\x01\x02 EI"
		tokens := tokenizeContentStream(input)
		// BI, /W, 10, /H, 10, /CS, /G, /BPC, 8, ID, <image data>, EI
		// Find the ID and EI tokens
		var idIdx, eiIdx int
		for i, tok := range tokens {
			if tok.Value == "ID" && tok.Type == "operator" {
				idIdx = i
			}
			if tok.Value == "EI" && tok.Type == "operator" {
				eiIdx = i
			}
		}
		if idIdx == 0 {
			t.Fatal("ID operator not found in tokenized output")
		}
		if eiIdx == 0 {
			t.Fatal("EI operator not found in tokenized output")
		}
		if eiIdx <= idIdx+1 {
			t.Errorf("EI (%d) should be at least 2 positions after ID (%d) to hold image data", eiIdx, idIdx)
		}
		// The token between ID and EI should be a string (the raw image data)
		dataTok := tokens[idIdx+1]
		if dataTok.Type != "string" {
			t.Errorf("image data token Type = %q, want \"string\"", dataTok.Type)
		}
	})

	t.Run("escaped paren in string", func(t *testing.T) {
		tokens := tokenizeContentStream(`(a\)b) Tj`)
		if len(tokens) != 2 {
			t.Fatalf("got %d tokens, want 2", len(tokens))
		}
		if tokens[0].Type != "string" || tokens[0].Value != `(a\)b)` {
			t.Errorf("token[0] = %+v, want string((a\\)b))", tokens[0])
		}
	})
}

// ---------------------------------------------------------------------------
// 3.3-UNIT-002 [P0]: Tokenizer classifies all standard PDF operators correctly.
// AC#1: Any non-number, non-string, non-name, non-comment word is classified
// as "operator". Table-driven test against known operator list.
// ---------------------------------------------------------------------------

func TestTokenizeContentStreamOperators(t *testing.T) {
	operators := []string{
		"BT", "ET", "Tf", "Td", "TD", "Tm", "Tj", "TJ", "T*",
		"Tc", "Tw", "Tz", "TL", "Tr", "Ts",
		"q", "Q", "cm", "m", "l", "c", "v", "y", "h",
		"re", "S", "s", "f", "F", "f*", "B", "B*", "b", "b*", "n",
		"W", "W*", "Do", "gs",
		"CS", "cs", "SC", "SCN", "sc", "scn",
		"G", "g", "RG", "rg", "K", "k",
		"w", "J", "j", "M", "d", "ri", "i",
		"BMC", "BDC", "EMC", "MP", "DP",
		"BI", "ID", "EI",
		"d0", "d1", "sh",
	}
	for _, op := range operators {
		t.Run(op, func(t *testing.T) {
			tokens := tokenizeContentStream(op)
			if len(tokens) != 1 {
				t.Fatalf("got %d tokens for %q, want 1", len(tokens), op)
			}
			if tokens[0].Type != "operator" {
				t.Errorf("Type = %q for %q, want \"operator\"", tokens[0].Type, op)
			}
			if tokens[0].Value != op {
				t.Errorf("Value = %q, want %q", tokens[0].Value, op)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 3.3-UNIT-LINE-COL [P1]: Tokenizer Line and Col are 1-based and track
// correctly across newlines.
// ---------------------------------------------------------------------------

func TestTokenizeContentStreamLineCol(t *testing.T) {
	input := "BT\n  /F1 12 Tf\nET"
	tokens := tokenizeContentStream(input)

	// BT at line 1, col 1
	if tokens[0].Line != 1 || tokens[0].Col != 1 {
		t.Errorf("BT: Line=%d Col=%d, want Line=1 Col=1", tokens[0].Line, tokens[0].Col)
	}
	// /F1 at line 2, col 3 (after two leading spaces)
	if tokens[1].Line != 2 || tokens[1].Col != 3 {
		t.Errorf("/F1: Line=%d Col=%d, want Line=2 Col=3", tokens[1].Line, tokens[1].Col)
	}
	// 12 at line 2, col 7
	if tokens[2].Line != 2 || tokens[2].Col != 7 {
		t.Errorf("12: Line=%d Col=%d, want Line=2 Col=7", tokens[2].Line, tokens[2].Col)
	}
	// Tf at line 2, col 10
	if tokens[3].Line != 2 || tokens[3].Col != 10 {
		t.Errorf("Tf: Line=%d Col=%d, want Line=2 Col=10", tokens[3].Line, tokens[3].Col)
	}
	// ET at line 3, col 1
	if tokens[4].Line != 3 || tokens[4].Col != 1 {
		t.Errorf("ET: Line=%d Col=%d, want Line=3 Col=1", tokens[4].Line, tokens[4].Col)
	}
}

// ---------------------------------------------------------------------------
// 3.3-UNIT-EDGE [P2]: Tokenizer edge cases not covered by the main test suite.
// Bare '-', bare '.', dict delimiters, unbalanced parens, \r\n line tracking.
// ---------------------------------------------------------------------------

func TestTokenizeContentStreamEdgeCases(t *testing.T) {
	t.Run("bare minus not followed by digit is operator", func(t *testing.T) {
		tokens := tokenizeContentStream("- 10 Td")
		if len(tokens) != 3 {
			t.Fatalf("got %d tokens, want 3", len(tokens))
		}
		if tokens[0].Type != "operator" || tokens[0].Value != "-" {
			t.Errorf("token[0] = %+v, want operator(-)", tokens[0])
		}
		if tokens[1].Type != "number" || tokens[1].Value != "10" {
			t.Errorf("token[1] = %+v, want number(10)", tokens[1])
		}
	})

	t.Run("bare dot not followed by digit is operator", func(t *testing.T) {
		tokens := tokenizeContentStream(". 10 Td")
		if len(tokens) != 3 {
			t.Fatalf("got %d tokens, want 3", len(tokens))
		}
		if tokens[0].Type != "operator" || tokens[0].Value != "." {
			t.Errorf("token[0] = %+v, want operator(.)", tokens[0])
		}
	})

	t.Run("dict delimiters << and >>", func(t *testing.T) {
		tokens := tokenizeContentStream("<< /Type /Font >>")
		if len(tokens) != 4 {
			t.Fatalf("got %d tokens, want 4", len(tokens))
		}
		if tokens[0].Type != "operator" || tokens[0].Value != "<<" {
			t.Errorf("token[0] = %+v, want operator(<<)", tokens[0])
		}
		if tokens[3].Type != "operator" || tokens[3].Value != ">>" {
			t.Errorf("token[3] = %+v, want operator(>>)", tokens[3])
		}
	})

	t.Run("unbalanced paren produces best-effort string token", func(t *testing.T) {
		tokens := tokenizeContentStream("(unclosed Tj")
		if len(tokens) == 0 {
			t.Fatal("got 0 tokens, want at least 1")
		}
		// Best-effort: the string token consumes to end of input.
		if tokens[0].Type != "string" {
			t.Errorf("token[0].Type = %q, want \"string\"", tokens[0].Type)
		}
	})

	t.Run("CRLF treated as single newline for line tracking", func(t *testing.T) {
		tokens := tokenizeContentStream("BT\r\nET")
		if len(tokens) != 2 {
			t.Fatalf("got %d tokens, want 2", len(tokens))
		}
		if tokens[0].Line != 1 {
			t.Errorf("BT Line = %d, want 1", tokens[0].Line)
		}
		if tokens[1].Line != 2 {
			t.Errorf("ET Line = %d, want 2", tokens[1].Line)
		}
		if tokens[1].Col != 1 {
			t.Errorf("ET Col = %d, want 1", tokens[1].Col)
		}
	})

	t.Run("whitespace-only input returns empty slice", func(t *testing.T) {
		tokens := tokenizeContentStream("   \t\n  ")
		if len(tokens) != 0 {
			t.Fatalf("got %d tokens, want 0 for whitespace-only input", len(tokens))
		}
	})

	t.Run("consecutive operators without whitespace delimiters", func(t *testing.T) {
		// Array brackets are self-delimiting: "[3]" should tokenize
		tokens := tokenizeContentStream("[3]")
		if len(tokens) != 3 {
			t.Fatalf("got %d tokens, want 3", len(tokens))
		}
		if tokens[0].Type != "operator" || tokens[0].Value != "[" {
			t.Errorf("token[0] = %+v, want operator([)", tokens[0])
		}
		if tokens[1].Type != "number" || tokens[1].Value != "3" {
			t.Errorf("token[1] = %+v, want number(3)", tokens[1])
		}
		if tokens[2].Type != "operator" || tokens[2].Value != "]" {
			t.Errorf("token[2] = %+v, want operator(])", tokens[2])
		}
	})

	t.Run("name immediately after string without whitespace", func(t *testing.T) {
		tokens := tokenizeContentStream("(text)/Name")
		if len(tokens) != 2 {
			t.Fatalf("got %d tokens, want 2", len(tokens))
		}
		if tokens[0].Type != "string" || tokens[0].Value != "(text)" {
			t.Errorf("token[0] = %+v, want string((text))", tokens[0])
		}
		if tokens[1].Type != "name" || tokens[1].Value != "/Name" {
			t.Errorf("token[1] = %+v, want name(/Name)", tokens[1])
		}
	})
}

// ---------------------------------------------------------------------------
// 3.3-BENCH-001 [P2]: Benchmark tokenizer on a ~100KB content stream.
// Verifies O(n) performance and detects regressions.
// ---------------------------------------------------------------------------

func BenchmarkTokenizeContentStream100KB(b *testing.B) {
	// Build a ~100KB content stream with realistic operations.
	var buf strings.Builder
	line := "q 1 0 0 1 72 720 cm BT /F1 12 Tf 0 0 Td (Hello World) Tj ET Q\n"
	for buf.Len() < 100*1024 {
		buf.WriteString(line)
	}
	input := buf.String()
	b.SetBytes(int64(len(input)))
	b.ResetTimer()
	for b.Loop() {
		tokenizeContentStream(input)
	}
}
