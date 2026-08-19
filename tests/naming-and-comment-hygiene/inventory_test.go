package naming_and_comment_hygiene_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Pre-change Go inventory, measured across every tracked *_test.go at the
// baseline commit. 1361 of the names are behavioural and must survive
// untouched; the remaining 169 are ID-shaped and are the ones being renamed.
const (
	baselineGoTestFuncTotal = 1530
	baselineIDShapedNames   = 169
)

// Pre-change frontend inventory, measured over the tracked Vitest and Playwright
// specs at the baseline commit. Nothing else watches this surface: the flat
// ESLint config ignores every test file and the TypeScript compiler does not
// read string contents, so a case deleted while its title was being rewritten
// would leave no trace at all.
const (
	baselineVitestFiles     = 60
	baselineVitestCases     = 814
	baselineVitestDescribes = 323
	baselinePlaywrightFiles = 6
	baselinePlaywrightCases = 12
)

// The modifier chains Vitest and Playwright accept. describe has no .fails.
const (
	describeModifiers = "(each|skip|only|todo|concurrent|sequential|runIf|skipIf)"
	caseModifiers     = "(each|skip|only|todo|concurrent|sequential|fails|runIf|skipIf)"
)

// specCall builds the line pattern for one call name. Three shapes count, and the
// scan is line by line, so each has to be recognisable from its own line:
//
//   - a modifier chain followed by '(' or by a backtick, the backtick being the
//     template-table form, test.each`…`;
//   - '(' then the opening quote of the title;
//   - '(' then end of line, which is how a formatter wraps a long title onto the
//     next line.
//
// The leading class rejects a member call on some other object, and also rejects a
// preceding backtick: prose in a doc comment writes the call in an inline code
// span, and `describe.skip` would otherwise be counted as a block. The quoted or
// wrapped shapes need that guard too, since prose writes the call name followed by
// an empty pair of parentheses.
func specCall(name, modifiers string) *regexp.Regexp {
	return regexp.MustCompile("(^|[^\\w.$`])" + name + "((\\." + modifiers + ")+(\\(|`)|\\([ \t]*(['\"`]|$))")
}

var (
	goTestFuncDecl = regexp.MustCompile(`^func ((Test|Benchmark|Fuzz)[A-Za-z0-9_]*)`)

	vitestFile     = regexp.MustCompile(`^frontend/src/.*\.test\.tsx?$`)
	playwrightFile = regexp.MustCompile(`^tests/e2e/.*\.spec\.ts$`)

	// describe / it / test call sites, built by specCall below.
	describeCall   = specCall("describe", describeModifiers)
	caseCall       = specCall("(it|test)", caseModifiers)
	playwrightCase = specCall("test", caseModifiers)

	// the title argument of a describe / it / test call, up to its closing quote.
	specTitle = regexp.MustCompile("(^|[^\\w.$])(describe|it|test)(\\.(each|skip|only|todo|concurrent|sequential|fails|runIf|skipIf))*\\([ \t]*(['\"`])([^'\"`]*)")
)

// approvedDeletion is one itemized removal, parsed from a fixture line: the name
// the function carried when it was deleted, plus, when that is not the name it
// carried at baseline, the digest that ties it back to one the baseline held.
type approvedDeletion struct {
	name           string
	baselineDigest string
	raw            string
}

// approvedDeletions parses the deletions fixture. The comment tail is cut first,
// so a grep target mentioning a function name cannot be read as a field.
func approvedDeletions(t *testing.T) []approvedDeletion {
	t.Helper()
	var out []approvedDeletion
	for _, line := range readFixtureLines(t, "approved-test-deletions.txt") {
		data, _, _ := strings.Cut(line, "#")
		fields := strings.Fields(data)
		if len(fields) == 0 {
			t.Fatalf("deletions fixture line has no function name before its comment: %q", line)
		}
		entry := approvedDeletion{name: fields[0], raw: line}
		for _, f := range fields[1:] {
			if digest, ok := strings.CutPrefix(f, "baseline:"); ok {
				entry.baselineDigest = digest
			}
		}
		out = append(out, entry)
	}
	return out
}

