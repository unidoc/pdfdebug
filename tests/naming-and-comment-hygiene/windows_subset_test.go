package naming_and_comment_hygiene_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// workflowPath is the CI workflow holding the Windows suite subset.
const workflowPath = ".github/workflows/ci.yml"

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
	for line := range strings.SplitSeq(string(data), "\n") {
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
//
// The entry count is deliberately not asserted, and neither is the shape of the
// workflow's own resolution guard. Both were migration-era checks: the first froze
// a number that any deliberate coverage change has to edit anyway, and the second
// regex-matched bash text inside the workflow, which is brittle and is now
// redundant with the hard failure the step performs itself. Resolving each entry
// to a real directory is the property worth keeping, so that is all this checks.
func TestWindowsSubsetNamesExistingUnnumberedSuites(t *testing.T) {
	root := projectRoot(t)
	entries := windowsSubset(t, root)
	if len(entries) == 0 {
		t.Fatalf("win_subset in %s is empty, so the Windows leg runs no acceptance suite at all", workflowPath)
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
