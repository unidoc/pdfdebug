// Package tree_node_builder_test provides acceptance tests for Tree Node
// Builder -- PDF Object Graph to Tree Nodes.
//
// Test Levels: Unit (Go) -- pdfcore tree builder API validation.
// No browser interaction required; all criteria are Go package validation.
//
// Run: go test ./tests/tree-node-builder/... -v -count=1
package tree_node_builder_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// projectRoot returns the absolute path to the project root directory.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if content, err := os.ReadFile(goModPath); err == nil {
			if strings.Contains(string(content), "module unidoc-pdf-debugger") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root (no go.mod with module unidoc-pdf-debugger found)")
		}
		dir = parent
	}
}

// testdataDir returns the absolute path to the testdata/ directory at project root.
func testdataDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(projectRoot(t), "testdata")
}

// runPdfcoreTest runs a named test pattern in internal/pdfcore/... and fails if
// the test does not pass or does not exist.
func runPdfcoreTest(t *testing.T, runPattern string) {
	t.Helper()
	root := projectRoot(t)
	cmd := exec.Command("go", "test", "-v", "-run", runPattern, "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	outStr := string(output)
	if err != nil {
		t.Fatalf("pdfcore test failed:\n%s", outStr)
	}
	if strings.Contains(outStr, "no tests to run") {
		t.Fatalf("no matching test found for pattern %q -- unit test not implemented yet:\n%s", runPattern, outStr)
	}
	if !strings.Contains(outStr, "PASS") {
		t.Fatalf("expected PASS in output but got:\n%s", outStr)
	}
}

// ---------------------------------------------------------------------------
// GetTreeRoot() returns TreeNode with correct fields: Given a parsed
// PDF document, When GetTreeRoot(tabID) is called,
//       Then it returns a TreeNode with id="root", label="Catalog",
//       hasChildren=true, iconHint="catalog", nodeType="dict",
//       And childCount reflects the number of top-level catalog entries.
// ---------------------------------------------------------------------------

func TestGetTreeRootValidPDF(t *testing.T) {
	// Verify testdata/minimal.pdf exists (prerequisite)
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/minimal.pdf does not exist -- create test fixture first")
	}

	// Delegates to internal/pdfcore/tree_test.go::TestGetTreeRoot
	// which must:
	// - Open minimal.pdf via Inspector.Open()
	// - Call GetTreeRoot(tabID)
	// - Assert TreeNode.ID == "root"
	// - Assert TreeNode.Label == "Catalog"
	// - Assert TreeNode.HasChildren == true
	// - Assert TreeNode.NodeType == "dict"
	// - Assert TreeNode.IconHint == "catalog"
	// - Assert TreeNode.ChildCount > 0 (catalog has entries)
	// - Assert no error returned
	runPdfcoreTest(t, "TestGetTreeRoot$")
}

// ---------------------------------------------------------------------------
// GetChildren("root") returns children with obj: IDs
// Given the root node, When GetChildren(tabID, "root") is called,
//       Then it returns a slice of TreeNode for immediate catalog entries,
//       And indirect ref values produce child IDs in "obj:{gen}:{num}" format.
// ---------------------------------------------------------------------------

func TestGetChildrenRoot(t *testing.T) {
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/minimal.pdf does not exist -- create test fixture first")
	}

	// Delegates to internal/pdfcore/tree_test.go::TestGetChildrenRoot
	// which must:
	// - Open minimal.pdf, call GetChildren(tabID, "root")
	// - Assert result is non-nil and non-empty
	// - Assert at least one child has ID starting with "obj:" (indirect ref)
	// - Assert children include entries for /Type and /Pages (from catalog)
	// - Assert each child has a non-empty Label, NodeType, and IconHint
	runPdfcoreTest(t, "TestGetChildrenRoot$")
}

// ---------------------------------------------------------------------------
// GetChildren() for dict node returns dict:{parent}:{key} IDs: Given a tree node
// ID for a dictionary, When GetChildren is called,
//       Then children that are direct dict entries (non-IndirectRef values)
//       have IDs in "dict:{parent_id}:{key}" format with bare keys (no slash).
// ---------------------------------------------------------------------------

func TestGetChildrenDictNode(t *testing.T) {
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/minimal.pdf does not exist -- create test fixture first")
	}

	// Delegates to internal/pdfcore/tree_test.go::TestGetChildrenDictNode
	// which must:
	// - Open minimal.pdf, navigate to a dict node (e.g., resolve /Pages -> page dict)
	// - Call GetChildren on that dict node
	// - Assert at least one child has ID starting with "dict:"
	// - Assert the dict key in the ID is bare (no leading slash)
	// - Assert each child has RawKey prefixed with "/" for dict entries
	runPdfcoreTest(t, "TestGetChildrenDictNode$")
}

