package open_source_docs_test

import (
	"regexp"
	"strings"
	"testing"
)

// goModGoMinor returns the major.minor of the root go.mod `go` directive.
func goModGoMinor(t *testing.T) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^go (\d+\.\d+)`).FindStringSubmatch(readFileAtRoot(t, "go.mod"))
	if m == nil {
		t.Fatalf("go.mod has no go directive")
	}
	return m[1]
}

// goModWailsVersion returns the wails/v3 version required by the root go.mod.
func goModWailsVersion(t *testing.T) string {
	t.Helper()
	m := regexp.MustCompile(`github\.com/wailsapp/wails/v3 (v\S+)`).FindStringSubmatch(readFileAtRoot(t, "go.mod"))
	if m == nil {
		t.Fatalf("go.mod does not require github.com/wailsapp/wails/v3")
	}
	return m[1]
}

// ciGolangciLintVersion returns the golangci-lint version CI installs.
func ciGolangciLintVersion(t *testing.T) string {
	t.Helper()
	m := regexp.MustCompile(`golangci-lint@(v\d+\.\d+\.\d+)`).FindStringSubmatch(readFileAtRoot(t, ".github/workflows/ci.yml"))
	if m == nil {
		t.Fatalf(".github/workflows/ci.yml does not install a pinned golangci-lint")
	}
	return m[1]
}

var docGoVersionRe = regexp.MustCompile(`\bGo (\d+\.\d+)`)

// TestReadmeGoVersionMatchesGoMod asserts every `Go X.Y` in README names the
// go.mod Go version and Prerequisites lists it as `Go X.Y.x`.
func TestReadmeGoVersionMatchesGoMod(t *testing.T) {
	want := goModGoMinor(t)
	readme := readFileAtRoot(t, "README.md")

	for _, m := range docGoVersionRe.FindAllStringSubmatch(readme, -1) {
		if m[1] != want {
			t.Errorf("README.md says %q; go.mod pins Go %s", m[0], want)
		}
	}
	if !strings.Contains(readme, "- Go "+want+".x") {
		t.Errorf("README.md Prerequisites has no `- Go %s.x` line", want)
	}
}

var wailsPinRe = regexp.MustCompile(`\bv3\.\d+\.\d+(?:-[0-9A-Za-z]+(?:\.[0-9A-Za-z]+)*)?`)

// TestWailsPinMatchesGoModInReadmeAndContributing asserts every Wails v3
// version in README and CONTRIBUTING equals the go.mod require, README's go
// install line uses it, and README's Architecture names the same pre-release
// channel, or none for a release version.
func TestWailsPinMatchesGoModInReadmeAndContributing(t *testing.T) {
	want := goModWailsVersion(t)
	for _, f := range []string{"README.md", "CONTRIBUTING.md"} {
		content := readFileAtRoot(t, f)
		found := wailsPinRe.FindAllString(content, -1)
		if len(found) == 0 {
			t.Errorf("%s names no Wails v3 CLI version; go.mod pins %s", f, want)
		}
		for _, v := range found {
			if v != want {
				t.Errorf("%s names Wails %s; go.mod pins %s", f, v, want)
			}
		}
	}

	readme := readFileAtRoot(t, "README.md")
	if !strings.Contains(readme, "go install github.com/wailsapp/wails/v3/cmd/wails3@"+want) {
		t.Errorf("README.md has no `go install github.com/wailsapp/wails/v3/cmd/wails3@%s` line", want)
	}
	channel := ""
	if m := regexp.MustCompile(`-(alpha|beta)`).FindStringSubmatch(want); m != nil {
		channel = m[1]
	}
	if m := regexp.MustCompile(`Wails v3 \((alpha|beta)\)`).FindStringSubmatch(readme); m != nil && m[1] != channel {
		t.Errorf("README.md says %q; go.mod pins %s", m[0], want)
	}
}

// TestContributingToolchainMatchesGoModAndCI asserts the CONTRIBUTING
// Toolchain table names the go.mod Go version, and every golangci-lint
// version in CONTRIBUTING equals the one CI installs.
func TestContributingToolchainMatchesGoModAndCI(t *testing.T) {
	content := readFileAtRoot(t, "CONTRIBUTING.md")

	goMinor := goModGoMinor(t)
	if !regexp.MustCompile(`(?m)^\|\s*Go\s*\|\s*` + regexp.QuoteMeta(goMinor) + `\.x\s*\|`).MatchString(content) {
		t.Errorf("CONTRIBUTING.md Toolchain has no `| Go | %s.x |` row", goMinor)
	}

	lint := ciGolangciLintVersion(t)
	found := regexp.MustCompile(`golangci-lint\s*\|?\s*(v\d+\.\d+\.\d+)`).FindAllStringSubmatch(content, -1)
	if len(found) == 0 {
		t.Errorf("CONTRIBUTING.md names no golangci-lint version; CI installs %s", lint)
	}
	for _, m := range found {
		if v := m[1]; v != lint {
			t.Errorf("CONTRIBUTING.md says golangci-lint %s; CI installs %s", v, lint)
		}
	}
}

// TestReadmeBuildStepsNameTaskPackageWithoutEpicReference asserts README's
// build steps mention `task package` for the macOS app bundle and README
// carries no epic reference.
func TestReadmeBuildStepsNameTaskPackageWithoutEpicReference(t *testing.T) {
	readme := readFileAtRoot(t, "README.md")
	if !strings.Contains(readme, "task package") {
		t.Errorf("README.md build steps do not mention `task package`")
	}
	if m := regexp.MustCompile(`(?i)\bepic \d+`).FindString(readme); m != "" {
		t.Errorf("README.md contains internal reference %q", m)
	}
}

// TestReadmeCLIExamplesCoverCommonTasks asserts the README CLI example block
// shows `dump pages`, `validate` and `diff` alongside the dump examples.
func TestReadmeCLIExamplesCoverCommonTasks(t *testing.T) {
	lines := mdLines(readFileAtRoot(t, "README.md"))
	hs := mdHeadings(lines)
	h, ok := findHeading(hs, 3, func(s string) bool { return s == "cli" })
	if !ok {
		t.Fatalf("README.md has no ### CLI section")
	}
	var shown []string
	for _, b := range fencedBlocks(sectionLines(lines, hs, h)) {
		shown = append(shown, pdfdebugCommandLines(b)...)
	}
	all := strings.Join(shown, "\n")
	for _, want := range []string{"pdfdebug dump pages ", "pdfdebug validate ", "pdfdebug diff "} {
		if !strings.Contains(all, want) {
			t.Errorf("README.md ### CLI examples have no %q line; shown:\n%s", strings.TrimSpace(want), all)
		}
	}
}

// TestReadmeGUIUsageNamesCurrentLayout asserts the README GUI usage section
// names the left rail destinations, Object Source and every detail tab.
func TestReadmeGUIUsageNamesCurrentLayout(t *testing.T) {
	lines := mdLines(readFileAtRoot(t, "README.md"))
	hs := mdHeadings(lines)
	h, ok := findHeading(hs, 3, func(s string) bool { return s == "gui" })
	if !ok {
		t.Fatalf("README.md has no ### GUI section")
	}
	text := proseText(sectionLines(lines, hs, h))
	names := []string{
		"Structure", "Pages", "Images", "Object Source",
		"Object", "XREF", "Plain Text", "Embedded", "Metadata", "Validate", "Diff", "Signatures",
	}
	for _, n := range names {
		if !regexp.MustCompile(`\b` + regexp.QuoteMeta(n) + `\b`).MatchString(text) {
			t.Errorf("README.md ### GUI does not name %q", n)
		}
	}
}

// TestReadmeShowsBothScreenshots asserts README references the main-window and
// images-navigator screenshots with alt text, main window first, and that both
// files exist.
func TestReadmeShowsBothScreenshots(t *testing.T) {
	readme := readFileAtRoot(t, "README.md")
	at := map[string]int{}
	for _, name := range []string{"main-window.png", "images-navigator.png"} {
		loc := regexp.MustCompile(`!\[[^\]]+\]\(docs/screenshots/` + regexp.QuoteMeta(name) + `\)`).FindStringIndex(readme)
		if loc == nil {
			t.Errorf("README.md has no image reference with alt text to docs/screenshots/%s", name)
			continue
		}
		at[name] = loc[0]
		readFileAtRoot(t, "docs/screenshots/"+name)
	}
	main, okMain := at["main-window.png"]
	images, okImages := at["images-navigator.png"]
	if okMain && okImages && images < main {
		t.Errorf("README.md shows images-navigator.png before main-window.png")
	}
}

// TestContributingReleaseProcessChecksGuideAgainstBinary asserts the Release
// Process section has a step to check docs/cli-usage.md against
// `pdfdebug --help` and to link the guide pinned to the release tag, and that
// CONTRIBUTING carries no story reference.
func TestContributingReleaseProcessChecksGuideAgainstBinary(t *testing.T) {
	content := readFileAtRoot(t, "CONTRIBUTING.md")
	lines := mdLines(content)
	hs := mdHeadings(lines)
	h, ok := findHeading(hs, 2, func(s string) bool { return s == "release process" })
	if !ok {
		t.Fatalf("CONTRIBUTING.md has no ## Release Process section")
	}
	text := strings.Join(sectionText(sectionLines(lines, hs, h)), "\n")
	for _, want := range []string{"pdfdebug --help", "docs/cli-usage.md", "https://github.com/unidoc/pdfdebug/blob/vX.Y.Z/docs/cli-usage.md"} {
		if !strings.Contains(text, want) {
			t.Errorf("CONTRIBUTING.md Release Process does not contain %q", want)
		}
	}
	if m := regexp.MustCompile(`(?i)\bstory \d+-\d+`).FindString(content); m != "" {
		t.Errorf("CONTRIBUTING.md contains internal reference %q", m)
	}
}
