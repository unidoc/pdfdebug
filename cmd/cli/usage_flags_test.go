package main

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// flagPattern matches a long flag name in a synopsis, e.g. "--resolve-depth".
var flagPattern = regexp.MustCompile(`--[a-z][a-z-]*`)

// synopsisFlags returns the sorted, de-duplicated flag names a synopsis lists.
func synopsisFlags(s string) []string {
	flags := flagPattern.FindAllString(s, -1)
	slices.Sort(flags)
	return slices.Compact(flags)
}

// helpSynopsis returns the synopsis part of name's Commands-block line: the
// text after the command name, up to the gap that opens the description.
func helpSynopsis(t *testing.T, help, name string) string {
	t.Helper()
	line := commandLine(t, help, name+" ")
	rest := strings.TrimPrefix(line, "  "+name+" ")
	synopsis, _, _ := strings.Cut(rest, "  ")
	return synopsis
}

// commandUsageLine runs name with no arguments, which every command rejects
// with its usage line, and returns that line.
func commandUsageLine(t *testing.T, name string) string {
	t.Helper()
	args := append([]string{"pdfdebug"}, strings.Fields(name)...)
	stderr := captureStderr(t, func() { dispatch(args) })
	first, _, _ := strings.Cut(stderr, "\n")
	prefix := "Usage: pdfdebug " + name + " "
	if !strings.HasPrefix(first, prefix) {
		t.Fatalf("`%s` with no arguments: first stderr line is not its usage line: %q", name, first)
	}
	return first
}

// Each command's line in `pdfdebug --help` and the usage line it prints on a
// usage error list the same flags, so neither advertises a flag the other
// leaves out.
func TestHelpLineAndUsageLineListSameFlags(t *testing.T) {
	help := usageText(t)
	names := usageCommandNames(t, help)
	if len(names) == 0 {
		t.Fatal("no commands parsed out of the help text")
	}
	for _, name := range names {
		fromHelp := synopsisFlags(helpSynopsis(t, help, name))
		fromUsage := synopsisFlags(commandUsageLine(t, name))
		if !slices.Equal(fromHelp, fromUsage) {
			t.Errorf("`%s`: help line lists %v, usage line lists %v", name, fromHelp, fromUsage)
		}
	}
}

// flagValues gives a valid value for each flag that takes one; a flag absent
// here is passed as a boolean.
var flagValues = map[string]string{
	"--depth":         "1",
	"--page":          "1",
	"--resolve-depth": "1",
	"--ref":           "1 0 R",
	"--xobject":       "X",
	"--info":          "1",
	"--forms-depth":   "1",
	"--section":       "geometry",
	"--profile":       "pdfa-1b",
	"--name":          "x",
}

// requiredArgs holds the flags a command needs before it gets past argument
// checking, so a probe with one extra flag reaches the file open.
var requiredArgs = map[string][]string{
	"dump object":      {"--ref", "1 0 R"},
	"dump stream":      {"--page", "1"},
	"dump page":        {"--info", "1"},
	"dump font":        {"--ref", "1 0 R"},
	"dump image":       {"--ref", "1 0 R"},
	"dump source":      {"--ref", "1 0 R"},
	"dump reverserefs": {"--ref", "1 0 R"},
}

// acceptsFlag reports whether name's flag parser accepts flagName. An unknown
// flag fails parsing and prints the usage line; an accepted one gets past
// parsing to the missing file (or a flag-value check), which does not.
func acceptsFlag(t *testing.T, name, flagName string, files []string) bool {
	t.Helper()
	args := append([]string{"pdfdebug"}, strings.Fields(name)...)
	args = append(args, requiredArgs[name]...)
	args = append(args, flagName)
	if v, ok := flagValues[flagName]; ok {
		args = append(args, v)
	}
	args = append(args, files...)
	stderr := captureStderr(t, func() { dispatch(args) })
	return !strings.Contains(stderr, "Usage: pdfdebug ")
}

// A command's usage line lists exactly the flags its parser accepts. Every flag
// named by any command's help or usage line is probed against every command: a
// listed flag has to parse, and an unlisted one has to be rejected.
func TestUsageLineListsExactlyTheAcceptedFlags(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.pdf")
	help := usageText(t)
	names := usageCommandNames(t, help)

	listed := map[string][]string{}
	var known []string
	for _, name := range names {
		listed[name] = synopsisFlags(commandUsageLine(t, name))
		known = append(known, listed[name]...)
		known = append(known, synopsisFlags(helpSynopsis(t, help, name))...)
	}
	slices.Sort(known)
	known = slices.Compact(known)

	for _, name := range names {
		files := []string{missing}
		if name == "diff" {
			files = []string{missing, missing}
		}
		for _, flagName := range known {
			isListed := slices.Contains(listed[name], flagName)
			accepted := acceptsFlag(t, name, flagName, files)
			switch {
			case isListed && !accepted:
				t.Errorf("`%s` usage line lists %s but the command rejects it", name, flagName)
			case !isListed && accepted:
				t.Errorf("`%s` accepts %s but its usage line does not list it", name, flagName)
			}
		}
	}
}
