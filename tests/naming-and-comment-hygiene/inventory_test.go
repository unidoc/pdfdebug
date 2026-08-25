package naming_and_comment_hygiene_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	goTestFuncDecl = regexp.MustCompile(`^func ((Test|Benchmark|Fuzz)[A-Za-z0-9_]*)`)

	vitestFile     = regexp.MustCompile(`^frontend/src/.*\.test\.tsx?$`)
	playwrightFile = regexp.MustCompile(`^tests/e2e/.*\.spec\.ts$`)

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

// goTestFuncNames returns every Go test, benchmark and fuzz function name in the
// tree. These gates live in a *_test.go of their own, so this suite is excluded:
// its own function names are not part of the surface being checked.
func goTestFuncNames(t *testing.T, root string) map[string]int {
	t.Helper()
	counts := map[string]int{}
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
			}
		}
	}
	return counts
}

// TestGoTestInventoryPreserved keeps Go test function names free of scenario IDs.
// A test name is read far more often than it is written, and an ID in one tells a
// reader which spreadsheet row it came from rather than what it asserts.
//
// This gate used to also pin the absolute function count against a committed
// baseline, net of an itemized deletions fixture. That was migration-only: it
// existed to prove no test was lost while 169 ID-shaped names were rewritten, that
// rename is done and was independently verified, and as a standing check it taxed
// every later change - a new test meant editing a constant, and a constant that has
// to be edited on every PR gets edited without looking, which is the failure it
// existed to catch. The count assertions, the two baseline fixtures and the
// deletions fixture were removed together, deliberately; they are not an oversight
// to restore.
func TestGoTestInventoryPreserved(t *testing.T) {
	root := projectRoot(t)

	var idShaped []string
	for name := range goTestFuncNames(t, root) {
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

// residueInSpecTitles returns every describe / it / test title still carrying a
// residue shape.
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

// TestFrontendTestInventoryPreserved is the frontend half of the naming rule. A
// Vitest or Playwright title is a test name, so the same rule applies to it, and
// nothing else watches this surface: the flat ESLint config ignores test files and
// the TypeScript compiler does not read string contents.
//
// The Vitest and Playwright case, describe and file counts used to be asserted
// against frozen baselines here. Those were migration-only, for the same reason
// the Go count was, and were removed deliberately along with the call-site
// counting patterns that fed them. Retitling is what this gate watches now.
func TestFrontendTestInventoryPreserved(t *testing.T) {
	root := projectRoot(t)
	titleHits := append(residueInSpecTitles(t, root, vitestFile), residueInSpecTitles(t, root, playwrightFile)...)
	if len(titleHits) > 0 {
		t.Errorf("spec titles still carrying a scenario ID, a story, epic or task number, a priority tag, a risk ID or an\n"+
			"acceptance-criterion citation. Rewrite the title to state the behaviour; do not touch the block structure,\n"+
			"an assertion or a mock.\n%s", reportHits(titleHits, 40, 25))
	}
}
