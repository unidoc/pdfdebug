// Package release_pipeline_test: acceptance tests for "Bundle the
// pdfdebug CLI with the desktop app archives (all platforms)".
//
// These tests verify the packaging-only change that ships the pdfdebug CLI
// INSIDE each platform's GUI archive while retaining the standalone CLI archive
// (net artifact count stays 6). The change touches only build/CI files:
//   - .github/workflows/release.yml ("Build CLI" reorder + per-platform GUI
//     staging now embeds the CLI)
//   - build/darwin/Taskfile.yml (create:app:bundle builds + copies the CLI into
//     Contents/Resources before the codesign:adhoc dispatch)
//
// Test Levels: Integration (Go) -- YAML/Taskfile parsing + filesystem checks.
// No browser or HTTP surface; this is a build/CI packaging story with no
// frontend/runtime code in scope, so no E2E applies.
//
// Each test function below names the packaging property it checks, so there is
// no mapping table to keep in sync.
//
// Run: cd tests/release-pipeline && go test -v -count=1 ./...
package release_pipeline_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// readDarwinTaskfile returns the raw build/darwin/Taskfile.yml text, failing if
// absent.
func readDarwinTaskfile(t *testing.T) string {
	t.Helper()
	root := projectRoot(t)
	p := filepath.Join(root, "build", "darwin", "Taskfile.yml")
	content, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("build/darwin/Taskfile.yml not found: %v", err)
	}
	return string(content)
}

// stepIndexMatching returns the index of the first step in the named job whose
// name OR run body matches the predicate, or -1.
func stepIndexMatching(t *testing.T, jobName string, predicate func(name, run string) bool) int {
	t.Helper()
	for i, s := range jobSteps(t, jobName) {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := m["name"].(string)
		run, _ := m["run"].(string)
		if predicate(name, run) {
			return i
		}
	}
	return -1
}

// darwinStageBlock returns the `darwin-arm64|darwin-amd64` case body of the
// Stage artifacts step. windowsStageBlock / linuxStageBlock return the
// respective case bodies. Returns "" if the Stage artifacts step or the case
// cannot be located.
func stageCaseBlock(t *testing.T, caseLabel string) string {
	t.Helper()
	step := findStepByPredicate(t, "build", func(m map[string]interface{}) bool {
		name, _ := m["name"].(string)
		return strings.Contains(strings.ToLower(name), "stage artifacts")
	})
	if step == nil {
		t.Fatalf("release.yml build job: `Stage artifacts` step not found")
	}
	run, _ := step["run"].(string)
	// Each platform case is delimited by `LABEL)` ... `;;`. Capture the body
	// between the label line and the terminating `;;`.
	idx := strings.Index(run, caseLabel+")")
	if idx == -1 {
		t.Fatalf("release.yml Stage artifacts: case label %q) not found", caseLabel)
	}
	rest := run[idx:]
	end := strings.Index(rest, ";;")
	if end == -1 {
		return rest
	}
	return rest[:end]
}

// ---------------------------------------------------------------------------
// "Build CLI" step is reordered to run BEFORE the macOS GUI package step so the
// CLI binary exists at package time and can be embedded in the .app. Covers
// Task 0.1.
// ---------------------------------------------------------------------------

func TestBuildCLIStepRunsBeforeMacGUIBuild(t *testing.T) {
	cliIdx := stepIndexMatching(t, "build", func(name, run string) bool {
		return strings.EqualFold(strings.TrimSpace(name), "Build CLI")
	})
	guiIdx := stepIndexMatching(t, "build", func(name, run string) bool {
		return strings.Contains(name, "Build macOS GUI")
	})
	if cliIdx == -1 {
		t.Fatalf("release.yml build job: `Build CLI` step not found")
	}
	if guiIdx == -1 {
		t.Fatalf("release.yml build job: `Build macOS GUI` step not found")
	}
	if cliIdx >= guiIdx {
		t.Errorf("release.yml build job: `Build CLI` step (idx %d) must run BEFORE `Build macOS GUI` step (idx %d) so bin/pdfdebug exists when darwin:package assembles the bundle", cliIdx, guiIdx)
	}
}

