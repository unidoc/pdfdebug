package pdfcpu_validation_report_test

import (
	"strings"
	"testing"
)

func TestDocs_ValidateGuideDocumentsThePdfcpuProfile(t *testing.T) {
	doc := readRepoFile(t, "docs/cli/validate-and-diff.md")
	low := strings.ToLower(doc)
	for _, want := range []string{
		"[--profile pdfa-1b|pdfua-1-structural|pdfcpu]",
		"pdfcpu-validator",
		"pdfcpu-notices",
		"separate parse",
		"/subtype",
	} {
		if !strings.Contains(low, strings.ToLower(want)) {
			t.Errorf("docs/cli/validate-and-diff.md must mention %q", want)
		}
	}
	if !strings.Contains(low, "partial") && !strings.Contains(low, "incomplete") {
		t.Errorf("docs/cli/validate-and-diff.md must say the pdfcpu notice list is partial")
	}
}

func TestDocs_ProfileListsNameThePdfcpuProfile(t *testing.T) {
	for _, c := range []struct {
		file string
		want string
	}{
		{"docs/cli-usage.md", "--profile pdfcpu"},
		{"docs/cli/scripting.md", "pdfcpu-validator"},
		{"README.md", "--profile pdfcpu"},
	} {
		if !strings.Contains(readRepoFile(t, c.file), c.want) {
			t.Errorf("%s lists the other profiles and must name the pdfcpu profile (%q)", c.file, c.want)
		}
	}
}

// changelogAddedLines returns the lines of every CHANGELOG.md version
// section's ### Added list, newest first, so the entry is found under
// [Unreleased] and after its release is cut alike.
func changelogAddedLines(t *testing.T) []string {
	t.Helper()
	var lines []string
	for _, sec := range strings.Split(readRepoFile(t, "CHANGELOG.md"), "\n## [")[1:] {
		_, added, ok := strings.Cut(sec, "\n### Added\n")
		if !ok {
			continue
		}
		if i := strings.Index(added, "\n### "); i >= 0 {
			added = added[:i]
		}
		lines = append(lines, strings.Split(added, "\n")...)
	}
	return lines
}

func TestChangelog_AddedLineForTheBackgroundCheckAndProfile(t *testing.T) {
	var line string
	for _, l := range changelogAddedLines(t) {
		if strings.Contains(l, "validate --profile pdfcpu") {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatal("no CHANGELOG.md ### Added list has a line naming `validate --profile pdfcpu`")
	}
	low := strings.ToLower(line)
	for _, want := range []string{"banner", "details"} {
		if !strings.Contains(low, want) {
			t.Errorf("changelog line must mention the %s: %q", want, line)
		}
	}
	if !strings.Contains(low, "partial") && !strings.Contains(low, "incomplete") && !strings.Contains(low, "not every") {
		t.Errorf("changelog line must say the list is partial: %q", line)
	}
}
