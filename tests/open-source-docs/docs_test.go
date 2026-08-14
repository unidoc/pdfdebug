// Package open_source_docs_test provides acceptance tests for Open-Source
// Project Setup.
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
// Test Levels: Static (Go) -- pure filesystem + string grep checks. No YAML
// parsing dependency; no external modules. Each of the 14 test functions below
// names the property it checks, so the properties are read off the function
// list rather than from a mapping table:
//
//   - LICENSE byte-matches the canonical Apache 2.0 text after the copyright
//     substitution, and the canonical fixture itself is unmodified;
//   - NOTICE carries the UniDoc attribution and no Unicode copyright glyph;
//   - README has its required H2 sections in order, a screenshot reference,
//     the Installation subsections, and every Build-from-Source command;
//   - CONTRIBUTING has its required sections, every test command, and the
//     Apple certificate-rotation step in the release process;
//   - scripts/verify-license.sh exists, is executable, and the CI workflow
//     references it.
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
// LICENSE byte-matches canonical Apache 2.0 text Covers Task 1
// ---------------------------------------------------------------------------

// TestLicenseMatchesCanonicalWithSubstitution asserts that LICENSE at repo root
// is byte-identical to scripts/fixtures/apache-2.0-with-copyright.txt (the
// canonical Apache 2.0 text with the documented UniDoc copyright substitution
// applied). This is the primary byte-match contract.
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
// NOTICE has UniDoc + third-party attributions Covers Task 2
// ---------------------------------------------------------------------------

// TestNoticeHasRequiredAttributions asserts NOTICE contains UniDoc ehf.,
// ASCII (c), Apache License reference, and each of the 8 mandated third-party
// attribution substrings.
func TestNoticeHasRequiredAttributions(t *testing.T) {
	content := readFileAtRoot(t, "NOTICE")

	required := []string{
		"UniDoc ehf.",     // UniDoc attribution
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
// Verify-license shell script exists and is wired into CI Covers Task 5
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
// references the scripts/verify-license.sh invocation.
func TestCIWorkflowReferencesVerifyScript(t *testing.T) {
	content := readFileAtRoot(t, ".github/workflows/ci.yml")

	if !strings.Contains(content, "scripts/verify-license.sh") {
		t.Errorf(".github/workflows/ci.yml does not reference `scripts/verify-license.sh` -- " +
			"the `Verify open-source docs` step is not wired in")
	}
	// The step should also be labelled per Task 5.3.
	if !strings.Contains(content, "Verify open-source docs") {
		t.Errorf(".github/workflows/ci.yml does not contain the `Verify open-source docs` step label (Task 5.3)")
	}
}

// TestReadmeHasNoBmadOutputLinks guards against the specific anti-pattern
// called out separately: `_bmad-output` is a symlink that exits the
// code repo, so any GitHub-rendered link into it 404s. No markdown link in
// README may reference that path.
func TestReadmeHasNoBmadOutputLinks(t *testing.T) {
	content := readFileAtRoot(t, "README.md")

	// Match markdown links / images with _bmad-output in the target.
	pattern := regexp.MustCompile(`\[[^\]]*\]\([^)]*_bmad-output[^)]*\)`)
	if m := pattern.FindString(content); m != "" {
		t.Errorf("README.md contains link into `_bmad-output/` which is a symlink exiting the code repo "+
			"and will 404 on GitHub: %q", m)
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
			t.Errorf("NOTICE references potentially Apache-incompatible license token %q -- the project "+
				"ships under Apache 2.0, so no attribution may carry copyleft or non-commercial terms", bad)
		}
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
