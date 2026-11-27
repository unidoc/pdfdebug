package mcp_gui_automation_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// browserRunner is the removed browser test runner's name, split so this file
// does not match its own scan.
var browserRunner = "play" + "wright"

func TestBrowserSuiteFilesAreGone(t *testing.T) {
	root := projectRoot(t)
	for _, rel := range []string{
		"tests/e2e",
		"tests/support",
		browserRunner + ".config.ts",
		".env.example",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			t.Errorf("%s still exists", rel)
		}
	}
}

func TestNoBrowserRunnerReferencesRemain(t *testing.T) {
	root := projectRoot(t)
	skipDirs := map[string]bool{
		"node_modules": true, ".git": true, ".claude": true,
		"_bmad": true, "_bmad-output": true, "output": true, "bin": true,
	}
	allowed := map[string]bool{
		"frontend/package-lock.json":        true,
		"tickets/T4-ux-and-shell-wiring.md": true,
		"CHANGELOG.md":                      true,
	}
	needle := []byte(browserRunner)
	var hits []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 || !d.Type().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if allowed[rel] {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(bytes.ToLower(data), needle) {
			hits = append(hits, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(hits) > 0 {
		t.Errorf("%d file(s) still name the removed browser test runner:\n%s", len(hits), strings.Join(hits, "\n"))
	}
}

func TestGateScriptAndCISkipOnlyTheBootSmokeSuite(t *testing.T) {
	for _, rel := range []string{"scripts/test-all.sh", ".github/workflows/ci.yml", "CONTRIBUTING.md"} {
		content := readRepoFile(t, rel)
		for _, gone := range []string{"e2e|", "support|"} {
			if strings.Contains(content, gone) {
				t.Errorf("%s still skips %q in the per-suite loop", rel, strings.TrimSuffix(gone, "|"))
			}
		}
	}
}
