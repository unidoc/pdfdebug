package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"unidoc-pdf-debugger/internal/guilaunch"
	"unidoc-pdf-debugger/internal/mcpclient"
)

// instanceState is what .guictl/state.json records about the running
// pdfdebug-mcp instance.
type instanceState struct {
	PID       int    `json:"pid"`
	StartTime string `json:"startTime"`
	URL       string `json:"url"`
	// WindowTools names the MCP tools whose input takes a window, so call can
	// target the main window without listing the tools first.
	WindowTools []string `json:"windowTools,omitempty"`
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
// removes a stale or unreadable one.
func ensureNoLiveInstance(statePath string, probe processProbe) error {
	st, err := readState(statePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		// An unreadable state file names no process, so it cannot block a start.
		return os.Remove(statePath)
	}
	if checkInstance(st, probe) == instanceLive {
		return fmt.Errorf("an instance is already running (pid %d, %s); run guictl stop first", st.PID, st.URL)
	}
	return os.Remove(statePath)
}

// stopInstance stops the recorded instance's process group and removes the
// state file, and returns the status it found. A state file whose process is
// dead or whose PID now belongs to another process is removed without
// signalling anything.
func stopInstance(statePath string, probe processProbe, group guilaunch.Group, timeout time.Duration) (instanceStatus, error) {
	st, err := readState(statePath)
	if err != nil {
		return instanceDead, err
	}
	s := checkInstance(st, probe)
	if s != instanceLive {
		return s, os.Remove(statePath)
	}
	if _, err := group.Stop(st.PID, timeout, killWait); err != nil {
		return s, err
	}
	return s, os.Remove(statePath)
}

// killWait is how long a stop waits for the process group to exit after
// SIGKILL.
const killWait = 2 * time.Second

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

// windowTools returns the names of the tools whose input schema has a window
// property.
func windowTools(tools []mcpclient.Tool) []string {
	var names []string
	for _, t := range tools {
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if json.Unmarshal(t.InputSchema, &schema) != nil {
			continue
		}
		if _, ok := schema.Properties["window"]; ok {
			names = append(names, t.Name)
		}
	}
	return names
}

// withDefaultWindow returns a copy of args with window "main" added when the
// tool takes a window and the caller gave none. The splash window would
// otherwise be picked while it is up.
func withDefaultWindow(args map[string]any, takesWindow bool) map[string]any {
	out := make(map[string]any, len(args)+1)
	for k, v := range args {
		out[k] = v
	}
	if _, given := out["window"]; takesWindow && !given {
		out["window"] = guilaunch.MainWindow
	}
	return out
}