// baselineNameDigest is how a baseline name is tied to a fixture entry without
// spelling the name out. Sixteen hex characters is 64 bits, which is far more
// than 169 names need to stay distinct, and is short enough to sit on one line.
func baselineNameDigest(name string) string {
	sum := sha256.Sum256([]byte(name))
	return hex.EncodeToString(sum[:])[:16]
}

// readFixtureLines returns the fixture's content lines, dropping blanks and
// comments.
func readFixtureLines(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var lines []string
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// goTestFuncOccurrences counts every Go test, benchmark and fuzz function name
// in the tree, keyed by name. These gates live in a *_test.go of their own, so
// this suite is excluded: the baseline it measures must not include the
// functions doing the measuring.
func goTestFuncOccurrences(t *testing.T, root string) (map[string]int, int) {
	t.Helper()
	counts := map[string]int{}
	total := 0
	for _, rel := range trackedFiles(t, root) {
		if !strings.HasSuffix(rel, "_test.go") || strings.HasPrefix(rel, thisSuite+"/") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		for line := range strings.SplitSeq(string(data), "\n") {
			if m := goTestFuncDecl.FindStringSubmatch(line); m != nil {
				counts[m[1]]++
				total++
			}
		}
	}
	return counts, total
}

// TestGoTestInventoryPreserved is the only thing that catches a test lost during
// the rename pass. A dropped Test prefix, or a name collision resolved by
// deleting one of the pair, removes coverage while the suite still passes.
//
// Three properties together pin the rename down without committing a snapshot of
// the ID-shaped names into the tree: the total is preserved net of deliberate
// deletions, every behavioural name survives, and no ID-shaped name is left.
func TestGoTestInventoryPreserved(t *testing.T) {
	root := projectRoot(t)
	current, total := goTestFuncOccurrences(t, root)

	behavioural := readFixtureLines(t, "go-test-func-baseline.txt")
	idShapedDigests := readFixtureLines(t, "go-test-func-baseline-id-shaped-digests.txt")
	if len(behavioural)+len(idShapedDigests) != baselineGoTestFuncTotal || len(idShapedDigests) != baselineIDShapedNames {
		t.Fatalf("the two baseline fixtures hold %d behavioural names and %d ID-shaped digests, which is %d, but the "+
			"constants say %d with %d ID-shaped; the fixtures and the constants describe different trees",
			len(behavioural), len(idShapedDigests), len(behavioural)+len(idShapedDigests),
			baselineGoTestFuncTotal, baselineIDShapedNames)
	}

	want := map[string]int{}
	for _, name := range behavioural {
		want[name]++
	}
	byDigest := map[string]bool{}
	for _, d := range idShapedDigests {
		byDigest[d] = true
	}

	// The two halves have to be disjoint. A behavioural name whose digest is also
	// listed as ID-shaped would let a deletion entry resolve the wrong way round,
	// skipping the by-name check that is the stronger of the two.
	var overlap []string
	for _, name := range behavioural {
		if byDigest[baselineNameDigest(name)] {
			overlap = append(overlap, name)
		}
	}
	if len(overlap) > 0 {
		sort.Strings(overlap)
		t.Fatalf("names present in both baseline fixtures, once by name and once by digest. The digest fixture holds the\n"+
			"ID-shaped half only; regenerate it as the baseline names minus testdata/go-test-func-baseline.txt.\n"+
			"names in both: %s", reportPaths(overlap))
	}

	// Each entry has to resolve to a function the baseline held, and its function
	// has to be gone. An entry that resolves to nothing used to decrement an
	// absent key to -1, which no later check could trip, so a typo lowered the
	// expected total and certified a loss that was never looked at.
	resolved := 0
	var unresolved, stale []string
	for _, entry := range approvedDeletions(t) {
		switch {
		case want[entry.name] > 0:
			want[entry.name]--
			resolved++
		case entry.baselineDigest != "" && byDigest[entry.baselineDigest]:
			resolved++
		default:
			unresolved = append(unresolved, entry.raw)
		}
		if current[entry.name] > 0 {
			stale = append(stale, fmt.Sprintf("%s (still declared %d time(s))", entry.name, current[entry.name]))
		}
	}
	if len(unresolved) > 0 {
		sort.Strings(unresolved)
		t.Errorf("itemized deletions that resolve to no function the baseline commit held. A behavioural name has to appear\n"+
			"in testdata/go-test-func-baseline.txt; a function renamed out of the ID-shaped half before deletion needs\n"+
			"baseline:<digest> naming the digest of the name it had at baseline. Until an entry resolves it is not\n"+
			"subtracted from the expected total, so this failure and the count failure below appear together.\n"+
			"unresolvable entries: %s", reportPaths(unresolved))
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("itemized deletions whose function is still declared in the tree. Each entry claims a removal that did not\n"+
			"happen, so it is subtracting from the expected total for nothing.\nstale entries: %s", reportPaths(stale))
	}

	wantTotal := baselineGoTestFuncTotal - resolved
	if total != wantTotal {
		remedy := "A deliberate removal belongs in testdata/approved-test-deletions.txt with its file, its grep target and\n" +
			"its triage bucket; that fixture is the only thing that may lower the expected total."
		if total > wantTotal {
			remedy = "A test was added, not removed. An addition is not itemized as a deletion - putting it in\n" +
				"testdata/approved-test-deletions.txt moves the expected total further from the tree. Raise\n" +
				"baselineGoTestFuncTotal instead, in the same commit as the new test."
		}
		t.Errorf("Go test/benchmark/fuzz function count is %d, expected %d (%d at baseline, %d resolved deletion(s)).\n"+
			"A rename must not change the count.\n%s", total, wantTotal, baselineGoTestFuncTotal, resolved, remedy)
	}

	var lost []string
	for name, n := range want {
		if got := current[name]; got < n {
			lost = append(lost, fmt.Sprintf("%s (expected %d, found %d)", name, n, got))
		}
	}
	if len(lost) > 0 {
		sort.Strings(lost)
		t.Errorf("behavioural test function names that vanished without being itemized as deletions. None of these was\n"+
			"scheduled to change, so each is either an accidental rename or a lost test.\nmissing: %s", reportPaths(lost))
	}

	var idShaped []string
	for name := range current {
		if scenarioIDUnderscore.MatchString(name) {
			idShaped = append(idShaped, name)
		}
	}
	if len(idShaped) > 0 {
		t.Errorf("test function names still carrying a scenario ID. Rename each to what it asserts, and rewrite its own\n"+
			"doc comment in the same edit; resolve any collision by making the behavioural name more specific, never by\n"+
			"deleting a test.\nID-shaped names remaining: %s", reportPaths(idShaped))
	}
}

// TestSpecCallPatternsSeeEveryCallShape is the frontend counters' equivalent of the
// residue patterns' spelling gate. These counters are the only thing watching the
// frontend surface, so a shape they cannot see is a case that can be deleted with
// nothing to show for it, and the counts stay reassuringly stable while it happens.
//
// Every sample is one line, because the scan is line by line.
func TestSpecCallPatternsSeeEveryCallShape(t *testing.T) {
	cases := []struct {
		what         string
		re           *regexp.Regexp
		mustMatch    []string
		mustNotMatch []string
	}{
		{
			what: "case calls",
			re:   caseCall,
			mustMatch: []string{
				"  it('does a thing', () => {",
				"  test(\"does a thing\", () => {",
				"  test(`does a thing`, () => {",
				"  test.skip('pending', () => {",
				"  test.each([",
				"    test.each(cases)('parses %j', (input, expected) => {",
				"  test.each`", // template table
				"  it(",        // title wrapped onto the next line
				"  test(  ",    // ... with trailing whitespace
				"  it.concurrent.skip(",
			},
			mustNotMatch: []string{
				"// prose mentioning it() and test() with empty parens",
				" * a doc comment naming `test.skip` in an inline code span",
				" * and `it.each` likewise",
				"  suite.it('member call on another object', () => {",
				"  awaitIt('not the call at all', () => {",
				"  expect(x).toBe(1);",
			},
		},
		{
			what: "describe calls",
			re:   describeCall,
			mustMatch: []string{
				"describe('DetailPanel', () => {",
				"describe.skip('pending block', () => {",
				"  describe.each`",
				"describe(",
			},
			mustNotMatch: []string{
				"// prose mentioning describe() with empty parens",
				" * a doc comment naming `describe.skip` in an inline code span",
				"  helper.describe('member call', () => {",
				"import { describe, test, expect } from 'vitest';",
			},
		},
	}

	for _, c := range cases {
		for _, sample := range c.mustMatch {
			if c.re.FindString(sample) == "" {
				t.Errorf("the %s counter cannot see %q. A shape it cannot see is a case that can be deleted without\n"+
					"moving the count.\npattern %s", c.what, sample, c.re)
			}
		}
		for _, sample := range c.mustNotMatch {
			if m := c.re.FindString(sample); m != "" {
				t.Errorf("the %s counter matched %q in %q, which is prose, an import or a call on another object.\n"+
					"pattern %s", c.what, m, sample, c.re)
			}
		}
	}
}

// countCalls sums the call sites of re across the tracked files matching pick.
func countCalls(t *testing.T, root string, pick, re *regexp.Regexp) (int, []string) {
	t.Helper()
	total := 0
	var files []string
	for _, rel := range trackedFiles(t, root) {
		if !pick.MatchString(rel) {
			continue
		}
		files = append(files, rel)
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		for line := range strings.SplitSeq(string(data), "\n") {
			total += len(re.FindAllString(line, -1))
		}
	}
	return total, files
}

// residueInSpecTitles returns every describe / it / test title still carrying a
// scenario ID, a priority tag, a risk ID or an acceptance-criterion citation.
func residueInSpecTitles(t *testing.T, root string, pick *regexp.Regexp) []hit {
	t.Helper()
	patterns := residuePatterns()
	var hits []hit
	for _, rel := range trackedFiles(t, root) {
		if !pick.MatchString(rel) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			for _, m := range specTitle.FindAllStringSubmatch(line, -1) {
				title := m[6]
				for _, re := range patterns {
					if found := re.FindString(title); found != "" {
						hits = append(hits, hit{path: rel, line: i + 1, text: found + "  in: " + title})
						break
					}
				}
			}
		}
	}
	return hits
}

