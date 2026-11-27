package release_pipeline_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const noMCPTagScript = "scripts/check-no-mcp-tag.sh"

// throwawaySource writes a minimal main package into a directory the test
// removes when it ends.
func throwawaySource(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module throwaway\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatalf("write throwaway go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("write throwaway main.go: %v", err)
	}
	return dir
}

// buildThrowaway builds the throwaway package the way release.yml builds
// shipped binaries (-trimpath, stripped) with the given tags.
func buildThrowaway(t *testing.T, out, tags string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	args := []string{"build", "-trimpath", "-ldflags=-s -w"}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	args = append(args, "-o", out, ".")
	cmd := exec.Command("go", args...)
	cmd.Dir = throwawaySource(t)
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=", "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build -tags %q: %v\n%s", tags, err, b)
	}
	return out
}

// runNoMCPTagCheck runs the release check script on the given binaries.
func runNoMCPTagCheck(t *testing.T, binaries ...string) (string, error) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not on PATH")
	}
	script := filepath.Join(projectRoot(t), filepath.FromSlash(noMCPTagScript))
	if _, err := os.Stat(script); err != nil {
		t.Fatalf("%s is missing: %v", noMCPTagScript, err)
	}
	cmd := exec.Command("bash", append([]string{script}, binaries...)...)
	cmd.Dir = projectRoot(t)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestNoMCPTagCheckRejectsAnMCPTaggedBinary(t *testing.T) {
	dir := t.TempDir()
	for _, tags := range []string{"mcp", "production,mcp", "mcp,production"} {
		t.Run(tags, func(t *testing.T) {
			bin := buildThrowaway(t, filepath.Join(dir, strings.ReplaceAll(tags, ",", "-"), "app"), tags)
			out, err := runNoMCPTagCheck(t, bin)
			if err == nil {
				t.Fatalf("%s passed a binary built with -tags %s:\n%s", noMCPTagScript, tags, out)
			}
			if !strings.Contains(out, bin) {
				t.Errorf("failure output does not name the binary %s:\n%s", bin, out)
			}
		})
	}
}

func TestNoMCPTagCheckPassesBinariesWithoutTheTag(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ name, tags string }{
		{"no tags", ""},
		{"production only", "production"},
		{"tag that only starts with mcp", "production,mcpfoo"},
	} {
		t.Run(c.name, func(t *testing.T) {
			bin := buildThrowaway(t, filepath.Join(dir, strings.ReplaceAll(c.name, " ", "-"), "app"), c.tags)
			if out, err := runNoMCPTagCheck(t, bin); err != nil {
				t.Errorf("%s rejected a binary built with -tags %q: %v\n%s", noMCPTagScript, c.tags, err, out)
			}
		})
	}
}

func TestNoMCPTagCheckIgnoresMCPInThePath(t *testing.T) {
	bin := buildThrowaway(t, filepath.Join(t.TempDir(), "mcp", "pdfdebug-mcp-notes"), "production")
	if out, err := runNoMCPTagCheck(t, bin); err != nil {
		t.Errorf("%s rejected an untagged binary because its path contains mcp: %v\n%s", noMCPTagScript, err, out)
	}
}

func TestNoMCPTagCheckChecksEveryArgument(t *testing.T) {
	dir := t.TempDir()
	clean := buildThrowaway(t, filepath.Join(dir, "clean", "app"), "production")
	tagged := buildThrowaway(t, filepath.Join(dir, "tagged", "app"), "mcp")
	out, err := runNoMCPTagCheck(t, clean, tagged)
	if err == nil {
		t.Fatalf("%s passed when its second argument carries the mcp tag:\n%s", noMCPTagScript, out)
	}
	if !strings.Contains(out, tagged) {
		t.Errorf("failure output does not name the tagged binary %s:\n%s", tagged, out)
	}
}

func TestNoMCPTagCheckFailsOnAMissingBinary(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	if out, err := runNoMCPTagCheck(t, missing); err == nil {
		t.Errorf("%s passed for a binary that does not exist:\n%s", noMCPTagScript, out)
	}
}

func TestReleaseWorkflowChecksEveryShippedBinaryForTheMCPTag(t *testing.T) {
	steps := jobSteps(t, "build")
	index := func(name string) int {
		for i, s := range steps {
			if m, ok := s.(map[string]interface{}); ok && m["name"] == name {
				return i
			}
		}
		t.Fatalf("release.yml build job has no %q step", name)
		return -1
	}
	lastBuild := index("Build CLI")
	for _, name := range []string{"Build macOS GUI", "Sign macOS app bundle", "Build Windows GUI", "Build Linux GUI"} {
		if i := index(name); i > lastBuild {
			lastBuild = i
		}
	}
	stage := index("Stage artifacts")

	var runs strings.Builder
	found := 0
	for i, s := range steps {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		run, _ := m["run"].(string)
		if !strings.Contains(run, noMCPTagScript) {
			continue
		}
		found++
		runs.WriteString(run)
		if i < lastBuild || i > stage {
			t.Errorf("step %q runs the mcp tag check at position %d; it must run after every CLI and GUI build step (last at %d) and before Stage artifacts (%d)", m["name"], i, lastBuild, stage)
		}
	}
	if found == 0 {
		t.Fatalf("release.yml build job never runs %s", noMCPTagScript)
	}
	all := runs.String()
	for _, want := range []string{
		"Contents/MacOS/unidoc-pdf-debugger",
		"Contents/Resources/pdfdebug",
		"unidoc-pdf-debugger.exe",
		"bin/unidoc-pdf-debugger",
		"pdfdebug.exe",
		"bin/pdfdebug",
	} {
		if !strings.Contains(all, want) {
			t.Errorf("the mcp tag check never covers %s", want)
		}
	}
}
