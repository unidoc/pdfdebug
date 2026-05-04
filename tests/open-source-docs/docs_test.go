// Package open_source_docs_test provides acceptance tests for Story 7.3:
// Open-Source Project Setup.
//
// These tests verify that the repository ships the open-source documentation
// deliverables:
//   - LICENSE  (rewritten to canonical Apache 2.0 with UniDoc copyright substitution)
//   - NOTICE   (expanded with UniDoc ehf. attribution + 8 mandated third-party deps)
//   - README.md (8 required H2 sections in order, screenshot ref, install/build/usage)
//   - CONTRIBUTING.md (6 required H2 sections in order, test commands, release process)
//   - scripts/verify-license.sh (executable, wired into .github/workflows/ci.yml)
//   - scripts/fixtures/apache-2.0.txt, apache-2.0-with-copyright.txt
//
// These are TDD RED PHASE tests -- they MUST fail until Story 7.3 is implemented.
// No t.Skip() sentinels (per story 7-1 and 7-2 dev-story outcome: the repo ships
// TDD-red tests directly).
//
// Test Levels: Static (Go) -- pure filesystem + string grep checks. No YAML
// parsing dependency; no external modules. The test design scenarios
// 7.3-STATIC-001 through 7.3-STATIC-006 are covered by the 14 test functions
// below (multiple tests per scenario for granularity).
//
// Trace (AC -> test):
//   AC #1 (all four files present)             -> TestVerifyLicenseScriptExistsAndExecutable
//                                                  (via fixtures + presence checks across other tests)
//   AC #2 (LICENSE byte-matches canonical)     -> TestLicenseMatchesCanonicalWithSubstitution,
//                                                  TestApache20FixtureIsCanonical,
//                                                  TestLicenseCopyrightHasUniDocAttribution
//   AC #3 (NOTICE attribution)                 -> TestNoticeHasRequiredAttributions,
//                                                  TestNoticeHasNoUnicodeCopyright
//   AC #4 (README H2 sections in order)        -> TestReadmeHasRequiredSections
//   AC #5 (README screenshot)                  -> TestReadmeHasScreenshotReference
//   AC #6 (README Installation subsections)    -> TestReadmeHasInstallationSubsections
//   AC #7 (README Build-from-Source cmds)      -> TestReadmeBuildFromSourceHasAllCommands
//   AC #8 (CONTRIBUTING structure)             -> TestContributingHasRequiredSections,
//                                                  TestContributingRunningTestsHasAllCommands,
//                                                  TestContributingReleaseProcessHasAppleCertRotation
//   AC #9 (CI wire-up)                         -> TestVerifyLicenseScriptExistsAndExecutable,
//                                                  TestCIWorkflowReferencesVerifyScript
//
// Test design scenarios (epic-7-test-design.md):
//   7.3-STATIC-001 -> TestLicenseMatchesCanonicalWithSubstitution,
//                     TestApache20FixtureIsCanonical,
//                     TestLicenseCopyrightHasUniDocAttribution
//   7.3-STATIC-002 -> TestNoticeHasRequiredAttributions,
//                     TestNoticeHasNoUnicodeCopyright
//   7.3-STATIC-003 -> TestReadmeHasRequiredSections,
//                     TestReadmeHasScreenshotReference,
//                     TestReadmeHasInstallationSubsections
//   7.3-STATIC-004 -> TestContributingHasRequiredSections
//   7.3-STATIC-005 -> TestReadmeBuildFromSourceHasAllCommands,
//                     TestContributingRunningTestsHasAllCommands,
//                     TestContributingReleaseProcessHasAppleCertRotation
//   7.3-STATIC-006 -> TestVerifyLicenseScriptExistsAndExecutable,
//                     TestCIWorkflowReferencesVerifyScript
//
// Run: cd tests/open-source-docs && go test -v -count=1 ./...
package open_source_docs_test

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// projectRoot returns the absolute path to the project root directory.
// It walks upward from the test file location to find the project root,
// identified by the presence of a go.mod whose module name is "unidoc-pdf-debugger".
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if content, err := os.ReadFile(goModPath); err == nil {
			if strings.Contains(string(content), "module unidoc-pdf-debugger") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root (no go.mod with module unidoc-pdf-debugger found)")
		}
		dir = parent
	}
}

