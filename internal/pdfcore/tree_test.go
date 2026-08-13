package pdfcore

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	pdfcpu_types "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

func openMinimal(t *testing.T) (*Inspector, string) {
	t.Helper()
	ins := NewInspector()
	tabID := "test-tab"
	_, err := ins.Open(tabID, filepath.Join(testdataDir(t), "minimal.pdf"))
	if err != nil {
		t.Fatalf("failed to open minimal.pdf: %v", err)
	}
	return ins, tabID
}

func openMultipage(t *testing.T) (*Inspector, string) {
	t.Helper()
	ins := NewInspector()
	tabID := "test-tab"
	_, err := ins.Open(tabID, filepath.Join(testdataDir(t), "multipage.pdf"))
	if err != nil {
		t.Fatalf("failed to open multipage.pdf: %v", err)
	}
	return ins, tabID
}

func TestGetTreeRoot(t *testing.T) {
	ins, tabID := openMinimal(t)
	root, err := ins.GetTreeRoot(tabID)
	if err != nil {
		t.Fatalf("GetTreeRoot returned error: %v", err)
	}
	if root.ID != "root" {
		t.Errorf("ID = %q, want %q", root.ID, "root")
	}
	if root.Label != "Catalog" {
		t.Errorf("Label = %q, want %q", root.Label, "Catalog")
	}
	if !root.HasChildren {
		t.Error("HasChildren = false, want true")
	}
	if root.NodeType != "dict" {
		t.Errorf("NodeType = %q, want %q", root.NodeType, "dict")
	}
	if root.IconHint != "catalog" {
		t.Errorf("IconHint = %q, want %q", root.IconHint, "catalog")
	}
	if root.ChildCount <= 0 {
		t.Errorf("ChildCount = %d, want > 0", root.ChildCount)
	}
}

func TestGetChildrenRoot(t *testing.T) {
	ins, tabID := openMinimal(t)
	children, err := ins.GetChildren(tabID, "root")
	if err != nil {
		t.Fatalf("GetChildren returned error: %v", err)
	}
	if len(children) == 0 {
		t.Fatal("GetChildren returned empty slice for root")
	}

	hasObjPrefix := false
	hasType := false
	hasPages := false
	for _, c := range children {
		if strings.HasPrefix(c.ID, "obj:") {
			hasObjPrefix = true
		}
		if c.RawKey == "/Type" {
			hasType = true
		}
		if c.RawKey == "/Pages" {
			hasPages = true
		}
		if c.Label == "" {
			t.Errorf("child %q has empty Label", c.ID)
		}
		if c.NodeType == "" {
			t.Errorf("child %q has empty NodeType", c.ID)
		}
		if c.IconHint == "" {
			t.Errorf("child %q has empty IconHint", c.ID)
		}
	}
	if !hasObjPrefix {
		t.Error("expected at least one child with obj: prefix ID")
	}
	if !hasType {
		t.Error("expected a child with RawKey /Type")
	}
	if !hasPages {
		t.Error("expected a child with RawKey /Pages")
	}
}

func TestGetChildrenDictNode(t *testing.T) {
	ins, tabID := openMinimal(t)

	// Get root children to find /Pages ref
	rootChildren, err := ins.GetChildren(tabID, "root")
	if err != nil {
		t.Fatalf("GetChildren(root) error: %v", err)
	}

	var pagesID string
	for _, c := range rootChildren {
		if c.RawKey == "/Pages" {
			pagesID = c.ID
			break
		}
	}
	if pagesID == "" {
		t.Fatal("could not find /Pages child in root")
	}

	// Get children of the Pages dict
	pagesChildren, err := ins.GetChildren(tabID, pagesID)
	if err != nil {
		t.Fatalf("GetChildren(%q) error: %v", pagesID, err)
	}

	hasDictPrefix := false
	for _, c := range pagesChildren {
		if strings.HasPrefix(c.ID, "dict:") {
			hasDictPrefix = true
			// Verify bare key (no slash in ID)
			parts := strings.Split(c.ID, ":")
			lastPart := parts[len(parts)-1]
			if strings.HasPrefix(lastPart, "/") {
				t.Errorf("dict ID %q contains slash in key", c.ID)
			}
		}
		if strings.HasPrefix(c.ID, "dict:") || strings.HasPrefix(c.ID, "obj:") {
			if strings.HasPrefix(c.RawKey, "/") || strings.HasPrefix(c.RawKey, "[") {
				// good - dict entries have "/" prefix, array entries have "[" prefix
			} else {
				t.Errorf("child %q has unexpected RawKey %q", c.ID, c.RawKey)
			}
		}
	}
	if !hasDictPrefix {
		t.Error("expected at least one child with dict: prefix ID")
	}
}