// ---------------------------------------------------------------------------
// GetChildren() for array node returns arr:{parent}:{index} IDs: Given a tree node
// ID for an array, When GetChildren is called,
//       Then children have IDs in "arr:{parent_id}:{index}" format.
// ---------------------------------------------------------------------------

func TestGetChildrenArrayNode(t *testing.T) {
	// multipage.pdf has /Pages with /Kids array containing multiple page refs
	multipagePDF := filepath.Join(testdataDir(t), "multipage.pdf")
	if _, err := os.Stat(multipagePDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/multipage.pdf does not exist -- create test fixture first")
	}

	// Delegates to internal/pdfcore/tree_test.go::TestGetChildrenArrayNode
	// which must:
	// - Open multipage.pdf, navigate to an array node (e.g., /Kids array)
	// - Call GetChildren on the array node
	// - Assert children have IDs starting with "arr:" or "obj:" (if elements are IndirectRefs)
	// - Assert array element children have RawKey like "[0]", "[1]", etc.
	runPdfcoreTest(t, "TestGetChildrenArrayNode$")
}

// ---------------------------------------------------------------------------
// Malformed object produces error node, siblings unaffected: Given a tree node
// for a malformed object, When GetChildren()
//       encounters a parsing error, Then it returns an error node with Error
//       field populated and Label="Error: {message}", And other sibling
//       nodes are still returned.
// ---------------------------------------------------------------------------

func TestErrorNodeCreation(t *testing.T) {
	// Delegates to internal/pdfcore/tree_test.go::TestErrorNodeCreation
	// which must:
	// - Trigger an error scenario (e.g., invalid node ID or malformed object)
	// - Assert the error node has Error field set (non-empty)
	// - Assert the error node Label starts with "Error:"
	// - Assert the error node NodeType is "scalar" (error nodes are leaves)
	// - Assert sibling nodes are still returned when one child errors
	runPdfcoreTest(t, "TestErrorNode")
}

// ---------------------------------------------------------------------------
// /Pages shows "Pages" with iconHint "page": Given tree
// builder, When building nodes for /Pages,
//       Then Label is "Pages" and iconHint is "page".
// ---------------------------------------------------------------------------

func TestSemanticLabelPages(t *testing.T) {
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/minimal.pdf does not exist -- create test fixture first")
	}

	// Delegates to internal/pdfcore/tree_test.go::TestSemanticLabelPages
	// which must:
	// - Open minimal.pdf, get children of root
	// - Find the child for /Pages
	// - Assert Label == "Pages"
	// - Assert IconHint == "page"
	runPdfcoreTest(t, "TestSemanticLabelPages$")
}

// ---------------------------------------------------------------------------
// /Font entries show "Font: {name}" with iconHint "font": Given tree
// builder, When building nodes for /Font entries,
//       Then Label follows "Font: {BaseFont}" pattern and iconHint is "font".
// ---------------------------------------------------------------------------

func TestSemanticLabelFont(t *testing.T) {
	// content-stream.pdf or multipage.pdf should have font resources
	multipagePDF := filepath.Join(testdataDir(t), "multipage.pdf")
	if _, err := os.Stat(multipagePDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/multipage.pdf does not exist -- create test fixture first")
	}

	// Delegates to internal/pdfcore/tree_test.go::TestSemanticLabelFont
	// which must:
	// - Open a PDF with font resources
	// - Navigate to a font dict entry
	// - Assert Label matches "Font: {name}" or "Font" pattern
	// - Assert IconHint == "font"
	runPdfcoreTest(t, "TestSemanticLabelFont$")
}

// ---------------------------------------------------------------------------
// Node types correctly assigned: Each child has correct nodeType (dict,
// array, stream, ref, scalar).
// ---------------------------------------------------------------------------

func TestNodeTypeAssignment(t *testing.T) {
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/minimal.pdf does not exist -- create test fixture first")
	}

	// Delegates to internal/pdfcore/tree_test.go::TestNodeTypeAssignment
	// which must:
	// - Open minimal.pdf, traverse tree nodes
	// - Assert dict nodes have NodeType "dict"
	// - Assert scalar leaf nodes have NodeType "scalar"
	// - Assert indirect ref children have NodeType reflecting the resolved type or "ref"
	// - Assert all NodeType values are one of: "dict", "array", "stream", "ref", "scalar"
	runPdfcoreTest(t, "TestNodeTypeAssignment$")
}

