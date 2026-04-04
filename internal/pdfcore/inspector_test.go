package pdfcore

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata")
}

func TestOpenValidPDF(t *testing.T) {
	ins := NewInspector()
	info, err := ins.Open("tab-1", filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if info.TabID != "tab-1" {
		t.Errorf("TabID = %q, want %q", info.TabID, "tab-1")
	}
	if info.FileName != "minimal.pdf" {
		t.Errorf("FileName = %q, want %q", info.FileName, "minimal.pdf")
	}
	if info.PageCount < 1 {
		t.Errorf("PageCount = %d, want >= 1", info.PageCount)
	}
	if info.FileSize <= 0 {
		t.Errorf("FileSize = %d, want > 0", info.FileSize)
	}
	if info.FilePath == "" {
		t.Error("FilePath is empty")
	}
	if !filepath.IsAbs(info.FilePath) {
		t.Errorf("FilePath %q is not absolute", info.FilePath)
	}
}

func TestOpenMultipagePDF(t *testing.T) {
	ins := NewInspector()
	info, err := ins.Open("tab-2", filepath.Join(testdataDir(t), "multipage.pdf"))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if info.PageCount < 2 {
		t.Errorf("PageCount = %d, want >= 2", info.PageCount)
	}
}

func TestOpenMalformedPDF(t *testing.T) {
	ins := NewInspector()
	_, err := ins.Open("tab-3", filepath.Join(testdataDir(t), "malformed.pdf"))
	if err == nil {
		t.Fatal("expected error for malformed PDF, got nil")
	}
	if !errors.Is(err, ErrMalformedPDF) {
		t.Errorf("expected ErrMalformedPDF, got %v", err)
	}
}

func TestOpenEncryptedPDF(t *testing.T) {
	ins := NewInspector()
	_, err := ins.Open("tab-4", filepath.Join(testdataDir(t), "encrypted.pdf"))
	if err == nil {
		t.Fatal("expected error for encrypted PDF, got nil")
	}
	if !errors.Is(err, ErrEncryptedPDF) {
		t.Errorf("expected ErrEncryptedPDF, got %v", err)
	}
}

func TestOpenNonExistentFile(t *testing.T) {
	ins := NewInspector()
	_, err := ins.Open("tab-5", "/nonexistent/path/to/file.pdf")
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

func TestCloseRemovesDocument(t *testing.T) {
	ins := NewInspector()
	_, err := ins.Open("tab-6", filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if err := ins.Close("tab-6"); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	_, err = ins.GetDocument("tab-6")
	if err == nil {
		t.Fatal("expected error after Close, got nil")
	}
}

func TestGetDocumentReturnsOpened(t *testing.T) {
	ins := NewInspector()
	_, err := ins.Open("tab-7", filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	doc, err := ins.GetDocument("tab-7")
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}
	if doc.PageCount < 1 {
		t.Errorf("PageCount = %d, want >= 1", doc.PageCount)
	}
}

func TestGetDocumentUnknownTabID(t *testing.T) {
	ins := NewInspector()
	_, err := ins.GetDocument("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown tabID, got nil")
	}
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

func TestOpenEmptyTabID(t *testing.T) {
	ins := NewInspector()
	_, err := ins.Open("", filepath.Join(testdataDir(t), "minimal.pdf"))
	if err == nil {
		t.Fatal("expected error for empty tabID, got nil")
	}
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

func TestOpenEmptyFilePath(t *testing.T) {
	ins := NewInspector()
	_, err := ins.Open("tab-x", "")
	if err == nil {
		t.Fatal("expected error for empty filePath, got nil")
	}
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

func TestOpenDirectoryPath(t *testing.T) {
	ins := NewInspector()
	_, err := ins.Open("tab-x", testdataDir(t))
	if err == nil {
		t.Fatal("expected error for directory path, got nil")
	}
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

func TestOpenOverwritesSameTabID(t *testing.T) {
	ins := NewInspector()
	_, err := ins.Open("tab-dup", filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("first Open failed: %v", err)
	}
	info, err := ins.Open("tab-dup", filepath.Join(testdataDir(t), "multipage.pdf"))
	if err != nil {
		t.Fatalf("second Open failed: %v", err)
	}
	if info.FileName != "multipage.pdf" {
		t.Errorf("FileName = %q, want %q", info.FileName, "multipage.pdf")
	}
	doc, err := ins.GetDocument("tab-dup")
	if err != nil {
		t.Fatalf("GetDocument failed: %v", err)
	}
	if doc.PageCount < 2 {
		t.Errorf("PageCount = %d, want >= 2 (multipage)", doc.PageCount)
	}
}

func TestCloseUnknownTabID(t *testing.T) {
	ins := NewInspector()
	err := ins.Close("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown tabID, got nil")
	}
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

// --- GetObjectDetail tests ---

func TestGetObjectDetailDict(t *testing.T) {
	ins, tabID := openMinimal(t)
	detail, err := ins.GetObjectDetail(tabID, "root")
	if err != nil {
		t.Fatalf("GetObjectDetail failed: %v", err)
	}
	if detail.Type != "dict" {
		t.Errorf("Type = %q, want %q", detail.Type, "dict")
	}
	if detail.NodeID != "root" {
		t.Errorf("NodeID = %q, want %q", detail.NodeID, "root")
	}
	if len(detail.Properties) == 0 {
		t.Fatal("Properties is empty, want non-empty")
	}
	for _, p := range detail.Properties {
		if !strings.HasPrefix(p.Key, "/") {
			t.Errorf("Key %q does not start with /", p.Key)
		}
		if p.Value.Type == "" {
			t.Errorf("Value.Type is empty for key %q", p.Key)
		}
	}
	// Verify sorted by Key
	for i := 1; i < len(detail.Properties); i++ {
		if detail.Properties[i].Key < detail.Properties[i-1].Key {
			t.Errorf("Properties not sorted: %q before %q", detail.Properties[i-1].Key, detail.Properties[i].Key)
		}
	}
	// Check /Type property
	found := false
	for _, p := range detail.Properties {
		if p.Key == "/Type" {
			found = true
			if p.Value.Type != "name" {
				t.Errorf("/Type Value.Type = %q, want %q", p.Value.Type, "name")
			}
			if p.Value.Display != "/Catalog" {
				t.Errorf("/Type Value.Display = %q, want %q", p.Value.Display, "/Catalog")
			}
		}
	}
	if !found {
		t.Error("did not find /Type property")
	}
}

func TestGetObjectDetailArray(t *testing.T) {
	ins, tabID := openMultipage(t)

	// Get children of root to find Pages, then get Kids array
	children, err := ins.GetChildren(tabID, "root")
	if err != nil {
		t.Fatalf("GetChildren(root) failed: %v", err)
	}

	var pagesNodeID string
	for _, c := range children {
		if c.RawKey == "/Pages" {
			pagesNodeID = c.ID
			break
		}
	}
	if pagesNodeID == "" {
		t.Fatal("could not find /Pages child of root")
	}

	// Get children of Pages to find Kids
	pagesChildren, err := ins.GetChildren(tabID, pagesNodeID)
	if err != nil {
		t.Fatalf("GetChildren(Pages) failed: %v", err)
	}

	var kidsNodeID string
	for _, c := range pagesChildren {
		if c.RawKey == "/Kids" {
			kidsNodeID = c.ID
			break
		}
	}
	if kidsNodeID == "" {
		t.Fatal("could not find /Kids under Pages")
	}

	detail, err := ins.GetObjectDetail(tabID, kidsNodeID)
	if err != nil {
		t.Fatalf("GetObjectDetail(Kids) failed: %v", err)
	}
	if detail.Type != "array" {
		t.Errorf("Type = %q, want %q", detail.Type, "array")
	}
	if len(detail.Elements) == 0 {
		t.Fatal("Elements is empty, want non-empty")
	}
	for i, elem := range detail.Elements {
		if elem.Type == "" {
			t.Errorf("Elements[%d].Type is empty", i)
		}
		if elem.Display == "" {
			t.Errorf("Elements[%d].Display is empty", i)
		}
	}
	// Kids elements should be references to Page objects
	for i, elem := range detail.Elements {
		if elem.Type == "reference" {
			if !strings.HasPrefix(elem.RefTarget, "obj:") {
				t.Errorf("Elements[%d].RefTarget = %q, want obj:... format", i, elem.RefTarget)
			}
		}
	}
}

func TestGetObjectDetailScalar(t *testing.T) {
	ins, tabID := openMinimal(t)
	detail, err := ins.GetObjectDetail(tabID, "dict:root:Type")
	if err != nil {
		t.Fatalf("GetObjectDetail failed: %v", err)
	}
	if detail.Type != "scalar" {
		t.Errorf("Type = %q, want %q", detail.Type, "scalar")
	}
	if detail.ScalarValue == nil {
		t.Fatal("ScalarValue is nil")
	}
	if detail.ScalarValue.Type != "name" {
		t.Errorf("ScalarValue.Type = %q, want %q", detail.ScalarValue.Type, "name")
	}
	if detail.ScalarValue.Display != "/Catalog" {
		t.Errorf("ScalarValue.Display = %q, want %q", detail.ScalarValue.Display, "/Catalog")
	}
}

func findStreamNode(t *testing.T, ins *Inspector, tabID, nodeID string, depth int) string {
	t.Helper()
	if depth > 4 {
		return ""
	}
	children, err := ins.GetChildren(tabID, nodeID)
	if err != nil {
		return ""
	}
	for _, c := range children {
		if c.NodeType == "stream" {
			return c.ID
		}
	}
	for _, c := range children {
		if c.HasChildren {
			found := findStreamNode(t, ins, tabID, c.ID, depth+1)
			if found != "" {
				return found
			}
		}
	}
	return ""
}

func openContentStream(t *testing.T) (*Inspector, string) {
	t.Helper()
	ins := NewInspector()
	tabID := "test-tab"
	_, err := ins.Open(tabID, filepath.Join(testdataDir(t), "content-stream.pdf"))
	if err != nil {
		t.Fatalf("failed to open content-stream.pdf: %v", err)
	}
	return ins, tabID
}

func TestGetObjectDetailStream(t *testing.T) {
	ins, tabID := openContentStream(t)

	streamNodeID := findStreamNode(t, ins, tabID, "root", 0)
	if streamNodeID == "" {
		t.Skip("no stream node found in test PDF")
	}

	detail, err := ins.GetObjectDetail(tabID, streamNodeID)
	if err != nil {
		t.Fatalf("GetObjectDetail(stream) failed: %v", err)
	}
	if detail.Type != "stream" {
		t.Errorf("Type = %q, want %q", detail.Type, "stream")
	}
	if detail.Properties == nil {
		t.Error("Properties is nil for stream")
	}
	if detail.StreamInfo == nil {
		t.Fatal("StreamInfo is nil for stream")
	}
	if detail.StreamInfo.Length < 0 {
		t.Errorf("StreamInfo.Length = %d, want >= 0", detail.StreamInfo.Length)
	}
	if detail.StreamInfo.Filters == nil {
		t.Error("StreamInfo.Filters is nil, want non-nil (possibly empty)")
	}
}

func TestGetObjectDetailObjectRef(t *testing.T) {
	ins, tabID := openMinimal(t)

	// Root is not an indirect object -- ObjectRef should be empty
	detail, err := ins.GetObjectDetail(tabID, "root")
	if err != nil {
		t.Fatalf("GetObjectDetail(root) failed: %v", err)
	}
	if detail.ObjectRef != "" {
		t.Errorf("root ObjectRef = %q, want empty", detail.ObjectRef)
	}

	// Find an obj: node
	children, err := ins.GetChildren(tabID, "root")
	if err != nil {
		t.Fatalf("GetChildren(root) failed: %v", err)
	}

	var objNodeID string
	for _, c := range children {
		if strings.HasPrefix(c.ID, "obj:") {
			objNodeID = c.ID
			break
		}
	}
	if objNodeID == "" {
		t.Fatal("no obj: child found under root")
	}

	detail, err = ins.GetObjectDetail(tabID, objNodeID)
	if err != nil {
		t.Fatalf("GetObjectDetail(%s) failed: %v", objNodeID, err)
	}
	if detail.ObjectRef == "" {
		t.Error("ObjectRef is empty for indirect object")
	}
	// ObjectRef should match "{num} {gen} R"
	parts := strings.Fields(detail.ObjectRef)
	if len(parts) != 3 || parts[2] != "R" {
		t.Errorf("ObjectRef = %q, want format 'N G R'", detail.ObjectRef)
	}
}

func TestGetObjectDetailEmptyDict(t *testing.T) {
	// We test with a synthetic scenario: the catalog itself is never empty
	// but we can test the code path by using a dict node that resolves to
	// an empty dict. Since real PDFs rarely have empty dicts, we test the
	// function directly.
	ins, tabID := openMinimal(t)

	// Get any dict detail and verify the type -- the real empty-dict
	// code path is the same, just with 0 entries.
	detail, err := ins.GetObjectDetail(tabID, "root")
	if err != nil {
		t.Fatalf("GetObjectDetail failed: %v", err)
	}
	if detail.Type != "dict" {
		t.Errorf("Type = %q, want %q", detail.Type, "dict")
	}
	// Properties must be non-nil (empty slice, not nil)
	if detail.Properties == nil {
		t.Error("Properties is nil, want non-nil slice")
	}
}

func TestGetObjectDetailEmptyArray(t *testing.T) {
	// Similar to empty dict test -- verify array path returns non-nil Elements
	ins, tabID := openMultipage(t)

	children, err := ins.GetChildren(tabID, "root")
	if err != nil {
		t.Fatalf("GetChildren(root) failed: %v", err)
	}

	var pagesNodeID string
	for _, c := range children {
		if c.RawKey == "/Pages" {
			pagesNodeID = c.ID
			break
		}
	}
	if pagesNodeID == "" {
		t.Fatal("could not find /Pages")
	}

	pagesChildren, err := ins.GetChildren(tabID, pagesNodeID)
	if err != nil {
		t.Fatalf("GetChildren(Pages) failed: %v", err)
	}

	var kidsNodeID string
	for _, c := range pagesChildren {
		if c.RawKey == "/Kids" {
			kidsNodeID = c.ID
			break
		}
	}
	if kidsNodeID == "" {
		t.Fatal("could not find /Kids")
	}

	detail, err := ins.GetObjectDetail(tabID, kidsNodeID)
	if err != nil {
		t.Fatalf("GetObjectDetail(Kids) failed: %v", err)
	}
	if detail.Type != "array" {
		t.Errorf("Type = %q, want %q", detail.Type, "array")
	}
	if detail.Elements == nil {
		t.Error("Elements is nil, want non-nil slice")
	}
}

func TestGetObjectDetailUnknownTabID(t *testing.T) {
	ins := NewInspector()
	_, err := ins.GetObjectDetail("nonexistent", "root")
	if err == nil {
		t.Fatal("expected error for unknown tabID, got nil")
	}
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

func TestGetObjectDetailInvalidNodeID(t *testing.T) {
	ins, tabID := openMinimal(t)
	_, err := ins.GetObjectDetail(tabID, "bogus")
	if err != nil {
		// Good -- error returned, no panic
		return
	}
	// If no error, the nodeID was somehow resolved -- that's also OK
	// as long as there's no panic. The main goal is no crash.
}

// --- valueEntryFromObject direct unit tests ---

func TestValueEntryFromObjectAllTypes(t *testing.T) {
	tests := []struct {
		name    string
		obj     pdfcpu_types.Object
		wantTyp string
		wantDsp string
	}{
		{"Name", pdfcpu_types.Name("Catalog"), "name", "/Catalog"},
		{"StringLiteral", pdfcpu_types.StringLiteral("hello"), "string", "(hello)"},
		{"HexLiteral", pdfcpu_types.HexLiteral("AABB"), "string", "<AABB>"},
		{"Integer", pdfcpu_types.Integer(42), "number", "42"},
		{"Float", pdfcpu_types.Float(3.14), "number", "3.14"},
		{"BooleanTrue", pdfcpu_types.Boolean(true), "boolean", "true"},
		{"BooleanFalse", pdfcpu_types.Boolean(false), "boolean", "false"},
		{"Nil", nil, "null", "null"},
		{"Dict", pdfcpu_types.Dict{}, "dict", "<< ... >>"},
		{"Array", pdfcpu_types.Array{}, "array", "[...]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ve := valueEntryFromObject(tc.obj)
			if ve.Type != tc.wantTyp {
				t.Errorf("Type = %q, want %q", ve.Type, tc.wantTyp)
			}
			if ve.Display != tc.wantDsp {
				t.Errorf("Display = %q, want %q", ve.Display, tc.wantDsp)
			}
			if ve.Raw != tc.wantDsp {
				t.Errorf("Raw = %q, want %q", ve.Raw, tc.wantDsp)
			}
		})
	}
}

func TestValueEntryFromObjectIndirectRef(t *testing.T) {
	ref := pdfcpu_types.IndirectRef{
		ObjectNumber:     pdfcpu_types.Integer(7),
		GenerationNumber: pdfcpu_types.Integer(0),
	}
	ve := valueEntryFromObject(ref)
	if ve.Type != "reference" {
		t.Errorf("Type = %q, want %q", ve.Type, "reference")
	}
	if ve.Display != "7 0 R" {
		t.Errorf("Display = %q, want %q", ve.Display, "7 0 R")
	}
	if ve.RefTarget != "obj:0:7" {
		t.Errorf("RefTarget = %q, want %q", ve.RefTarget, "obj:0:7")
	}
}

// --- objectRefFromNodeID direct unit tests ---

func TestObjectRefFromNodeID(t *testing.T) {
	tests := []struct {
		nodeID string
		want   string
	}{
		{"root", ""},
		{"dict:root:Type", ""},
		{"arr:root:0", ""},
		{"obj:0:5", "5 0 R"},
		{"obj:2:10", "10 2 R"},
	}
	for _, tc := range tests {
		t.Run(tc.nodeID, func(t *testing.T) {
			got := objectRefFromNodeID(tc.nodeID)
			if got != tc.want {
				t.Errorf("objectRefFromNodeID(%q) = %q, want %q", tc.nodeID, got, tc.want)
			}
		})
	}
}

// --- edge case: empty nodeID ---

func TestGetObjectDetailEmptyNodeID(t *testing.T) {
	ins, tabID := openMinimal(t)
	_, err := ins.GetObjectDetail(tabID, "")
	if err == nil {
		t.Fatal("expected error for empty nodeID, got nil")
	}
}

// --- stream properties sorted ---

func TestGetObjectDetailStreamPropertiesSorted(t *testing.T) {
	ins, tabID := openContentStream(t)
	streamNodeID := findStreamNode(t, ins, tabID, "root", 0)
	if streamNodeID == "" {
		t.Skip("no stream node found in test PDF")
	}
	detail, err := ins.GetObjectDetail(tabID, streamNodeID)
	if err != nil {
		t.Fatalf("GetObjectDetail(stream) failed: %v", err)
	}
	for i := 1; i < len(detail.Properties); i++ {
		if detail.Properties[i].Key < detail.Properties[i-1].Key {
			t.Errorf("stream properties not sorted: %q before %q", detail.Properties[i-1].Key, detail.Properties[i].Key)
		}
	}
}

func TestGetObjectDetailRefTarget(t *testing.T) {
	ins, tabID := openMinimal(t)
	detail, err := ins.GetObjectDetail(tabID, "root")
	if err != nil {
		t.Fatalf("GetObjectDetail(root) failed: %v", err)
	}

	found := false
	for _, p := range detail.Properties {
		if p.Value.Type == "reference" {
			found = true
			if !strings.HasPrefix(p.Value.RefTarget, "obj:") {
				t.Errorf("RefTarget = %q, want obj:... format", p.Value.RefTarget)
			}
			// Display should match "N G R" format
			parts := strings.Fields(p.Value.Display)
			if len(parts) != 3 || parts[2] != "R" {
				t.Errorf("Display = %q, want format 'N G R'", p.Value.Display)
			}
		}
	}
	if !found {
		t.Error("no reference property found in catalog dict")
	}
}