func TestGetChildrenArrayNode(t *testing.T) {
	ins, tabID := openMultipage(t)

	// Navigate: root -> /Pages -> /Kids (array)
	rootChildren, err := ins.GetChildren(tabID, "root")
	if err != nil {
		t.Fatalf("GetChildren(root) error: %v", err)
	}

	var pagesID string
	for _, c := range rootChildren {
		if c.RawKey == "/Pages" {
			pagesID = c.ID
			break
		}
	}
	if pagesID == "" {
		t.Fatal("could not find /Pages child")
	}

	pagesChildren, err := ins.GetChildren(tabID, pagesID)
	if err != nil {
		t.Fatalf("GetChildren(%q) error: %v", pagesID, err)
	}

	var kidsID string
	for _, c := range pagesChildren {
		if c.RawKey == "/Kids" {
			kidsID = c.ID
			break
		}
	}
	if kidsID == "" {
		t.Fatal("could not find /Kids child in Pages dict")
	}

	// /Kids is an array - if it's a direct array, its ID starts with dict:
	// Get its children
	kidsChildren, err := ins.GetChildren(tabID, kidsID)
	if err != nil {
		t.Fatalf("GetChildren(%q) error: %v", kidsID, err)
	}

	if len(kidsChildren) < 2 {
		t.Fatalf("expected at least 2 children in /Kids array, got %d", len(kidsChildren))
	}

	for _, c := range kidsChildren {
		// Array elements that are IndirectRefs get obj: IDs, others get arr: IDs
		if !strings.HasPrefix(c.ID, "arr:") && !strings.HasPrefix(c.ID, "obj:") {
			t.Errorf("array child %q has unexpected ID prefix (expected arr: or obj:)", c.ID)
		}
		if !strings.HasPrefix(c.RawKey, "[") {
			t.Errorf("array child %q has RawKey %q, expected [index] format", c.ID, c.RawKey)
		}
	}
}

func TestErrorNodeCreation(t *testing.T) {
	ins, tabID := openMinimal(t)

	// Invalid node ID format should return error
	_, err := ins.GetChildren(tabID, "bogus:invalid")
	if err == nil {
		t.Fatal("expected error for invalid node ID, got nil")
	}
}

func TestErrorNodeLabel(t *testing.T) {
	node := makeErrorNode("test-id", "/Key", errors.New("test error"))
	if !strings.HasPrefix(node.Label, "Error:") {
		t.Errorf("error node Label = %q, want prefix 'Error:'", node.Label)
	}
	if node.Error == "" {
		t.Error("error node Error field is empty")
	}
	if node.NodeType != "scalar" {
		t.Errorf("error node NodeType = %q, want 'scalar'", node.NodeType)
	}
}

func TestSemanticLabelPages(t *testing.T) {
	ins, tabID := openMinimal(t)
	children, err := ins.GetChildren(tabID, "root")
	if err != nil {
		t.Fatalf("GetChildren error: %v", err)
	}
	for _, c := range children {
		if c.RawKey == "/Pages" {
			if c.Label != "Pages" {
				t.Errorf("Pages Label = %q, want %q", c.Label, "Pages")
			}
			if c.IconHint != "pages" {
				t.Errorf("Pages IconHint = %q, want %q", c.IconHint, "pages")
			}
			return
		}
	}
	t.Error("did not find /Pages child in root children")
}

