// Package story_14_2_wails_alpha2_117_upgrade_test provides RED-PHASE acceptance
// tests for Story 14.2: the governed Wails v3 version bump from the current pin
// v3.0.0-alpha2.103 to the committed target v3.0.0-alpha2.117 (Go module +
// wails3 CLI in lockstep), holding the @wailsio/runtime JS runtime at whatever
// upstream actually publishes (still alpha.79; no alpha2 runtime exists on npm).
//
// TDD RED PHASE: the version-pin tests MUST fail on the pre-bump tree (current
// pin alpha2.103) and pass after the Dev step lands the bump to the target per
// the story. The presence + wire-shape guards pass today and stand as the
// standing regression net across the bump.
//
// Test pyramid for this story (per the directive to favour API/integration over
// E2E, unit only where there is business logic):
//
//   - The story introduces ZERO new business logic. The whole change is a
//     platform-version pin move + binding regen + policy doc. There is no new
//     function/hook/component to unit-test.
//   - The decisive coverage (the live bindings round-trip inside the platform
//     WebView, multi-WebView desktop smoke) is the native runtime layer. It
//     needs real macOS + Windows hardware and a human observer and is, by the
//     story's own design (AC4, a DEFERRED HUMAN/HARDWARE gate), MANUAL cross-OS
//     smoke recorded in the Dev Agent Record. Playwright cannot drive it; no red
//     E2E is authored (see deferred_gates_test.go for the documented skips).
//   - So every automated test here is a Go structural / CLI-integration
//     acceptance test in this independent module (mirrors
//     tests/12-3-wails-alpha2-103-upgrade/).
//
// This module is an independent go.mod (the project convention: no `replace`
// link into the main module, which would drag the whole Wails tree into a test
// module). It reads the repo's static files (go.mod, the two CI workflows,
// frontend/package.json, the regenerated binding) and exercises the CLI, which
// is the executable expression of the SAME internal/pdfcore model.go structs.
//
// Run: cd tests/14-2-wails-alpha2-117-upgrade && go test -v -count=1 ./...
package story_14_2_wails_alpha2_117_upgrade_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// alpha2Base offsets the alpha2.* scheme onto one monotonic ordinal line above
// every plain alpha.* number Wails has shipped, so alpha2.N always sorts after
// alpha.N (matching upstream ordering: alpha.102 < alpha2.103 < alpha2.117).
const alpha2Base = 1000

// currentBaselineOrdinal is the ordinal of the pre-bump pin v3.0.0-alpha2.103.
// Every Go-side touch-point (go.mod, ci.yml, release.yml) must move STRICTLY
// past this on a bump; the tree sat exactly at it when these tests were authored
// (RED phase), which is why the version-pin tests were RED before the bump landed.
const currentBaselineOrdinal = alpha2Base + 103

// targetOrdinal is the committed AC1/AC2 target v3.0.0-alpha2.117. The go.mod
// pin must reach this floor (>=), which encodes "the bump landed the committed
// target" while staying non-brittle to a defensibly-newer most-baked pick.
// Fallback rungs (AC7): rung 1 = alpha2.103 (current known-good), rung 2 =
// alpha.102 (deeper) -- both sit BELOW this floor, so a rollback correctly
// leaves this gate red (the story does not reach done on a rolled-back tree).
const targetOrdinal = alpha2Base + 117

// jsRuntimeCurrentAlpha is the current @wailsio/runtime pin alpha number. npm
// tops out at alpha.79 (AC5): the runtime is held to what upstream publishes, so
// the pin must stay a well-formed 3.0.0-alpha.N tag with N >= this and must NOT
// be rewritten to a phantom alpha2.* tag (none is published on npm).
const jsRuntimeCurrentAlpha = 79

// goWailsRe matches a Wails Go pin in either scheme and captures the variant
// digit (empty for plain `alpha`, "2" for `alpha2`) and the trailing number:
//
//	v3.0.0-alpha.102   -> groups ("",  "102")
//	v3.0.0-alpha2.117  -> groups ("2", "117")
var goWailsRe = regexp.MustCompile(`v3\.0\.0-alpha(2)?\.(\d+)`)

