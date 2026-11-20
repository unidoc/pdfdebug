package pdfcpu_validation_report_test

import (
	"os"
	"path/filepath"
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

// unreleasedAdded returns the ### Added list of the [Unreleased] section.
func unreleasedAdded(t *testing.T) string {
	t.Helper()
	log := readRepoFile(t, "CHANGELOG.md")
	_, rest, ok := strings.Cut(log, "## [Unreleased]")
	if !ok {
		t.Fatal("CHANGELOG.md has no [Unreleased] section")
	}
	if i := strings.Index(rest, "\n## ["); i >= 0 {
		rest = rest[:i]
	}
	_, added, ok := strings.Cut(rest, "### Added")
	if !ok {
		t.Fatal("CHANGELOG.md [Unreleased] has no ### Added list")
	}
	if i := strings.Index(added, "\n### "); i >= 0 {
		added = added[:i]
	}
	return added
}

func TestChangelog_AddedLineForTheBackgroundCheckAndProfile(t *testing.T) {
	var line string
	for l := range strings.SplitSeq(unreleasedAdded(t), "\n") {
		if strings.Contains(l, "validate --profile pdfcpu") {
			line = l
		}
	}
	if line == "" {
		t.Fatal("CHANGELOG.md [Unreleased] ### Added needs a line naming `validate --profile pdfcpu`")
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

func TestProjectContext_ListsThePdfcpuProfile(t *testing.T) {
	path := filepath.Join(projectRoot(t), "_bmad-output", "project-context.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("project-context.md not reachable: %v", err)
	}
	doc := string(data)
	for _, want := range []string{"pdfcpu-validator", "pdfcpu-notices"} {
		if !strings.Contains(doc, want) {
			t.Errorf("project-context.md must describe the pdfcpu profile (%q)", want)
		}
	}
	if strings.Contains(doc, "a separate, on-demand parse") {
		t.Errorf("project-context.md still says validator findings come from a separate, on-demand parse")
	}
}
