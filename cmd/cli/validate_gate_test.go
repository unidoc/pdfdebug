package main

import (
	"os"
	"testing"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// TestRenderValidate_ExitCodeGate locks the three-way exit contract at the point
// it is decided (renderValidate), including the fail-closed rule added in review
// #2: an info-only degraded run under a GATING profile (pdfa-1b) must exit 1, so
// a required rule that never actually ran cannot silently report a clean exit 0.
// The same info-only run under the non-gating pdfua-1-structural profile exits 0.
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

	tests := []struct {
		name    string
		profile string
		summary pdfcore.ValidationSummary
		want    int
	}{
		{"clean pdfa-1b exits 0", pdfcore.ProfilePDFA1B, pdfcore.ValidationSummary{}, 0},
		{"error problem exits 1", pdfcore.ProfilePDFA1B, pdfcore.ValidationSummary{Errors: 1}, 1},
		{"info-only under gating profile fails closed (exit 1)", pdfcore.ProfilePDFA1B, pdfcore.ValidationSummary{Info: 1}, 1},
		{"info-only under non-gating profile exits 0", pdfcore.ProfilePDFUA1Structural, pdfcore.ValidationSummary{Info: 1}, 0},
		{"warnings alone do not gate", pdfcore.ProfilePDFUA1Structural, pdfcore.ValidationSummary{Warnings: 3}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := &pdfcore.ValidationResult{
				Profile:    tc.profile,
				Summary:    tc.summary,
				Problems:   []pdfcore.Problem{},
				Disclaimer: pdfcore.DisclaimerText,
			}
			if got := renderValidate(res, false, false); got != tc.want {
				t.Errorf("renderValidate exit = %d, want %d", got, tc.want)
			}
		})
	}
}
