package ua_structural_rules_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// requireRuleList checks that rules lists every pdfua-1-structural rule in
// order with the registry severity and spec reference of the structure rules.
func requireRuleList(t *testing.T, r validationResult) {
	t.Helper()
	ids := make([]string, len(r.Rules))
	for i, ri := range r.Rules {
		ids[i] = ri.RuleID
	}
	if !slices.Equal(ids, uaRuleIDs) {
		t.Fatalf("rules = %v, want %v", ids, uaRuleIDs)
	}
	for _, id := range structureRuleIDs {
		ri := ruleByID(t, r, id)
		want := structureRuleSpec[id]
		if ri.Severity != want.severity || ri.SpecRef != want.specRef {
			t.Errorf("rule %s severity %q specRef %q, want %q %q", id, ri.Severity, ri.SpecRef, want.severity, want.specRef)
		}
		if strings.TrimSpace(ri.Checks) == "" {
			t.Errorf("rule %s has no checks phrase", id)
		}
	}
}

func TestValidateUAJSON_CleanFixtureHasNoFinding(t *testing.T) {
	r, code := validateJSON(t, fixture(t, "ua-structural-clean.pdf"))
	if code != 0 {
		t.Errorf("exit = %d, want 0", code)
	}
	requireRuleList(t, r)
	for _, ri := range r.Rules {
		if !ri.Evaluated || ri.Findings != 0 {
			t.Errorf("rule %+v on the clean fixture, want evaluated with 0 findings", ri)
		}
	}
	if len(r.Problems) != 0 || r.Summary != (summary{}) {
		t.Errorf("problems %+v summary %+v, want none", r.Problems, r.Summary)
	}
}

func TestValidateUAJSON_ExistingTaggedFixturesGetNoStructureFinding(t *testing.T) {
	for _, file := range []string{"structure.pdf", "tagged.pdf", "untagged.pdf"} {
		t.Run(file, func(t *testing.T) {
			r, _ := validateJSON(t, fixture(t, file))
			requireRuleList(t, r)
			for _, id := range structureRuleIDs {
				if ri := ruleByID(t, r, id); !ri.Evaluated || ri.Findings != 0 {
					t.Errorf("rule %s on %s = %+v, want evaluated with 0 findings", id, file, ri)
				}
			}
			if file == "structure.pdf" && (len(r.Problems) != 0 || r.Summary != (summary{})) {
				t.Errorf("structure.pdf problems %+v summary %+v, want none", r.Problems, r.Summary)
			}
		})
	}
}

// faultCase is one fault fixture with the one rule it trips.
type faultCase struct {
	file     string
	rule     string
	exit     int
	severity string
	// message returns the substrings the finding's message must carry, from
	// the fixture's own dump structure report.
	message func(t *testing.T, rep structureReport) []string
	// objRef returns the reference the finding must point at.
	objRef func(t *testing.T, rep structureReport) string
}

func pageRef(t *testing.T, rep structureReport) string {
	return firstPage(t, rep).Ref
}

var faultCases = []faultCase{
	{
		file: "ua-structural-unbalanced.pdf", rule: "marked-content-balance", exit: 0, severity: "warning",
		message: func(t *testing.T, rep structureReport) []string {
			p := firstPage(t, rep)
			if len(p.Streams) == 0 {
				t.Fatal("page 1 has no content streams")
			}
			return []string{"page 1: EMC with no open marked-content sequence", "content stream " + p.Streams[0].Ref}
		},
		objRef: pageRef,
	},
	{
		file: "ua-structural-duplicate-mcid.pdf", rule: "mcid-unique", exit: 0, severity: "warning",
		message: func(t *testing.T, rep structureReport) []string {
			p := firstPage(t, rep)
			occ := occurrences(t, p, 0)
			if len(occ) != 2 {
				t.Fatalf("dump structure lists MCID 0 at %v, want 2 occurrences", occ)
			}
			if len(p.Streams) != 2 || !strings.Contains(occ[1], p.Streams[1].Ref) {
				t.Errorf("second MCID 0 at %q, want it in the second /Contents stream of %+v", occ[1], p.Streams)
			}
			return append([]string{"page 1: MCID 0 marks 2 BDC sequences"}, occ...)
		},
		objRef: pageRef,
	},
	{
		file: "ua-structural-orphan-mcid.pdf", rule: "mcid-in-parent-tree", exit: 1, severity: "error",
		message: func(t *testing.T, rep structureReport) []string {
			occ := occurrences(t, firstPage(t, rep), 2)
			if len(occ) != 1 {
				t.Fatalf("dump structure lists MCID 2 at %v, want 1 occurrence", occ)
			}
			return []string{"page 1: MCID 2 (" + occ[0], "has no structure element in ParentTree key 0"}
		},
		objRef: pageRef,
	},
	{
		file: "ua-structural-nested-mcid.pdf", rule: "mcid-not-nested", exit: 0, severity: "warning",
		message: func(t *testing.T, rep structureReport) []string {
			occ := occurrences(t, firstPage(t, rep), 1)
			if len(occ) != 1 {
				t.Fatalf("dump structure lists MCID 1 at %v, want 1 occurrence", occ)
			}
			return []string{"page 1: MCID 1 (" + occ[0] + ") is nested inside MCID 0"}
		},
		objRef: pageRef,
	},
	{
		file: "ua-structural-empty-s.pdf", rule: "struct-elem-type", exit: 1, severity: "error",
		message: func(t *testing.T, rep structureReport) []string {
			return []string{"structure element " + emptySElement(t, rep) + " has no usable /S ("}
		},
		objRef: emptySElement,
	},
}