// readFileAtRoot reads a file relative to project root, failing the test if absent.
func readFileAtRoot(t *testing.T, relPath string) string {
	t.Helper()
	root := projectRoot(t)
	content, err := os.ReadFile(filepath.Join(root, relPath))
	if err != nil {
		t.Fatalf("%s not found: %v", relPath, err)
	}
	return string(content)
}

// ---------------------------------------------------------------------------
// 7.3-STATIC-001 (P0): LICENSE byte-matches canonical Apache 2.0 text
// Covers AC #2 (Task 1)
// ---------------------------------------------------------------------------

// TestLicenseMatchesCanonicalWithSubstitution asserts that LICENSE at repo root
// is byte-identical to scripts/fixtures/apache-2.0-with-copyright.txt (the
// canonical Apache 2.0 text with the documented UniDoc copyright substitution
// applied). This is the primary byte-match contract per Epic 7 risk E7-R-004.
func TestLicenseMatchesCanonicalWithSubstitution(t *testing.T) {
	license := readFileAtRoot(t, "LICENSE")
	fixture := readFileAtRoot(t, "scripts/fixtures/apache-2.0-with-copyright.txt")

	if license != fixture {
		// One-line diff hint: report first differing byte offset and the
		// surrounding 40-byte window from each file.
		minLen := len(license)
		if len(fixture) < minLen {
			minLen = len(fixture)
		}
		var diffIdx = -1
		for i := 0; i < minLen; i++ {
			if license[i] != fixture[i] {
				diffIdx = i
				break
			}
		}
		if diffIdx == -1 {
			t.Fatalf("LICENSE and fixture differ in length: LICENSE=%d bytes, fixture=%d bytes",
				len(license), len(fixture))
		}
		start := diffIdx - 20
		if start < 0 {
			start = 0
		}
		endL := diffIdx + 20
		if endL > len(license) {
			endL = len(license)
		}
		endF := diffIdx + 20
		if endF > len(fixture) {
			endF = len(fixture)
		}
		t.Fatalf("LICENSE does not match canonical Apache 2.0 + UniDoc substitution at byte %d\n"+
			"  LICENSE  : %q\n"+
			"  fixture  : %q\n"+
			"  hint     : run `diff scripts/fixtures/apache-2.0-with-copyright.txt LICENSE`",
			diffIdx, license[start:endL], fixture[start:endF])
	}
}

// TestApache20FixtureIsCanonical asserts that scripts/fixtures/apache-2.0.txt
// contains the canonical Apache 2.0 sentinel lines. This verifies the UNMODIFIED
// upstream fixture is committed (for audit), distinct from the -with-copyright
// variant that LICENSE is diffed against. CI must be offline-safe so we do NOT
// re-fetch upstream; we just sentinel-check the committed fixture.
func TestApache20FixtureIsCanonical(t *testing.T) {
	content := readFileAtRoot(t, "scripts/fixtures/apache-2.0.txt")

	sentinels := []string{
		"Apache License",
		"Version 2.0, January 2004",
		"http://www.apache.org/licenses/",
		"Copyright [yyyy] [name of copyright owner]", // unmodified -- substitution NOT applied here
	}
	for _, s := range sentinels {
		if !strings.Contains(content, s) {
			t.Errorf("scripts/fixtures/apache-2.0.txt missing canonical sentinel: %q", s)
		}
	}

	// Byte length sanity check: canonical Apache 2.0 is ~11,358 bytes / 202 lines.
	// Allow +/- 200 bytes for line-ending variance; the fixture MUST NOT be the
	// divergent 200-line current-repo variant (which would be ~11,100 bytes with
	// different content, but still within range -- so the sentinel grep above is
	// the real gate).
	if len(content) < 10_000 || len(content) > 12_000 {
		t.Errorf("scripts/fixtures/apache-2.0.txt has unexpected size %d bytes (expected ~11,358)",
			len(content))
	}
}

