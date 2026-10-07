package validate_output_honesty_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// run is one validate invocation over an in-repo fixture with its expected
// exit code. Exit codes here are the ones the CLI returns today.
type run struct {
	name    string
	profile string
	file    string
	exit    int
}

// normalRuns cover clean and non-clean results under both profiles. None of
// these opens an encrypted file, so every profile rule runs.
var normalRuns = []run{
	{name: "tagged under pdfua-1-structural", profile: profilePDFUA, file: "tagged.pdf", exit: 0},
	{name: "untagged under pdfua-1-structural", profile: profilePDFUA, file: "untagged.pdf", exit: 0},
	{name: "pdfa-1b-clean under pdfa-1b", profile: profilePDFA, file: "pdfa-1b-clean.pdf", exit: 0},
	{name: "untagged under pdfa-1b", profile: profilePDFA, file: "untagged.pdf", exit: 1},
}

// profileRules returns the full rule list of a profile, read from a run over a
// file that opens.
func profileRules(t *testing.T, profile string) []ruleInfo {
	t.Helper()
	file, exit := "tagged.pdf", 0
	if profile == profilePDFA {
		file = "pdfa-1b-clean.pdf"
	}
	res := runJSON(t, profile, fixture(t, file), exit)
	if len(res.Rules) == 0 {
		t.Fatalf("%s: --json reports no rules", profile)
	}
	return res.Rules
}

func TestValidateJSON_AppendsScopeAndRulesAfterExistingKeys(t *testing.T) {
	wantKeys := []string{"profile", "summary", "problems", "disclaimer", "scope", "rules"}
	wantRuleKeys := []string{"checks", "evaluated", "findings", "ruleId", "severity", "specRef"}
	for _, r := range normalRuns {
		t.Run(r.name, func(t *testing.T) {
			stdout, _, ec := runCLI(t, "validate", "--profile", r.profile, "--json", fixture(t, r.file))
			if ec != r.exit {
				t.Fatalf("exit %d, want %d\n%s", ec, r.exit, stdout)
			}
			if got := topLevelKeys(t, stdout); !slices.Equal(got, wantKeys) {
				t.Errorf("top-level keys = %v, want %v", got, wantKeys)
			}
			res := parseResult(t, stdout)
			if res.Profile != r.profile {
				t.Errorf("profile = %q, want %q", res.Profile, r.profile)
			}
			if !strings.Contains(res.Disclaimer, "structural checks only") {
				t.Errorf("disclaimer changed: %q", res.Disclaimer)
			}
			if strings.TrimSpace(res.Scope) == "" {
				t.Errorf("scope must be a non-empty string")
			}
			if res.Rules == nil {
				t.Fatalf("rules must be an array, got null or missing")
			}
			if len(res.Rules) == 0 {
				t.Fatalf("rules must list every rule of %s, got none", r.profile)
			}

			var raw struct {
				Rules []map[string]json.RawMessage `json:"rules"`
			}
			if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
				t.Fatalf("decode rules: %v", err)
			}
			for i, entry := range raw.Rules {
				keys := make([]string, 0, len(entry))
				for k := range entry {
					keys = append(keys, k)
				}
				slices.Sort(keys)
				if !slices.Equal(keys, wantRuleKeys) {
					t.Errorf("rules[%d] keys = %v, want %v", i, keys, wantRuleKeys)
				}
			}
			seen := map[string]bool{}
			for _, rule := range res.Rules {
				if rule.RuleID == "" || rule.SpecRef == "" || rule.Severity == "" || strings.TrimSpace(rule.Checks) == "" {
					t.Errorf("rule entry has an empty field: %+v", rule)
				}
				if seen[rule.RuleID] {
					t.Errorf("rule %q listed twice", rule.RuleID)
				}
				seen[rule.RuleID] = true
			}
		})
	}
}