// ---------------------------------------------------------------------------
// Node ID round-trip: encode then decode produces
//                     original components
// Node IDs follow the scheme and can be parsed back to their components.
// ---------------------------------------------------------------------------

func TestNodeIDRoundTrip(t *testing.T) {
	// Delegates to internal/pdfcore/tree_test.go::TestNodeIDRoundTrip
	// which must:
	// - Test parseNodeID("root") returns kind="root"
	// - Test parseNodeID("obj:0:5") returns kind="obj", parts=["0","5"]
	// - Test parseNodeID("dict:root:Pages") returns kind="dict", parentID="root", key="Pages"
	// - Test parseNodeID("dict:obj:0:5:Type") returns kind="dict", parentID="obj:0:5", key="Type"
	// - Test parseNodeID("arr:obj:0:12:3") returns kind="arr", parentID="obj:0:12", index="3"
	runPdfcoreTest(t, "TestNodeIDRoundTrip$")
}

// ---------------------------------------------------------------------------
// XObject image entries get iconHint "image": /XObject image
// entries show with iconHint "image".
// ---------------------------------------------------------------------------

func TestIconHintXObjectImage(t *testing.T) {
	// Delegates to internal/pdfcore/tree_test.go::TestIconHintXObjectImage
	// which must:
	// - Test that an XObject with Subtype=Image gets iconHint "image"
	// - Can use a real PDF with images or a unit test with mock pdfcpu objects
	runPdfcoreTest(t, "TestIconHintXObjectImage$")
}

// ---------------------------------------------------------------------------
// Empty dictionary returns empty slice, not nil (edge case): GetChildren on a
// dict with no entries returns []*TreeNode{}.
// ---------------------------------------------------------------------------

func TestGetChildrenEmptyDict(t *testing.T) {
	// Delegates to internal/pdfcore/tree_test.go::TestGetChildrenEmptyDict
	// which must:
	// - Create or navigate to a dict with zero entries
	// - Call GetChildren
	// - Assert result is non-nil empty slice (not nil)
	runPdfcoreTest(t, "TestGetChildrenEmptyDict$")
}

// ---------------------------------------------------------------------------
// Empty array returns empty slice, not nil (edge case): GetChildren on an array
// with no elements returns []*TreeNode{}.
// ---------------------------------------------------------------------------

func TestGetChildrenEmptyArray(t *testing.T) {
	// Delegates to internal/pdfcore/tree_test.go::TestGetChildrenEmptyArray
	// which must:
	// - Create or navigate to an array with zero elements
	// - Call GetChildren
	// - Assert result is non-nil empty slice (not nil)
	runPdfcoreTest(t, "TestGetChildrenEmptyArray$")
}

// ---------------------------------------------------------------------------
// GetTreeRoot with unknown tabID returns error (negative): Given an
// unknown tabID, When GetTreeRoot is called,
//                  Then it returns ErrDocumentNotFound.
// ---------------------------------------------------------------------------

func TestGetTreeRootUnknownTabID(t *testing.T) {
	// Delegates to internal/pdfcore/tree_test.go::TestGetTreeRootUnknownTabID
	// which must:
	// - Call GetTreeRoot with a tabID that was never opened
	// - Assert error is returned
	// - Assert error wraps ErrDocumentNotFound
	runPdfcoreTest(t, "TestGetTreeRootUnknownTabID$")
}

// ---------------------------------------------------------------------------
// GetChildren with unknown tabID returns error (negative): Given an
// unknown tabID, When GetChildren is called,
//                  Then it returns ErrDocumentNotFound.
// ---------------------------------------------------------------------------

func TestGetChildrenUnknownTabID(t *testing.T) {
	// Delegates to internal/pdfcore/tree_test.go::TestGetChildrenUnknownTabID
	// which must:
	// - Call GetChildren with a tabID that was never opened
	// - Assert error is returned
	// - Assert error wraps ErrDocumentNotFound
	runPdfcoreTest(t, "TestGetChildrenUnknownTabID$")
}

// ---------------------------------------------------------------------------
// GetChildren with invalid nodeID format returns error (negative): Given
// a malformed nodeID, When GetChildren is called,
//                  Then it returns an error (not panic).
// ---------------------------------------------------------------------------

