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

// TestZeroDiffBindings records a deferred process gate: after `wails3 generate
// bindings -clean=true`, `git diff --exit-code frontend/bindings/` must be empty,
// or a non-empty diff must be reconciled and re-checked against the binding
// presence and wire-shape tests. It needs the wails3 CLI at the target pin and a
// clean working tree, so it runs in CI or locally rather than as a headless Go
// test.
func TestZeroDiffBindings(t *testing.T) {
	t.Skip("DEFERRED to the Dev/CI gate -- `wails3 generate bindings -clean=true` must yield an EMPTY `git diff frontend/bindings/`, or a reconciled diff that still passes the binding presence and wire-shape tests. Needs the wails3 CLI at the target pin and a clean git tree; not automatable in a headless Go test. TestBindingsExportConsumerMethods and TestObjectDetailWireShape are the automated drift nets.")
}

// TestThreeRunnerBuildSmoke records a deferred gate: the 3-runner
// (ubuntu/macos/windows) `wails3 build` smoke proves compile and link, not
// runtime. It runs in the CI matrix with the wails3 CLI installed at the target
// pin and is not reproducible as a Go unit test.
func TestThreeRunnerBuildSmoke(t *testing.T) {
	t.Skip("DEFERRED to the CI 3-runner matrix -- host `wails3 build` on ubuntu/macos/windows (compile+link only, not runtime). Needs the wails3 CLI at the target pin; not automatable in this Go module.")
}

// TestDesktopWebViewSmoke records a deferred human/hardware gate: the 5-step
// desktop smoke (launch with no white screen, load a real PDF round trip,
// exercise one method per binding category, surface one error path in the UI,
// clean quit) runs out-of-band on real macOS and Windows hardware with a human
// observer. Headless CI cannot see the WebView and Playwright cannot drive it, so
// the evidence is dated screenshots and recordings.
func TestDesktopWebViewSmoke(t *testing.T) {
	t.Skip("DEFERRED human/hardware gate -- 5-step WebView smoke on real macOS + Windows, run out-of-band by a human, evidence in the Dev Agent Record (pending-human). Not automatable; no E2E authored per the test-pyramid directive.")
}

// TestGuiRegressionSweep records a deferred human/hardware gate folded into the
// same manual pass as the desktop WebView smoke: window placement per OS (Windows
// first-launch centering), splash init, About dialog populated, WebView rendering
// without artifacts, plus a record-only yes/no on the two framework-gated GUI
// items. Human observation on real hardware; nothing is implemented against
// either GUI primitive.
func TestGuiRegressionSweep(t *testing.T) {
	t.Skip("DEFERRED human/hardware gate -- GUI regression sweep plus the record-only unblock check on the two framework-gated GUI items, folded into the manual pass on real macOS + Windows. Not automatable; evidence recorded in the Dev Agent Record.")
}
