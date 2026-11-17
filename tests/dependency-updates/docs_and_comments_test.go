package dependency_updates_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildDocsNameTheGoFloorAndItsFix(t *testing.T) {
	for _, rel := range []string{"README.md", "CONTRIBUTING.md"} {
		doc := readRepoFile(t, rel)
		if !strings.Contains(doc, "1.27.4 or later") {
			t.Errorf("%s must say Go \"1.27.4 or later\"", rel)
		}
		if strings.Contains(doc, "1.27.3 or later") {
			t.Errorf("%s still says Go \"1.27.3 or later\"", rel)
		}
		if !strings.Contains(doc, "GO-2026-6859") {
			t.Errorf("%s must name the net/http fix GO-2026-6859", rel)
		}
	}
}

// unreleasedChanged returns the `### Changed` section of `## [Unreleased]`.
func unreleasedChanged(t *testing.T) string {
	t.Helper()
	log := readRepoFile(t, "CHANGELOG.md")
	start := strings.Index(log, "## [Unreleased]")
	if start < 0 {
		t.Fatal("CHANGELOG.md has no [Unreleased] section")
	}
	unreleased := log[start+len("## [Unreleased]"):]
	if end := strings.Index(unreleased, "\n## "); end >= 0 {
		unreleased = unreleased[:end]
	}
	i := strings.Index(unreleased, "### Changed")
	if i < 0 {
		t.Fatal("CHANGELOG.md [Unreleased] has no ### Changed section")
	}
	changed := unreleased[i+len("### Changed"):]
	if end := strings.Index(changed, "\n### "); end >= 0 {
		changed = changed[:end]
	}
	return changed
}

func TestChangelogRecordsTheDependencyUpdate(t *testing.T) {
	changed := unreleasedChanged(t)
	for _, want := range []struct{ text, why string }{
		{"1.27.4", "the Go floor"},
		{"GO-2026-6859", "the net/http advisory the Go floor fixes"},
		{"0.16.4", "the pdfcpu release"},
		{"validator", "files pdfcpu's validator rejects or repairs now open as written"},
		{"PDFDocEncoding", "text strings decoded as PDFDocEncoding"},
		{"512 MiB", "the decoded-stream cap on the content stream, XMP, font and validate paths"},
	} {
		if !strings.Contains(changed, want.text) {
			t.Errorf("CHANGELOG [Unreleased] ### Changed has no line with %q (%s)", want.text, want.why)
		}
	}
	if strings.Contains(readRepoFile(t, "CHANGELOG.md"), "### Security") {
		t.Errorf("CHANGELOG.md has a ### Security heading; the entries go under ### Changed")
	}
}

// Comments that describe pdfcpu's behaviour describe the release in use.
func TestCommentsDescribeTheCurrentPdfcpu(t *testing.T) {
	for _, tc := range []struct{ file, stale, why string }{
		{"main.go", "ReadContextFile", "Open parses with ReadContext"},
		{"internal/pdfcore/inspector_test.go", "ReadContextFile", "Open parses with ReadContext"},
		{"internal/pdfcore/inspector_test.go", "hard to craft", "a root /Pages with no /Count now reaches the partial-success path"},
		{"internal/pdfcore/metadata_test.go", "ReadContextFile", "Open parses with ReadContext"},
		{"internal/pdfcore/metadata_test.go", "STRICTLY decodes", "Open does not decode /Metadata"},
		{"internal/pdfcore/metadata.go", "unreachable through Inspector.Open", "an undecodable /Metadata reaches GetDocumentMetadata"},
		{"internal/pdfcore/embedded_test.go", "ReadContextFile", "Open parses with ReadContext"},
		{"tests/error-handling/error_handling_test.go", "ReadContextFile", "Open parses with ReadContext"},
		{"testdata/correctness/README.md", "ReadContextFile", "Open parses with ReadContext"},
		{"internal/pdfcore/font.go", "pdfcpu v0.12", "the comment names a pdfcpu release the module no longer uses"},
		{"internal/pdfcore/textstring.go", "Latin-1", "text strings decode as PDFDocEncoding"},
		{"internal/pdfcore/textstring.go", "quirk", "a 0x5C byte in a hex string is kept"},
		{"internal/pdfcore/image.go", "dereferences without checking", "the colour-space lookup returns errors for the shapes that faulted it"},
		{"internal/pdfcore/boundeddecode_test.go", "the same decoder pdfcpu wraps", "LZW is counted through internal/lzwdecode"},
		{"internal/pdfcore/boundeddecode_test.go", "hhrutter", "LZW is counted through internal/lzwdecode"},
		{"tests/naming-and-comment-hygiene/naming_test.go", "bump_test.go has to stay legal", "the renamed pin suite"},
	} {
		if strings.Contains(readRepoFile(t, tc.file), tc.stale) {
			t.Errorf("%s still says %q; %s", tc.file, tc.stale, tc.why)
		}
	}
}

// The project context lives in the docs repo, reached through the
// _bmad-output symlink; the check is skipped where that is not checked out.
func TestProjectContextDescribesTheCurrentDependencies(t *testing.T) {
	path := filepath.Join(projectRoot(t), "_bmad-output", "project-context.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("project context not available: %v", err)
	}
	ctx := string(data)
	for _, want := range []string{"1.27.4", "v0.16.4", "internal/lzwdecode", "decodeStream", "PDFDocEncoding", "512 MiB"} {
		if !strings.Contains(ctx, want) {
			t.Errorf("project-context.md does not mention %q", want)
		}
	}
	for _, stale := range []string{"v0.12.1", "hhrutter/lzw", "sd.Decode()", "still fails at open"} {
		if strings.Contains(ctx, stale) {
			t.Errorf("project-context.md still says %q", stale)
		}
	}
}
