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
