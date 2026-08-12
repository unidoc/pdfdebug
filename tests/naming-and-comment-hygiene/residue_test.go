package naming_and_comment_hygiene_test

import (
	"regexp"
	"strings"
	"testing"
)

// The six residue shapes. Every one is written as a bracketed character class,
// which is what keeps this file from matching its own gates: a class such as
// the digit class contains no digit-then-dot-then-digit run, so the pattern
// text is not an instance of the pattern.
//
// Two spellings are avoided throughout, because both hide residue rather than
// exclude anything: a fixed-width run where the width varies in practice, and a
// trailing word-boundary anchor, which cannot fall between two word characters
// and so is blind to every suffixed form of the shape it anchors.
var (
	// scenario IDs in dot-dash form. Neither the kind token nor the segments
	// after it are enumerated: kind tokens carry digits, an ID may carry a
	// fourth segment, and the last segment is a sequence number in most
	// spellings but a word or an uppercase placeholder in others. Requiring at
	// least one letter in the kind token, and at least two hyphen-joined
	// segments after the number, is what keeps release versions out.
	scenarioIDDotDash = regexp.MustCompile(`[0-9]+\.[0-9]+-[A-Z0-9]*[A-Z][A-Z0-9]*(-[A-Z0-9]+)+`)

	// scenario IDs in underscore form, which is the shape the Go test
	// function names use. None of the other five patterns can see these:
	// underscore is a regex word character, so a word-boundary anchor never
	// falls between an underscore and the token that follows it. The ID is not
	// required to close on an underscore either: a declaration ends its name at
	// the sequence number, and so does a comment naming that declaration.
	scenarioIDUnderscore = regexp.MustCompile(`_[0-9]+_[0-9]+_([A-Z0-9]*[A-Z][A-Z0-9]*_[A-Z0-9]+|AC[0-9]+)`)

	// acceptance-criterion citations, in the bare, hash, spaced, hyphenated,
	// parenthesised and lettered-sub-criterion spellings. The separator run is
	// counted rather than optional, since the spaced and hashed separators occur
	// together; the citation number takes a letter suffix and has no trailing
	// anchor. The leading anchor stays, and is what keeps the tail of a longer
	// word out.
	acceptanceCriterionTag = regexp.MustCompile(`\bAC[ #(-]{0,3}[0-9]+[a-z]*`)

	// priority tags, in the bracketed and parenthesised spellings.
	priorityTag = regexp.MustCompile(`[\[(]P[0-9]+[\])]`)

	// risk IDs, either segment one to three digits wide, either case.
	riskID = regexp.MustCompile(`(?i)R-[0-9]{1,3}-[0-9]{1,3}`)

	// numbered acceptance-suite directory paths appearing in file contents.
	// Anchored to the tests/ prefix, in both path separators, so version-bearing
	// names are not caught; it stops at the number, because a reference that
	// gives the number and not the rest of the name is the same dead pointer.
	numberedSuitePath = regexp.MustCompile(`(?i)tests[/\\][0-9]+-[0-9]+`)
)

func TestNoDotDashScenarioIDsInTree(t *testing.T) {
	root := projectRoot(t)
	hits := scanTree(t, root, scenarioIDDotDash)
	if len(hits) > 0 {
		t.Errorf("dot-dash scenario IDs must not appear in tracked source, comments, test names or assertion messages.\n"+
			"pattern %s matched %s", scenarioIDDotDash, reportHits(hits, 40, 25))
	}
}

func TestNoUnderscoreScenarioIDsInTree(t *testing.T) {
	root := projectRoot(t)
	hits := scanTree(t, root, scenarioIDUnderscore)
	if len(hits) > 0 {
		t.Errorf("underscore-form scenario IDs must not appear in Go test function names or the doc comments naming them.\n"+
			"This is the only pattern that sees the ID-shaped test function names; the other five match none of them.\n"+
			"pattern %s matched %s", scenarioIDUnderscore, reportHits(hits, 40, 25))
	}
}

