// Package boot_smoke_test smoke-tests that the GUI binary builds and starts
// without panicking.
//
// Purpose: replaces the source-grep tests (Story 4-5: tests #1
// TestNativeMenuBarCreated, #3 TestMainGoSetupOrdering, #7 TestMainGoCallsSetApp)
// with a single behavioural assertion: "the boot path runs to the event loop
// without panic." If main.go's window/menu wiring or pdfservice.NewPDFService
// registration crashes, this test catches it; if a contributor refactors
// main.go's textual layout while preserving boot semantics, this test stays
// green.
//
// Non-goals: this harness does NOT directly verify PDF service correctness.
// Service correctness is covered by tests/wails-service-layer and
// tests/pdf-core-inspector. This harness only asserts the boot path does not
// panic.
//
// CI environment requirements:
//   - Linux: requires Xvfb (`apt-get install -y xvfb`); the test wraps the
//     binary with `xvfb-run -a` when DISPLAY is unset.
//   - macOS: GitHub runners ship a real WindowServer; no wrapper needed.
//   - Windows: GitHub runners ship WebView2; no wrapper needed.
//
// Per-suite go.mod isolation: this module (`boot-smoke-tests`) is intentionally
// separate from `tests/app-shell`, `tests/empty-state`, etc. The `Test acceptance
// suites (per-module)` step in `.github/workflows/ci.yml` MUST list `boot-smoke`
// in its skip-list (`case ... continue ;;`) because boot-smoke needs the
// xvfb-run wrapper that the per-module loop does not provide on Linux.
//
// Extending: keep TestAppBootsWithoutPanic as the single high-value smoke. Do
// NOT add per-feature behavioural assertions here -- those belong in the
// feature's own suite.
//
// Run: cd tests/boot-smoke && go test -count=1 ./...
package boot_smoke_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// projectRoot walks upward from the test file location to find the project
// root, identified by go.mod with module name "unidoc-pdf-debugger".
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if content, err := os.ReadFile(goModPath); err == nil {
			if strings.Contains(string(content), "module unidoc-pdf-debugger") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root (no go.mod with module unidoc-pdf-debugger found)")
		}
		dir = parent
	}
}

// platformLaunchDeadline returns the per-OS deadline for the boot probe.
// Linux is slowest because Xvfb + GTK init is heavy; Windows is medium
// because WebView2 cold-start; macOS is fastest because WKWebView is in-tree.
func platformLaunchDeadline() time.Duration {
	switch runtime.GOOS {
	case "linux":
		return 10 * time.Second
	case "windows":
		return 8 * time.Second
	case "darwin":
		return 5 * time.Second
	}
	return 10 * time.Second
}

// buildSmokeBinary builds the GUI binary into a temp directory and returns
// its absolute path. Prefers `wails3 build` (which handles platform manifests
// and icon preprocessing); falls back to `go build -tags production` if
// wails3 is not on PATH.
//
// wails3 alpha.74 writes to `bin/<APP_NAME>` with no -o flag (R3 in story
// 4-5). We copy that artifact into t.TempDir() to keep the test hermetic.
func buildSmokeBinary(t *testing.T) string {
	t.Helper()
	root := projectRoot(t)
	tmp := t.TempDir()
	appName := "unidoc-pdf-debugger"
	if runtime.GOOS == "windows" {
		appName += ".exe"
	}
	dst := filepath.Join(tmp, "pdfdebug-smoke")
	if runtime.GOOS == "windows" {
		dst += ".exe"
	}

	if path, err := exec.LookPath("wails3"); err == nil {
		t.Logf("[boot-smoke] using wails3 at %s", path)
		src := filepath.Join(root, "bin", appName)
		// Capture pre-build mtime (if any). After build we require either
		// the file to be new (stat err pre-build) or its mtime to advance,
		// otherwise we may have copied a stale artifact from a prior run
		// where wails3 exited 0 but did not actually rewrite the binary.
		var preMtime time.Time
		if info, err := os.Stat(src); err == nil {
			preMtime = info.ModTime()
		}
		// Wails alpha.93 made GTK4 + WebKitGTK 6.0 the Linux default; CI installs
		// only the GTK3 stack, so opt back into the legacy GTK3 build path here.
		// `wails3 build -tags gtk3` routes the flag through the Taskfile's
		// EXTRA_TAGS slot (becomes `-tags production,gtk3`). macOS + Windows
		// ignore the flag.
		args := []string{"build"}
		if runtime.GOOS == "linux" {
			args = append(args, "-tags", "gtk3")
		}
		cmd := exec.Command("wails3", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("[boot-smoke] wails3 build failed: %v\noutput:\n%s", err, string(out))
		}
		info, err := os.Stat(src)
		if err != nil {
			t.Fatalf("[boot-smoke] wails3 build did not produce %s: %v", src, err)
		}
		if !preMtime.IsZero() && !info.ModTime().After(preMtime) {
			t.Fatalf("[boot-smoke] wails3 build exited 0 but did not rewrite %s "+
				"(mtime unchanged: %s); refusing to run a stale artifact",
				src, info.ModTime())
		}
		if err := copyFile(src, dst); err != nil {
			t.Fatalf("[boot-smoke] copy %s -> %s: %v", src, dst, err)
		}
		if err := os.Chmod(dst, 0o755); err != nil {
			t.Fatalf("[boot-smoke] chmod %s: %v", dst, err)
		}
		return dst
	}

	// Fallback path mirrors the wails3-build tag choice: production everywhere,
	// plus gtk3 on Linux where Wails alpha.93+ otherwise tries to link GTK4.
	tags := "production"
	if runtime.GOOS == "linux" {
		tags = "production,gtk3"
	}
	t.Logf("[boot-smoke] wails3 not on PATH; falling back to `go build -tags %s`", tags)
	cmd := exec.Command("go", "build", "-tags", tags, "-o", dst, ".")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("[boot-smoke] go build fallback failed: %v\noutput:\n%s", err, string(out))
	}
	return dst
}

