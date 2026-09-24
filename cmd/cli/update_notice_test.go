package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adrg/xdg"

	"unidoc-pdf-debugger/internal/updatecheck"
)

// referenceBox is the notice for 0.4.0 -> 0.5.0: every row 49 columns.
const referenceBox = "" +
	"+-----------------------------------------------+\n" +
	"|  Update available: 0.4.0 -> 0.5.0             |\n" +
	"|  https://github.com/unidoc/pdfdebug/releases  |\n" +
	"+-----------------------------------------------+\n"

// harness is one CLI invocation's notice environment with a temp cache, a
// local releases server and a settable clock.
type harness struct {
	env    *noticeEnv
	vars   map[string]string
	stdout *bytes.Buffer
	stderr *bytes.Buffer
	hits   *atomic.Int32
	dir    string
	path   string
	now    time.Time
	tag    string
	status int
}

func newHarness(t *testing.T, version string) *harness {
	t.Helper()
	h := &harness{
		vars:   map[string]string{},
		stdout: &bytes.Buffer{},
		stderr: &bytes.Buffer{},
		hits:   &atomic.Int32{},
		dir:    filepath.Join(t.TempDir(), "pdfdebug"),
		now:    time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC),
		tag:    "v0.5.0",
		status: http.StatusOK,
	}
	h.path = filepath.Join(h.dir, "updatecheck-cli.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.hits.Add(1)
		if h.status != http.StatusOK {
			http.Error(w, "unavailable", h.status)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{"tag_name": h.tag}})
	}))
	t.Cleanup(srv.Close)
	cache, err := updatecheck.Open(h.dir, updatecheck.SurfaceCLI)
	if err != nil {
		t.Fatal(err)
	}
	checker := &updatecheck.Checker{BaseURL: srv.URL, Client: srv.Client()}
	h.env = &noticeEnv{
		version: version,
		lookupEnv: func(k string) (string, bool) {
			v, ok := h.vars[k]
			return v, ok
		},
		stdoutTTY: true,
		stderrTTY: true,
		now:       func() time.Time { return h.now },
		stdout:    h.stdout,
		stderr:    h.stderr,
		cache:     cache,
		latest:    checker.LatestStable,
	}
	return h
}

func (h *harness) seed(t *testing.T, checkedAt time.Time, latest string) {
	t.Helper()
	if err := h.env.cache.Store(updatecheck.Snapshot{CheckedAt: checkedAt, LatestVersion: latest}); err != nil {
		t.Fatal(err)
	}
}

// seedApp writes the desktop app's record in the harness cache directory.
func (h *harness) seedApp(t *testing.T, s updatecheck.Snapshot) {
	t.Helper()
	app, err := updatecheck.Open(h.dir, updatecheck.SurfaceApp)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Store(s); err != nil {
		t.Fatal(err)
	}
}

// notice runs the end-of-run notice path for args with no command in between.
func (h *harness) notice(args ...string) string {
	h.stderr.Reset()
	startNotice(h.env, args).finish()
	return h.stderr.String()
}

func dirExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestNoticeBoxMatchesTheReferenceRows(t *testing.T) {
	var b bytes.Buffer
	writeNotice(&b, "0.4.0", "v0.5.0", 0, false)
	if b.String() != referenceBox {
		t.Errorf("box:\n%s\nwant:\n%s", b.String(), referenceBox)
	}
	for _, row := range strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n") {
		if len(row) != 49 {
			t.Errorf("row %q is %d columns, want 49", row, len(row))
		}
	}
	for i, c := range b.Bytes() {
		if c > 0x7f {
			t.Fatalf("non-ASCII byte at %d", i)
		}
	}
}

func TestNoticeLeadingBlankLineOnOrdinaryCommands(t *testing.T) {
	var b bytes.Buffer
	writeNotice(&b, "0.4.0", "0.5.0", 0, true)
	if b.String() != "\n"+referenceBox {
		t.Errorf("notice = %q, want a blank line then the box", b.String())
	}
}

func TestNoticeDegradesToPlainLinesWhenTheBoxDoesNotFit(t *testing.T) {
	plain := "Update available: 0.4.0 -> 0.5.0\nhttps://github.com/unidoc/pdfdebug/releases\n"
	for width, want := range map[int]string{0: referenceBox, 50: referenceBox, 120: referenceBox, 49: plain, 48: plain, 20: plain} {
		var b bytes.Buffer
		writeNotice(&b, "0.4.0", "0.5.0", width, false)
		if b.String() != want {
			t.Errorf("width %d:\n%s\nwant:\n%s", width, b.String(), want)
		}
	}
}

