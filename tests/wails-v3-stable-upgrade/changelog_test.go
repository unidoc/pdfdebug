package wails_v3_stable_upgrade_test

import (
	"strings"
	"testing"
)

// unreleasedSection returns the CHANGELOG.md text between "## [Unreleased]"
// and the next "## [" heading.
func unreleasedSection(t *testing.T) string {
	t.Helper()
	src := readRepoFile(t, "CHANGELOG.md")
	start := strings.Index(src, "## [Unreleased]")
	if start < 0 {
		t.Fatal("CHANGELOG.md has no [Unreleased] section")
	}
	rest := src[start+len("## [Unreleased]"):]
	if end := strings.Index(rest, "\n## ["); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

func TestChangelogUnreleasedRecordsTheUpgrade(t *testing.T) {
	section := unreleasedSection(t)
	pin := goModWailsPin(t)
	if !strings.Contains(section, pin) && !strings.Contains(section, strings.TrimPrefix(pin, "v")) {
		t.Errorf("CHANGELOG [Unreleased] does not name the Wails version %s", pin)
	}
	for _, want := range []string{"1.27.3", "GTK4", "Open Recent"} {
		if !strings.Contains(section, want) {
			t.Errorf("CHANGELOG [Unreleased] does not mention %q", want)
		}
	}
}