func TestSemanticLabelFont(t *testing.T) {
	// Test semantic labeling with synthetic pdfcpu objects
	t.Run("font_with_basefont", func(t *testing.T) {
		fontDict := pdfcpu_types.Dict{
			"Type":     pdfcpu_types.Name("Font"),
			"BaseFont": pdfcpu_types.Name("Helvetica"),
		}
		label := semanticLabel("Font", fontDict)
		if label != "Font: Helvetica" {
			t.Errorf("label = %q, want %q", label, "Font: Helvetica")
		}
		hint := iconHint("Font", "dict", fontDict)
		if hint != "font" {
			t.Errorf("iconHint = %q, want %q", hint, "font")
		}
	})

	t.Run("font_without_basefont", func(t *testing.T) {
		fontDict := pdfcpu_types.Dict{
			"Type": pdfcpu_types.Name("Font"),
		}
		label := semanticLabel("Font", fontDict)
		if label != "Font" {
			t.Errorf("label = %q, want %q", label, "Font")
		}
	})

	t.Run("font_via_type_detection", func(t *testing.T) {
		fontDict := pdfcpu_types.Dict{
			"Type":     pdfcpu_types.Name("Font"),
			"BaseFont": pdfcpu_types.Name("Times-Roman"),
		}
		// When bareKey is not "Font", detect via /Type
		label := semanticLabel("F1", fontDict)
		if label != "Font: Times-Roman" {
			t.Errorf("label = %q, want %q", label, "Font: Times-Roman")
		}
	})
}

func TestNodeTypeAssignment(t *testing.T) {
	ins, tabID := openMinimal(t)
	children, err := ins.GetChildren(tabID, "root")
	if err != nil {
		t.Fatalf("GetChildren error: %v", err)
	}

	validTypes := map[string]bool{
		"dict": true, "array": true, "stream": true, "ref": true, "scalar": true,
	}

	foundScalar := false
	for _, c := range children {
		if !validTypes[c.NodeType] {
			t.Errorf("child %q has invalid NodeType %q", c.ID, c.NodeType)
		}
		if c.NodeType == "scalar" {
			foundScalar = true
			if c.HasChildren {
				t.Errorf("scalar node %q has HasChildren=true", c.ID)
			}
		}
	}
	if !foundScalar {
		t.Error("expected at least one scalar child in root (e.g., /Type)")
	}
}

func TestNodeIDRoundTrip(t *testing.T) {
	tests := []struct {
		nodeID   string
		wantKind string
		wantPID  string
		wantLast string
	}{
		{"root", "root", "", ""},
		{"obj:0:5", "obj", "0", "5"},
		{"dict:root:Pages", "dict", "root", "Pages"},
		{"dict:obj:0:5:Type", "dict", "obj:0:5", "Type"},
		{"arr:obj:0:12:3", "arr", "obj:0:12", "3"},
	}
	for _, tt := range tests {
		t.Run(tt.nodeID, func(t *testing.T) {
			kind, parentID, lastPart := parseNodeID(tt.nodeID)
			if kind != tt.wantKind {
				t.Errorf("kind = %q, want %q", kind, tt.wantKind)
			}
			if parentID != tt.wantPID {
				t.Errorf("parentID = %q, want %q", parentID, tt.wantPID)
			}
			if lastPart != tt.wantLast {
				t.Errorf("lastPart = %q, want %q", lastPart, tt.wantLast)
			}
		})
	}
}

func TestIconHintXObjectImage(t *testing.T) {
	// Test with synthetic pdfcpu objects
	imgDict := pdfcpu_types.Dict{
		"Type":    pdfcpu_types.Name("XObject"),
		"Subtype": pdfcpu_types.Name("Image"),
	}
	hint := iconHint("", "dict", imgDict)
	if hint != "image" {
		t.Errorf("iconHint for XObject/Image = %q, want %q", hint, "image")
	}

	formDict := pdfcpu_types.Dict{
		"Type":    pdfcpu_types.Name("XObject"),
		"Subtype": pdfcpu_types.Name("Form"),
	}
	hint = iconHint("", "dict", formDict)
	if hint == "image" {
		t.Errorf("iconHint for XObject/Form should not be 'image', got %q", hint)
	}
}

func TestGetChildrenEmptyDict(t *testing.T) {
	// Use buildDictChildren with an empty dict directly
	nodes := buildDictChildren(nil, "test-parent", pdfcpu_types.Dict{})
	if nodes == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 children, got %d", len(nodes))
	}
}

func TestGetChildrenEmptyArray(t *testing.T) {
	nodes := buildArrayChildren(nil, "test-parent", pdfcpu_types.Array{})
	if nodes == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 children, got %d", len(nodes))
	}
}

func TestGetTreeRootUnknownTabID(t *testing.T) {
	ins := NewInspector()
	_, err := ins.GetTreeRoot("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown tabID, got nil")
	}
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

