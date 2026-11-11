package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"unidoc-pdf-debugger/internal/guilaunch"
	"unidoc-pdf-debugger/internal/mcpclient"
)

// fakeProcess reports a process table with one entry.
func fakeProcess(pid int, startTime string, alive *atomic.Bool) processProbe {
	return func(p int) (string, bool) {
		if p != pid || !alive.Load() {
			return "", false
		}
		return startTime, true
	}
}

// fakeGroup is a process group that is alive while alive holds; terminate and
// kill record their PID and run the matching hook.
type fakeGroup struct {
	alive               *atomic.Bool
	terminated, killed  []int
	onTerminate, onKill func()
}

func (f *fakeGroup) group() guilaunch.Group {
	return guilaunch.Group{
		Terminate: func(pid int) error {
			f.terminated = append(f.terminated, pid)
			if f.onTerminate != nil {
				f.onTerminate()
			}
			return nil
		},
		Kill: func(pid int) error {
			f.killed = append(f.killed, pid)
			if f.onKill != nil {
				f.onKill()
			}
			return nil
		},
		Alive: func(int) bool { return f.alive.Load() },
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
	want := instanceState{PID: 4242, StartTime: "Mon Nov 10 10:00:00 2026", URL: "http://127.0.0.1:54321/mcp", WindowTools: []string{"dom_query", "js_eval"}}
	if err := writeState(path, want); err != nil {
		t.Fatalf("writeState: %v", err)
	}
	got, err := readState(path)
	if err != nil {
		t.Fatalf("readState: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
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
			g := &fakeGroup{alive: &alive}
			s, err := stopInstance(path, fakeProcess(4242, c.startTime, &alive), g.group(), time.Second)
			if err != nil {
				t.Fatalf("stopInstance: %v", err)
			}
			if s == instanceLive {
				t.Errorf("stopInstance status = %v, want the stale status it acted on", s)
			}
			if len(g.terminated)+len(g.killed) > 0 {
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
	g := &fakeGroup{alive: &alive, onTerminate: func() {
		go func() {
			time.Sleep(100 * time.Millisecond)
			alive.Store(false)
		}()
	}}
	s, err := stopInstance(path, fakeProcess(4242, "t0", &alive), g.group(), 5*time.Second)
	if err != nil {
		t.Fatalf("stopInstance: %v", err)
	}
	if s != instanceLive {
		t.Errorf("stopInstance status = %v, want live", s)
	}
	if len(g.terminated) != 1 || g.terminated[0] != 4242 || len(g.killed) != 0 {
		t.Errorf("terminate calls = %v, kill calls = %v; want one SIGTERM for 4242 and no SIGKILL", g.terminated, g.killed)
	}
	if alive.Load() {
		t.Error("stopInstance returned before the process exited")
	}
	if fileExists(path) {
		t.Error("state file still exists after the instance stopped")
	}
}

func TestStopKillsAnInstanceThatIgnoresSIGTERM(t *testing.T) {
	var alive atomic.Bool
	alive.Store(true)
	path := writeTestState(t, instanceState{PID: 4242, StartTime: "t0"})
	g := &fakeGroup{alive: &alive, onKill: func() { alive.Store(false) }}
	if _, err := stopInstance(path, fakeProcess(4242, "t0", &alive), g.group(), 300*time.Millisecond); err != nil {
		t.Fatalf("stopInstance: %v", err)
	}
	if len(g.killed) != 1 || g.killed[0] != 4242 {
		t.Errorf("kill calls = %v, want one call for 4242", g.killed)
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
	g := &fakeGroup{alive: &alive}
	_, err := stopInstance(path, fakeProcess(4242, "t0", &alive), g.group(), 300*time.Millisecond)
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

func TestWindowToolsNamesTheToolsThatTakeAWindow(t *testing.T) {
	tools := []mcpclient.Tool{
		{Name: "dom_query", InputSchema: json.RawMessage(`{"type":"object","properties":{"selector":{"type":"string"},"window":{"type":"string"}}}`)},
		{Name: "app_info", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)},
		{Name: "broken", InputSchema: json.RawMessage(`not json`)},
	}
	if got := windowTools(tools); len(got) != 1 || got[0] != "dom_query" {
		t.Errorf("windowTools = %v, want [dom_query]", got)
	}
}

func TestWithDefaultWindowTargetsTheMainWindow(t *testing.T) {
	const withWindow, withoutWindow = true, false

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
