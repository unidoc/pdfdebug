package splash

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeTimer is the test stand-in for *time.Timer. It records its
// scheduled duration and lets the test fire its callback on demand.
type fakeTimer struct {
	clock   *fakeClock
	at      time.Time
	fn      func()
	stopped atomic.Bool
	fired   atomic.Bool
}

func (t *fakeTimer) Stop() bool {
	already := t.stopped.Swap(true)
	return !already && !t.fired.Load()
}

// fakeClock is a deterministic Clock implementation. AfterFunc records
// the firing time; tests advance the clock via Advance() which fires any
// timers whose deadlines are now in the past.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) AfterFunc(d time.Duration, f func()) Timer {
	c.mu.Lock()
	t := &fakeTimer{clock: c, at: c.now.Add(d), fn: f}
	c.timers = append(c.timers, t)
	c.mu.Unlock()
	return t
}

// Advance moves the clock forward by d and fires any non-stopped timers
// whose firing time is now <= clock.now. Timer callbacks are run on the
// goroutine that called Advance, mirroring the synchronous-test pattern
// from tests/object-source-and-reverse-refs.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	target := c.now
	due := make([]*fakeTimer, 0)
	for _, t := range c.timers {
		if !t.stopped.Load() && !t.fired.Load() && !t.at.After(target) {
			due = append(due, t)
		}
	}
	c.mu.Unlock()
	for _, t := range due {
		if t.stopped.Load() {
			continue
		}
		if t.fired.Swap(true) {
			continue
		}
		t.fn()
	}
}

// shortTimings shrinks the production constants so a single Scheduler
// run completes in microseconds without sleeping.
func shortTimings() (min, to time.Duration) {
	return 400 * time.Microsecond, 30_000 * time.Microsecond
}

// Min-display floor defers dismissal when MainWindowReady fires earlier
// than splashMinDisplayMs.
func TestSplashSchedulerMinDisplayFloorDefersDismissal(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	var dismissed atomic.Int32
	var timedOut atomic.Int32
	min, to := shortTimings()
	s := newSchedulerWithTimings(clk,
		func() { dismissed.Add(1) },
		func() { timedOut.Add(1) },
		min, to,
	)
	_ = s

	// Fire MainWindowReady at t=100us, well before the 400us floor.
	clk.Advance(100 * time.Microsecond)
	s.MainWindowReady()

	if got := dismissed.Load(); got != 0 {
		t.Fatalf("dismissal must NOT fire before min-display floor; got dismissed=%d at elapsed=100us (floor=400us)", got)
	}

	// Advance to t=399us -- still inside the floor.
	clk.Advance(299 * time.Microsecond)
	if got := dismissed.Load(); got != 0 {
		t.Fatalf("dismissal must NOT fire at t=399us; got dismissed=%d", got)
	}

	// Cross the floor: t=400us. The AfterFunc deferred dismissal must now run.
	clk.Advance(1 * time.Microsecond)
	if got := dismissed.Load(); got != 1 {
		t.Fatalf("dismissal must fire exactly once after min-display floor met; got dismissed=%d", got)
	}
	if got := timedOut.Load(); got != 0 {
		t.Fatalf("timeout must NOT fire on the deferred-dismiss path; got timedOut=%d", got)
	}
}

// When MainWindowReady fires AFTER min-display, dismissal runs
// immediately (no deferred AfterFunc).
func TestSplashSchedulerMinDisplayFloorPassthrough(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	var dismissed atomic.Int32
	var timedOut atomic.Int32
	min, to := shortTimings()
	s := newSchedulerWithTimings(clk,
		func() { dismissed.Add(1) },
		func() { timedOut.Add(1) },
		min, to,
	)

	// Advance past the floor BEFORE MainWindowReady fires.
	clk.Advance(500 * time.Microsecond)
	s.MainWindowReady()

	if got := dismissed.Load(); got != 1 {
		t.Fatalf("dismissal must fire immediately when MainWindowReady is past floor; got dismissed=%d", got)
	}
	if got := timedOut.Load(); got != 0 {
		t.Fatalf("timeout must NOT fire on the immediate-dismiss path; got timedOut=%d", got)
	}
}

// After splashTimeoutMs elapses without MainWindowReady, onTimeout is
// called exactly once.
func TestSplashSchedulerTimeoutFires(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	var dismissed atomic.Int32
	var timedOut atomic.Int32
	min, to := shortTimings()
	_ = newSchedulerWithTimings(clk,
		func() { dismissed.Add(1) },
		func() { timedOut.Add(1) },
		min, to,
	)

	// Cross the timeout horizon (30_000us).
	clk.Advance(to + 1*time.Microsecond)

	if got := timedOut.Load(); got != 1 {
		t.Fatalf("timeout must fire exactly once after timeout horizon; got timedOut=%d", got)
	}
	if got := dismissed.Load(); got != 0 {
		t.Fatalf("dismissal must NOT fire on the timeout path; got dismissed=%d", got)
	}
}

