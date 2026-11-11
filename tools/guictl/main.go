// Command guictl builds, launches and drives a pdfdebug-mcp instance of the
// desktop app through its Wails MCP server. Run it from the repo root with
// go run ./tools/guictl <command>. State, the app log and the instance's
// XDG directories live under .guictl/.
//
// Commands:
//
//	start [--hide-cursor] [file.pdf...]  build, launch, wait for the main window, open files
//	open file.pdf...                     open files in the running instance (second launch)
//	pick file.pdf...                     queue paths for the next file dialog
//	tools                                list the MCP tools
//	call <tool> ['<json args>']          call one MCP tool
//	status                               PID, URL, uptime and the log tail
//	stop                                 SIGTERM the instance's process group and wait
//
// macOS and Linux only.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"unidoc-pdf-debugger/internal/mcpclient"
)

const (
	mainWindow    = "main"
	launchTimeout = 90 * time.Second
	callTimeout   = 60 * time.Second
	stopTimeout   = 15 * time.Second
	logTailLines  = 20

	queueDialogMethod = "unidoc-pdf-debugger/internal/pdfservice.PDFService.QueueDialogPaths"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "guictl:", err)
		os.Exit(1)
	}
}

func usage() error {
	return errors.New("usage: guictl start [--hide-cursor] [file.pdf...] | open file.pdf... | pick file.pdf... | tools | call <tool> ['<json>'] | status | stop")
}

func run(args []string) error {
	if err := platformSupported(); err != nil {
		return err
	}
	if len(args) == 0 {
		return usage()
	}
	root, err := findRepoRoot()
	if err != nil {
		return err
	}
	w := workspace{root: root, dir: filepath.Join(root, ".guictl")}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "start":
		fs := flag.NewFlagSet("start", flag.ContinueOnError)
		hide := fs.Bool("hide-cursor", false, "hide the MCP cursor overlay (WAILS_MCP_HIDE_CURSOR=1)")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		return w.start(*hide, fs.Args())
	case "open":
		if len(rest) == 0 {
			return usage()
		}
		return w.open(rest)
	case "pick":
		if len(rest) == 0 {
			return usage()
		}
		return w.pick(rest)
	case "tools":
		return w.tools()
	case "call":
		if len(rest) < 1 || len(rest) > 2 {
			return usage()
		}
		argsJSON := "{}"
		if len(rest) == 2 {
			argsJSON = rest[1]
		}
		return w.call(rest[0], argsJSON)
	case "status":
		return w.status()
	case "stop":
		return w.stop()
	}
	return usage()
}

// findRepoRoot walks up from the working directory to the root go.mod.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
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
			return "", errors.New("run guictl inside the unidoc-pdf-debugger repo")
		}
		dir = parent
	}
}

// workspace holds the repo root and its .guictl directory.
type workspace struct {
	root string
	dir  string
}

func (w workspace) statePath() string { return filepath.Join(w.dir, "state.json") }
func (w workspace) logPath() string   { return filepath.Join(w.dir, "app.log") }
func (w workspace) binary() string    { return filepath.Join(w.root, "bin", "pdfdebug-mcp") }

func (w workspace) start(hideCursor bool, files []string) error {
	if err := ensureNoLiveInstance(w.statePath(), probeProcess); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	paths, err := resolvePDFPaths(cwd, files)
	if err != nil {
		return err
	}

	build := exec.Command("wails3", "task", "build:mcp")
	build.Dir = w.root
	build.Stdout = os.Stderr
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("wails3 task build:mcp: %w", err)
	}

	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return err
	}
	logFile, err := os.Create(w.logPath())
	if err != nil {
		return err
	}
	defer func() { _ = logFile.Close() }()

	child := exec.Command(w.binary())
	child.Dir = w.root
	child.Env = childEnv(w.root, os.Environ(), hideCursor)
	child.Stdout = logFile
	child.Stderr = logFile
	setProcessGroup(child)
	if err := child.Start(); err != nil {
		return fmt.Errorf("start %s: %w", w.binary(), err)
	}
	exited := make(chan error, 1)
	go func() { exited <- child.Wait() }()

	// Until the state file is written, stop cannot reach the child, so every
	// failure before that terminates its process group.
	url, err := w.waitForURL(exited)
	if err != nil {
		_ = terminateGroup(child.Process.Pid)
		return err
	}
	startTime, alive := probeProcess(child.Process.Pid)
	if !alive {
		_ = terminateGroup(child.Process.Pid)
		return fmt.Errorf("pdfdebug-mcp (pid %d) exited after the MCP server started\n%s", child.Process.Pid, w.logTail())
	}
	st := instanceState{PID: child.Process.Pid, StartTime: startTime, URL: url}
	if err := writeState(w.statePath(), st); err != nil {
		_ = terminateGroup(child.Process.Pid)
		return err
	}
	if err := waitForMainWindow(url, exited); err != nil {
		// An instance whose window never rendered is not usable by open, pick
		// or call, so it is stopped and its state removed.
		tail := w.logTail()
		if stopErr := stopInstance(w.statePath(), probeProcess, terminateGroup, killGroup, stopTimeout); stopErr != nil {
			return fmt.Errorf("%w; stopping it failed: %v\n%s", err, stopErr, tail)
		}
		return fmt.Errorf("%w; the instance was stopped\n%s", err, tail)
	}
	fmt.Printf("started pid %d %s\n", st.PID, st.URL)
	if len(paths) > 0 {
		return w.launchSecond(paths)
	}
	return nil
}

