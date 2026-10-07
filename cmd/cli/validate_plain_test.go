package main

import (
	"io"
	"os"
	"strings"
	"testing"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// A rule that degraded to an info problem prints "not evaluated" in the rules
// block, and the clean sentence does not print because a problem is present.
func TestPrintValidatePlain_DegradedRuleShowsNotEvaluated(t *testing.T) {
	rules := pdfcore.ProfileRules(pdfcore.ProfilePDFUA1Structural)
	if len(rules) < 2 {
		t.Fatalf("need at least two pdfua-1-structural rules, got %d", len(rules))
	}
	for i := range rules {
		rules[i].Evaluated = true
	}
	degraded := rules[len(rules)-1]
	rules[len(rules)-1].Evaluated = false
	res := &pdfcore.ValidationResult{
		Profile: pdfcore.ProfilePDFUA1Structural,
		Summary: pdfcore.ValidationSummary{Info: 1},
		Problems: []pdfcore.Problem{{
			RuleID:   degraded.RuleID,
			Severity: "info",
			Message:  "rule could not be evaluated: boom",
			SpecRef:  degraded.SpecRef,
		}},
		Disclaimer: pdfcore.DisclaimerText,
		Scope:      pdfcore.ProfileScope(pdfcore.ProfilePDFUA1Structural),
		Rules:      rules,
	}

	var b strings.Builder
	if err := printValidatePlain(&b, res); err != nil {
		t.Fatalf("printValidatePlain: %v", err)
	}
	out := b.String()

	for _, r := range rules {
		var line string
		for l := range strings.SplitSeq(out, "\n") {
			if f := strings.Fields(l); len(f) > 0 && f[0] == r.RuleID {
				line = l
				break
			}
		}
		if line == "" {
			t.Errorf("no rules-block line for %q:\n%s", r.RuleID, out)
			continue
		}
		want, unwanted := "0 found", "not evaluated"
		if r.RuleID == degraded.RuleID {
			want, unwanted = "not evaluated", "found"
		}
		if !strings.Contains(line, want) || strings.Contains(line, unwanted) {
			t.Errorf("line for %q = %q, want %q and not %q", r.RuleID, line, want, unwanted)
		}
	}
	if strings.Contains(out, "none of the") {
		t.Errorf("a degraded run must not print the clean sentence:\n%s", out)
	}
	if !strings.Contains(out, "1 info problem") {
		t.Errorf("summary must surface the degraded rule:\n%s", out)
	}
}

// The spec ref, outcome and checks columns start at the same offset on every
// line, even when ids, spec refs and outcomes differ in width.
func TestWriteRulesChecked_AlignsColumns(t *testing.T) {
	rules := []pdfcore.RuleInfo{
		{RuleID: "a", SpecRef: "ISO 1, 6.1", Evaluated: true, Findings: 0, Checks: "first check"},
		{RuleID: "longer-rule-id", SpecRef: "ISO 19005-1:2005, 6.7.2/6.7.3", Evaluated: false, Checks: "second check"},
		{RuleID: "mid-id", SpecRef: "x", Evaluated: true, Findings: 12, Checks: "third check"},
	}
	var b strings.Builder
	writeRulesChecked(&b, rules)
	out := b.String()

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 1+len(rules) {
		t.Fatalf("got %d lines, want header plus %d rule lines:\n%s", len(lines), len(rules), out)
	}
	if lines[0] != "Rules checked (3):" {
		t.Errorf("header = %q, want %q", lines[0], "Rules checked (3):")
	}
	if !strings.HasSuffix(out, "\n\n") {
		t.Errorf("rules block must end with a blank line:\n%q", out)
	}

	specCol, outCol, checksCol := -1, -1, -1
	for i, r := range rules {
		line := lines[i+1]
		outcome := ruleOutcome(r)
		s := strings.Index(line, r.SpecRef)
		o := strings.Index(line[s+len(r.SpecRef):], outcome) + s + len(r.SpecRef)
		c := strings.LastIndex(line, r.Checks)
		if !strings.HasPrefix(line, "  "+r.RuleID) || s < 0 || c < 0 {
			t.Fatalf("line %q does not carry id, spec ref and checks of %+v", line, r)
		}
		if i == 0 {
			specCol, outCol, checksCol = s, o, c
			continue
		}
		if s != specCol || o != outCol || c != checksCol {
			t.Errorf("line %q columns at %d/%d/%d, want %d/%d/%d", line, s, o, c, specCol, outCol, checksCol)
		}
	}
}

// captureStderr runs fn with os.Stderr redirected and returns what it wrote.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	saved := os.Stderr
	os.Stderr = w
	done := make(chan string)
	go func() {
		data, _ := io.ReadAll(r)
		done <- string(data)
	}()
	defer func() { os.Stderr = saved }()
	fn()
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}

// -h prints the usage line followed by each profile's rule line; any other
// flag parse error prints the usage line alone. Both exit 2.
func TestRunValidate_RuleLinesOnlyOnHelp(t *testing.T) {
	ruleLines := profileRuleLines()
	if len(ruleLines) != len(pdfcore.ValidProfiles) {
		t.Fatalf("profileRuleLines returned %d lines, want one per profile", len(ruleLines))
	}

	var code int
	help := captureStderr(t, func() { code = runValidate([]string{"-h"}) })
	if code != 2 {
		t.Errorf("validate -h exit %d, want 2", code)
	}
	want := validateUsage + "\n" + strings.Join(ruleLines, "\n") + "\n"
	if help != want {
		t.Errorf("validate -h stderr =\n%q\nwant\n%q", help, want)
	}

	bad := captureStderr(t, func() { code = runValidate([]string{"--no-such-flag", "x.pdf"}) })
	if code != 2 {
		t.Errorf("unknown flag exit %d, want 2", code)
	}
	if bad != validateUsage+"\n" {
		t.Errorf("unknown flag stderr = %q, want only the usage line", bad)
	}
}
