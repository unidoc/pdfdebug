package compliance_validation_test

// Story 13.5 -- veraPDF oracle cross-check (AC 6).
//
// veraPDF is the TEST ORACLE, never a runtime dependency (NFR7 forbids
// bundling Java). This test shells veraPDF on the SAME fixtures our `validate`
// command runs on and asserts our PDF/A-1b error-severity problems stay honest
// against veraPDF's verdict.
//
// SKIPS CLEANLY when veraPDF is absent so CI without veraPDF stays green
// (t.Skip, never t.Fatal). Override the binary path with VERAPDF=/path env; the
// machine-readable flag is `--format json` on veraPDF 1.30.x (confirmed during
// ATDD authoring -- CLAUDE.md warns flags drift, so re-confirm on version bump).
//
// veraPDF 1.30.x JSON shape (confirmed):
//	report.jobs[0].validationResult[0].compliant           bool
//	report.jobs[0].validationResult[0].details.ruleSummaries[].clause  string (FAILED rules)
//
// Correspondence (AC6): a static keyword -> {veraPDF PDF/A-1 clause set}
// mapping. A negative fixture passes the oracle when (a) veraPDF reports
// non-compliant AND (b) every mapped error we emit lands on a clause veraPDF
// also failed (no false errors). The clean fixture passes when both agree on
// zero PDF/A-1b failures.
//
// RULE DELTA (veraPDF checks we deliberately do NOT implement -- structural
// firewall, see the rule-registry comment in internal/pdfcore/validate.go):
// transparency/blend-mode rules (6.2.4/6.4), annotation-flag rules (6.5.3),
// action restrictions beyond JS/launch (6.6.1), the full XMP-property schema
// (6.7.x beyond presence + /Info consistency), interactive-form/NeedAppearances
// (6.9), and ICC-profile internal correctness (only OutputIntent PRESENCE is
// ours). These are veraPDF's job; we never claim authoritative conformance.

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"
)

// veraPDFPath returns the veraPDF binary path (VERAPDF env override, else the
// owner's install path), or "" when it is not runnable on this machine.
func veraPDFPath() string {
	if p := os.Getenv("VERAPDF"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		return ""
	}
	def := "/Users/ace/Applications/verapdf/verapdf"
	if _, err := os.Stat(def); err == nil {
		return def
	}
	return ""
}