// emptySElement returns the ref of the one element whose /S is empty.
func emptySElement(t *testing.T, rep structureReport) string {
	t.Helper()
	var refs []string
	eachElement(rep.Elements, func(e *structElem) {
		if e.S == "" {
			refs = append(refs, e.Ref)
		}
	})
	if len(refs) != 1 || refs[0] == "" {
		t.Fatalf("elements with an empty /S = %v, want exactly one indirect element", refs)
	}
	return refs[0]
}

func TestValidateUAJSON_EachFaultFixtureTripsOneRule(t *testing.T) {
	for _, tc := range faultCases {
		t.Run(tc.file, func(t *testing.T) {
			path := fixture(t, tc.file)
			r, code := validateJSON(t, path)
			if code != tc.exit {
				t.Errorf("exit = %d, want %d", code, tc.exit)
			}
			requireRuleList(t, r)
			for _, ri := range r.Rules {
				if !ri.Evaluated {
					t.Errorf("rule %s not evaluated: %+v", ri.RuleID, ri)
				}
				if ri.RuleID == tc.rule {
					if ri.Findings != 1 {
						t.Errorf("rule %s findings = %d, want 1", ri.RuleID, ri.Findings)
					}
				} else if ri.Findings != 0 {
					t.Errorf("rule %s findings = %d, want 0 (only %s trips on this fixture)", ri.RuleID, ri.Findings, tc.rule)
				}
			}
			ps := problemsOf(r, tc.rule)
			if len(ps) != 1 || len(r.Problems) != 1 {
				t.Fatalf("problems = %+v, want exactly one from %s", r.Problems, tc.rule)
			}
			p := ps[0]
			spec := structureRuleSpec[tc.rule]
			if p.Profile != uaProfile || p.Severity != tc.severity || p.SpecRef != spec.specRef {
				t.Errorf("problem profile %q severity %q specRef %q, want %q %q %q", p.Profile, p.Severity, p.SpecRef, uaProfile, tc.severity, spec.specRef)
			}
			rep := dumpStructure(t, path)
			if want := tc.objRef(t, rep); p.ObjRef != want || p.ObjNodeID != nodeIDOf(t, want) {
				t.Errorf("problem objRef %q objNodeId %q, want %q %q", p.ObjRef, p.ObjNodeID, want, nodeIDOf(t, want))
			}
			for _, sub := range tc.message(t, rep) {
				if !strings.Contains(p.Message, sub) {
					t.Errorf("message %q does not contain %q", p.Message, sub)
				}
			}
			wantSummary := summary{}
			if tc.severity == "error" {
				wantSummary.Errors = 1
			} else {
				wantSummary.Warnings = 1
			}
			if r.Summary != wantSummary {
				t.Errorf("summary = %+v, want %+v", r.Summary, wantSummary)
			}
		})
	}
}

func TestValidateUA_ThousandsOfFindingsAreAllReported(t *testing.T) {
	const n = 2000
	path := writeTemp(t, "many-mcids.pdf", manyFindingsPDF(n))

	r, code := validateJSON(t, path)
	if code != 0 {
		t.Errorf("exit = %d, want 0 for warning findings only", code)
	}
	requireRuleList(t, r)
	for _, ri := range r.Rules {
		want := 0
		if ri.RuleID == "mcid-unique" || ri.RuleID == "mcid-not-nested" {
			want = n
		}
		if !ri.Evaluated || ri.Findings != want {
			t.Errorf("rule %s = %+v, want evaluated with %d findings", ri.RuleID, ri, want)
		}
	}
	if len(r.Problems) != 2*n || r.Summary != (summary{Warnings: 2 * n}) {
		t.Fatalf("got %d problems, summary %+v; want %d warning problems and nothing else", len(r.Problems), r.Summary, 2*n)
	}

	rep := dumpStructure(t, path)
	pg := firstPage(t, rep)
	unique := problemsOf(r, "mcid-unique")
	nested := problemsOf(r, "mcid-not-nested")
	if len(unique) != n || len(nested) != n {
		t.Fatalf("mcid-unique %d, mcid-not-nested %d findings; want %d each", len(unique), len(nested), n)
	}
	for m := 0; m < n; m++ {
		u := unique[m]
		occ := occurrences(t, pg, m)
		wantU := fmt.Sprintf("page 1: MCID %d marks 2 BDC sequences: %s, %s", m, occ[0], occ[1])
		if u.Message != wantU {
			t.Fatalf("mcid-unique finding %d = %q, want %q", m, u.Message, wantU)
		}
		inner := occurrences(t, pg, m+n)
		wantN := fmt.Sprintf("page 1: MCID %d (%s) is nested inside MCID %d", m+n, inner[0], m)
		if nested[m].Message != wantN {
			t.Fatalf("mcid-not-nested finding %d = %q, want %q", m, nested[m].Message, wantN)
		}
		if u.ObjRef != pg.Ref || nested[m].ObjRef != pg.Ref {
			t.Fatalf("finding %d objRefs %q %q, want page %q", m, u.ObjRef, nested[m].ObjRef, pg.Ref)
		}
	}

	stdout, _, code := runCLI(t, "validate", "--profile", uaProfile, path)
	if code != 0 {
		t.Errorf("plain exit = %d, want 0", code)
	}
	for _, p := range r.Problems {
		if !strings.Contains(stdout, "["+p.RuleID+"] "+p.Message+"\n") {
			t.Fatalf("plain text does not print the finding %q in full", p.Message)
		}
	}
	if !strings.Contains(stdout, fmt.Sprintf("Summary: 0 errors, %d warnings\n", 2*n)) {
		t.Errorf("plain text summary does not count all %d warnings", 2*n)
	}
}
