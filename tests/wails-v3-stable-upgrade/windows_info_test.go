package wails_v3_stable_upgrade_test

import (
	"encoding/json"
	"regexp"
	"testing"
)

// releaseInfoKey matches the language key the release workflow's jq filter
// writes ProductVersion under, as in `.info["0409"].ProductVersion = $v`.
var releaseInfoKey = regexp.MustCompile(`\.info\["([0-9A-Fa-f]{4})"\]\.ProductVersion\s*=\s*\$v`)

// The release stamps its version into build/windows/info.json with jq. jq
// creates a missing key instead of failing, so a key that differs from the one
// in info.json adds a second, partial language block and leaves the real one
// at the static fallback version.
func TestReleaseStampsTheVersionUnderTheInfoJSONLanguageKey(t *testing.T) {
	m := releaseInfoKey.FindStringSubmatch(readRepoFile(t, ".github/workflows/release.yml"))
	if m == nil {
		t.Fatal("release.yml has no jq filter that sets .info[\"<lang>\"].ProductVersion")
	}
	var info struct {
		Info map[string]map[string]string `json:"info"`
	}
	if err := json.Unmarshal([]byte(readRepoFile(t, "build/windows/info.json")), &info); err != nil {
		t.Fatalf("parse build/windows/info.json: %v", err)
	}
	if len(info.Info) != 1 {
		t.Fatalf("build/windows/info.json has %d language blocks, want exactly one", len(info.Info))
	}
	block, ok := info.Info[m[1]]
	if !ok {
		t.Fatalf("release.yml stamps .info[%q] but build/windows/info.json has no such language block", m[1])
	}
	if block["ProductVersion"] == "" {
		t.Fatalf("build/windows/info.json .info[%q] has no ProductVersion", m[1])
	}
}
