package main

import (
	"os"
	"strings"
	"testing"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// renderValidate gates per rule: a run exits 1 on an error-severity problem or
// when a rule whose registry severity is error could not be evaluated, under
// either profile. A warning-severity rule that could not be evaluated does not
// gate. The plain-text exit explanation prints exactly when the exit is 1 with
// no errors.
func TestRenderValidate_GatesOnUnevaluatedErrorRule(t *testing.T) {
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open devnull: %v", err)
	}
	defer func() { _ = devnull.Close() }()
	old := os.Stdout
	os.Stdout = devnull
	defer func() { os.Stdout = old }()

	rule := func(id string, sev pdfcore.Severity, evaluated bool, findings int) pdfcore.RuleInfo {
		return pdfcore.RuleInfo{RuleID: id, Severity: sev, SpecRef: "spec", Checks: "checks", Evaluated: evaluated, Findings: findings}
	}
	tests := []struct {
		name    string
		profile string
		summary pdfcore.ValidationSummary
		rules   []pdfcore.RuleInfo
		want    int
	}{
		{
			"UA error rule not evaluated exits 1", pdfcore.ProfilePDFUA1Structural,
			pdfcore.ValidationSummary{Info: 1},
			[]pdfcore.RuleInfo{rule("lang", "warning", true, 0), rule("mcid-unique", "error", false, 0)},
			1,
		},
		{
			"UA warning rule not evaluated exits 0", pdfcore.ProfilePDFUA1Structural,
			pdfcore.ValidationSummary{Info: 1},
			[]pdfcore.RuleInfo{rule("lang", "warning", false, 0), rule("mcid-unique", "error", true, 0)},
			0,
		},
		{
			"UA error finding exits 1", pdfcore.ProfilePDFUA1Structural,
			pdfcore.ValidationSummary{Errors: 1},
			[]pdfcore.RuleInfo{rule("lang", "warning", true, 0), rule("mcid-unique", "error", true, 1)},
			1,
		},
		{
			"UA warnings only exit 0", pdfcore.ProfilePDFUA1Structural,
			pdfcore.ValidationSummary{Warnings: 2},
			[]pdfcore.RuleInfo{rule("lang", "warning", true, 1), rule("struct-elem-type", "warning", true, 1), rule("mcid-unique", "error", true, 0)},
			0,
		},
		{
			"pdfa-1b error rule not evaluated exits 1", pdfcore.ProfilePDFA1B,
			pdfcore.ValidationSummary{Info: 1},
			[]pdfcore.RuleInfo{rule("font-embedding", "error", false, 0), rule("output-intent", "error", true, 0)},
			1,
		},
		{
			"pdfa-1b clean exits 0", pdfcore.ProfilePDFA1B,
			pdfcore.ValidationSummary{},
			[]pdfcore.RuleInfo{rule("font-embedding", "error", true, 0)},
			0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := &pdfcore.ValidationResult{
				Profile:    tc.profile,
				Summary:    tc.summary,
				Problems:   []pdfcore.Problem{},
				Disclaimer: pdfcore.DisclaimerText,
				Rules:      tc.rules,
			}
			if got := renderValidate(res, false, false); got != tc.want {
				t.Errorf("renderValidate exit = %d, want %d", got, tc.want)
			}
			if got := renderValidate(res, true, false); got != tc.want {
				t.Errorf("renderValidate --json exit = %d, want %d", got, tc.want)
			}
			var b strings.Builder
			if err := printValidatePlain(&b, res); err != nil {
				t.Fatalf("printValidatePlain: %v", err)
			}
			wantLine := tc.want == 1 && tc.summary.Errors == 0
			if has := strings.Contains(b.String(), "Exit status 1: a rule that gates the exit code could not be evaluated"); has != wantLine {
				t.Errorf("exit explanation present = %v, want %v:\n%s", has, wantLine, b.String())
			}
		})
	}
}
