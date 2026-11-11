package ci_pipeline_test

import (
	"regexp"
	"strings"
	"testing"
)

// stepsRunning returns the build-and-test steps whose run body contains needle.
func stepsRunning(t *testing.T, needle string) []map[string]interface{} {
	t.Helper()
	var out []map[string]interface{}
	for _, s := range jobSteps(t) {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		if run, _ := m["run"].(string); strings.Contains(run, needle) {
			out = append(out, m)
		}
	}
	return out
}

// runsOnEveryLeg reports whether a step has no `if` that limits it to one OS.
func runsOnEveryLeg(step map[string]interface{}) bool {
	cond, _ := step["if"].(string)
	return !strings.Contains(cond, "runner.os") && !strings.Contains(cond, "matrix.os")
}

func TestCIWorkflowVetsTheMCPTaggedCodeOnEveryLeg(t *testing.T) {
	steps := stepsRunning(t, "go vet -tags mcp,guimcp ./...")
	if len(steps) == 0 {
		t.Fatal("ci.yml never runs `go vet -tags mcp,guimcp ./...`")
	}
	for _, s := range steps {
		if !runsOnEveryLeg(s) {
			t.Errorf("step %q vets the mcp-tagged code on one OS only (if: %v); it must run on all three legs", s["name"], s["if"])
		}
	}
}

func TestCIWorkflowRunsTheMCPTaggedUnitTestsOnEveryLeg(t *testing.T) {
	steps := stepsRunning(t, "go test -tags mcp -run 'DialogQueue|InstanceIdentity' ./internal/pdfservice/ .")
	if len(steps) == 0 {
		t.Fatal("ci.yml never runs `go test -tags mcp -run 'DialogQueue|InstanceIdentity' ./internal/pdfservice/ .`")
	}
	for _, s := range steps {
		if !runsOnEveryLeg(s) {
			t.Errorf("step %q runs the mcp-tagged unit tests on one OS only (if: %v)", s["name"], s["if"])
		}
	}
}

func TestCIWorkflowRunsTheGUISuiteOnLinuxAfterBootSmoke(t *testing.T) {
	boot := stepIndexByPredicate(t, func(m map[string]interface{}) bool { return m["name"] == "Boot-smoke" })
	if boot < 0 {
		t.Fatal("ci.yml has no Boot-smoke step")
	}
	gui := stepIndexByPredicate(t, func(m map[string]interface{}) bool {
		run, _ := m["run"].(string)
		return strings.Contains(run, "./tests/gui-mcp/")
	})
	if gui < 0 {
		t.Fatal("ci.yml has no step that runs ./tests/gui-mcp/")
	}
	if gui < boot {
		t.Errorf("the GUI suite step (%d) runs before Boot-smoke (%d)", gui, boot)
	}
	step := jobSteps(t)[gui].(map[string]interface{})
	if cond, _ := step["if"].(string); !strings.Contains(cond, "Linux") {
		t.Errorf("GUI suite step if = %q, want it limited to the Linux leg", cond)
	}
	if _, ok := step["timeout-minutes"]; !ok {
		t.Error("GUI suite step has no timeout-minutes of its own")
	}
	run, _ := step["run"].(string)
	for _, want := range []string{
		"wails3 task build:mcp",
		"apparmor_parser -r",
		"xvfb-run -a dbus-run-session --",
		"-tags guimcp",
	} {
		if !strings.Contains(run, want) {
			t.Errorf("GUI suite step does not run %q", want)
		}
	}
	for _, banned := range []string{
		"WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS",
		"apparmor_restrict_unprivileged_userns=0",
	} {
		if strings.Contains(run, banned) {
			t.Errorf("GUI suite step uses %q; it must load an AppArmor profile instead of disabling the sandbox", banned)
		}
	}
	m := regexp.MustCompile(`profile\s+([\w.-]+)`).FindStringSubmatch(run)
	if m == nil {
		t.Fatal("GUI suite step writes no AppArmor profile for bin/pdfdebug-mcp")
	}
	if m[1] == "unidoc-pdf-debugger" {
		t.Error("GUI suite step names its AppArmor profile unidoc-pdf-debugger, which replaces the packaged profile")
	}
	if !strings.Contains(run, "bin/pdfdebug-mcp") {
		t.Error("GUI suite step's AppArmor profile does not cover bin/pdfdebug-mcp")
	}
}

func TestCIWorkflowPerSuiteLoopSkipsOnlyBootSmoke(t *testing.T) {
	step := stepIndexByPredicate(t, func(m map[string]interface{}) bool {
		return m["name"] == "Test acceptance suites (per-module)"
	})
	if step < 0 {
		t.Fatal("ci.yml has no per-module acceptance suite step")
	}
	run, _ := jobSteps(t)[step].(map[string]interface{})["run"].(string)
	m := regexp.MustCompile(`(?m)^\s*([\w|-]+)\)\s*continue\s*;;`).FindStringSubmatch(run)
	if m == nil {
		t.Fatal("per-module loop has no skip case")
	}
	if m[1] != "boot-smoke" {
		t.Errorf("per-module loop skips %q, want boot-smoke only", m[1])
	}
}
