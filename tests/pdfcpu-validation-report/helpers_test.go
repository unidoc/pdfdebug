// Package pdfcpu_validation_report_test checks the pdfcpu validation report:
// `validate --profile pdfcpu` exit codes, plain text and JSON on the in-repo
// fixtures, the open path that never runs pdfcpu's validator, and the docs and
// changelog lines that describe the profile.
//
// The pdfcore and cmd/cli unit tests for the check live next to the code and
// are run again from here (colocated_unit_test.go). The CLI-versus-GUI binding
// comparison lives in cmd/cli, since this module cannot import internal
// packages.
//
// Run: cd tests/pdfcpu-validation-report && go test -count=1 ./...
package pdfcpu_validation_report_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

const profilePdfcpu = "pdfcpu"

func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			if strings.Contains(string(data), "module unidoc-pdf-debugger") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root (no go.mod with module unidoc-pdf-debugger)")
		}
		dir = parent
	}
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(projectRoot(t), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(projectRoot(t), "testdata", name)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("fixture %s missing: %v", name, err)
	}
	return p
}

var (
	cliBuildOnce sync.Once
	cliBinPath   string
	cliBuildErr  string
)

// buildCLI compiles the pdfdebug CLI once per run and returns its path.
func buildCLI(t *testing.T) string {
	t.Helper()
	cliBuildOnce.Do(func() {
		name := "pdfdebug"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		dir, err := os.MkdirTemp("", "pdfdebug-pdfcpu-report-")
		if err != nil {
			cliBuildErr = err.Error()
			return
		}
		bin := filepath.Join(dir, name)
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/cli/")
		cmd.Dir = projectRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			cliBuildErr = "go build ./cmd/cli/: " + err.Error() + "\n" + string(out)
			return
		}
		cliBinPath = bin
	})
	if cliBuildErr != "" {
		t.Fatal(cliBuildErr)
	}
	return cliBinPath
}

// runCLI runs the CLI and returns stdout, stderr and the exit code.
func runCLI(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(buildCLI(t), args...)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run %v: %v", args, err)
		}
		return stdout.String(), stderr.String(), ee.ExitCode()
	}
	return stdout.String(), stderr.String(), 0
}

// runGoTest runs `go test` with args from the project root.
func runGoTest(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("go", append([]string{"test"}, args...)...)
	cmd.Dir = projectRoot(t)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

type ruleInfo struct {
	RuleID    string `json:"ruleId"`
	SpecRef   string `json:"specRef"`
	Severity  string `json:"severity"`
	Checks    string `json:"checks"`
	Evaluated bool   `json:"evaluated"`
	Findings  int    `json:"findings"`
}

type problem struct {
	RuleID    string `json:"ruleId"`
	Profile   string `json:"profile"`
	Severity  string `json:"severity"`
	Message   string `json:"message"`
	ObjRef    string `json:"objRef"`
	ObjNodeID string `json:"objNodeId"`
	SpecRef   string `json:"specRef"`
}

type result struct {
	Profile string `json:"profile"`
	Summary struct {
		Errors   int `json:"errors"`
		Warnings int `json:"warnings"`
		Info     int `json:"info"`
	} `json:"summary"`
	Problems   []problem  `json:"problems"`
	Disclaimer string     `json:"disclaimer"`
	Scope      string     `json:"scope"`
	Rules      []ruleInfo `json:"rules"`
}

// runPdfcpuJSON runs `validate --profile pdfcpu --json` on a fixture and checks
// the exit code.
func runPdfcpuJSON(t *testing.T, file string, wantExit int) result {
	t.Helper()
	stdout, stderr, ec := runCLI(t, "validate", "--profile", profilePdfcpu, "--json", fixture(t, file))
	if ec != wantExit {
		t.Fatalf("validate --profile pdfcpu --json %s: exit %d, want %d\nstdout: %s\nstderr: %s", file, ec, wantExit, stdout, stderr)
	}
	var res result
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("decode JSON for %s: %v\n%s", file, err, stdout)
	}
	return res
}

// runPdfcpuPlain runs `validate --profile pdfcpu` on a fixture and checks the
// exit code.
func runPdfcpuPlain(t *testing.T, file string, wantExit int) string {
	t.Helper()
	stdout, stderr, ec := runCLI(t, "validate", "--profile", profilePdfcpu, fixture(t, file))
	if ec != wantExit {
		t.Fatalf("validate --profile pdfcpu %s: exit %d, want %d\nstdout: %s\nstderr: %s", file, ec, wantExit, stdout, stderr)
	}
	return stdout
}

// verdictPhrases are authoritative conformance verdicts no output may contain.
var verdictPhrases = []string{
	"pdf/a compliant", "pdf/a-compliant", "pdf/ua compliant", "is compliant",
	"fully compliant", "conformant", "is valid", "valid pdf/a", "pdf/a valid",
	"passed validation", "validation passed", "compliance: pass",
}

func assertNoVerdict(t *testing.T, label, s string) {
	t.Helper()
	low := strings.ToLower(s)
	for _, p := range verdictPhrases {
		if strings.Contains(low, p) {
			t.Errorf("%s contains verdict %q:\n%s", label, p, s)
		}
	}
}

func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
