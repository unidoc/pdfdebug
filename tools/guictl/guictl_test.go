package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseMCPURLReadsTheUncolouredURLAttribute(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
		ok   bool
	}{
		{
			name: "tint line written to a pipe",
			line: `10:42AM INF MCP server started. Connect MCP clients using the streamable HTTP transport. url=http://127.0.0.1:54321/mcp`,
			want: "http://127.0.0.1:54321/mcp",
			ok:   true,
		},
		{
			name: "url attribute followed by another attribute",
			line: `10:42AM INF MCP server started. Connect MCP clients using the streamable HTTP transport. url=http://127.0.0.1:9099/mcp extra=1`,
			want: "http://127.0.0.1:9099/mcp",
			ok:   true,
		},
		{
			name: "unrelated log line",
			line: `10:42AM INF Build Info: Wails=v3.0.0`,
			ok:   false,
		},
		{
			name: "url attribute on a line that is not the MCP start line",
			line: `10:42AM INF Asset request url=http://wails.localhost/index.html`,
			ok:   false,
		},
		{
			name: "empty line",
			line: ``,
			ok:   false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := parseMCPURL(c.line)
			if ok != c.ok || got != c.want {
				t.Errorf("parseMCPURL(%q) = %q, %v; want %q, %v", c.line, got, ok, c.want, c.ok)
			}
		})
	}
}

// fakeProcess reports a process table with one entry.
func fakeProcess(pid int, startTime string, alive *atomic.Bool) processProbe {
	return func(p int) (string, bool) {
		if p != pid || !alive.Load() {
			return "", false
		}
		return startTime, true
	}
}

func TestCheckInstanceClassifiesTheStateFile(t *testing.T) {
	var alive atomic.Bool
	alive.Store(true)
	probe := fakeProcess(4242, "Mon Nov 10 10:00:00 2026", &alive)
	cases := []struct {
		name  string
		state instanceState
		alive bool
		want  instanceStatus
	}{
		{"live process with matching start time", instanceState{PID: 4242, StartTime: "Mon Nov 10 10:00:00 2026"}, true, instanceLive},
		{"pid no longer exists", instanceState{PID: 4242, StartTime: "Mon Nov 10 10:00:00 2026"}, false, instanceDead},
		{"other pid", instanceState{PID: 9999, StartTime: "Mon Nov 10 10:00:00 2026"}, true, instanceDead},
		{"pid reused by a process started later", instanceState{PID: 4242, StartTime: "Mon Nov 10 09:00:00 2026"}, true, instancePIDReused},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			alive.Store(c.alive)
			if got := checkInstance(c.state, probe); got != c.want {
				t.Errorf("checkInstance(%+v) = %v, want %v", c.state, got, c.want)
			}
		})
	}
}

func TestStateFileRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".guictl", "state.json")
	want := instanceState{PID: 4242, StartTime: "Mon Nov 10 10:00:00 2026", URL: "http://127.0.0.1:54321/mcp"}
	if err := writeState(path, want); err != nil {
		t.Fatalf("writeState: %v", err)
	}
	got, err := readState(path)
	if err != nil {
		t.Fatalf("readState: %v", err)
	}
	if got != want {
		t.Errorf("readState = %+v, want %+v", got, want)
	}
}

func TestReadStateMissingFileIsNotExist(t *testing.T) {
	_, err := readState(filepath.Join(t.TempDir(), "state.json"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("readState on a missing file: err = %v, want os.ErrNotExist", err)
	}
}

func writeTestState(t *testing.T, st instanceState) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.json")
	if err := writeState(path, st); err != nil {
		t.Fatalf("writeState: %v", err)
	}
	return path
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestStartRefusesWhenTheStateFilePointsAtALiveInstance(t *testing.T) {
	var alive atomic.Bool
	alive.Store(true)
	st := instanceState{PID: 4242, StartTime: "t0", URL: "http://127.0.0.1:1/mcp"}
	path := writeTestState(t, st)
	err := ensureNoLiveInstance(path, fakeProcess(4242, "t0", &alive))
	if err == nil {
		t.Fatal("ensureNoLiveInstance: want an error while the recorded instance is alive")
	}
	if !strings.Contains(err.Error(), "4242") {
		t.Errorf("error %q does not name the running PID", err)
	}
	if !fileExists(path) {
		t.Error("the state file of a live instance was deleted")
	}
}

func TestStartRemovesAStaleStateFile(t *testing.T) {
	for _, c := range []struct {
		name      string
		startTime string
		alive     bool
	}{
		{"dead pid", "t0", false},
		{"reused pid", "t-other", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			var alive atomic.Bool
			alive.Store(c.alive)
			path := writeTestState(t, instanceState{PID: 4242, StartTime: "t0"})
			if err := ensureNoLiveInstance(path, fakeProcess(4242, c.startTime, &alive)); err != nil {
				t.Fatalf("ensureNoLiveInstance with a stale state file: %v", err)
			}
			if fileExists(path) {
				t.Error("stale state file was not removed")
			}
		})
	}
}