func TestInteractiveStaleRunRefreshesAndShowsTheNoticeInTheSameRun(t *testing.T) {
	h := newHarness(t, "0.4.0")
	got := h.notice("dump", "tree", "f.pdf")
	if got != "\n"+referenceBox {
		t.Errorf("stderr = %q, want the box", got)
	}
	if n := h.hits.Load(); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
	snap, ok := h.env.cache.Load()
	if !ok || snap.LatestVersion != "v0.5.0" || !snap.CheckedAt.Equal(h.now) {
		t.Errorf("record = %+v, %v", snap, ok)
	}
}

func TestFinishedRefreshWinsWhenTheCommandOutlastsTheDeadline(t *testing.T) {
	h := newHarness(t, "0.4.0")
	for i := 0; i < 50; i++ {
		h.stderr.Reset()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		done := make(chan updatecheck.Snapshot, 1)
		done <- updatecheck.Snapshot{Schema: 1, CheckedAt: h.now, LatestVersion: "v0.5.0"}
		answered := make(chan struct{})
		close(answered)
		p := &pendingNotice{env: h.env, active: true, done: done, answered: answered, ctx: ctx, cancel: cancel}
		p.finish()
		if got := h.stderr.String(); got != "\n"+referenceBox {
			t.Fatalf("iteration %d: stderr = %q, want the box from the finished refresh", i, got)
		}
	}
}

func TestAnsweredRefreshIsAwaitedPastTheDeadline(t *testing.T) {
	h := newHarness(t, "0.4.0")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	answered := make(chan struct{})
	close(answered)
	done := make(chan updatecheck.Snapshot, 1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		done <- updatecheck.Snapshot{Schema: 1, CheckedAt: h.now, LatestVersion: "v0.5.0"}
	}()
	p := &pendingNotice{env: h.env, active: true, done: done, answered: answered, ctx: ctx, cancel: cancel}
	p.finish()
	if got := h.stderr.String(); got != "\n"+referenceBox {
		t.Errorf("stderr = %q, want the box from the refresh still writing its record", got)
	}
}

func TestUnansweredRefreshIsNotAwaitedPastTheDeadline(t *testing.T) {
	h := newHarness(t, "0.4.0")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &pendingNotice{env: h.env, active: true, done: make(chan updatecheck.Snapshot, 1), answered: make(chan struct{}), ctx: ctx, cancel: cancel}
	finished := make(chan struct{})
	go func() {
		p.finish()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("finish waited on a server call that never returned")
	}
}

func TestNoticeShowsOnEveryEligibleRunFromTheCache(t *testing.T) {
	h := newHarness(t, "0.4.0")
	h.seed(t, h.now.Add(-time.Hour), "v0.5.0")
	for i := 0; i < 3; i++ {
		if got := h.notice("dump", "tree", "f.pdf"); got != "\n"+referenceBox {
			t.Fatalf("run %d: stderr = %q, want the box", i+1, got)
		}
	}
	if n := h.hits.Load(); n != 0 {
		t.Errorf("%d requests, want 0 while the record is fresh", n)
	}
	if names, _ := os.ReadDir(h.dir); len(names) != 1 {
		t.Errorf("cache dir holds %v, want only the record", names)
	}
}

func TestConfirmedAppRecordSparesTheRefreshAndDrivesTheNotice(t *testing.T) {
	h := newHarness(t, "0.4.0")
	h.seedApp(t, updatecheck.Snapshot{CheckedAt: h.now.Add(-time.Hour), SucceededAt: h.now.Add(-time.Hour), LatestVersion: "v0.5.0"})
	if got := h.notice("dump", "tree", "f.pdf"); got != "\n"+referenceBox {
		t.Errorf("stderr = %q, want the box from the app's record", got)
	}
	if n := h.hits.Load(); n != 0 {
		t.Errorf("%d requests, want 0 while the app's check is confirmed", n)
	}
	if _, err := os.Stat(h.path); !os.IsNotExist(err) {
		t.Errorf("the CLI wrote its own record without refreshing (stat err %v)", err)
	}
}