func TestGetChildrenUnknownTabID(t *testing.T) {
	ins := NewInspector()
	_, err := ins.GetChildren("nonexistent", "root")
	if err == nil {
		t.Fatal("expected error for unknown tabID, got nil")
	}
	if !errors.Is(err, ErrDocumentNotFound) {
		t.Errorf("expected ErrDocumentNotFound, got %v", err)
	}
}

func TestGetChildrenInvalidNodeID(t *testing.T) {
	ins, tabID := openMinimal(t)

	tests := []string{"bogus", "obj:", "obj:abc:def"}
	for _, nodeID := range tests {
		t.Run(nodeID, func(t *testing.T) {
			_, err := ins.GetChildren(tabID, nodeID)
			if err == nil {
				t.Errorf("expected error for invalid nodeID %q, got nil", nodeID)
			}
		})
	}
}

func TestScalarLeafNodes(t *testing.T) {
	ins, tabID := openMinimal(t)
	children, err := ins.GetChildren(tabID, "root")
	if err != nil {
		t.Fatalf("GetChildren error: %v", err)
	}

	found := false
	for _, c := range children {
		if c.RawKey == "/Type" {
			found = true
			if c.HasChildren {
				t.Errorf("/Type HasChildren = true, want false")
			}
			if c.NodeType != "scalar" {
				t.Errorf("/Type NodeType = %q, want 'scalar'", c.NodeType)
			}
			if c.ValueType != "name" {
				t.Errorf("/Type ValueType = %q, want 'name'", c.ValueType)
			}
			break
		}
	}
	if !found {
		t.Error("did not find /Type scalar in root children")
	}
}

// --- Gap coverage tests below ---

// TestErrorNodeSiblingSurvival verifies: when one dict entry causes a panic,
// sibling nodes are still returned.
func TestErrorNodeSiblingSurvival(t *testing.T) {
	// Build a dict where one entry is a well-known type and another triggers
	// the per-child safeCall. We can't easily inject a panic, but we can test
	// buildDictChildren directly with a mix of good and nil entries to confirm
	// all entries produce nodes and none are dropped.
	d := pdfcpu_types.Dict{
		"GoodName":  pdfcpu_types.Name("Hello"),
		"GoodInt":   pdfcpu_types.Integer(42),
		"NullEntry": nil,
	}
	nodes := buildDictChildren(nil, "test-parent", d)
	if len(nodes) != 3 {
		t.Fatalf("expected 3 children, got %d", len(nodes))
	}
	// Verify all 3 nodes exist with proper IDs
	ids := map[string]bool{}
	for _, n := range nodes {
		ids[n.ID] = true
	}
	for _, key := range []string{"GoodName", "GoodInt", "NullEntry"} {
		expected := "dict:test-parent:" + key
		if !ids[expected] {
			t.Errorf("missing child node with ID %q", expected)
		}
	}
}

// TestBuildChildrenStreamDict verifies stream dicts produce dict children.
func TestBuildChildrenStreamDict(t *testing.T) {
	sd := pdfcpu_types.StreamDict{
		Dict: pdfcpu_types.Dict{
			"Length": pdfcpu_types.Integer(100),
			"Filter": pdfcpu_types.Name("FlateDecode"),
		},
	}
	nodes := buildChildrenDepth(nil, "parent", sd, 0)
	if len(nodes) != 2 {
		t.Fatalf("expected 2 children from StreamDict, got %d", len(nodes))
	}
}

// TestBuildChildrenScalar verifies that building children of a scalar returns empty slice.
func TestBuildChildrenScalar(t *testing.T) {
	nodes := buildChildren(nil, "parent", pdfcpu_types.Name("Hello"))
	if nodes == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 children for scalar, got %d", len(nodes))
	}
}

// TestBuildChildrenNil verifies that building children of nil returns empty slice.
func TestBuildChildrenNil(t *testing.T) {
	nodes := buildChildren(nil, "parent", nil)
	if nodes == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(nodes) != 0 {
		t.Errorf("expected 0 children for nil, got %d", len(nodes))
	}
}