func TestStartWithNoStateFileIsAllowed(t *testing.T) {
	var alive atomic.Bool
	if err := ensureNoLiveInstance(filepath.Join(t.TempDir(), "state.json"), fakeProcess(1, "t0", &alive)); err != nil {
		t.Errorf("ensureNoLiveInstance with no state file: %v", err)
	}
}

func TestStopNeverSignalsAStaleInstance(t *testing.T) {
	for _, c := range []struct {
		name      string
		startTime string
		alive     bool
	}{
		{"dead pid", "t0", false},
		{"pid alive with a different start time", "t-other", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			var alive atomic.Bool
			alive.Store(c.alive)
			path := writeTestState(t, instanceState{PID: 4242, StartTime: "t0"})
			signalled := false
			terminate := func(pid int) error {
				signalled = true
				return nil
			}
			if err := stopInstance(path, fakeProcess(4242, c.startTime, &alive), terminate, terminate, time.Second); err != nil {
				t.Fatalf("stopInstance: %v", err)
			}
			if signalled {
				t.Error("stopInstance signalled a PID it could not match to the recorded start time")
			}
			if fileExists(path) {
				t.Error("stale state file was not removed")
			}
		})
	}
}

func TestStopTerminatesALiveInstanceAndWaitsForItToExit(t *testing.T) {
	var alive atomic.Bool
	alive.Store(true)
	path := writeTestState(t, instanceState{PID: 4242, StartTime: "t0"})
	var signalled []int
	terminate := func(pid int) error {
		signalled = append(signalled, pid)
		go func() {
			time.Sleep(100 * time.Millisecond)
			alive.Store(false)
		}()
		return nil
	}
	if err := stopInstance(path, fakeProcess(4242, "t0", &alive), terminate, noSignal, 5*time.Second); err != nil {
		t.Fatalf("stopInstance: %v", err)
	}
	if len(signalled) != 1 || signalled[0] != 4242 {
		t.Errorf("terminate calls = %v, want one call for 4242", signalled)
	}
	if alive.Load() {
		t.Error("stopInstance returned before the process exited")
	}
	if fileExists(path) {
		t.Error("state file still exists after the instance stopped")
	}
}

func noSignal(int) error { return nil }

func TestStopKillsAnInstanceThatIgnoresSIGTERM(t *testing.T) {
	var alive atomic.Bool
	alive.Store(true)
	path := writeTestState(t, instanceState{PID: 4242, StartTime: "t0"})
	var killed []int
	kill := func(pid int) error {
		killed = append(killed, pid)
		alive.Store(false)
		return nil
	}
	if err := stopInstance(path, fakeProcess(4242, "t0", &alive), noSignal, kill, 300*time.Millisecond); err != nil {
		t.Fatalf("stopInstance: %v", err)
	}
	if len(killed) != 1 || killed[0] != 4242 {
		t.Errorf("kill calls = %v, want one call for 4242", killed)
	}
	if fileExists(path) {
		t.Error("state file still exists after the instance was killed")
	}
}

func TestStopReportsAnInstanceThatSurvivesSIGKILL(t *testing.T) {
	var alive atomic.Bool
	alive.Store(true)
	path := writeTestState(t, instanceState{PID: 4242, StartTime: "t0"})
	start := time.Now()
	err := stopInstance(path, fakeProcess(4242, "t0", &alive), noSignal, noSignal, 300*time.Millisecond)
	if err == nil {
		t.Fatal("stopInstance: want an error when the process outlives SIGTERM and SIGKILL")
	}
	if !strings.Contains(err.Error(), "4242") {
		t.Errorf("error %q does not name the PID", err)
	}
	if elapsed := time.Since(start); elapsed > 300*time.Millisecond+killWait+time.Second {
		t.Errorf("stopInstance waited %v with a 300ms timeout", elapsed)
	}
	if !fileExists(path) {
		t.Error("state file removed while the instance is still running")
	}
}

func envValue(env []string, key string) (string, int) {
	val, n := "", 0
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && k == key {
			val = v
			n++
		}
	}
	return val, n
}

