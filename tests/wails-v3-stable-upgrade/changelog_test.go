package wails_v3_stable_upgrade_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// changelogSections returns CHANGELOG.md's version sections, newest first,
// each from its "## [" heading to the next, so an entry is found under
// [Unreleased] and after its release is cut alike.
func changelogSections(t *testing.T) []string {
	t.Helper()
	return strings.Split(readRepoFile(t, "CHANGELOG.md"), "\n## [")[1:]
}

func TestChangelogRecordsTheUpgrade(t *testing.T) {
	// The newest section with a line that mentions Wails and a vX.Y.Z version
	// names exactly the go.mod pin among those versions, so a bump written
	// "old -> new" passes, moving the pin needs a new entry, and a passing
	// mention of Wails without a version is not taken for the upgrade entry.
	pin := goModWailsPin(t)
	version := regexp.MustCompile(`\bv\d+\.\d+\.\d+(?:-[0-9A-Za-z]+(?:\.[0-9A-Za-z]+)*)?`)
	var named []string
	for _, sec := range changelogSections(t) {
		for _, line := range strings.Split(sec, "\n") {
			if strings.Contains(line, "Wails") {
				named = append(named, version.FindAllString(line, -1)...)
			}
		}
		if len(named) > 0 {
			break
		}
	}
	if !slices.Contains(named, pin) {
		t.Errorf("the newest CHANGELOG section that names a Wails version names %q, want the go.mod pin %q among them", named, pin)
	}

	wants := []string{"GTK4", "Open Recent", "1.27.3"}
	for _, sec := range changelogSections(t) {
		all := true
		for _, w := range wants {
			all = all && strings.Contains(sec, w)
		}
		if all {
			return
		}
	}
	t.Errorf("no CHANGELOG version section mentions all of %q", wants)
}
