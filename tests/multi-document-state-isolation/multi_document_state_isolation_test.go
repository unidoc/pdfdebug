// Package multi_document_state_isolation_test provides acceptance tests for
// Story 4.2: Multi-Document State Isolation.
//
// Test Levels: Integration (Go) and Unit (Go) -- pdfcore API validation.
// No browser interaction required; all criteria are Go package validation.
//
// Run: cd tests/multi-document-state-isolation && go test -v -count=1 ./...
package multi_document_state_isolation_test

import (
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

// runColocatedTest runs a specific test by name in the given Go package and
// verifies it was actually executed (not just "no tests to run"), so a missing
// co-located test fails rather than passing silently.
func runColocatedTest(t *testing.T, root, testName, pkg string) {
	t.Helper()
	cmd := exec.Command("go", "test", "-v", "-run", "^"+testName+"$", "-count=1", pkg)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	out := string(output)
	if err != nil {
		t.Fatalf("test %s failed:\n%s", testName, out)
	}
	// go test -run with no matching tests prints "no tests to run" but exits 0.
	// We must detect this and fail: the co-located test does not exist yet.
	if strings.Contains(out, "no tests to run") {
		t.Fatalf("co-located test %s does not exist in %s\n%s", testName, pkg, out)
	}
	if !strings.Contains(out, "PASS") {
		t.Fatalf("expected PASS for %s but got:\n%s", testName, out)
	}
}

// ---------------------------------------------------------------------------
// Inspector.Open() with two different tabIDs creates two independent
// DocumentState entries.
// Each document has its own entry in the documents map keyed by tabID.
//
// Given two different PDF files,
// When Inspector.Open() is called with distinct tabIDs for each,
// Then both are accessible via GetDocument (no error returned for either),
// And GetTreeRoot on each returns distinct root nodes.
// ---------------------------------------------------------------------------

func TestTwoIndependentDocumentStates(t *testing.T) {
	root := projectRoot(t)

	// Verify fixtures exist
	minimalPDF := filepath.Join(testdataDir(t), "minimal.pdf")
	multipagePDF := filepath.Join(testdataDir(t), "multipage.pdf")
	for _, f := range []string{minimalPDF, multipagePDF} {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			t.Fatalf("test fixture does not exist: %s", f)
		}
	}

	runColocatedTest(t, root, "TestTwoDocumentStatesIndependent", "./internal/pdfcore/...")
}

// ---------------------------------------------------------------------------
// Inspector.Close() removes only the specified tabID.
// Closing a tab calls CloseDocument() which frees resources for that
// document only.
//
// Given two documents are open with different tabIDs,
// When Inspector.Close() is called for tab-1,
// Then GetDocument(tab-1) returns ErrDocumentNotFound,
// And GetTreeRoot(tab-2) still works normally.
// ---------------------------------------------------------------------------

func TestCloseRemovesOnlyTargetTab(t *testing.T) {
	root := projectRoot(t)
	runColocatedTest(t, root, "TestCloseRemovesOnlyTargetTab", "./internal/pdfcore/...")
}

// ---------------------------------------------------------------------------
// Malformed PDF in one tab does not affect queries to another.
// Error isolation between documents.
//
// Given testdata/multipage.pdf is open as tab-1,
// And testdata/malformed.pdf is opened as tab-2 (may open with warning or fail),
// When GetObjectDetail is called on tab-1 for the root node,
// Then it returns correct data unaffected by tab-2's state.
// ---------------------------------------------------------------------------

func TestMalformedPDFDoesNotAffectOtherTab(t *testing.T) {
	root := projectRoot(t)

	malformedPDF := filepath.Join(testdataDir(t), "malformed.pdf")
	if _, err := os.Stat(malformedPDF); os.IsNotExist(err) {
		t.Fatalf("testdata/malformed.pdf does not exist -- create test fixture first")
	}

	runColocatedTest(t, root, "TestMalformedPDFDoesNotAffectOtherTab", "./internal/pdfcore/...")
}

// ---------------------------------------------------------------------------
// Failed PDF open (encrypted) does not affect other tabs.
// Error path isolation.
//
// Given testdata/multipage.pdf is open as tab-1,
// When testdata/encrypted.pdf is opened as tab-2 (should fail with ErrEncryptedPDF),
// Then tab-1 remains queryable via GetTreeRoot and GetObjectDetail.
// ---------------------------------------------------------------------------

func TestEncryptedPDFFailDoesNotAffectOtherTab(t *testing.T) {
	root := projectRoot(t)

	encryptedPDF := filepath.Join(testdataDir(t), "encrypted.pdf")
	if _, err := os.Stat(encryptedPDF); os.IsNotExist(err) {
		t.Fatalf("testdata/encrypted.pdf does not exist -- create test fixture first")
	}

	runColocatedTest(t, root, "TestEncryptedPDFFailDoesNotAffectOtherTab", "./internal/pdfcore/...")
}

// ---------------------------------------------------------------------------
// Content stream cache is per-document -- closing one tab does not clear
// another tab's cache.
// Stream cache isolation.
//
// Given two documents are open,
// When GetContentStream is called on tab-1 for a stream node,
// And tab-2 is closed,
// Then calling GetContentStream on tab-1 again returns the cached result.
// ---------------------------------------------------------------------------

func TestStreamCacheIsolationAfterClose(t *testing.T) {
	root := projectRoot(t)
	runColocatedTest(t, root, "TestStreamCacheIsolationAfterClose", "./internal/pdfcore/...")
}

// ---------------------------------------------------------------------------
// Regression: All pdfcore tests still pass
// ---------------------------------------------------------------------------

func TestPdfcoreNoRegression(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "test", "-v", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pdfcore regression -- tests failed:\n%s", string(output))
	}
	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// Regression: All pdfservice tests still pass
// ---------------------------------------------------------------------------

func TestPdfserviceNoRegression(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "test", "-v", "-count=1", "./internal/pdfservice/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("pdfservice regression -- tests failed:\n%s", string(output))
	}
	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("expected PASS in output but got:\n%s", string(output))
	}
}
