// Package splash implements the startup splash window's pure-logic surface:
// the min-display-floor scheduler, the failure-path timeout, the version
// string renderer, and the inline HTML template that the Wails WebviewWindow
// loads at boot. The Wails-side window creation, event subscription, and
// thread dispatch live in main.go; this package is pure Go with no Wails
// dependency, so the min-display floor, the timeout race and the version
// render are unit-testable in isolation.
//
// Picked Option B (separate WebviewWindow as the splash) over
// Option A (native pre-WebView window) because Wails v3 alpha.85 does not
// expose a pre-WebView native window primitive on Windows; rolling our own
// Win32 CreateWindowEx + NSWindow + GtkWindow trio would have ballooned the
// estimate past the 3-point budget and added platform-CGO surface area
// orthogonal to the v0.2.0 release. The Windows perception trade-off (the
// splash itself pays WebView2 cold-init tax on a clean Win11 install) is
// documented in deferred-work.md; see it for the
// follow-up to revisit when Wails exposes a true pre-WebView splash API.
package splash

import (
	"bytes"
	"html"
	"strings"
	"sync"
	"text/template"
	"time"
)

// splashMinDisplayMs is the floor (in milliseconds) the splash must remain
// visible before the dismissal handler is allowed to run. The floor prevents
// a flash-of-splash on fast platforms where the main window's WindowShow
// signal fires within a few hundred ms of splash creation.
const splashMinDisplayMs = 400

// splashTimeoutMs is the hard ceiling (in milliseconds) after which the
// splash transitions to its error pane via the `splash:timeout` Wails
// event. The ceiling bounds the perceived hang on installs where the main
// window's WebView never reaches WindowShow (e.g. WebView2 missing on
// Windows, WebKit2GTK missing on Linux).
const splashTimeoutMs = 30000

// Clock abstracts the wall clock + deferred-callback scheduler used by
// the splash logic so tests can drive virtual time without sleeping.
// Tests substitute a fake Clock that records AfterFunc registrations and
// fires them on demand.
type Clock interface {
	Now() time.Time
	AfterFunc(d time.Duration, f func()) Timer
}

// Timer is the minimal surface of time.Timer the splash scheduler uses.
// The interface lets the test Clock return a fake timer whose Stop() the
// scheduler can call when the success path beats the timeout.
type Timer interface {
	Stop() bool
}

// RealClock is the production Clock implementation: time.Now + time.AfterFunc.
type RealClock struct{}

// Now returns the current wall-clock time.
func (RealClock) Now() time.Time { return time.Now() }

// AfterFunc schedules f to run after d via time.AfterFunc and returns a
// Timer that wraps the underlying *time.Timer.
func (RealClock) AfterFunc(d time.Duration, f func()) Timer {
	return &realTimer{t: time.AfterFunc(d, f)}
}

type realTimer struct{ t *time.Timer }

func (r *realTimer) Stop() bool { return r.t.Stop() }

// Scheduler owns the splash's two time-sensitive callbacks: the dismissal
// (gated by the min-display floor) and the failure-path timeout. It is
// safe for concurrent use: the WindowShow event fires on a Wails
// dispatcher goroutine while AfterFunc callbacks fire on time-package
// goroutines, so all mutations of the once-flags go through atomic
// boolean writes under the embedded mutex.
type Scheduler struct {
	clock      Clock
	shownAt    time.Time
	minDisplay time.Duration
	timeout    time.Duration

	onDismiss func() // called when min-display floor met AND main-ready fired
	onTimeout func() // called once if main-ready never fires within timeout

	mu             sync.Mutex
	timeoutTimer   Timer
	mainReadyFired bool
	timeoutFired   bool
	dismissFired   bool
}

// NewScheduler returns a Scheduler primed with the production timings
// (splashMinDisplayMs / splashTimeoutMs) and the given callbacks. It
// immediately records the splash-shown-at timestamp from clock.Now() and
// arms the timeout timer. Callers MUST invoke MainWindowReady() when the
// main window's runtime-ready signal fires so the scheduler can run the
// dismissal callback (or no-op if the timeout already fired).
//
// onDismiss and onTimeout run on a clock-driven goroutine. The caller's
// callbacks are responsible for any thread dispatching they need; Wails
// alpha.85 window mutators (SetAlwaysOnTop / Show / Close) and
// app.Event.Emit already handle their own main-thread dispatch
// internally, so direct calls from a worker goroutine are safe.
func NewScheduler(clock Clock, onDismiss, onTimeout func()) *Scheduler {
	return newSchedulerWithTimings(clock, onDismiss, onTimeout,
		time.Duration(splashMinDisplayMs)*time.Millisecond,
		time.Duration(splashTimeoutMs)*time.Millisecond)
}