func TestValidateJSON_FindingsMatchProblemsPerRule(t *testing.T) {
	for _, r := range normalRuns {
		t.Run(r.name, func(t *testing.T) {
			res := runJSON(t, r.profile, fixture(t, r.file), r.exit)
			if len(res.Rules) == 0 {
				t.Fatalf("rules must list every rule of %s, got none", r.profile)
			}
			index := map[string]int{}
			for i, rule := range res.Rules {
				index[rule.RuleID] = i
				declared, degraded := 0, 0
				for _, p := range res.Problems {
					if p.RuleID != rule.RuleID {
						continue
					}
					switch p.Severity {
					case rule.Severity:
						declared++
					case "info":
						degraded++
					default:
						t.Errorf("problem for %q has severity %q, rule declares %q", rule.RuleID, p.Severity, rule.Severity)
					}
					if p.SpecRef != rule.SpecRef {
						t.Errorf("problem for %q has specRef %q, rule lists %q", rule.RuleID, p.SpecRef, rule.SpecRef)
					}
				}
				if degraded > 0 {
					if rule.Evaluated || rule.Findings != 0 {
						t.Errorf("degraded rule %q must report evaluated=false findings=0, got %+v", rule.RuleID, rule)
					}
					continue
				}
				if !rule.Evaluated {
					t.Errorf("rule %q ran but reports evaluated=false", rule.RuleID)
				}
				if rule.Findings != declared {
					t.Errorf("rule %q findings = %d, problems with that ruleId and severity %q = %d",
						rule.RuleID, rule.Findings, rule.Severity, declared)
				}
			}
			// Problems come out in rule order, so rules is in the same order.
			last := -1
			for _, p := range res.Problems {
				i, ok := index[p.RuleID]
				if !ok {
					t.Errorf("problem rule %q is not in the rules list %v", p.RuleID, ruleIDs(res.Rules))
					continue
				}
				if i < last {
					t.Errorf("rules order %v disagrees with problem order", ruleIDs(res.Rules))
				}
				last = i
			}
		})
	}
}

func TestValidatePlain_ListsEveryRuleWithOutcomeAndChecks(t *testing.T) {
	for _, r := range normalRuns {
		t.Run(r.name, func(t *testing.T) {
			path := fixture(t, r.file)
			res := runJSON(t, r.profile, path, r.exit)
			out := runPlain(t, r.profile, path, r.exit)
			if len(res.Rules) == 0 {
				t.Fatalf("rules must list every rule of %s, got none", r.profile)
			}

			assertASCII(t, "plain", out)
			assertNoVerdict(t, "plain", out)
			if !strings.Contains(out, res.Disclaimer) {
				t.Errorf("plain output must keep the disclaimer:\n%s", out)
			}
			if !strings.Contains(collapseSpace(out), collapseSpace(res.Scope)) {
				t.Errorf("plain output must print the scope sentence %q:\n%s", res.Scope, out)
			}
			header := fmt.Sprintf("rules checked (%d)", len(res.Rules))
			if !strings.Contains(strings.ToLower(out), header) {
				t.Errorf("plain output must carry %q:\n%s", header, out)
			}

			lastPos := -1
			for _, rule := range res.Rules {
				line := ruleLine(out, rule.RuleID)
				if line == "" {
					t.Errorf("no plain-text line names rule %q:\n%s", rule.RuleID, out)
					continue
				}
				for _, want := range []string{rule.SpecRef, outcome(rule), rule.Checks} {
					if !strings.Contains(line, want) {
						t.Errorf("rule line for %q lacks %q:\n%s", rule.RuleID, want, line)
					}
				}
				pos := strings.Index(out, line)
				if pos < lastPos {
					t.Errorf("rule %q is printed out of registry order", rule.RuleID)
				}
				lastPos = pos
			}
		})
	}
}

func TestValidatePlain_CleanRunNamesRuleCount(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		profile string
		file    string
	}{
		{name: "tagged under pdfua-1-structural", args: []string{"--profile", profilePDFUA}, profile: profilePDFUA, file: "tagged.pdf"},
		{name: "pdfa-1b-clean under pdfa-1b", args: []string{"--profile", profilePDFA}, profile: profilePDFA, file: "pdfa-1b-clean.pdf"},
		{name: "pdfa-1b-clean under the default profile", profile: profilePDFA, file: "pdfa-1b-clean.pdf"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := fixture(t, c.file)
			res := runJSON(t, c.profile, path, 0)
			if len(res.Problems) != 0 {
				t.Fatalf("fixture is expected to be clean under %s, got %+v", c.profile, res.Problems)
			}
			args := append(append([]string{"validate"}, c.args...), path)
			out, stderr, ec := runCLI(t, args...)
			if ec != 0 {
				t.Fatalf("clean run exit %d, want 0\nstdout: %s\nstderr: %s", ec, out, stderr)
			}
			low := strings.ToLower(out)
			if strings.Contains(low, "no structural problems") {
				t.Errorf("clean run still prints the unscoped clean claim:\n%s", out)
			}
			count := strconv.Itoa(len(res.Rules))
			found := false
			for line := range strings.SplitSeq(low, "\n") {
				if strings.Contains(line, "none") && strings.Contains(line, count+" rule") && !strings.Contains(line, "rules checked (") {
					found = true
					break
				}
			}
			if len(res.Rules) == 0 || !found {
				t.Errorf("clean run must say none of the %s rules checked found a problem:\n%s", count, out)
			}
			for _, rule := range res.Rules {
				if line := ruleLine(out, rule.RuleID); !strings.Contains(line, "0 found") {
					t.Errorf("clean run must show %q with 0 found, got line %q", rule.RuleID, line)
				}
			}
			if !strings.Contains(out, "Summary: 0 errors, 0 warnings") {
				t.Errorf("summary line changed:\n%s", out)
			}
		})
	}
}