// TestLicenseCopyrightHasUniDocAttribution asserts that LICENSE contains the
// documented substitution line `Copyright 2026 UniDoc ehf.` replacing the
// canonical `Copyright [yyyy] [name of copyright owner]` placeholder.
func TestLicenseCopyrightHasUniDocAttribution(t *testing.T) {
	content := readFileAtRoot(t, "LICENSE")

	if !strings.Contains(content, "Copyright 2026 UniDoc ehf.") {
		t.Errorf("LICENSE missing `Copyright 2026 UniDoc ehf.` substitution (Task 1.2)")
	}
	// The unmodified placeholder MUST be gone (otherwise the substitution was not applied).
	if strings.Contains(content, "Copyright [yyyy] [name of copyright owner]") {
		t.Errorf("LICENSE still contains canonical placeholder `Copyright [yyyy] [name of copyright owner]` -- substitution not applied")
	}
}

// ---------------------------------------------------------------------------
// 7.3-STATIC-002 (P0): NOTICE has UniDoc + third-party attributions
// Covers AC #3 (Task 2)
// ---------------------------------------------------------------------------

// TestNoticeHasRequiredAttributions asserts NOTICE contains UniDoc ehf.,
// ASCII (c), Apache License reference, and each of the 8 mandated third-party
// attribution substrings.
func TestNoticeHasRequiredAttributions(t *testing.T) {
	content := readFileAtRoot(t, "NOTICE")

	required := []string{
		"UniDoc ehf.",     // AC #3 UniDoc attribution
		"(c)",             // ASCII copyright marker (project ASCII-only rule)
		"Apache License",  // Apache 2.0 reference
		"pdfcpu",          // Go runtime dep #1
		"wails",           // Go runtime dep #2 (matches "wails" or "wails v3")
		"@wailsio/runtime",// Frontend dep
		"react",           // Frontend dep (matches react, react-dom)
		"@radix-ui",       // Frontend dep (matches @radix-ui/*)
		"react-arborist",  // Frontend dep
		"allotment",       // Frontend dep
		"tailwindcss",     // Frontend dep (devDep but shipped in bundle)
	}
	for _, s := range required {
		if !strings.Contains(content, s) {
			t.Errorf("NOTICE missing required substring: %q", s)
		}
	}
}

// TestNoticeHasNoUnicodeCopyright asserts NOTICE does NOT contain the Unicode
// copyright symbol. Project CLAUDE.md ASCII-only rule requires ASCII `(c)`.
func TestNoticeHasNoUnicodeCopyright(t *testing.T) {
	content := readFileAtRoot(t, "NOTICE")

	// U+00A9 COPYRIGHT SIGN. The UTF-8 encoding is 0xC2 0xA9.
	if strings.ContainsRune(content, '\u00A9') {
		t.Errorf("NOTICE contains Unicode copyright symbol (U+00A9) -- use ASCII `(c)` per project ASCII-only rule")
	}
}

// ---------------------------------------------------------------------------
// 7.3-STATIC-003 (P1): README.md has required sections + screenshot + install
// Covers AC #4, AC #5, AC #6 (Task 3)
// ---------------------------------------------------------------------------

// TestReadmeHasRequiredSections asserts all 8 H2 sections from AC #4 are
// present in README.md IN ORDER. Uses a simple left-to-right scan.
func TestReadmeHasRequiredSections(t *testing.T) {
	content := readFileAtRoot(t, "README.md")

	sections := []string{
		"## Overview",
		"## Screenshot",
		"## Installation",
		"## Build from Source",
		"## Usage",
		"## Architecture",
		"## Contributing",
		"## License",
	}
	cursor := 0
	for _, sec := range sections {
		idx := strings.Index(content[cursor:], sec)
		if idx == -1 {
			// Detect presence-but-out-of-order vs totally missing for clearer diagnostic.
			if strings.Contains(content, sec) {
				t.Errorf("README.md section %q is present but out of order (expected order: %v)",
					sec, sections)
			} else {
				t.Errorf("README.md missing required section: %q", sec)
			}
			return // bail early -- downstream order checks are meaningless once one is missing
		}
		cursor += idx + len(sec)
	}
}

