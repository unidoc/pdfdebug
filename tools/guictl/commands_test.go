//go:build !windows

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"unidoc-pdf-debugger/internal/guilaunch"
	"unidoc-pdf-debugger/internal/mcpclient"
)

// toolServer answers tools/call like the Wails MCP server, with the text and
// isError that reply returns, and records each call's arguments.
type toolServer struct {
	mu    sync.Mutex
	calls []map[string]any
	reply func(n int, name string, args map[string]any) (text string, isError bool)
}

func (s *toolServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var req struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		} `json:"params"`
	}
	if err := json.Unmarshal(raw, &req); err != nil || req.Method != "tools/call" {
		http.Error(w, "unexpected request", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	s.calls = append(s.calls, req.Params.Arguments)
	n := len(s.calls)
	s.mu.Unlock()
	text, isError := s.reply(n, req.Params.Name, req.Params.Arguments)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      req.ID,
		"result": map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
			"isError": isError,
		},
	})
}

func (s *toolServer) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func newToolServer(t *testing.T, reply func(n int, name string, args map[string]any) (string, bool)) (*toolServer, string) {
	t.Helper()
	s := &toolServer{reply: reply}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	return s, srv.URL + "/mcp"
}

func testWorkspace(t *testing.T) workspace {
	t.Helper()
	root := t.TempDir()
	return workspace{root: root, dir: filepath.Join(root, ".guictl")}
}