// TestClassifyObjectAllTypes verifies classifyObject for every pdfcpu type.
func TestClassifyObjectAllTypes(t *testing.T) {
	tests := []struct {
		name         string
		obj          pdfcpu_types.Object
		wantNodeType string
		wantValType  string
		wantHasKids  bool
	}{
		{"Dict", pdfcpu_types.Dict{"A": pdfcpu_types.Integer(1)}, "dict", "", true},
		{"Array", pdfcpu_types.Array{pdfcpu_types.Integer(1)}, "array", "", true},
		{"StreamDict", pdfcpu_types.StreamDict{Dict: pdfcpu_types.Dict{}}, "stream", "", true},
		{"ObjectStreamDict", pdfcpu_types.ObjectStreamDict{StreamDict: pdfcpu_types.StreamDict{Dict: pdfcpu_types.Dict{}}}, "stream", "", true},
		{"XRefStreamDict", pdfcpu_types.XRefStreamDict{StreamDict: pdfcpu_types.StreamDict{Dict: pdfcpu_types.Dict{}}}, "stream", "", true},
		{"IndirectRef", pdfcpu_types.IndirectRef{ObjectNumber: pdfcpu_types.Integer(1), GenerationNumber: pdfcpu_types.Integer(0)}, "ref", "reference", true},
		{"Name", pdfcpu_types.Name("Test"), "scalar", "name", false},
		{"StringLiteral", pdfcpu_types.StringLiteral("hello"), "scalar", "string", false},
		{"HexLiteral", pdfcpu_types.HexLiteral("48656C6C6F"), "scalar", "string", false},
		{"Integer", pdfcpu_types.Integer(42), "scalar", "number", false},
		{"Float", pdfcpu_types.Float(3.14), "scalar", "number", false},
		{"Boolean", pdfcpu_types.Boolean(true), "scalar", "boolean", false},
		{"nil", nil, "scalar", "null", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeType, valType, hasKids, _ := classifyObject(tt.obj)
			if nodeType != tt.wantNodeType {
				t.Errorf("nodeType = %q, want %q", nodeType, tt.wantNodeType)
			}
			if valType != tt.wantValType {
				t.Errorf("valueType = %q, want %q", valType, tt.wantValType)
			}
			if hasKids != tt.wantHasKids {
				t.Errorf("hasChildren = %v, want %v", hasKids, tt.wantHasKids)
			}
		})
	}
}