// When MainWindowReady fires BEFORE the timeout, the timeout
// callback does NOT fire even if the clock later crosses the timeout
// horizon. Guards the success-path race per Task 5.3.
func TestSplashSchedulerTimeoutRaceWinsByMainReady(t *testing.T) {
	clk := newFakeClock(time.Unix(0, 0))
	var dismissed atomic.Int32
	var timedOut atomic.Int32
	min, to := shortTimings()
	s := newSchedulerWithTimings(clk,
		func() { dismissed.Add(1) },
		func() { timedOut.Add(1) },
		min, to,
	)

	// Beat the timeout: MainWindowReady at t=500us, dismissal runs immediately.
	clk.Advance(500 * time.Microsecond)
	s.MainWindowReady()
	if got := dismissed.Load(); got != 1 {
		t.Fatalf("dismissal must fire on success path; got dismissed=%d", got)
	}

	// Now advance well past the timeout horizon; the timer was Stop()'d so
	// onTimeout must NOT fire.
	clk.Advance(to * 2)
	if got := timedOut.Load(); got != 0 {
		t.Fatalf("timeout must NOT fire when MainWindowReady beat it; got timedOut=%d", got)
	}
	if got := dismissed.Load(); got != 1 {
		t.Fatalf("dismissal must remain idempotent (exactly one call); got dismissed=%d", got)
	}
}

// Full semver renders verbatim with `v` prefix.
func TestSplashRenderVersionFullSemver(t *testing.T) {
	if got := RenderVersion("0.2.0"); got != "v0.2.0" {
		t.Errorf("RenderVersion(0.2.0) = %q, want v0.2.0", got)
	}
	if got := RenderVersion("v0.2.0"); got != "v0.2.0" {
		t.Errorf("RenderVersion(v0.2.0) = %q, want v0.2.0 (idempotent v-prefix)", got)
	}
}

// Prerelease suffix MUST be preserved (no stripping the -rc1).
func TestSplashRenderVersionPrereleaseSuffix(t *testing.T) {
	cases := []struct{ in, want string }{
		{"0.2.0-rc1", "v0.2.0-rc1"},
		{"v0.2.0-rc1", "v0.2.0-rc1"},
		{"v1.0.0-beta.2+build.7", "v1.0.0-beta.2+build.7"},
	}
	for _, c := range cases {
		got := RenderVersion(c.in)
		if got != c.want {
			t.Errorf("RenderVersion(%q) = %q, want %q (full semver, no strip)", c.in, got, c.want)
		}
		if strings.HasSuffix(c.in, "-rc1") && !strings.Contains(got, "-rc1") {
			t.Errorf("RenderVersion(%q) dropped the -rc1 suffix: got %q", c.in, got)
		}
	}
}

// The literal `dev` (untagged local build) renders as `dev` with NO `v`
// prefix to make the build origin obvious.
func TestSplashRenderVersionDevLiteral(t *testing.T) {
	if got := RenderVersion("dev"); got != "dev" {
		t.Errorf("RenderVersion(dev) = %q, want dev (no v-prefix per Task 3.4)", got)
	}
	if got := RenderVersion(""); got != "dev" {
		t.Errorf("RenderVersion(\"\") = %q, want dev (empty defaults to dev)", got)
	}
	if got := RenderVersion("  "); got != "dev" {
		t.Errorf("RenderVersion(whitespace) = %q, want dev", got)
	}
}

// Render() returns HTML that contains the rendered version and the
// signature splash markers. Sanity check that the template substitution
// happened (the {{.Version}} placeholder is gone, the rendered value is
// in the output, the wordmark + dots + error-pane fingerprints are
// present).
func TestSplashRenderEmbedsVersionAndMarkers(t *testing.T) {
	html := Render("v0.2.0-rc1")
	if strings.Contains(html, "{{.Version}}") {
		t.Errorf("Render() left the {{.Version}} placeholder unreplaced")
	}
	if !strings.Contains(html, "v0.2.0-rc1") {
		t.Errorf("Render() output missing the rendered version v0.2.0-rc1")
	}
	for _, marker := range []string{
		"UniDoc PDF Debugger",
		"splash-dot",
		"@keyframes splash-dot-pulse",
		"Could not start. Please reinstall.",
		"splash:timeout",
		"splash:dismiss",
		"id=\"splashClose\"",
	} {
		if !strings.Contains(html, marker) {
			t.Errorf("Render() output missing marker %q", marker)
		}
	}
}

func TestSplashMinDisplayAndTimeoutConstants(t *testing.T) {
	if MinDisplay() != 400*time.Millisecond {
		t.Errorf("MinDisplay() = %v, want 400ms", MinDisplay())
	}
	if Timeout() != 30*time.Second {
		t.Errorf("Timeout() = %v, want 30s", Timeout())
	}
}

// The runtime-ready ping give-up branch must emit a single diagnostic
// console.warn before clearInterval. The test layer cannot execute the inline
// JS, so it grep-asserts the diagnostic SOURCE is present in Render() output:
// the warn message fragment and the wailsShape payload key.
//
// RED PHASE: fails against the current splash.go give-up branch (the
// `else if (tries > 50)` block) which only calls clearInterval(iv) with no
// console.warn. Once the diagnostic is added, both fragments appear in the
// rendered HTML template.
func TestSplashRuntimeReadyGiveUpEmitsDiagnostic(t *testing.T) {
	html := Render("0.0.0")

	// The give-up warn message. specifies the rendered text fragment.
	if !strings.Contains(html, "runtime-ready ping gave up after") {
		t.Errorf("Render() output missing the give-up console.warn message fragment %q", "runtime-ready ping gave up after")
	}

	// The diagnostic payload object must carry the last-observed _wails shape
	// under the wailsShape key so support can recover the runtime state.
	if !strings.Contains(html, "wailsShape") {
		t.Errorf("Render() output missing the %q diagnostic payload key", "wailsShape")
	}

	// The give-up branch must still clear the interval so the warn is one-shot.
	if !strings.Contains(html, "clearInterval(iv)") {
		t.Errorf("Render() output missing clearInterval(iv) in the runtime-ready ping")
	}
}
