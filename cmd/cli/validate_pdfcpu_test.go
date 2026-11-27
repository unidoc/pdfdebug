package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"unidoc-pdf-debugger/internal/pdfcore"
	"unidoc-pdf-debugger/internal/pdfservice"
)

// The pdfcpu profile's help line names its two rules without the structural
// subset wording, and the two structural profile lines stay as they were.
func TestProfileRuleLines_PdfcpuLineAndUnchangedStructuralLines(t *testing.T) {
	lines := profileRuleLines()
	if len(lines) != 3 {
		t.Fatalf("profileRuleLines() = %d lines, want one per profile (3):\n%s", len(lines), strings.Join(lines, "\n"))
	}
	for i, profile := range []string{pdfcore.ProfilePDFA1B, pdfcore.ProfilePDFUA1Structural} {
		rules := pdfcore.ProfileRules(profile)
		ids := make([]string, 0, len(rules))
		for _, r := range rules {
			ids = append(ids, r.RuleID)
		}
		standard := pdfcore.ProfileStandard(profile)
		want := fmt.Sprintf("  %s checks %d rules: %s (a structural subset of %s, not a %s conformance check)",
			profile, len(rules), strings.Join(ids, ", "), standard, standard)
		if lines[i] != want {
			t.Errorf("help line for %s changed:\n got %q\nwant %q", profile, lines[i], want)
		}
	}
	line := lines[2]
	if !strings.HasPrefix(line, "  pdfcpu checks 2 rules: pdfcpu-validator, pdfcpu-notices") {
		t.Errorf("pdfcpu help line = %q, want it to start with %q", line, "  pdfcpu checks 2 rules: pdfcpu-validator, pdfcpu-notices")
	}
	if strings.Contains(line, "structural subset of") {
		t.Errorf("pdfcpu help line must not use the structural subset wording: %q", line)
	}
	if !strings.Contains(line, "not a conformance check") {
		t.Errorf("pdfcpu help line must say it is not a conformance check: %q", line)
	}
}

func TestValidateSynopsisNamesThePdfcpuProfile(t *testing.T) {
	help := usageText(t)
	if line := commandLine(t, help, "validate "); !strings.Contains(line, "pdfcpu") {
		t.Errorf("validate line in --help must name the pdfcpu profile: %q", line)
	}
	if line := commandUsageLine(t, "validate"); !strings.Contains(line, "pdfcpu") {
		t.Errorf("validate usage line must name the pdfcpu profile: %q", line)
	}
}

// pdfcpuProblem is the part of a problem both surfaces must agree on.
type pdfcpuProblem struct {
	RuleID    string `json:"ruleId"`
	Severity  string `json:"severity"`
	Message   string `json:"message"`
	ObjRef    string `json:"objRef"`
	ObjNodeID string `json:"objNodeId"`
}

func buildCLIBinary(t *testing.T) string {
	t.Helper()
	name := "pdfdebug"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	bin := filepath.Join(t.TempDir(), name)
	cmd := exec.Command("go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

// The CLI's --json output and the GUI binding return the same pdfcpu problems
// for the same file.
func TestPdfcpuProblemsMatchBetweenCLIAndService(t *testing.T) {
	bin := buildCLIBinary(t)
	cases := []struct {
		file string
		exit int
	}{
		{"minimal.pdf", 0},
		{"structure-broken.pdf", 1},
		{"fonts-mixed.pdf", 0},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			path, err := filepath.Abs(filepath.Join("..", "..", "testdata", c.file))
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(bin, "validate", "--profile", "pdfcpu", "--json", path)
			var stdout, stderr strings.Builder
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			exit := 0
			if err := cmd.Run(); err != nil {
				ee, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatalf("run CLI: %v", err)
				}
				exit = ee.ExitCode()
			}
			if exit != c.exit {
				t.Fatalf("validate --profile pdfcpu %s exit %d, want %d\nstdout: %s\nstderr: %s", c.file, exit, c.exit, stdout.String(), stderr.String())
			}
			var cli struct {
				Profile  string          `json:"profile"`
				Problems []pdfcpuProblem `json:"problems"`
			}
			if err := json.Unmarshal([]byte(stdout.String()), &cli); err != nil {
				t.Fatalf("decode CLI JSON: %v\n%s", err, stdout.String())
			}

			svc := pdfservice.NewPDFService(nil)
			info, err := svc.OpenFile(path)
			if err != nil {
				t.Fatalf("service OpenFile: %v", err)
			}
			defer func() { _ = svc.CloseDocument(info.TabID) }()
			res, err := svc.Validate(info.TabID, "pdfcpu")
			if err != nil {
				t.Fatalf("service Validate: %v", err)
			}
			gui := make([]pdfcpuProblem, 0, len(res.Problems))
			for _, p := range res.Problems {
				gui = append(gui, pdfcpuProblem{p.RuleID, p.Severity, p.Message, p.ObjRef, p.ObjNodeID})
			}
			if cli.Profile != "pdfcpu" || res.Profile != "pdfcpu" {
				t.Errorf("profiles = CLI %q, service %q, want pdfcpu", cli.Profile, res.Profile)
			}
			if !slices.Equal(cli.Problems, gui) {
				t.Errorf("problem lists differ:\n CLI     %+v\n service %+v", cli.Problems, gui)
			}
			if c.file != "minimal.pdf" && len(gui) == 0 {
				t.Errorf("%s: expected pdfcpu problems, both surfaces returned none", c.file)
			}
		})
	}
}
