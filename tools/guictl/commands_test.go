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

func TestFindURLReadsTheMCPLineFromTheAppLog(t *testing.T) {
	w := testWorkspace(t)
	if url, ok, err := w.findURL(); err != nil || ok || url != "" {
		t.Errorf("findURL with no log = %q, %v, %v; want not found and no error", url, ok, err)
	}

	writeLog(t, w, "10:42AM INF Build Info: Wails=v3.0.0", "10:42AM INF AssetServer Info: middleware=true")
	if _, ok, err := w.findURL(); err != nil || ok {
		t.Errorf("findURL before the MCP line = %v, %v; want not found and no error", ok, err)
	}

	writeLog(t, w,
		"10:42AM INF Build Info: Wails=v3.0.0",
		"10:42AM INF MCP server started. Connect MCP clients using the streamable HTTP transport. url=http://127.0.0.1:54321/mcp",
		"10:42AM INF [AssetFileServerFS] Handling request url=/ file=.",
	)
	url, ok, err := w.findURL()
	if err != nil || !ok || url != "http://127.0.0.1:54321/mcp" {
		t.Errorf("findURL = %q, %v, %v; want the MCP server URL", url, ok, err)
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
	full := w.logPath() + ".full"
	if err := os.WriteFile(full, []byte("10:42AM INF MCP server started. Connect MCP clients using the streamable HTTP transport. url=http://127.0.0.1:9099/mcp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The rename swaps the whole log in at once, so findURL never reads a
	// partly written line.
	renamed := make(chan error, 1)
	go func() {
		time.Sleep(200 * time.Millisecond)
		renamed <- os.Rename(full, w.logPath())
	}()
	url, err := w.waitForURL(make(chan error))
	if err != nil || url != "http://127.0.0.1:9099/mcp" {
		t.Errorf("waitForURL = %q, %v; want the URL written after the call started", url, err)
	}
	if err := <-renamed; err != nil {
		t.Fatal(err)
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
	if args["window"] != "main" || args["selector"] != `[data-testid="empty-state"]` {
		t.Errorf("dom_query arguments = %v, want the empty-state selector on the main window", args)
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
