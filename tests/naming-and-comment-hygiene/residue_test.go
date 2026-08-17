package naming_and_comment_hygiene_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// The residue shapes. Every one is written as a bracketed character class,
// which is what keeps this file from matching its own gates: a class such as
// the digit class contains no digit-then-dot-then-digit run, so the pattern
// text is not an instance of the pattern.
//
// Three spellings are avoided throughout, because each hides residue rather than
// excluding anything: a fixed-width run where the width varies in practice, a
// trailing word-boundary anchor, which cannot fall between two word characters
// and so is blind to every suffixed form of the shape it anchors, and a required
// trailing segment on a shape whose tail is optional in the tree.
//
// A scenario ID takes four spellings, and they are four patterns rather than one
// because no two of them can share a leading guard. The fully spelled dotted form
// needs none, and still reports an ID buried inside a longer token. The other
// three each need a different one: without it a date, a semver prerelease and a
// release qualifier all read as scenario IDs. Adding any of those guards to the
// dotted form would drop matches it makes today, and RE2 has no lookbehind, so
// the guard character is part of the match text wherever one is used.
var (
	// scenario IDs in dot-dash form. Neither the kind token nor the segments
	// after it are enumerated: kind tokens carry digits, an ID may carry a
	// fourth segment, and the last segment is a sequence number in most
	// spellings but a word or an uppercase placeholder in others.
	scenarioIDDotDash = regexp.MustCompile(`[0-9]+\.[0-9]+-[A-Z0-9]*[A-Z][A-Z0-9]*(-[A-Z0-9]+)+`)

	// scenario IDs with the epic and story joined by a hyphen rather than a dot.
	// This spelling has no dot to tell it apart from a date, a semver
	// prerelease or a clause number, so it carries three guards the dotted form
	// does not need: the number segments are at most two digits wide, the kind
	// token is letter-led and at least three characters, and the leading
	// character proves the number does not continue leftwards.
	scenarioIDAllHyphen = regexp.MustCompile(`(^|[^-0-9A-Za-z])[0-9]{1,2}-[0-9]{1,2}-[A-Z][A-Z0-9]{2,}(-[A-Z0-9]+)*`)

	// scenario IDs with a space, and sometimes a closing bracket, where the
	// hyphen before the kind token should be. The sequence number must be
	// numeric here: a space is the commonest character in a comment, and
	// without that the shape also reads every phase label and licence name
	// that happens to follow a story number.
	scenarioIDSpacedKind = regexp.MustCompile(`(^|[^-0-9A-Za-z])[0-9]{1,2}[.-][0-9]{1,2}[\])]? [A-Z][A-Z0-9]{2,}-[0-9]{2,3}`)

	// scenario IDs citing a kind token with no sequence number after it. This is
	// the shape a release qualifier also has, so it is the most tightly guarded
	// of the four: the kind token is letters only, which excludes a qualifier
	// ending in a digit, the leading guard rejects a preceding dot, which
	// excludes the tail of a three-part version, and the trailing guard rejects a
	// following hyphen, which is what stops this pattern from also reporting
	// every fully spelled ID the dotted pattern already counts.
	scenarioIDBareKind = regexp.MustCompile(`(^|[^.0-9A-Za-z])[0-9]{1,2}\.[0-9]{1,2}-[A-Z]{3,}([^-A-Z0-9]|$)`)

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

	// story and epic number references. The number has to sit directly against
	// the word, with only separator characters between: that adjacency is the
	// whole guard, and it is what tells a reference apart from the word used in
	// prose next to an unrelated number. The separator class carries the hyphen
	// and the underscore as well as the space and the hash, because the shape
	// also occurs in a Go package name and in a hyphenated comment reference.
	//
	// The plural is not in the alternation. Neither plural takes a number
	// anywhere in this tree, and the singular cannot match inside the plural,
	// so adding it would only widen the surface with nothing behind it.
	storyOrEpicReference = regexp.MustCompile(`(?i)\b(story|epic)[ #_-]*[0-9]+([._-][0-9]+)?`)

	// numbered acceptance-suite directory paths appearing in file contents.
	// Anchored to the tests/ prefix, in both path separators, so version-bearing
	// names are not caught; it stops at the number, because a reference that
	// gives the number and not the rest of the name is the same dead pointer.
	numberedSuitePath = regexp.MustCompile(`(?i)tests[/\\][0-9]+-[0-9]+`)
)

