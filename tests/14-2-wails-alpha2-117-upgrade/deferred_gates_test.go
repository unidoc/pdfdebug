// Deferred / non-unit-automatable gate scenarios, encoded as documented
// skipped-with-reason placeholders rather than fabricated automation. These
// exist so the story's full gate is TRACEABLE in the suite (every 14.2 scenario
// has a home) without pretending a headless Go test can close a check that needs
// the wails3 toolchain, a git working tree, or real desktop hardware.
//
// Per the ATDD directive: follow the test pyramid, do NOT create E2E/browser
// tests for criteria that need full desktop/WebView interaction, and encode the
// bindings zero-diff check as a documented integration check (not a grep).
package story_14_2_wails_alpha2_117_upgrade_test

import "testing"

// Test_14_2_GATE_001_ZeroDiffBindings [P0] AC3.2 (risk R-14-08), THE acceptance
// criterion. DEFERRED: this is a process/git assertion, not a pure unit test.
// The check is: after `wails3 generate bindings -clean=true`, `git diff
// --exit-code frontend/bindings/` is EMPTY (or a non-empty diff has been fully
// reconciled and re-passed against 14.2-INTG-002/003). It requires the wails3
// CLI installed at the target pin and a clean git working tree -- neither
// available to a headless red-phase Go test. The Dev step runs it in CI/locally
// as a blocking gate and records the result in the Dev Agent Record; the
// presence contract (14.2-INTG-002) and wire-shape guard (14.2-INTG-003) are the
// automated nets that catch the specific drift a non-empty diff would carry.
func Test_14_2_GATE_001_ZeroDiffBindings(t *testing.T) {
	t.Skip("[P0] 14.2-GATE-001: DEFERRED to the Dev/CI gate -- `wails3 generate bindings -clean=true` must yield an EMPTY `git diff frontend/bindings/` (or a reconciled diff re-passing 14.2-INTG-002/003). Needs the wails3 CLI at the target pin + a clean git tree; not automatable in a headless red-phase Go test. See 14.2-INTG-002 / 14.2-INTG-003 for the automated drift nets.")
}

// Test_14_2_GATE_002_ThreeRunnerBuildSmoke [P1] AC3.4 (risk R-14-07). DEFERRED:
// the 3-runner (ubuntu/macos/windows) `wails3 build` smoke proves compile+link,
// NOT runtime. It runs in the CI matrix with the wails3 CLI installed at the
// target pin; it is not a Go unit test and is not reproducible here.
func Test_14_2_GATE_002_ThreeRunnerBuildSmoke(t *testing.T) {
	t.Skip("[P1] 14.2-GATE-002: DEFERRED to the CI 3-runner matrix -- host `wails3 build` on ubuntu/macos/windows (compile+link only, not runtime). Needs the wails3 CLI at the target pin; not automatable in this Go module.")
}

// Test_14_2_MANUAL_001_DesktopWebViewSmoke [P0] AC4 (risk R-14-07). DEFERRED
// HUMAN/HARDWARE GATE: the 5-step desktop smoke (launch/no-white-screen, load a
// real PDF round trip, exercise one method per binding category, one error path
// surfaces in the UI, clean quit) runs OUT-OF-BAND on real macOS + Windows
// hardware with a human observer. Headless CI cannot see the WebView. NO
// E2E/browser test is authored (Playwright cannot drive the native WebView).
// Evidence is captured as dated screenshots/recordings in the Dev Agent Record;
// the story does not reach `done` until that evidence is attached.
func Test_14_2_MANUAL_001_DesktopWebViewSmoke(t *testing.T) {
	t.Skip("[P0] 14.2-MANUAL-001: DEFERRED human/hardware gate -- 5-step WebView smoke on real macOS + Windows, run out-of-band by a human, evidence in the Dev Agent Record (pending-human). Not automatable; no E2E authored per the test-pyramid directive.")
}

// Test_14_2_MANUAL_002_GuiRegressionSweep [P1] AC9/AC10 (risk R-14-10). DEFERRED
// HUMAN/HARDWARE GATE folded into the same manual pass as 14.2-MANUAL-001:
// window placement per OS (Windows first-launch centering), splash init, About
// dialog populates, WebView renders without artifacts; plus the AC10 record-only
// yes/no on the two Epic-15 framework-gated GUI items. Human observation on real
// hardware; not automatable, nothing implemented against either GUI primitive.
func Test_14_2_MANUAL_002_GuiRegressionSweep(t *testing.T) {
	t.Skip("[P1] 14.2-MANUAL-002: DEFERRED human/hardware gate -- GUI regression sweep + AC10 record-only unblock check, folded into the manual pass on real macOS + Windows. Not automatable; evidence/record in the Dev Agent Record.")
}
