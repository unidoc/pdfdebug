package main

import (
	"os"
	"strings"
	"testing"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// TestRenderValidate_ExitCodeGate locks the three-way exit contract at the point
// it is decided (renderValidate), including the fail-closed rule: a run where an
// error-severity rule degraded to info must exit 1, so a required rule that
// never actually ran cannot silently report a clean exit 0. A run where only a
// warning-severity rule degraded exits 0. The plain-text exit explanation
// prints exactly when the exit is 1 with no errors.
func TestRenderValidate_ExitCodeGate(t *testing.T) {
	// Silence the plain-text report the function writes to os.Stdout.
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open devnull: %v", err)
	}
	defer func() { _ = devnull.Close() }()
	old := os.Stdout
	os.Stdout = devnull
	defer func() { os.Stdout = old }()

	degraded := func(id string, sev pdfcore.Severity) []pdfcore.RuleInfo {
		return []pdfcore.RuleInfo{{RuleID: id, Severity: sev, Evaluated: false}}
	}
	tests := []struct {
		name    string
		profile string
		summary pdfcore.ValidationSummary
		rules   []pdfcore.RuleInfo
		want    int
	}{
		{"clean pdfa-1b exits 0", pdfcore.ProfilePDFA1B, pdfcore.ValidationSummary{}, nil, 0},
		{"error problem exits 1", pdfcore.ProfilePDFA1B, pdfcore.ValidationSummary{Errors: 1}, nil, 1},
		{"info-only from a degraded error rule fails closed (exit 1)", pdfcore.ProfilePDFA1B, pdfcore.ValidationSummary{Info: 1}, degraded("font-embedding", "error"), 1},
		{"info-only from a degraded warning rule exits 0", pdfcore.ProfilePDFUA1Structural, pdfcore.ValidationSummary{Info: 1}, degraded("lang", "warning"), 0},
		{"warnings alone do not gate", pdfcore.ProfilePDFUA1Structural, pdfcore.ValidationSummary{Warnings: 3}, nil, 0},
		{"errors and a degraded error rule exit 1", pdfcore.ProfilePDFA1B, pdfcore.ValidationSummary{Errors: 1, Info: 1}, degraded("font-embedding", "error"), 1},
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
			got := renderValidate(res, false, false)
			if got != tc.want {
				t.Errorf("renderValidate exit = %d, want %d", got, tc.want)
			}
			var b strings.Builder
			if err := printValidatePlain(&b, res); err != nil {
				t.Fatalf("printValidatePlain: %v", err)
			}
			wantLine := got == 1 && tc.summary.Errors == 0
			if has := strings.Contains(b.String(), "Exit status 1: a rule that gates the exit code could not be evaluated"); has != wantLine {
				t.Errorf("exit explanation present = %v, want %v:\n%s", has, wantLine, b.String())
			}
		})
	}
}
