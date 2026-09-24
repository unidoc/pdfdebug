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
	"time"

	"github.com/adrg/xdg"
	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/mod/semver"

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
	// cache is the shared check record; nil when its path could not be
	// resolved, in which case every cache touch is a no-op.
	cache *updatecheck.Cache
}

// NewUpdateService returns a Service that reports updates newer than version.
// version is the ldflag-injected build version ("dev" for untagged builds, for
// which the check is skipped and no network call is made). app is used to emit
// download-progress events; it may be nil in tests. Check results are shared
// with the CLI through the record at updatecheck.DefaultPath.
func NewUpdateService(app *application.App, version string) *Service {
	path, err := updatecheck.DefaultPath()
	if err != nil {
		path = ""
	}
	return newService(app, version, path)
}

// newService builds the Service with its cache at cachePath. An unusable path
// leaves the cache nil.
func newService(app *application.App, version, cachePath string) *Service {
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
	cache, err := updatecheck.Open(cachePath)
	if err != nil {
		cache = nil
	}
	return &Service{app: app, version: version, checker: checker, pause: gate, cache: cache}
}

// SetDownloadPaused pauses or resumes an in-flight download. Pausing stops
// reading the response body (the transfer halts via TCP backpressure); resuming
// continues it. Safe to call when no download is active.
func (s *Service) SetDownloadPaused(paused bool) {
	s.pause.set(paused)
}

// CheckForUpdate runs the cumulative release check for the running version and
// platform, always live, and records the outcome in the cache shared with the
// CLI. Errors are returned for logging; the frontend treats any error as
// "no update" and stays silent on the automatic path.
func (s *Service) CheckForUpdate(ctx context.Context) (updatecheck.Result, error) {
	res, err := s.checker.Check(ctx, s.version)
	s.record(ctx, res, err)
	return res, err
}

// CheckForUpdateAtStartup is the automatic launch check. When a check
// succeeded within the TTL and its latest version is not newer than the
// running version it answers from the record with no request; otherwise it
// runs CheckForUpdate. A failed attempt never counts, a record that names a
// newer version still goes live because it carries no release notes or
// download asset, and a prerelease build always goes live because the record
// holds stable versions only.
func (s *Service) CheckForUpdateAtStartup(ctx context.Context) (updatecheck.Result, error) {
	if v, ok := updatecheck.CheckableVersion(s.version); ok && semver.Prerelease(v) == "" {
		if snap, ok := s.loadSnapshot(); ok && snap.Confirmed(time.Now(), updatecheck.CacheTTL) {
			if _, newer := snap.Notice(s.version); !newer {
				return updatecheck.Result{InstalledVersion: s.version}, nil
			}
		}
	}
	return s.CheckForUpdate(ctx)
}

// loadSnapshot reads the shared record; a nil cache loads as absent.
func (s *Service) loadSnapshot() (updatecheck.Snapshot, bool) {
	if s.cache == nil {
		return updatecheck.Snapshot{}, false
	}
	return s.cache.Load()
}

// record stores the outcome of a live check. Every check advances
// checked_at. A successful one also stores the newest stable tag the server
// listed and advances succeeded_at; a failed one, or a success whose stable
// tag could not be learned, keeps the previous latest version and
// succeeded_at. A dev or non-SemVer build stores nothing, and a store error
// is ignored.
func (s *Service) record(ctx context.Context, res updatecheck.Result, err error) {
	if s.cache == nil {
		return
	}
	if _, ok := updatecheck.CheckableVersion(s.version); !ok {
		return
	}
	now := time.Now()
	next, _ := s.cache.Load()
	next.CheckedAt = now
	if err == nil {
		if latest, ok := s.latestStable(ctx, res); ok {
			next.SucceededAt = now
			if latest != "" {
				next.LatestVersion = latest
			}
		}
	}
	_ = s.cache.Store(next)
}

// latestStable returns the newest stable tag the release server listed. When
// res holds a stable release newer than the running version that is it;
// otherwise one releases page is fetched, because the running version itself
// may not be published and res only lists newer releases. "" with ok true
// means the server listed no stable release.
func (s *Service) latestStable(ctx context.Context, res updatecheck.Result) (string, bool) {
	// res.Releases is newest first.
	for _, r := range res.Releases {
		if tag, ok := updatecheck.CheckableVersion(r.TagName); ok && semver.Prerelease(tag) == "" {
			return tag, true
		}
	}
	tag, err := s.checker.LatestStable(ctx)
	return tag, err == nil
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
