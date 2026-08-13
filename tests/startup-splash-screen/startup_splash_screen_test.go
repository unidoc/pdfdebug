// Package startup_splash_screen_test provides acceptance tests for Story 9.13:
// Startup Splash Screen.
//
// Test Pyramid placement per story spec:
//   - Unit (Go, internal/splash): min-display floor, timeout race,
//     version-string passthrough -- delegated via subprocess to
//     internal/splash/splash_test.go to keep pdfcore-style delegation
//     consistent with tests/object-source-and-reverse-refs
//   - Integration (Go, source-content scans):
//     (structural single-instance gating guard)
//   - NO E2E. Wails alpha.85 splash windows are not Playwright-drivable:
//     Playwright drives browsers, not native frameless OS windows. Task 7
//     of the story spec calls out the visual rendering / crossfade /
//     font-fallback as MANUAL verification across three platforms.
//     Asserting them in CI is out of scope.
//
// Run: cd tests/startup-splash-screen && go test -v -count=1 ./...
package startup_splash_screen_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// projectRoot walks upward from the test file to find the project root,
// identified by go.mod with module name "unidoc-pdf-debugger".
// Mirrors the convention used in tests/boot-smoke, tests/app-shell, etc.
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

// readFile reads a file relative to the project root and returns its content.
func readFile(t *testing.T, relPath string) string {
	t.Helper()
	root := projectRoot(t)
	absPath := filepath.Join(root, relPath)
	content, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", relPath, err)
	}
	return string(content)
}

// fileExists checks if a file exists relative to the project root.
func fileExists(t *testing.T, relPath string) bool {
	t.Helper()
	root := projectRoot(t)
	absPath := filepath.Join(root, relPath)
	_, err := os.Stat(absPath)
	return err == nil
}

