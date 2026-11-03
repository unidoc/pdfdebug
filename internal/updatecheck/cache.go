package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/adrg/xdg"
	"golang.org/x/mod/semver"

	"unidoc-pdf-debugger/internal/atomicfile"
)

// CacheTTL is how long a stored check result stays fresh. The GUI and the CLI
// use the same value.
const CacheTTL = 24 * time.Hour

// Surface names the binary that owns a record. Each surface writes only its
// own record, so the desktop app and the CLI never overwrite each other, and
// either one works when the other is not installed.
type Surface string

const (
	// SurfaceApp is the desktop app, whose check walks up to maxPages release
	// pages.
	SurfaceApp Surface = "app"
	// SurfaceCLI is the command-line tool, whose refresh reads one page.
	SurfaceCLI Surface = "cli"
)

// peer returns the other surface.
func (s Surface) peer() Surface {
	if s == SurfaceApp {
		return SurfaceCLI
	}
	return SurfaceApp
}

// fileName is the surface's record file name.
func (s Surface) fileName() string {
	return "updatecheck-" + string(s) + ".json"
}

const (
	// cacheSchema is the only record schema Load accepts.
	cacheSchema = 1
	// cacheDirName is the per-user directory under xdg.CacheHome.
	cacheDirName = "pdfdebug"
)

// Snapshot is one surface's check record: what the release server said and
// when it was last asked. It holds no installed version and no derived "update
// available" flag, because one machine can run a GUI and a standalone CLI at
// different versions; each binary compares LatestVersion against its own.
//
// CheckedAt is the last attempt and throttles retries; SucceededAt is the last
// check that got an answer and is what makes LatestVersion trustworthy. A
// failed or interrupted attempt advances CheckedAt only.
type Snapshot struct {
	Schema        int       `json:"schema"`
	CheckedAt     time.Time `json:"checked_at"`
	SucceededAt   time.Time `json:"succeeded_at"`
	LatestVersion string    `json:"latest_version"`
}

// Fresh reports whether the last attempt was less than ttl before now. The
// zero Snapshot and a CheckedAt in the future are both stale.
func (s Snapshot) Fresh(now time.Time, ttl time.Duration) bool {
	return within(s.CheckedAt, now, ttl)
}

// Confirmed reports whether the last successful check was less than ttl
// before now, so LatestVersion can stand in for a live answer. A zero or
// future SucceededAt is not confirmed.
func (s Snapshot) Confirmed(now time.Time, ttl time.Duration) bool {
	return within(s.SucceededAt, now, ttl)
}

func within(t, now time.Time, ttl time.Duration) bool {
	if t.IsZero() || t.After(now) {
		return false
	}
	return now.Sub(t) < ttl
}

// Notice reports whether LatestVersion is newer than installedVersion. latest
// is returned without a leading "v", ready for display. Both sides must be
// valid SemVer and installedVersion must not be the "dev" sentinel; nothing
// that fails validation is ever returned.
func (s Snapshot) Notice(installedVersion string) (latest string, available bool) {
	installed, ok := CheckableVersion(installedVersion)
	if !ok {
		return "", false
	}
	target, ok := canonicalVersion(s.LatestVersion)
	if !ok || target == "" || semver.Compare(target, installed) <= 0 {
		return "", false
	}
	return strings.TrimPrefix(target, "v"), true
}

// Cache reads and writes its surface's record and reads the peer surface's.
// Both live in one per-user directory. The mutex serialises every read, write
// and load-then-store inside a process; processes of the same surface are
// last-writer-wins through an atomic rename.
type Cache struct {
	path     string
	peerPath string
	// mu also covers reads because on Windows an open reader makes a
	// concurrent rename onto the file fail; readers in other processes are
	// what write's rename retry is for.
	mu sync.Mutex
	// rename and attempts are os.Rename and atomicfile.DefaultAttempts,
	// replaced in tests.
	rename   func(oldpath, newpath string) error
	attempts int
}

// DefaultDir returns xdg.CacheHome/pdfdebug. It never touches the disk, and
// errors only when xdg.CacheHome is empty or relative.
func DefaultDir() (string, error) {
	base := xdg.CacheHome
	if base == "" || !filepath.IsAbs(base) {
		return "", fmt.Errorf("cache home %q is not an absolute path", base)
	}
	return filepath.Join(base, cacheDirName), nil
}

// Open returns the Cache for surface own in dir, an absolute directory such
// as DefaultDir returns. Open does not touch the disk; the first Store
// creates the directory.
func Open(dir string, own Surface) (*Cache, error) {
	if dir == "" || !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("cache dir %q is not an absolute path", dir)
	}
	if own != SurfaceApp && own != SurfaceCLI {
		return nil, fmt.Errorf("unknown cache surface %q", own)
	}
	return &Cache{
		path:     filepath.Join(dir, own.fileName()),
		peerPath: filepath.Join(dir, own.peer().fileName()),
		rename:   os.Rename,
		attempts: atomicfile.DefaultAttempts,
	}, nil
}

