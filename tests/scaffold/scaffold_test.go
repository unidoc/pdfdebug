// Package scaffold_test provides acceptance tests for Initialize Wails v3
// Project with React-TypeScript Scaffold.
//
// These tests verify that the project scaffolding was performed correctly.
// Test Levels: Integration (Go) -- shell commands + filesystem checks.
// No browser interaction required; all criteria are build/scaffold validation.
//
// Run: go test ./tests/scaffold/... -v -count=1
package scaffold_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// projectRoot returns the absolute path to the project root directory.
// It walks upward from the test file location to find the project root,
// identified by the presence of a go.mod whose module name is "unidoc-pdf-debugger".
// This distinguishes the project root from the test module's own go.mod.
func projectRoot(t *testing.T) string {
	t.Helper()
	// Start from the test file's directory and walk up to find project root
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	// Walk up until we find the project root's go.mod (module unidoc-pdf-debugger)
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
// Wails v3 project builds successfully: wails3 build
// produces a native binary
// ---------------------------------------------------------------------------

func TestWailsBuildProducesBinary(t *testing.T) {
	root := projectRoot(t)

	// Verify wails3 build succeeds. On Linux, CI runners ship only the GTK3 +
	// WebKit2GTK 4.1 stack; Wails alpha.93+ defaults to GTK4 + WebKitGTK 6.0
	// on Linux, so we drive the linux:build task with EXTRA_TAGS=gtk3 (the
	// Taskfile combines it into `-tags production,gtk3`). Matches ci.yml +
	// release.yml.
	var cmd *exec.Cmd
	if runtime.GOOS == "linux" {
		cmd = exec.Command("wails3", "task", "linux:build", "EXTRA_TAGS=gtk3")
	} else {
		cmd = exec.Command("wails3", "build")
	}
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("wails3 build failed: %v\nOutput:\n%s", err, string(output))
	}

	// Verify binary was produced in bin/ (Wails v3 alpha outputs to bin/, not build/bin/)
	binDir := filepath.Join(root, "bin")
	entries, err := os.ReadDir(binDir)
	if err != nil {
		t.Fatalf("bin/ directory not found after build: %v", err)
	}

	foundBinary := false
	for _, entry := range entries {
		if !entry.IsDir() {
			foundBinary = true
			break
		}
	}
	if !foundBinary {
		t.Fatalf("no binary found in bin/ after wails3 build")
	}
}

// ---------------------------------------------------------------------------
// Go module resolves all dependencies: pdfcpu is
// present; go mod tidy + go vet succeed
// ---------------------------------------------------------------------------

func TestGoModuleDependencies(t *testing.T) {
	root := projectRoot(t)

	// Verify go.mod exists
	goModPath := filepath.Join(root, "go.mod")
	goModContent, err := os.ReadFile(goModPath)
	if err != nil {
		t.Fatalf("go.mod not found at project root: %v", err)
	}

	// Verify pdfcpu dependency is declared
	if !strings.Contains(string(goModContent), "github.com/pdfcpu/pdfcpu") {
		t.Fatal("pdfcpu dependency not found in go.mod")
	}

	// Verify go mod tidy succeeds (no unresolved dependencies)
	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = root
	tidyOutput, err := tidyCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go mod tidy failed: %v\nOutput:\n%s", err, string(tidyOutput))
	}

	// Verify go vet passes (no structural issues)
	vetCmd := exec.Command("go", "vet", "./...")
	vetCmd.Dir = root
	vetOutput, err := vetCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go vet ./... failed: %v\nOutput:\n%s", err, string(vetOutput))
	}
}

// ---------------------------------------------------------------------------
// Frontend dependencies install and TypeScript compiles: Required frontend
// deps installed; strict mode enabled
// ---------------------------------------------------------------------------