func TestNoAcceptanceCriterionCitationsInTree(t *testing.T) {
	root := projectRoot(t)
	hits := scanTree(t, root, acceptanceCriterionTag)
	if len(hits) > 0 {
		t.Errorf("acceptance-criterion citations must not appear in tracked source, comments, test names or assertion messages.\n"+
			"Where the citation sits inside a sentence stating an engineering constraint, rewrite the sentence to state the\n"+
			"constraint without the citation; do not drop the sentence.\n"+
			"pattern %s matched %s", acceptanceCriterionTag, reportHits(hits, 40, 25))
	}
}

func TestNoPriorityTagsInTree(t *testing.T) {
	root := projectRoot(t)
	hits := scanTree(t, root, priorityTag)
	if len(hits) > 0 {
		t.Errorf("priority tags must not appear in tracked source, comments, test names or assertion messages.\n"+
			"pattern %s matched %s", priorityTag, reportHits(hits, 40, 25))
	}
}

func TestNoRiskIDsInTree(t *testing.T) {
	root := projectRoot(t)
	hits := scanTree(t, root, riskID)
	if len(hits) > 0 {
		t.Errorf("risk IDs must not appear in tracked source, comments, test names or assertion messages.\n"+
			"pattern %s matched %s", riskID, reportHits(hits, 40, 25))
	}
}

func TestNoNumberedSuitePathsInFileContents(t *testing.T) {
	root := projectRoot(t)
	hits := scanTree(t, root, numberedSuitePath)
	if len(hits) > 0 {
		t.Errorf("no comment or string may name a numbered acceptance-suite directory; those directories are being renamed,\n"+
			"so each of these becomes a pointer to a path that does not exist.\n"+
			"pattern %s matched %s", numberedSuitePath, reportHits(hits, 40, 40))
	}
}

