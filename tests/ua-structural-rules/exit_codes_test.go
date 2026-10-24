package ua_structural_rules_test

import (
	"strings"
	"testing"
)

func TestValidateUA_ExitCodesOnFixtures(t *testing.T) {
	cases := []struct {
		file string
		want int
	}{
		{"ua-structural-clean.pdf", 0},
		{"untagged.pdf", 0},
		{"ua-structural-empty-s.pdf", 1},
		{"ua-structural-unbalanced.pdf", 0},
		{"ua-structural-duplicate-mcid.pdf", 0},
		{"ua-structural-orphan-mcid.pdf", 1},
		{"ua-structural-nested-mcid.pdf", 0},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			for _, format := range [][]string{nil, {"--json"}} {
				args := append([]string{"validate", "--profile", uaProfile}, format...)
				_, stderr, code := runCLI(t, append(args, fixture(t, tc.file))...)
				if code != tc.want {
					t.Errorf("validate %v %s exit = %d, want %d\nstderr: %s", format, tc.file, code, tc.want, stderr)
				}
			}
		})
	}
}

func TestValidateUA_BrokenStructureFixtureExitsOne(t *testing.T) {
	r, code := validateJSON(t, fixture(t, "structure-broken.pdf"))
	if code != 1 {
		t.Errorf("exit = %d, want 1 for a file with an MCID that has no ParentTree slot", code)
	}
	for _, id := range []string{"marked-content-balance", "mcid-unique", "mcid-in-parent-tree"} {
		if ri := ruleByID(t, r, id); !ri.Evaluated || ri.Findings == 0 {
			t.Errorf("rule %s on structure-broken.pdf = %+v, want evaluated with findings", id, ri)
		}
	}
}

func TestValidateUA_EncryptedFileIsOperationalError(t *testing.T) {
	_, _, code := runCLI(t, "validate", "--profile", uaProfile, fixture(t, "encrypted.pdf"))
	if code != 2 {
		t.Errorf("validate --profile %s encrypted.pdf exit = %d, want 2", uaProfile, code)
	}
}

func TestValidatePDFA_ExitCodes(t *testing.T) {
	cases := []struct {
		file string
		want int
	}{
		{"pdfa-1b-clean.pdf", 0},
		{"no-output-intent.pdf", 1},
		{"non-embedded-font.pdf", 1},
		{"encrypted.pdf", 1},
		{"ua-structural-duplicate-mcid.pdf", 1},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			stdout, stderr, code := runCLI(t, "validate", "--profile", "pdfa-1b", fixture(t, tc.file))
			if code != tc.want {
				t.Errorf("validate --profile pdfa-1b %s exit = %d, want %d\nstdout: %s\nstderr: %s", tc.file, code, tc.want, stdout, stderr)
			}
			for _, id := range structureRuleIDs {
				if strings.Contains(stdout, id) {
					t.Errorf("a pdfa-1b run names the PDF/UA rule %s:\n%s", id, stdout)
				}
			}
		})
	}
}

func TestValidateUA_ExitLineOnlyWithoutErrors(t *testing.T) {
	for _, file := range []string{"ua-structural-unbalanced.pdf", "ua-structural-clean.pdf", "ua-structural-empty-s.pdf"} {
		stdout, _, _ := runCLI(t, "validate", "--profile", uaProfile, fixture(t, file))
		if strings.Contains(stdout, "Exit status 1: a rule that gates the exit code could not be evaluated") {
			t.Errorf("%s: the not-evaluated exit line printed on a run where every rule was evaluated:\n%s", file, stdout)
		}
	}
}
