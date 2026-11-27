package mcp_gui_automation_test

import (
	"regexp"
	"strings"
	"testing"
)

// contributingSection returns the body of the CONTRIBUTING.md section whose
// heading contains title, up to the next heading of the same or higher level.
func contributingSection(t *testing.T, title string) string {
	t.Helper()
	content := readRepoFile(t, "CONTRIBUTING.md")
	heading := regexp.MustCompile(`(?m)^(#{1,6})\s+(.*)$`)
	locs := heading.FindAllStringSubmatchIndex(content, -1)
	for i, loc := range locs {
		text := content[loc[4]:loc[5]]
		if !strings.Contains(strings.ToLower(text), strings.ToLower(title)) {
			continue
		}
		level := loc[3] - loc[2]
		end := len(content)
		for _, next := range locs[i+1:] {
			if next[3]-next[2] <= level {
				end = next[0]
				break
			}
		}
		return content[loc[1]:end]
	}
	t.Fatalf("CONTRIBUTING.md has no %q section", title)
	return ""
}

func TestContributingDocumentsGUIAutomation(t *testing.T) {
	section := contributingSection(t, "GUI automation")
	for _, want := range []string{
		"task build:mcp",
		"wails3 task build:mcp",
		"go run ./tools/guictl",
		"guictl start",
		"guictl open",
		"guictl pick",
		"guictl tools",
		"guictl call",
		"guictl status",
		"guictl stop",
		"--hide-cursor",
		"WAILS_MCP=1 wails3 dev",
		"WAILS_MCP_TOKEN",
		"WAILS_MCP_HOST",
		"isTrusted",
		"localStorage",
		"apparmor_parser",
		"claude mcp add --transport http pdfdebug-gui http://127.0.0.1:9099/mcp",
		"tests/gui-mcp",
		"--no-gui",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("CONTRIBUTING.md GUI automation section does not mention %q", want)
		}
	}
	if strings.Contains(section, "export WAILS_MCP") {
		t.Error("CONTRIBUTING.md tells the reader to export WAILS_MCP; it is set for one command only")
	}
}

func TestTestsReadmeDescribesTheGoSuites(t *testing.T) {
	readme := readRepoFile(t, "tests/README.md")
	for _, want := range []string{"boot-smoke", "gui-mcp", "go.mod", "-tags guimcp"} {
		if !strings.Contains(readme, want) {
			t.Errorf("tests/README.md does not mention %q", want)
		}
	}
}

// A CHANGELOG version section records the GUI automation tooling, under
// [Unreleased] or after its release is cut.
func TestChangelogRecordsGUIAutomation(t *testing.T) {
	wants := []string{"guictl", "build:mcp"}
	for _, sec := range strings.Split(readRepoFile(t, "CHANGELOG.md"), "\n## [")[1:] {
		if strings.Contains(sec, wants[0]) && strings.Contains(sec, wants[1]) {
			return
		}
	}
	t.Errorf("no CHANGELOG.md version section mentions both %q and %q", wants[0], wants[1])
}