func TestUnconfirmedAppRecordDoesNotSpareTheRefresh(t *testing.T) {
	h := newHarness(t, "0.4.0")
	h.seedApp(t, updatecheck.Snapshot{CheckedAt: h.now.Add(-time.Minute), SucceededAt: h.now.Add(-48 * time.Hour), LatestVersion: "v0.4.0"})
	if got := h.notice("dump", "tree", "f.pdf"); got != "\n"+referenceBox {
		t.Errorf("stderr = %q, want the box from the CLI's own refresh", got)
	}
	if n := h.hits.Load(); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
}

func TestNoticeShowsTheHigherOfTheCLIAndAppRecords(t *testing.T) {
	const box060 = "+-----------------------------------------------+\n" +
		"|  Update available: 0.4.0 -> 0.6.0             |\n" +
		"|  https://github.com/unidoc/pdfdebug/releases  |\n" +
		"+-----------------------------------------------+\n"
	h := newHarness(t, "0.4.0")
	h.seed(t, h.now.Add(-time.Hour), "v0.5.0")
	h.seedApp(t, updatecheck.Snapshot{CheckedAt: h.now.Add(-48 * time.Hour), SucceededAt: h.now.Add(-48 * time.Hour), LatestVersion: "v0.6.0"})
	if got := h.notice("dump", "tree", "f.pdf"); got != "\n"+box060 {
		t.Errorf("stderr = %q, want the app's higher 0.6.0", got)
	}
	h.stderr.Reset()
	h.stdout.Reset()
	runVersion(h.env)
	if got := h.stderr.String(); got != box060 {
		t.Errorf("--version stderr = %q, want the app's higher 0.6.0 after the CLI refresh found 0.5.0", got)
	}
	if snap, _ := h.env.cache.Load(); snap.LatestVersion != "v0.5.0" {
		t.Errorf("CLI record = %+v; the app's version must not be copied into it", snap)
	}
}

