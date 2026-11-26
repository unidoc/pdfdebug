package dependency_updates_test

import (
	"regexp"
	"strings"
	"testing"
)

// The build docs name the go.mod floor as "<floor> or later", and no other
// three-part version "or later" is left behind; two-part versions such as an
// Ubuntu release are not Go floors and are ignored.
func TestBuildDocsNameTheGoFloor(t *testing.T) {
	floor := goModFloor(t)
	orLater := regexp.MustCompile(`\b(\d+\.\d+\.\d+) or later`)
	for _, rel := range []string{"README.md", "CONTRIBUTING.md"} {
		doc := readRepoFile(t, rel)
		if !strings.Contains(doc, floor+" or later") {
			t.Errorf("%s must say Go \"%s or later\", the go.mod floor", rel, floor)
		}
		for _, m := range orLater.FindAllStringSubmatch(doc, -1) {
			if m[1] != floor {
				t.Errorf("%s still says %q; the go.mod floor is %s", rel, m[0], floor)
			}
		}
	}
}

// goModFloor returns the version in the root go.mod go directive.
func goModFloor(t *testing.T) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^go (\S+)$`).FindStringSubmatch(readRepoFile(t, "go.mod"))
	if m == nil {
		t.Fatal("go.mod has no go directive")
	}
	return m[1]
}

// The dependency update is recorded in the ### Changed list of the CHANGELOG
// version section that records the update to pdfcpu 0.16.4, under
// [Unreleased] or after its release is cut, and that section has no
// ### Security list. A later update away from 0.16.4 does not match.
func TestChangelogRecordsTheDependencyUpdate(t *testing.T) {
	var section, changed string
	for _, sec := range strings.Split(readRepoFile(t, "CHANGELOG.md"), "\n## [")[1:] {
		_, list, ok := strings.Cut(sec, "\n### Changed\n")
		if !ok {
			continue
		}
		if i := strings.Index(list, "\n### "); i >= 0 {
			list = list[:i]
		}
		if strings.Contains(list, "to 0.16.4") {
			section, changed = sec, list
			break
		}
	}
	if section == "" {
		t.Fatal("no CHANGELOG version section has a ### Changed list that records the update to pdfcpu 0.16.4")
	}
	for _, want := range []struct{ text, why string }{
		{"1.27.4", "the Go floor"},
		{"GO-2026-6859", "the net/http advisory the Go floor fixes"},
		{"validator", "files pdfcpu's validator rejects or repairs now open as written"},
		{"PDFDocEncoding", "text strings decoded as PDFDocEncoding"},
		{"512 MiB", "the decoded-stream cap on the content stream, XMP, font and validate paths"},
	} {
		if !strings.Contains(changed, want.text) {
			t.Errorf("the ### Changed list that records the update to pdfcpu 0.16.4 has no line with %q (%s)", want.text, want.why)
		}
	}
	if strings.Contains(section, "\n### Security\n") {
		t.Errorf("the CHANGELOG section that records the update to pdfcpu 0.16.4 has a ### Security list; the entries go under ### Changed")
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