// jsRuntimeRe matches an @wailsio/runtime pin in either scheme (same shape as
// goWailsRe, sans the leading `v`).
var jsRuntimeRe = regexp.MustCompile(`3\.0\.0-alpha(2)?\.(\d+)`)

// alphaOrdinal collapses the two version schemes into ONE monotonic ordinal so
// "strictly newer" comparisons work across the alpha2.103 -> alpha2.117 move.
// Returns -1 on no match.
func alphaOrdinal(re *regexp.Regexp, s string) int {
	m := re.FindStringSubmatch(s)
	if len(m) < 3 {
		return -1
	}
	n, err := strconv.Atoi(m[2])
	if err != nil {
		return -1
	}
	if m[1] == "2" {
		return alpha2Base + n
	}
	return n
}

// allAlphaOrdinals returns the ordinal of every Wails/runtime pin matched in s.
// Used to catch a workflow file that still carries the old pin alongside a new
// one (a half-done bump).
func allAlphaOrdinals(re *regexp.Regexp, s string) []int {
	all := re.FindAllStringSubmatch(s, -1)
	out := make([]int, 0, len(all))
	for _, m := range all {
		if len(m) < 3 {
			continue
		}
		n, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		if m[1] == "2" {
			out = append(out, alpha2Base+n)
		} else {
			out = append(out, n)
		}
	}
	return out
}

// projectRoot walks up from the working directory until it finds the project
// go.mod (module unidoc-pdf-debugger), and returns its absolute path.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			if strings.Contains(string(data), "module unidoc-pdf-debugger") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root (no go.mod with module unidoc-pdf-debugger)")
		}
		dir = parent
	}
}

// readSource reads a file relative to the project root.
func readSource(t *testing.T, relPath string) string {
	t.Helper()
	root := projectRoot(t)
	content, err := os.ReadFile(filepath.Join(root, relPath))
	if err != nil {
		t.Fatalf("cannot read %s: %v", relPath, err)
	}
	return string(content)
}

// fileExists returns true when relPath exists under projectRoot.
func fileExists(t *testing.T, relPath string) bool {
	t.Helper()
	root := projectRoot(t)
	_, err := os.Stat(filepath.Join(root, relPath))
	return err == nil
}

// goWailsLine returns the `github.com/wailsapp/wails/v3 ` require line from
// go.mod, or "" if absent.
func goWailsLine(t *testing.T) string {
	t.Helper()
	src := readSource(t, "go.mod")
	for _, l := range strings.Split(src, "\n") {
		if strings.Contains(l, "github.com/wailsapp/wails/v3 ") {
			return l
		}
	}
	return ""
}

// fmtOrdinal renders an ordinal back to a human-readable Wails tag for messages.
func fmtOrdinal(ord int) string {
	if ord >= alpha2Base {
		return fmt.Sprintf("alpha2.%d", ord-alpha2Base)
	}
	return fmt.Sprintf("alpha.%d", ord)
}

// tagFromOrdinal renders an ordinal back to the bare tag form used in go.sum
// (`alpha.N` / `alpha2.N`), without the leading `v3.0.0-`.
func tagFromOrdinal(ord int) string {
	if ord >= alpha2Base {
		return "alpha2." + strconv.Itoa(ord-alpha2Base)
	}
	return "alpha." + strconv.Itoa(ord)
}

// buildCLI compiles the pdfdebug CLI into a temp dir and returns its path. The
// CLI is the executable expression of the internal/pdfcore model.go structs, so
// its JSON output is the real-Marshal wire shape the wire-shape guard inspects
// without this module having to import (and thus replace-link) the main module.
func buildCLI(t *testing.T) string {
	t.Helper()
	root := projectRoot(t)
	tmpDir := t.TempDir()
	binName := "pdfdebug"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(tmpDir, binName)
	cmd := exec.Command("go", "build", "-o", binPath, "./cmd/cli/")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build CLI binary: %s\n%s", err, output)
	}
	return binPath
}

// runCLI executes the CLI binary with args and returns stdout, stderr, exit code.
func runCLI(t *testing.T, binPath string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run CLI: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// mustParseJSONObject parses s as a single JSON object, failing on error.
func mustParseJSONObject(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(s)), &m); err != nil {
		t.Fatalf("failed to parse JSON object: %v\nraw: %s", err, s)
	}
	return m
}
