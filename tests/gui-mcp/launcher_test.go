//go:build guimcp && !windows

// Package guimcp_test drives a pdfdebug-mcp build of the desktop app through
// the Wails MCP server: it launches the binary, opens committed fixtures and
// checks what the main window shows.
//
// Build the binary first, then run the suite from the repo root:
//
//	wails3 task build:mcp
//	go test -tags guimcp -count=1 ./tests/gui-mcp/
//
// PDFDEBUG_MCP_BINARY overrides the binary path (default bin/pdfdebug-mcp).
// Every build with the mcp tag shares one single-instance lock, so tests run
// one at a time and a running guictl instance makes the launch fail.
package guimcp_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"unidoc-pdf-debugger/internal/mcpclient"
)

const (
	mainWindow     = "main"
	launchDeadline = 90 * time.Second
	waitDeadline   = 30 * time.Second
	stopDeadline   = 15 * time.Second
	pollInterval   = 200 * time.Millisecond
)

var mcpURLPattern = regexp.MustCompile(`MCP server started.*\burl=(http://\S+/mcp)`)

// repoRoot walks up from the test directory to the root go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil && strings.Contains(string(data), "module unidoc-pdf-debugger") {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find the repo root (go.mod with module unidoc-pdf-debugger)")
		}
		dir = parent
	}
}

// fixture returns the absolute path of a committed testdata file.
func fixture(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(repoRoot(t), "testdata", name)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return p
}

func binaryPath(t *testing.T) string {
	t.Helper()
	p := os.Getenv("PDFDEBUG_MCP_BINARY")
	if p == "" {
		p = filepath.Join(repoRoot(t), "bin", "pdfdebug-mcp")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatalf("resolve %s: %v", p, err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("pdfdebug-mcp binary not found at %s; build it with `wails3 task build:mcp` or set PDFDEBUG_MCP_BINARY", abs)
	}
	return abs
}

// appEnv is the parent environment with per-test XDG directories and the MCP
// settings the suite needs. Every value is absolute.
func appEnv(t *testing.T) []string {
	t.Helper()
	base := t.TempDir()
	override := map[string]string{
		"XDG_CONFIG_HOME":       filepath.Join(base, "config"),
		"XDG_CACHE_HOME":        filepath.Join(base, "cache"),
		"XDG_DATA_HOME":         filepath.Join(base, "data"),
		"WAILS_MCP_PORT":        "0",
		"WAILS_MCP_HIDE_CURSOR": "1",
	}
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if _, replaced := override[k]; replaced || k == "WAILS_MCP_HOST" || k == "WAILS_MCP_TOKEN" {
			continue
		}
		env = append(env, kv)
	}
	for k, v := range override {
		env = append(env, k+"="+v)
	}
	return env
}

// lockedBuffer collects the child's output for failure messages.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) tail(n int) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	lines := strings.Split(strings.TrimRight(b.buf.String(), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// app is one running pdfdebug-mcp instance.
type app struct {
	t       *testing.T
	bin     string
	env     []string
	cmd     *exec.Cmd
	log     *lockedBuffer
	done    chan struct{}
	waitErr error
	client  *mcpclient.Client
}

// launch starts pdfdebug-mcp with the given files on its command line, waits
// until the main window answers, and stops it when the test ends.
func launch(t *testing.T, files ...string) *app {
	t.Helper()
	a := &app{t: t, bin: binaryPath(t), env: appEnv(t), log: &lockedBuffer{}, done: make(chan struct{})}

	a.cmd = exec.Command(a.bin, files...)
	a.cmd.Env = a.env
	a.cmd.Dir = repoRoot(t)
	a.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	a.cmd.Stdout = a.log
	stderr, err := a.cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	if err := a.cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", a.bin, err)
	}

	urls := make(chan string, 1)
	scanned := make(chan struct{})
	go func() {
		defer close(scanned)
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		sent := false
		for sc.Scan() {
			line := sc.Text()
			_, _ = fmt.Fprintln(a.log, line)
			if m := mcpURLPattern.FindStringSubmatch(line); m != nil && !sent {
				urls <- m[1]
				sent = true
			}
		}
		_, _ = io.Copy(a.log, stderr)
	}()
	go func() {
		<-scanned
		a.waitErr = a.cmd.Wait()
		close(a.done)
	}()
	t.Cleanup(a.stop)

	var url string
	select {
	case url = <-urls:
	case <-a.done:
		t.Fatalf("pdfdebug-mcp exited before the MCP server started (%v); another pdfdebug-mcp instance may hold the single-instance lock, stop it with `go run ./tools/guictl stop`.\nlast output:\n%s", a.waitErr, a.log.tail(20))
	case <-time.After(launchDeadline):
		t.Fatalf("no MCP server URL on stderr after %v.\nlast output:\n%s", launchDeadline, a.log.tail(20))
	}

	a.client = mcpclient.New(url)
	a.poll("MCP initialize to succeed", launchDeadline, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return a.client.Initialize(ctx) == nil
	})
	a.poll("the main window to render the app shell", launchDeadline, func() bool {
		n, err := a.tryCount(`[data-testid="empty-state"], [data-testid="tab-bar"]`)
		return err == nil && n > 0
	})
	return a
}