// ---------------------------------------------------------------------------
// "Build CLI" step runs BEFORE the conditional "Sign macOS app bundle" step,
// so a Mach-O added to the bundle is covered by the signature whenever
// Developer ID signing is later enabled. Covers Task 0.1.
// ---------------------------------------------------------------------------

func TestBuildCLIStepRunsBeforeMacSignStep(t *testing.T) {
	cliIdx := stepIndexMatching(t, "build", func(name, run string) bool {
		return strings.EqualFold(strings.TrimSpace(name), "Build CLI")
	})
	signIdx := stepIndexMatching(t, "build", func(name, run string) bool {
		return strings.Contains(name, "Sign macOS app bundle")
	})
	if cliIdx == -1 {
		t.Fatalf("release.yml build job: `Build CLI` step not found")
	}
	if signIdx == -1 {
		t.Fatalf("release.yml build job: `Sign macOS app bundle` step not found")
	}
	if cliIdx >= signIdx {
		t.Errorf("release.yml build job: `Build CLI` step (idx %d) must run BEFORE `Sign macOS app bundle` step (idx %d) so a CLI added to Contents/Resources is covered by the signature when signing is enabled", cliIdx, signIdx)
	}
}

// ---------------------------------------------------------------------------
// The "CLI smoke test" step still runs AFTER the (now-earlier) "Build CLI"
// step. Task 0.1 reorders Build CLI before the macOS GUI/sign steps but
// explicitly requires the smoke test to stay downstream of Build CLI -- the
// smoke step invokes the produced bin/pdfdebug, so if a future edit moved it
// ahead of Build CLI it would run against a missing binary.
// Covers Task 0.1 ("Keep the CLI smoke test step after the Build CLI step").
// ---------------------------------------------------------------------------

func TestCLISmokeTestStepRunsAfterBuildCLI(t *testing.T) {
	cliIdx := stepIndexMatching(t, "build", func(name, run string) bool {
		return strings.EqualFold(strings.TrimSpace(name), "Build CLI")
	})
	smokeIdx := stepIndexMatching(t, "build", func(name, run string) bool {
		return strings.Contains(strings.ToLower(name), "smoke")
	})
	if cliIdx == -1 {
		t.Fatalf("release.yml build job: `Build CLI` step not found")
	}
	if smokeIdx == -1 {
		t.Fatalf("release.yml build job: `CLI smoke test` step not found")
	}
	if smokeIdx <= cliIdx {
		t.Errorf("release.yml build job: `CLI smoke test` step (idx %d) must run AFTER `Build CLI` step (idx %d) -- the smoke step invokes the produced bin/pdfdebug", smokeIdx, cliIdx)
	}
}

// ---------------------------------------------------------------------------
// build/darwin/Taskfile.yml create:app:bundle is self-sufficient -- it builds
// the CLI itself (CGO_ENABLED=0, ./cmd/cli, with VERSION resolution) rather
// than assuming a pre-built bin/pdfdebug. Covers Task 1.1.
// ---------------------------------------------------------------------------

func TestDarwinBundleBuildsCLI(t *testing.T) {
	raw := readDarwinTaskfile(t)

	// The CLI must be built within the package flow: either an explicit
	// `go build ./cmd/cli` cmd, or a deps:-wired task that does so. Require the
	// CGO-free CLI target to be referenced somewhere in the Taskfile.
	if !strings.Contains(raw, "./cmd/cli") {
		t.Errorf("build/darwin/Taskfile.yml: must build the CLI via `./cmd/cli` (create:app:bundle/package must produce pdfdebug, not assume a pre-built bin/pdfdebug)")
	}
	// CGO_ENABLED=0 for the CLI (it has zero Wails/CGo dependency).
	if !regexp.MustCompile(`CGO_ENABLED:\s*['"]?0['"]?`).MatchString(raw) {
		t.Errorf("build/darwin/Taskfile.yml: CLI build must set CGO_ENABLED=0 (CLI consumes internal/pdfcore with no Wails dependency)")
	}
	// VERSION resolution must match the existing build tasks' idiom:
	// `env "VERSION" | default "dev"`.
	if !strings.Contains(raw, `env "VERSION"`) {
		t.Errorf("build/darwin/Taskfile.yml: CLI build must resolve VERSION via `env \"VERSION\" | default \"dev\"`")
	}
	// The version ldflag must embed main.version for the CLI.
	if !strings.Contains(raw, "-X main.version=") {
		t.Errorf("build/darwin/Taskfile.yml: CLI build must pass `-X main.version=...` ldflag")
	}
}

