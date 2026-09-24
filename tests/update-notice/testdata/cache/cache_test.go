// Package harness exercises the shared update-check record from inside the main
// module. It is copied into a dot-prefixed directory and run by the acceptance
// suite; the go tool ignores it where it lives.
package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"unidoc-pdf-debugger/internal/updatecheck"
)

func recordPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "updatecheck-cli.json")
}

func openCache(t *testing.T, path string) *updatecheck.Cache {
	t.Helper()
	if filepath.Base(path) != "updatecheck-cli.json" {
		t.Fatalf("openCache wants the CLI record path, got %q", path)
	}
	c, err := updatecheck.Open(filepath.Dir(path), updatecheck.SurfaceCLI)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	return c
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func assertOnlyRecord(t *testing.T, dir string) {
	t.Helper()
	names := dirNames(t, dir)
	if len(names) != 1 || names[0] != "updatecheck-cli.json" {
		t.Errorf("expected only updatecheck-cli.json in %s, found %v", dir, names)
	}
}

func TestCacheDefaultDirFollowsXDGCacheHomeAndCreatesNothing(t *testing.T) {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		t.Fatal("harness must run with XDG_CACHE_HOME set to an empty temp dir")
	}
	got, err := updatecheck.DefaultDir()
	if err != nil {
		t.Fatalf("DefaultDir: %v", err)
	}
	want := filepath.Join(base, "pdfdebug")
	if got != want {
		t.Errorf("DefaultDir = %q, want %q", got, want)
	}
	c := openCache(t, filepath.Join(got, "updatecheck-cli.json"))
	if _, ok := c.Load(); ok {
		t.Error("Load on a never-written default path reported a record")
	}
	if names := dirNames(t, base); len(names) != 0 {
		t.Errorf("resolving, opening and loading the default path created %v under XDG_CACHE_HOME", names)
	}
}

func TestCacheOpenRejectsEmptyAndRelativePaths(t *testing.T) {
	for _, p := range []string{"", "pdfdebug", filepath.Join("cache", "pdfdebug")} {
		if c, err := updatecheck.Open(p, updatecheck.SurfaceCLI); err == nil {
			t.Errorf("Open(%q) = %v, nil; want an error", p, c)
		}
	}
}

func TestCacheOpenAndLoadTouchNoDisk(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "pdfdebug")
	c := openCache(t, filepath.Join(dir, "updatecheck-cli.json"))
	if _, ok := c.Load(); ok {
		t.Error("Load on a missing file reported a record")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("Open or Load created the cache directory (stat err = %v)", err)
	}
}

