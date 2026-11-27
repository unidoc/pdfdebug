package mcp_gui_automation_test

import (
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

var goBuildTags = regexp.MustCompile(`-tags[ =]"?([^\s"]+)`)

// dryRunGoBuilds runs `wails3 task <target> --dry` and returns the go build
// command lines it would run.
func dryRunGoBuilds(t *testing.T, target string) []string {
	t.Helper()
	requireTool(t, "wails3")
	out, err := runInRoot(t, 2*time.Minute, nil, "wails3", "task", target, "--dry")
	if err != nil {
		t.Fatalf("wails3 task %s --dry failed: %v\n%s", target, err, out)
	}
	var builds []string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "go build") {
			builds = append(builds, line)
		}
	}
	return builds
}

func buildTags(line string) []string {
	m := goBuildTags.FindStringSubmatch(line)
	if m == nil {
		return nil
	}
	return strings.Split(m[1], ",")
}

func hasTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}

func TestBuildMCPTargetBuildsTheAgentBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("build:mcp supports macOS and Linux only")
	}
	builds := dryRunGoBuilds(t, "build:mcp")
	var gui string
	for _, line := range builds {
		if strings.Contains(line, "bin/pdfdebug-mcp") {
			gui = line
		}
	}
	if gui == "" {
		t.Fatalf("wails3 task build:mcp would not build bin/pdfdebug-mcp; go build lines:\n%s", strings.Join(builds, "\n"))
	}
	tags := buildTags(gui)
	if !hasTag(tags, "mcp") {
		t.Errorf("build:mcp go build has tags %v, want mcp:\n%s", tags, gui)
	}
	if hasTag(tags, "production") {
		t.Errorf("build:mcp go build has the production tag; the agent build is a DEV build:\n%s", gui)
	}
}

func TestShippedBuildTargetsNeverPassTheMCPTag(t *testing.T) {
	for _, target := range []string{"build", "package"} {
		t.Run(target, func(t *testing.T) {
			builds := dryRunGoBuilds(t, target)
			if len(builds) == 0 {
				t.Fatalf("wails3 task %s --dry shows no go build", target)
			}
			for _, line := range builds {
				if hasTag(buildTags(line), "mcp") {
					t.Errorf("wails3 task %s would build with the mcp tag:\n%s", target, line)
				}
				if strings.Contains(line, "pdfdebug-mcp") {
					t.Errorf("wails3 task %s would build the agent binary:\n%s", target, line)
				}
			}
		})
	}
}