// TestResiduePatternsSeeTheSpellingsTheTreeContains is the gate on the gates. A
// residue pattern that misses a spelling reports success over the sites it cannot
// see, which is the one failure here that looks exactly like being finished, so
// each pattern is held against the spellings the tree actually carries and
// against the numbered shapes that are not residue.
//
// Every positive sample is stored in fragments and joined at read time. That is
// what lets this file assert on real residue without containing any: no sample
// appears contiguously in the source, so the six gates still pass over it.
func TestResiduePatternsSeeTheSpellingsTheTreeContains(t *testing.T) {
	cases := []struct {
		what         string
		re           *regexp.Regexp
		mustMatch    [][]string
		mustNotMatch []string
	}{
		{
			what: "dot-dash scenario IDs",
			re:   scenarioIDDotDash,
			mustMatch: [][]string{
				{"14.4-", "UNIT-002"},
				{"2.4-", "E2E-001"},          // kind token carrying a digit
				{"11.6-", "INTG-XCUT-001"},   // fourth segment
				{"13.2-", "UNIT-NNN"},        // placeholder sequence number
				{"2.5-", "UNIT-EMPTY-ROOT"},  // worded sequence segment
				{"11.5-INTG-", "AC", "1-00"}, // citation used as the sequence segment
			},
			mustNotMatch: []string{
				"ISO 32000-1", "14.8.4", "7.3.4.2", "%PDF-1.7",
				"v3.0.0-alpha2.117", "pdfcpu 0.12.1", "wails3 v3.0.0-alpha.79",
				"/Type1", "D:20260807", "12 0 obj",
			},
		},
		{
			what: "underscore scenario IDs",
			re:   scenarioIDUnderscore,
			mustMatch: [][]string{
				{"func Test", "_14_4", "_UNIT_001(t *testing.T) {"}, // ID closes the declared name
				{"successors are Test", "_12_3", "_INTG_011 (go.sum)"},
				{"Test", "_14_4", "_", "AC", "5"},
			},
			mustNotMatch: []string{"Test_1_2_3", "LEVEL_1_2_A", "buf_16_32_bytes"},
		},
		{
			what: "acceptance-criterion citations",
			re:   acceptanceCriterionTag,
			mustMatch: [][]string{
				{"AC", "4"},
				{"AC", " 4"},
				{"AC", "#4"},
				{"AC", "-4"},
				{"AC", " #4"}, // spaced and hashed together
				{"AC", "#2a"}, // lettered sub-criterion
				{"AC", " #5b"},
				{"AC", " (1-7)"},
				{"AC", " (#10)"},
			},
			mustNotMatch: []string{
				"MAC 10", "HVAC 20", "U+20AC (CP1252)", "hmac256", "0xAC10",
				"ISO 32000-1", "14.8.4", "7.3.4.2", "%PDF-1.7", "/Type1",
				"v3.0.0-alpha2.117", "pdfcpu 0.12.1", "D:20260807", "12 0 obj",
			},
		},
		{
			what: "priority tags",
			re:   priorityTag,
			mustMatch: [][]string{
				{"[P", "0]"},
				{"(P", "1)"}, // parenthesised spelling
				{"[P", "10]"},
			},
			mustNotMatch: []string{
				"/Pattern << /P0 13 0 R >>", "want one P0 patternType 2",
				"p := arr[0]", "(Page 1)",
			},
		},
		{
			what: "risk IDs",
			re:   riskID,
			mustMatch: [][]string{
				{"R-", "14-13"},
				{"R-", "14-2"}, // single-digit segment
				{"r-", "14-05"},
				{"R-", "140-02"},
			},
			mustNotMatch: []string{"ISO 32000-1", "2026-08-07", "%PDF-1.7", "12 0 obj"},
		},
		{
			what: "numbered acceptance-suite paths",
			re:   numberedSuitePath,
			mustMatch: [][]string{
				{"tests/", "14-4-shared-text-string-decoder"},
				{"mirrors tests/", "13-2 / 13-3"}, // number alone, no suite name
				{"tests", "\\14-4-shared-text-string-decoder"},
				{"Tests/", "10-1-app-shell"},
			},
			mustNotMatch: []string{
				"tests/e2e/app-shell.spec.ts", "tests/shared-text-string-decoder",
				"attestations/1-2", "latest-1-2",
			},
		},
	}

	for _, c := range cases {
		for _, parts := range c.mustMatch {
			sample := strings.Join(parts, "")
			if !c.re.MatchString(sample) {
				t.Errorf("the %s pattern is blind to %q, a spelling this tree contains. Its gate counts every site\n"+
					"written that way as clean.\npattern %s", c.what, sample, c.re)
			}
		}
		for _, sample := range c.mustNotMatch {
			if m := c.re.FindString(sample); m != "" {
				t.Errorf("the %s pattern matched %q in %q, which is a clause, version, date or object reference rather\n"+
					"than residue.\npattern %s", c.what, m, sample, c.re)
			}
		}
	}
}

// TestResidueGatesDoNotMatchThemselves proves the gate files are not exempted
// from the scan by exemption but by construction. If a future edit writes a
// banned shape literally into this suite, the six gates above could never go
// green, and this check names the offender instead of leaving it to be found by
// elimination.
func TestResidueGatesDoNotMatchThemselves(t *testing.T) {
	root := projectRoot(t)
	patterns := []*regexp.Regexp{
		scenarioIDDotDash,
		scenarioIDUnderscore,
		acceptanceCriterionTag,
		priorityTag,
		riskID,
		numberedSuitePath,
	}
	for _, re := range patterns {
		var own []hit
		for _, h := range scanTree(t, root, re) {
			if len(h.path) >= len(thisSuite) && h.path[:len(thisSuite)] == thisSuite {
				own = append(own, h)
			}
		}
		if len(own) > 0 {
			t.Errorf("pattern %s matches this gate suite's own files, so its gate can never pass.\n"+
				"Express the shape as a bracketed character class instead of writing an instance of it. %s",
				re, reportHits(own, 10, 10))
		}
	}
}
