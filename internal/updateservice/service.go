// Package updateservice is the thin Wails-bound adapter over internal/updatecheck.
// It holds the running version and delegates discovery and verified download to
// the pure-Go checker, adding only the host-coupled concerns the checker keeps
// out: resolving the user's Downloads directory and revealing the saved file in
// the OS file manager. Browser-open is handled frontend-side via the Wails
// runtime, so it is not exposed here.
package updateservice

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/adrg/xdg"
	"github.com/wailsapp/wails/v3/pkg/application"

	"unidoc-pdf-debugger/internal/updatecheck"
)

// updateProgressEvent is emitted to the frontend during a verified download so it
// can render a phase label and progress bar.
const updateProgressEvent = "update:download-progress"

// pauseGate blocks a download while paused and wakes on resume or context cancel.
type pauseGate struct {
	mu     sync.Mutex
	cond   *sync.Cond
	paused bool
}

func newPauseGate() *pauseGate {
	g := &pauseGate{}
	g.cond = sync.NewCond(&g.mu)
	return g
}

// set flips the paused flag, waking any waiter when unpausing.
func (g *pauseGate) set(paused bool) {
	g.mu.Lock()
	g.paused = paused
	if !paused {
		g.cond.Broadcast()
	}
	g.mu.Unlock()
}

// wait blocks while paused, returning ctx.Err() if the context is cancelled
// during the pause. A cancelled context also wakes the wait via AfterFunc.
func (g *pauseGate) wait(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.paused {
		return ctx.Err()
	}
	stop := context.AfterFunc(ctx, func() {
		g.mu.Lock()
		g.cond.Broadcast()
		g.mu.Unlock()
	})
	defer stop()
	for g.paused && ctx.Err() == nil {
		g.cond.Wait()
	}
	return ctx.Err()
}

// Service is the Wails service exposing update discovery and assisted download.
type Service struct {
	app     *application.App
	version string
	checker *updatecheck.Checker
	pause   *pauseGate
}

// NewUpdateService returns a Service that reports updates newer than version.
// version is the ldflag-injected build version ("dev" for untagged builds, for
// which the check is skipped and no network call is made). app is used to emit
// download-progress events; it may be nil in tests.
func NewUpdateService(app *application.App, version string) *Service {
	checker := updatecheck.New()
	gate := newPauseGate()
	checker.WaitIfPaused = gate.wait
	if app != nil {
		checker.OnProgress = func(p updatecheck.Progress) {
			app.Event.Emit(updateProgressEvent, map[string]any{
				"phase":    p.Phase,
				"received": p.Received,
				"total":    p.Total,
			})
		}
	}
	return &Service{app: app, version: version, checker: checker, pause: gate}
}

// SetDownloadPaused pauses or resumes an in-flight download. Pausing stops
// reading the response body (the transfer halts via TCP backpressure); resuming
// continues it. Safe to call when no download is active.
func (s *Service) SetDownloadPaused(paused bool) {
	s.pause.set(paused)
}

// CheckForUpdate runs the cumulative release check for the running version and
// platform. Errors are returned for logging; the frontend treats any error as
// "no update" and stays silent on the automatic path.
func (s *Service) CheckForUpdate(ctx context.Context) (updatecheck.Result, error) {
	return s.checker.Check(ctx, s.version)
}

// DownloadUpdate downloads assetURL, verifies it against sumsURL, moves the
// verified file into the user's Downloads directory, reveals it in the file
// manager (best effort), and returns the saved path. An unverified download
// never reaches Downloads.
func (s *Service) DownloadUpdate(ctx context.Context, assetURL, assetName, sumsURL string) (string, error) {
	// Clear any stale paused state from a prior download so this one is not
	// blocked before it starts.
	s.pause.set(false)
	dest, err := downloadsDir()
	if err != nil {
		return "", err
	}
	saved, err := s.checker.DownloadAndVerify(ctx, assetURL, assetName, sumsURL, dest)
	if err != nil {
		return "", err
	}
	revealInFileManager(saved)
	return saved, nil
}

// revealAndWait starts a best-effort reveal command without blocking, reaping the
// child in the background. Reveal failures never fail the download (already saved);
// some file managers (Explorer) exit non-zero even on success, so the status is
// ignored.
func revealAndWait(cmd *exec.Cmd) {
	if err := cmd.Start(); err != nil {
		return
	}
	go func() { _ = cmd.Wait() }()
}

// downloadsDir returns the user's Downloads directory, creating it if absent.
// It uses the platform's real location: adrg/xdg resolves the XDG user dir on
// Linux (which may be a localized name), and falls back to <home>/Downloads on
// macOS/Windows or when unset.
func downloadsDir() (string, error) {
	dir := xdg.UserDirs.Download
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, "Downloads")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}