// TestReadmeHasScreenshotReference asserts that the Screenshot section either
// references a markdown image in docs/screenshots/ OR contains the AC #5
// placeholder TODO comment (opt-out clause for pre-release).
func TestReadmeHasScreenshotReference(t *testing.T) {
	content := readFileAtRoot(t, "README.md")

	// Regex matches ![alt text](docs/screenshots/<file>.(png|jpg))
	imagePattern := regexp.MustCompile(`!\[[^\]]*\]\(docs/screenshots/[^)]+\.(png|jpg|jpeg)\)`)
	hasImage := imagePattern.MatchString(content)

	// Placeholder TODO comment path (AC #5 opt-out)
	hasTODO := strings.Contains(content, "<!-- TODO: replace with real screenshot")

	if !hasImage && !hasTODO {
		t.Errorf("README.md Screenshot section must contain either a markdown image reference " +
			"matching `![...](docs/screenshots/*.png|jpg)` OR the `<!-- TODO: replace with real screenshot...` " +
			"placeholder comment (AC #5)")
	}
}

// TestReadmeHasInstallationSubsections asserts that the ## Installation section
// references the V1 distribution channels per AC #6: GitHub Releases and
// Build from Source.
func TestReadmeHasInstallationSubsections(t *testing.T) {
	content := readFileAtRoot(t, "README.md")

	// Narrow the search window to just the Installation section to avoid
	// false positives from unrelated earlier/later mentions.
	installStart := strings.Index(content, "## Installation")
	if installStart == -1 {
		t.Fatalf("README.md missing `## Installation` section (prerequisite for this test)")
	}
	// Section ends at the next H2 heading (any line starting with "## ") or EOF.
	sectionBody := content[installStart:]
	// Skip past the `## Installation` header itself to find the NEXT H2.
	remainder := sectionBody[len("## Installation"):]
	if next := strings.Index(remainder, "\n## "); next != -1 {
		sectionBody = sectionBody[:len("## Installation")+next]
	}

	required := []string{
		"github.com/unidoc/pdfdebug/releases", // GitHub Releases link
		"Build from Source",                   // reference to build section
	}
	for _, s := range required {
		if !strings.Contains(sectionBody, s) {
			t.Errorf("README.md `## Installation` section missing required substring: %q", s)
		}
	}
}

// ---------------------------------------------------------------------------
// 7.3-STATIC-005 (P1): README Build-from-Source has all AC #7 commands
// Covers AC #7 (Task 3.5)
// ---------------------------------------------------------------------------

// TestReadmeBuildFromSourceHasAllCommands asserts that the ## Build from Source
// section contains every command listed in AC #7.
func TestReadmeBuildFromSourceHasAllCommands(t *testing.T) {
	content := readFileAtRoot(t, "README.md")

	buildStart := strings.Index(content, "## Build from Source")
	if buildStart == -1 {
		t.Fatalf("README.md missing `## Build from Source` section (prerequisite for this test)")
	}
	sectionBody := content[buildStart:]
	remainder := sectionBody[len("## Build from Source"):]
	if next := strings.Index(remainder, "\n## "); next != -1 {
		sectionBody = sectionBody[:len("## Build from Source")+next]
	}

	required := []string{
		"git clone",                  // AC #7 step 1
		"@v3.0.0-alpha.74",           // Wails CLI pin suffix -- matches both short and full forms
		"npm ci --prefix frontend",   // AC #7 step 4
		"wails3 generate bindings",   // AC #7 step 5 (bindings prereq)
		"wails3 dev",                 // AC #7 step 6 (interactive dev)
		"go build -o bin/pdfdebug ./cmd/cli", // AC #7 step 7 (CLI build)
	}
	for _, s := range required {
		if !strings.Contains(sectionBody, s) {
			t.Errorf("README.md `## Build from Source` section missing required command: %q", s)
		}
	}
}

