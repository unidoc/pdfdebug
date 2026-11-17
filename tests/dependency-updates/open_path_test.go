package dependency_updates_test

import (
	"path/filepath"
	"strings"
	"testing"
)

// Inspector.Open parses the file with pdfcpu's reader and does not run
// pdfcpu's validator.
func TestInspectorOpenParsesWithoutValidating(t *testing.T) {
	src := readRepoFile(t, "internal/pdfcore/inspector.go")
	if strings.Contains(src, "ReadContextFile(") {
		t.Errorf("internal/pdfcore/inspector.go still opens with ReadContextFile, which runs pdfcpu's validator")
	}
	if !strings.Contains(src, "pdfcpu_api.ReadContext(") {
		t.Errorf("internal/pdfcore/inspector.go must parse with pdfcpu_api.ReadContext")
	}
	if strings.Contains(src, "ValidateContext(") {
		t.Errorf("internal/pdfcore/inspector.go must not run pdfcpu's validator")
	}
}

// The test helpers that sanity-check a fixture accept what Open accepts.
func TestFixtureHelpersCheckWithTheOpenParser(t *testing.T) {
	for _, rel := range []string{
		"internal/pdfcore/resolve_ref_atdd_test.go",
		"internal/pdfcore/diff_test.go",
	} {
		if strings.Contains(readRepoFile(t, rel), "ReadContextFile(") {
			t.Errorf("%s sanity-checks fixtures with ReadContextFile, which is stricter than Open; use ReadContext", rel)
		}
	}
}

func TestDumpStructureOnCyclicStructureTreeExitsZero(t *testing.T) {
	bin := buildCLI(t)
	file := filepath.Join(projectRoot(t), "testdata", "structure-broken.pdf")
	stdout, stderr, code := runCLI(t, bin, "dump", "structure", file)
	if code != 0 {
		t.Fatalf("dump structure exit %d, want 0\nstderr: %s", code, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Errorf("dump structure printed nothing")
	}
}

func TestEncryptedAndMalformedFixturesStillExitTwo(t *testing.T) {
	bin := buildCLI(t)
	for _, tc := range []struct{ file, want string }{
		{"encrypted.pdf", "encrypted PDF"},
		{"malformed.pdf", "malformed PDF"},
	} {
		stdout, stderr, code := runCLI(t, bin, "dump", "xref", filepath.Join(projectRoot(t), "testdata", tc.file))
		if code != 2 {
			t.Errorf("%s: exit %d, want 2", tc.file, code)
		}
		if !strings.Contains(stdout+stderr, tc.want) {
			t.Errorf("%s: output does not name %q:\n%s%s", tc.file, tc.want, stdout, stderr)
		}
	}
}

// A file pdfcpu's validator refuses opens in the CLI too: a /Kids cycle lists
// its pages and exits 0.
func TestDumpPagesOnKidsCycleExitsZero(t *testing.T) {
	bin := buildCLI(t)
	pdf := classicPDF(
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] >>",
		"<< /Type /Pages /Parent 2 0 R /Kids [2 0 R] /Count 1 >>",
	)
	_, stderr, code := runCLI(t, bin, "dump", "pages", writeTemp(t, "kids-cycle.pdf", pdf))
	if code != 0 {
		t.Errorf("dump pages on a /Kids cycle exit %d, want 0\nstderr: %s", code, stderr)
	}
}
