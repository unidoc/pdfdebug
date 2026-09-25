package updatecheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adrg/xdg"
)

func tempCache(t *testing.T) (*Cache, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "pdfdebug")
	c, err := Open(dir, SurfaceCLI)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return c, filepath.Join(dir, "updatecheck-cli.json")
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

// assertNoTempFiles fails when dir still holds a write's temp file.
func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	for _, name := range entries(t, dir) {
		if strings.HasSuffix(name, ".tmp") {
			t.Errorf("temp file %s left in %s", name, dir)
		}
	}
}

// DefaultDir reads the package variable xdg.CacheHome, so these cases
// mutate it and must not run in parallel.
func TestDefaultDirJoinsCacheHomeAndRejectsUnusableBases(t *testing.T) {
	orig := xdg.CacheHome
	t.Cleanup(func() { xdg.CacheHome = orig })

	base := t.TempDir()
	xdg.CacheHome = base
	got, err := DefaultDir()
	if err != nil {
		t.Fatalf("DefaultDir: %v", err)
	}
	if want := filepath.Join(base, "pdfdebug"); got != want {
		t.Errorf("DefaultDir = %q, want %q", got, want)
	}
	if names := entries(t, base); len(names) != 0 {
		t.Errorf("DefaultDir created %v", names)
	}

	for _, bad := range []string{"", "relative/cache"} {
		xdg.CacheHome = bad
		if p, err := DefaultDir(); err == nil {
			t.Errorf("DefaultDir with CacheHome %q = %q, want an error", bad, p)
		}
	}
}

