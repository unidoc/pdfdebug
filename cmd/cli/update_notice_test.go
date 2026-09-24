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
	h.path = filepath.Join(h.dir, "updatecheck.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.hits.Add(1)
		if h.status != http.StatusOK {
			http.Error(w, "unavailable", h.status)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{"tag_name": h.tag}})
	}))
	t.Cleanup(srv.Close)
	cache, err := updatecheck.Open(h.path)
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
	for width, want := range map[int]string{0: referenceBox, 49: referenceBox, 120: referenceBox, 48: plain, 20: plain} {
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
		_ = os.Remove(filepath.Join(h.dir, "updatenotice.json"))
		h.stderr.Reset()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		done := make(chan updatecheck.Snapshot, 1)
		done <- updatecheck.Snapshot{Schema: 1, CheckedAt: h.now, LatestVersion: "v0.5.0"}
		p := &pendingNotice{env: h.env, active: true, done: done, ctx: ctx, cancel: cancel}
		p.finish()
		if got := h.stderr.String(); got != "\n"+referenceBox {
			t.Fatalf("iteration %d: stderr = %q, want the box from the finished refresh", i, got)
		}
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

func TestNoticeShowsOncePerVersionWithOneRearmAfterAWeek(t *testing.T) {
	h := newHarness(t, "0.4.0")
	start := h.now
	h.seed(t, start, "v0.5.0")
	steps := []struct {
		after time.Duration
		want  bool
	}{
		{0, true},
		{time.Hour, false},
		{6 * 24 * time.Hour, false},
		{7 * 24 * time.Hour, true},
		{8 * 24 * time.Hour, false},
		{30 * 24 * time.Hour, false},
	}
	for _, s := range steps {
		h.now = start.Add(s.after)
		h.seed(t, h.now, "v0.5.0")
		if got := h.notice("dump", "tree", "f.pdf") != ""; got != s.want {
			t.Errorf("after %v: shown = %v, want %v", s.after, got, s.want)
		}
	}
	h.now = start.Add(31 * 24 * time.Hour)
	h.seed(t, h.now, "v0.6.0")
	if h.notice("dump", "tree", "f.pdf") == "" {
		t.Error("a new target version did not show")
	}
	h.now = h.now.Add(time.Hour)
	if h.notice("dump", "tree", "f.pdf") != "" {
		t.Error("the new target version showed twice")
	}
}

func TestShownStateInTheFutureWaitsAWeekPastIt(t *testing.T) {
	h := newHarness(t, "0.4.0")
	h.seed(t, h.now, "v0.5.0")
	future := h.now.Add(48 * time.Hour)
	if err := h.env.cache.StoreShown(updatecheck.Shown{Version: "v0.5.0", FirstShownAt: future}); err != nil {
		t.Fatal(err)
	}
	h.now = h.now.Add(8 * 24 * time.Hour)
	h.seed(t, h.now, "v0.5.0")
	if h.notice("dump", "tree", "f.pdf") != "" {
		t.Error("re-armed before a week past the future first_shown_at")
	}
	h.now = future.Add(7 * 24 * time.Hour)
	h.seed(t, h.now, "v0.5.0")
	if h.notice("dump", "tree", "f.pdf") == "" {
		t.Error("did not re-arm a week past the future first_shown_at")
	}
}

func TestGUIAndCLIVersionSpellingsAreOneTarget(t *testing.T) {
	h := newHarness(t, "0.4.0")
	if err := os.MkdirAll(h.dir, 0o700); err != nil {
		t.Fatal(err)
	}
	record := `{"schema":1,"checked_at":"` + h.now.Format(time.RFC3339) + `","latest_version":"0.5.0"}`
	if err := os.WriteFile(h.path, []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}
	if h.notice("dump", "tree", "f.pdf") == "" {
		t.Fatal("first notice did not show")
	}
	h.seed(t, h.now, "v0.5.0")
	if h.notice("dump", "tree", "f.pdf") != "" {
		t.Error("0.5.0 and v0.5.0 counted as two targets")
	}
	shown := `{"schema":1,"version":"0.5.0","first_shown_at":"` + h.now.Format(time.RFC3339) + `","rearmed":false}`
	if err := os.WriteFile(filepath.Join(h.dir, "updatenotice.json"), []byte(shown), 0o600); err != nil {
		t.Fatal(err)
	}
	if h.notice("dump", "tree", "f.pdf") != "" {
		t.Error("a shown-state of 0.5.0 did not match a record of v0.5.0")
	}
}

func TestUnwritableShownStatePrintsNothing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not apply on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not bind root")
	}
	h := newHarness(t, "0.4.0")
	h.seed(t, h.now, "v0.5.0")
	if err := os.Chmod(h.dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(h.dir, 0o700) })
	for range 3 {
		if got := h.notice("dump", "tree", "f.pdf"); got != "" {
			t.Fatalf("stderr = %q, want nothing when the shown-state cannot be written", got)
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
		{name: "piped and in CI", version: "0.4.0", setup: func(t *testing.T, h *harness) {
			h.env.stdoutTTY, h.env.stderrTTY = false, false
			h.vars["CI"] = "true"
		}, wantStderr: referenceBox, wantHits: 1},
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

func TestVersionPrintsTheBoxBeforeTheVersionLine(t *testing.T) {
	h := newHarness(t, "0.4.0")
	var both bytes.Buffer
	h.env.stdout, h.env.stderr = &both, &both
	runVersion(h.env)
	if want := referenceBox + "pdfdebug version 0.4.0\n"; both.String() != want {
		t.Errorf("output = %q, want %q", both.String(), want)
	}
}

func TestVersionBoxCountsAsShown(t *testing.T) {
	h := newHarness(t, "0.4.0")
	runVersion(h.env)
	if h.notice("dump", "tree", "f.pdf") != "" {
		t.Error("an ordinary command repeated the notice --version just showed")
	}
}

func TestVersionBoxSpendsADueRearm(t *testing.T) {
	h := newHarness(t, "0.4.0")
	first := h.now.Add(-8 * 24 * time.Hour)
	if err := h.env.cache.StoreShown(updatecheck.Shown{Version: "v0.5.0", FirstShownAt: first}); err != nil {
		t.Fatal(err)
	}
	runVersion(h.env)
	if h.notice("dump", "tree", "f.pdf") != "" {
		t.Error("an ordinary command showed the re-arm right after --version showed the box")
	}
	shown, ok := h.env.cache.LoadShown()
	if !ok || !shown.Rearmed || !shown.FirstShownAt.Equal(first) {
		t.Errorf("shown-state = %+v, want the re-arm spent and first_shown_at kept", shown)
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

func TestPlainFallbackCountsAsShown(t *testing.T) {
	h := newHarness(t, "0.4.0")
	h.env.width = 20
	h.seed(t, h.now, "v0.5.0")
	if got := h.notice("dump", "tree", "f.pdf"); got != "\nUpdate available: 0.4.0 -> 0.5.0\n"+releasesURL+"\n" {
		t.Errorf("stderr = %q, want the two plain lines", got)
	}
	if h.notice("dump", "tree", "f.pdf") != "" {
		t.Error("the plain fallback did not count as shown")
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
	if !dirExists(filepath.Join(base, "pdfdebug", "updatecheck.json")) {
		t.Error("the cache does not live at the default path")
	}

	xdg.CacheHome = "relative"
	if env := newNoticeEnv(); env.cache != nil {
		t.Error("a relative cache home produced a cache")
	}
}