// Load returns this surface's record. A missing, unreadable, corrupt or
// truncated file, an unknown schema, or a latest version that is not SemVer
// all load as absent. It never returns an error and never writes output.
func (c *Cache) Load() (Snapshot, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return load(c.path)
}

// LoadPeer returns the other surface's record under the same rules as Load.
// It is absent when the other surface is not installed or has never checked.
func (c *Cache) LoadPeer() (Snapshot, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return load(c.peerPath)
}

// Store writes this surface's record atomically, setting the schema itself
// and writing LatestVersion in canonical form (leading "v") or empty. A
// non-empty LatestVersion that is not valid SemVer is refused and nothing is
// written. Callers treat any error as "no cache".
func (c *Cache) Store(s Snapshot) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.store(s)
}

// RecordAttempt advances checked_at to now and keeps the rest of the stored
// record. Call it before asking the server, so a failed, slow or interrupted
// request still throttles the next retry without counting as an answer. It
// returns the record as written, or as loaded when the write fails.
func (c *Cache) RecordAttempt(now time.Time) (Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, _ := load(c.path)
	s.CheckedAt = now
	return s, c.store(s)
}

// RecordSuccess records an answer from the server at now: checked_at and
// succeeded_at advance, and latest replaces the stored latest version unless
// it is "". The returned record is the one written, even when the write fails.
func (c *Cache) RecordSuccess(now time.Time, latest string) (Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, _ := load(c.path)
	s.CheckedAt = now
	s.SucceededAt = now
	if latest != "" {
		s.LatestVersion = latest
	}
	return s, c.store(s)
}

func load(path string) (Snapshot, bool) {
	var s Snapshot
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &s) != nil || s.Schema != cacheSchema {
		return Snapshot{}, false
	}
	latest, ok := canonicalVersion(s.LatestVersion)
	if !ok {
		return Snapshot{}, false
	}
	s.LatestVersion = latest
	return s, true
}

// store validates s and writes it; the caller holds c.mu.
func (c *Cache) store(s Snapshot) error {
	latest, ok := canonicalVersion(s.LatestVersion)
	if !ok {
		return fmt.Errorf("latest version %q is not valid SemVer", s.LatestVersion)
	}
	s.Schema = cacheSchema
	s.LatestVersion = latest
	return c.write(s)
}

// write marshals s and replaces the record through a temp file in the same
// directory. The caller holds c.mu.
func (c *Cache) write(s Snapshot) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	pattern := strings.TrimSuffix(filepath.Base(c.path), ".json") + "-*.tmp"
	return atomicfile.Writer{Rename: c.rename, Attempts: c.attempts}.Write(c.path, data, pattern, 0o700)
}

// Newer returns whichever of a and b names the higher latest version, and a
// when they tie or neither names a valid one.
func Newer(a, b Snapshot) Snapshot {
	av, _ := canonicalVersion(a.LatestVersion)
	bv, _ := canonicalVersion(b.LatestVersion)
	if bv != "" && (av == "" || semver.Compare(bv, av) > 0) {
		return b
	}
	return a
}

// CheckableVersion reports whether a build version can be compared against a
// release: not the "dev" sentinel and valid SemVer with or without a leading
// "v". It returns the version in canonical form.
func CheckableVersion(v string) (string, bool) {
	if v == devVersion {
		return "", false
	}
	c, ok := canonicalVersion(v)
	return c, ok && c != ""
}

// canonicalVersion returns v with a leading "v", or "" for an empty v. ok is
// false when a non-empty v is not valid SemVer.
func canonicalVersion(v string) (string, bool) {
	if strings.TrimSpace(v) == "" {
		return "", true
	}
	n := normalizeVersion(v)
	if !semver.IsValid(n) {
		return "", false
	}
	return n, true
}

// LatestStable fetches one page of releases and returns the highest stable
// tag on it. It never follows rel="next". A page with no stable tag returns ""
// and no error. The caller bounds it through ctx.
func (c *Checker) LatestStable(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s%s?per_page=%d", strings.TrimRight(c.BaseURL, "/"), releasesPath, perPage)
	page, _, err := c.fetchPage(ctx, url)
	if err != nil {
		return "", err
	}
	return highestStable("", page), nil
}

// highestStable returns the higher of best and every stable tag in page, in
// canonical form.
func highestStable(best string, page []githubRelease) string {
	for _, r := range page {
		if tag, ok := stableTag(r); ok && (best == "" || semver.Compare(tag, best) > 0) {
			best = tag
		}
	}
	return best
}

// stableTag returns r's tag in canonical form when r counts as a stable
// release: valid SemVer, not a draft, not flagged prerelease, and with no
// SemVer prerelease suffix even when the flag was left unset. The live check
// and the cache both use it, so they agree on what an update is.
func stableTag(r githubRelease) (string, bool) {
	if r.Draft || r.Prerelease {
		return "", false
	}
	tag := normalizeVersion(r.TagName)
	if !semver.IsValid(tag) || semver.Prerelease(tag) != "" {
		return "", false
	}
	return tag, true
}
