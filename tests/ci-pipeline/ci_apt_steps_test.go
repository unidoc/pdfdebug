package ci_pipeline_test

import (
	"strings"
	"testing"
)

// namedStep returns the build-and-test step with the given name.
func namedStep(t *testing.T, name string) map[string]interface{} {
	t.Helper()
	for _, s := range jobSteps(t) {
		m, ok := s.(map[string]interface{})
		if ok && m["name"] == name {
			return m
		}
	}
	t.Fatalf("ci.yml: build-and-test has no step named %q", name)
	return nil
}

func stepTimeout(t *testing.T, step map[string]interface{}) int {
	t.Helper()
	v, ok := step["timeout-minutes"].(int)
	if !ok {
		t.Fatalf("ci.yml: step %q has no integer timeout-minutes (got %v)", step["name"], step["timeout-minutes"])
	}
	return v
}

// aptLines returns the run lines of step that call apt-get.
func aptLines(step map[string]interface{}) []string {
	run, _ := step["run"].(string)
	var out []string
	for l := range strings.SplitSeq(run, "\n") {
		if strings.Contains(l, "apt-get") {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

// The native deps step installs gdb with the other packages, bounds each apt
// call with retries and a connection timeout, and has its own step timeout, so
// a stalled mirror fails the step instead of the whole job.
func TestCIWorkflowNativeDepsStepInstallsGdbAndBoundsApt(t *testing.T) {
	step := namedStep(t, "Install Linux native deps")
	lines := aptLines(step)
	var update, install string
	for _, l := range lines {
		switch {
		case strings.Contains(l, "apt-get update"):
			update = l
		case strings.Contains(l, "apt-get install"):
			install = l
		}
	}
	if update == "" || install == "" {
		t.Fatalf("native deps step must run apt-get update and apt-get install, got %q", lines)
	}
	if !strings.Contains(" "+install+" ", " gdb ") {
		t.Errorf("native deps apt-get install must include gdb: %q", install)
	}
	for _, l := range []string{update, install} {
		for _, opt := range []string{"-o Acquire::Retries=3", "-o Acquire::http::Timeout=30"} {
			if !strings.Contains(l, opt) {
				t.Errorf("apt call %q must pass %q", l, opt)
			}
		}
	}
	if got := stepTimeout(t, step); got != 35 {
		t.Errorf("native deps step timeout-minutes = %d, want 35", got)
	}
}

// Boot-smoke runs only the smoke test inside its 5-minute step timeout; it
// installs nothing.
func TestCIWorkflowBootSmokeStepInstallsNothing(t *testing.T) {
	step := namedStep(t, "Boot-smoke")
	if lines := aptLines(step); len(lines) != 0 {
		t.Errorf("Boot-smoke must not call apt-get, found %q", lines)
	}
	if got := stepTimeout(t, step); got != 5 {
		t.Errorf("Boot-smoke timeout-minutes = %d, want 5", got)
	}
}