func TestValidatePlain_UntaggedUnderUAShowsOneFoundPerRule(t *testing.T) {
	path := fixture(t, "untagged.pdf")
	res := runJSON(t, profilePDFUA, path, 0)
	out := runPlain(t, profilePDFUA, path, 0)
	if len(res.Rules) == 0 {
		t.Fatalf("rules must list every pdfua-1-structural rule, got none")
	}
	for _, rule := range res.Rules {
		if rule.Findings != 1 || !rule.Evaluated {
			t.Errorf("untagged file: rule %q = %+v, want evaluated with 1 finding", rule.RuleID, rule)
		}
		if line := ruleLine(out, rule.RuleID); !strings.Contains(line, "1 found") {
			t.Errorf("rule %q line must show 1 found, got %q", rule.RuleID, line)
		}
	}
	if want := fmt.Sprintf("Summary: 0 errors, %d warnings", len(res.Rules)); !strings.Contains(out, want) {
		t.Errorf("summary must read %q:\n%s", want, out)
	}
	if strings.Contains(strings.ToLower(out), "none of the") {
		t.Errorf("a run with findings must not print the clean sentence:\n%s", out)
	}
}

func TestValidate_UAScopeSaysSubsetAndNamesWhatIsNotExamined(t *testing.T) {
	path := fixture(t, "tagged.pdf")
	res := runJSON(t, profilePDFUA, path, 0)
	out := runPlain(t, profilePDFUA, path, 0)
	scope := strings.ToLower(res.Scope)
	for _, want := range []string{
		"subset",
		"pdf/ua-1",
		"not a pdf/ua-1 conformance check",
		"marked content",
		"structure tree",
		"alternate text",
		"font",
	} {
		if !strings.Contains(scope, want) {
			t.Errorf("pdfua-1-structural scope must mention %q, got %q", want, res.Scope)
		}
	}
	assertNoVerdict(t, "scope", res.Scope)
	assertASCII(t, "scope", res.Scope)
	if res.Scope == "" || !strings.Contains(collapseSpace(out), collapseSpace(res.Scope)) {
		t.Errorf("plain output must carry the pdfua-1-structural scope:\n%s", out)
	}

	pdfa := runJSON(t, profilePDFA, fixture(t, "pdfa-1b-clean.pdf"), 0)
	pscope := strings.ToLower(pdfa.Scope)
	for _, want := range []string{"subset", "pdf/a-1b"} {
		if !strings.Contains(pscope, want) {
			t.Errorf("pdfa-1b scope must mention %q, got %q", want, pdfa.Scope)
		}
	}
	assertNoVerdict(t, "pdfa-1b scope", pdfa.Scope)
	assertASCII(t, "pdfa-1b scope", pdfa.Scope)
	if pdfa.Scope == res.Scope {
		t.Errorf("each profile carries its own scope sentence, both read %q", pdfa.Scope)
	}
}

func TestValidateJSON_ChecksCarryCoverageBounds(t *testing.T) {
	// Each listed rule's check phrase, or the profile scope when the bound does
	// not fit the phrase, states how far the check reaches.
	bounds := map[string][]string{
		"struct-tree-root": {"presence"},
		"output-intent":    {"device colo"},
		"font-embedding":   {"type3", "type 3"},
		"xmp-metadata":     {"both"},
		"no-js-launch":     {"/aa", "/names"},
	}
	seen := 0
	for _, profile := range []string{profilePDFA, profilePDFUA} {
		file := "tagged.pdf"
		if profile == profilePDFA {
			file = "pdfa-1b-clean.pdf"
		}
		res := runJSON(t, profile, fixture(t, file), 0)
		for _, rule := range res.Rules {
			assertNoVerdict(t, "checks for "+rule.RuleID, rule.Checks)
			assertASCII(t, "checks for "+rule.RuleID, rule.Checks)
			wants, ok := bounds[rule.RuleID]
			if !ok {
				continue
			}
			seen++
			text := strings.ToLower(rule.Checks + " " + res.Scope)
			hit := false
			for _, w := range wants {
				if strings.Contains(text, w) {
					hit = true
				}
			}
			if !hit {
				t.Errorf("rule %q: checks %q (or the scope) must state its bound, one of %v", rule.RuleID, rule.Checks, wants)
			}
		}
	}
	if seen == 0 {
		t.Errorf("no rule with a known coverage bound was listed in --json")
	}
}

