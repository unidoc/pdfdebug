package naming_and_comment_hygiene_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// workflowPath is the CI workflow holding the Windows suite subset.
const workflowPath = ".github/workflows/ci.yml"

// perModuleStepName is the workflow step that discovers acceptance suites by
// glob and decides, per suite, whether the Windows runner executes it.
const perModuleStepName = "Test acceptance suites (per-module)"

// windowsSubsetSize is the number of entries the Windows leg is expected to
// run. The step echoes this count, so the Windows job log can be checked
// mechanically rather than by eye.
const windowsSubsetSize = 12

// windowsSubset extracts the entries of the hardcoded bash array naming the
// suites the Windows runner executes.
func windowsSubset(t *testing.T, root string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(workflowPath)))
	if err != nil {
		t.Fatalf("read %s: %v", workflowPath, err)
	}
	var entries []string
	inArray := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if !inArray {
			if strings.HasPrefix(trimmed, "win_subset=(") {
				inArray = true
			}
			continue
		}
		if trimmed == ")" {
			return entries
		}
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			entries = append(entries, trimmed)
		}
	}
	if !inArray {
		t.Fatalf("no win_subset=( array found in %s", workflowPath)
	}
	t.Fatalf("win_subset=( array in %s is not closed", workflowPath)
	return nil
}

// TestWindowsSubsetNamesExistingUnnumberedSuites is the gate on the one failure
// mode of the directory rename that reports success: win_subset is a bash string
// array, so a renamed directory does not fail to match, it silently fails the
// membership test and the Windows job prints a skip line and carries on green
// with fewer suites covered.
func TestWindowsSubsetNamesExistingUnnumberedSuites(t *testing.T) {
	root := projectRoot(t)
	entries := windowsSubset(t, root)

	if len(entries) != windowsSubsetSize {
		t.Errorf("win_subset in %s has %d entries, expected %d. The step echoes this count on the Windows leg,\n"+
			"so changing it changes the log line the Definition of Done reads. Adding a suite to the Windows leg is a\n"+
			"deliberate decision, not a side effect of a rename.", workflowPath, len(entries), windowsSubsetSize)
	}

	var missing, numbered []string
	for _, name := range entries {
		if _, err := os.Stat(filepath.Join(root, "tests", name, "go.mod")); err != nil {
			missing = append(missing, name)
		}
		if numberedDirName.MatchString(name) {
			numbered = append(numbered, name)
		}
	}
	if len(missing) > 0 {
		t.Errorf("win_subset entries with no tests/<name>/go.mod. Each one is a suite silently dropped from Windows\n"+
			"coverage while CI stays green.\nunresolved entries: %s", reportPaths(missing))
	}
	if len(numbered) > 0 {
		t.Errorf("win_subset entries still carrying an epic-story prefix. These name directories that the naming rule\n"+
			"requires renamed, so the array has to be rewritten in the same commit as, or immediately before, the rename.\n"+
			"numbered entries: %s", reportPaths(numbered))
	}
}

// TestPerModuleStepGuardsWindowsSubsetResolution requires the workflow itself to
// fail on an unresolvable win_subset entry, so the mistake above cannot recur
// silently once this story's renames are done. The step runs on every runner, so
// a bad entry is caught on the first CI run rather than on the next Windows leg.
func TestPerModuleStepGuardsWindowsSubsetResolution(t *testing.T) {
	root := projectRoot(t)
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(workflowPath)))
	if err != nil {
		t.Fatalf("read %s: %v", workflowPath, err)
	}
	text := string(data)

	idx := strings.Index(text, perModuleStepName)
	if idx < 0 {
		t.Fatalf("step %q not found in %s", perModuleStepName, workflowPath)
	}
	// The step's script ends where the next step begins.
	body := text[idx:]
	if next := regexp.MustCompile(`(?m)^      - name: `).FindStringIndex(body); next != nil {
		body = body[:next[0]]
	}

	hasExistenceTest := strings.Contains(body, `-d "tests/`)
	hasHardFailure := strings.Contains(body, "exit 1")
	if !hasExistenceTest || !hasHardFailure {
		t.Errorf("the %q step must fail hard when a win_subset entry has no tests/<name>/ directory.\n"+
			"Found a tests/ directory existence test: %v. Found a non-zero exit: %v.\n"+
			"Without it, a renamed suite turns into a skip line and the job reports success with less coverage.",
			perModuleStepName, hasExistenceTest, hasHardFailure)
	}
}
