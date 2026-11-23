package pdfcpu_validation_report_test

import (
	"slices"
	"strings"
	"testing"
)

const disclaimer = "structural checks only - not full conformance; use veraPDF for authoritative PDF/A / PDF/UA validation"

func ruleIDs(rules []ruleInfo) []string {
	ids := make([]string, 0, len(rules))
	for _, r := range rules {
		ids = append(ids, r.RuleID)
	}
	return ids
}

func TestPdfcpuProfile_ExitCodes(t *testing.T) {
	for _, c := range []struct {
		file string
		exit int
	}{
		{"structure-broken.pdf", 1},
		{"fonts-mixed.pdf", 0},
		{"minimal.pdf", 0},
		{"encrypted.pdf", 2},
		{"malformed.pdf", 2},
	} {
		t.Run(c.file, func(t *testing.T) {
			for _, extra := range [][]string{nil, {"--json"}} {
				args := append(append([]string{"validate", "--profile", profilePdfcpu}, extra...), fixture(t, c.file))
				stdout, stderr, ec := runCLI(t, args...)
				if ec != c.exit {
					t.Errorf("%v: exit %d, want %d\nstdout: %s\nstderr: %s", extra, ec, c.exit, stdout, stderr)
				}
			}
		})
	}
}

func TestPdfcpuProfile_StructureBrokenPrintsTheFatalProblem(t *testing.T) {
	out := runPdfcpuPlain(t, "structure-broken.pdf", 1)
	if !strings.Contains(out, "[pdfcpu-validator]") || !strings.Contains(out, "circular structure tree") {
		t.Errorf("plain output must list the pdfcpu-validator problem naming the circular structure tree:\n%s", out)
	}
	if !strings.Contains(out, "12 0 R") {
		t.Errorf("plain output must name object 12 0 R:\n%s", out)
	}

	res := runPdfcpuJSON(t, "structure-broken.pdf", 1)
	var fatal []problem
	for _, p := range res.Problems {
		if p.RuleID == "pdfcpu-validator" {
			fatal = append(fatal, p)
		}
	}
	if len(fatal) != 1 {
		t.Fatalf("pdfcpu-validator problems = %+v, want one", fatal)
	}
	if fatal[0].Severity != "error" || fatal[0].ObjRef != "12 0 R" || fatal[0].ObjNodeID != "obj:0:12" || fatal[0].Profile != profilePdfcpu {
		t.Errorf("fatal problem = %+v, want error at 12 0 R / obj:0:12 under profile pdfcpu", fatal[0])
	}
	if res.Summary.Errors != 1 {
		t.Errorf("summary = %+v, want 1 error", res.Summary)
	}
}

func TestPdfcpuProfile_FontsMixedPrintsTheOneNotice(t *testing.T) {
	want := "digested during validate: Type1 font CustomFont: missing required entries Widths"
	out := runPdfcpuPlain(t, "fonts-mixed.pdf", 0)
	if !strings.Contains(out, "[pdfcpu-notices] "+want) {
		t.Errorf("plain output must list the notice %q:\n%s", want, out)
	}
	if !strings.Contains(out, "5 0 R") {
		t.Errorf("plain output must name object 5 0 R:\n%s", out)
	}

	res := runPdfcpuJSON(t, "fonts-mixed.pdf", 0)
	if len(res.Problems) != 1 {
		t.Fatalf("problems = %+v, want exactly the one notice", res.Problems)
	}
	p := res.Problems[0]
	if p.RuleID != "pdfcpu-notices" || p.Severity != "warning" || p.Message != want || p.ObjRef != "5 0 R" || p.ObjNodeID != "obj:0:5" {
		t.Errorf("notice = %+v, want pdfcpu-notices warning %q at 5 0 R / obj:0:5", p, want)
	}
}

