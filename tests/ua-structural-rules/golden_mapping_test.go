package ua_structural_rules_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// veraFailedRule is one failed ua1 rule of a reduced veraPDF golden.
type veraFailedRule struct {
	Clause       string `json:"clause"`
	TestNumber   int    `json:"testNumber"`
	FailedChecks int    `json:"failedChecks"`
}

// veraLog is one job log message of a reduced veraPDF golden.
type veraLog struct {
	Level       string `json:"level"`
	Message     string `json:"message"`
	Occurrences int    `json:"occurrences"`
}

// veraGolden is a committed testdata/verapdf-ua1/<fixture>.json file.
type veraGolden struct {
	VeraPDFVersion string           `json:"veraPDFVersion"`
	Flavour        string           `json:"flavour"`
	Compliant      bool             `json:"compliant"`
	FailedRules    []veraFailedRule `json:"failedRules"`
	Logs           []veraLog        `json:"logs"`
}

// readGolden reads and decodes the golden of a fixture, rejecting unknown keys.
func readGolden(t *testing.T, file string) veraGolden {
	t.Helper()
	name := strings.TrimSuffix(file, ".pdf") + ".json"
	path := filepath.Join(projectRoot(t), "testdata", "verapdf-ua1", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("veraPDF golden %s missing: %v", name, err)
	}
	for i, c := range data {
		if c > 0x7f {
			t.Fatalf("golden %s has a non-ASCII byte at offset %d", name, i)
		}
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	var g veraGolden
	if err := dec.Decode(&g); err != nil {
		t.Fatalf("golden %s does not decode: %v", name, err)
	}
	if g.VeraPDFVersion == "" || g.Flavour != "ua1" {
		t.Errorf("golden %s version %q flavour %q, want a version and flavour ua1", name, g.VeraPDFVersion, g.Flavour)
	}
	if !slices.IsSortedFunc(g.FailedRules, func(a, b veraFailedRule) int {
		if c := strings.Compare(a.Clause, b.Clause); c != 0 {
			return c
		}
		return a.TestNumber - b.TestNumber
	}) {
		t.Errorf("golden %s failedRules are not sorted by clause and test number", name)
	}
	if !slices.IsSortedFunc(g.Logs, func(a, b veraLog) int {
		if c := strings.Compare(a.Level, b.Level); c != 0 {
			return c
		}
		return strings.Compare(a.Message, b.Message)
	}) {
		t.Errorf("golden %s logs are not sorted by level and message", name)
	}
	for _, l := range g.Logs {
		if l.Occurrences < 1 {
			t.Errorf("golden %s log %q has occurrences %d", name, l.Message, l.Occurrences)
		}
	}
	return g
}

// signals returns the veraPDF signals of a golden as "clause-testNumber" for
// failed rules and "LEVEL: message" for log lines.
func signals(g veraGolden) []string {
	var out []string
	for _, r := range g.FailedRules {
		out = append(out, r.Clause+"-"+strconv.Itoa(r.TestNumber))
	}
	for _, l := range g.Logs {
		out = append(out, l.Level+": "+l.Message)
	}
	return out
}

// noSignal marks a rule that veraPDF does not flag on its fixture.
const noSignal = ""

// unrecorded marks a fixture whose veraPDF signal has not been captured yet.
const unrecorded = "unrecorded"

// veraSignalForFixture is the veraPDF ua1 signal each fault fixture adds beyond
// the clean fixture's golden: a substring that every added signal must
// contain, or noSignal when the fault adds none. It is updated when the
// goldens are captured again.
var veraSignalForFixture = map[string]string{
	"ua-structural-unbalanced.pdf":     "EMC operator does not have a balancing BMC/BDC operator",
	"ua-structural-duplicate-mcid.pdf": "Duplicate MCID - 0",
	"ua-structural-orphan-mcid.pdf":    "7.1-3",
	"ua-structural-nested-mcid.pdf":    "Nested MCID - 1",
	"ua-structural-empty-s.pdf":        "7.1-5",
}

func TestVeraPDFGoldens_FaultSignalsMapToTheirRule(t *testing.T) {
	clean := readGolden(t, "ua-structural-clean.pdf")
	base := signals(clean)

	cleanRes, _ := validateJSON(t, fixture(t, "ua-structural-clean.pdf"))
	for _, id := range structureRuleIDs {
		if ri := ruleByID(t, cleanRes, id); !ri.Evaluated || ri.Findings != 0 {
			t.Errorf("clean fixture: rule %s = %+v, want evaluated with 0 findings", id, ri)
		}
	}

	for _, tc := range faultCases {
		t.Run(tc.file, func(t *testing.T) {
			g := readGolden(t, tc.file)
			var added []string
			for _, s := range signals(g) {
				if !slices.Contains(base, s) {
					added = append(added, s)
				}
			}
			want, ok := veraSignalForFixture[tc.file]
			switch {
			case !ok || want == unrecorded:
				t.Errorf("no recorded veraPDF signal for %s; its golden adds %q over the clean golden", tc.file, added)
			case want == noSignal:
				if len(added) != 0 {
					t.Errorf("%s is recorded as having no veraPDF signal, but its golden adds %q", tc.file, added)
				}
			default:
				if len(added) == 0 || slices.ContainsFunc(added, func(s string) bool { return !strings.Contains(s, want) }) {
					t.Errorf("%s golden adds %q over the clean golden, want only signals containing %q", tc.file, added, want)
				}
			}

			r, _ := validateJSON(t, fixture(t, tc.file))
			if ri := ruleByID(t, r, tc.rule); !ri.Evaluated || ri.Findings == 0 {
				t.Errorf("rule %s = %+v on %s, want it to fire", tc.rule, ri, tc.file)
			}
		})
	}
}
