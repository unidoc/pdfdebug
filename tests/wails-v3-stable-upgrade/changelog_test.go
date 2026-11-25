package wails_v3_stable_upgrade_test

import (
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
	// The newest section that mentions Wails names the go.mod pin, so moving
	// the pin to a version an older section names needs a new entry.
	pin := "Wails " + goModWailsPin(t)
	newest := ""
	for _, sec := range changelogSections(t) {
		if strings.Contains(sec, "Wails") {
			newest = sec
			break
		}
	}
	if !strings.Contains(newest, pin) {
		t.Errorf("the newest CHANGELOG section that mentions Wails does not name %q", pin)
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
