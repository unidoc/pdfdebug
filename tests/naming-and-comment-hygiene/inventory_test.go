package naming_and_comment_hygiene_test

import (
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

var (
	goTestFuncDecl = regexp.MustCompile(`^func ((Test|Benchmark|Fuzz)[A-Za-z0-9_]*)`)

	vitestFile     = regexp.MustCompile(`^frontend/src/.*\.test\.tsx?$`)
	playwrightFile = regexp.MustCompile(`^tests/e2e/.*\.spec\.ts$`)

	// describe / it / test call sites. The leading class rejects member calls on
	// some other object. The bare form additionally requires a quoted first
	// argument, because prose in a doc comment writes the call name followed by
	// an empty pair of parentheses and would otherwise be counted as a case; the
	// modifier form needs no such guard, since prose does not spell those out.
	describeCall   = regexp.MustCompile("(^|[^\\w.$])describe((\\.(each|skip|only|todo|concurrent|sequential|runIf|skipIf))+\\(|\\([ \t]*['\"`])")
	caseCall       = regexp.MustCompile("(^|[^\\w.$])(it|test)((\\.(each|skip|only|todo|concurrent|sequential|fails|runIf|skipIf))+\\(|\\([ \t]*['\"`])")
	playwrightCase = regexp.MustCompile("(^|[^\\w.$])test\\([ \t]*['\"`]")

	// the title argument of a describe / it / test call, up to its closing quote.
	specTitle = regexp.MustCompile("(^|[^\\w.$])(describe|it|test)(\\.(each|skip|only|todo|concurrent|sequential|fails|runIf|skipIf))*\\([ \t]*(['\"`])([^'\"`]*)")
)

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
	if len(behavioural) != baselineGoTestFuncTotal-baselineIDShapedNames {
		t.Fatalf("baseline fixture holds %d names but the constants say %d - %d = %d; the fixture and the constants "+
			"describe different trees", len(behavioural), baselineGoTestFuncTotal, baselineIDShapedNames,
			baselineGoTestFuncTotal-baselineIDShapedNames)
	}
	approved := readFixtureLines(t, "approved-test-deletions.txt")

	wantTotal := baselineGoTestFuncTotal - len(approved)
	if total != wantTotal {
		t.Errorf("Go test/benchmark/fuzz function count is %d, expected %d (%d at baseline, %d itemized deletion(s)).\n"+
			"A rename must not change the count. Every deliberate removal belongs in testdata/approved-test-deletions.txt\n"+
			"with its file, its grep target and its triage bucket.", total, wantTotal, baselineGoTestFuncTotal, len(approved))
	}

	want := map[string]int{}
	for _, name := range behavioural {
		want[name]++
	}
	for _, name := range approved {
		want[strings.Fields(name)[0]]--
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
	patterns := []*regexp.Regexp{
		scenarioIDDotDash, scenarioIDUnderscore, acceptanceCriterionTag, priorityTag, riskID,
	}
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