// TestScalarDisplayAllTypes verifies scalarDisplay for each scalar pdfcpu type.
func TestScalarDisplayAllTypes(t *testing.T) {
	tests := []struct {
		name string
		obj  pdfcpu_types.Object
		want string
	}{
		{"Name", pdfcpu_types.Name("Helvetica"), "/Helvetica"},
		{"StringLiteral", pdfcpu_types.StringLiteral("hello"), "(hello)"},
		{"HexLiteral", pdfcpu_types.HexLiteral("ABCD"), "<ABCD>"},
		{"Integer", pdfcpu_types.Integer(42), "42"},
		{"Float", pdfcpu_types.Float(3.14), "3.14"},
		{"BooleanTrue", pdfcpu_types.Boolean(true), "true"},
		{"BooleanFalse", pdfcpu_types.Boolean(false), "false"},
		{"nil", nil, "null"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scalarDisplay(tt.obj)
			if got != tt.want {
				t.Errorf("scalarDisplay = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestIconHintStreamType verifies stream nodes get iconHint "stream".
func TestIconHintStreamType(t *testing.T) {
	// A stream dict without a recognized Type/Subtype should get "stream" from nodeType.
	sd := pdfcpu_types.Dict{
		"Length": pdfcpu_types.Integer(100),
	}
	hint := iconHint("", "stream", sd)
	if hint != "stream" {
		t.Errorf("iconHint for stream nodeType = %q, want %q", hint, "stream")
	}
}

// TestIconHintContents verifies /Contents gets iconHint "stream".
func TestIconHintContents(t *testing.T) {
	hint := iconHint("Contents", "ref", nil)
	if hint != "stream" {
		t.Errorf("iconHint for Contents = %q, want %q", hint, "stream")
	}
}

// TestIconHintPagesVsPage verifies intermediate /Pages and leaf /Page get
// distinct iconHints so the frontend can render different icons.
func TestIconHintPagesVsPage(t *testing.T) {
	pagesDict := pdfcpu_types.Dict{
		"Type":  pdfcpu_types.Name("Pages"),
		"Count": pdfcpu_types.Integer(10),
	}
	pageDict := pdfcpu_types.Dict{
		"Type": pdfcpu_types.Name("Page"),
	}

	// bareKey-driven path: catalog's /Pages reference and explicit /Pages key.
	if hint := iconHint("Pages", "dict", pagesDict); hint != "pages" {
		t.Errorf("iconHint(\"Pages\", ...) = %q, want %q", hint, "pages")
	}
	if hint := iconHint("Page", "dict", pageDict); hint != "page" {
		t.Errorf("iconHint(\"Page\", ...) = %q, want %q", hint, "page")
	}

	// Type-driven path: array-element resolution where bareKey == "".
	if hint := iconHint("", "dict", pagesDict); hint != "pages" {
		t.Errorf("iconHint(empty, /Type=Pages) = %q, want %q", hint, "pages")
	}
	if hint := iconHint("", "dict", pageDict); hint != "page" {
		t.Errorf("iconHint(empty, /Type=Page) = %q, want %q", hint, "page")
	}
}

// TestSemanticLabelContents verifies /Contents returns "Contents".
func TestSemanticLabelContents(t *testing.T) {
	label := semanticLabel("Contents", nil)
	if label != "Contents" {
		t.Errorf("semanticLabel(Contents) = %q, want %q", label, "Contents")
	}
}

// TestSemanticLabelResources verifies /Resources returns "Resources".
func TestSemanticLabelResources(t *testing.T) {
	label := semanticLabel("Resources", pdfcpu_types.Dict{})
	if label != "Resources" {
		t.Errorf("semanticLabel(Resources) = %q, want %q", label, "Resources")
	}
}

// TestSemanticLabelBoxTypes verifies box keys return their bare key.
func TestSemanticLabelBoxTypes(t *testing.T) {
	for _, key := range []string{"MediaBox", "CropBox", "BleedBox", "TrimBox", "ArtBox"} {
		t.Run(key, func(t *testing.T) {
			label := semanticLabel(key, pdfcpu_types.Array{})
			if label != key {
				t.Errorf("semanticLabel(%s) = %q, want %q", key, label, key)
			}
		})
	}
}

// TestSemanticLabelXObject verifies /XObject returns "XObject".
func TestSemanticLabelXObject(t *testing.T) {
	label := semanticLabel("XObject", pdfcpu_types.Dict{})
	if label != "XObject" {
		t.Errorf("semanticLabel(XObject) = %q, want %q", label, "XObject")
	}
}

// TestSemanticLabelImageSubtype verifies dict with Subtype=Image returns "Image".
func TestSemanticLabelImageSubtype(t *testing.T) {
	d := pdfcpu_types.Dict{
		"Subtype": pdfcpu_types.Name("Image"),
	}
	label := semanticLabel("", d)
	if label != "Image" {
		t.Errorf("semanticLabel for Image subtype = %q, want %q", label, "Image")
	}
}

// TestSemanticLabelFormXObject verifies dict with Subtype=Form returns "Form XObject".
func TestSemanticLabelFormXObject(t *testing.T) {
	d := pdfcpu_types.Dict{
		"Subtype": pdfcpu_types.Name("Form"),
	}
	label := semanticLabel("", d)
	if label != "Form XObject" {
		t.Errorf("semanticLabel for Form subtype = %q, want %q", label, "Form XObject")
	}
}

// TestSemanticLabelCatalogViaType verifies dict with Type=Catalog returns "Catalog".
func TestSemanticLabelCatalogViaType(t *testing.T) {
	d := pdfcpu_types.Dict{
		"Type": pdfcpu_types.Name("Catalog"),
	}
	label := semanticLabel("", d)
	if label != "Catalog" {
		t.Errorf("semanticLabel for Catalog type = %q, want %q", label, "Catalog")
	}
}

// TestParseNodeIDDeeplyNested verifies parseNodeID with deeper nesting.
func TestParseNodeIDDeeplyNested(t *testing.T) {
	tests := []struct {
		nodeID   string
		wantKind string
		wantPID  string
		wantLast string
	}{
		{"dict:dict:root:Pages:Type", "dict", "dict:root:Pages", "Type"},
		{"arr:dict:root:Kids:2", "arr", "dict:root:Kids", "2"},
		{"dict:arr:obj:0:12:3:Length", "dict", "arr:obj:0:12:3", "Length"},
	}
	for _, tt := range tests {
		t.Run(tt.nodeID, func(t *testing.T) {
			kind, parentID, lastPart := parseNodeID(tt.nodeID)
			if kind != tt.wantKind {
				t.Errorf("kind = %q, want %q", kind, tt.wantKind)
			}
			if parentID != tt.wantPID {
				t.Errorf("parentID = %q, want %q", parentID, tt.wantPID)
			}
			if lastPart != tt.wantLast {
				t.Errorf("lastPart = %q, want %q", lastPart, tt.wantLast)
			}
		})
	}
}

// TestBuildTreeNodeFallbackLabel verifies buildTreeNode uses rawKey as label
// when semanticLabel returns empty (container with no bareKey).
func TestBuildTreeNodeFallbackLabel(t *testing.T) {
	node := buildTreeNode("arr:parent:0", "[0]", "", pdfcpu_types.Dict{"A": pdfcpu_types.Integer(1)})
	if node.Label != "[0]" {
		t.Errorf("Label = %q, want %q (fallback to rawKey)", node.Label, "[0]")
	}
	if node.NodeType != "dict" {
		t.Errorf("NodeType = %q, want %q", node.NodeType, "dict")
	}
	if !node.HasChildren {
		t.Error("HasChildren = false, want true for dict")
	}
}

// TestGetChildrenReturnsPageChildren verifies tree traversal to individual page level.
func TestGetChildrenReturnsPageChildren(t *testing.T) {
	ins, tabID := openMinimal(t)

	// Navigate root -> /Pages -> resolve -> /Kids -> first page
	rootChildren, err := ins.GetChildren(tabID, "root")
	if err != nil {
		t.Fatalf("GetChildren(root) error: %v", err)
	}

	var pagesID string
	for _, c := range rootChildren {
		if c.RawKey == "/Pages" {
			pagesID = c.ID
			break
		}
	}
	if pagesID == "" {
		t.Fatal("could not find /Pages child")
	}

	pagesChildren, err := ins.GetChildren(tabID, pagesID)
	if err != nil {
		t.Fatalf("GetChildren(%q) error: %v", pagesID, err)
	}

	// Find a child that is a page (dict with /Type, /MediaBox, etc.)
	// In minimal.pdf, /Kids should have at least one page ref
	var kidsID string
	for _, c := range pagesChildren {
		if c.RawKey == "/Kids" {
			kidsID = c.ID
			break
		}
	}
	if kidsID == "" {
		t.Fatal("could not find /Kids in Pages dict")
	}

	kidsChildren, err := ins.GetChildren(tabID, kidsID)
	if err != nil {
		t.Fatalf("GetChildren(%q) error: %v", kidsID, err)
	}
	if len(kidsChildren) == 0 {
		t.Fatal("expected at least one page in /Kids")
	}

	// Expand the first page to verify 3-level deep traversal
	pageID := kidsChildren[0].ID
	pageChildren, err := ins.GetChildren(tabID, pageID)
	if err != nil {
		t.Fatalf("GetChildren(%q) error: %v", pageID, err)
	}
	if len(pageChildren) == 0 {
		t.Fatal("expected page to have children (e.g., /Type, /MediaBox)")
	}

	// Verify page has expected entries
	hasType := false
	for _, c := range pageChildren {
		if c.RawKey == "/Type" {
			hasType = true
		}
	}
	if !hasType {
		t.Error("expected page dict to have /Type child")
	}
}

// TestBuildArrayChildrenMixedTypes verifies array with mixed scalar types.
func TestBuildArrayChildrenMixedTypes(t *testing.T) {
	arr := pdfcpu_types.Array{
		pdfcpu_types.Integer(100),
		pdfcpu_types.Float(200.5),
		pdfcpu_types.Name("Test"),
		pdfcpu_types.Boolean(true),
		nil,
	}
	nodes := buildArrayChildren(nil, "parent", arr)
	if len(nodes) != 5 {
		t.Fatalf("expected 5 children, got %d", len(nodes))
	}
	// All should be scalars with arr: prefix IDs
	for i, n := range nodes {
		expectedID := fmt.Sprintf("arr:parent:%d", i)
		if n.ID != expectedID {
			t.Errorf("nodes[%d].ID = %q, want %q", i, n.ID, expectedID)
		}
		if n.NodeType != "scalar" {
			t.Errorf("nodes[%d].NodeType = %q, want 'scalar'", i, n.NodeType)
		}
		expectedRawKey := fmt.Sprintf("[%d]", i)
		if n.RawKey != expectedRawKey {
			t.Errorf("nodes[%d].RawKey = %q, want %q", i, n.RawKey, expectedRawKey)
		}
	}
}
