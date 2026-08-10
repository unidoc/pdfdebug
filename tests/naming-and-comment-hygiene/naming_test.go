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

	// an epic-story number embedded in a Go test file name. Anchored to the
	// _test.go tail so version-bearing file names are not caught.
	numberedTestFileName = regexp.MustCompile(`_[0-9]+_[0-9]+_test\.go$`)

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

func TestNoNumberedTestFileNames(t *testing.T) {
	root := projectRoot(t)
	var numbered []string
	for _, p := range trackedFiles(t, root) {
		if numberedTestFileName.MatchString(p) {
			numbered = append(numbered, p)
		}
	}
	if len(numbered) > 0 {
		t.Errorf("test file names must carry no epic or story number. These sit inside already-unnumbered suites,\n"+
			"which is why a clean directory name is not evidence of a clean suite.\n"+
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