func TestVersionSpellingsCompareAsOneVersion(t *testing.T) {
	h := newHarness(t, "v0.5.0")
	if err := os.MkdirAll(h.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	record := `{"schema":1,"checked_at":"` + h.now.Format(time.RFC3339) + `","latest_version":"0.5.0"}`
	if err := os.WriteFile(h.path, []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := h.notice("dump", "tree", "f.pdf"); got != "" {
		t.Errorf("stderr = %q; a v0.5.0 build told about 0.5.0", got)
	}
}

func TestFreshRecordShowsTheNoticeWithoutARequest(t *testing.T) {
	h := newHarness(t, "0.4.0")
	h.seed(t, h.now.Add(-time.Hour), "v0.5.0")
	if got := h.notice("dump", "tree", "f.pdf"); got != "\n"+referenceBox {
		t.Errorf("stderr = %q, want the box", got)
	}
	if n := h.hits.Load(); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
}

func TestEachGuardSuppressesTheNoticeAndTheRefresh(t *testing.T) {
	cases := []struct {
		name    string
		version string
		args    []string
		setup   func(h *harness)
	}{
		{name: "CI set to false", setup: func(h *harness) { h.vars["CI"] = "false" }},
		{name: "PDFDEBUG_NO_UPDATE_CHECK empty", setup: func(h *harness) { h.vars["PDFDEBUG_NO_UPDATE_CHECK"] = "" }},
		{name: "NO_UPDATE_NOTIFIER zero", setup: func(h *harness) { h.vars["NO_UPDATE_NOTIFIER"] = "0" }},
		{name: "stdout piped, stderr a terminal", setup: func(h *harness) { h.env.stdoutTTY = false }},
		{name: "stderr piped, stdout a terminal", setup: func(h *harness) { h.env.stderrTTY = false }},
		{name: "json flag", args: []string{"dump", "tree", "--json", "f.pdf"}},
		{name: "dump bytes", args: []string{"dump", "bytes", "f.pdf"}},
		{name: "embedded extraction", args: []string{"dump", "embedded", "--name", "x.xml", "f.pdf"}},
		{name: "dev build", version: "dev"},
		{name: "non-SemVer build", version: "abc"},
		{name: "no cache path", setup: func(h *harness) { h.env.cache = nil }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			version := c.version
			if version == "" {
				version = "0.4.0"
			}
			h := newHarness(t, version)
			if c.setup != nil {
				c.setup(h)
			}
			args := c.args
			if args == nil {
				args = []string{"dump", "tree", "f.pdf"}
			}
			if got := h.notice(args...); got != "" {
				t.Errorf("stderr = %q, want nothing", got)
			}
			if n := h.hits.Load(); n != 0 {
				t.Errorf("%d requests, want 0", n)
			}
			if dirExists(h.dir) {
				t.Error("a suppressed run created the cache directory")
			}
		})
	}
}

func TestMachineFormatDetection(t *testing.T) {
	cases := map[string]bool{
		"dump tree f.pdf":                  false,
		"dump tree --json f.pdf":           true,
		"dump tree -json f.pdf":            true,
		"dump tree --json=true f.pdf":      true,
		"dump tree --json=T f.pdf":         true,
		"dump tree --json=1 f.pdf":         true,
		"dump tree --json=false f.pdf":     false,
		"dump tree -json=0 f.pdf":          false,
		"dump tree --json=F f.pdf":         false,
		"dump tree --json=maybe f.pdf":     true,
		"dump stream --ops --page 1 f.pdf": true,
		"dump stream --raw --page 1 f.pdf": true,
		"dump source --raw=false f.pdf":    false,
		"dump bytes f.pdf":                 true,
		"dump plaintext f.pdf":             true,
		"dump embedded f.pdf":              false,
		"dump embedded --ref 4_0_R f.pdf":  true,
		"dump embedded --name=x.xml f.pdf": true,
		"dump object --ref 4_0_R f.pdf":    false,
		"validate --json f.pdf":            true,
		"diff -- --json":                   false,
		"--help":                           false,
		"":                                 false,
	}
	for line, want := range cases {
		args := strings.Fields(line)
		if got := machineFormat(args); got != want {
			t.Errorf("machineFormat(%q) = %v, want %v", line, got, want)
		}
	}
}

func TestAttemptRecordIsWrittenBeforeTheRequest(t *testing.T) {
	h := newHarness(t, "0.4.0")
	h.seed(t, h.now.Add(-48*time.Hour), "v0.5.0")
	var seen updatecheck.Snapshot
	latest := func(context.Context) (string, error) {
		seen, _ = h.env.cache.Load()
		return "", errors.New("offline")
	}
	snap, err := refreshSnapshot(t.Context(), h.env.cache, latest, h.now)
	if err == nil {
		t.Fatal("a failed request reported success")
	}
	if !seen.CheckedAt.Equal(h.now) || seen.LatestVersion != "v0.5.0" {
		t.Errorf("record at request time = %+v, want the attempt record", seen)
	}
	stored, _ := h.env.cache.Load()
	if !stored.CheckedAt.Equal(h.now) || stored.LatestVersion != "v0.5.0" || snap.LatestVersion != "v0.5.0" {
		t.Errorf("after a failed refresh: stored %+v, returned %+v", stored, snap)
	}
}

func TestOnlyASuccessfulRefreshAdvancesSucceededAt(t *testing.T) {
	h := newHarness(t, "0.4.0")
	h.seed(t, h.now.Add(-48*time.Hour), "v0.5.0")
	before, _ := h.env.cache.Load()
	failed := func(context.Context) (string, error) { return "", errors.New("offline") }
	if _, err := refreshSnapshot(t.Context(), h.env.cache, failed, h.now); err == nil {
		t.Fatal("a failed request reported success")
	}
	stored, _ := h.env.cache.Load()
	if !stored.SucceededAt.Equal(before.SucceededAt) {
		t.Errorf("succeeded_at = %v after a failed refresh, want %v kept", stored.SucceededAt, before.SucceededAt)
	}
	later := h.now.Add(time.Minute)
	ok := func(context.Context) (string, error) { return "v0.6.0", nil }
	snap, err := refreshSnapshot(t.Context(), h.env.cache, ok, later)
	if err != nil {
		t.Fatal(err)
	}
	stored, _ = h.env.cache.Load()
	if !stored.SucceededAt.Equal(later) || !snap.SucceededAt.Equal(later) || stored.LatestVersion != "v0.6.0" {
		t.Errorf("after a successful refresh: stored %+v, returned %+v", stored, snap)
	}
}

func TestNullDevicePipesAndFilesAreNotTerminals(t *testing.T) {
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = null.Close() })
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
	file, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	for name, f := range map[string]*os.File{"null device": null, "pipe": w, "regular file": file} {
		if tty, width := terminalInfo(f); tty || width != 0 {
			t.Errorf("%s: terminalInfo = %v, %d; want not a terminal", name, tty, width)
		}
	}
}