// residuePatterns returns every residue shape, in one place so a shape added
// here cannot be left out of the gates that iterate over all of them.
func residuePatterns() []*regexp.Regexp {
	return []*regexp.Regexp{
		scenarioIDDotDash,
		scenarioIDAllHyphen,
		scenarioIDSpacedKind,
		scenarioIDBareKind,
		scenarioIDUnderscore,
		acceptanceCriterionTag,
		priorityTag,
		riskID,
		storyOrEpicReference,
		numberedSuitePath,
	}
}

func TestNoDotDashScenarioIDsInTree(t *testing.T) {
	root := projectRoot(t)
	hits := scanTree(t, root, scenarioIDDotDash)
	if len(hits) > 0 {
		t.Errorf("dot-dash scenario IDs must not appear in tracked source, comments, test names or assertion messages.\n"+
			"pattern %s matched %s", scenarioIDDotDash, reportHits(hits, 40, 25))
	}
}

func TestNoAllHyphenScenarioIDsInTree(t *testing.T) {
	root := projectRoot(t)
	hits := scanTree(t, root, scenarioIDAllHyphen)
	if len(hits) > 0 {
		t.Errorf("scenario IDs whose epic and story are joined by a hyphen must not appear in tracked source, comments,\n"+
			"test names or assertion messages. The dotted pattern cannot see these; the leading character of each match\n"+
			"below is the one the pattern had to capture to prove the number does not continue leftwards.\n"+
			"pattern %s matched %s", scenarioIDAllHyphen, reportHits(hits, 40, 25))
	}
}

func TestNoSpacedKindScenarioIDsInTree(t *testing.T) {
	root := projectRoot(t)
	hits := scanTree(t, root, scenarioIDSpacedKind)
	if len(hits) > 0 {
		t.Errorf("scenario IDs whose kind token is separated from the number by a space must not appear in tracked source,\n"+
			"comments, test names or assertion messages. Neither hyphen-joined pattern can see these.\n"+
			"pattern %s matched %s", scenarioIDSpacedKind, reportHits(hits, 40, 25))
	}
}

