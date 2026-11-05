package wails_v3_stable_upgrade_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// eventCallLiteral matches an Events.On/Once/OnMultiple/Off/Emit call whose
// first argument is a quoted string.
var eventCallLiteral = regexp.MustCompile("Events\\.(On|Once|OnMultiple|Off|Emit)\\(\\s*['\"`]([^'\"`]+)['\"`]")

// wailsSystemPrefixes are Wails' own window and application events, which are
// not registered by the app and keep their string names.
var wailsSystemPrefixes = []string{"common:", "mac:", "windows:", "linux:"}

func isFrontendTestFile(rel string) bool {
	base := filepath.Base(rel)
	return strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") ||
		base == "test-setup.ts" || strings.Contains(filepath.ToSlash(rel), "/__tests__/")
}

// frontendSourceLines calls fn for every non-comment line of every non-test
// source file under frontend/src.
func frontendSourceLines(t *testing.T, fn func(rel string, lineNo int, line string)) {
	t.Helper()
	root := filepath.Join(projectRoot(t), "frontend", "src")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		switch filepath.Ext(path) {
		case ".js", ".jsx", ".ts", ".tsx":
		default:
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if isFrontendTestFile(rel) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*") {
				continue
			}
			fn(filepath.ToSlash(rel), i+1, line)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk frontend/src: %v", err)
	}
}

func TestFrontendUsesGeneratedConstantsForAppEvents(t *testing.T) {
	frontendSourceLines(t, func(rel string, lineNo int, line string) {
		for _, m := range eventCallLiteral.FindAllStringSubmatch(line, -1) {
			name := m[2]
			system := false
			for _, p := range wailsSystemPrefixes {
				if strings.HasPrefix(name, p) {
					system = true
				}
			}
			if !system {
				t.Errorf("frontend/src/%s:%d passes the event name %q as a string literal; use the generated constant", rel, lineNo, name)
			}
		}
	})
}

func TestDeadDocumentWarningListenerIsGone(t *testing.T) {
	frontendSourceLines(t, func(rel string, lineNo int, line string) {
		if strings.Contains(line, "document:warning") {
			t.Errorf("frontend/src/%s:%d still refers to document:warning, which nothing emits", rel, lineNo)
		}
	})
}

func TestSaveBytesToFileIsGoneEverywhere(t *testing.T) {
	root := projectRoot(t)
	for _, dir := range []string{"frontend/src", "internal"} {
		base := filepath.Join(root, filepath.FromSlash(dir))
		err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			switch filepath.Ext(path) {
			case ".go", ".js", ".jsx", ".ts", ".tsx":
			default:
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(data), "SaveBytesToFile") {
				rel, _ := filepath.Rel(root, path)
				if strings.HasSuffix(rel, "save_embedded_and_recent_test.go") {
					return nil
				}
				t.Errorf("%s still names SaveBytesToFile", filepath.ToSlash(rel))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
}
