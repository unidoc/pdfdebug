// Acceptance harness for the scope and rule listing that `pdfdebug validate`
// prints on every run.
//
// Black-box: the pdfdebug CLI is built once and run as a subprocess over the
// fixtures in testdata/. Expected rule ids, counts, spec refs and check phrases
// are read from the `--json` output of the same run, so a rule added to the
// registry does not break this suite.
//
// JSON wire contract asserted here (additive to the existing validate result):
//
//	profile, summary, problems, disclaimer  unchanged, in that order
//	scope   string, non-empty
//	rules   array (never null), registry order, each:
//	  ruleId    string
//	  specRef   string
//	  severity  string  registry-declared severity
//	  checks    string  what the check tests, with its coverage bound
//	  evaluated bool    false when the rule degraded to an info problem
//	  findings  int     problems the rule emitted in this run
//
// Run: cd tests/validate-output-honesty && go test -count=1 ./...
package validate_output_honesty_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const (
	profilePDFA  = "pdfa-1b"
	profilePDFUA = "pdfua-1-structural"
)

// projectRoot walks up from the test directory to the main module's go.mod.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	for {
		if content, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
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

// fixture returns the absolute path of a file in the project's testdata/.
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

// buildCLI compiles the CLI once per test package and returns the binary path.
func buildCLI(t *testing.T) string {
	t.Helper()
	cliBuildOnce.Do(func() {
		binName := "pdfdebug"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		tmpDir, err := os.MkdirTemp("", "pdfdebug-validate-honesty-")
		if err != nil {
			cliBuildErr = "failed to create temp dir: " + err.Error()
			return
		}
		binPath := filepath.Join(tmpDir, binName)
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/cli/")
		cmd.Dir = projectRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			cliBuildErr = "failed to build CLI binary: " + err.Error() + "\n" + string(out)
			return
		}
		cliBinPath = binPath
	})
	if cliBuildErr != "" {
		t.Fatalf("%s", cliBuildErr)
	}
	return cliBinPath
}

// runCLI runs the CLI with args and returns stdout, stderr and the exit code.
func runCLI(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(buildCLI(t), args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("failed to run CLI: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// ruleInfo mirrors one entry of the `rules` array.
type ruleInfo struct {
	RuleID    string `json:"ruleId"`
	SpecRef   string `json:"specRef"`
	Severity  string `json:"severity"`
	Checks    string `json:"checks"`
	Evaluated bool   `json:"evaluated"`
	Findings  int    `json:"findings"`
}

// problem mirrors one entry of the `problems` array.
type problem struct {
	RuleID    string `json:"ruleId"`
	Profile   string `json:"profile"`
	Severity  string `json:"severity"`
	Message   string `json:"message"`
	ObjRef    string `json:"objRef"`
	ObjNodeID string `json:"objNodeId"`
	SpecRef   string `json:"specRef"`
}

// validateResult mirrors the `validate --json` top-level object.
type validateResult struct {
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

// parseResult decodes `validate --json` stdout into the typed result, rejecting
// keys the contract does not define.
func parseResult(t *testing.T, stdout string) validateResult {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	dec.DisallowUnknownFields()
	var res validateResult
	if err := dec.Decode(&res); err != nil {
		t.Fatalf("failed to decode --json output: %v\nraw: %s", err, stdout)
	}
	return res
}

// runJSON runs `validate --profile P --json file`, checks the exit code and
// returns the parsed result.
func runJSON(t *testing.T, profile, file string, wantExit int) validateResult {
	t.Helper()
	stdout, stderr, ec := runCLI(t, "validate", "--profile", profile, "--json", file)
	if ec != wantExit {
		t.Fatalf("validate --profile %s --json %s: exit %d, want %d\nstdout: %s\nstderr: %s",
			profile, filepath.Base(file), ec, wantExit, stdout, stderr)
	}
	return parseResult(t, stdout)
}

// runPlain runs `validate --profile P file`, checks the exit code and returns
// stdout.
func runPlain(t *testing.T, profile, file string, wantExit int) string {
	t.Helper()
	stdout, stderr, ec := runCLI(t, "validate", "--profile", profile, file)
	if ec != wantExit {
		t.Fatalf("validate --profile %s %s: exit %d, want %d\nstdout: %s\nstderr: %s",
			profile, filepath.Base(file), ec, wantExit, stdout, stderr)
	}
	return stdout
}

// topLevelKeys returns the keys of a JSON object in document order.
func topLevelKeys(t *testing.T, raw string) []string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		t.Fatalf("--json output is not an object: %v\nraw: %s", err, raw)
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("read key: %v", err)
		}
		keys = append(keys, tok.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatalf("skip value of %q: %v", tok, err)
		}
	}
	return keys
}

// ruleIDs returns the ids of rules, in order.
func ruleIDs(rules []ruleInfo) []string {
	ids := make([]string, 0, len(rules))
	for _, r := range rules {
		ids = append(ids, r.RuleID)
	}
	return ids
}

// outcome is the plain-text outcome column expected for a rule.
func outcome(r ruleInfo) string {
	if !r.Evaluated {
		return "not evaluated"
	}
	return strconv.Itoa(r.Findings) + " found"
}

// ruleLine returns the line of the "Rules checked (N):" block whose first token
// is ruleID, or "" when the block has no such line. Lines outside the block,
// such as the scope sentence, are never matched.
func ruleLine(out, ruleID string) string {
	inBlock := false
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, "Rules checked (") {
			inBlock = true
			continue
		}
		if !inBlock {
			continue
		}
		f := strings.Fields(line)
		if len(f) == 0 {
			return ""
		}
		if f[0] == ruleID {
			return line
		}
	}
	return ""
}

// collapseSpace folds every whitespace run into one space, so a scope sentence
// is matched whether or not the plain-text renderer wraps it.
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// forbiddenVerdictPhrases are authoritative conformance verdicts the tool must
// never emit, matched case-insensitively as substrings.
var forbiddenVerdictPhrases = []string{
	"pdf/a compliant",
	"pdf/a-compliant",
	"pdf/ua compliant",
	"is compliant",
	"fully compliant",
	"conformant",
	"is valid",
	"valid pdf/a",
	"pdf/a valid",
	"passed validation",
	"validation passed",
	"compliance: pass",
}

// assertNoVerdict fails when out contains an authoritative conformance verdict.
func assertNoVerdict(t *testing.T, label, out string) {
	t.Helper()
	low := strings.ToLower(out)
	for _, p := range forbiddenVerdictPhrases {
		if strings.Contains(low, p) {
			t.Errorf("%s: authoritative verdict %q present:\n%s", label, p, out)
		}
	}
}

// assertASCII fails when out contains a non-ASCII byte.
func assertASCII(t *testing.T, label, out string) {
	t.Helper()
	for i := 0; i < len(out); i++ {
		if out[i] > 0x7f {
			t.Errorf("%s: non-ASCII byte 0x%02x at offset %d", label, out[i], i)
			return
		}
	}
}