func TestOpenRejectsEmptyAndRelativePathsAndTouchesNothing(t *testing.T) {
	for _, p := range []string{"", "pdfdebug"} {
		if _, err := Open(p, SurfaceCLI); err == nil {
			t.Errorf("Open(%q) succeeded, want an error", p)
		}
	}
	if _, err := Open(t.TempDir(), Surface("gui")); err == nil {
		t.Error("Open with an unknown surface succeeded")
	}
	c, path := tempCache(t)
	if _, ok := c.Load(); ok {
		t.Error("Load of a missing file reported a record")
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
	if names := entries(t, filepath.Dir(path)); len(names) != 1 || names[0] != "updatecheck-cli.json" {
		t.Errorf("directory holds %v, want only updatecheck-cli.json", names)
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
	c, err := Open(dir, SurfaceCLI)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Store(Snapshot{CheckedAt: time.Now(), LatestVersion: "v0.5.0"}); err == nil {
		t.Error("Store into a read-only directory succeeded")
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

func TestStoreRetriesARenameHeldOpenByAnotherReader(t *testing.T) {
	c, path := tempCache(t)
	c.attempts = 5
	calls := 0
	c.rename = func(oldpath, newpath string) error {
		calls++
		if calls < 3 {
			return errors.New("sharing violation")
		}
		return os.Rename(oldpath, newpath)
	}
	if err := c.Store(Snapshot{CheckedAt: time.Now(), LatestVersion: "v0.5.0"}); err != nil {
		t.Fatalf("Store after two failed renames: %v", err)
	}
	if calls != 3 {
		t.Errorf("rename called %d times, want 3", calls)
	}
	if s, ok := c.Load(); !ok || s.LatestVersion != "v0.5.0" {
		t.Errorf("Load = %+v, %v; want the stored record", s, ok)
	}
	assertNoTempFiles(t, filepath.Dir(path))
}

func TestStoreGivesUpAfterTheLastRenameAttempt(t *testing.T) {
	c, path := tempCache(t)
	c.attempts = 3
	calls := 0
	c.rename = func(string, string) error {
		calls++
		return errors.New("sharing violation")
	}
	if err := c.Store(Snapshot{CheckedAt: time.Now(), LatestVersion: "v0.5.0"}); err == nil {
		t.Fatal("Store reported success with every rename failing")
	}
	if calls != 3 {
		t.Errorf("rename called %d times, want 3", calls)
	}
	assertNoTempFiles(t, filepath.Dir(path))
}

func TestRenameIsRetriedOnlyOnWindows(t *testing.T) {
	want := 1
	if runtime.GOOS == "windows" {
		want = 5
	}
	if renameAttempts != want {
		t.Errorf("renameAttempts = %d on %s, want %d", renameAttempts, runtime.GOOS, want)
	}
}

func TestEachSurfaceWritesOnlyItsOwnRecordAndReadsThePeer(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pdfdebug")
	app, err := Open(dir, SurfaceApp)
	if err != nil {
		t.Fatal(err)
	}
	cli, err := Open(dir, SurfaceCLI)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cli.LoadPeer(); ok {
		t.Error("LoadPeer reported a record with the desktop app never having checked")
	}
	now := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	if _, err := app.RecordSuccess(now, "v0.6.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := cli.RecordSuccess(now, "v0.5.0"); err != nil {
		t.Fatal(err)
	}
	if names := entries(t, dir); strings.Join(names, ",") != "updatecheck-app.json,updatecheck-cli.json" {
		t.Errorf("directory holds %v, want one record per surface", names)
	}
	for name, got := range map[string]func() (Snapshot, bool){
		"app Load": app.Load, "cli LoadPeer": cli.LoadPeer,
	} {
		if s, ok := got(); !ok || s.LatestVersion != "v0.6.0" {
			t.Errorf("%s = %+v, %v; want the app's v0.6.0", name, s, ok)
		}
	}
	for name, got := range map[string]func() (Snapshot, bool){
		"cli Load": cli.Load, "app LoadPeer": app.LoadPeer,
	} {
		if s, ok := got(); !ok || s.LatestVersion != "v0.5.0" {
			t.Errorf("%s = %+v, %v; want the CLI's v0.5.0", name, s, ok)
		}
	}
}

func TestNewerPicksTheHigherLatestVersion(t *testing.T) {
	a := Snapshot{LatestVersion: "v0.5.0"}
	b := Snapshot{LatestVersion: "v0.6.0"}
	none := Snapshot{}
	bad := Snapshot{LatestVersion: "garbage"}
	cases := []struct {
		name       string
		x, y, want Snapshot
	}{
		{"second higher", a, b, b},
		{"first higher", b, a, b},
		{"tie keeps the first", a, Snapshot{LatestVersion: "0.5.0", Schema: 9}, a},
		{"empty second", a, none, a},
		{"empty first", none, a, a},
		{"invalid second", a, bad, a},
		{"both empty", none, none, none},
	}
	for _, c := range cases {
		if got := Newer(c.x, c.y); got != c.want {
			t.Errorf("%s: Newer = %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestConcurrentAttemptsNeverDropASuccess(t *testing.T) {
	c, _ := tempCache(t)
	for i := 0; i < 50; i++ {
		now := time.Date(2026, 9, 20, 10, 0, i, 0, time.UTC)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = c.RecordSuccess(now, "v0.6.0") }()
		go func() { defer wg.Done(); _, _ = c.RecordAttempt(now) }()
		wg.Wait()
		if s, _ := c.Load(); !s.SucceededAt.Equal(now) || s.LatestVersion != "v0.6.0" {
			t.Fatalf("iteration %d: %+v; a concurrent attempt dropped the success", i, s)
		}
	}
}

func TestConfirmedFollowsTheLastSuccessNotTheLastAttempt(t *testing.T) {
	at := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	failed := Snapshot{Schema: 1, CheckedAt: at, SucceededAt: at.Add(-48 * time.Hour)}
	if !failed.Fresh(at.Add(time.Hour), CacheTTL) {
		t.Error("a recent attempt did not count as fresh")
	}
	if failed.Confirmed(at.Add(time.Hour), CacheTTL) {
		t.Error("a recent failed attempt counted as confirmed")
	}
	ok := Snapshot{Schema: 1, CheckedAt: at, SucceededAt: at}
	if !ok.Confirmed(at.Add(CacheTTL-time.Nanosecond), CacheTTL) {
		t.Error("unconfirmed just inside the TTL")
	}
	if ok.Confirmed(at.Add(CacheTTL), CacheTTL) {
		t.Error("confirmed at the TTL")
	}
	if ok.Confirmed(at.Add(-time.Minute), CacheTTL) {
		t.Error("a succeeded_at in the future counted as confirmed")
	}
}

func TestSucceededAtRoundTrips(t *testing.T) {
	c, _ := tempCache(t)
	at := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	if err := c.Store(Snapshot{CheckedAt: at.Add(time.Hour), SucceededAt: at, LatestVersion: "0.5.0"}); err != nil {
		t.Fatal(err)
	}
	s, ok := c.Load()
	if !ok || !s.SucceededAt.Equal(at) || !s.CheckedAt.Equal(at.Add(time.Hour)) {
		t.Errorf("Load = %+v, %v; want both timestamps kept apart", s, ok)
	}
}

func TestCheckableVersion(t *testing.T) {
	cases := map[string]string{"0.5.0": "v0.5.0", "v0.5.0": "v0.5.0", " 0.5.0 ": "v0.5.0", "0.6.0-rc1": "v0.6.0-rc1"}
	for in, want := range cases {
		if got, ok := CheckableVersion(in); !ok || got != want {
			t.Errorf("CheckableVersion(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"dev", "", "abc", "1.2.3.4"} {
		if got, ok := CheckableVersion(in); ok {
			t.Errorf("CheckableVersion(%q) = %q, true; want not checkable", in, got)
		}
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

func TestCheckReportsTheLatestStableTagItSaw(t *testing.T) {
	page := []githubRelease{
		{TagName: "v0.5.0"},
		{TagName: "v0.9.0", Draft: true},
		{TagName: "v0.8.0", Prerelease: true},
		{TagName: "v0.7.0-rc1"},
		{TagName: "0.6.1"},
	}
	cases := []struct {
		installed, want string
	}{
		{"0.5.0", "v0.6.1"},
		{"0.6.1", "v0.6.1"},
		{"0.7.0", "v0.6.1"},
	}
	for _, c := range cases {
		var hits atomic.Int32
		srv := releasePage(t, &hits, page)
		res, err := testChecker(srv).Check(t.Context(), c.installed)
		if err != nil {
			t.Fatalf("Check(%s): %v", c.installed, err)
		}
		if res.LatestStable != c.want {
			t.Errorf("Check(%s).LatestStable = %q, want %s", c.installed, res.LatestStable, c.want)
		}
	}
}

func TestLatestStableIsNotSentToTheFrontend(t *testing.T) {
	data, err := json.Marshal(Result{LatestStable: "v0.6.1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "v0.6.1") {
		t.Errorf("Result JSON carries LatestStable: %s", data)
	}
}

func TestRecordAttemptAdvancesOnlyCheckedAt(t *testing.T) {
	c, _ := tempCache(t)
	at := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	if err := c.Store(Snapshot{CheckedAt: at, SucceededAt: at, LatestVersion: "v0.5.0"}); err != nil {
		t.Fatal(err)
	}
	later := at.Add(time.Hour)
	got, err := c.RecordAttempt(later)
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := c.Load()
	for name, s := range map[string]Snapshot{"returned": got, "stored": stored} {
		if !s.CheckedAt.Equal(later) || !s.SucceededAt.Equal(at) || s.LatestVersion != "v0.5.0" {
			t.Errorf("%s = %+v; want checked_at %v, succeeded_at and latest kept", name, s, later)
		}
	}
}

func TestRecordSuccessAdvancesBothAndKeepsLatestOnAnEmptyTag(t *testing.T) {
	c, _ := tempCache(t)
	at := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	if err := c.Store(Snapshot{CheckedAt: at, SucceededAt: at, LatestVersion: "v0.5.0"}); err != nil {
		t.Fatal(err)
	}
	later := at.Add(time.Hour)
	for _, step := range []struct{ tag, want string }{{"", "v0.5.0"}, {"0.6.0", "v0.6.0"}} {
		tag, want := step.tag, step.want
		if _, err := c.RecordSuccess(later, tag); err != nil {
			t.Fatal(err)
		}
		s, _ := c.Load()
		if !s.CheckedAt.Equal(later) || !s.SucceededAt.Equal(later) || s.LatestVersion != want {
			t.Errorf("after RecordSuccess(%q): %+v; want both at %v, latest %s", tag, s, later, want)
		}
	}
}

func TestRecordSuccessReturnsTheRecordWhenTheWriteFails(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Open(filepath.Join(blocker, "pdfdebug"), SurfaceCLI)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	s, err := c.RecordSuccess(now, "v0.6.0")
	if err == nil {
		t.Fatal("a write under a regular file reported success")
	}
	if s.LatestVersion != "v0.6.0" || !s.SucceededAt.Equal(now) {
		t.Errorf("returned %+v; want the answer even though it was not written", s)
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

func TestStoreOntoADirectoryFailsAndRemovesTheTempFile(t *testing.T) {
	c, path := tempCache(t)
	if err := os.MkdirAll(filepath.Join(path, "occupied"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := c.Store(Snapshot{CheckedAt: time.Now(), LatestVersion: "v0.5.0"}); err == nil {
		t.Fatal("Store replaced a non-empty directory")
	}
	if names := entries(t, filepath.Dir(path)); len(names) != 1 || names[0] != "updatecheck-cli.json" {
		t.Errorf("a failed rename left %v, want only the blocking directory", names)
	}
	if _, ok := c.Load(); ok {
		t.Error("Load read a record through a directory")
	}
}
