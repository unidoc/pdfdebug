package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// instanceState is what .guictl/state.json records about the running
// pdfdebug-mcp instance.
type instanceState struct {
	PID       int    `json:"pid"`
	StartTime string `json:"startTime"`
	URL       string `json:"url"`
}

// processProbe reports whether pid is alive and, if so, its start time.
type processProbe func(pid int) (startTime string, alive bool)

// instanceStatus classifies a state file against the process table.
type instanceStatus int

const (
	instanceLive instanceStatus = iota
	instanceDead
	instancePIDReused
)

// String names the status for messages.
func (s instanceStatus) String() string {
	switch s {
	case instanceLive:
		return "live"
	case instanceDead:
		return "dead"
	case instancePIDReused:
		return "pid reused"
	}
	return fmt.Sprintf("instanceStatus(%d)", int(s))
}

// checkInstance reports whether the recorded process is still the one guictl
// started: alive with the recorded start time.
func checkInstance(st instanceState, probe processProbe) instanceStatus {
	startTime, alive := probe(st.PID)
	if !alive {
		return instanceDead
	}
	if startTime != st.StartTime {
		return instancePIDReused
	}
	return instanceLive
}

func writeState(path string, st instanceState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// readState returns the recorded state; a missing file is os.ErrNotExist.
func readState(path string) (instanceState, error) {
	var st instanceState
	data, err := os.ReadFile(path)
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return st, fmt.Errorf("%s: %w", path, err)
	}
	return st, nil
}

// ensureNoLiveInstance fails when the state file points at a live instance and
// removes a stale one.
func ensureNoLiveInstance(statePath string, probe processProbe) error {
	st, err := readState(statePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if checkInstance(st, probe) == instanceLive {
		return fmt.Errorf("an instance is already running (pid %d, %s); run guictl stop first", st.PID, st.URL)
	}
	return os.Remove(statePath)
}

// stopInstance terminates the recorded instance and waits up to timeout for it
// to exit. A state file whose process is dead or whose PID now belongs to
// another process is removed without signalling anything.
func stopInstance(statePath string, probe processProbe, terminate func(pid int) error, timeout time.Duration) error {
	st, err := readState(statePath)
	if err != nil {
		return err
	}
	if checkInstance(st, probe) != instanceLive {
		return os.Remove(statePath)
	}
	if err := terminate(st.PID); err != nil {
		return fmt.Errorf("terminate pid %d: %w", st.PID, err)
	}
	deadline := time.Now().Add(timeout)
	for checkInstance(st, probe) == instanceLive {
		if time.Now().After(deadline) {
			return fmt.Errorf("pid %d still running %v after SIGTERM", st.PID, timeout)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return os.Remove(statePath)
}

// parseMCPURL returns the url attribute of the Wails "MCP server started" log
// line.
func parseMCPURL(line string) (string, bool) {
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

// childEnv is base with the XDG directories pointed at absolute paths under
// <repoRoot>/.guictl, a free MCP port, and the cursor overlay hidden only when
// asked. An inherited WAILS_MCP_HOST or WAILS_MCP_TOKEN is dropped so the
// server binds to loopback.
func childEnv(repoRoot string, base []string, hideCursor bool) []string {
	dir := filepath.Join(repoRoot, ".guictl")
	override := []string{
		"XDG_CONFIG_HOME=" + filepath.Join(dir, "config"),
		"XDG_CACHE_HOME=" + filepath.Join(dir, "cache"),
		"XDG_DATA_HOME=" + filepath.Join(dir, "data"),
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

// resolvePDFPaths makes each path absolute against cwd and checks it is an
// existing regular file. The running instance ignores the caller's working
// directory, so only absolute paths reach it.
func resolvePDFPaths(cwd string, paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		abs := p
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, abs)
		}
		abs = filepath.Clean(abs)
		info, err := os.Stat(abs)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("%s is a directory", p)
		}
		out = append(out, abs)
	}
	return out, nil
}

// withDefaultWindow returns a copy of args with window "main" added when the
// tool's input schema has a window property and the caller gave none. The
// splash window would otherwise be picked while it is up.
func withDefaultWindow(args map[string]any, inputSchema json.RawMessage) map[string]any {
	out := make(map[string]any, len(args)+1)
	for k, v := range args {
		out[k] = v
	}
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if json.Unmarshal(inputSchema, &schema) != nil {
		return out
	}
	if _, takesWindow := schema.Properties["window"]; takesWindow {
		if _, given := out["window"]; !given {
			out["window"] = mainWindow
		}
	}
	return out
}