func TestHungRefreshIsCutOffAtTheBudgetAndSaysNothing(t *testing.T) {
	h := newHarness(t, "0.4.0")
	stale := h.now.Add(-48 * time.Hour)
	h.seed(t, stale, "v0.4.0")
	h.env.latest = func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	start := time.Now()
	got := h.notice("dump", "tree", "f.pdf")
	if d := time.Since(start); d < refreshTimeout || d > refreshTimeout+time.Second {
		t.Errorf("notice path took %v, want about the %v refresh budget", d, refreshTimeout)
	}
	if got != "" {
		t.Errorf("stderr = %q, want nothing after a failed refresh", got)
	}
	snap, ok := h.env.cache.Load()
	if !ok || !snap.CheckedAt.Equal(h.now) || snap.LatestVersion != "v0.4.0" {
		t.Errorf("record = %+v, %v; want checked_at advanced and latest kept", snap, ok)
	}
}

func TestUnwritableCacheMakesNoRequest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not apply on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not bind root")
	}
	h := newHarness(t, "0.4.0")
	if err := os.MkdirAll(h.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(h.dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(h.dir, 0o700) })
	if got := h.notice("dump", "tree", "f.pdf"); got != "" {
		t.Errorf("stderr = %q", got)
	}
	if n := h.hits.Load(); n != 0 {
		t.Errorf("%d requests with an unwritable cache, want 0", n)
	}
}

// captureStdout points os.Stdout at a temp file for the duration of fn, so
// callers must not run in parallel.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = old }()
	fn()
	_ = f.Close()
	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func fixture(name string) string {
	return filepath.Join("..", "..", "testdata", name)
}

func TestGateExitCodesAreUnchangedWithANoticePending(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.pdf")
	cases := []struct {
		name string
		args []string
		code int
	}{
		{"validate clean", []string{"validate", "--profile", "pdfua-1-structural", fixture("tagged.pdf")}, 0},
		{"validate errors", []string{"validate", fixture("non-embedded-font.pdf")}, 1},
		{"validate operational", []string{"validate", missing}, 2},
		{"diff identical", []string{"diff", fixture("minimal.pdf"), fixture("minimal.pdf")}, 0},
		{"diff differ", []string{"diff", fixture("minimal.pdf"), fixture("multipage.pdf")}, 1},
		{"diff operational", []string{"diff", fixture("minimal.pdf"), fixture("malformed.pdf")}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, "0.4.0")
			h.seed(t, h.now, "v0.5.0")
			var code int
			out := captureStdout(t, func() {
				code = run(h.env, append([]string{"pdfdebug"}, c.args...))
			})
			if code != c.code {
				t.Errorf("exit = %d, want %d", code, c.code)
			}
			if h.stderr.String() != "\n"+referenceBox {
				t.Errorf("notice = %q, want the box", h.stderr.String())
			}
			if strings.Contains(out, "Update available") {
				t.Error("the notice reached stdout")
			}
		})
	}
}

func TestPanickingCheckerLeavesTheExitCodeIntact(t *testing.T) {
	h := newHarness(t, "0.4.0")
	h.env.latest = func(context.Context) (string, error) { panic("boom") }
	var code int
	captureStdout(t, func() {
		code = run(h.env, []string{"pdfdebug", "validate", fixture("non-embedded-font.pdf")})
	})
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if h.stderr.Len() != 0 {
		t.Errorf("stderr = %q, want nothing", h.stderr.String())
	}
}