// newSchedulerWithTimings is the timing-injected constructor tests use.
// It mirrors NewScheduler but accepts explicit min-display + timeout
// durations so tests can shrink them to microseconds.
func newSchedulerWithTimings(clock Clock, onDismiss, onTimeout func(), minDisplay, timeout time.Duration) *Scheduler {
	s := &Scheduler{
		clock:      clock,
		minDisplay: minDisplay,
		timeout:    timeout,
		onDismiss:  onDismiss,
		onTimeout:  onTimeout,
	}
	s.shownAt = clock.Now()
	s.timeoutTimer = clock.AfterFunc(timeout, s.fireTimeout)
	return s
}

// MainWindowReady tells the scheduler the main window's WindowShow event
// has fired. If min-display floor has been met, onDismiss runs
// immediately on the caller's goroutine. Otherwise, it is scheduled via
// the Clock to run after the remaining floor elapses. Either way the
// timeout timer is stopped so onTimeout cannot also fire.
func (s *Scheduler) MainWindowReady() {
	s.mu.Lock()
	if s.mainReadyFired || s.timeoutFired || s.dismissFired {
		// idempotent: ignore double-fire and post-timeout calls
		s.mu.Unlock()
		return
	}
	s.mainReadyFired = true
	timer := s.timeoutTimer
	s.mu.Unlock()

	if timer != nil {
		timer.Stop()
	}

	elapsed := s.clock.Now().Sub(s.shownAt)
	if elapsed >= s.minDisplay {
		s.runDismiss()
		return
	}
	// Schedule via Clock so tests can drive virtual time.
	remaining := s.minDisplay - elapsed
	s.clock.AfterFunc(remaining, s.runDismiss)
}

