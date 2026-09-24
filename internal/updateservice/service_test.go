package updateservice

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"unidoc-pdf-debugger/internal/updatecheck"
)

// releasesServer serves tags as one stable page and counts requests.
func releasesServer(t *testing.T, tags ...string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body := []map[string]any{}
		for _, tag := range tags {
			body = append(body, map[string]any{"tag_name": tag, "assets": []any{}})
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func failingServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// testService builds a Service against srv with its cache in a temp dir.
func testService(t *testing.T, srv *httptest.Server, version string) (*Service, *updatecheck.Cache) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pdfdebug", "updatecheck.json")
	s := newService(nil, version, path)
	s.checker.BaseURL = srv.URL
	s.checker.Client = srv.Client()
	c, err := updatecheck.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return s, c
}

func TestExplicitCheckStoresTheNewerRelease(t *testing.T) {
	srv, _ := releasesServer(t, "v0.6.0", "v0.5.0")
	s, c := testService(t, srv, "0.5.0")
	res, err := s.CheckForUpdate(t.Context())
	if err != nil || !res.UpdateAvailable {
		t.Fatalf("CheckForUpdate = %+v, %v", res, err)
	}
	snap, ok := c.Load()
	if !ok || snap.LatestVersion != "v0.6.0" {
		t.Errorf("stored %+v, %v; want latest v0.6.0", snap, ok)
	}
}

func TestExplicitCheckWithNothingNewerStoresTheRunningVersion(t *testing.T) {
	srv, _ := releasesServer(t, "v0.5.0", "v0.4.0")
	s, c := testService(t, srv, "0.5.0")
	if _, err := s.CheckForUpdate(t.Context()); err != nil {
		t.Fatal(err)
	}
	snap, ok := c.Load()
	if !ok || snap.LatestVersion != "v0.5.0" {
		t.Errorf("stored %+v, %v; want the running version as v0.5.0", snap, ok)
	}
}

func TestPrereleaseBuildWithNothingNewerKeepsThePreviousLatest(t *testing.T) {
	srv, _ := releasesServer(t, "v0.5.0")
	s, c := testService(t, srv, "0.6.0-rc1")
	if err := c.Store(updatecheck.Snapshot{CheckedAt: time.Now().Add(-48 * time.Hour), LatestVersion: "v0.5.0"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CheckForUpdate(t.Context()); err != nil {
		t.Fatal(err)
	}
	snap, _ := c.Load()
	if snap.LatestVersion != "v0.5.0" {
		t.Errorf("latest = %q, want v0.5.0 kept for a prerelease build", snap.LatestVersion)
	}
	if !snap.Fresh(time.Now(), updatecheck.CacheTTL) {
		t.Error("checked_at was not advanced")
	}
}

func TestFailedCheckAdvancesCheckedAtAndKeepsLastKnownGood(t *testing.T) {
	srv, _ := failingServer(t)
	s, c := testService(t, srv, "0.4.0")
	seeded := time.Now().Add(-48 * time.Hour)
	if err := c.Store(updatecheck.Snapshot{CheckedAt: seeded, LatestVersion: "v0.5.0"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CheckForUpdateAtStartup(t.Context()); err == nil {
		t.Fatal("a failing server returned no error")
	}
	snap, _ := c.Load()
	if snap.LatestVersion != "v0.5.0" || !snap.CheckedAt.After(seeded) {
		t.Errorf("stored %+v; want v0.5.0 kept and checked_at advanced", snap)
	}
}

func TestStartupWithAFreshCurrentRecordMakesNoRequest(t *testing.T) {
	srv, hits := releasesServer(t, "v0.6.0")
	s, c := testService(t, srv, "0.5.0")
	if err := c.Store(updatecheck.Snapshot{CheckedAt: time.Now().Add(-time.Hour), LatestVersion: "v0.5.0"}); err != nil {
		t.Fatal(err)
	}
	res, err := s.CheckForUpdateAtStartup(t.Context())
	if err != nil || res.UpdateAvailable || res.InstalledVersion != "0.5.0" {
		t.Errorf("startup = %+v, %v", res, err)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
}

func TestStartupGoesLiveWhenTheFreshRecordNamesANewerVersion(t *testing.T) {
	srv, hits := releasesServer(t, "v0.5.0")
	s, c := testService(t, srv, "0.4.0")
	if err := c.Store(updatecheck.Snapshot{CheckedAt: time.Now().Add(-time.Hour), LatestVersion: "v0.5.0"}); err != nil {
		t.Fatal(err)
	}
	res, err := s.CheckForUpdateAtStartup(t.Context())
	if err != nil || !res.UpdateAvailable || len(res.Releases) != 1 {
		t.Errorf("startup = %+v, %v; want the live result with its release list", res, err)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
}

func TestExplicitCheckIgnoresAFreshRecord(t *testing.T) {
	srv, hits := releasesServer(t, "v0.5.0")
	s, c := testService(t, srv, "0.5.0")
	if err := c.Store(updatecheck.Snapshot{CheckedAt: time.Now(), LatestVersion: "v0.5.0"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CheckForUpdate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
}

func TestNilCacheLeavesBothChecksWorking(t *testing.T) {
	srv, hits := releasesServer(t, "v0.6.0")
	s := newService(nil, "0.5.0", "relative/updatecheck.json")
	if s.cache != nil {
		t.Fatal("a relative cache path produced a cache")
	}
	s.checker.BaseURL = srv.URL
	s.checker.Client = srv.Client()
	for name, check := range map[string]func() (updatecheck.Result, error){
		"startup":  func() (updatecheck.Result, error) { return s.CheckForUpdateAtStartup(t.Context()) },
		"explicit": func() (updatecheck.Result, error) { return s.CheckForUpdate(t.Context()) },
	} {
		if res, err := check(); err != nil || !res.UpdateAvailable {
			t.Errorf("%s with a nil cache = %+v, %v", name, res, err)
		}
	}
	if n := hits.Load(); n != 2 {
		t.Errorf("%d requests, want 2", n)
	}
}

func TestUnwritableCacheDirLeavesBothChecksWorking(t *testing.T) {
	srv, _ := releasesServer(t, "v0.6.0")
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := newService(nil, "0.5.0", filepath.Join(blocker, "pdfdebug", "updatecheck.json"))
	s.checker.BaseURL = srv.URL
	s.checker.Client = srv.Client()
	if res, err := s.CheckForUpdateAtStartup(t.Context()); err != nil || !res.UpdateAvailable {
		t.Errorf("startup = %+v, %v", res, err)
	}
	if res, err := s.CheckForUpdate(t.Context()); err != nil || !res.UpdateAvailable {
		t.Errorf("explicit = %+v, %v", res, err)
	}
}

func TestDevBuildStoresNothing(t *testing.T) {
	srv, hits := releasesServer(t, "v0.6.0")
	dir := t.TempDir()
	s := newService(nil, "dev", filepath.Join(dir, "pdfdebug", "updatecheck.json"))
	s.checker.BaseURL = srv.URL
	s.checker.Client = srv.Client()
	if _, err := s.CheckForUpdateAtStartup(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CheckForUpdate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if list, _ := os.ReadDir(dir); len(list) != 0 {
		t.Errorf("a dev build wrote %v", list)
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
}

func TestUnflaggedPrereleaseTagIsNotStoredAsLatest(t *testing.T) {
	cases := []struct {
		name string
		tags []string
		want string
	}{
		{"stable release behind the rc", []string{"v0.7.0-rc1", "v0.6.0"}, "v0.6.0"},
		{"only the rc is newer", []string{"v0.7.0-rc1"}, "v0.5.0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv, _ := releasesServer(t, c.tags...)
			s, cache := testService(t, srv, "0.5.0")
			if err := cache.Store(updatecheck.Snapshot{CheckedAt: time.Now().Add(-48 * time.Hour), LatestVersion: "v0.4.0"}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.CheckForUpdate(t.Context()); err != nil {
				t.Fatal(err)
			}
			snap, _ := cache.Load()
			if snap.LatestVersion != c.want {
				t.Errorf("latest = %q, want %s", snap.LatestVersion, c.want)
			}
		})
	}
}