// ---------------------------------------------------------------------------
// 7.3-STATIC-004 (P1): CONTRIBUTING.md has required sections in order
// Covers AC #8 (Task 4)
// ---------------------------------------------------------------------------

// TestContributingHasRequiredSections asserts all 6 H2 sections from AC #8 are
// present in CONTRIBUTING.md IN ORDER.
func TestContributingHasRequiredSections(t *testing.T) {
	content := readFileAtRoot(t, "CONTRIBUTING.md")

	sections := []string{
		"## Development Environment",
		"## Running Tests",
		"## Code Style",
		"## Submitting Pull Requests",
		"## Release Process",
		"## Reporting Issues",
	}
	cursor := 0
	for _, sec := range sections {
		idx := strings.Index(content[cursor:], sec)
		if idx == -1 {
			if strings.Contains(content, sec) {
				t.Errorf("CONTRIBUTING.md section %q is present but out of order (expected order: %v)",
					sec, sections)
			} else {
				t.Errorf("CONTRIBUTING.md missing required section: %q", sec)
			}
			return
		}
		cursor += idx + len(sec)
	}
}

// TestContributingRunningTestsHasAllCommands asserts that ## Running Tests
// section documents every test command per AC #8.
func TestContributingRunningTestsHasAllCommands(t *testing.T) {
	content := readFileAtRoot(t, "CONTRIBUTING.md")

	sectionStart := strings.Index(content, "## Running Tests")
	if sectionStart == -1 {
		t.Fatalf("CONTRIBUTING.md missing `## Running Tests` section (prerequisite for this test)")
	}
	sectionBody := content[sectionStart:]
	remainder := sectionBody[len("## Running Tests"):]
	if next := strings.Index(remainder, "\n## "); next != -1 {
		sectionBody = sectionBody[:len("## Running Tests")+next]
	}

	required := []string{
		"go test ./...",                    // Go unit tests
		"tests/*/go.mod",                   // per-suite loop pattern (literal)
		"npm run test --prefix frontend",   // Vitest
		"npm run lint --prefix frontend",   // ESLint
		"npm run typecheck --prefix frontend", // tsc
		"golangci-lint run ./...",          // Go lint
	}
	for _, s := range required {
		if !strings.Contains(sectionBody, s) {
			t.Errorf("CONTRIBUTING.md `## Running Tests` section missing required command: %q", s)
		}
	}
}

// TestContributingReleaseProcessHasAppleCertRotation asserts that
// ## Release Process references the Apple Developer ID cert rotation secret
// name (documented in story 7-2 Task 2.5, deferred from 7-2 Dev Notes).
func TestContributingReleaseProcessHasAppleCertRotation(t *testing.T) {
	content := readFileAtRoot(t, "CONTRIBUTING.md")

	sectionStart := strings.Index(content, "## Release Process")
	if sectionStart == -1 {
		t.Fatalf("CONTRIBUTING.md missing `## Release Process` section (prerequisite for this test)")
	}
	sectionBody := content[sectionStart:]
	remainder := sectionBody[len("## Release Process"):]
	if next := strings.Index(remainder, "\n## "); next != -1 {
		sectionBody = sectionBody[:len("## Release Process")+next]
	}

	if !strings.Contains(sectionBody, "APPLE_DEVELOPER_ID_CERT_P12_BASE64") {
		t.Errorf("CONTRIBUTING.md `## Release Process` missing `APPLE_DEVELOPER_ID_CERT_P12_BASE64` " +
			"secret name (Apple cert rotation paragraph deferred from story 7-2)")
	}
}

// ---------------------------------------------------------------------------
// 7.3-STATIC-006 (P1): Verify-license shell script exists and is wired into CI
// Covers AC #9 (Task 5)
// ---------------------------------------------------------------------------