func TestValidate_EncryptedPDFAListsOnlyNoEncryption(t *testing.T) {
	full := runJSON(t, profilePDFA, fixture(t, "pdfa-1b-clean.pdf"), 0)
	var registryEntry *ruleInfo
	for i := range full.Rules {
		if full.Rules[i].RuleID == "no-encryption" {
			registryEntry = &full.Rules[i]
		}
	}
	if registryEntry == nil {
		t.Fatalf("pdfa-1b rules %v must include no-encryption", ruleIDs(full.Rules))
	}

	path := fixture(t, "encrypted.pdf")
	res := runJSON(t, profilePDFA, path, 1)
	if res.Summary.Errors != 1 || len(res.Problems) != 1 || res.Problems[0].RuleID != "no-encryption" {
		t.Errorf("encrypted result problems changed: %+v", res.Problems)
	}
	if len(res.Rules) != 1 {
		t.Fatalf("encrypted file must list only the decided rule, got %v", ruleIDs(res.Rules))
	}
	got := res.Rules[0]
	if got.RuleID != "no-encryption" || !got.Evaluated || got.Findings != 1 {
		t.Errorf("encrypted rule entry = %+v, want no-encryption evaluated with 1 finding", got)
	}
	if got.SpecRef != registryEntry.SpecRef || got.Checks != registryEntry.Checks || got.Severity != registryEntry.Severity {
		t.Errorf("encrypted no-encryption entry %+v differs from the profile entry %+v", got, *registryEntry)
	}
	if full.Scope == "" || !strings.HasPrefix(res.Scope, full.Scope) || len(res.Scope) <= len(full.Scope) {
		t.Errorf("encrypted scope must start with the pdfa-1b scope %q and add why the other rules did not run, got %q",
			full.Scope, res.Scope)
	}

	out := runPlain(t, profilePDFA, path, 1)
	assertASCII(t, "plain", out)
	assertNoVerdict(t, "plain", out)
	if !strings.Contains(strings.ToLower(out), "rules checked (1)") {
		t.Errorf("encrypted plain output must carry rules checked (1):\n%s", out)
	}
	if line := ruleLine(out, "no-encryption"); !strings.Contains(line, "1 found") {
		t.Errorf("no-encryption line must show 1 found, got %q", line)
	}
	if res.Scope == "" || !strings.Contains(collapseSpace(out), collapseSpace(res.Scope)) {
		t.Errorf("encrypted plain output must print its scope %q:\n%s", res.Scope, out)
	}
	for _, rule := range full.Rules {
		if rule.RuleID == "no-encryption" {
			continue
		}
		if line := ruleLine(out, rule.RuleID); line != "" {
			t.Errorf("rule %q did not run on the encrypted file but is listed: %q", rule.RuleID, line)
		}
	}
}

func TestValidate_EncryptedUnderUAStaysOperational(t *testing.T) {
	path := fixture(t, "encrypted.pdf")
	for _, args := range [][]string{
		{"validate", "--profile", profilePDFUA, path},
		{"validate", "--profile", profilePDFUA, "--json", path},
	} {
		stdout, stderr, ec := runCLI(t, args...)
		if ec != 2 {
			t.Errorf("%v: exit %d, want 2", args, ec)
		}
		if strings.Contains(strings.ToLower(stdout+stderr), "rules checked") {
			t.Errorf("%v: an operational failure must not list rules:\n%s%s", args, stdout, stderr)
		}
	}
}

func TestValidateHelp_ListsEveryProfileRules(t *testing.T) {
	uaRules := profileRules(t, profilePDFUA)
	paRules := profileRules(t, profilePDFA)
	wantLines := map[string]string{
		profilePDFUA: fmt.Sprintf("%s checks %d rules: %s", profilePDFUA, len(uaRules), strings.Join(ruleIDs(uaRules), ", ")),
		profilePDFA:  fmt.Sprintf("%s checks %d rules: %s", profilePDFA, len(paRules), strings.Join(ruleIDs(paRules), ", ")),
	}
	cases := []struct {
		args []string
		exit int
	}{
		{args: []string{"--help"}, exit: 0},
		{args: []string{"validate", "--help"}, exit: 2},
		{args: []string{"validate", "-h"}, exit: 2},
	}
	for _, c := range cases {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			stdout, stderr, ec := runCLI(t, c.args...)
			if ec != c.exit {
				t.Errorf("exit %d, want %d", ec, c.exit)
			}
			help := stdout + stderr
			assertNoVerdict(t, "help", help)
			assertASCII(t, "help", help)
			for profile, want := range wantLines {
				line := ""
				for l := range strings.SplitSeq(help, "\n") {
					if strings.Contains(l, want) {
						line = l
						break
					}
				}
				if line == "" {
					t.Errorf("help must list %s rules as %q:\n%s", profile, want, help)
					continue
				}
				standard := "PDF/A-1b"
				if profile == profilePDFUA {
					standard = "PDF/UA-1"
				}
				for _, w := range []string{
					"structural subset of " + standard,
					"not a " + standard + " conformance check",
				} {
					if !strings.Contains(line, w) {
						t.Errorf("help line for %s must say %q, got %q", profile, w, line)
					}
				}
			}
		})
	}
}