// splashSource returns the concatenated source of every file that could
// legitimately host the inline splash HTML / window-creation call:
//   - main.go
//   - internal/splash/*.go (if the package was extracted per Dev Notes)
//   - assets/splash/splash.html (if dev keeps a development source mirror
//     of the inlined Go string per Task 2.3)
//
// The dev step picks ONE of these locations; the integration assertions
// search across all of them so the tests survive whichever path dev takes.
func splashSource(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	b.WriteString(readFile(t, "main.go"))
	b.WriteString("\n")
	root := projectRoot(t)
	splashDir := filepath.Join(root, "internal", "splash")
	if entries, err := os.ReadDir(splashDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
				continue
			}
			rel := filepath.Join("internal", "splash", e.Name())
			b.WriteString(readFile(t, rel))
			b.WriteString("\n")
		}
	}
	if fileExists(t, filepath.Join("assets", "splash", "splash.html")) {
		b.WriteString(readFile(t, filepath.Join("assets", "splash", "splash.html")))
		b.WriteString("\n")
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Splash window is created before the main WebviewWindow.
//
// A frameless splash window appears within 500ms of main() entry. The
// actual wall-clock measurement is manual (Task 7.1/7.3); this test only
// verifies the call to create a splash window exists in main.go and is
// positioned before app.Window.NewWithOptions for the main window.
// ---------------------------------------------------------------------------

func TestSplashWindowCreatedBeforeMainWindow(t *testing.T) {
	src := readFile(t, "main.go")

	// Some form of splash window creation must exist. Be permissive about
	// the exact symbol -- dev may extract it into internal/splash or
	// inline it. The structural guarantee is: SOMETHING with the word
	// "splash" creates a window via Wails before the main window does.
	splashIdx := regexp.MustCompile(`(?i)splash\s*[:=].*Window`).FindStringIndex(src)
	if splashIdx == nil {
		splashIdx = regexp.MustCompile(`(?i)(createSplash|newSplash|showSplash|splashWindow|splash\s*:=)`).FindStringIndex(src)
	}
	if splashIdx == nil {
		t.Fatalf("no splash window creation found in main.go. " +
			"Expected something like `splash := app.Window.NewWithOptions(...)` or " +
			"`splash := createSplash(...)`.")
	}

	mainWinIdx := strings.Index(src, "app.Window.NewWithOptions(application.WebviewWindowOptions{")
	if mainWinIdx == -1 {
		t.Fatalf("expected main WebviewWindow creation site (`app.Window.NewWithOptions(...)`) was not found in main.go -- has main window creation been refactored away?")
	}

	if splashIdx[0] >= mainWinIdx {
		t.Fatalf("splash creation (at byte %d) must occur BEFORE main window creation (at byte %d) so the splash is visible during WebView2 cold init. Reorder per story Task 2.1.", splashIdx[0], mainWinIdx)
	}
}

// ---------------------------------------------------------------------------
// Splash window options match (size, framing, non-interactivity).
//
// 480x320 logical pixels, frameless, no close/resize/minimise.
// Partial: frameless.
// ---------------------------------------------------------------------------

func TestSplashWindowOptionsMatchSpec(t *testing.T) {
	src := splashSource(t)

	requiredFields := []struct {
		pattern string
		why     string
	}{
		{`Width:\s*480`, "splash width must be 480"},
		{`Height:\s*320`, "splash height must be 320"},
		{`Frameless:\s*true`, "splash must be frameless (no title bar / chrome)"},
		{`AlwaysOnTop:\s*true`, "splash must be AlwaysOnTop until dismissal, then cleared before the crossfade"},
		{`Resizable:\s*false`, "splash must not be resizable"},
		// alpha.85 may name these Minimisable / Closable; tolerate either spelling
		{`(Minimisable|Minimizable):\s*false`, "splash must not be minimisable"},
		{`Closable:\s*false`, "splash must have no close button"},
		// Pins Code Review #2 M-1 fix: WebView's default context menu must be
		// disabled so right-click on the splash does not expose Reload / Inspect /
		// Back / Forward entries that would violate the "no context menu".
		{`DefaultContextMenuDisabled:\s*true`, "splash must disable the WebView default context menu"},
	}

	for _, f := range requiredFields {
		if !regexp.MustCompile(f.pattern).MatchString(src) {
			t.Errorf("splash window option %q not found. %s. "+
				"Searched: main.go and internal/splash/*.go.", f.pattern, f.why)
		}
	}
}

// ---------------------------------------------------------------------------
// Splash HTML content matches (icon, wordmark, activity indicator,
// version string).
//
// Shows icon + wordmark + three pulsing dots + literal version
// string. NO progress bar, NO percentage, NO "Loading..." text.
// HTML is bundled inline -- no external assets, no fetch over IPC.
// ---------------------------------------------------------------------------

func TestSplashHTMLContent(t *testing.T) {
	src := splashSource(t)

	// Wordmark literal text
	if !strings.Contains(src, "UniDoc PDF Debugger") {
		t.Errorf("splash content must contain wordmark 'UniDoc PDF Debugger'")
	}

	// Inlined SVG icon: <svg ... or base64 PNG fallback. Story Task 3.1
	// says inline the SVG contents of assets/branding/icon.svg.
	if !regexp.MustCompile(`(?is)<svg[^>]*>`).MatchString(src) &&
		!regexp.MustCompile(`data:image/(svg\+xml|png);base64,`).MatchString(src) {
		t.Errorf("splash content must inline the brand icon (either <svg...> or data:image/...;base64,). External /assets/... URLs are forbidden.")
	}

	// Three-dot activity indicator: CSS @keyframes referenced + at least
	// three elements with staggered animation-delay (0s / 0.4s / 0.8s per
	// Task 3.3).
	if !regexp.MustCompile(`@keyframes`).MatchString(src) {
		t.Errorf("splash content must define a CSS @keyframes for the three-dot pulse animation (Task 3.3, 1.2s cycle, opacity 0.3 -> 1 -> 0.3).")
	}
	if !regexp.MustCompile(`animation-delay:\s*0\.4s`).MatchString(src) ||
		!regexp.MustCompile(`animation-delay:\s*0\.8s`).MatchString(src) {
		t.Errorf("splash three-dot indicator must use 0s / 0.4s / 0.8s animation-delay (Task 3.3).")
	}

	// Version string placeholder: dev injects the runtime value of
	// main.version. Either {{.Version}} (templated) or a string-format
	// placeholder like %s, or a JS variable substitution.
	if !regexp.MustCompile(`\{\{\.?[Vv]ersion\}\}|%s|__VERSION__|window\.__VERSION__|\$\{version\}|<!--VERSION-->`).MatchString(src) {
		t.Errorf(": splash must contain a version-string placeholder bound to the Go main.version variable. Searched for {{.Version}}, %%s, __VERSION__, ${version}, <!--VERSION-->.")
	}

	// Forbidden content: no progress bar / percent / "Loading..." text
	forbidden := []string{
		"Loading...",
		"<progress",
		"role=\"progressbar\"",
		"role='progressbar'",
		"width: 100%; height: ", // common progress-bar pattern; permissive heuristic
	}
	for _, bad := range forbidden {
		if strings.Contains(src, bad) {
			t.Errorf("splash must NOT contain %q (no progress bar, no percentage, no 'Loading...' text).", bad)
		}
	}
}

// ---------------------------------------------------------------------------
// Splash background matches the main window's RGB.
//
// Background MUST match #f8fafc (the literal RGB(248, 250, 252) set on the
// main window at main.go:348). No dark-mode handling in this story.
// ---------------------------------------------------------------------------

func TestSplashBackgroundColorLiteral(t *testing.T) {
	src := splashSource(t)

	// Accept any of: BackgroundColour: application.NewRGB(248, 250, 252),
	// CSS background: #f8fafc, or CSS rgb(248, 250, 252).
	patterns := []string{
		`NewRGB\(\s*248\s*,\s*250\s*,\s*252\s*\)`,
		`(?i)background[^:]*:\s*#f8fafc`,
		`(?i)background-color\s*:\s*#f8fafc`,
		`rgb\(\s*248\s*,\s*250\s*,\s*252\s*\)`,
	}
	for _, p := range patterns {
		if regexp.MustCompile(p).MatchString(src) {
			return
		}
	}
	t.Errorf("splash background must use literal #f8fafc (rgb 248,250,252) to match main window's main.go:348. None of [NewRGB(248,250,252) / #f8fafc / rgb(248,250,252)] were found.")
}

// ---------------------------------------------------------------------------
// Splash HTML is bundled inline -- no external fetches.
//
// No external assets, no fetch over IPC. The inline HTML may reference
// data:image, system fonts, or base64 woff2, but must NOT load any
// http(s):// URL, /fonts/*, /assets/*, or anything else the splash
// WebView cannot reach without a server roundtrip.
// ---------------------------------------------------------------------------

func TestSplashHTMLHasNoExternalResources(t *testing.T) {
	src := splashSource(t)

	// Isolate the splash-specific portion. We look at the inline HTML
	// constant in Go or the standalone splash.html file. Heuristic:
	// search for the substring after "UniDoc PDF Debugger" and before
	// the next backtick / closing tag, but here we just scan the whole
	// splashSource because main.go's other strings are not HTML.
	//
	// Forbidden patterns INSIDE HTML-looking blocks.
	forbidden := []struct {
		pattern string
		why     string
	}{
		{`<link[^>]+href\s*=\s*["']https?://`, "external stylesheet URL forbidden"},
		{`<script[^>]+src\s*=\s*["']https?://`, "external script URL forbidden"},
		{`<img[^>]+src\s*=\s*["']/`, "absolute-path image src forbidden -- the splash WebView cannot fetch /assets/*"},
		{`@import\s+url\(\s*["']?https?://`, "CSS @import of external URL forbidden"},
		// /fonts/Inter-SemiBold.woff2 cannot be reached by the splash
		// WebView (it lives in the main frontend bundle), so an attempt
		// to load it would be a bug.
		{`url\(\s*["']?/fonts/`, "/fonts/* URL forbidden in splash -- splash WebView cannot reach main frontend bundle (Task 3.2)"},
		{`fetch\s*\(\s*["']https?://`, "fetch() of external URL forbidden"},
	}

	for _, f := range forbidden {
		if re := regexp.MustCompile(f.pattern); re.MatchString(src) {
			t.Errorf("splash HTML contains forbidden external resource pattern %q. %s", f.pattern, f.why)
		}
	}
}

// ---------------------------------------------------------------------------
// Min-display floor constant exists and equals 400ms.
//
// splashMinDisplayMs = 400. Constant must be declared as a named
// constant (not a magic number buried in a literal) so the unit-test
// delegation in can pin it.
// ---------------------------------------------------------------------------

func TestSplashMinDisplayMsConstant(t *testing.T) {
	src := splashSource(t)

	// Accept either `const splashMinDisplayMs = 400` or
	// `splashMinDisplayMs = 400 * time.Millisecond` or the typed-duration
	// form. Reject magic-number-only (e.g. raw `400` in a time.AfterFunc
	// call with no named constant binding).
	re := regexp.MustCompile(`splashMinDisplayMs\s*=\s*400\b`)
	if !re.MatchString(src) {
		t.Fatalf("a named constant `splashMinDisplayMs = 400` must exist (Task 4.1). Magic 400ms literals inside time.AfterFunc are not sufficient.")
	}
}

// ---------------------------------------------------------------------------
// Timeout constant exists and equals 30000ms.
//
// splashTimeoutMs = 30000.
// ---------------------------------------------------------------------------

func TestSplashTimeoutMsConstant(t *testing.T) {
	src := splashSource(t)

	re := regexp.MustCompile(`splashTimeoutMs\s*=\s*30000\b`)
	if !re.MatchString(src) {
		t.Fatalf("a named constant `splashTimeoutMs = 30000` must exist (Task 5.1).")
	}
}

// ---------------------------------------------------------------------------
// Dismissal path clears AlwaysOnTop and triggers crossfade.
//
// Before dismissal begins, the splash's AlwaysOnTop flag MUST be cleared
// so the main window can render above it. Crossfade is preferred but
// instantaneous swap is acceptable; we therefore require the
// AlwaysOnTop-clear assertion AND require some opacity/transition hook
// in the splash HTML or a SetAlpha call on the splash window.
// ---------------------------------------------------------------------------

func TestSplashDismissalClearsAlwaysOnTopAndFades(t *testing.T) {
	src := splashSource(t)

	// AlwaysOnTop clear: either splash.SetAlwaysOnTop(false), splash.AlwaysOnTop = false,
	// or whatever alpha.85 spells it. Be permissive.
	clearPattern := regexp.MustCompile(`(?i)(SetAlwaysOnTop\s*\(\s*false\s*\)|AlwaysOnTop\s*=\s*false)`)
	if !clearPattern.MatchString(src) {
		t.Errorf("splash dismissal must clear AlwaysOnTop before crossfade. " +
			"Searched for SetAlwaysOnTop(false) and AlwaysOnTop = false; neither found.")
	}

	// Crossfade hook: transition: opacity Xms in the splash HTML, OR a SetAlpha
	// call on the splash window, OR an EvaluateJS that toggles an
	// opacity-transition class. The instantaneous-swap fallback is allowed BUT
	// requires a Dev Notes entry explaining why. We assert the
	// crossfade-hook here; if dev falls back to instantaneous swap, dev must
	// edit this test to a `t.Skip` with a Dev Notes link per the fallback
	// clause.
	fadePatterns := []string{
		`transition\s*:\s*opacity\b`,
		`SetAlpha\s*\(`,
		`opacity\s*:\s*0\b.*transition`,
	}
	matched := false
	for _, p := range fadePatterns {
		if regexp.MustCompile(p).MatchString(src) {
			matched = true
			break
		}
	}
	if !matched {
		t.Errorf("splash dismissal must implement crossfade (transition: opacity / SetAlpha / opacity-transition CSS). Found none. " +
			"If instantaneous swap is being shipped instead, this test must be t.Skip'd with a Dev Notes reference.")
	}
}

// ---------------------------------------------------------------------------
// Splash is fully unmounted after dismissal.
//
// Splash MUST be fully unmounted (not just hidden behind the main
// window). Verify the dismissal path calls Close() / Destroy() on the
// splash window, not just SetVisible(false) or similar.
// ---------------------------------------------------------------------------

func TestSplashIsClosedAfterDismissal(t *testing.T) {
	src := splashSource(t)

	closePatterns := []string{
		`splash[A-Za-z]*\.Close\s*\(`,
		`splash[A-Za-z]*\.Destroy\s*\(`,
	}
	for _, p := range closePatterns {
		if regexp.MustCompile(p).MatchString(src) {
			return
		}
	}
	t.Errorf("splash window must be Close'd or Destroy'd after dismissal -- hiding alone leaves it in the OS window list. Searched for splash*.Close(...) and splash*.Destroy(...).")
}

// ---------------------------------------------------------------------------
// Splash creation does NOT appear inside OnSecondInstanceLaunch
// or ApplicationOpenedWithFile callbacks.
//
// Structural regression guard. Splash MUST live in the
// first-instance bootstrap path (top of main()), never inside the
// reentrant single-instance / file-association handlers.
// ---------------------------------------------------------------------------

func TestSplashNotCreatedInsideSecondInstanceCallback(t *testing.T) {
	src := readFile(t, "main.go")

	// Locate the callback bodies and check they do not contain anything
	// that could create a splash window. The body of each callback is
	// the next `{...}` block after the callback opening line.
	checkRegions := []struct {
		startMarker string
		why         string
	}{
		{"OnSecondInstanceLaunch:", "single-instance second launches must NOT spawn a splash"},
		{"ApplicationOpenedWithFile", "file-association open must NOT spawn a splash (the file goes to the existing window)"},
	}

	for _, region := range checkRegions {
		startIdx := strings.Index(src, region.startMarker)
		if startIdx == -1 {
			// callback isn't present -- nothing to gate against
			continue
		}
		// Find the opening `{` after the marker and walk balanced braces.
		braceIdx := strings.Index(src[startIdx:], "{")
		if braceIdx == -1 {
			continue
		}
		braceIdx += startIdx
		depth := 0
		end := braceIdx
		for i := braceIdx; i < len(src); i++ {
			switch src[i] {
			case '{':
				depth++
			case '}':
				depth--
			}
			if depth == 0 && i > braceIdx {
				end = i
				break
			}
		}
		body := src[braceIdx:end]

		forbidden := regexp.MustCompile(`(?i)(splash|createSplash|newSplash|showSplash|splashWindow)`)
		if forbidden.MatchString(body) {
			t.Errorf("callback at marker %q contains a splash reference. %s. "+
				"Body excerpt: %s", region.startMarker, region.why, snippet(body, 200))
		}
	}
}

// snippet returns the first n bytes of s, with newlines collapsed for
// single-line error messages.
func snippet(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ---------------------------------------------------------------------------
// No first-ever-launch persistence branch around splash creation.
//
// The splash is shown on every launch by design -- consistency is the
// brand signal. No "first-ever-launch only" gating.
// ---------------------------------------------------------------------------

func TestSplashHasNoFirstLaunchPersistenceGate(t *testing.T) {
	src := splashSource(t)

	// Forbidden: any condition that reads/writes a persistence file
	// keyed by "first launch" / "has launched" around the splash creation
	// call. Be permissive about the exact word -- check both common
	// idioms.
	forbidden := []string{
		`(?i)firstLaunch[^/]*splash`,
		`(?i)splash[^/]*firstLaunch`,
		`(?i)hasLaunchedBefore[^/]*splash`,
		`(?i)splash[^/]*hasLaunchedBefore`,
		`(?i)warmLaunch[^/]*splash`,
		`(?i)splash[^/]*warmLaunch`,
	}
	for _, p := range forbidden {
		if regexp.MustCompile(p).MatchString(src) {
			t.Errorf("splash must show on every launch. Found a first-launch/warm-launch gate matching %q. + 2026-05-19 design decision, no such branch is allowed.", p)
		}
	}
}

// ---------------------------------------------------------------------------
// Splash timeout error pane content.
//
// pre-bundled error pane reads "Could not start. Please reinstall." with
// a Close button. The platform-specific install URL is OUT of scope.
// ---------------------------------------------------------------------------

func TestSplashTimeoutErrorPaneContent(t *testing.T) {
	src := splashSource(t)

	if !strings.Contains(src, "Could not start. Please reinstall.") {
		t.Errorf("timeout error pane must contain the exact literal 'Could not start. Please reinstall.'.")
	}

	// Close button: either a <button>Close</button>, or a button with
	// id/class containing "close", or a button with Close text.
	if !regexp.MustCompile(`(?is)<button[^>]*>[^<]*Close[^<]*</button>`).MatchString(src) &&
		!regexp.MustCompile(`(?i)id\s*=\s*["']splashClose["']`).MatchString(src) {
		t.Errorf("timeout error pane must include a Close button (Task 5.2). Searched for <button>Close</button> and id=splashClose.")
	}

	// Wails event channel that the inline JS listens on for timeout
	// transition (revised mechanism).
	if !regexp.MustCompile(`splash:timeout`).MatchString(src) {
		t.Errorf("backend must emit a `splash:timeout` Wails event for the error-pane handoff (Task 5.2). Event name not found.")
	}
}

// ---------------------------------------------------------------------------
// Min-display floor + timeout race + version-string passthrough --
// delegated to internal/splash/splash_test.go.
//
// Story Task 6.1 requires extracting a small internal/splash
// package with an injectable clock interface so the scheduling
// logic is unit-testable without spinning up Wails.
//
// This delegation pattern mirrors tests/object-source-and-reverse-refs.
// The integration test runs `go test -run TestSplashScheduler -v ./...`
// inside internal/splash/ as a subprocess. If the package or named tests
// are missing, the subprocess fails and this test fails.
// ---------------------------------------------------------------------------

func TestDelegated_SplashSchedulerAndVersionRender(t *testing.T) {
	root := projectRoot(t)
	pkgDir := filepath.Join(root, "internal", "splash")
	if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
		t.Fatalf(": internal/splash package missing. Story Task 6.1 requires extracting the splash scheduler + version-render into an importable package with an injectable clock. Expected at %s", pkgDir)
	}

	// Delegate. Run all tests matching the splash unit-test naming
	// convention agreed in the ATDD checklist: TestSplashScheduler*,
	// TestSplashTimeout*, TestSplashRenderVersion*. If any are missing,
	// `go test -run` will report `no tests to run` and we treat that as
	// a fail (the checklist names every required pattern).
	requiredPatterns := []string{
		"TestSplashSchedulerMinDisplayFloorDefersDismissal",
		"TestSplashSchedulerMinDisplayFloorPassthrough",
		"TestSplashSchedulerTimeoutFires",
		"TestSplashSchedulerTimeoutRaceWinsByMainReady",
		"TestSplashRenderVersionFullSemver",
		"TestSplashRenderVersionPrereleaseSuffix",
		"TestSplashRenderVersionDevLiteral",
	}

	for _, pattern := range requiredPatterns {
		cmd := exec.Command("go", "test", "-run", "^"+pattern+"$", "-count=1", "-v", "./...")
		cmd.Dir = pkgDir
		out, err := cmd.CombinedOutput()
		output := string(out)
		if err != nil {
			t.Errorf("delegated unit test %s failed in internal/splash:\n%s\nerr: %v", pattern, output, err)
			continue
		}
		// `go test -run <regex>` exits 0 when no tests match. Catch that.
		if strings.Contains(output, "no tests to run") || strings.Contains(output, "no test files") {
			t.Errorf("no matching test found for pattern %q in internal/splash. The story Task 6.1 contract requires this test name.", pattern)
		}
	}
}

// ---------------------------------------------------------------------------
// Version string is rendered verbatim, not stripped.
//
// Full semver appears (`v0.2.0-rc1`), NOT the stripped `0.2.0`.
// Static-analysis check: the splash content path must NOT call any
// version-stripping helper (e.g. SemVer.Major(), strings.Split on '-').
// The pure-logic assertion is delegated to internal/splash (patterns
// TestSplashRenderVersion*); this integration test guards against a
// regression where someone wires the stripped form into the splash by
// accident.
// ---------------------------------------------------------------------------

func TestSplashVersionRenderIsNotStripped(t *testing.T) {
	src := splashSource(t)

	// We look for known stripping anti-patterns NEAR the version usage.
	// Permissive: if the file references the version variable at all,
	// it must not also call .Split('-') or .TrimSuffix('-rc') around
	// that reference.
	if !regexp.MustCompile(`(?i)version`).MatchString(src) {
		// The version string injection must exist somewhere; if it
		// doesn't, has not been wired and the dedicated content test
		// already fails. No need to double-fail.
		return
	}

	antiPatterns := []string{
		`strings\.Split\([^)]*[Vv]ersion[^)]*,\s*"-"\)`,
		`strings\.TrimSuffix\([^)]*[Vv]ersion[^)]*,\s*"-[a-z]+"\)`,
		`strings\.TrimSuffix\([^)]*[Vv]ersion[^)]*,\s*"-rc[0-9]*"\)`,
	}
	for _, p := range antiPatterns {
		if regexp.MustCompile(p).MatchString(src) {
			t.Errorf("version stripping pattern detected (%q). the FULL semver including prerelease suffix MUST be rendered.", p)
		}
	}
}

// ---------------------------------------------------------------------------
// Main window is created with body opacity 0 so it crossfades up
// cleanly.
//
// "The main window MUST be created with its body at opacity 0 (or
// hidden) and fade up only at dismissal -- otherwise the first paint
// shows a fully opaque main window for a frame before its transition
// starts, defeating the crossfade."
//
// This is observable in frontend/src/index.html, frontend/src/style.css,
// or frontend/src/App.tsx -- whichever surface the dev step chooses.
// ---------------------------------------------------------------------------

func TestMainWindowFirstPaintIsTransparent(t *testing.T) {
	root := projectRoot(t)

	// Candidate locations the dev step may wire opacity-0-on-first-paint.
	candidates := []string{
		filepath.Join("frontend", "index.html"),
		filepath.Join("frontend", "src", "main.tsx"),
		filepath.Join("frontend", "src", "main.jsx"),
		filepath.Join("frontend", "src", "App.tsx"),
		filepath.Join("frontend", "src", "App.jsx"),
		filepath.Join("frontend", "src", "style.css"),
		filepath.Join("frontend", "src", "index.css"),
	}

	var concat strings.Builder
	for _, c := range candidates {
		abs := filepath.Join(root, c)
		if data, err := os.ReadFile(abs); err == nil {
			concat.Write(data)
			concat.WriteString("\n")
		}
	}
	src := concat.String()

	// Either CSS `body { opacity: 0 }` (with transition managed by
	// dismissal IPC), or a React initial state that keeps the root
	// invisible until a `splash:dismissed` event from Wails.
	patterns := []string{
		`(?is)body\s*\{[^}]*opacity\s*:\s*0\b`,
		`(?is)html\s*\{[^}]*opacity\s*:\s*0\b`,
		`#root\s*\{[^}]*opacity\s*:\s*0\b`,
		`splash:dismissed`,
		`splashReady`,
	}
	for _, p := range patterns {
		if regexp.MustCompile(p).MatchString(src) {
			return
		}
	}
	t.Errorf("main window must start invisible (body/html/#root opacity 0, or wait on a splash:dismissed event) so the crossfade is not defeated by an opaque first paint. None of the expected patterns found in %d candidate files.", len(candidates))
}

// ---------------------------------------------------------------------------
// Build-time -ldflags wiring preserves the full semver for.
//
// Contingent on `VERSION` being set to the full semver by the
// upstream CI / release pipeline. The story explicitly says: "If
// VERSION itself is stripped upstream, that fix is a prerequisite and
// not scope-creep for this story."
//
// This test scans build/{darwin,linux,windows}/Taskfile.yml for the
// -X main.version={{.VERSION}} substitution and asserts no strip is
// applied to {{.VERSION}} in those files.
// ---------------------------------------------------------------------------

func TestVersionLDFlagsPreserveFullSemver(t *testing.T) {
	for _, platform := range []string{"darwin", "linux", "windows"} {
		rel := filepath.Join("build", platform, "Taskfile.yml")
		if !fileExists(t, rel) {
			// Acceptable to skip -- a platform-specific Taskfile may
			// not exist in every checkout (e.g. CI-only).
			continue
		}
		content := readFile(t, rel)

		// -X main.version=... must appear, bound to the unstripped VERSION.
		if !regexp.MustCompile(`-X\s+main\.version=`).MatchString(content) {
			t.Errorf("build/%s/Taskfile.yml does not contain `-X main.version=...` ldflag binding. The splash version-render depends on it.", platform)
			continue
		}

		// Disallow obvious stripping like
		// `-X main.version={{ .VERSION | trimSuffix "-rc1" }}` -- if
		// upstream sed-strips the rc suffix before it reaches Go, the
		// splash will inherit the stripped form.
		stripPatterns := []string{
			`trimSuffix\s+"-rc`,
			`splitList\s+"-"`,
			`\| strip`,
		}
		for _, p := range stripPatterns {
			if regexp.MustCompile(p).MatchString(content) {
				t.Errorf("build/%s/Taskfile.yml strips the version (%q) before reaching the Go binary. the prerelease suffix must be preserved.", platform, p)
			}
		}
	}
}