func TestVersionOutcomes(t *testing.T) {
	cases := []struct {
		name       string
		version    string
		setup      func(t *testing.T, h *harness)
		wantStderr string
		wantHits   int32
	}{
		{name: "update available", version: "0.4.0", wantStderr: referenceBox, wantHits: 1},
		{name: "current", version: "0.5.0", wantStderr: "pdfdebug: no newer release is available\n", wantHits: 1},
		{name: "check failed", version: "0.4.0", setup: func(t *testing.T, h *harness) { h.status = http.StatusForbidden },
			wantStderr: "pdfdebug: could not check for updates\n", wantHits: 1},
		{name: "no cache", version: "0.4.0", setup: func(t *testing.T, h *harness) { h.env.cache = nil },
			wantStderr: "pdfdebug: could not check for updates\n"},
		{name: "dev build", version: "dev", wantStderr: "pdfdebug: update checks are skipped for development builds\n"},
		{name: "opted out", version: "0.4.0", setup: func(t *testing.T, h *harness) { h.vars["NO_UPDATE_NOTIFIER"] = "" },
			wantStderr: "pdfdebug: update check disabled (PDFDEBUG_NO_UPDATE_CHECK or NO_UPDATE_NOTIFIER is set)\n"},
		{name: "check failed, app confirms newer", version: "0.4.0", setup: func(t *testing.T, h *harness) {
			h.status = http.StatusForbidden
			h.seedApp(t, updatecheck.Snapshot{CheckedAt: h.now.Add(-time.Hour), SucceededAt: h.now.Add(-time.Hour), LatestVersion: "v0.5.0"})
		}, wantStderr: referenceBox, wantHits: 1},
		{name: "check failed, app record unconfirmed", version: "0.4.0", setup: func(t *testing.T, h *harness) {
			h.status = http.StatusForbidden
			h.seedApp(t, updatecheck.Snapshot{CheckedAt: h.now.Add(-time.Hour), SucceededAt: h.now.Add(-48 * time.Hour), LatestVersion: "v0.5.0"})
		}, wantStderr: "pdfdebug: could not check for updates\n", wantHits: 1},
		{name: "check failed, app confirms current", version: "0.5.0", setup: func(t *testing.T, h *harness) {
			h.status = http.StatusForbidden
			h.seedApp(t, updatecheck.Snapshot{CheckedAt: h.now.Add(-time.Hour), SucceededAt: h.now.Add(-time.Hour), LatestVersion: "v0.5.0"})
		}, wantStderr: "pdfdebug: no newer release is available\n", wantHits: 1},
	}
	for _, c := range cases {
		for _, flag := range []string{"--version", "-v"} {
			t.Run(c.name+" "+flag, func(t *testing.T) {
				h := newHarness(t, c.version)
				if c.setup != nil {
					c.setup(t, h)
				}
				if code := run(h.env, []string{"pdfdebug", flag}); code != 0 {
					t.Errorf("exit = %d, want 0", code)
				}
				if want := "pdfdebug version " + c.version + "\n"; h.stdout.String() != want {
					t.Errorf("stdout = %q, want %q", h.stdout.String(), want)
				}
				if h.stderr.String() != c.wantStderr {
					t.Errorf("stderr = %q, want %q", h.stderr.String(), c.wantStderr)
				}
				if n := h.hits.Load(); n != c.wantHits {
					t.Errorf("%d requests, want %d", n, c.wantHits)
				}
			})
		}
	}
}

func TestVersionOutsideATerminalPrintsOnlyTheVersionLine(t *testing.T) {
	cases := map[string]struct {
		version string
		setup   func(h *harness)
	}{
		"stdout piped":       {"0.4.0", func(h *harness) { h.env.stdoutTTY = false }},
		"stderr redirected":  {"0.4.0", func(h *harness) { h.env.stderrTTY = false }},
		"CI set":             {"0.4.0", func(h *harness) { h.vars["CI"] = "false" }},
		"dev build piped":    {"dev", func(h *harness) { h.env.stdoutTTY = false }},
		"opted out in CI":    {"0.4.0", func(h *harness) { h.vars["CI"] = ""; h.vars["NO_UPDATE_NOTIFIER"] = "1" }},
		"update pending, CI": {"0.4.0", func(h *harness) { h.vars["CI"] = "true"; h.seed(t, h.now.Add(-time.Hour), "v0.5.0") }},
	}
	for name, c := range cases {
		for _, flag := range []string{"--version", "-v"} {
			t.Run(name+" "+flag, func(t *testing.T) {
				h := newHarness(t, c.version)
				c.setup(h)
				before, _ := os.ReadFile(h.path)
				if code := run(h.env, []string{"pdfdebug", flag}); code != 0 {
					t.Errorf("exit = %d, want 0", code)
				}
				if want := "pdfdebug version " + c.version + "\n"; h.stdout.String() != want {
					t.Errorf("stdout = %q, want %q", h.stdout.String(), want)
				}
				if got := h.stderr.String(); got != "" {
					t.Errorf("stderr = %q, want nothing", got)
				}
				if n := h.hits.Load(); n != 0 {
					t.Errorf("%d requests, want 0", n)
				}
				if after, _ := os.ReadFile(h.path); !bytes.Equal(before, after) {
					t.Errorf("the CLI record changed: %q -> %q", before, after)
				}
			})
		}
	}
}