// TestVerifyLicenseScriptExistsAndExecutable asserts scripts/verify-license.sh
// exists and (on Unix) has the executable bit set. On Windows, the executable
// bit check is skipped because Git for Windows does not preserve chmod.
func TestVerifyLicenseScriptExistsAndExecutable(t *testing.T) {
	root := projectRoot(t)
	scriptPath := filepath.Join(root, "scripts", "verify-license.sh")

	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("scripts/verify-license.sh not found: %v", err)
	}

	if runtime.GOOS != "windows" {
		// Any executable bit (owner, group, or other) is acceptable.
		if info.Mode()&0o111 == 0 {
			t.Errorf("scripts/verify-license.sh is not executable (mode=%v); " +
				"run `chmod +x scripts/verify-license.sh` and `git update-index --chmod=+x scripts/verify-license.sh`",
				info.Mode())
		}
	}

	// Bonus: sanity-check the shebang so a stray text file doesn't pass.
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("failed to read scripts/verify-license.sh: %v", err)
	}
	if !strings.HasPrefix(string(content), "#!/usr/bin/env bash") && !strings.HasPrefix(string(content), "#!/bin/bash") {
		t.Errorf("scripts/verify-license.sh missing bash shebang (first line must be `#!/usr/bin/env bash` or `#!/bin/bash`)")
	}
}

// TestCIWorkflowReferencesVerifyScript asserts .github/workflows/ci.yml
// references the scripts/verify-license.sh invocation per AC #9.
func TestCIWorkflowReferencesVerifyScript(t *testing.T) {
	content := readFileAtRoot(t, ".github/workflows/ci.yml")

	if !strings.Contains(content, "scripts/verify-license.sh") {
		t.Errorf(".github/workflows/ci.yml does not reference `scripts/verify-license.sh` -- " +
			"the `Verify open-source docs` step from AC #9 / Task 5.3 is not wired in")
	}
	// The step should also be labelled per Task 5.3.
	if !strings.Contains(content, "Verify open-source docs") {
		t.Errorf(".github/workflows/ci.yml does not contain the `Verify open-source docs` step label (Task 5.3)")
	}
}

// ---------------------------------------------------------------------------
// Automation expansion (2026-04-17): gap-filling static tests at the lowest
// viable layer. Pushed to this same Go module because every check is a pure
// filesystem + string assertion -- no runtime, no browser, no workflow dispatch.
// E2E is explicitly 0 for Story 7-3 per Epic 7 test design Story-Level Test
// Summary; these additions keep the pyramid inverted-static and close gaps
// identified in the 7-3 traceability report.
// ---------------------------------------------------------------------------

// sectionBody returns the substring of doc starting at the given H2 header and
// ending at the next H2 header (or EOF). Returns empty string if header absent.
func sectionBody(doc, h2 string) string {
	start := strings.Index(doc, h2)
	if start == -1 {
		return ""
	}
	body := doc[start:]
	remainder := body[len(h2):]
	if next := strings.Index(remainder, "\n## "); next != -1 {
		return body[:len(h2)+next]
	}
	return body
}

// TestReadmeHasH1AndTagline asserts README.md opens with the H1 title
// `# UniDoc PDF Debugger` and a one-line blockquote tagline (AC #4 top-of-file
// contract). Regression guard against accidental demotion of the H1 or loss of
// the tagline.
func TestReadmeHasH1AndTagline(t *testing.T) {
	content := readFileAtRoot(t, "README.md")

	if !strings.HasPrefix(content, "# UniDoc PDF Debugger\n") {
		first := content
		if n := strings.IndexByte(content, '\n'); n != -1 {
			first = content[:n]
		}
		t.Errorf("README.md must start with H1 `# UniDoc PDF Debugger` (AC #4); first line = %q", first)
	}
	// Tagline is a GitHub blockquote (`> ...`) appearing before the first H2.
	firstH2 := strings.Index(content, "\n## ")
	if firstH2 == -1 {
		t.Fatalf("README.md has no H2 sections (prerequisite for tagline check)")
	}
	head := content[:firstH2]
	if !strings.Contains(head, "\n> ") {
		t.Errorf("README.md missing blockquote tagline (`> ...`) between H1 and first H2 (AC #4)")
	}
}