func TestCacheLoadTreatsBrokenRecordsAsAbsent(t *testing.T) {
	cases := map[string]string{
		"empty file":         "",
		"not json":           "this is not json",
		"truncated":          `{"schema":1,"checked_at":"2026-09-20T10:00:00Z","latest_ver`,
		"json array":         `[1,2,3]`,
		"schema two":         `{"schema":2,"checked_at":"2026-09-20T10:00:00Z","latest_version":"v0.5.0"}`,
		"schema zero":        `{"schema":0,"checked_at":"2026-09-20T10:00:00Z","latest_version":"v0.5.0"}`,
		"schema missing":     `{"checked_at":"2026-09-20T10:00:00Z","latest_version":"v0.5.0"}`,
		"schema as a string": `{"schema":"1","checked_at":"2026-09-20T10:00:00Z","latest_version":"v0.5.0"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := recordPath(t)
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if s, ok := openCache(t, path).Load(); ok {
				t.Errorf("Load reported a record for %s: %+v", name, s)
			}
		})
	}

	t.Run("path is a directory", func(t *testing.T) {
		path := recordPath(t)
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if _, ok := openCache(t, path).Load(); ok {
			t.Error("Load reported a record when the path is a directory")
		}
	})
}

func TestCacheLoadReadsTheOnDiskRecord(t *testing.T) {
	path := recordPath(t)
	body := `{"schema":1,"checked_at":"2026-09-20T10:00:00Z","latest_version":"v0.5.0"}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	s, ok := openCache(t, path).Load()
	if !ok {
		t.Fatal("Load rejected a valid schema-1 record")
	}
	want := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	if s.Schema != 1 || !s.CheckedAt.Equal(want) || s.LatestVersion != "v0.5.0" {
		t.Errorf("Load = %+v, want schema 1, checked_at %v, latest v0.5.0", s, want)
	}
}

func TestCacheStoreRoundTripLeavesOnlyTheRecord(t *testing.T) {
	path := recordPath(t)
	c := openCache(t, path)
	now := time.Now().UTC().Truncate(time.Second)
	if err := c.Store(updatecheck.Snapshot{CheckedAt: now, LatestVersion: "v0.5.0"}); err != nil {
		t.Fatalf("Store: %v", err)
	}
	s, ok := openCache(t, path).Load()
	if !ok {
		t.Fatal("Load after Store reported no record")
	}
	if s.Schema != 1 {
		t.Errorf("Store must set schema 1 itself, loaded %d", s.Schema)
	}
	if !s.CheckedAt.Equal(now) || s.LatestVersion != "v0.5.0" {
		t.Errorf("round trip = %+v, want checked_at %v latest v0.5.0", s, now)
	}
	assertOnlyRecord(t, filepath.Dir(path))
}

func TestCacheRecordHoldsOnlyServerFacts(t *testing.T) {
	path := recordPath(t)
	if err := openCache(t, path).Store(updatecheck.Snapshot{CheckedAt: time.Now(), LatestVersion: "v0.5.0"}); err != nil {
		t.Fatalf("Store: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("record is not a JSON object: %v\n%s", err, data)
	}
	var keys []string
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if strings.Join(keys, ",") != "checked_at,latest_version,schema,succeeded_at" {
		t.Errorf("record keys = %v, want exactly checked_at, latest_version, schema, succeeded_at", keys)
	}
	if raw["schema"] != float64(1) {
		t.Errorf("schema = %v, want 1", raw["schema"])
	}
}

func TestCacheStoreCanonicalisesLatestVersion(t *testing.T) {
	path := recordPath(t)
	c := openCache(t, path)
	for _, in := range []string{"0.5.0", "v0.5.0"} {
		if err := c.Store(updatecheck.Snapshot{CheckedAt: time.Now(), LatestVersion: in}); err != nil {
			t.Fatalf("Store(%q): %v", in, err)
		}
		s, ok := c.Load()
		if !ok || s.LatestVersion != "v0.5.0" {
			t.Errorf("Store(%q) loaded as %q (ok=%v), want v0.5.0", in, s.LatestVersion, ok)
		}
	}
	if err := c.Store(updatecheck.Snapshot{CheckedAt: time.Now(), LatestVersion: ""}); err != nil {
		t.Fatalf("Store with an empty latest version: %v", err)
	}
	if s, ok := c.Load(); !ok || s.LatestVersion != "" {
		t.Errorf("empty latest version loaded as %q (ok=%v), want an empty string", s.LatestVersion, ok)
	}
}

func TestCacheStoreRefusesNonSemverLatest(t *testing.T) {
	path := recordPath(t)
	c := openCache(t, path)
	if err := c.Store(updatecheck.Snapshot{CheckedAt: time.Now(), LatestVersion: "v0.5.0"}); err != nil {
		t.Fatalf("Store: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"banana", "latest", "\x1b[31mv9.9.9", "0.5.0.1"} {
		if err := c.Store(updatecheck.Snapshot{CheckedAt: time.Now(), LatestVersion: bad}); err == nil {
			t.Errorf("Store accepted non-SemVer latest version %q", bad)
		}
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("a refused Store rewrote the record:\nbefore %s\nafter  %s", before, after)
	}
	assertOnlyRecord(t, filepath.Dir(path))
}

func TestCacheStoreCreatesPrivateDirAndFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not apply on Windows")
	}
	dir := filepath.Join(t.TempDir(), "pdfdebug")
	path := filepath.Join(dir, "updatecheck-cli.json")
	if err := openCache(t, path).Store(updatecheck.Snapshot{CheckedAt: time.Now(), LatestVersion: "v0.5.0"}); err != nil {
		t.Fatalf("Store: %v", err)
	}
	di, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := di.Mode().Perm(); got != 0o700 {
		t.Errorf("cache dir mode = %o, want 700", got)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fi.Mode().Perm(); got != 0o600 {
		t.Errorf("record mode = %o, want 600", got)
	}
}

func TestCacheStoreIntoUnwritableDirFailsCleanly(t *testing.T) {
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

	c := openCache(t, filepath.Join(dir, "updatecheck-cli.json"))
	if err := c.Store(updatecheck.Snapshot{CheckedAt: time.Now(), LatestVersion: "v0.5.0"}); err == nil {
		t.Error("Store into a read-only directory reported success")
	}
	if names := dirNames(t, dir); len(names) != 0 {
		t.Errorf("a failed Store left %v behind", names)
	}
	if _, ok := c.Load(); ok {
		t.Error("Load after a failed Store reported a record")
	}
}

func TestCacheConcurrentStoresLeaveParseableRecord(t *testing.T) {
	path := recordPath(t)
	c := openCache(t, path)
	const n = 16
	valid := map[string]bool{}
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		v := fmt.Sprintf("v0.5.%d", i)
		valid[v] = true
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.Store(updatecheck.Snapshot{CheckedAt: time.Now(), LatestVersion: v})
		}()
	}
	wg.Wait()
	s, ok := openCache(t, path).Load()
	if !ok {
		t.Fatal("record is not loadable after concurrent stores")
	}
	if !valid[s.LatestVersion] {
		t.Errorf("record holds %q, which no writer stored", s.LatestVersion)
	}
	assertOnlyRecord(t, filepath.Dir(path))
}

func TestCacheTTLIsOneDay(t *testing.T) {
	if updatecheck.CacheTTL != 24*time.Hour {
		t.Errorf("CacheTTL = %v, want 24h", updatecheck.CacheTTL)
	}
}

func TestCacheFreshAroundTheTTLBoundary(t *testing.T) {
	base := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	ttl := updatecheck.CacheTTL
	s := updatecheck.Snapshot{Schema: 1, CheckedAt: base, LatestVersion: "v0.5.0"}
	cases := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"at the check", base, true},
		{"one second before expiry", base.Add(ttl - time.Second), true},
		{"exactly at expiry", base.Add(ttl), false},
		{"one second after expiry", base.Add(ttl + time.Second), false},
	}
	for _, c := range cases {
		if got := s.Fresh(c.now, ttl); got != c.want {
			t.Errorf("%s: Fresh = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestCacheFreshRejectsFutureCheckAndZeroSnapshot(t *testing.T) {
	now := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	future := updatecheck.Snapshot{Schema: 1, CheckedAt: now.Add(time.Hour), LatestVersion: "v0.5.0"}
	if future.Fresh(now, updatecheck.CacheTTL) {
		t.Error("a checked_at in the future must be stale, not trusted")
	}
	if (updatecheck.Snapshot{}).Fresh(now, updatecheck.CacheTTL) {
		t.Error("the zero Snapshot (an absent Load) must not be fresh")
	}
}

func TestCacheNoticeComparesAgainstTheCallersVersion(t *testing.T) {
	s := updatecheck.Snapshot{Schema: 1, CheckedAt: time.Now(), LatestVersion: "v0.5.0"}
	cases := []struct {
		installed  string
		wantLatest string
		wantAvail  bool
	}{
		{"0.4.0", "0.5.0", true},
		{"v0.4.0", "0.5.0", true},
		{"0.4.9", "0.5.0", true},
		{"0.5.0", "", false},
		{"v0.5.0", "", false},
		{"0.6.0", "", false},
		{"dev", "", false},
		{"abc", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		latest, avail := s.Notice(c.installed)
		if avail != c.wantAvail {
			t.Errorf("Notice(%q) available = %v, want %v", c.installed, avail, c.wantAvail)
		}
		if c.wantAvail && latest != c.wantLatest {
			t.Errorf("Notice(%q) latest = %q, want %q (no leading v)", c.installed, latest, c.wantLatest)
		}
	}
}

func TestCacheNoticeWithoutALatestVersion(t *testing.T) {
	if _, avail := (updatecheck.Snapshot{Schema: 1, CheckedAt: time.Now()}).Notice("0.4.0"); avail {
		t.Error("an empty latest version must never report an update")
	}
	if _, avail := (updatecheck.Snapshot{}).Notice("0.4.0"); avail {
		t.Error("the zero Snapshot must never report an update")
	}
}

func TestCacheNoticeNeverReturnsNonSemverLatest(t *testing.T) {
	for _, bad := range []string{"\x1b]0;pwned\x07", "v9.9.9\x1b[2J", "banana"} {
		s := updatecheck.Snapshot{Schema: 1, CheckedAt: time.Now(), LatestVersion: bad}
		latest, avail := s.Notice("0.4.0")
		if avail {
			t.Errorf("Notice reported an update for non-SemVer latest %q", bad)
		}
		if strings.ContainsAny(latest, "\x1b\x07") {
			t.Errorf("Notice returned control bytes from latest %q: %q", bad, latest)
		}
	}
}

func TestCacheOneRecordServesConsumersAtDifferentVersions(t *testing.T) {
	path := recordPath(t)
	if err := openCache(t, path).Store(updatecheck.Snapshot{CheckedAt: time.Now(), LatestVersion: "0.5.0"}); err != nil {
		t.Fatalf("Store: %v", err)
	}

	s, ok := openCache(t, path).Load()
	if !ok {
		t.Fatal("second consumer could not load the record")
	}
	if latest, avail := s.Notice("0.4.0"); !avail || latest != "0.5.0" {
		t.Errorf("consumer at 0.4.0: Notice = (%q, %v), want (0.5.0, true)", latest, avail)
	}
	if latest, avail := s.Notice("0.5.0"); avail {
		t.Errorf("consumer at 0.5.0: Notice = (%q, true), want no update", latest)
	}
}
