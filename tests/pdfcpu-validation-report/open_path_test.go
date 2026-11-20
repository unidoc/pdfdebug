package pdfcpu_validation_report_test

import (
	"strings"
	"testing"
)

// The open path parses without pdfcpu's validator, through one shared parse
// helper; the validator runs only in the check's own file, on a separate parse.
func TestOpenPath_InspectorNeverCallsTheValidator(t *testing.T) {
	src := readRepoFile(t, "internal/pdfcore/inspector.go")
	if strings.Contains(src, "ValidateContext") {
		t.Errorf("internal/pdfcore/inspector.go must not mention ValidateContext in any form")
	}
	if n := strings.Count(src, "pdfcpu_api.ReadContext("); n != 1 {
		t.Errorf("inspector.go has %d pdfcpu_api.ReadContext( calls, want 1 (one shared open parse)", n)
	}
}

func TestOpenPath_CheckLivesInItsOwnFile(t *testing.T) {
	src := readRepoFile(t, "internal/pdfcore/pdfcpucheck.go")
	if !strings.Contains(src, "ValidateContextWithReport") {
		t.Errorf("internal/pdfcore/pdfcpucheck.go must run pdfcpu_api.ValidateContextWithReport")
	}
}
