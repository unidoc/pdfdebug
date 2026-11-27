//go:build !windows

package guilaunch

import (
	"bufio"
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// startGroup runs script in its own process group and returns once it has
// printed "ready", so traps are installed before a signal arrives.
func startGroup(t *testing.T, script string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sh", "-c", script)
	SetProcessGroup(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = cmd.Wait() }()
	t.Cleanup(func() { _ = signalGroup(cmd.Process.Pid, 9) })
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "ready" {
		t.Fatalf("group did not start: %q, %v", line, err)
	}
	return cmd
}

func TestStopEndsAGroupThatExitsOnSIGTERM(t *testing.T) {
	cmd := startGroup(t, "echo ready; exec sleep 30")
	killed, err := ProcessGroup.Stop(cmd.Process.Pid, 5*time.Second, time.Second)
	if err != nil || killed {
		t.Errorf("Stop = %v, %v; want stopped by SIGTERM", killed, err)
	}
	if groupAlive(cmd.Process.Pid) {
		t.Error("group still alive after Stop")
	}
}

func TestStopKillsAGroupThatIgnoresSIGTERM(t *testing.T) {
	cmd := startGroup(t, "trap '' TERM; echo ready; while :; do sleep 0.1; done")
	killed, err := ProcessGroup.Stop(cmd.Process.Pid, 300*time.Millisecond, 2*time.Second)
	if err != nil || !killed {
		t.Errorf("Stop = %v, %v; want stopped by SIGKILL", killed, err)
	}
	if groupAlive(cmd.Process.Pid) {
		t.Error("group still alive after Stop")
	}
}

func TestStopWaitsForAHelperThatOutlivesTheLeader(t *testing.T) {
	// The leader exits on SIGTERM; its background helper ignores SIGTERM and
	// keeps the group alive until SIGKILL.
	cmd := startGroup(t, "(trap '' TERM; echo ready; while :; do sleep 0.1; done) & wait")
	killed, err := ProcessGroup.Stop(cmd.Process.Pid, 500*time.Millisecond, 2*time.Second)
	if err != nil || !killed {
		t.Errorf("Stop = %v, %v; want the helper killed", killed, err)
	}
	if groupAlive(cmd.Process.Pid) {
		t.Error("helper still alive after Stop")
	}
}

func TestStopOfAGoneGroupIsNotAnError(t *testing.T) {
	cmd := exec.Command("true")
	SetProcessGroup(cmd)
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if _, err := ProcessGroup.Stop(cmd.Process.Pid, time.Second, time.Second); err != nil {
		t.Errorf("Stop of an exited group = %v", err)
	}
}

func TestLaunchSecondReportsExitAndTimeout(t *testing.T) {
	if out, err := LaunchSecond(context.Background(), "sh", t.TempDir(), nil, []string{"-c", "echo handed off"}); err != nil || strings.TrimSpace(string(out)) != "handed off" {
		t.Errorf("LaunchSecond of a handoff that exits 0 = %q, %v", out, err)
	}
	out, err := LaunchSecond(context.Background(), "sh", t.TempDir(), nil, []string{"-c", "echo crashed; exit 3"})
	if err == nil || errors.Is(err, ErrLaunchTimeout) || !strings.Contains(string(out), "crashed") {
		t.Errorf("LaunchSecond of a handoff that exits 3 = %q, %v; want the exit error and its output", out, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	pidFile := t.TempDir() + "/pid"
	_, err = LaunchSecond(ctx, "sh", t.TempDir(), nil, []string{"-c", "sleep 30 & echo $! > " + pidFile + "; wait"})
	if !errors.Is(err, ErrLaunchTimeout) {
		t.Fatalf("LaunchSecond of a hung handoff = %v, want ErrLaunchTimeout", err)
	}
	data, readErr := exec.Command("cat", pidFile).Output()
	if readErr != nil {
		t.Fatal(readErr)
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data)))
	if pid > 0 && exec.Command("kill", "-0", strconv.Itoa(pid)).Run() == nil {
		t.Errorf("child %d of the hung handoff is still running", pid)
	}
}