// TestReadmeHasBadgeRow asserts the README badge row (between H1 and H2
// sections) contains CI, License, and Platforms badges per AC #4.
func TestReadmeHasBadgeRow(t *testing.T) {
	content := readFileAtRoot(t, "README.md")

	firstH2 := strings.Index(content, "\n## ")
	if firstH2 == -1 {
		t.Fatalf("README.md has no H2 sections (prerequisite for badge check)")
	}
	head := content[:firstH2]

	required := []string{
		"actions/workflows/ci.yml/badge.svg", // CI badge (shields/GH actions form)
		"img.shields.io/badge/license",       // License badge (shields.io)
		"img.shields.io/badge/platforms",     // Platforms badge (shields.io)
	}
	for _, s := range required {
		if !strings.Contains(head, s) {
			t.Errorf("README.md badge row (between H1 and first H2) missing badge URL substring: %q (AC #4)", s)
		}
	}
}

// TestReadmeLicenseSectionHasRelativeLinksAndBrandBlurb covers AC #10: the
// `## License` section must link to `./LICENSE` and `./NOTICE` via relative
// markdown, explicitly state "Apache License 2.0", carry the `(c) 2026 UniDoc
// ehf.` attribution, and include the one-line brand-halo blurb to unidoc.io.
// This closes the PARTIAL coverage flagged in the 7-3 traceability report.
func TestReadmeLicenseSectionHasRelativeLinksAndBrandBlurb(t *testing.T) {
	body := sectionBody(readFileAtRoot(t, "README.md"), "## License")
	if body == "" {
		t.Fatalf("README.md missing `## License` section (prerequisite)")
	}

	required := []string{
		"(./LICENSE)",     // relative markdown link to LICENSE
		"(./NOTICE)",      // relative markdown link to NOTICE
		"Apache License 2.0",
		"(c) 2026 UniDoc ehf.",
		"unidoc.io",              // brand-halo blurb destination
		"commercial PDF toolkit", // brand-halo blurb phrasing (AC #10 quote)
	}
	for _, s := range required {
		if !strings.Contains(body, s) {
			t.Errorf("README.md `## License` section missing required substring: %q (AC #10)", s)
		}
	}
}

// TestReadmeInstallationHasPerPlatformArtifacts asserts the ## Installation
// section documents the three per-platform release artifact naming patterns
// (macOS `.dmg`, Windows `.zip` containing `.exe`, Linux `.tar.gz`) plus the
// `sudo xattr -cr` Gatekeeper bypass command for unsigned macOS builds.
func TestReadmeInstallationHasPerPlatformArtifacts(t *testing.T) {
	body := sectionBody(readFileAtRoot(t, "README.md"), "## Installation")
	if body == "" {
		t.Fatalf("README.md missing `## Installation` section (prerequisite)")
	}

	required := []string{
		"darwin-",         // darwin architecture suffix
		".dmg",            // macOS artifact extension
		"windows-amd64",   // Windows arch token
		".zip",            // Windows artifact extension (zip contains the .exe + LICENSE + NOTICE)
		".exe",            // Windows binary inside the zip
		"linux-amd64",     // Linux arch token
		".tar.gz",         // Linux extension
		"sudo xattr -cr",  // Gatekeeper bypass command for unsigned macOS builds
		"libwebkit2gtk",   // Linux dep hint
	}
	for _, s := range required {
		if !strings.Contains(body, s) {
			t.Errorf("README.md `## Installation` section missing required substring: %q (AC #6)", s)
		}
	}
}

// TestReadmeHasNoBmadOutputLinks guards against the specific anti-pattern
// called out in story 7-3 Task 3.7: `_bmad-output` is a symlink that exits the
// code repo, so any GitHub-rendered link into it 404s. No markdown link in
// README may reference that path.
func TestReadmeHasNoBmadOutputLinks(t *testing.T) {
	content := readFileAtRoot(t, "README.md")

	// Match markdown links / images with _bmad-output in the target.
	pattern := regexp.MustCompile(`\[[^\]]*\]\([^)]*_bmad-output[^)]*\)`)
	if m := pattern.FindString(content); m != "" {
		t.Errorf("README.md contains link into `_bmad-output/` which is a symlink exiting the code repo "+
			"and will 404 on GitHub: %q (story 7-3 Task 3.7)", m)
	}
}