// TestFrontendTestInventoryPreserved is the frontend half of the count gate.
// A Vitest title is a test name, so the same rule applies to it, and the retitle
// pass has no compiler or linter behind it - the count is the whole guard.
func TestFrontendTestInventoryPreserved(t *testing.T) {
	root := projectRoot(t)

	cases, files := countCalls(t, root, vitestFile, caseCall)
	describes, _ := countCalls(t, root, vitestFile, describeCall)
	if len(files) != baselineVitestFiles {
		t.Errorf("found %d Vitest files, expected %d", len(files), baselineVitestFiles)
	}
	if cases != baselineVitestCases {
		t.Errorf("Vitest case count is %d, expected %d. Retitling must not add or remove a case.", cases, baselineVitestCases)
	}
	if describes != baselineVitestDescribes {
		t.Errorf("Vitest describe count is %d, expected %d. Retitling must not restructure a block.", describes, baselineVitestDescribes)
	}

	pwCases, pwFiles := countCalls(t, root, playwrightFile, playwrightCase)
	if len(pwFiles) != baselinePlaywrightFiles {
		t.Errorf("found %d Playwright specs, expected %d", len(pwFiles), baselinePlaywrightFiles)
	}
	if pwCases != baselinePlaywrightCases {
		t.Errorf("Playwright case count is %d, expected %d. The per-suite CI loop skips these specs, so nothing else "+
			"counts them.", pwCases, baselinePlaywrightCases)
	}

	titleHits := append(residueInSpecTitles(t, root, vitestFile), residueInSpecTitles(t, root, playwrightFile)...)
	if len(titleHits) > 0 {
		t.Errorf("spec titles still carrying a scenario ID, a priority tag, a risk ID or an acceptance-criterion citation.\n"+
			"Rewrite the title to state the behaviour; do not touch the block structure, an assertion or a mock.\n%s",
			reportHits(titleHits, 40, 25))
	}
}
