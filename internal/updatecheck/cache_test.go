package updatecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adrg/xdg"
)

func tempCache(t *testing.T) (*Cache, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pdfdebug", "updatecheck.json")
	c, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return c, path
}

func writeRaw(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func entries(t *testing.T, dir string) []string {
	t.Helper()
	list, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var names []string
	for _, e := range list {
		names = append(names, e.Name())
	}
	return names
}

// DefaultPath reads the package variable xdg.CacheHome, so these cases
// mutate it and must not run in parallel.
func TestDefaultPathJoinsCacheHomeAndRejectsUnusableBases(t *testing.T) {
	orig := xdg.CacheHome
	t.Cleanup(func() { xdg.CacheHome = orig })

	base := t.TempDir()
	xdg.CacheHome = base
	got, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if want := filepath.Join(base, "pdfdebug", "updatecheck.json"); got != want {
		t.Errorf("DefaultPath = %q, want %q", got, want)
	}
	if names := entries(t, base); len(names) != 0 {
		t.Errorf("DefaultPath created %v", names)
	}

	for _, bad := range []string{"", "relative/cache"} {
		xdg.CacheHome = bad
		if p, err := DefaultPath(); err == nil {
			t.Errorf("DefaultPath with CacheHome %q = %q, want an error", bad, p)
		}
	}
}

func TestOpenRejectsEmptyAndRelativePathsAndTouchesNothing(t *testing.T) {
	for _, p := range []string{"", "updatecheck.json"} {
		if _, err := Open(p); err == nil {
			t.Errorf("Open(%q) succeeded, want an error", p)
		}
	}
	c, path := tempCache(t)
	if _, ok := c.Load(); ok {
		t.Error("Load of a missing file reported a record")
	}
	if _, ok := c.LoadShown(); ok {
		t.Error("LoadShown of a missing file reported a state")
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Errorf("Open or Load created the directory (stat err %v)", err)
	}
}

func TestLoadTreatsBrokenRecordsAsAbsent(t *testing.T) {
	for name, body := range map[string]string{
		"truncated":         `{"schema":1,"checked_at":"2026-09-20T10:00:00Z","lat`,
		"unknown schema":    `{"schema":2,"checked_at":"2026-09-20T10:00:00Z","latest_version":"v0.5.0"}`,
		"no schema":         `{"checked_at":"2026-09-20T10:00:00Z","latest_version":"v0.5.0"}`,
		"non-semver latest": `{"schema":1,"checked_at":"2026-09-20T10:00:00Z","latest_version":"banana"}`,
	} {
		t.Run(name, func(t *testing.T) {
			c, path := tempCache(t)
			writeRaw(t, path, body)
			if s, ok := c.Load(); ok {
				t.Errorf("Load = %+v, want absent", s)
			}
		})
	}
}

func TestLoadCanonicalisesAHandWrittenLatestVersion(t *testing.T) {
	c, path := tempCache(t)
	writeRaw(t, path, `{"schema":1,"checked_at":"2026-09-20T10:00:00Z","latest_version":"0.5.0"}`)
	s, ok := c.Load()
	if !ok || s.LatestVersion != "v0.5.0" {
		t.Errorf("Load = %+v, %v; want latest v0.5.0", s, ok)
	}
}

func TestStoreCanonicalisesAndRoundTrips(t *testing.T) {
	c, path := tempCache(t)
	now := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	if err := c.Store(Snapshot{Schema: 7, CheckedAt: now, LatestVersion: "0.5.0"}); err != nil {
		t.Fatalf("Store: %v", err)
	}
	s, ok := c.Load()
	if !ok || s.Schema != 1 || !s.CheckedAt.Equal(now) || s.LatestVersion != "v0.5.0" {
		t.Errorf("Load = %+v, %v; want schema 1, %v, v0.5.0", s, ok, now)
	}
	if names := entries(t, filepath.Dir(path)); len(names) != 1 || names[0] != "updatecheck.json" {
		t.Errorf("directory holds %v, want only updatecheck.json", names)
	}
}

func TestStoreRefusesNonSemverLatestAndWritesNothing(t *testing.T) {
	c, path := tempCache(t)
	if err := c.Store(Snapshot{CheckedAt: time.Now(), LatestVersion: "not-a-version"}); err == nil {
		t.Fatal("Store accepted a non-SemVer latest version")
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Errorf("a refused Store created the directory (stat err %v)", err)
	}
}

func TestStoreIntoUnwritableDirFailsAndLeavesNothing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not apply on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not bind root")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	c, err := Open(filepath.Join(dir, "updatecheck.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Store(Snapshot{CheckedAt: time.Now(), LatestVersion: "v0.5.0"}); err == nil {
		t.Error("Store into a read-only directory succeeded")
	}
	if err := c.StoreShown(Shown{Version: "v0.5.0", FirstShownAt: time.Now()}); err == nil {
		t.Error("StoreShown into a read-only directory succeeded")
	}
	if names := entries(t, dir); len(names) != 0 {
		t.Errorf("failed stores left %v", names)
	}
}