// copyFile copies src to dst byte-for-byte. Used to lift the wails3 build
// artifact out of the project's bin/ into a hermetic t.TempDir().
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// requireXvfbOnLinux returns the command + args used to launch the binary.
// On Linux without DISPLAY, prepends `xvfb-run -a` and fails fast if xvfb-run
// is missing. On other platforms, returns the binary path as the command.
func requireXvfbOnLinux(t *testing.T, bin string) (string, []string) {
	t.Helper()
	if runtime.GOOS != "linux" {
		return bin, nil
	}
	if os.Getenv("DISPLAY") != "" {
		return bin, nil
	}
	if _, err := exec.LookPath("xvfb-run"); err != nil {
		t.Fatalf("[boot-smoke] Linux without DISPLAY requires xvfb-run on PATH: %v\n"+
			"Install with: apt-get install -y xvfb", err)
	}
	return "xvfb-run", []string{"-a", bin}
}

// processAlive returns true if the OS still tracks the process. On Unix we
// use signal 0 (no-op delivery, but errors EPERM/ESRCH if the pid is gone).
// On Windows, syscall.Signal(0) is not portable; we check exit state instead.
func processAlive(p *os.Process) bool {
	if p == nil {
		return false
	}
	if runtime.GOOS == "windows" {
		// On Windows, syscall.Signal(0) is not a portable liveness probe.
		// This function therefore returns true unconditionally on Windows;
		// the probe loop in TestAppBootsWithoutPanic must rely on its
		// `<-exitCh` select arm (closed by the reaper goroutine when
		// cmd.Wait() returns) to detect process death.
		return true
	}
	err := p.Signal(syscall.Signal(0))
	return err == nil
}

// shutdownGracefully attempts a clean termination and waits up to 3s for
// the existing reaper goroutine (signalled via exitCh) to observe the exit.
// On Windows, GUI processes do not honour SIGTERM, so we use taskkill /T.
// The waitErr pointer is read after exitCh closes; callers must not touch
// it concurrently.
func shutdownGracefully(t *testing.T, cmd *exec.Cmd, exitCh <-chan struct{}, waitErr *error) error {
	t.Helper()
	if cmd.Process == nil {
		return nil
	}
	// If the process has already exited (early crash, ctx kill, etc.), skip
	// the signal step entirely. Sending a signal to a reaped pid races with
	// pid reuse on long-lived runners.
	select {
	case <-exitCh:
		return *waitErr
	default:
	}
	if runtime.GOOS == "windows" {
		pid := fmt.Sprintf("%d", cmd.Process.Pid)
		kill := exec.Command("taskkill", "/T", "/F", "/PID", pid)
		if out, err := kill.CombinedOutput(); err != nil {
			t.Logf("[boot-smoke] taskkill failed: %v\noutput:\n%s", err, string(out))
		}
	} else {
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Logf("[boot-smoke] SIGTERM error: %v", err)
		}
	}

	select {
	case <-exitCh:
		return *waitErr
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		<-exitCh
		return fmt.Errorf("[boot-smoke] process did not exit within 3s of SIGTERM/taskkill; SIGKILL'd")
	}
}