// waitForURL polls the app log for the MCP server line until the child exits
// or the launch timeout passes.
func (w workspace) waitForURL(exited <-chan error) (string, error) {
	deadline := time.Now().Add(launchTimeout)
	for {
		url, ok, err := w.findURL()
		if err != nil {
			return "", err
		}
		if ok {
			return url, nil
		}
		select {
		case err := <-exited:
			code := 0
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				code = exitErr.ExitCode()
			} else if err != nil {
				return "", fmt.Errorf("wait for pdfdebug-mcp: %w\n%s", err, w.logTail())
			}
			return "", fmt.Errorf("pdfdebug-mcp exited with code %d before the MCP server started; another pdfdebug-mcp instance may hold the single-instance lock (guictl stop, or quit WAILS_MCP=1 wails3 dev)\n%s", code, w.logTail())
		default:
		}
		if time.Now().After(deadline) {
			return "", fmt.Errorf("no MCP server URL in %s after %v\n%s", w.logPath(), launchTimeout, w.logTail())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// findURL scans the app log for the MCP server URL. A log that does not
// exist yet is not an error.
func (w workspace) findURL() (string, bool, error) {
	f, err := os.Open(w.logPath())
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		if url, ok := parseMCPURL(sc.Text()); ok {
			return url, true, nil
		}
	}
	if err := sc.Err(); err != nil {
		return "", false, fmt.Errorf("read %s: %w", w.logPath(), err)
	}
	return "", false, nil
}

func (w workspace) logTail() string {
	data, err := os.ReadFile(w.logPath())
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > logTailLines {
		lines = lines[len(lines)-logTailLines:]
	}
	return "last lines of " + w.logPath() + ":\n" + strings.Join(lines, "\n")
}

// waitForMainWindow polls dom_query on the main window until the empty state
// renders. The MCP server answers before any window has loaded.
func waitForMainWindow(url string, exited <-chan error) error {
	client := mcpclient.New(url)
	deadline := time.Now().Add(launchTimeout)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		res, err := client.CallTool(ctx, "dom_query", map[string]any{"selector": `[data-testid="empty-state"]`, "window": mainWindow})
		cancel()
		if err == nil && !res.IsError {
			var q struct {
				Count int `json:"count"`
			}
			if json.Unmarshal([]byte(res.Text), &q) == nil && q.Count > 0 {
				return nil
			}
		}
		select {
		case err := <-exited:
			return fmt.Errorf("pdfdebug-mcp exited before the main window rendered (%v)", err)
		default:
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("main window did not render the empty state within %v", launchTimeout)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// liveState returns the state of a running instance or an error saying none
// is running.
func (w workspace) liveState() (instanceState, error) {
	st, err := readState(w.statePath())
	if errors.Is(err, os.ErrNotExist) {
		return st, errors.New("no instance running; run guictl start")
	}
	if err != nil {
		return st, err
	}
	if s := checkInstance(st, probeProcess); s != instanceLive {
		return st, fmt.Errorf("recorded instance pid %d is %v; run guictl stop to clear it, then guictl start", st.PID, s)
	}
	return st, nil
}

func (w workspace) open(files []string) error {
	if _, err := w.liveState(); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	paths, err := resolvePDFPaths(cwd, files)
	if err != nil {
		return err
	}
	return w.launchSecond(paths)
}

// launchSecond runs pdfdebug-mcp again with absolute paths, so they reach the
// running instance through the single-instance handoff, and waits for the
// handoff process to exit. The handoff exits 0, so any other exit code is a
// failed launch.
func (w workspace) launchSecond(paths []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, w.binary(), paths...)
	cmd.Dir = w.root
	cmd.Env = childEnv(w.root, os.Environ(), false)
	setProcessGroup(cmd)
	// Bounds the wait for output pipes that a surviving grandchild holds open
	// after the timeout kills the launch process.
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("second launch did not exit within %v\n%s", callTimeout, out)
	}
	if err != nil {
		return fmt.Errorf("second launch: %w\n%s", err, out)
	}
	fmt.Printf("sent %d file(s) to the running instance\n", len(paths))
	return nil
}

func (w workspace) pick(files []string) error {
	st, err := w.liveState()
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	paths, err := resolvePDFPaths(cwd, files)
	if err != nil {
		return err
	}
	anyPaths := make([]any, len(paths))
	for i, p := range paths {
		anyPaths[i] = p
	}
	return callAndPrint(mcpclient.New(st.URL), "call_bound_method", map[string]any{
		"name":   queueDialogMethod,
		"args":   []any{anyPaths},
		"window": mainWindow,
	})
}

func (w workspace) tools() error {
	st, err := w.liveState()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	begin := time.Now()
	tools, err := mcpclient.New(st.URL).ListTools(ctx)
	if err != nil {
		fmt.Printf("tools/list {} error %v\n", time.Since(begin).Round(time.Millisecond))
		return err
	}
	fmt.Printf("tools/list {} ok %v\n", time.Since(begin).Round(time.Millisecond))
	for _, t := range tools {
		fmt.Printf("%s\t%s\n\t%s\n", t.Name, t.Description, t.InputSchema)
	}
	return nil
}

func (w workspace) call(tool, argsJSON string) error {
	st, err := w.liveState()
	if err != nil {
		return err
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	client := mcpclient.New(st.URL)
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	tools, err := client.ListTools(ctx)
	if err != nil {
		return err
	}
	for _, t := range tools {
		if t.Name == tool {
			args = withDefaultWindow(args, t.InputSchema)
		}
	}
	return callAndPrint(client, tool, args)
}

// callAndPrint prints "tool args ok|error duration", then the result text. A
// tool error is returned so the command exits non-zero.
func callAndPrint(client *mcpclient.Client, tool string, args map[string]any) error {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(args)
	argsJSON := strings.TrimSpace(buf.String())
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	begin := time.Now()
	res, err := client.CallTool(ctx, tool, args)
	elapsed := time.Since(begin).Round(time.Millisecond)
	if err != nil {
		fmt.Printf("%s %s error %v\n", tool, argsJSON, elapsed)
		return err
	}
	outcome := "ok"
	if res.IsError {
		outcome = "error"
	}
	fmt.Printf("%s %s %s %v\n%s\n", tool, argsJSON, outcome, elapsed, res.Text)
	if res.IsError {
		return fmt.Errorf("%s reported an error", tool)
	}
	return nil
}

func (w workspace) status() error {
	st, err := readState(w.statePath())
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println("no instance recorded")
		return nil
	}
	if err != nil {
		return err
	}
	s := checkInstance(st, probeProcess)
	fmt.Printf("pid %d (%v)\nurl %s\n", st.PID, s, st.URL)
	if started, err := time.ParseInLocation(psTimeLayout, st.StartTime, time.Local); err == nil && s == instanceLive {
		fmt.Printf("uptime %v\n", time.Since(started).Round(time.Second))
	}
	fmt.Println(w.logTail())
	return nil
}

func (w workspace) stop() error {
	st, err := readState(w.statePath())
	if errors.Is(err, os.ErrNotExist) {
		fmt.Println("no instance recorded")
		return nil
	}
	if err != nil {
		// An unreadable state file names no process to signal.
		if rmErr := os.Remove(w.statePath()); rmErr != nil {
			return rmErr
		}
		fmt.Printf("removed unreadable state file (%v); nothing signalled\n", err)
		return nil
	}
	s := checkInstance(st, probeProcess)
	if err := stopInstance(w.statePath(), probeProcess, terminateGroup, killGroup, stopTimeout); err != nil {
		return err
	}
	if s == instanceLive {
		fmt.Printf("stopped pid %d\n", st.PID)
	} else {
		fmt.Printf("removed stale state for pid %d (%v); nothing signalled\n", st.PID, s)
	}
	return nil
}
