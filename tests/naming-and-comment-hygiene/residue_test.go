package naming_and_comment_hygiene_test

import (
	"regexp"
	"testing"
)

// The six residue shapes. Every one is written as a bracketed character class,
// which is what keeps this file from matching its own gates: a class such as
// the digit class contains no digit-then-dot-then-digit run, so the pattern
// text is not an instance of the pattern.
var (
	// scenario IDs in dot-dash form. The kind token is never enumerated:
	// fourteen distinct tokens occur in this tree and the set grows whenever
	// somebody invents a category, so an alternation is wrong by construction.
	scenarioIDDotDash = regexp.MustCompile(`[0-9]+\.[0-9]+-[A-Z]+-[0-9]{3}`)

	// scenario IDs in underscore form, which is the shape the Go test
	// function names use. None of the other five patterns can see these:
	// underscore is a regex word character, so a word-boundary anchor never
	// falls between an underscore and the token that follows it.
	scenarioIDUnderscore = regexp.MustCompile(`_[0-9]+_[0-9]+_([A-Z]+_[0-9]{3}|AC[0-9]+)_`)

	// acceptance-criterion citations, in the bare, hash, spaced and hyphenated
	// spellings. The bare-digit form alone misses over a thousand sites.
	acceptanceCriterionTag = regexp.MustCompile(`\bAC[ #-]?[0-9]+\b`)

	// priority tags.
	priorityTag = regexp.MustCompile(`\[P[0-9]\]`)

	// risk IDs.
	riskID = regexp.MustCompile(`R-[0-9]{2}-[0-9]{2}`)

	// numbered acceptance-suite directory paths appearing in file contents.
	// Anchored to the tests/ prefix so version-bearing names are not caught.
	numberedSuitePath = regexp.MustCompile(`tests/[0-9]+-[0-9]+-`)
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