func TestConcurrentStoresLeaveAParseableRecord(t *testing.T) {
	c, path := tempCache(t)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.Store(Snapshot{CheckedAt: time.Now(), LatestVersion: fmt.Sprintf("v0.5.%d", i)})
		}()
	}
	wg.Wait()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("record does not parse: %v\n%s", err, data)
	}
	if names := entries(t, filepath.Dir(path)); len(names) != 1 {
		t.Errorf("directory holds %v, want only the record", names)
	}
}

func TestStoresSucceedWhileTheSameCacheIsRead(t *testing.T) {
	c, _ := tempCache(t)
	if err := c.Store(Snapshot{CheckedAt: time.Now(), LatestVersion: "v0.5.0"}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var failed atomic.Int32
	for i := range 20 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := c.Store(Snapshot{CheckedAt: time.Now(), LatestVersion: fmt.Sprintf("v0.5.%d", i)}); err != nil {
				failed.Add(1)
			}
		}()
		go func() {
			defer wg.Done()
			c.Load()
		}()
	}
	wg.Wait()
	if n := failed.Load(); n != 0 {
		t.Errorf("%d stores failed while another goroutine read the record", n)
	}
}

func TestFreshEitherSideOfTheTTL(t *testing.T) {
	at := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	s := Snapshot{Schema: 1, CheckedAt: at}
	if !s.Fresh(at.Add(CacheTTL-time.Nanosecond), CacheTTL) {
		t.Error("stale just inside the TTL")
	}
	if s.Fresh(at.Add(CacheTTL), CacheTTL) {
		t.Error("fresh at the TTL")
	}
	if s.Fresh(at.Add(-time.Minute), CacheTTL) {
		t.Error("a checked_at in the future counted as fresh")
	}
	if (Snapshot{}).Fresh(at, CacheTTL) {
		t.Error("the zero Snapshot counted as fresh")
	}
}

func TestNoticeAgainstInstalledVersions(t *testing.T) {
	s := Snapshot{Schema: 1, CheckedAt: time.Now(), LatestVersion: "v0.5.0"}
	for installed, want := range map[string]bool{
		"0.4.0": true, "v0.4.9": true, "0.5.0": false, "0.6.0": false, "dev": false, "junk": false,
	} {
		latest, ok := s.Notice(installed)
		if ok != want {
			t.Errorf("Notice(%q) = %v, want %v", installed, ok, want)
		}
		if ok && latest != "0.5.0" {
			t.Errorf("Notice(%q) latest = %q, want 0.5.0", installed, latest)
		}
	}
	if _, ok := (Snapshot{Schema: 1}).Notice("0.4.0"); ok {
		t.Error("an empty latest version reported an update")
	}
}