func TestFrontendDependenciesAndTypeScript(t *testing.T) {
	root := projectRoot(t)
	frontendDir := filepath.Join(root, "frontend")

	// Verify frontend/package.json exists
	pkgJSONPath := filepath.Join(frontendDir, "package.json")
	pkgJSONContent, err := os.ReadFile(pkgJSONPath)
	if err != nil {
		t.Fatalf("frontend/package.json not found: %v", err)
	}

	// Parse package.json to check dependencies
	var pkgJSON map[string]interface{}
	if err := json.Unmarshal(pkgJSONContent, &pkgJSON); err != nil {
		t.Fatalf("failed to parse frontend/package.json: %v", err)
	}

	// Check required dependencies exist (in either dependencies or devDependencies)
	deps := make(map[string]bool)
	for _, section := range []string{"dependencies", "devDependencies"} {
		if sectionMap, ok := pkgJSON[section].(map[string]interface{}); ok {
			for pkg := range sectionMap {
				deps[pkg] = true
			}
		}
	}

	requiredDeps := []string{
		"tailwindcss",
		"@tailwindcss/vite",
		"@radix-ui/react-tabs",
		"@radix-ui/react-scroll-area",
		"@radix-ui/react-separator",
		"@radix-ui/react-dialog",
		"@radix-ui/react-tooltip",
		"@radix-ui/react-toggle",
		"react-arborist",
		"allotment",
	}

	for _, dep := range requiredDeps {
		if !deps[dep] {
			t.Errorf("required frontend dependency missing from package.json: %s", dep)
		}
	}

	// Verify TypeScript strict mode is enabled
	tsconfigPath := filepath.Join(frontendDir, "tsconfig.json")
	tsconfigContent, err := os.ReadFile(tsconfigPath)
	if err != nil {
		t.Fatalf("frontend/tsconfig.json not found: %v", err)
	}

	// Note: tsconfig.json may have comments (jsonc format). We do a string check
	// as a pragmatic approach since Go's encoding/json doesn't handle comments.
	if !strings.Contains(string(tsconfigContent), `"strict"`) {
		t.Fatal("TypeScript strict mode not found in tsconfig.json")
	}

	// Verify TypeScript compiles successfully
	tscCmd := exec.Command("npx", "tsc", "--noEmit")
	tscCmd.Dir = frontendDir
	tscOutput, err := tscCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("npx tsc --noEmit failed in frontend/: %v\nOutput:\n%s", err, string(tscOutput))
	}
}

// ---------------------------------------------------------------------------
// Directory structure matches architecture: internal/pdfcore/,
// internal/pdfservice/, cmd/cli/, testdata/ exist
// ---------------------------------------------------------------------------