func TestGetChildrenInvalidNodeID(t *testing.T) {
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/minimal.pdf does not exist -- create test fixture first")
	}

	// Delegates to internal/pdfcore/tree_test.go::TestGetChildrenInvalidNodeID
	// which must:
	// - Open minimal.pdf
	// - Call GetChildren with an invalid nodeID (e.g., "bogus", "obj:", "obj:abc:def")
	// - Assert error is returned
	// - Assert no panic occurs
	runPdfcoreTest(t, "TestGetChildrenInvalidNodeID$")
}

// ---------------------------------------------------------------------------
// Scalar leaf nodes have HasChildren=false and valueType set: Scalar values
// (Name, String, Integer, etc.) are leaf nodes.
// ---------------------------------------------------------------------------

func TestScalarLeafNodes(t *testing.T) {
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/minimal.pdf does not exist -- create test fixture first")
	}

	// Delegates to internal/pdfcore/tree_test.go::TestScalarLeafNodes
	// which must:
	// - Open minimal.pdf, get children of root
	// - Find a scalar child (e.g., /Type which is a Name value "Catalog")
	// - Assert HasChildren == false
	// - Assert ValueType is set (e.g., "name" for Name objects)
	// - Assert NodeType == "scalar"
	runPdfcoreTest(t, "TestScalarLeafNodes$")
}

// ---------------------------------------------------------------------------
// Root iconHint is "catalog": Root catalog node has
// iconHint "catalog".
// ---------------------------------------------------------------------------

func TestIconHintCatalog(t *testing.T) {
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	if _, err := os.Stat(minimalPDF); errors.Is(err, os.ErrNotExist) {
		t.Fatalf("testdata/minimal.pdf does not exist -- create test fixture first")
	}

	// Delegates to internal/pdfcore/tree_test.go::TestGetTreeRoot
	// The root node test already covers iconHint="catalog" but we verify
	// explicitly via the tree_test.go root test.
	runPdfcoreTest(t, "TestGetTreeRoot$")
}

// ---------------------------------------------------------------------------
// tree.go file exists with required method signatures: GetTreeRoot and
// GetChildren methods exist on Inspector.
// ---------------------------------------------------------------------------

func TestTreeFileAndMethodsExist(t *testing.T) {
	root := projectRoot(t)

	treePath := filepath.Join(root, "internal", "pdfcore", "tree.go")
	content, err := os.ReadFile(treePath)
	if err != nil {
		t.Fatalf("internal/pdfcore/tree.go does not exist: %v", err)
	}

	treeContent := string(content)

	// Verify GetTreeRoot method signature
	if !strings.Contains(treeContent, "func (ins *Inspector) GetTreeRoot(") {
		t.Error("tree.go missing GetTreeRoot method on Inspector")
	}

	// Verify GetChildren method signature
	if !strings.Contains(treeContent, "func (ins *Inspector) GetChildren(") {
		t.Error("tree.go missing GetChildren method on Inspector")
	}

	// Verify unexported helpers exist
	helpers := []string{
		"func buildTreeNode(",
		"func semanticLabel(",
		"func iconHint(",
		"func parseNodeID(",
		"func resolveNodeObject(",
	}
	for _, h := range helpers {
		if !strings.Contains(treeContent, h) {
			t.Errorf("tree.go missing helper: %s", h)
		}
	}

	// Verify no Wails imports
	if strings.Contains(treeContent, "wailsapp") {
		t.Error("tree.go imports Wails -- pdfcore must have zero Wails dependencies")
	}
}

// ---------------------------------------------------------------------------
// All tree-related pdfcore tests pass: unit tests cover tree root building,
// child enumeration for dicts/arrays/scalars/refs, error node creation, and
// semantic labeling.
// ---------------------------------------------------------------------------

func TestAllTreeTestsPass(t *testing.T) {
	root := projectRoot(t)

	// Run all pdfcore tests -- ensures tree.go + tree_test.go compile and pass
	cmd := exec.Command("go", "test", "-v", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pdfcore test suite failed:\n%s", string(output))
	}

	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// go vet passes on pdfcore with tree.go: No vet warnings
// after adding tree.go.
// ---------------------------------------------------------------------------

func TestPdfcoreGoVetWithTree(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "vet", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go vet failed on pdfcore:\n%s", string(output))
	}
	_ = output
}

// ---------------------------------------------------------------------------
// Project compiles with tree.go added: go build ./...
// succeeds.
// ---------------------------------------------------------------------------

func TestPdfcoreCompiles(t *testing.T) {
	root := projectRoot(t)

	// Verify pdfcore package compiles (go build on the package, not full project)
	cmd := exec.Command("go", "build", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build ./internal/pdfcore/... failed:\n%s", string(output))
	}
	_ = output
}