func TestNoBareKindScenarioIDsInTree(t *testing.T) {
	root := projectRoot(t)
	hits := scanTree(t, root, scenarioIDBareKind)
	if len(hits) > 0 {
		t.Errorf("scenario IDs citing a kind token with no sequence number must not appear in tracked source, comments,\n"+
			"test names or assertion messages. The three patterns that require a sequence number cannot see these.\n"+
			"pattern %s matched %s", scenarioIDBareKind, reportHits(hits, 40, 25))
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

// releaseDocuments are the files the story-and-epic gate does not read.
//
// The naming rules govern two surfaces: comments, test names and assertion
// messages; and the names of files, directories, packages and modules. A line
// recording which epic shipped a feature is neither. It is release documentation,
// where naming the epic is the information the reader came for, and this project
// treats a changelog, a contributing guide and a README as prose written for
// people rather than as annotation on code.
//
// Three named paths, not a pattern over markdown: an epic number in a
// testdata README was ruled residue and swept, and a blanket markdown skip would
// quietly re-admit it.
//
// The carve-out is this gate's alone. Every other gate still reads these files,
// because a scenario ID, a priority tag or a risk ID is residue wherever it turns
// up, release documentation included.
var releaseDocuments = map[string]bool{
	"CHANGELOG.md":    true,
	"CONTRIBUTING.md": true,
	"README.md":       true,
}

func TestNoStoryOrEpicNumberReferencesInTree(t *testing.T) {
	root := projectRoot(t)
	var hits []hit
	carved := map[string]int{}
	for _, h := range scanTree(t, root, storyOrEpicReference) {
		if releaseDocuments[h.path] {
			carved[h.path]++
			continue
		}
		hits = append(hits, h)
	}

	// Reported on a pass as well as a failure, so the carve-out does not have to
	// be rediscovered from the source by whoever widens this gate next.
	var carvedPaths []string
	total := 0
	for p := range releaseDocuments {
		carvedPaths = append(carvedPaths, fmt.Sprintf("%s (%d reference(s) read as content)", p, carved[p]))
		total += carved[p]
	}
	t.Logf("release documentation is outside this gate and inside every other one. %d reference(s) carved out: %s",
		total, reportPaths(carvedPaths))

	if len(hits) > 0 {
		t.Errorf("story and epic number references must not appear in tracked source, comments, test names, assertion\n"+
			"messages or Go package names. Rewrite the sentence to state what the code or the case does; where the\n"+
			"reference is a package name, rename the package after what the suite covers.\n"+
			"Release documentation is the one exception and is not scanned here: %s"+
			"pattern %s matched %s",
			reportPaths(carvedPaths), storyOrEpicReference, reportHits(hits, 40, 25))
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
		mustNotMatch [][]string
	}{
		{
			what: "dot-dash scenario IDs",
			re:   scenarioIDDotDash,
			mustMatch: [][]string{
				{"14.4-", "UNIT-002"},
				{"2.4-", "E2E-001"},           // kind token carrying a digit
				{"11.6-", "INTG-XCUT-001"},    // fourth segment
				{"13.2-", "UNIT-NNN"},         // placeholder sequence number
				{"2.5-", "UNIT-EMPTY-ROOT"},   // worded sequence segment
				{"11.5-", "INTG-", "AC", "1"}, // citation used as the kind suffix
			},
			mustNotMatch: [][]string{
				{"ISO 32000-1"}, {"14.8.4"}, {"7.3.4.2"}, {"%PDF-1.7"},
				{"v3.0.0-alpha2.117"}, {"pdfcpu 0.12.1"}, {"wails3 v3.0.0-alpha.79"},
				{"/Type1"}, {"D:20260807"}, {"12 0 obj"},
				{"1.2-", "RC1"}, {"0.12.1-", "BETA"}, // release qualifiers
			},
		},
		{
			what: "all-hyphen scenario IDs",
			re:   scenarioIDAllHyphen,
			mustMatch: [][]string{
				{"// 10-1-", "UNIT-001: renders the tree"},
				{"* 10-7-", "HOOK-004 covers the guard"},
				{"4-5-", "UNIT-001"},    // single-digit epic
				{"10-8-", "HOOK-NNN"},   // placeholder sequence number
				{"\"10-2-", "REDU-003"}, // opening quote as the leading character
				{"(10-2-", "COMP-011)"},
				{"10-1-", "UNIT"}, // kind token with no sequence number
			},
			mustNotMatch: [][]string{
				{"2026-08-07"}, {"2026-08-13T10-30-00"}, {"at 10-30-AM"},
				{"3-0-0-", "BETA-1"}, {"1.2-", "RC1"}, {"0.12.1-", "BETA"},
				{"v3.0.0-alpha2.117"}, {"pdfcpu 0.12.1"},
				{"ISO 32000-1"}, {"32000-1-2008"},
				{"14.8.4"}, {"7.3.4.2"}, {"%PDF-1.7"}, {"/Type1"}, {"D:20260807"},
				{"12 0 obj"}, {"2 0 R"}, {"1 12 T"},
				{"tests/", "14-4-shared-text-string-decoder"},
				{"10-5-ac2-soak"}, {"R-", "14-02"},
			},
		},
		{
			what: "spaced-kind scenario IDs",
			re:   scenarioIDSpacedKind,
			mustMatch: [][]string{
				{"[13.1]", " STREAM-005"}, // closing bracket before the space
				{"13.1", " STREAM-006"},
				{"tests/", "12-3", " INTG-020"},
			},
			mustNotMatch: [][]string{
				{"Story ", "13-6 ", "RED-PHASE unit tests"},       // phase label, not a kind token
				{"Story ", "11-5 ", "RED-PHASE acceptance tests"}, //
				{"enforces Apache 2.0 LICENSE + NOTICE"},          // licence version
				{"reserved bits 5, 8-16 MUST NOT render"},         // font flag bit range
				{"per the 13-4 JSON contract"},                    //
				{"Story ", "9-8 ", "AC", "4-", "AC", "10"},        // citation range
				{"ISO 32000-1"}, {"2026-08-07"}, {"12 0 obj"}, {"%PDF-1.7"},
			},
		},
		{
			what: "bare-kind scenario IDs",
			re:   scenarioIDBareKind,
			mustMatch: [][]string{
				{" * 2.9-", "UNIT: ErrorBanner severity"},
				{"// 5.3-", "UNIT: GetPageContentStreamNode"},
				{" * 2.4-", "UNIT-003 / 2.5-", "INTG: MainLayout renders"},
			},
			mustNotMatch: [][]string{
				{"1.2-", "RC1"}, {"0.12.1-", "BETA"}, {"v3.0-", "BETA"}, // release qualifiers
				{"v3.0.0-alpha2.117"}, {"3.0.0-alpha2"}, {"pdfcpu 0.12.1"},
				{"ISO 32000-1"}, {"14.8.4"}, {"7.3.4.2"}, {"%PDF-1.7"},
				{"/Type1"}, {"D:20260807"}, {"12 0 obj"},
				// a fully spelled ID belongs to the dotted gate, not this one
				{"14.4-", "UNIT-002"}, {"13.2-", "UNIT-NNN"}, {"2.5-", "UNIT-EMPTY-ROOT"},
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
			mustNotMatch: [][]string{{"Test_1_2_3"}, {"LEVEL_1_2_A"}, {"buf_16_32_bytes"}},
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
			mustNotMatch: [][]string{
				{"MAC 10"}, {"HVAC 20"}, {"U+20AC (CP1252)"}, {"hmac256"}, {"0xAC10"},
				{"ISO 32000-1"}, {"14.8.4"}, {"7.3.4.2"}, {"%PDF-1.7"}, {"/Type1"},
				{"v3.0.0-alpha2.117"}, {"pdfcpu 0.12.1"}, {"D:20260807"}, {"12 0 obj"},
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
			mustNotMatch: [][]string{
				{"/Pattern << /P0 13 0 R >>"}, {"want one P0 patternType 2"},
				{"p := arr[0]"}, {"(Page 1)"},
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
			mustNotMatch: [][]string{{"ISO 32000-1"}, {"2026-08-07"}, {"%PDF-1.7"}, {"12 0 obj"}},
		},
		{
			what: "story and epic number references",
			re:   storyOrEpicReference,
			mustMatch: [][]string{
				{"// Story ", "10-1: async plain-text load"},
				{"* Story ", "13.3: Font CMap"},
				{"Story ", "#12 covers this"}, // hash separator
				{"Epic ", "9 retro"},          // epic with no story part
				{"epic", "-7-test-design.md"}, // hyphen against the word
				{"existing Story", "-10-1 tests survive"},
				{"package story", "_12_3_wails_alpha2_103_upgrade_test"}, // Go package name
				{"STORY ", "10-1"},
			},
			mustNotMatch: [][]string{
				{"the story file"}, {"this story"}, {"user stories"}, {"per the story spec"},
				{"2 stories and 3 epics"}, {"epics 3 and 4"}, {"epic test design"},
				{"a story is fully covered"}, {"story Testing Requirements"},
				// the word next to a number that is not a reference
				{"Story says 20 today; the count is a moving target"},
				{"history 2 entries deep"}, {"the directory 12 levels down"},
				{"epicenter of the change"}, {"storyPrefixedModuleLine = regexp"},
				// the rules this suite states, which name the words but no number
				{"no story or epic numbers in directory names"},
				{"an epic-story number embedded in a file name"},
				// a task reference is a class of its own, and is not claimed here
				{"Story Task 6.1 requires"},
			},
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
			mustNotMatch: [][]string{
				{"tests/e2e/app-shell.spec.ts"}, {"tests/shared-text-string-decoder"},
				{"attestations/1-2"}, {"latest-1-2"},
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
		for _, parts := range c.mustNotMatch {
			sample := strings.Join(parts, "")
			if m := c.re.FindString(sample); m != "" {
				t.Errorf("the %s pattern matched %q in %q, which is a clause, version, date, phase label or object\n"+
					"reference rather than residue.\npattern %s", c.what, m, sample, c.re)
			}
		}
	}
}

// citationShapedToken is deliberately broader than any gate above. It matches the
// whole family a scenario ID belongs to - a short number, a short number, and an
// uppercase kind token, joined by any punctuation at all - so a spelling nobody
// has met yet still lands in it. It is far too broad to gate on directly: the
// family also holds line:column assertions, licence names and font bit ranges.
var citationShapedToken = regexp.MustCompile(
	`(^|[^-0-9A-Za-z])[0-9]{1,2}[^0-9A-Za-z]{1,2}[0-9]{1,2}[^0-9A-Za-z]{1,2}[A-Z][A-Z0-9]{2,}`)

// unclaimedShapesFixture records the citation-shaped spellings no residue
// pattern claims, as of the last time a human looked at them.
const unclaimedShapesFixture = "unclaimed-citation-shapes.txt"

// digitRun reduces a token to its spelling. The numbers are what vary between
// sites; the separators are what this gate is about, and they are what a new
// spelling changes. Recording the spelling also keeps the fixture out of its own
// scan: a line carrying no digits cannot be citation-shaped, so a recorded entry
// stops appearing once the sites behind it are swept or claimed, instead of
// holding itself in the set forever.
var digitRun = regexp.MustCompile(`[0-9]+`)

// unclaimedCitationShapes returns the citation-shaped spellings found on lines
// that no residue pattern matches, each with one site to look at. The leading
// character a pattern has to capture is trimmed so the key is the token, not its
// neighbour.
func unclaimedCitationShapes(t *testing.T, root string) map[string]hit {
	t.Helper()
	claimed := map[string]bool{}
	for _, re := range residuePatterns() {
		for _, h := range scanTree(t, root, re) {
			claimed[fmt.Sprintf("%s:%d", h.path, h.line)] = true
		}
	}
	out := map[string]hit{}
	for _, h := range scanTree(t, root, citationShapedToken) {
		if claimed[fmt.Sprintf("%s:%d", h.path, h.line)] {
			continue
		}
		spelling := digitRun.ReplaceAllString(strings.TrimLeft(h.text, " \t\"'`([{<*/,:;=&|+"), "N")
		if _, seen := out[spelling]; !seen {
			out[spelling] = h
		}
	}
	return out
}

// TestNoNewUnclaimedCitationShapes is the answer to the question the table above
// cannot answer. A table of spellings only ever holds the ones somebody has
// already run into, so it grows one incident at a time and is green in between;
// three separator spellings of one scenario ID reached this tree that way. This
// check inverts it: it enumerates the whole shape family, subtracts everything
// the gates already claim, and fails when the remainder gains a member. A
// spelling nobody has thought of therefore fails on arrival, and the failure
// names the token and where it is rather than waiting to be noticed.
//
// Shrinking the remainder needs no edit here. The fixture is a ceiling, not a
// baseline: sweeping one of these, or widening a pattern to claim it, is a pass.
func TestNoNewUnclaimedCitationShapes(t *testing.T) {
	root := projectRoot(t)
	recorded := map[string]bool{}
	for _, line := range readFixtureLines(t, unclaimedShapesFixture) {
		recorded[line] = true
	}

	var added []string
	for spelling, where := range unclaimedCitationShapes(t, root) {
		if !recorded[spelling] {
			added = append(added, fmt.Sprintf("%s (as %q at %s:%d)", spelling, where.text, where.path, where.line))
		}
	}
	if len(added) > 0 {
		t.Errorf("citation-shaped spellings that no residue pattern claims and testdata/%s does not record.\n"+
			"Each is either a spelling of a scenario ID that the gates cannot see - widen the pattern, do not sweep the\n"+
			"site and leave the gate blind - or text that only resembles one, in which case record it in the fixture with\n"+
			"the reason. Numbers are shown as N: the spelling is what this gate is about.\n"+
			"family pattern %s\nnew unclaimed spellings: %s",
			unclaimedShapesFixture, citationShapedToken, reportPaths(added))
	}
}

// TestResidueGatesDoNotMatchThemselves proves the gate files are not exempted
// from the scan by exemption but by construction. If a future edit writes a
// banned shape literally into this suite, the six gates above could never go
// green, and this check names the offender instead of leaving it to be found by
// elimination.
func TestResidueGatesDoNotMatchThemselves(t *testing.T) {
	root := projectRoot(t)
	for _, re := range residuePatterns() {
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
