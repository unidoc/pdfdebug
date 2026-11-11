// Package guilaunch holds what tools/guictl and the tests/gui-mcp suite share
// to launch a pdfdebug-mcp instance and reach its main window: the MCP server
// log line, the child environment, the readiness check, the repo root lookup,
// the second-launch handoff and process-group shutdown.
package guilaunch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"unidoc-pdf-debugger/internal/mcpclient"
)

// MainWindow is the name of the app's main window. MCP calls target it by
// name, because with no window argument the server picks the focused window,
// which during start-up is the always-on-top splash.
const MainWindow = "main"

// ReadySelector matches what the main window renders once the frontend has
// mounted: the empty state, or the tab bar when documents are already open.
const ReadySelector = `[data-testid="empty-state"], [data-testid="tab-bar"]`

// ParseMCPURL returns the url attribute of the Wails "MCP server started" log
// line.
func ParseMCPURL(line string) (string, bool) {
	if !strings.Contains(line, "MCP server started") {
		return "", false
	}
	_, rest, ok := strings.Cut(line, "url=")
	if !ok {
		return "", false
	}
	if i := strings.IndexAny(rest, " \t"); i >= 0 {
		rest = rest[:i]
	}
	return rest, rest != ""
}

// Env is base with the XDG directories pointed at config, cache and data under
// xdgRoot, a free MCP port, and the cursor overlay hidden only when asked.
// xdgRoot must be absolute: adrg/xdg ignores a relative XDG value and falls
// back to the user's real directory. An inherited WAILS_MCP_HOST or
// WAILS_MCP_TOKEN is dropped so the server binds to loopback.
func Env(base []string, xdgRoot string, hideCursor bool) []string {
	override := []string{
		"XDG_CONFIG_HOME=" + filepath.Join(xdgRoot, "config"),
		"XDG_CACHE_HOME=" + filepath.Join(xdgRoot, "cache"),
		"XDG_DATA_HOME=" + filepath.Join(xdgRoot, "data"),
		"WAILS_MCP_PORT=0",
	}
	if hideCursor {
		override = append(override, "WAILS_MCP_HIDE_CURSOR=1")
	}
	drop := map[string]bool{
		"XDG_CONFIG_HOME": true, "XDG_CACHE_HOME": true, "XDG_DATA_HOME": true,
		"WAILS_MCP_PORT": true, "WAILS_MCP_HIDE_CURSOR": true,
		"WAILS_MCP_HOST": true, "WAILS_MCP_TOKEN": true,
	}
	env := make([]string, 0, len(base)+len(override))
	for _, kv := range base {
		k, _, _ := strings.Cut(kv, "=")
		if !drop[k] {
			env = append(env, kv)
		}
	}
	return append(env, override...)
}

// FindRepoRoot walks up from dir to the directory whose go.mod declares
// module unidoc-pdf-debugger.
func FindRepoRoot(dir string) (string, error) {
	for {
		if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.TrimSpace(line) == "module unidoc-pdf-debugger" {
					return dir, nil
				}
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("not inside the unidoc-pdf-debugger repo (no go.mod with module unidoc-pdf-debugger)")
		}
		dir = parent
	}
}

// MainWindowRendered reports whether the main window shows ReadySelector. The
// MCP server answers before any window has loaded, so a reachable server is
// not enough.
func MainWindowRendered(ctx context.Context, c *mcpclient.Client) bool {
	res, err := c.CallTool(ctx, "dom_query", map[string]any{"selector": ReadySelector, "window": MainWindow})
	if err != nil || res.IsError {
		return false
	}
	var q struct {
		Count int `json:"count"`
	}
	return json.Unmarshal([]byte(res.Text), &q) == nil && q.Count > 0
}

// Group signals a process group by its leader's PID and reports whether any
// member is left. ProcessGroup is the real one; tests supply fakes.
type Group struct {
	Terminate func(pid int) error
	Kill      func(pid int) error
	Alive     func(pid int) bool
}

// Stop sends SIGTERM to the group led by pid and waits up to timeout for every
// member to exit, then sends SIGKILL and waits up to killWait more. killed
// reports whether SIGKILL was needed. The wait covers the whole group, so a
// WebKit or D-Bus helper that outlives the app is killed too.
func (g Group) Stop(pid int, timeout, killWait time.Duration) (killed bool, err error) {
	if err := g.Terminate(pid); err != nil {
		return false, fmt.Errorf("terminate process group %d: %w", pid, err)
	}
	if g.waitGone(pid, timeout) {
		return false, nil
	}
	if err := g.Kill(pid); err != nil {
		return true, fmt.Errorf("kill process group %d: %w", pid, err)
	}
	if g.waitGone(pid, killWait) {
		return true, nil
	}
	return true, fmt.Errorf("process group %d still running after SIGTERM and SIGKILL", pid)
}

func (g Group) waitGone(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for g.Alive(pid) {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
	return true
}

// ErrLaunchTimeout is returned by LaunchSecond when the handoff process does
// not exit before its context ends.
var ErrLaunchTimeout = errors.New("second launch did not exit in time")
