package compliance_validation_test

// veraPDF PDF/UA-1 goldens of the ua-structural-*.pdf fixtures.
//
// Opt-in: the test re-runs veraPDF --flavour ua1 --addlogs on each fixture and
// compares the reduced report with the committed golden in
// testdata/verapdf-ua1/. With PDFDEBUG_WRITE_VERAPDF_GOLDENS=1 it rewrites
// the goldens instead. It skips when veraPDF is absent, so CI never needs it;
// the tests/ua-structural-rules suite reads the goldens without veraPDF.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// uaStructuralFixtures are the fixtures with a committed veraPDF ua1 golden.
var uaStructuralFixtures = []string{
	"ua-structural-clean.pdf",
	"ua-structural-unbalanced.pdf",
	"ua-structural-duplicate-mcid.pdf",
	"ua-structural-orphan-mcid.pdf",
	"ua-structural-nested-mcid.pdf",
	"ua-structural-empty-s.pdf",
}

// veraUAFailedRule is one failed ua1 rule of a reduced report.
type veraUAFailedRule struct {
	Clause       string `json:"clause"`
	TestNumber   int    `json:"testNumber"`
	FailedChecks int    `json:"failedChecks"`
}

// veraUALog is one job log message of a reduced report.
type veraUALog struct {
	Level       string `json:"level"`
	Message     string `json:"message"`
	Occurrences int    `json:"occurrences"`
}

// veraUAGolden is the reduced veraPDF report committed per fixture.
type veraUAGolden struct {
	VeraPDFVersion string             `json:"veraPDFVersion"`
	Flavour        string             `json:"flavour"`
	Compliant      bool               `json:"compliant"`
	FailedRules    []veraUAFailedRule `json:"failedRules"`
	Logs           []veraUALog        `json:"logs"`
}

// veraUAReport runs veraPDF --flavour ua1 --format json --addlogs on path and
// reduces its report to the golden shape: failed rules sorted by clause and
// test number, log messages sorted by level and message.
func veraUAReport(t *testing.T, veraBin, path string) veraUAGolden {
	t.Helper()
	out, _ := exec.Command(veraBin, "--flavour", "ua1", "--format", "json", "--addlogs", path).Output()
	var doc struct {
		Report struct {
			BuildInformation struct {
				ReleaseDetails []struct {
					ID      string `json:"id"`
					Version string `json:"version"`
				} `json:"releaseDetails"`
			} `json:"buildInformation"`
			Jobs []struct {
				ValidationResult []struct {
					Compliant bool `json:"compliant"`
					Details   struct {
						RuleSummaries []struct {
							Clause       string `json:"clause"`
							TestNumber   int    `json:"testNumber"`
							FailedChecks int    `json:"failedChecks"`
						} `json:"ruleSummaries"`
					} `json:"details"`
				} `json:"validationResult"`
				Logs struct {
					Logs []veraUALog `json:"logs"`
				} `json:"logs"`
			} `json:"jobs"`
		} `json:"report"`
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("veraPDF JSON parse failed for %s (flag drift? re-check `verapdf --help`): %v\nraw: %s", path, err, out)
	}
	if len(doc.Report.Jobs) == 0 || len(doc.Report.Jobs[0].ValidationResult) == 0 {
		t.Fatalf("veraPDF produced no validationResult for %s:\n%s", path, out)
	}
	job := doc.Report.Jobs[0]
	vr := job.ValidationResult[0]
	g := veraUAGolden{Flavour: "ua1", Compliant: vr.Compliant, FailedRules: []veraUAFailedRule{}, Logs: []veraUALog{}}
	for _, rd := range doc.Report.BuildInformation.ReleaseDetails {
		if rd.ID == "core" {
			g.VeraPDFVersion = rd.Version
		}
	}
	for _, r := range vr.Details.RuleSummaries {
		g.FailedRules = append(g.FailedRules, veraUAFailedRule(r))
	}
	sort.Slice(g.FailedRules, func(i, j int) bool {
		a, b := g.FailedRules[i], g.FailedRules[j]
		if a.Clause != b.Clause {
			return a.Clause < b.Clause
		}
		return a.TestNumber < b.TestNumber
	})
	g.Logs = append(g.Logs, job.Logs.Logs...)
	sort.Slice(g.Logs, func(i, j int) bool {
		a, b := g.Logs[i], g.Logs[j]
		if a.Level != b.Level {
			return a.Level < b.Level
		}
		return a.Message < b.Message
	})
	return g
}

// asciiJSON marshals v indented with every non-ASCII rune escaped as \uXXXX.
func asciiJSON(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden: %v", err)
	}
	var b strings.Builder
	for _, r := range string(data) {
		switch {
		case r < 0x80:
			b.WriteRune(r)
		case r > 0xffff:
			r -= 0x10000
			fmt.Fprintf(&b, `\u%04x\u%04x`, 0xd800+(r>>10), 0xdc00+(r&0x3ff))
		default:
			fmt.Fprintf(&b, `\u%04x`, r)
		}
	}
	b.WriteString("\n")
	return []byte(b.String())
}

func TestVeraPDFUA1Goldens_MatchVeraPDF(t *testing.T) {
	veraBin := veraPDFPath()
	if veraBin == "" {
		t.Skip("veraPDF not available (set VERAPDF=/path or install at the default path); golden comparison skipped")
	}
	write := os.Getenv("PDFDEBUG_WRITE_VERAPDF_GOLDENS") == "1"
	goldenDir := filepath.Join(testdataDir(t), "verapdf-ua1")
	for _, name := range uaStructuralFixtures {
		t.Run(name, func(t *testing.T) {
			got := asciiJSON(t, veraUAReport(t, veraBin, filepath.Join(testdataDir(t), name)))
			goldenPath := filepath.Join(goldenDir, strings.TrimSuffix(name, ".pdf")+".json")
			if write {
				if err := os.MkdirAll(goldenDir, 0o755); err != nil {
					t.Fatalf("create %s: %v", goldenDir, err)
				}
				if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
					t.Fatalf("write %s: %v", goldenPath, err)
				}
				return
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("golden %s missing (run with PDFDEBUG_WRITE_VERAPDF_GOLDENS=1 to capture it): %v", goldenPath, err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("veraPDF ua1 report on %s differs from its golden\ngot:\n%s\nwant:\n%s", name, got, want)
			}
		})
	}
}