// veraFailedClauses runs veraPDF --flavour 1b --format json on path and returns
// (compliant, set-of-failed-clauses).
func veraFailedClauses(t *testing.T, veraBin, path string) (compliant bool, failed map[string]bool) {
	t.Helper()
	cmd := exec.Command(veraBin, "--flavour", "1b", "--format", "json", path)
	out, _ := cmd.Output() // veraPDF exits non-zero on non-compliant; JSON still on stdout
	var doc struct {
		Report struct {
			Jobs []struct {
				ValidationResult []struct {
					Compliant bool `json:"compliant"`
					Details   struct {
						RuleSummaries []struct {
							Clause string `json:"clause"`
						} `json:"ruleSummaries"`
					} `json:"details"`
				} `json:"validationResult"`
			} `json:"jobs"`
		} `json:"report"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("veraPDF JSON parse failed (flag drift? re-check `verapdf --help`): %v\nraw: %s", err, out)
	}
	if len(doc.Report.Jobs) == 0 || len(doc.Report.Jobs[0].ValidationResult) == 0 {
		t.Fatalf("veraPDF produced no validationResult for %s:\n%s", path, out)
	}
	vr := doc.Report.Jobs[0].ValidationResult[0]
	failed = map[string]bool{}
	for _, r := range vr.Details.RuleSummaries {
		failed[r.Clause] = true
	}
	return vr.Compliant, failed
}

// clauseSetsForRule maps our stable RuleID to the veraPDF PDF/A-1 clauses that
// cover the same defect (AC6: a static RuleID -> veraPDF rule-clause table).
// The oracle requires an intersection, not exact equality, because veraPDF may
// split one structural defect across several clause/testNumber rows. Keyed on
// RuleID (not message text) so rewording a message never silently drops a rule
// from the cross-check.
var clauseSetsForRule = map[string][]string{
	"font-embedding": {"6.3.4", "6.3.5", "6.3.7", "6.3.8"},
	"output-intent":  {"6.2.2"},
	"no-encryption":  {"6.1.3"},
}

// ---------------------------------------------------------------------------
// 13.5-INTG-050 [P0] AC6: on a negative fixture, veraPDF reports non-compliant
// AND every mapped PDF/A-1b error we emit lands on a clause veraPDF also
// failed (no false errors). Skips cleanly without veraPDF.
// ---------------------------------------------------------------------------

func TestOracle_NegativeFixtureNoFalseErrors(t *testing.T) {
	veraBin := veraPDFPath()
	if veraBin == "" {
		t.Skip("veraPDF not installed (set VERAPDF=/path to enable the oracle cross-check)")
	}
	bin := buildCLI(t)
	pdf := writeTempPDF(t, "non-embedded-font.pdf", nonEmbeddedFontPDF())

	compliant, vfailed := veraFailedClauses(t, veraBin, pdf)
	if compliant {
		t.Fatalf("veraPDF unexpectedly PASSED the non-embedded-font fixture; the negative direction is meaningless")
	}

	stdout, stderr, ec := runCLI(t, bin, "validate", "--json", pdf)
	if ec != 1 {
		t.Fatalf("our validate must exit 1 on the negative fixture, got %d (stderr: %s)", ec, stderr)
	}
	res := validateResult(t, stdout)
	for _, p := range problemsOf(t, res) {
		if getStr(p, "severity") != "error" {
			continue
		}
		clauses, mapped := clauseSetsForRule[getStr(p, "ruleId")]
		if !mapped {
			continue
		}
		if !anyClauseFailed(vfailed, clauses) {
			t.Errorf("FALSE ERROR -- we flag rule %q (%q, clauses %v) but veraPDF failed none of them (failed: %v)",
				getStr(p, "ruleId"), getStr(p, "message"), clauses, keys(vfailed))
		}
	}
}

// ---------------------------------------------------------------------------
// 13.5-INTG-051 [P0] AC6: the clean-case agreement. A genuinely veraPDF-passing
// PDF/A-1b fixture must exist for the oracle to be meaningful; when present,
// veraPDF passes it AND our rule set flags ZERO errors. Skips (with a directive
// to Dev) when the fixture is absent -- per AC6 the clean fixture may need to
// be a committed static file with recorded provenance.
// ---------------------------------------------------------------------------

func TestOracle_CleanFixtureZeroFailureAgreement(t *testing.T) {
	veraBin := veraPDFPath()
	if veraBin == "" {
		t.Skip("veraPDF not installed (set VERAPDF=/path to enable the oracle cross-check)")
	}
	clean := testdataDir(t) + "/pdfa-1b-clean.pdf"
	if _, err := os.Stat(clean); err != nil {
		t.Skip("clean PDF/A-1b positive fixture testdata/pdfa-1b-clean.pdf not present; " +
			"Dev must supply a genuinely veraPDF-passing fixture to arm this assertion")
	}
	bin := buildCLI(t)

	compliant, vfailed := veraFailedClauses(t, veraBin, clean)
	if !compliant {
		t.Fatalf("the \"clean\" fixture does NOT pass veraPDF --flavour 1b (failed clauses: %v); "+
			"the oracle clean case is worthless until the fixture genuinely passes", keys(vfailed))
	}

	stdout, stderr, ec := runCLI(t, bin, "validate", "--json", clean)
	if ec != 0 {
		t.Fatalf("our validate must exit 0 on the clean fixture, got %d (stderr: %s)", ec, stderr)
	}
	res := validateResult(t, stdout)
	if errs, _ := summaryOf(t, res); errs != 0 {
		t.Errorf("our rule set flags %d errors on a veraPDF-clean PDF/A-1b file; both must agree on zero:\n%s", errs, stdout)
	}
}

// anyClauseFailed reports whether any clause in want is in the failed set.
func anyClauseFailed(failed map[string]bool, want []string) bool {
	for _, c := range want {
		if failed[c] {
			return true
		}
	}
	return false
}

// keys returns the map keys (for failure messages).
func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
