package dependency_updates_test

import (
	"regexp"
	"strings"
	"testing"
)

func TestBuildDocsNameTheGoFloorAndItsFix(t *testing.T) {
	floor := goModFloor(t)
	for _, rel := range []string{"README.md", "CONTRIBUTING.md"} {
		doc := readRepoFile(t, rel)
		if !strings.Contains(doc, floor+" or later") {
			t.Errorf("%s must say Go \"%s or later\", the go.mod floor", rel, floor)
		}
		if !strings.Contains(doc, "GO-2026-6859") {
			t.Errorf("%s must name the net/http fix GO-2026-6859", rel)
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

// changelogLists returns the "### heading" list of every CHANGELOG.md version
// section, newest first, so an entry is found under [Unreleased] and after
// its release is cut alike.
func changelogLists(t *testing.T, heading string) []string {
	t.Helper()
	var lists []string
	for _, sec := range strings.Split(readRepoFile(t, "CHANGELOG.md"), "\n## [")[1:] {
		_, list, ok := strings.Cut(sec, "\n### "+heading+"\n")
		if !ok {
			continue
		}
		if i := strings.Index(list, "\n### "); i >= 0 {
			list = list[:i]
		}
		lists = append(lists, list)
	}
	return lists
}

// recording returns the first text holding every want, else the first holding
// wants[0], else "".
func recording(texts []string, wants ...string) string {
	first := ""
	for _, text := range texts {
		all := true
		for _, w := range wants {
			all = all && strings.Contains(text, w)
		}
		if all {
			return text
		}
		if first == "" && strings.Contains(text, wants[0]) {
			first = text
		}
	}
	return first
}

func TestChangelogRecordsTheDependencyUpdate(t *testing.T) {
	wants := []struct{ text, why string }{
		{"0.16.4", "the pdfcpu release"},
		{"1.27.4", "the Go floor"},
		{"GO-2026-6859", "the net/http advisory the Go floor fixes"},
		{"validator", "files pdfcpu's validator rejects or repairs now open as written"},
		{"PDFDocEncoding", "text strings decoded as PDFDocEncoding"},
		{"512 MiB", "the decoded-stream cap on the content stream, XMP, font and validate paths"},
	}
	texts := make([]string, len(wants))
	for i, w := range wants {
		texts[i] = w.text
	}
	changed := recording(changelogLists(t, "Changed"), texts...)
	for _, want := range wants {
		if !strings.Contains(changed, want.text) {
			t.Errorf("no CHANGELOG ### Changed list that names pdfcpu 0.16.4 has a line with %q (%s)", want.text, want.why)
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