func TestChildEnvPointsXDGDirsAtAbsolutePathsUnderGuictl(t *testing.T) {
	root := t.TempDir()
	base := []string{
		"PATH=/usr/bin",
		"XDG_CONFIG_HOME=/home/owner/.config",
		"XDG_CACHE_HOME=relative/cache",
		"WAILS_MCP_HIDE_CURSOR=1",
	}
	env := childEnv(root, base, false)
	for key, sub := range map[string]string{
		"XDG_CONFIG_HOME": "config",
		"XDG_CACHE_HOME":  "cache",
		"XDG_DATA_HOME":   "data",
	} {
		val, n := envValue(env, key)
		if n != 1 {
			t.Errorf("%s appears %d times in the child env, want once", key, n)
		}
		if !filepath.IsAbs(val) {
			t.Errorf("%s = %q is not absolute; adrg/xdg ignores relative values and falls back to the owner's real directory", key, val)
		}
		if want := filepath.Join(root, ".guictl", sub); val != want {
			t.Errorf("%s = %q, want %q", key, val, want)
		}
	}
	if v, _ := envValue(env, "PATH"); v != "/usr/bin" {
		t.Errorf("PATH = %q, want the parent's value kept", v)
	}
	if v, _ := envValue(env, "WAILS_MCP_PORT"); v != "0" {
		t.Errorf("WAILS_MCP_PORT = %q, want 0 (free port)", v)
	}
	if _, n := envValue(env, "WAILS_MCP_HIDE_CURSOR"); n != 0 {
		t.Error("WAILS_MCP_HIDE_CURSOR is set without --hide-cursor; the cursor overlay is visible by default")
	}
}

func TestChildEnvHideCursorSetsTheWailsVariable(t *testing.T) {
	env := childEnv(t.TempDir(), []string{"PATH=/usr/bin"}, true)
	if v, n := envValue(env, "WAILS_MCP_HIDE_CURSOR"); v != "1" || n != 1 {
		t.Errorf("WAILS_MCP_HIDE_CURSOR = %q (%d entries), want exactly one entry with 1", v, n)
	}
}

func TestResolvePDFPathsMakesEveryPathAbsolute(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "testdata"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"testdata/minimal.pdf", "signed.pdf"} {
		if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte("%PDF-1.4\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	abs := filepath.Join(dir, "signed.pdf")
	got, err := resolvePDFPaths(dir, []string{"testdata/minimal.pdf", "./testdata/../signed.pdf", abs})
	if err != nil {
		t.Fatalf("resolvePDFPaths: %v", err)
	}
	want := []string{filepath.Join(dir, "testdata", "minimal.pdf"), abs, abs}
	if len(got) != len(want) {
		t.Fatalf("resolvePDFPaths = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("path %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestResolvePDFPathsRejectsAMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := resolvePDFPaths(dir, []string{"nope.pdf"})
	if err == nil {
		t.Fatal("resolvePDFPaths: want an error for a file that does not exist")
	}
	if !strings.Contains(err.Error(), "nope.pdf") {
		t.Errorf("error %q does not name the missing file", err)
	}
}

func TestResolvePDFPathsRejectsADirectory(t *testing.T) {
	dir := t.TempDir()
	if _, err := resolvePDFPaths(dir, []string{"."}); err == nil {
		t.Error("resolvePDFPaths: want an error for a directory")
	}
}

func TestWithDefaultWindowTargetsTheMainWindow(t *testing.T) {
	withWindow := json.RawMessage(`{"type":"object","properties":{"selector":{"type":"string"},"window":{"type":"string"}}}`)
	withoutWindow := json.RawMessage(`{"type":"object","properties":{}}`)

	got := withDefaultWindow(map[string]any{"selector": "#x"}, withWindow)
	if got["window"] != "main" {
		t.Errorf("window = %v, want main when the tool takes a window and none was given", got["window"])
	}
	if got["selector"] != "#x" {
		t.Errorf("selector = %v, want the caller's argument kept", got["selector"])
	}

	got = withDefaultWindow(map[string]any{"window": "window-1"}, withWindow)
	if got["window"] != "window-1" {
		t.Errorf("window = %v, want the caller's explicit window kept", got["window"])
	}

	got = withDefaultWindow(map[string]any{}, withoutWindow)
	if _, ok := got["window"]; ok {
		t.Error("window added to a tool whose input schema has no window property")
	}

	got = withDefaultWindow(nil, withWindow)
	if got == nil || got["window"] != "main" {
		t.Errorf("withDefaultWindow(nil, ...) = %v, want a map with window main", got)
	}
}