// scanForFatalOutput inspects the captured stdout+stderr for known
// failure-mode signatures. Empty string return = clean.
func scanForFatalOutput(s string) string {
	patterns := []struct {
		needle string
		why    string
	}{
		{"panic:", "Go panic"},
		{"runtime error:", "Go runtime error"},
		{"fatal error:", "Go fatal error"},
	}
	for _, p := range patterns {
		if strings.Contains(s, p.needle) {
			return p.why + ": found '" + p.needle + "'"
		}
	}
	// Service-init failures are emitted as `Error: <msg>` lines mentioning
	// PDF service. Catch those specifically; ignore generic `Error:` strings
	// that may appear in benign GTK/WebKit messages.
	// SplitSeq avoids allocating the full slice; matters when output is large
	// (GTK/WebKit can emit thousands of warning lines on first boot).
	for line := range strings.SplitSeq(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "Error:") &&
			strings.Contains(line, "PDF") &&
			(strings.Contains(strings.ToLower(line), "service") || strings.Contains(strings.ToLower(line), "init")) {
			return "PDF service init error: " + line
		}
	}
	return ""
}

// TestAppBootsWithoutPanic builds the GUI binary, launches it, asserts the
// process stays alive ~400ms across two consecutive 200ms probes, then sends
// SIGTERM (or taskkill on Windows) and verifies clean shutdown.
//
// This is a behavioural replacement for source-grep tests #1, #3, #7
// (Story 4-5). It does NOT verify PDF service registration directly; it
// verifies that the boot path -- including pdfservice.NewPDFService
// registration -- runs to the event loop without panicking.
func TestAppBootsWithoutPanic(t *testing.T) {
	bin := buildSmokeBinary(t)

	deadline := platformLaunchDeadline()
	ctx, cancel := context.WithTimeout(t.Context(), deadline)
	defer cancel()

	name, extraArgs := requireXvfbOnLinux(t, bin)
	cmd := exec.CommandContext(ctx, name, extraArgs...)
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined

	if err := cmd.Start(); err != nil {
		t.Fatalf("[boot-smoke] failed to start binary %s: %v", bin, err)
	}

	// Reap the process exactly once via a long-lived goroutine. exitCh
	// closes when Wait() returns, so the probe loop can detect early
	// crashes (including zombie state on Unix where signal-0 keeps
	// returning nil until the kernel reaps the child) without racing
	// shutdownGracefully, which also calls Wait().
	exitCh := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(exitCh)
	}()

	// Probe the process every 200ms. Pass when we see the process alive on
	// two consecutive probes (~400ms after first observation). 1s as a
	// single sample was too aggressive on cold runners (R5 in story 4-5).
	const probeInterval = 200 * time.Millisecond
	const probeBudget = 3 * time.Second

	consecutive := 0
	probeDeadline := time.Now().Add(probeBudget)
	alive := false
probeLoop:
	for time.Now().Before(probeDeadline) {
		select {
		case <-exitCh:
			// Process exited under its own steam before we asked it to;
			// treat as not-alive so the output scan / exit-status check
			// downstream produces a useful failure message.
			break probeLoop
		default:
		}
		if processAlive(cmd.Process) {
			consecutive++
			if consecutive >= 2 {
				alive = true
				break
			}
		} else {
			consecutive = 0
		}
		time.Sleep(probeInterval)
	}

	// Whether alive or not, attempt graceful shutdown so we drain output
	// and reap the process. shutdownGracefully no longer calls Wait()
	// itself; it waits on exitCh so the goroutine above remains the sole
	// reaper.
	shutdownErr := shutdownGracefully(t, cmd, exitCh, &waitErr)

	output := combined.String()
	if reason := scanForFatalOutput(output); reason != "" {
		t.Fatalf("[boot-smoke] fatal output detected: %s\n--- captured output ---\n%s",
			reason, output)
	}

	if !alive {
		t.Fatalf("[boot-smoke] process did not stay alive across two consecutive probes (~400ms).\n"+
			"--- captured output ---\n%s", output)
	}

	// Accept exit code 0 or signal-induced exit (-1 on Unix when killed).
	// Reject any other non-zero exit, since that indicates the binary
	// crashed AFTER passing the alive probes.
	//
	// On Windows there is no `Signaled()` concept and `taskkill /F`
	// (which we always use to terminate) calls `TerminateProcess(h, 1)`,
	// so the exit code is non-zero by design. Any ExitError on Windows
	// after we initiated shutdown is therefore expected; panics and
	// runtime errors are caught above by `scanForFatalOutput`.
	if shutdownErr != nil {
		if exitErr, ok := errors.AsType[*exec.ExitError](shutdownErr); ok {
			if runtime.GOOS == "windows" {
				return
			}
			ws, ok := exitErr.Sys().(syscall.WaitStatus)
			if ok && (ws.Signaled() || ws.ExitStatus() == 0) {
				return
			}
			t.Fatalf("[boot-smoke] non-zero clean exit after SIGTERM: %v\n"+
				"--- captured output ---\n%s", shutdownErr, output)
		}
		// Non-ExitError failures (e.g. shutdown timeout) are already logged
		// above; re-raise as a hard failure with captured output.
		t.Fatalf("[boot-smoke] shutdown failure: %v\n--- captured output ---\n%s",
			shutdownErr, output)
	}
}
