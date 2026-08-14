package pdfcore

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// ---------------------------------------------------------------------------
// GetContentStream returns decoded plain text for a page's Contents node
// using testdata/content-stream.pdf.
// Given a tree node ID corresponding to a page's Contents entry,
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

	// Task 2.3: Verify tokenizer integration in GetContentStream. Tokenized field must be
	// non-nil, non-empty, and contain at least BT and ET operators.
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
// GetContentStream with non-stream nodeID returns error in
// ContentStreamData.Error field, not a Go error.
// Given a node that is not a stream, When GetContentStream is called,
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
// GetContentStream with invalid tabID returns
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
// GetContentStream with empty nodeID returns Go error.
// ---------------------------------------------------------------------------

func TestGetContentStreamEmptyNodeID(t *testing.T) {
	ins, tabID := openMinimal(t)
	_, err := ins.GetContentStream(tabID, "")
	if err == nil {
		t.Fatal("expected Go error for empty nodeID, got nil")
	}
}

// ---------------------------------------------------------------------------
// Decoded content stream is cached per-document; second call returns same
// result without re-decoding.
// Caching requirement.
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
// GetContentStream with error-prefixed nodeID returns
// ContentStreamData.Error, no panic, no Go error.
// Graceful degradation for error nodes.
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
// ContentStreamData.NodeID field is populated correctly.
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
// Empty content stream (zero-length) returns empty Raw string and no
// error. Edge case: page with no drawing commands.
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
// Cache is cleared when document is closed. After Inspector.Close(tabID),
// GetDocument returns ErrDocumentNotFound, confirming the DocumentState (and
// its streamCache) has been removed.
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
// GetPageContentStreamNodeID resolves a page number to the node ID of its
// content stream.
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

// GetPageContentStreamNodeID returns empty string (no error) for a page with
// no Contents entry. Uses minimal.pdf whose page 1 has no Contents key in its
// page dict.
// page with no content stream returns non-fatal empty result.
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
// pageContentStreamNodeIDs enumerates the page's content-stream
// refs (the concatenation order/set).
//
// Per ISO 32000-1 7.8.2 a page's content is the concatenation of its /Contents
// array's STREAM refs, so a degenerate null / non-ref element contributes NO
// stream and must be skipped (not counted, not concatenated). This test pins
// that: a single indirect ref, a one-element array `[ref]`, and the degenerate
// `[ref null]` all enumerate to 1 stream; a genuine multi-ref array `[ref ref]`
// enumerates 2; a page with no /Contents enumerates 0.
//
// The `[ref null]` case is built by an in-memory page-dict mutation, not a disk
// fixture: pdfcpu rejects an on-disk /Contents array containing a null element
// at read time (DereferenceStreamDict: wrong type <nil>), so the only way to
// drive that degenerate array into pageContentStreamNodeIDs is to inject it
// after a valid open. This still exercises the production enumeration verbatim.
// ---------------------------------------------------------------------------

// contentStreamObj builds a minimal, valid content-stream object body numbered
// n for the pageContentStreamNodeIDs fixtures.
func contentStreamObj(n int) string {
	body := "BT /F1 12 Tf 100 700 Td (x) Tj ET"
	return fmt.Sprintf("%d 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", n, len(body), body)
}