func TestPdfcpuProfile_MinimalSaysNoneOfTheTwoRulesFoundAProblem(t *testing.T) {
	out := runPdfcpuPlain(t, "minimal.pdf", 0)
	if !strings.Contains(out, "none of the 2 rules checked found a problem") {
		t.Errorf("clean run must say none of the 2 rules checked found a problem:\n%s", out)
	}
	res := runPdfcpuJSON(t, "minimal.pdf", 0)
	if len(res.Problems) != 0 || res.Problems == nil {
		t.Errorf("problems = %#v, want an empty array", res.Problems)
	}
}

func TestPdfcpuProfile_JSONCarriesProfileRulesScopeAndDisclaimer(t *testing.T) {
	for _, c := range []struct {
		file string
		exit int
	}{
		{"minimal.pdf", 0},
		{"fonts-mixed.pdf", 0},
		{"structure-broken.pdf", 1},
	} {
		t.Run(c.file, func(t *testing.T) {
			res := runPdfcpuJSON(t, c.file, c.exit)
			if res.Profile != profilePdfcpu {
				t.Errorf("profile = %q, want pdfcpu", res.Profile)
			}
			if got := ruleIDs(res.Rules); !slices.Equal(got, []string{"pdfcpu-validator", "pdfcpu-notices"}) {
				t.Errorf("rules = %v, want [pdfcpu-validator pdfcpu-notices]", got)
			}
			for _, r := range res.Rules {
				if !r.Evaluated || r.SpecRef != "pdfcpu" {
					t.Errorf("rule %+v must be evaluated with specRef pdfcpu", r)
				}
			}
			if res.Disclaimer != disclaimer {
				t.Errorf("disclaimer = %q, want the unchanged %q", res.Disclaimer, disclaimer)
			}
			scope := strings.ToLower(res.Scope)
			if !strings.Contains(scope, "separate parse") {
				t.Errorf("scope must say the check runs on a separate parse: %q", res.Scope)
			}
			if !strings.Contains(scope, "incomplete") && !strings.Contains(scope, "partial") {
				t.Errorf("scope must say the notice list can be incomplete: %q", res.Scope)
			}

			out := runPdfcpuPlain(t, c.file, c.exit)
			if !strings.Contains(out, disclaimer) {
				t.Errorf("plain output must keep the disclaimer:\n%s", out)
			}
			if res.Scope == "" || !strings.Contains(collapseSpace(out), collapseSpace(res.Scope)) {
				t.Errorf("plain output must print the scope sentence:\n%s", out)
			}
			assertNoVerdict(t, "plain", out)
			assertNoVerdict(t, "scope", res.Scope)
			for _, r := range res.Rules {
				assertNoVerdict(t, "checks for "+r.RuleID, r.Checks)
			}
		})
	}
}

func TestPdfcpuProfile_HelpListsItsOwnLine(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"validate", "--help"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, _ := runCLI(t, args...)
			help := stdout + stderr
			line := ""
			for l := range strings.SplitSeq(help, "\n") {
				if strings.Contains(l, "pdfcpu checks 2 rules: pdfcpu-validator, pdfcpu-notices") {
					line = l
				}
			}
			if line == "" {
				t.Fatalf("help must list the pdfcpu profile's rules:\n%s", help)
			}
			if strings.Contains(line, "structural subset of") {
				t.Errorf("pdfcpu help line must not say structural subset: %q", line)
			}
			assertNoVerdict(t, "help", help)
		})
	}
}

func TestPdfcpuProfile_UnknownProfileMessageListsIt(t *testing.T) {
	_, stderr, ec := runCLI(t, "validate", "--profile", "no-such-profile", fixture(t, "minimal.pdf"))
	if ec != 2 {
		t.Errorf("unknown profile exit %d, want 2", ec)
	}
	if !strings.Contains(stderr, "pdfa-1b, pdfua-1-structural, pdfcpu") {
		t.Errorf("unknown-profile message must list all three profiles:\n%s", stderr)
	}
}