// stop sends SIGTERM to the process group and waits for the process to exit,
// so the next test can take the single-instance lock.
func (a *app) stop() {
	select {
	case <-a.done:
		return
	default:
	}
	_ = syscall.Kill(-a.cmd.Process.Pid, syscall.SIGTERM)
	select {
	case <-a.done:
		return
	case <-time.After(stopDeadline):
	}
	_ = syscall.Kill(-a.cmd.Process.Pid, syscall.SIGKILL)
	select {
	case <-a.done:
		a.t.Errorf("pdfdebug-mcp did not exit within %v of SIGTERM and was killed", stopDeadline)
	case <-time.After(stopDeadline):
		a.t.Errorf("pdfdebug-mcp (pid %d) did not exit after SIGKILL", a.cmd.Process.Pid)
	}
}

// call runs one MCP tool against the main window.
func (a *app) call(name string, args map[string]any) (mcpclient.Result, error) {
	if args == nil {
		args = map[string]any{}
	}
	args["window"] = mainWindow
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	res, err := a.client.CallTool(ctx, name, args)
	if err != nil {
		return res, err
	}
	if res.IsError {
		return res, fmt.Errorf("%s: %s", name, res.Text)
	}
	return res, nil
}

func (a *app) mustCall(name string, args map[string]any) string {
	a.t.Helper()
	res, err := a.call(name, args)
	if err != nil {
		a.t.Fatalf("%s %v: %v\nlast app output:\n%s", name, args, err, a.log.tail(20))
	}
	return res.Text
}

// js evaluates an async function body in the main window and decodes its
// JSON result into out. The body's value is serialised in the page, because
// js_eval returns a string result as bare text and anything else as JSON.
func (a *app) js(body string, out any) error {
	wrapped := "return JSON.stringify(await (async () => {\n" + body + "\n})());"
	res, err := a.call("js_eval", map[string]any{"js": wrapped})
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal([]byte(res.Text), out)
}

func (a *app) mustJS(body string, out any) {
	a.t.Helper()
	if err := a.js(body, out); err != nil {
		a.t.Fatalf("js_eval: %v\n%s", err, body)
	}
}

type queryResult struct {
	Count    int `json:"count"`
	Elements []struct {
		Text    string `json:"text"`
		Visible bool   `json:"visible"`
	} `json:"elements"`
}

func (a *app) query(selector string) (queryResult, error) {
	var q queryResult
	res, err := a.call("dom_query", map[string]any{"selector": selector})
	if err != nil {
		return q, err
	}
	err = json.Unmarshal([]byte(res.Text), &q)
	return q, err
}

func (a *app) tryCount(selector string) (int, error) {
	q, err := a.query(selector)
	return q.Count, err
}

func (a *app) count(selector string) int {
	a.t.Helper()
	n, err := a.tryCount(selector)
	if err != nil {
		a.t.Fatalf("dom_query %s: %v", selector, err)
	}
	return n
}

func (a *app) visible(selector string) bool {
	q, err := a.query(selector)
	if err != nil {
		return false
	}
	for _, el := range q.Elements {
		if el.Visible {
			return true
		}
	}
	return false
}

// clickJS fires the pointer and mouse events of one left click on the element
// under (x, y), in the order Wails' mouse_click fires them, focusing the
// target on mousedown. It runs synchronously: Wails' mouse_click awaits
// setTimeout and requestAnimationFrame for its cursor animation, which WebKit
// throttles while the window is not frontmost and visible, and the call can
// then outlast its 30 s deadline.
const clickJS = `const el = document.elementFromPoint(x, y);
	if (!el) return {error: 'no element at ' + x + ',' + y};
	const base = {bubbles: true, cancelable: true, composed: true, view: window, clientX: x, clientY: y, button: 0, detail: 1};
	const pointer = {pointerId: 1, pointerType: 'mouse', isPrimary: true};
	el.dispatchEvent(new PointerEvent('pointerdown', {...base, ...pointer, buttons: 1}));
	if (el.dispatchEvent(new MouseEvent('mousedown', {...base, buttons: 1}))) {
		const f = el.closest('input, textarea, select, button, a[href], [tabindex], [contenteditable]');
		if (f) f.focus();
	}
	el.dispatchEvent(new PointerEvent('pointerup', {...base, ...pointer, buttons: 0}));
	el.dispatchEvent(new MouseEvent('mouseup', {...base, buttons: 0}));
	el.dispatchEvent(new MouseEvent('click', {...base, buttons: 0}));
	return {error: ''};`

func (a *app) dispatchClick(setup string) {
	a.t.Helper()
	var res struct {
		Error string `json:"error"`
	}
	a.mustJS(setup+"\n"+clickJS, &res)
	if res.Error != "" {
		a.t.Fatalf("click: %s", res.Error)
	}
}