// ---------------------------------------------------------------------------
// create:app:bundle copies the CLI to
// Contents/Resources/pdfdebug and marks it executable.
// Covers Task 1.1.
// ---------------------------------------------------------------------------

func TestDarwinBundleCopiesCLIIntoResources(t *testing.T) {
	raw := readDarwinTaskfile(t)

	// A cmd must copy pdfdebug into the bundle's Contents/Resources. The exact
	// destination is Contents/Resources/pdfdebug (Dev Notes "Bundle location
	// rationale"); accept either an explicit `/pdfdebug` filename or a copy
	// targeting the Resources directory with pdfdebug as the source.
	hasResourcesPdfdebug := regexp.MustCompile(`Contents/Resources/pdfdebug`).MatchString(raw)
	hasCopyToResources := regexp.MustCompile(`pdfdebug.*Contents/Resources`).MatchString(raw)
	if !hasResourcesPdfdebug && !hasCopyToResources {
		t.Errorf("build/darwin/Taskfile.yml: create:app:bundle must copy the CLI to {{.BIN_DIR}}/{{.BUNDLE_NAME}}.app/Contents/Resources/pdfdebug")
	}
	// The copied CLI must be executable.
	if !regexp.MustCompile(`chmod\s+\+x.*pdfdebug|chmod\s+\d*7\d*.*pdfdebug`).MatchString(raw) {
		t.Errorf("build/darwin/Taskfile.yml: create:app:bundle must `chmod +x` the bundled CLI so it is executable")
	}
}

// ---------------------------------------------------------------------------
// The CLI copy cmd in create:app:bundle is placed BEFORE the final `task:
// codesign:adhoc`/`codesign:skip` dispatch, so the ad-hoc signature (on a
// darwin host) covers the nested Mach-O. Covers copy-before-sign ordering +
// Task 1.1.
// ---------------------------------------------------------------------------

func TestDarwinBundleCLICopyPrecedesCodesignDispatch(t *testing.T) {
	raw := readDarwinTaskfile(t)

	// Locate the create:app:bundle task body. It runs from the `create:app:bundle:`
	// key to the next top-level task key (`codesign:adhoc:`).
	startRe := regexp.MustCompile(`(?m)^\s{2}create:app:bundle:`)
	startLoc := startRe.FindStringIndex(raw)
	if startLoc == nil {
		t.Fatalf("build/darwin/Taskfile.yml: create:app:bundle task not found (Task 1.1)")
	}
	body := raw[startLoc[1]:]
	// End the body at the next top-level (2-space-indented) task key.
	endRe := regexp.MustCompile(`(?m)^\s{2}\S+:`)
	if endLoc := endRe.FindStringIndex(body); endLoc != nil {
		body = body[:endLoc[0]]
	}

	copyIdx := regexp.MustCompile(`pdfdebug`).FindStringIndex(body)
	dispatchIdx := strings.Index(body, "task: '{{if eq OS \"darwin\"}}codesign:adhoc")
	if dispatchIdx == -1 {
		// Fall back to any codesign:adhoc dispatch reference within the body.
		dispatchIdx = strings.Index(body, "codesign:adhoc")
	}
	if copyIdx == nil {
		t.Errorf("build/darwin/Taskfile.yml: create:app:bundle body has no CLI (pdfdebug) copy cmd")
		return
	}
	if dispatchIdx == -1 {
		t.Fatalf("build/darwin/Taskfile.yml: create:app:bundle body has no codesign dispatch to order against (Task 1.1)")
	}
	if copyIdx[0] >= dispatchIdx {
		t.Errorf("build/darwin/Taskfile.yml: the CLI copy cmd must appear BEFORE the final `task: codesign:adhoc` dispatch in create:app:bundle (a Mach-O added after signing is unsigned and breaks the signature)")
	}
}