// runDismiss invokes onDismiss exactly once. Subsequent calls are no-ops.
// The min-display-floor branch in MainWindowReady can call runDismiss
// from two goroutines (the synchronous fast path and the AfterFunc
// scheduled path) if MainWindowReady is called twice; the once-flag
// guards against that.
func (s *Scheduler) runDismiss() {
	s.mu.Lock()
	if s.dismissFired || s.timeoutFired {
		s.mu.Unlock()
		return
	}
	s.dismissFired = true
	cb := s.onDismiss
	s.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// fireTimeout is the timer-armed callback. It invokes onTimeout once
// unless MainWindowReady beat it.
func (s *Scheduler) fireTimeout() {
	s.mu.Lock()
	if s.mainReadyFired || s.timeoutFired || s.dismissFired {
		s.mu.Unlock()
		return
	}
	s.timeoutFired = true
	cb := s.onTimeout
	s.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// RenderVersion converts the value of main.version into the display
// string for the splash version label. The label carries the FULL semver
// including any prerelease suffix (e.g. v0.2.0-rc1, NOT 0.2.0). The
// special-case `dev` (untagged local build) is rendered verbatim with no
// "v" prefix so the build origin is obvious. Empty / whitespace-only
// input is treated as `dev` so a build without an -X main.version flag
// shows the untagged-build label rather than a blank line.
func RenderVersion(version string) string {
	v := strings.TrimSpace(version)
	if v == "" || v == "dev" {
		return "dev"
	}
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

// splashHTMLTemplate is the inline HTML the splash WebviewWindow loads.
// It is intentionally a single Go string constant (no external assets,
// no fetch over IPC) -- the dev source mirror lives at
// assets/splash/splash.html and must be kept in sync by hand.
//
// {{.Version}} is the only substitution point; Render(version) replaces
// it with RenderVersion(version) output.
const splashHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>UniDoc PDF Debugger</title>
<style>
  html, body {
    margin: 0;
    padding: 0;
    width: 100%;
    height: 100%;
    background: #f8fafc;
    background-color: #f8fafc;
    opacity: 1;
    transition: opacity 200ms ease-out;
    overflow: hidden;
    user-select: none;
    -webkit-user-select: none;
    cursor: default;
  }
  body.splash-dismissing { opacity: 0; }
  body {
    font-family: 'Inter', system-ui, -apple-system, BlinkMacSystemFont,
                 'Segoe UI', Roboto, sans-serif;
    color: #0f172a;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: flex-start;
  }
  .splash-stack {
    width: 100%;
    height: 100%;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: flex-start;
    padding-top: 64px;
    box-sizing: border-box;
    position: relative;
  }
  .splash-icon { width: 128px; height: 128px; display: block; }
  .splash-wordmark {
    margin: 20px 0 0 0;
    font-size: 19px;
    font-weight: 600;
    letter-spacing: 0.1px;
    color: #0f172a;
    line-height: 1.2;
  }
  .splash-dots {
    margin-top: 32px;
    display: flex;
    gap: 8px;
  }
  .splash-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: #3b82f6;
    background-color: #3b82f6;
    opacity: 0.3;
    animation: splash-dot-pulse 1.2s ease-in-out infinite;
  }
  .splash-dot:nth-child(1) { animation-delay: 0s; }
  .splash-dot:nth-child(2) { animation-delay: 0.4s; }
  .splash-dot:nth-child(3) { animation-delay: 0.8s; }
  @keyframes splash-dot-pulse {
    0%   { opacity: 0.3; }
    50%  { opacity: 1; }
    100% { opacity: 0.3; }
  }
  .splash-version {
    position: absolute;
    bottom: 16px;
    left: 0;
    right: 0;
    text-align: center;
    color: #64748b;
    font-size: 12px;
    line-height: 1;
  }
  .splash-error {
    display: none;
    position: absolute;
    inset: 0;
    background: #f8fafc;
    background-color: #f8fafc;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 24px;
    box-sizing: border-box;
    text-align: center;
  }
  body.splash-failed .splash-stack > :not(.splash-error) { display: none; }
  body.splash-failed .splash-error { display: flex; }
  .splash-error-msg {
    font-size: 15px;
    font-weight: 500;
    color: #0f172a;
    margin-bottom: 20px;
  }
  .splash-error-close {
    appearance: none;
    border: 1px solid #cbd5e1;
    background: #ffffff;
    color: #0f172a;
    padding: 8px 20px;
    font-size: 14px;
    font-family: inherit;
    border-radius: 6px;
    cursor: pointer;
  }
  .splash-error-close:hover { background: #f1f5f9; }
</style>
</head>
<body>
<div class="splash-stack">
  <svg class="splash-icon" viewBox="0 0 1024 1024" xmlns="http://www.w3.org/2000/svg" role="img" aria-label="UniDoc PDF Debugger icon">
    <rect width="1024" height="1024" rx="200" fill="#7F1D1D"/>
    <path d="M 240 200 L 640 200 L 800 360 L 800 824 L 240 824 Z" fill="#FAF1DD"/>
    <polygon points="640,200 640,360 800,360" fill="#CBC0A4"/>
    <path d="M 380 480 L 540 600 L 380 720" stroke="#7F1D1D" stroke-width="80" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
  </svg>
  <h1 class="splash-wordmark">UniDoc PDF Debugger</h1>
  <div class="splash-dots" aria-hidden="true">
    <span class="splash-dot"></span>
    <span class="splash-dot"></span>
    <span class="splash-dot"></span>
  </div>
  <div class="splash-version" id="splashVersion">{{.Version}}</div>
  <div class="splash-error" role="alert">
    <div class="splash-error-msg">Could not start. Please reinstall.</div>
    <button class="splash-error-close" id="splashClose" type="button">Close</button>
  </div>
</div>
<script>
  // The splash WebView loads its HTML via WebviewWindowOptions.HTML
  // (loadHTMLString on macOS, NavigateToString on Windows), so the page
  // has no baseURL and never fetches /wails/runtime.js. That means the
  // full @wailsio/runtime module is NOT loaded here: window.wails.Events
  // is undefined, and Wails-side app.Event.Emit() cannot reach a JS
  // Events.On() listener that does not exist.
  //
  // Wails Go-side dispatch falls back to ExecJS that calls
  //   if (window._wails && window._wails.dispatchWailsEvent) {
  //     window._wails.dispatchWailsEvent(event);
  //   }
  // (see WebviewWindow.DispatchWailsEvent in alpha.85). The platform
  // runtime.Core injection that fires after navigation completion sets
  // window._wails to an object but does NOT define dispatchWailsEvent
  // (that lives in @wailsio/runtime/src/events.ts which is never loaded
  // for HTML-loaded windows). We define our own dispatchWailsEvent stub
  // below so splash:dismiss (fade) and splash:timeout (error pane
  // reveal) reach the splash. See the splash review notes for the full
  // analysis.
  //
  // The Close button on the error pane cannot Events.Emit back to Go
  // for the same reason. We instead use a hard signal: close the splash
  // window. Go listens for WindowClosing on the splash window and, if
  // the failure state was reached, calls app.Quit().
  window._wails = window._wails || {};
  var originalDispatch = window._wails.dispatchWailsEvent;
  window._wails.dispatchWailsEvent = function (event) {
    try {
      if (event && event.name === 'splash:timeout') {
        document.body.classList.add('splash-failed');
      } else if (event && event.name === 'splash:dismiss') {
        document.body.classList.add('splash-dismissing');
      }
    } catch (_) { /* swallow */ }
    if (typeof originalDispatch === 'function') {
      try { originalDispatch(event); } catch (_) { /* swallow */ }
    }
  };
  // Wails gates WebviewWindow.ExecJS (and therefore Go-side
  // DispatchWailsEvent) behind w.runtimeLoaded, which only flips true
  // when JS sends the "wails:runtime:ready" invoke message. The full
  // @wailsio/runtime module normally does that from its index.ts, but
  // the splash WebView never loads it. Without the manual signal below
  // every app.Event.Emit() to the splash is queued forever and the
  // splash:dismiss / splash:timeout events never reach our dispatch
  // stub. window._wails.invoke is installed by runtime.Core after
  // NavigationCompleted, so we poll until it appears, then fire the
  // ready signal exactly once. 50 tries x 100ms = 5s ceiling.
  (function () {
    var tries = 0;
    var iv = setInterval(function () {
      tries++;
      if (window._wails && typeof window._wails.invoke === 'function') {
        clearInterval(iv);
        try { window._wails.invoke('wails:runtime:ready'); } catch (_) { /* swallow */ }
      } else if (tries > 50) {
        console.warn("splash: runtime-ready ping gave up after " + (tries * 100) + "ms", { tries: tries, elapsedMs: tries * 100, wailsShape: typeof window._wails === 'object' ? Object.keys(window._wails || {}) : null });
        clearInterval(iv);
      }
    }, 100);
  })();
  var splashCloseBtn = document.getElementById('splashClose');
  if (splashCloseBtn) {
    splashCloseBtn.addEventListener('click', function () {
      // window.close() is honoured by WebView2 on Windows and propagates
      // to WM_CLOSE which Wails maps to WindowClosing. On WKWebView /
      // WebKit2GTK it is generally a no-op for top-level windows; Go's
      // 60s force-quit timer armed alongside splash:timeout is the
      // safety net there.
      try { window.close(); } catch (_) { /* swallow */ }
    });
  }
</script>
</body>
</html>`

// splashTmpl is the parsed splashHTMLTemplate, compiled once at package init.
// text/template (not html/template) is deliberate: the only dynamic field is
// pre-escaped by Render via html.EscapeString, so html/template would
// double-escape it. Using text/template keeps the substitution composable for
// future fields without changing the escaping contract.
var splashTmpl = template.Must(template.New("splash").Parse(splashHTMLTemplate))

// Render returns the splash HTML with {{.Version}} replaced by the
// rendered form of the supplied version string (see RenderVersion).
// The rendered version is HTML-escaped so a malformed `-ldflags -X
// main.version=...` value (e.g. one containing `</div><script>`) cannot
// break out of the version div and inject markup into the splash WebView.
func Render(version string) string {
	var buf bytes.Buffer
	// Ignoring the error is safe: splashTmpl is a valid parsed template and
	// bytes.Buffer writes never fail, so Execute cannot error here.
	_ = splashTmpl.Execute(&buf, struct{ Version string }{Version: html.EscapeString(RenderVersion(version))})
	return buf.String()
}

// MinDisplay returns the min-display floor as a time.Duration. Exposed
// so main.go can log the configured floor for boot diagnostics and so
// tests can read the same value the scheduler uses without re-parsing
// the millisecond constant.
func MinDisplay() time.Duration {
	return time.Duration(splashMinDisplayMs) * time.Millisecond
}

// Timeout returns the failure-path timeout as a time.Duration.
func Timeout() time.Duration {
	return time.Duration(splashTimeoutMs) * time.Millisecond
}