func TestDirectoryStructure(t *testing.T) {
	root := projectRoot(t)

	requiredDirs := []struct {
		path        string
		description string
	}{
		{"internal/pdfcore", "PDF inspection and parsing package"},
		{"internal/pdfservice", "Wails service adapter package"},
		{"cmd/cli", "CLI binary package"},
		{"testdata", "test data directory"},
	}

	for _, dir := range requiredDirs {
		dirPath := filepath.Join(root, dir.path)
		info, err := os.Stat(dirPath)
		if err != nil {
			t.Errorf("required directory missing: %s (%s): %v", dir.path, dir.description, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("%s exists but is not a directory", dir.path)
		}
	}

	// Verify Go package files exist in internal directories
	goPackageFiles := []struct {
		path    string
		content string
	}{
		{"internal/pdfcore/doc.go", "package pdfcore"},
		{"internal/pdfservice/doc.go", "package pdfservice"},
		{"cmd/cli/doc.go", "package main"},
	}

	for _, f := range goPackageFiles {
		filePath := filepath.Join(root, f.path)
		content, err := os.ReadFile(filePath)
		if err != nil {
			t.Errorf("required package file missing: %s: %v", f.path, err)
			continue
		}
		if !strings.Contains(string(content), f.content) {
			t.Errorf("%s does not contain expected package declaration %q", f.path, f.content)
		}
	}

	// Verify testdata/ directory exists (originally had .gitkeep, now has real PDF fixtures)
	testdataPath := filepath.Join(root, "testdata")
	if info, err := os.Stat(testdataPath); err != nil {
		t.Errorf("testdata/ directory not found: %v", err)
	} else if !info.IsDir() {
		t.Errorf("testdata is not a directory")
	}
}

// ---------------------------------------------------------------------------
// LICENSE and NOTICE files present: Apache 2.0 LICENSE and UniDoc
// NOTICE files at project root
// ---------------------------------------------------------------------------

func TestLegalFiles(t *testing.T) {
	root := projectRoot(t)

	// Verify LICENSE file exists and contains Apache 2.0 text
	licensePath := filepath.Join(root, "LICENSE")
	licenseContent, err := os.ReadFile(licensePath)
	if err != nil {
		t.Fatalf("LICENSE file not found at project root: %v", err)
	}
	if !strings.Contains(string(licenseContent), "Apache License") {
		t.Error("LICENSE file does not contain 'Apache License' text")
	}
	if !strings.Contains(string(licenseContent), "Version 2.0") {
		t.Error("LICENSE file does not contain 'Version 2.0' text")
	}

	// Verify NOTICE file exists and contains UniDoc attribution
	noticePath := filepath.Join(root, "NOTICE")
	noticeContent, err := os.ReadFile(noticePath)
	if err != nil {
		t.Fatalf("NOTICE file not found at project root: %v", err)
	}
	if !strings.Contains(string(noticeContent), "UniDoc") {
		t.Error("NOTICE file does not contain 'UniDoc' attribution")
	}
	if !strings.Contains(string(noticeContent), "Apache License") {
		t.Error("NOTICE file does not reference Apache License")
	}
}

// ---------------------------------------------------------------------------
// .gitignore covers required patterns: .gitignore covers Go,
// Node.js, and Wails build artifacts
// ---------------------------------------------------------------------------

func TestGitignoreCoverage(t *testing.T) {
	root := projectRoot(t)

	gitignorePath := filepath.Join(root, ".gitignore")
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf(".gitignore not found at project root: %v", err)
	}

	gitignoreContent := string(content)

	// Required patterns (exact strings or reasonable variants)
	requiredPatterns := []struct {
		pattern     string
		description string
	}{
		{"build/bin", "Wails build output"},
		{"node_modules", "Node.js dependencies"},
		{"frontend/dist", "Frontend build output"},
		{".DS_Store", "macOS metadata"},
	}

	for _, p := range requiredPatterns {
		if !strings.Contains(gitignoreContent, p.pattern) {
			t.Errorf(".gitignore missing pattern for %s (expected to contain %q)", p.description, p.pattern)
		}
	}

	// Verify important files are NOT ignored
	mustNotIgnore := []string{
		"go.mod",
		"go.sum",
	}

	lines := strings.Split(gitignoreContent, "\n")
	for _, mustKeep := range mustNotIgnore {
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			// Skip comments and empty lines
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			// Simple check: the gitignore line should not exactly match the file we want to keep
			if trimmed == mustKeep || trimmed == "/"+mustKeep {
				t.Errorf(".gitignore must NOT ignore %s but found matching rule: %q", mustKeep, trimmed)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Wails project configuration files exist: verifies the essential config
// files produced by scaffold
// ---------------------------------------------------------------------------

func TestWailsProjectConfigFiles(t *testing.T) {
	root := projectRoot(t)

	configFiles := []struct {
		path        string
		description string
	}{
		{filepath.Join("build", "config.yml"), "Wails v3 project configuration"},
		{"Taskfile.yml", "Wails v3 build system (Taskfile)"},
		{"go.mod", "Go module definition"},
		{"go.sum", "Go module checksums"},
		{filepath.Join("frontend", "tsconfig.json"), "TypeScript configuration"},
		{filepath.Join("frontend", "vite.config.ts"), "Vite build configuration"},
		{filepath.Join("frontend", "package.json"), "Frontend package manifest"},
	}

	for _, cf := range configFiles {
		filePath := filepath.Join(root, cf.path)
		if _, err := os.Stat(filePath); err != nil {
			t.Errorf("required config file missing: %s (%s): %v", cf.path, cf.description, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Vite config includes Tailwind CSS plugin (partial): Tailwind CSS v4
// configured with @tailwindcss/vite plugin
// ---------------------------------------------------------------------------

func TestViteConfigIncludesTailwind(t *testing.T) {
	root := projectRoot(t)

	viteConfigPath := filepath.Join(root, "frontend", "vite.config.ts")
	content, err := os.ReadFile(viteConfigPath)
	if err != nil {
		t.Fatalf("frontend/vite.config.ts not found: %v", err)
	}

	viteConfig := string(content)

	// Verify Tailwind CSS v4 Vite plugin is imported and used
	if !strings.Contains(viteConfig, "@tailwindcss/vite") {
		t.Error("vite.config.ts does not import @tailwindcss/vite plugin")
	}
	if !strings.Contains(viteConfig, "tailwindcss()") && !strings.Contains(viteConfig, "tailwindcss(") {
		t.Error("vite.config.ts does not appear to use the tailwindcss() plugin in plugins array")
	}
}

// ---------------------------------------------------------------------------
// Pdfcpu blank import pins dependency (detail): pdfcpu is pinned via blank import
// so go mod tidy won't remove it
// ---------------------------------------------------------------------------

func TestPdfcpuBlankImport(t *testing.T) {
	root := projectRoot(t)

	docGoPath := filepath.Join(root, "internal", "pdfcore", "doc.go")
	content, err := os.ReadFile(docGoPath)
	if err != nil {
		t.Fatalf("internal/pdfcore/doc.go not found: %v", err)
	}

	docGoContent := string(content)

	// Verify blank import of pdfcpu to pin the dependency
	if !strings.Contains(docGoContent, `_ "github.com/pdfcpu/pdfcpu`) {
		t.Error("internal/pdfcore/doc.go does not contain blank import for pdfcpu (needed to pin dependency in go.mod)")
	}
}
