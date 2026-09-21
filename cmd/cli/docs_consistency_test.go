package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// removalVersionPattern extracts the semver a removal statement names, e.g.
// "will be removed in 0.6.0" or "is removed in 0.6.0".
var removalVersionPattern = regexp.MustCompile(`removed in (\d+\.\d+\.\d+)`)

// commandNamePattern matches a command table's first cell. Flag rows (`--json`
// and friends) do not match, so only the command table contributes.
var commandNamePattern = regexp.MustCompile(`^(dump [a-z]+|validate|diff)$`)

// repoRoot is the project root relative to this package directory.
const repoRoot = "../.."

// readRepoDoc reads a Markdown file relative to the project root.
func readRepoDoc(t *testing.T, relPath string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repoRoot, relPath))
	if err != nil {
		t.Fatalf("%s not found: %v", relPath, err)
	}
	return string(content)
}

// docChunks splits Markdown into the units a single statement occupies: a list
// item or table row is one chunk, and a run of prose lines is joined into one
// so a sentence that wraps across lines stays whole.
func docChunks(content string) []string {
	var chunks []string
	var prose []string
	flush := func() {
		if len(prose) > 0 {
			chunks = append(chunks, strings.Join(prose, " "))
			prose = nil
		}
	}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			flush()
		case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "), strings.HasPrefix(trimmed, "|"):
			flush()
			chunks = append(chunks, trimmed)
		default:
			prose = append(prose, trimmed)
		}
	}
	flush()
	return chunks
}

// aliasRemovalVersions returns every removal version named by a chunk that also
// mentions the deprecated spelling, so an unrelated deprecation elsewhere in
// the file cannot contribute.
func aliasRemovalVersions(content string) []string {
	var versions []string
	for _, chunk := range docChunks(content) {
		if !strings.Contains(chunk, "dump plaintext") {
			continue
		}
		for _, m := range removalVersionPattern.FindAllStringSubmatch(chunk, -1) {
			versions = append(versions, m[1])
		}
	}
	return versions
}

// The removal version lives in three places: the stderr notice, the CLI usage
// doc and the changelog entry. Only the notice is pinned by an equality
// assertion, so this ties the other two to it. The docs are matched on the
// number alone, within the statement that names the deprecated spelling, so
// rewording either file is free and changing the version in one place is not.
func TestAliasRemovalVersionAgreesAcrossDocs(t *testing.T) {
	noticeMatch := removalVersionPattern.FindStringSubmatch(deprecatedPlaintextNotice)
	if noticeMatch == nil {
		t.Fatalf("the deprecation notice names no removal version: %s", deprecatedPlaintextNotice)
	}
	noticeVersion := noticeMatch[1]

	for _, docPath := range []string{"docs/cli-usage.md", "CHANGELOG.md"} {
		versions := aliasRemovalVersions(readRepoDoc(t, docPath))
		if len(versions) == 0 {
			t.Errorf("%s names no removal version alongside the deprecated spelling; the notice announces %s and the doc has to say the same",
				docPath, noticeVersion)
			continue
		}
		for _, v := range versions {
			if v != noticeVersion {
				t.Errorf("%s says the alias is removed in %s, the notice says %s", docPath, v, noticeVersion)
			}
		}
	}
}

// usageCommandNames returns the commands printUsage lists in its Commands
// block, e.g. "dump bytes", "validate".
func usageCommandNames(t *testing.T, help string) []string {
	t.Helper()
	lines := strings.Split(help, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "Commands (") {
			start = i + 1
			break
		}
	}
	if start < 0 {
		t.Fatalf("help text has no Commands block")
	}

	var names []string
	for _, line := range lines[start:] {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			break
		}
		name := fields[0]
		if name == "dump" && len(fields) > 1 {
			name = "dump " + fields[1]
		}
		names = append(names, name)
	}
	return names
}

// docCommandNames returns the commands the usage doc's command table lists.
func docCommandNames(content string) []string {
	var names []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			continue
		}
		cells := strings.Split(trimmed, "|")
		if len(cells) < 2 {
			continue
		}
		first := strings.Trim(strings.TrimSpace(cells[1]), "`")
		if commandNamePattern.MatchString(first) {
			names = append(names, first)
		}
	}
	return names
}

// Every command the help advertises has a row in the usage doc's command
// table, and the table lists nothing the help does not advertise. Compares name
// sets, not descriptions, so the doc's prose stays free to change; a renamed
// command that reached the help but not the table fails, and so does a table
// row left behind under the old spelling.
func TestUsageCommandsMatchDocTable(t *testing.T) {
	fromUsage := usageCommandNames(t, usageText(t))
	fromDoc := docCommandNames(readRepoDoc(t, "docs/cli-usage.md"))
	if len(fromUsage) == 0 {
		t.Fatalf("no commands parsed out of the help text")
	}
	if len(fromDoc) == 0 {
		t.Fatalf("no command rows parsed out of docs/cli-usage.md")
	}

	documented := make(map[string]bool, len(fromDoc))
	for _, name := range fromDoc {
		documented[name] = true
	}
	advertised := make(map[string]bool, len(fromUsage))
	for _, name := range fromUsage {
		advertised[name] = true
	}

	var undocumented, stale []string
	for name := range advertised {
		if !documented[name] {
			undocumented = append(undocumented, name)
		}
	}
	for name := range documented {
		if !advertised[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(undocumented)
	sort.Strings(stale)

	if len(undocumented) > 0 {
		t.Errorf("commands in the help text with no row in the docs/cli-usage.md command table: %s",
			strings.Join(undocumented, ", "))
	}
	if len(stale) > 0 {
		t.Errorf("rows in the docs/cli-usage.md command table naming commands the help text does not list: %s",
			strings.Join(stale, ", "))
	}
}