// ---------------------------------------------------------------------------
// The darwin GUI (dmg) staging carries NO loose pdfdebug file -- the CLI ships
// inside the .app. The dmg stage copies only the app + LICENSE/NOTICE + the
// /Applications symlink. Covers Task 2.1.
// ---------------------------------------------------------------------------

func TestDarwinGUIStageHasNoLooseCLI(t *testing.T) {
	block := stageCaseBlock(t, "darwin-arm64|darwin-amd64")

	// Split the darwin case into the DMG (GUI) staging section and the CLI
	// (standalone tar.gz) staging section. The CLI archive legitimately copies
	// pdfdebug; the DMG stage must NOT. The DMG stage is the part before the
	// standalone CLI staging begins.
	cliStageIdx := strings.Index(block, "darwin-cli-stage")
	if cliStageIdx == -1 {
		// Standalone CLI staging must still exist; if it is gone this assertion cannot
		// scope correctly -- fail loudly (also caught by retention test).
		t.Fatalf("release.yml darwin stage: standalone CLI staging (darwin-cli-stage) not found; cannot scope the DMG-stage loose-CLI check")
	}
	dmgStage := block[:cliStageIdx]

	// The DMG staging section must not copy pdfdebug into the dmg stage dir.
	if regexp.MustCompile(`cp\b[^\n]*pdfdebug[^\n]*dmg-stage|cp\b[^\n]*pdfdebug[^\n]*"\$STAGE"`).MatchString(dmgStage) {
		t.Errorf("release.yml darwin stage: the DMG staging section must NOT copy a loose `pdfdebug` into the dmg stage -- the CLI ships inside the .app")
	}
}

// ---------------------------------------------------------------------------
// The standalone darwin CLI tar.gz is still produced. Covers Task 2.2.
// ---------------------------------------------------------------------------

func TestDarwinStandaloneCLIArchiveRetained(t *testing.T) {
	block := stageCaseBlock(t, "darwin-arm64|darwin-amd64")

	if !strings.Contains(block, "pdfdebug-cli-${VERSION}-${PLATFORM}.tar.gz") {
		t.Errorf("release.yml darwin stage: standalone `pdfdebug-cli-${VERSION}-${PLATFORM}.tar.gz` must still be produced for headless/CI users")
	}
}

// ---------------------------------------------------------------------------
// The windows GUI zip staging copies pdfdebug.exe into the GUI stage dir
// BEFORE the GUI `7z a -tzip`, so the GUI zip contains both the GUI exe and
// the CLI exe. Covers Task 3.1.
// ---------------------------------------------------------------------------

func TestWindowsGUIStageBundlesCLI(t *testing.T) {
	block := stageCaseBlock(t, "windows-amd64")

	// Scope to the GUI-stage section (before the standalone CLI staging begins).
	cliStageIdx := strings.Index(block, "win-cli-stage")
	guiStage := block
	if cliStageIdx != -1 {
		guiStage = block[:cliStageIdx]
	}

	// A cp of pdfdebug.exe into the GUI stage dir ($GUI_STAGE / win-gui-stage).
	if !regexp.MustCompile(`cp\b[^\n]*pdfdebug\.exe[^\n]*(\$GUI_STAGE|win-gui-stage)`).MatchString(guiStage) {
		t.Errorf("release.yml windows stage: the GUI zip staging must copy `bin/pdfdebug.exe` into the GUI stage dir before `7z a -tzip`")
	}
}