func TestPageContentStreamNodeIDs(t *testing.T) {
	catalog := "1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"
	pages := "2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"

	cases := []struct {
		name string
		page string
		objs []string
		want int
	}{
		{
			name: "single indirect ref",
			page: "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>\nendobj\n",
			objs: []string{contentStreamObj(4)},
			want: 1,
		},
		{
			name: "one-element array [ref]",
			page: "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents [4 0 R] >>\nendobj\n",
			objs: []string{contentStreamObj(4)},
			want: 1,
		},
		{
			name: "multi-ref array [ref ref]",
			page: "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents [4 0 R 5 0 R] >>\nendobj\n",
			objs: []string{contentStreamObj(4), contentStreamObj(5)},
			want: 2,
		},
		{
			name: "no /Contents entry",
			page: "3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
			objs: nil,
			want: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			objs := append([]string{catalog, pages, tc.page}, tc.objs...)
			ins, tabID := writeTempPDF(t, "count.pdf", assembleDiffPDF(1, objs...))
			ids, err := ins.pageContentStreamNodeIDs(tabID, 1)
			got := len(ids)
			if err != nil {
				t.Fatalf("%s: unexpected error: %v", tc.name, err)
			}
			if got != tc.want {
				t.Errorf("%s: count = %d, want %d", tc.name, got, tc.want)
			}
		})
	}

	// Anti-false-positive: a degenerate [ref null] array must count 1, not 2.
	// pdfcpu rejects this shape on disk, so open a valid [ref] fixture and inject
	// the trailing null into the live page dict before the count call.
	t.Run("degenerate array [ref null] is not truncation", func(t *testing.T) {
		objs := []string{
			catalog,
			pages,
			"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents [4 0 R] >>\nendobj\n",
			contentStreamObj(4),
		}
		ins, tabID := writeTempPDF(t, "refnull.pdf", assembleDiffPDF(1, objs...))
		doc, err := ins.GetDocument(tabID)
		if err != nil {
			t.Fatalf("GetDocument: %v", err)
		}
		pageDict, _, _, err := doc.PDFContext.PageDict(1, false)
		if err != nil {
			t.Fatalf("PageDict: %v", err)
		}
		arr, ok := pageDict["Contents"].(pdfcpu_types.Array)
		if !ok || len(arr) != 1 {
			t.Fatalf("fixture broken: /Contents = %v, want a one-element array", pageDict["Contents"])
		}
		ref := arr[0].(pdfcpu_types.IndirectRef)
		// Inject the degenerate [ref null] shape the fix guards against.
		pageDict["Contents"] = pdfcpu_types.Array{ref, nil}

		ids, err := ins.pageContentStreamNodeIDs(tabID, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got := len(ids); got != 1 {
			t.Errorf("[ref null] stream count = %d, want 1 (a null element is not a stream, so it is skipped in concatenation)", got)
		}
	})

	// A null at index 0 is skipped just like a null anywhere else (PR review:
	// null handling must not depend on position). [null ref] enumerates the one
	// real stream, exactly like [ref null].
	t.Run("degenerate array [null ref] skips the leading null", func(t *testing.T) {
		objs := []string{
			catalog,
			pages,
			"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents [4 0 R] >>\nendobj\n",
			contentStreamObj(4),
		}
		ins, tabID := writeTempPDF(t, "nullref.pdf", assembleDiffPDF(1, objs...))
		doc, err := ins.GetDocument(tabID)
		if err != nil {
			t.Fatalf("GetDocument: %v", err)
		}
		pageDict, _, _, err := doc.PDFContext.PageDict(1, false)
		if err != nil {
			t.Fatalf("PageDict: %v", err)
		}
		arr, ok := pageDict["Contents"].(pdfcpu_types.Array)
		if !ok || len(arr) != 1 {
			t.Fatalf("fixture broken: /Contents = %v, want a one-element array", pageDict["Contents"])
		}
		ref := arr[0].(pdfcpu_types.IndirectRef)
		// Inject the null AT INDEX 0.
		pageDict["Contents"] = pdfcpu_types.Array{nil, ref}

		ids, err := ins.pageContentStreamNodeIDs(tabID, 1)
		if err != nil {
			t.Fatalf("[null ref] unexpected error: %v (a leading null must be skipped, not rejected)", err)
		}
		if got := len(ids); got != 1 {
			t.Errorf("[null ref] stream count = %d, want 1 (leading null skipped like any other null)", got)
		}
	})

	// A non-null, non-ref element is malformed rather than empty: /Contents
	// elements are refs to streams (7.8.2) and streams are always indirect
	// (7.3.8). Skipping it could omit real page content, so it must be reported.
	// Same in-memory injection as above - pdfcpu rejects the shape on disk.
	t.Run("[ref junk] errors instead of silently skipping", func(t *testing.T) {
		objs := []string{
			catalog,
			pages,
			"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents [4 0 R] >>\nendobj\n",
			contentStreamObj(4),
		}
		ins, tabID := writeTempPDF(t, "refjunk.pdf", assembleDiffPDF(1, objs...))
		doc, err := ins.GetDocument(tabID)
		if err != nil {
			t.Fatalf("GetDocument: %v", err)
		}
		pageDict, _, _, err := doc.PDFContext.PageDict(1, false)
		if err != nil {
			t.Fatalf("PageDict: %v", err)
		}
		arr, ok := pageDict["Contents"].(pdfcpu_types.Array)
		if !ok || len(arr) != 1 {
			t.Fatalf("fixture broken: /Contents = %v, want a one-element array", pageDict["Contents"])
		}
		ref := arr[0].(pdfcpu_types.IndirectRef)
		pageDict["Contents"] = pdfcpu_types.Array{ref, pdfcpu_types.Integer(42)}

		ids, err := ins.pageContentStreamNodeIDs(tabID, 1)
		if err == nil {
			t.Fatalf("[ref 42] returned %d ids and no error; a non-null non-ref element must be reported, not skipped", len(ids))
		}
		if !strings.Contains(err.Error(), "element 1") {
			t.Errorf("error must name the offending index, got: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Tokenizer produces correct Token structs for a reference content stream
// line: "BT /F1 12 Tf (Hello) Tj ET".
// tokenizeContentStream produces Token structs with type, value, line, col.
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
// Tokenizer classifies all standard PDF operators correctly.
// Any non-number, non-string, non-name, non-comment word is classified as
// "operator". Table-driven test against known operator list.
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
// Tokenizer Line and Col are 1-based and track correctly across
// newlines.
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
// Tokenizer edge cases not covered by the main test suite. Bare '-', bare '.',
// dict delimiters, unbalanced parens, \r\n line tracking.
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
// Tokenizer delivers inline-image payloads opaquely.
//
// Locks the contract the content-stream formatter depends on:
// for any BI..ID..<bytes>..EI sequence the tokenizer emits exactly
// [..., {operator ID}, {string <payload>}, {operator EI}] with the payload
// delivered as ONE string token regardless of whitespace, newlines, or the
// literal byte sequence "EI" appearing inside the payload (without the
// required surrounding whitespace that signals the real terminator).
// ---------------------------------------------------------------------------

func TestTokenizeInlineImagePayloadOpaque(t *testing.T) {
	cases := []struct {
		name    string
		payload string
	}{
		{name: "binary with NULs and high bytes", payload: "\x00\x01\xFF\xFE\x80"},
		{name: "payload contains newlines and CR/LF", payload: "abc\ndef\r\nghi"},
		{name: "payload contains literal EI without surrounding whitespace", payload: "abEIcd"},
		{name: "payload contains literal EI preceded by non-whitespace", payload: "xEI yz"}, // 'x' before E disqualifies
		{name: "payload contains EI followed by non-whitespace", payload: " EIx"},          // 'x' after I disqualifies
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := "BI /W 1 /H 1 /CS /G /BPC 8 ID " + tc.payload + " EI"
			toks := tokenizeContentStream(input)

			// Find ID and EI operator indices.
			idIdx, eiIdx := -1, -1
			for i, tk := range toks {
				if tk.Type == "operator" && tk.Value == "ID" && idIdx == -1 {
					idIdx = i
				}
				if tk.Type == "operator" && tk.Value == "EI" && eiIdx == -1 && idIdx != -1 && i > idIdx {
					eiIdx = i
				}
			}
			if idIdx == -1 {
				t.Fatalf("no ID operator emitted for %q", input)
			}
			if eiIdx == -1 {
				t.Fatalf("no EI operator emitted (terminator missed) for %q", input)
			}
			// Exactly one token between ID and EI: the opaque payload string.
			if eiIdx-idIdx != 2 {
				t.Fatalf("expected exactly one payload token between ID and EI; got %d tokens, sequence: %+v",
					eiIdx-idIdx-1, toks[idIdx:eiIdx+1])
			}
			payloadTok := toks[idIdx+1]
			if payloadTok.Type != "string" {
				t.Errorf("payload token Type = %q, want \"string\"", payloadTok.Type)
			}
			if payloadTok.Value != tc.payload {
				t.Errorf("payload token Value = %q, want %q", payloadTok.Value, tc.payload)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// inline-image data token has no trailing whitespace (F3). A "\r\nEI"
// sequence must drop the CR as well as the LF delimiter (one CRLF unit), not
// leave the stray '\r' the single-byte strip left behind, so the opaque data
// token carries no trailing EOL fragment.
// ---------------------------------------------------------------------------

func TestTokenizeInlineImageTrailingWhitespaceStripped(t *testing.T) {
	input := "q\r\nBI /W 1 /H 1 ID\x00\x01\x02\r\nEI\nQ"
	toks := tokenizeContentStream(input)

	idIdx, eiIdx := -1, -1
	for i, tk := range toks {
		if tk.Type == "operator" && tk.Value == "ID" && idIdx == -1 {
			idIdx = i
		}
		if tk.Type == "operator" && tk.Value == "EI" && eiIdx == -1 && idIdx != -1 && i > idIdx {
			eiIdx = i
		}
	}
	if idIdx == -1 || eiIdx == -1 {
		t.Fatalf("expected ID and EI operators; got tokens=%+v", toks)
	}
	if eiIdx-idIdx != 2 {
		t.Fatalf("expected exactly one payload token between ID and EI; got %d, sequence=%+v",
			eiIdx-idIdx-1, toks[idIdx:eiIdx+1])
	}
	payload := toks[idIdx+1]
	if payload.Value != "\x00\x01\x02" {
		t.Errorf("payload Value = %q, want %q (\"\\r\\n\" CRLF delimiter before EI must be stripped, CR included)", payload.Value, "\x00\x01\x02")
	}
	if strings.HasSuffix(payload.Value, "\r") || strings.HasSuffix(payload.Value, "\n") {
		t.Errorf("payload Value = %q retains trailing whitespace", payload.Value)
	}
}

// A whitespace-valued byte that is genuinely part of the opaque inline-image
// payload (here a 0x20 sample immediately before the single-LF delimiter) must
// be preserved. The EOL strip is bounded to one delimiter (CRLF unit), not an
// unbounded whitespace run that would eat real data bytes.
func TestTokenizeInlineImagePayloadWhitespaceValuedByteKept(t *testing.T) {
	input := "q\nBI /W 1 /H 1 ID\x00\x20\nEI\nQ"
	toks := tokenizeContentStream(input)

	idIdx, eiIdx := -1, -1
	for i, tk := range toks {
		if tk.Type == "operator" && tk.Value == "ID" && idIdx == -1 {
			idIdx = i
		}
		if tk.Type == "operator" && tk.Value == "EI" && eiIdx == -1 && idIdx != -1 && i > idIdx {
			eiIdx = i
		}
	}
	if idIdx == -1 || eiIdx == -1 {
		t.Fatalf("expected ID and EI operators; got tokens=%+v", toks)
	}
	if eiIdx-idIdx != 2 {
		t.Fatalf("expected exactly one payload token between ID and EI; got %d, sequence=%+v",
			eiIdx-idIdx-1, toks[idIdx:eiIdx+1])
	}
	payload := toks[idIdx+1]
	if payload.Value != "\x00\x20" {
		t.Errorf("payload Value = %q, want %q (whitespace-valued payload byte before the single-LF delimiter must be preserved, not stripped)", payload.Value, "\x00\x20")
	}
}

// ---------------------------------------------------------------------------
// leading-sign number tokenization (F1).
//
// ISO 32000-1 7.3.3 permits a leading '+' as well as '-' on integers and
// reals, so a spec-valid operand like "+5" must classify as a SINGLE "number"
// token rather than falling through to the word/operator branch. This
// table-driven test feeds bare number byte-slices and asserts that.
//
// The bare "+"/"-" cases -- a sign not followed by a digit or ".digit" --
// REMAIN word/operator tokens.
// ---------------------------------------------------------------------------

func TestTokenizeLeadingSignNumbers(t *testing.T) {
	numberCases := []struct {
		name  string
		input string
		want  string
	}{
		{"leading plus integer", "+5", "+5"},
		{"leading plus leading-dot real", "+.5", "+.5"},
		{"leading minus integer", "-3", "-3"},
		{"leading minus leading-dot real", "-.5", "-.5"},
		{"leading plus real", "+5.0", "+5.0"},
	}
	for _, tc := range numberCases {
		t.Run(tc.name, func(t *testing.T) {
			tokens := tokenizeContentStream(tc.input)
			if len(tokens) != 1 {
				t.Fatalf("%q: got %d tokens, want exactly 1 (single number token); tokens=%+v",
					tc.input, len(tokens), tokens)
			}
			if tokens[0].Type != "number" {
				t.Errorf("%q: token Type = %q, want \"number\"", tc.input, tokens[0].Type)
			}
			if tokens[0].Value != tc.want {
				t.Errorf("%q: token Value = %q, want %q", tc.input, tokens[0].Value, tc.want)
			}
		})
	}

	// A bare sign not followed by a digit or ".digit" stays a word/operator
	// token (unchanged behavior). Guards against an over-broad fix.
	bareCases := []struct {
		name  string
		input string
		want  string
	}{
		{"bare plus stays operator", "+ 10 Td", "+"},
		{"bare minus stays operator", "- 10 Td", "-"},
	}
	for _, tc := range bareCases {
		t.Run(tc.name, func(t *testing.T) {
			tokens := tokenizeContentStream(tc.input)
			if len(tokens) == 0 {
				t.Fatalf("%q: got 0 tokens, want at least 1", tc.input)
			}
			if tokens[0].Type != "operator" || tokens[0].Value != tc.want {
				t.Errorf("%q: token[0] = %+v, want operator(%s)", tc.input, tokens[0], tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Benchmark tokenizer on a ~100KB content stream. Verifies O(n)
// performance and detects regressions.
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

// ---------------------------------------------------------------------------
// Concurrent cache-build race avoidance
// ---------------------------------------------------------------------------

// TestGetContentStreamConcurrentSameNode asserts two concurrent GetContentStream
// calls against the SAME nodeID with an initially-empty streamCache collapse to
// one resolve+decode pass: both callers receive the same *ContentStreamData
// pointer and Raw is non-empty.
//
// The goroutines park on a shared `start` channel and are released
// simultaneously so they hit the cache-check critical section in the same window.
// Without that barrier the two calls could serialize naturally and pass even when
// streamMu is dropped between the cache check and the cache write.
//
// The failure mode this catches is pointer inequality between the two returned
// *ContentStreamData.
func TestGetContentStreamConcurrentSameNode(t *testing.T) {
	ins, tabID := openContentStream(t)
	t.Cleanup(func() { _ = ins.Close(tabID) })

	// Resolve a real stream nodeID via the page-1 Contents lookup, matching
	// the spec verbatim ("the node ID resolved via
	// GetPageContentStreamNodeID(tabID, 1)").
	nodeID, err := ins.GetPageContentStreamNodeID(tabID, 1)
	if err != nil {
		t.Fatalf("GetPageContentStreamNodeID(1) failed: %v", err)
	}
	if nodeID == "" {
		t.Fatalf("page 1 of content-stream.pdf has no Contents node -- fixture broken")
	}

	// Sanity check: cache MUST start empty. If a prior call populated it
	// (e.g. through Open eagerly building anything), the race window
	// collapses and the test passes for the wrong reason.
	doc, err := ins.GetDocument(tabID)
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}
	doc.streamMu.Lock()
	if _, exists := doc.streamCache[nodeID]; exists {
		doc.streamMu.Unlock()
		t.Fatalf("streamCache already populated for %q -- prerequisite violated", nodeID)
	}
	doc.streamMu.Unlock()

	start := make(chan struct{})
	var ready sync.WaitGroup
	var done sync.WaitGroup
	ready.Add(2)
	done.Add(2)

	results := make([]*ContentStreamData, 2)
	errs := make([]error, 2)

	for i := 0; i < 2; i++ {
		go func(idx int) {
			defer done.Done()
			ready.Done()
			<-start // park until both goroutines are at the barrier
			r, e := ins.GetContentStream(tabID, nodeID)
			results[idx] = r
			errs[idx] = e
		}(i)
	}

	ready.Wait()
	close(start)
	done.Wait()

	for i, e := range errs {
		if e != nil {
			t.Fatalf("goroutine %d returned error: %v", i, e)
		}
		if results[i] == nil {
			t.Fatalf("goroutine %d returned nil *ContentStreamData", i)
		}
	}

	// Pointer equality: both goroutines MUST see the SAME cached pointer.
	// Inequality is the bug signal (each goroutine wrote its own object).
	if results[0] != results[1] {
		t.Errorf("concurrent GetContentStream returned different *ContentStreamData pointers (%p vs %p) -- expected pointer equality (single cache entry)", results[0], results[1])
	}

	// Raw must be non-empty -- a clobbered placeholder (zero-length result
	// written before decode completed) is the other tell-tale of the race.
	if results[0] != nil && results[0].Raw == "" {
		t.Errorf("result[0].Raw is empty -- expected decoded content (clobbered placeholder?)")
	}

	// Cache must contain exactly one entry for this nodeID, and that entry
	// must equal the pointer returned to the callers.
	doc.streamMu.Lock()
	cached, ok := doc.streamCache[nodeID]
	cacheLen := len(doc.streamCache)
	doc.streamMu.Unlock()
	if !ok {
		t.Errorf("streamCache missing %q after concurrent calls -- single resolve+decode pass should have written exactly one entry", nodeID)
	}
	if cached != results[0] {
		t.Errorf("cached pointer differs from returned pointer (%p vs %p)", cached, results[0])
	}
	if cacheLen < 1 {
		t.Errorf("streamCache size = %d, expected >= 1", cacheLen)
	}
}