// TestNoticeEntriesDeclareCompatibleLicense asserts each of the mandated
// third-party attributions explicitly states a license compatible with
// Apache 2.0 (i.e. MIT or Apache 2.0). This is the NOTICE-side mirror of
// Task 2.3's license-compatibility audit and a regression guard against a
// future maintainer dropping a license tag or adding a GPL/LGPL/AGPL dep.
func TestNoticeEntriesDeclareCompatibleLicense(t *testing.T) {
	content := readFileAtRoot(t, "NOTICE")

	// Both license tokens must appear somewhere in NOTICE (pdfcpu is Apache,
	// everything else is MIT).
	if !strings.Contains(content, "Apache License 2.0") {
		t.Errorf("NOTICE missing `Apache License 2.0` license tag (pdfcpu attribution should carry it)")
	}
	if !strings.Contains(content, "MIT License") {
		t.Errorf("NOTICE missing `MIT License` license tag (frontend deps should carry it)")
	}

	// Regression guard against non-Apache-compatible licenses slipping in.
	incompatible := []string{"GPL", "AGPL", "LGPL", "SSPL", "CC-BY-NC", "Commons Clause"}
	for _, bad := range incompatible {
		if strings.Contains(content, bad) {
			t.Errorf("NOTICE references potentially Apache-incompatible license token %q -- "+
				"project license policy (PRD line 56) forbids this", bad)
		}
	}
}

// TestContributingSubmittingPRsHasBranchProtection asserts the
// `## Submitting Pull Requests` section names the three required CI matrix
// status checks and documents the `dev` branch target per AC #8.
func TestContributingSubmittingPRsHasBranchProtection(t *testing.T) {
	body := sectionBody(readFileAtRoot(t, "CONTRIBUTING.md"), "## Submitting Pull Requests")
	if body == "" {
		t.Fatalf("CONTRIBUTING.md missing `## Submitting Pull Requests` section (prerequisite)")
	}

	required := []string{
		"dev",                        // branch target
		"build-and-test (macos-latest)",
		"build-and-test (windows-latest)",
		"build-and-test (ubuntu-latest)",
	}
	for _, s := range required {
		if !strings.Contains(body, s) {
			t.Errorf("CONTRIBUTING.md `## Submitting Pull Requests` missing required substring: %q (AC #8)", s)
		}
	}
}

// TestContributingReportingIssuesHasSecurityContact asserts the
// `## Reporting Issues` section carries the security triage email address
// specified in AC #8 (`security@unidoc.io`) so security reports do not land
// on a public issue tracker.
func TestContributingReportingIssuesHasSecurityContact(t *testing.T) {
	body := sectionBody(readFileAtRoot(t, "CONTRIBUTING.md"), "## Reporting Issues")
	if body == "" {
		t.Fatalf("CONTRIBUTING.md missing `## Reporting Issues` section (prerequisite)")
	}

	if !strings.Contains(body, "security@unidoc.io") {
		t.Errorf("CONTRIBUTING.md `## Reporting Issues` missing `security@unidoc.io` security contact (AC #8)")
	}
}

// TestVerifyLicenseScriptUsesStrictMode asserts scripts/verify-license.sh
// declares `set -euo pipefail` (Task 5.1 contract: "any failure aborts"). A
// missing strict-mode pragma would let a silent failure mid-script pass CI
// even though a downstream grep failed.
func TestVerifyLicenseScriptUsesStrictMode(t *testing.T) {
	root := projectRoot(t)
	content, err := os.ReadFile(filepath.Join(root, "scripts", "verify-license.sh"))
	if err != nil {
		t.Fatalf("scripts/verify-license.sh not readable: %v", err)
	}
	if !strings.Contains(string(content), "set -euo pipefail") {
		t.Errorf("scripts/verify-license.sh missing `set -euo pipefail` strict-mode pragma (Task 5.1)")
	}
}
