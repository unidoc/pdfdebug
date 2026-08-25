package naming_and_comment_hygiene_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	// a leading epic-story number on an acceptance-suite directory name.
	numberedDirName = regexp.MustCompile(`^[0-9]+-[0-9]+-`)

	// an underscore-separated all-digit token in a file name.
	nameNumberToken = regexp.MustCompile(`^[0-9]+$`)

	// a story-number prefix on an acceptance-suite module line. The trailing
	// -test / -tests suffix drift is a separate, unruled inconsistency.
	storyPrefixedModuleLine = regexp.MustCompile(`(?m)^module[ \t]+story-[0-9]+-[0-9]+-`)
)

func TestNoNumberedSuiteDirectories(t *testing.T) {
	root := projectRoot(t)
	entries, err := os.ReadDir(filepath.Join(root, "tests"))
	if err != nil {
		t.Fatalf("read tests dir: %v", err)
	}
	var numbered []string
	for _, e := range entries {
		if e.IsDir() && numberedDirName.MatchString(e.Name()) {
			numbered = append(numbered, "tests/"+e.Name())
		}
	}
	if len(numbered) > 0 {
		t.Errorf("acceptance-suite directories must be named for what they contain, with no epic or story number.\n"+
			"Unnumbered siblings already set the target style, and a numbered sibling is not a licence to copy the pattern forward.\n"+
			"numbered directories: %s", reportPaths(numbered))
	}
}

// hasEpicStoryNumberPair reports whether an underscore-separated file name
// carries an epic-story number pair in any position.
//
// The test is on the run, not on a position: an earlier version anchored to the
// _test.go tail, which let 12_3_wire_shape_test.go and wire_12_3_shape_test.go
// through. A version is what makes the run rather than the position the thing to
// look at, since pdfcpu_0_12_1_bump_test.go has to stay legal - so a run has to be
// exactly two numbers long to count, and each of them at most two digits wide.
// That keeps a three-part version out by length, a single build number out by
// length, and a four-digit standard number such as iso_8859_1 out by width.
func hasEpicStoryNumberPair(name string) bool {
	tokens := strings.Split(name, "_")
	run := 0
	shortRun := true
	for i := 0; i <= len(tokens); i++ {
		if i < len(tokens) && nameNumberToken.MatchString(tokens[i]) {
			run++
			if len(tokens[i]) > 2 {
				shortRun = false
			}
			continue
		}
		if run == 2 && shortRun {
			return true
		}
		run, shortRun = 0, true
	}
	return false
}

// TestFileNameNumberPairDetection pins the file-name rule against the names it has
// to accept as well as the ones it has to reject. The reject list is the point: an
// earlier expression anchored to the _test.go tail and let every leading and middle
// position through, and the accept list is what stops the obvious repair - matching
// any number pair anywhere - from outlawing a version.
func TestFileNameNumberPairDetection(t *testing.T) {
	numbered := []string{
		"12_3_wire_shape_test.go",
		"wire_12_3_shape_test.go",
		"wire_shape_12_3_test.go",
		"12_3_test.go",
		"5_1_helpers_test.go",
	}
	clean := []string{
		"pdfcpu_0_12_1_bump_test.go", // three-number version run
		"wails_alpha_95_upgrade_test.go",
		"wails_alpha2_117_upgrade_test.go",
		"iso_8859_1_test.go", // four-digit standard number
		"utf_16_be_test.go",
		"helpers_test.go",
		"object_source_and_reverse_refs_test.go",
		"main.go",
	}
	for _, name := range numbered {
		if !hasEpicStoryNumberPair(name) {
			t.Errorf("%q carries an epic-story number pair and the rule does not see it", name)
		}
	}
	for _, name := range clean {
		if hasEpicStoryNumberPair(name) {
			t.Errorf("%q is a legal name and the rule rejects it", name)
		}
	}
}

func TestNoNumberedFileNames(t *testing.T) {
	root := projectRoot(t)
	var numbered []string
	for _, p := range trackedFiles(t, root) {
		if hasEpicStoryNumberPair(filepath.Base(p)) {
			numbered = append(numbered, p)
		}
	}
	if len(numbered) > 0 {
		t.Errorf("file names must carry no epic or story number, in any position. These sit inside already-unnumbered\n"+
			"suites, which is why a clean directory name is not evidence of a clean suite.\n"+
			"numbered files: %s", reportPaths(numbered))
	}
}

func TestNoStoryNumberedSuiteModuleNames(t *testing.T) {
	root := projectRoot(t)
	mods, err := filepath.Glob(filepath.Join(root, "tests", "*", "go.mod"))
	if err != nil {
		t.Fatalf("glob suite modules: %v", err)
	}
	if len(mods) == 0 {
		t.Fatalf("no tests/*/go.mod found under %s", root)
	}
	var offenders []string
	for _, mod := range mods {
		data, err := os.ReadFile(mod)
		if err != nil {
			t.Fatalf("read %s: %v", mod, err)
		}
		if !storyPrefixedModuleLine.Match(data) {
			continue
		}
		rel, err := filepath.Rel(root, mod)
		if err != nil {
			rel = mod
		}
		line, _, _ := strings.Cut(strings.TrimSpace(string(data)), "\n")
		offenders = append(offenders, filepath.ToSlash(rel)+": "+strings.TrimSpace(line))
	}
	if len(offenders) > 0 {
		t.Errorf("module names must carry no epic or story number. A module name and its directory are the same fact,\n"+
			"so each of these is one line to change alongside its directory rename.\n"+
			"offending module lines: %s", reportPaths(offenders))
	}
}