func TestShownStateRoundTripsInCanonicalForm(t *testing.T) {
	c, path := tempCache(t)
	now := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	if err := c.StoreShown(Shown{Version: "0.5.0", FirstShownAt: now, Rearmed: true}); err != nil {
		t.Fatalf("StoreShown: %v", err)
	}
	s, ok := c.LoadShown()
	if !ok || s.Version != "v0.5.0" || !s.FirstShownAt.Equal(now) || !s.Rearmed {
		t.Errorf("LoadShown = %+v, %v", s, ok)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "updatenotice.json")); err != nil {
		t.Errorf("shown state is not next to the record: %v", err)
	}
	if err := c.StoreShown(Shown{Version: ""}); err == nil {
		t.Error("StoreShown accepted an empty version")
	}

	writeRaw(t, filepath.Join(filepath.Dir(path), "updatenotice.json"), `{"schema":9,"version":"v0.5.0"}`)
	if _, ok := c.LoadShown(); ok {
		t.Error("an unknown shown-state schema loaded")
	}
}

func releasePage(t *testing.T, hits *atomic.Int32, body []githubRelease) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Link", "<"+srv.URL+releasesPath+"?page=2>; rel=\"next\"")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLatestStableReadsOnePageAndSkipsUnstableTags(t *testing.T) {
	var hits atomic.Int32
	srv := releasePage(t, &hits, []githubRelease{
		{TagName: "v0.5.0"},
		{TagName: "v0.9.0", Draft: true},
		{TagName: "v0.8.0", Prerelease: true},
		{TagName: "v0.7.0-rc1"},
		{TagName: "garbage"},
		{TagName: "0.6.1"},
	})
	got, err := testChecker(srv).LatestStable(t.Context())
	if err != nil {
		t.Fatalf("LatestStable: %v", err)
	}
	if got != "v0.6.1" {
		t.Errorf("LatestStable = %q, want v0.6.1", got)
	}
	if n := hits.Load(); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
}

func TestLatestStableEmptyPageAndSlowServer(t *testing.T) {
	var hits atomic.Int32
	srv := releasePage(t, &hits, []githubRelease{})
	if got, err := testChecker(srv).LatestStable(t.Context()); err != nil || got != "" {
		t.Errorf("empty page = %q, %v; want empty and no error", got, err)
	}

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(slow.Close)
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := testChecker(slow).LatestStable(ctx); err == nil {
		t.Error("a server slower than the deadline returned no error")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("LatestStable took %v past a 200ms deadline", d)
	}
}

func TestLoadShownTreatsBadVersionsAsAbsent(t *testing.T) {
	for name, body := range map[string]string{
		"empty version": `{"schema":1,"version":"","first_shown_at":"2026-09-20T10:00:00Z"}`,
		"blank version": `{"schema":1,"version":"  ","first_shown_at":"2026-09-20T10:00:00Z"}`,
		"no version":    `{"schema":1,"first_shown_at":"2026-09-20T10:00:00Z"}`,
		"non-semver":    `{"schema":1,"version":"banana","first_shown_at":"2026-09-20T10:00:00Z"}`,
		"truncated":     `{"schema":1,"version":"v0.5`,
		"no schema":     `{"version":"v0.5.0","first_shown_at":"2026-09-20T10:00:00Z"}`,
	} {
		t.Run(name, func(t *testing.T) {
			c, path := tempCache(t)
			writeRaw(t, filepath.Join(filepath.Dir(path), "updatenotice.json"), body)
			if s, ok := c.LoadShown(); ok {
				t.Errorf("LoadShown = %+v, want absent", s)
			}
		})
	}
}

func TestStoreOntoADirectoryFailsAndRemovesTheTempFile(t *testing.T) {
	c, path := tempCache(t)
	if err := os.MkdirAll(filepath.Join(path, "occupied"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := c.Store(Snapshot{CheckedAt: time.Now(), LatestVersion: "v0.5.0"}); err == nil {
		t.Fatal("Store replaced a non-empty directory")
	}
	if names := entries(t, filepath.Dir(path)); len(names) != 1 || names[0] != "updatecheck.json" {
		t.Errorf("a failed rename left %v, want only the blocking directory", names)
	}
	if _, ok := c.Load(); ok {
		t.Error("Load read a record through a directory")
	}
}