// click clicks the centre of the first element matching selector after
// scrolling it into view.
func (a *app) click(selector string) {
	a.t.Helper()
	sel, _ := json.Marshal(selector)
	a.dispatchClick(`const target = document.querySelector(` + string(sel) + `);
	if (!target) return {error: 'no element matches selector ' + ` + string(sel) + `};
	target.scrollIntoView({block: 'center', inline: 'center'});
	const r = target.getBoundingClientRect();
	if (r.width === 0 && r.height === 0) return {error: 'element has zero size: ' + ` + string(sel) + `};
	const x = r.x + r.width / 2, y = r.y + r.height / 2;`)
}

// clickAt clicks the viewport point (x, y) in CSS pixels.
func (a *app) clickAt(x, y float64) {
	a.t.Helper()
	a.dispatchClick(fmt.Sprintf("const x = %g, y = %g;", x, y))
}

// poll checks cond until it holds or the deadline passes.
func (a *app) poll(what string, deadline time.Duration, cond func() bool) {
	a.t.Helper()
	end := time.Now().Add(deadline)
	for {
		if cond() {
			return
		}
		select {
		case <-a.done:
			a.t.Fatalf("pdfdebug-mcp exited while waiting for %s (%v).\nlast output:\n%s", what, a.waitErr, a.log.tail(20))
		default:
		}
		if time.Now().After(end) {
			a.t.Fatalf("timed out after %v waiting for %s.\nlast app output:\n%s", deadline, what, a.log.tail(20))
		}
		time.Sleep(pollInterval)
	}
}

func (a *app) waitFor(what string, cond func() bool) {
	a.t.Helper()
	a.poll(what, waitDeadline, cond)
}

// openInRunningInstance launches pdfdebug-mcp a second time with the same
// environment and absolute file paths, so the files reach the running window
// through the single-instance handoff. The handoff exits 0, so any other exit
// is a failure.
func (a *app) openInRunningInstance(files ...string) {
	a.t.Helper()
	for _, f := range files {
		if !filepath.IsAbs(f) {
			a.t.Fatalf("second-launch path %q is not absolute; the running instance ignores the caller's working directory", f)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	cmd := exec.CommandContext(ctx, a.bin, files...)
	cmd.Env = a.env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// Bounds the wait for output pipes that a surviving grandchild holds open
	// after the timeout kills the launch process.
	cmd.WaitDelay = 5 * time.Second
	out, err := cmd.CombinedOutput()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		a.t.Fatalf("second launch did not exit within %v.\noutput:\n%s", waitDeadline, out)
	}
	if err != nil {
		a.t.Fatalf("second launch: %v\noutput:\n%s", err, out)
	}
}

// showStructurePanel selects the Structure rail entry and opens the left
// panel. The collapsed flag persists in localStorage, which every
// pdfdebug-mcp run shares, so a fresh instance can start with it collapsed.
// A keyboard-style click (detail 0) selects and opens, never collapses.
func (a *app) showStructurePanel() {
	a.t.Helper()
	a.waitFor("the Structure panel to open", func() bool {
		var open bool
		err := a.js(`const b = document.querySelector('[aria-label="Navigators"] [role="tab"][aria-label="Structure"]');
			if (!b) return false;
			const open = b.getAttribute('aria-selected') === 'true' && b.getAttribute('aria-expanded') === 'true';
			if (!open) b.click();
			return open;`, &open)
		return err == nil && open
	})
}

// tabNames returns the file names on the document tab bar, in order.
func (a *app) tabNames() []string {
	var names []string
	if err := a.js(`return Array.from(document.querySelectorAll('[data-testid="tab-list"] [role="tab"]')).map(el => (el.innerText || '').trim());`, &names); err != nil {
		return nil
	}
	return names
}

// activeTabName returns the file name on the selected document tab.
func (a *app) activeTabName() string {
	var name string
	if err := a.js(`const el = document.querySelector('[data-testid="tab-list"] [role="tab"][data-state="active"]'); return el ? (el.innerText || '').trim() : '';`, &name); err != nil {
		return ""
	}
	return name
}

func containsName(names []string, want string) bool {
	for _, n := range names {
		if strings.Contains(n, want) {
			return true
		}
	}
	return false
}

// openFixtures opens fixtures through a second launch, waits for a tab per
// file and shows the Structure panel.
func (a *app) openFixtures(names ...string) {
	a.t.Helper()
	var paths []string
	for _, n := range names {
		paths = append(paths, fixture(a.t, n))
	}
	a.openInRunningInstance(paths...)
	for _, n := range names {
		a.waitFor("a document tab for "+n, func() bool { return containsName(a.tabNames(), n) })
	}
	a.showStructurePanel()
	a.waitFor("the object tree to render", func() bool {
		n, err := a.tryCount(`[data-testid="tree-node"]`)
		return err == nil && n > 0
	})
}