// ---------------------------------------------------------------------------
// The standalone windows CLI zip is still produced. Covers Task 3.2.
// ---------------------------------------------------------------------------

func TestWindowsStandaloneCLIArchiveRetained(t *testing.T) {
	block := stageCaseBlock(t, "windows-amd64")

	if !strings.Contains(block, "pdfdebug-cli-${VERSION}-${PLATFORM}.zip") {
		t.Errorf("release.yml windows stage: standalone `pdfdebug-cli-${VERSION}-${PLATFORM}.zip` must still be produced")
	}
}

// ---------------------------------------------------------------------------
// The linux GUI tar.gz staging copies pdfdebug into the GUI stage dir BEFORE
// the GUI `tar -czf`, so the GUI tar.gz contains both the GUI binary and the
// CLI. Covers Task 4.1.
// ---------------------------------------------------------------------------

func TestLinuxGUIStageBundlesCLI(t *testing.T) {
	block := stageCaseBlock(t, "linux-amd64")

	// Scope to the GUI-stage section (before the standalone CLI staging begins).
	cliStageIdx := strings.Index(block, "linux-cli-stage")
	guiStage := block
	if cliStageIdx != -1 {
		guiStage = block[:cliStageIdx]
	}

	// A cp of bin/pdfdebug into the GUI stage dir ($GUI_STAGE / linux-gui-stage).
	if !regexp.MustCompile(`cp\b[^\n]*\bpdfdebug\b[^\n]*(\$GUI_STAGE|linux-gui-stage)`).MatchString(guiStage) {
		t.Errorf("release.yml linux stage: the GUI tar.gz staging must copy `bin/pdfdebug` into the GUI stage dir before `tar -czf`")
	}
}

// ---------------------------------------------------------------------------
// The standalone linux CLI tar.gz is still produced. Covers Task 4.2.
// ---------------------------------------------------------------------------

func TestLinuxStandaloneCLIArchiveRetained(t *testing.T) {
	block := stageCaseBlock(t, "linux-amd64")

	if !strings.Contains(block, "pdfdebug-cli-${VERSION}-${PLATFORM}.tar.gz") {
		t.Errorf("release.yml linux stage: standalone `pdfdebug-cli-${VERSION}-${PLATFORM}.tar.gz` must still be produced")
	}
}

// ---------------------------------------------------------------------------
// The net artifact count stays 6 -- the bundled-in CLI is not a separate
// counted file. EXPECTED_FILES=6 is unchanged. Covers Task 5.1.
// ---------------------------------------------------------------------------

func TestArtifactCountStaysSix(t *testing.T) {
	run := jobRunBodies(t, "release")
	if !strings.Contains(run, "EXPECTED_FILES=6") {
		t.Errorf("release.yml release job: EXPECTED_FILES must remain 6 (3 GUI archives + 3 standalone CLI archives; the bundled-in CLI is not a separate file)")
	}
}

// ---------------------------------------------------------------------------
// The SHA256SUMS integrity-guard comment no longer references the stale "8
// assets" / "4 matrix cells" rationale; it must state the real 6-asset
// invariant (3 platforms x 2 archives each). Covers Task 5.2.
// ---------------------------------------------------------------------------

func TestSHA256SumsCommentHasNoStaleEightAssets(t *testing.T) {
	raw := readReleaseWorkflow(t)

	// The stale comment said: `... contractual "8 assets" invariant (4 matrix
	// cells x 2 artifacts/cell)`. Both the literal "8 assets" token and the
	// "4 matrix cells" rationale must be gone.
	if strings.Contains(raw, `"8 assets"`) || strings.Contains(raw, "8 assets") {
		t.Errorf("release.yml: stale `8 assets` reference must be corrected to the real 6-asset invariant")
	}
	if strings.Contains(raw, "4 matrix cells") {
		t.Errorf("release.yml: stale `4 matrix cells x 2 artifacts/cell` rationale must be corrected to `3 platforms x 2 archives each`")
	}
}
