// Package harness exercises the GUI update service's use of the shared cache
// through its public constructor. The acceptance suite runs each test in its
// own subprocess with XDG_CACHE_HOME set to a fresh temp dir and HTTP(S)_PROXY
// set to a local tripwire that refuses and counts requests, so a live check
// fails fast without reaching the network and the suite can count attempts.
package harness

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"unidoc-pdf-debugger/internal/updatecheck"
	"unidoc-pdf-debugger/internal/updateservice"
)

func cacheBase(t *testing.T) string {
	t.Helper()
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		t.Fatal("harness must run with XDG_CACHE_HOME set")
	}
	return base
}

func recordFile(t *testing.T) string {
	return filepath.Join(cacheBase(t), "pdfdebug", "updatecheck-app.json")
}

func seed(t *testing.T, checkedAt time.Time, latest string) {
	t.Helper()
	path := recordFile(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{
		"schema":         1,
		"checked_at":     checkedAt.UTC().Format(time.RFC3339Nano),
		"succeeded_at":   checkedAt.UTC().Format(time.RFC3339Nano),
		"latest_version": latest,
	})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func load(t *testing.T) (updatecheck.Snapshot, bool) {
	t.Helper()
	c, err := updatecheck.Open(filepath.Dir(recordFile(t)), updatecheck.SurfaceApp)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return c.Load()
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	return c
}

// A fresh record that says nothing is newer answers the startup check alone.
func TestServiceStartupFreshAndCurrentMakesNoRequest(t *testing.T) {
	seed(t, time.Now().Add(-time.Hour), "v0.5.0")
	for _, v := range []string{"0.5.0", "0.6.0"} {
		s := updateservice.NewUpdateService(nil, v)
		res, err := s.CheckForUpdateAtStartup(ctx(t))
		if err != nil {
			t.Errorf("version %s: startup check returned %v; a fresh current record needs no request", v, err)
		}
		if res.UpdateAvailable || res.InstalledVersion != v {
			t.Errorf("version %s: startup result = %+v, want no update and InstalledVersion %s", v, res, v)
		}
	}
}

// A fresh record that says something is newer cannot render the changelog, so
// startup goes live. The suite asserts the tripwire saw the request.
func TestServiceStartupFreshButNewerGoesLive(t *testing.T) {
	seed(t, time.Now().Add(-time.Hour), "v0.5.0")
	s := updateservice.NewUpdateService(nil, "0.4.0")
	_, _ = s.CheckForUpdateAtStartup(ctx(t))
}

// A stale record sends startup live; the failed check advances checked_at and
// keeps the last-known-good latest version.
func TestServiceStartupStaleGoesLiveAndKeepsLastKnownGood(t *testing.T) {
	seeded := time.Now().Add(-48 * time.Hour)
	seed(t, seeded, "v0.5.0")
	s := updateservice.NewUpdateService(nil, "0.4.0")
	res, err := s.CheckForUpdateAtStartup(ctx(t))
	if err == nil {
		t.Error("the live check went through a refusing proxy and must report an error")
	}
	if res.UpdateAvailable {
		t.Errorf("a failed check must not report an update: %+v", res)
	}
	snap, ok := load(t)
	if !ok {
		t.Fatal("record is gone after a failed startup check")
	}
	if snap.LatestVersion != "v0.5.0" {
		t.Errorf("latest_version = %q after a failed check, want v0.5.0 kept", snap.LatestVersion)
	}
	if !snap.CheckedAt.After(seeded.Add(time.Hour)) {
		t.Errorf("checked_at = %v, want it advanced past the seeded %v", snap.CheckedAt, seeded)
	}
}

// The explicit check ignores a fresh record and goes live, then records the attempt.
func TestServiceExplicitCheckStaysLiveAndRecordsTheAttempt(t *testing.T) {
	seeded := time.Now().Add(-time.Hour)
	seed(t, seeded, "v0.5.0")
	s := updateservice.NewUpdateService(nil, "0.5.0")
	if _, err := s.CheckForUpdate(ctx(t)); err == nil {
		t.Error("the explicit check went through a refusing proxy and must report an error")
	}
	snap, ok := load(t)
	if !ok {
		t.Fatal("record is gone after a failed explicit check")
	}
	if snap.LatestVersion != "v0.5.0" {
		t.Errorf("latest_version = %q after a failed check, want v0.5.0 kept", snap.LatestVersion)
	}
	if !snap.CheckedAt.After(seeded.Add(30 * time.Minute)) {
		t.Errorf("checked_at = %v, want it advanced past the seeded %v", snap.CheckedAt, seeded)
	}
}

// A dev build makes no request and writes no cache.
func TestServiceDevBuildTouchesNothing(t *testing.T) {
	s := updateservice.NewUpdateService(nil, "dev")
	if res, err := s.CheckForUpdateAtStartup(ctx(t)); err != nil || res.UpdateAvailable {
		t.Errorf("dev startup check = %+v, %v; want no update, no error", res, err)
	}
	if res, err := s.CheckForUpdate(ctx(t)); err != nil || res.UpdateAvailable {
		t.Errorf("dev explicit check = %+v, %v; want no update, no error", res, err)
	}
	entries, err := os.ReadDir(cacheBase(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("a dev build wrote under the cache base: %v", entries)
	}
}

// A cache base that cannot hold a directory leaves both checks working.
func TestServiceUnusableCacheLeavesChecksWorking(t *testing.T) {
	s := updateservice.NewUpdateService(nil, "0.5.0")
	res, _ := s.CheckForUpdateAtStartup(ctx(t))
	if res.InstalledVersion != "0.5.0" {
		t.Errorf("startup check with an unusable cache = %+v, want InstalledVersion 0.5.0", res)
	}
	res, _ = s.CheckForUpdate(ctx(t))
	if res.InstalledVersion != "0.5.0" {
		t.Errorf("explicit check with an unusable cache = %+v, want InstalledVersion 0.5.0", res)
	}
}
