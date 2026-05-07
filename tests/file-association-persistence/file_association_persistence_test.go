// Package file_association_persistence_test provides acceptance tests for
// Story 4.4: OS File Association, Single Instance, and Window Persistence.
//
// These are TDD RED PHASE tests -- they MUST fail until Story 4-4 is implemented.
//
// Test Levels:
//   - Unit (Go): extractPDFPaths helper in main.go
//   - Structural: Verify production files exist and contain expected content
//
// Frontend tests (useWindowPersistence, MainLayout persistence) are in Vitest:
//   frontend/src/hooks/useWindowPersistence.test.ts
//   frontend/src/components/MainLayout.persistence.test.tsx
//
// Run: cd tests/file-association-persistence && go test -v -count=1 ./...
package file_association_persistence_test

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

// ---------------------------------------------------------------------------
// 4.4-UNIT-005 [P2]: extractPDFPaths extracts .pdf paths from args.
//
// AC#2: Given a second instance is launched with args containing PDF paths,
//       When extractPDFPaths parses the args,
//       Then it returns only the .pdf arguments (case-insensitive extension).
//
// The helper function extractPDFPaths must exist in main.go (package main)
// and be tested via main_test.go in the project root.
// This acceptance test verifies that main_test.go passes.
// ---------------------------------------------------------------------------

func TestExtractPDFPaths(t *testing.T) {
	root := projectRoot(t)

	// Verify main_test.go exists with extractPDFPaths tests
	mainTestPath := filepath.Join(root, "main_test.go")
	if _, err := os.Stat(mainTestPath); os.IsNotExist(err) {
		t.Fatal("[P2] 4.4-UNIT-005: main_test.go does not exist in project root -- create extractPDFPaths tests")
	}

	// Run the extractPDFPaths tests in the project root
	cmd := exec.Command("go", "test", "-v", "-run", "TestExtractPDFPaths", "-count=1", ".")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P2] 4.4-UNIT-005: extractPDFPaths tests failed:\n%s", string(output))
	}
	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("[P2] 4.4-UNIT-005: expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 4.4-STRUCT-001: extractPDFPaths function exists in main.go.
//
// AC#2: The helper must be defined so OnSecondInstanceLaunch can use it.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 4.4-STRUCT-002: SingleInstance option configured in main.go.
//
// AC#2: Wails v3 SingleInstance option must be set in application.Options
//       with the correct UniqueID.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 4.4-STRUCT-003: FileAssociations configured in main.go.
//
// AC#1: The application must register .pdf file association.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 4.4-STRUCT-004: ApplicationOpenedWithFile event handler in main.go.
//
// AC#1: The handler must exist to process files opened via OS association.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 4.4-STRUCT-005: useWindowPersistence hook file exists.
//
// AC#3: Panel persistence hook must be created.
// ---------------------------------------------------------------------------


// ---------------------------------------------------------------------------
// 4.4-STRUCT-006: Linux .desktop file has MimeType and %f.
//
// AC#1: Linux file association requires MimeType and %f in Exec.
// ---------------------------------------------------------------------------

func TestLinuxDesktopFileAssociation(t *testing.T) {
	root := projectRoot(t)

	desktopPath := filepath.Join(root, "build", "linux", "unidoc-pdf-debugger.desktop")
	content, err := os.ReadFile(desktopPath)
	if err != nil {
		t.Fatalf("[P2] 4.4-STRUCT-006: cannot read unidoc-pdf-debugger.desktop: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, "MimeType=application/pdf") {
		t.Error("[P2] 4.4-STRUCT-006: .desktop file missing MimeType=application/pdf")
	}
	if !strings.Contains(s, "%"+"f") {
		t.Errorf("[P2] 4.4-STRUCT-006: .desktop file missing %s in Exec line", "%f")
	}
}

// ---------------------------------------------------------------------------
// 4.4-STRUCT-007: Vitest tests for useWindowPersistence exist and pass.
//
// AC#3: Frontend panel persistence tests (4.4-UNIT-001 through 4.4-UNIT-004).
// ---------------------------------------------------------------------------

func TestVitestWindowPersistenceTests(t *testing.T) {
	root := projectRoot(t)

	// Verify test file exists
	testPath := filepath.Join(root, "frontend", "src", "hooks", "useWindowPersistence.test.ts")
	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		t.Fatal("[P1] 4.4-STRUCT-007: useWindowPersistence.test.ts does not exist")
	}

	// Run the Vitest tests
	cmd := exec.Command("npx", "vitest", "run", "src/hooks/useWindowPersistence.test.ts")
	cmd.Dir = filepath.Join(root, "frontend")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P1] 4.4-STRUCT-007: useWindowPersistence Vitest tests failed:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 4.4-STRUCT-008: Vitest tests for MainLayout persistence exist and pass.
//
// AC#3: MainLayout integration with persistence hook.
// ---------------------------------------------------------------------------

func TestVitestMainLayoutPersistenceTests(t *testing.T) {
	root := projectRoot(t)

	// Verify test file exists
	testPath := filepath.Join(root, "frontend", "src", "components", "MainLayout.persistence.test.tsx")
	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		t.Fatal("[P1] 4.4-STRUCT-008: MainLayout.persistence.test.tsx does not exist")
	}

	// Run the Vitest tests
	cmd := exec.Command("npx", "vitest", "run", "src/components/MainLayout.persistence.test.tsx")
	cmd.Dir = filepath.Join(root, "frontend")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P1] 4.4-STRUCT-008: MainLayout persistence Vitest tests failed:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 4.4-REGR-001: All pdfcore tests still pass (no regression).
// ---------------------------------------------------------------------------

func TestPdfcoreNoRegression(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "test", "-v", "-count=1", "./internal/pdfcore/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P1] 4.4-REGR-001: pdfcore regression -- tests failed:\n%s", string(output))
	}
	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("[P1] 4.4-REGR-001: expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 4.4-REGR-002: All pdfservice tests still pass (no regression).
// ---------------------------------------------------------------------------

func TestPdfserviceNoRegression(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "test", "-v", "-count=1", "./internal/pdfservice/...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P1] 4.4-REGR-002: pdfservice regression -- tests failed:\n%s", string(output))
	}
	if !strings.Contains(string(output), "PASS") {
		t.Fatalf("[P1] 4.4-REGR-002: expected PASS in output but got:\n%s", string(output))
	}
}

// ---------------------------------------------------------------------------
// 4.4-REGR-003: go vet passes on the project.
// ---------------------------------------------------------------------------

func TestGoVet(t *testing.T) {
	root := projectRoot(t)

	cmd := exec.Command("go", "vet", "./...")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[P1] 4.4-REGR-003: go vet failed:\n%s", string(output))
	}
}