func writeLog(t *testing.T, w workspace, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w.logPath(), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunRejectsMissingOrExtraArguments(t *testing.T) {
	for _, args := range [][]string{
		nil,
		{"open"},
		{"pick"},
		{"call"},
		{"call", "dom_query", "{}", "extra"},
		{"restart"},
	} {
		t.Run(strings.Join(append([]string{"guictl"}, args...), " "), func(t *testing.T) {
			err := run(args)
			if err == nil || !strings.HasPrefix(err.Error(), "usage: guictl") {
				t.Errorf("run(%q) = %v, want the usage error", args, err)
			}
		})
	}
}

func appendLog(t *testing.T, w workspace, text string) {
	t.Helper()
	f, err := os.OpenFile(w.logPath(), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

const mcpLine = "10:42AM INF MCP server started. Connect MCP clients using the streamable HTTP transport. url=http://127.0.0.1:54321/mcp"

func TestLogFollowerReadsTheMCPLineAsTheLogGrows(t *testing.T) {
	w := testWorkspace(t)
	log := &logFollower{path: w.logPath()}
	defer log.close()
	if url, ok, err := log.next(); err != nil || ok || url != "" {
		t.Errorf("next with no log = %q, %v, %v; want not found and no error", url, ok, err)
	}

	writeLog(t, w, "10:42AM INF Build Info: Wails=v3.0.0", "10:42AM INF AssetServer Info: middleware=true")
	if _, ok, err := log.next(); err != nil || ok {
		t.Errorf("next before the MCP line = %v, %v; want not found and no error", ok, err)
	}

	// A line split across two writes is only parsed once it is complete.
	appendLog(t, w, mcpLine[:40])
	if _, ok, err := log.next(); err != nil || ok {
		t.Errorf("next on half a line = %v, %v; want not found and no error", ok, err)
	}
	appendLog(t, w, mcpLine[40:]+"\n10:42AM INF [AssetFileServerFS] Handling request url=/ file=.\n")
	url, ok, err := log.next()
	if err != nil || !ok || url != "http://127.0.0.1:54321/mcp" {
		t.Errorf("next = %q, %v, %v; want the MCP server URL", url, ok, err)
	}
}

func TestLogFollowerReadsOnlyWhatWasAppended(t *testing.T) {
	w := testWorkspace(t)
	writeLog(t, w, "10:42AM INF Build Info: Wails=v3.0.0")
	log := &logFollower{path: w.logPath()}
	defer log.close()
	if _, ok, err := log.next(); err != nil || ok {
		t.Fatalf("next = %v, %v", ok, err)
	}
	// Overwriting the start of the file in place is not seen: the follower
	// continues from its offset.
	f, err := os.OpenFile(w.logPath(), os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte(mcpLine[:30]), 0); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if _, ok, err := log.next(); err != nil || ok {
		t.Errorf("next after rewriting already-read bytes = %v, %v; want nothing new", ok, err)
	}
}

func TestLogFollowerRejectsAnOverlongLine(t *testing.T) {
	w := testWorkspace(t)
	writeLog(t, w)
	appendLog(t, w, strings.Repeat("x", maxLogLine+1))
	log := &logFollower{path: w.logPath()}
	defer log.close()
	if _, _, err := log.next(); err == nil || !strings.Contains(err.Error(), "longer than") {
		t.Errorf("next on an overlong line = %v, want an error", err)
	}
}

func TestWaitForURLReportsAChildThatExitsBeforeTheServerStarts(t *testing.T) {
	w := testWorkspace(t)
	writeLog(t, w, "10:42AM INF Build Info: Wails=v3.0.0", "single instance lock held")
	exitErr := exec.Command("sh", "-c", "exit 3").Run()
	var ee *exec.ExitError
	if !errors.As(exitErr, &ee) {
		t.Fatalf("sh -c 'exit 3' returned %v, want an exit error", exitErr)
	}
	exited := make(chan error, 1)
	exited <- exitErr

	start := time.Now()
	_, err := w.waitForURL(exited)
	if err == nil {
		t.Fatal("waitForURL: want an error when the child exits without printing the MCP URL")
	}
	for _, want := range []string{"exited with code 3", "single-instance lock", "single instance lock held"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("waitForURL took %v to notice the exit", elapsed)
	}
}

func TestWaitForURLReturnsOnceTheLogHasTheURL(t *testing.T) {
	w := testWorkspace(t)
	writeLog(t, w, "10:42AM INF Build Info: Wails=v3.0.0")
	go func() {
		time.Sleep(200 * time.Millisecond)
		appendLog(t, w, "10:42AM INF MCP server started. Connect MCP clients using the streamable HTTP transport. url=http://127.0.0.1:9099/mcp\n")
	}()
	url, err := w.waitForURL(make(chan error))
	if err != nil || url != "http://127.0.0.1:9099/mcp" {
		t.Errorf("waitForURL = %q, %v; want the URL written after the call started", url, err)
	}
}

func TestLogTailKeepsTheLastLines(t *testing.T) {
	w := testWorkspace(t)
	if got := w.logTail(); got != "" {
		t.Errorf("logTail with no log = %q, want empty", got)
	}
	var lines []string
	for i := 1; i <= logTailLines+5; i++ {
		lines = append(lines, fmt.Sprintf("line-%02d", i))
	}
	writeLog(t, w, lines...)
	got := w.logTail()
	if !strings.HasPrefix(got, "last lines of "+w.logPath()+":\n") {
		t.Errorf("logTail does not name the log file: %q", got)
	}
	if strings.Contains(got, "line-05") || !strings.Contains(got, "line-06") || !strings.HasSuffix(got, "line-25") {
		t.Errorf("logTail = %q, want lines 06 to 25 only", got)
	}
}

func TestWaitForMainWindowReturnsOnceTheEmptyStateRenders(t *testing.T) {
	srv, url := newToolServer(t, func(n int, name string, args map[string]any) (string, bool) {
		if n < 3 {
			return `{"count":0,"elements":[]}`, false
		}
		return `{"count":1,"elements":[{"visible":true}]}`, false
	})
	if err := waitForMainWindow(url, make(chan error)); err != nil {
		t.Fatalf("waitForMainWindow: %v", err)
	}
	if n := srv.callCount(); n != 3 {
		t.Errorf("dom_query calls = %d, want 3 (two empty answers, then the empty state)", n)
	}
	srv.mu.Lock()
	args := srv.calls[0]
	srv.mu.Unlock()
	if args["window"] != "main" || args["selector"] != guilaunch.ReadySelector {
		t.Errorf("dom_query arguments = %v, want the app-shell selector on the main window", args)
	}
}

func TestWaitForMainWindowKeepsPollingThroughToolErrors(t *testing.T) {
	_, url := newToolServer(t, func(n int, name string, args map[string]any) (string, bool) {
		if n == 1 {
			return "no window named \"main\"", true
		}
		return `{"count":1}`, false
	})
	if err := waitForMainWindow(url, make(chan error)); err != nil {
		t.Errorf("waitForMainWindow after one tool error: %v", err)
	}
}

func TestWaitForMainWindowStopsWhenTheChildExits(t *testing.T) {
	_, url := newToolServer(t, func(int, string, map[string]any) (string, bool) {
		return `{"count":0}`, false
	})
	exited := make(chan error, 1)
	exited <- errors.New("signal: killed")
	start := time.Now()
	err := waitForMainWindow(url, exited)
	if err == nil || !strings.Contains(err.Error(), "exited before the main window rendered") || !strings.Contains(err.Error(), "signal: killed") {
		t.Errorf("waitForMainWindow = %v, want the exit reported with its cause", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("waitForMainWindow took %v to notice the exit", elapsed)
	}
}

func TestCallAndPrintFailsOnAToolError(t *testing.T) {
	_, url := newToolServer(t, func(_ int, name string, _ map[string]any) (string, bool) {
		return "no element matches selector", name == "mouse_click"
	})
	client := mcpclient.New(url)
	if err := callAndPrint(client, "dom_query", map[string]any{"selector": "#x"}); err != nil {
		t.Errorf("callAndPrint for a tool that succeeded: %v", err)
	}
	err := callAndPrint(client, "mouse_click", map[string]any{"selector": "#x"})
	if err == nil || !strings.Contains(err.Error(), "mouse_click reported an error") {
		t.Errorf("callAndPrint for a tool error = %v, want an error naming the tool", err)
	}
}

func TestLiveStateNamesWhyNoInstanceIsUsable(t *testing.T) {
	w := testWorkspace(t)
	if _, err := w.liveState(); err == nil || !strings.Contains(err.Error(), "no instance running") {
		t.Errorf("liveState with no state file = %v, want a no-instance error", err)
	}
	if err := writeState(w.statePath(), instanceState{PID: 0, StartTime: "t0"}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.liveState(); err == nil || !strings.Contains(err.Error(), "guictl stop") {
		t.Errorf("liveState with a dead PID = %v, want an error pointing at guictl stop", err)
	}
	if err := os.WriteFile(w.statePath(), []byte(`{"pid": 12`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := w.liveState(); err == nil || !strings.Contains(err.Error(), "guictl stop") {
		t.Errorf("liveState with a truncated state file = %v, want an error pointing at guictl stop", err)
	}
	for _, cmd := range []func() error{
		func() error { return w.open([]string{"x.pdf"}) },
		func() error { return w.pick([]string{"x.pdf"}) },
		w.tools,
		func() error { return w.call("dom_query", "{}") },
	} {
		if err := cmd(); err == nil || !strings.Contains(err.Error(), "guictl stop") {
			t.Errorf("command against a dead instance = %v, want the liveState error", err)
		}
	}
}

func TestStopRemovesAnUnreadableStateFile(t *testing.T) {
	w := testWorkspace(t)
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w.statePath(), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := w.stop(); err != nil {
		t.Fatalf("stop with an unreadable state file: %v", err)
	}
	if fileExists(w.statePath()) {
		t.Error("unreadable state file was not removed")
	}
}

func TestStopAndStatusWithNoStateFileSucceed(t *testing.T) {
	w := testWorkspace(t)
	if err := w.stop(); err != nil {
		t.Errorf("stop with no state file: %v", err)
	}
	if err := w.status(); err != nil {
		t.Errorf("status with no state file: %v", err)
	}
}

func TestStartTreatsAnUnreadableStateFileAsStale(t *testing.T) {
	w := testWorkspace(t)
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(w.statePath(), []byte(`{"pid": 12`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureNoLiveInstance(w.statePath(), probeProcess); err != nil {
		t.Errorf("ensureNoLiveInstance with a truncated state file = %v, want it cleared", err)
	}
	if fileExists(w.statePath()) {
		t.Error("truncated state file was not removed")
	}
}

// liveTestState records this test process as the instance, so liveState
// accepts it.
func liveTestState(t *testing.T, w workspace, url string, windowTools []string) {
	t.Helper()
	startTime, alive := probeProcess(os.Getpid())
	if !alive {
		t.Fatal("probeProcess does not see the test process")
	}
	st := instanceState{PID: os.Getpid(), StartTime: startTime, URL: url, WindowTools: windowTools}
	if err := writeState(w.statePath(), st); err != nil {
		t.Fatal(err)
	}
}

func TestCallUsesTheRecordedWindowToolsWithoutListingTools(t *testing.T) {
	srv, url := newToolServer(t, func(int, string, map[string]any) (string, bool) {
		return `{"count":1}`, false
	})
	w := testWorkspace(t)
	liveTestState(t, w, url, []string{"dom_query"})
	if err := w.call("dom_query", `{"selector":"#x"}`); err != nil {
		t.Fatalf("call dom_query: %v", err)
	}
	if err := w.call("app_info", `{}`); err != nil {
		t.Fatalf("call app_info: %v", err)
	}
	// The fake server answers tools/call only, so a tools/list request would
	// have failed the call.
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.calls) != 2 {
		t.Fatalf("tool calls = %d, want 2", len(srv.calls))
	}
	if srv.calls[0]["window"] != "main" {
		t.Errorf("dom_query arguments = %v, want window main added", srv.calls[0])
	}
	if _, ok := srv.calls[1]["window"]; ok {
		t.Errorf("app_info arguments = %v, want no window for a tool that takes none", srv.calls[1])
	}
}

func TestStopUnrecordedStopsTheChildAndReturnsTheStartFailure(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	guilaunch.SetProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	reaped := make(chan struct{})
	go func() { _ = cmd.Wait(); close(reaped) }()
	cause := errors.New("no MCP server URL")
	if err := stopUnrecorded(cmd.Process.Pid, cause); !errors.Is(err, cause) {
		t.Errorf("stopUnrecorded = %v, want the start failure returned", err)
	}
	select {
	case <-reaped:
	case <-time.After(5 * time.Second):
		t.Fatal("child still running after stopUnrecorded")
	}
}