func TestVersionPrintsTheBoxBeforeTheVersionLine(t *testing.T) {
	h := newHarness(t, "0.4.0")
	var both bytes.Buffer
	h.env.stdout, h.env.stderr = &both, &both
	runVersion(h.env)
	if want := referenceBox + "pdfdebug version 0.4.0\n"; both.String() != want {
		t.Errorf("output = %q, want %q", both.String(), want)
	}
}

func TestDevAndOptedOutVersionTouchNoCache(t *testing.T) {
	for name, h := range map[string]*harness{"dev": newHarness(t, "dev"), "opted out": newHarness(t, "0.4.0")} {
		if name == "opted out" {
			h.vars["PDFDEBUG_NO_UPDATE_CHECK"] = "1"
		}
		runVersion(h.env)
		if dirExists(h.dir) {
			t.Errorf("%s: --version created the cache directory", name)
		}
	}
}

func TestEmptyReleasePageKeepsThePreviousLatestAndCountsAsChecked(t *testing.T) {
	h := newHarness(t, "0.4.0")
	h.seed(t, h.now.Add(-48*time.Hour), "v0.5.0")
	snap, err := refreshSnapshot(t.Context(), h.env.cache, func(context.Context) (string, error) { return "", nil }, h.now)
	if err != nil {
		t.Fatalf("an empty page reported %v, want success", err)
	}
	stored, ok := h.env.cache.Load()
	if !ok || !stored.CheckedAt.Equal(h.now) || stored.LatestVersion != "v0.5.0" {
		t.Errorf("stored %+v, %v; want checked_at advanced and v0.5.0 kept", stored, ok)
	}
	if !snap.CheckedAt.Equal(h.now) || snap.LatestVersion != "v0.5.0" {
		t.Errorf("returned %+v, want checked_at advanced and v0.5.0 kept", snap)
	}
}

func TestEmptyReleasePageOutcomes(t *testing.T) {
	t.Run("ordinary command still shows the known newer version", func(t *testing.T) {
		h := newHarness(t, "0.4.0")
		h.tag = ""
		h.seed(t, h.now.Add(-48*time.Hour), "v0.5.0")
		if got := h.notice("dump", "tree", "f.pdf"); got != "\n"+referenceBox {
			t.Errorf("stderr = %q, want the box", got)
		}
		if n := h.hits.Load(); n != 1 {
			t.Errorf("%d requests, want 1", n)
		}
	})
	t.Run("version with a known newer version shows the box", func(t *testing.T) {
		h := newHarness(t, "0.4.0")
		h.tag = ""
		h.seed(t, h.now.Add(-48*time.Hour), "v0.5.0")
		runVersion(h.env)
		if h.stderr.String() != referenceBox {
			t.Errorf("stderr = %q, want the box", h.stderr.String())
		}
	})
	t.Run("version with no record says nothing is newer", func(t *testing.T) {
		h := newHarness(t, "0.4.0")
		h.tag = ""
		runVersion(h.env)
		if want := "pdfdebug: no newer release is available\n"; h.stderr.String() != want {
			t.Errorf("stderr = %q, want %q", h.stderr.String(), want)
		}
	})
}

func TestProductionNoticeEnvResolvesTheCacheAndTouchesNoDisk(t *testing.T) {
	orig := xdg.CacheHome
	t.Cleanup(func() { xdg.CacheHome = orig })

	base := t.TempDir()
	xdg.CacheHome = base
	env := newNoticeEnv()
	if env.cache == nil {
		t.Fatal("an absolute cache home produced no cache")
	}
	if env.version != version || env.latest == nil || env.now == nil || env.lookupEnv == nil {
		t.Errorf("environment not wired: %+v", env)
	}
	if env.stdout != os.Stdout || env.stderr != os.Stderr {
		t.Error("environment does not write to the process streams")
	}
	if names, _ := os.ReadDir(base); len(names) != 0 {
		t.Errorf("building the environment wrote %v", names)
	}
	if err := env.cache.Store(updatecheck.Snapshot{CheckedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if !dirExists(filepath.Join(base, "pdfdebug", "updatecheck-cli.json")) {
		t.Error("the CLI record does not live at the default path")
	}

	xdg.CacheHome = "relative"
	if env := newNoticeEnv(); env.cache != nil {
		t.Error("a relative cache home produced a cache")
	}
}
