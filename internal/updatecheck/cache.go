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
)

// CacheTTL is how long a stored check result stays fresh. The GUI and the CLI
// use the same value.
const CacheTTL = 24 * time.Hour

const (
	// cacheSchema is the only record schema Load accepts.
	cacheSchema = 1
	// shownSchema is the only shown-state schema LoadShown accepts.
	shownSchema = 1
	// cacheDirName is the per-user directory under xdg.CacheHome.
	cacheDirName = "pdfdebug"
	// cacheFileName is the shared check record.
	cacheFileName = "updatecheck.json"
	// shownFileName is the CLI notice state, kept next to the record.
	shownFileName = "updatenotice.json"
)

// Snapshot is the shared check record: what the release server said and when
// it was last asked. It holds no installed version and no derived "update
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
	if installedVersion == devVersion {
		return "", false
	}
	installed := normalizeVersion(installedVersion)
	target := normalizeVersion(s.LatestVersion)
	if !semver.IsValid(installed) || !semver.IsValid(target) {
		return "", false
	}
	if semver.Compare(target, installed) <= 0 {
		return "", false
	}
	return strings.TrimPrefix(target, "v"), true
}

// Shown is the CLI's once-per-version notice state: the target version last
// announced, when it was first shown, and whether the one-time re-arm has
// been spent.
type Shown struct {
	Schema       int       `json:"schema"`
	Version      string    `json:"version"`
	FirstShownAt time.Time `json:"first_shown_at"`
	Rearmed      bool      `json:"rearmed"`
}

// Cache reads and writes the shared check record and the notice state. Both
// files live in one directory; the mutex serialises reads and writes inside a
// process, and cross-process writes are last-writer-wins through an atomic
// rename.
type Cache struct {
	path string
	mu   sync.Mutex
}

// DefaultPath returns xdg.CacheHome/pdfdebug/updatecheck.json. It never
// touches the disk, and errors only when xdg.CacheHome is empty or relative.
func DefaultPath() (string, error) {
	base := xdg.CacheHome
	if base == "" || !filepath.IsAbs(base) {
		return "", fmt.Errorf("cache home %q is not an absolute path", base)
	}
	return filepath.Join(base, cacheDirName, cacheFileName), nil
}

// Open returns a Cache for the record at path, an absolute file path such as
// DefaultPath returns. The notice state lives next to it. Open does not touch
// the disk; the first Store creates the directory.
func Open(path string) (*Cache, error) {
	if path == "" || !filepath.IsAbs(path) {
		return nil, fmt.Errorf("cache path %q is not an absolute path", path)
	}
	return &Cache{path: path}, nil
}

// Load returns the stored record. A missing, unreadable, corrupt or truncated
// file, an unknown schema, or a latest version that is not SemVer all load as
// absent. It never returns an error and never writes output.
func (c *Cache) Load() (Snapshot, bool) {
	var s Snapshot
	if !c.readJSON(c.path, &s) || s.Schema != cacheSchema {
		return Snapshot{}, false
	}
	latest, ok := canonicalVersion(s.LatestVersion)
	if !ok {
		return Snapshot{}, false
	}
	s.LatestVersion = latest
	return s, true
}

// Store writes s atomically, setting the schema itself and writing
// LatestVersion in canonical form (leading "v") or empty. A non-empty
// LatestVersion that is not valid SemVer is refused and nothing is written.
// Callers treat any error as "no cache".
func (c *Cache) Store(s Snapshot) error {
	latest, ok := canonicalVersion(s.LatestVersion)
	if !ok {
		return fmt.Errorf("latest version %q is not valid SemVer", s.LatestVersion)
	}
	s.Schema = cacheSchema
	s.LatestVersion = latest
	return c.write(c.path, "updatecheck-*.tmp", s)
}

// LoadShown returns the stored notice state, with the same load-as-absent
// rules as Load.
func (c *Cache) LoadShown() (Shown, bool) {
	var s Shown
	if !c.readJSON(c.shownPath(), &s) || s.Schema != shownSchema {
		return Shown{}, false
	}
	v, ok := canonicalVersion(s.Version)
	if !ok || v == "" {
		return Shown{}, false
	}
	s.Version = v
	return s, true
}

// StoreShown writes the notice state atomically, with Version in canonical
// form. An empty or non-SemVer Version is refused.
func (c *Cache) StoreShown(s Shown) error {
	v, ok := canonicalVersion(s.Version)
	if !ok || v == "" {
		return fmt.Errorf("shown version %q is not valid SemVer", s.Version)
	}
	s.Schema = shownSchema
	s.Version = v
	return c.write(c.shownPath(), "updatenotice-*.tmp", s)
}

func (c *Cache) shownPath() string {
	return filepath.Join(filepath.Dir(c.path), shownFileName)
}

// write marshals v and replaces path through a temp file in the same
// directory. The temp file is removed on every failure path.
func (c *Cache) write(path, pattern string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	renamed = true
	return nil
}

// readJSON decodes the file at path into v, reporting false on any failure.
// It holds the mutex because on Windows an open reader makes a concurrent
// rename onto the same file fail. The mutex covers this process only; a
// reader in another process can still fail a rename here, which the caller
// sees as a failed store.
func (c *Cache) readJSON(path string, v any) bool {
	c.mu.Lock()
	data, err := os.ReadFile(path)
	c.mu.Unlock()
	if err != nil {
		return false
	}
	return json.Unmarshal(data, v) == nil
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

// LatestStable fetches one page of releases and returns the highest tag that
// is valid SemVer, not a draft, not flagged prerelease and carries no SemVer
// prerelease suffix. It never follows rel="next". A page with no qualifying
// tag returns "" and no error. The caller bounds it through ctx.
func (c *Checker) LatestStable(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s%s?per_page=%d", strings.TrimRight(c.BaseURL, "/"), releasesPath, perPage)
	page, _, err := c.fetchPage(ctx, url)
	if err != nil {
		return "", err
	}
	best := ""
	for _, r := range page {
		if r.Draft || r.Prerelease {
			continue
		}
		tag := normalizeVersion(r.TagName)
		if !semver.IsValid(tag) || semver.Prerelease(tag) != "" {
			continue
		}
		if best == "" || semver.Compare(tag, best) > 0 {
			best = tag
		}
	}
	return best, nil
}
