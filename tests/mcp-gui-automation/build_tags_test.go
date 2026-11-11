package mcp_gui_automation_test

import (
	"regexp"
	"strings"
	"testing"
)

var undefinedIdent = regexp.MustCompile(`undefined: (\w+)`)

func TestProductionBuildWithTheMCPTagFailsOnTheGuard(t *testing.T) {
	out, err := runGo(t, nil, "vet", "-tags", "production,mcp", ".")
	if err == nil {
		t.Fatalf("go vet -tags production,mcp . succeeded; a production build with the mcp tag must not compile:\n%s", out)
	}
	for _, m := range undefinedIdent.FindAllStringSubmatch(out, -1) {
		name := strings.ToLower(m[1])
		if strings.Contains(name, "mcp") && strings.Contains(name, "production") {
			return
		}
	}
	t.Errorf("the production,mcp build failed without naming the guard identifier (an undefined name mentioning mcp and production):\n%s", out)
}

func TestMainPackageCompilesWithEachTagAlone(t *testing.T) {
	for _, tags := range []string{"mcp", "production"} {
		t.Run(tags, func(t *testing.T) {
			out, err := runGo(t, nil, "vet", "-tags", tags, ".")
			if err != nil {
				t.Errorf("go vet -tags %s . failed:\n%s", tags, out)
			}
		})
	}
}

func TestAgentPackagesAndSuiteVetWithTheirTags(t *testing.T) {
	out, err := runGo(t, nil, "vet", "-tags", "mcp,guimcp",
		"./internal/mcpclient/", "./internal/pdfservice/", "./tools/guictl/", "./tests/gui-mcp/")
	if err != nil {
		t.Fatalf("go vet -tags mcp,guimcp on the agent packages and the GUI suite failed:\n%s", out)
	}
}

func TestGuictlAndClientCompileForWindows(t *testing.T) {
	env := []string{"GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0"}
	out, err := runGo(t, env, "vet", "-tags", "mcp,guimcp", "./internal/mcpclient/", "./tools/guictl/")
	if err != nil {
		t.Fatalf("GOOS=windows go vet of guictl and the client failed:\n%s", out)
	}
}
