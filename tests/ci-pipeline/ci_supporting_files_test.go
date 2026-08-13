// Package ci_pipeline_test: supporting-file acceptance tests for Story 7.1.
//
// Validates the files produced alongside .github/workflows/ci.yml:
//   - frontend/eslint.config.js (ESLint 9 flat config)
//   - frontend/package.json devDependencies + `lint` + `typecheck` scripts
//   - .golangci.yml (v2 schema)
//   - .gitattributes (line-ending normalization)
//
// These are TDD RED PHASE tests -- they MUST fail until Story 7.1 is implemented.
// Run: cd tests/ci-pipeline && go test -v -count=1 ./...
package ci_pipeline_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// readFileAtRoot reads a file relative to project root, failing the test if absent.
func readFileAtRoot(t *testing.T, relPath string) string {
	t.Helper()
	root := projectRoot(t)
	content, err := os.ReadFile(filepath.Join(root, relPath))
	if err != nil {
		t.Fatalf("%s not found: %v", relPath, err)
	}
	return string(content)
}

// ---------------------------------------------------------------------------
// frontend/eslint.config.js exists and is flat config Covers Task 3.4
// (ESLint 9 flat config)
// ---------------------------------------------------------------------------

func TestESLintFlatConfigExists(t *testing.T) {

	content := readFileAtRoot(t, "frontend/eslint.config.js")

	// Flat config uses `export default [...]` or `module.exports = [...]`
	if !strings.Contains(content, "export default") && !strings.Contains(content, "module.exports") {
		t.Errorf("frontend/eslint.config.js must export a flat config array (ESLint 9 form)")
	}

	// typescript-eslint meta-package must be imported (Task 3.4)
	if !strings.Contains(content, "typescript-eslint") {
		t.Errorf("frontend/eslint.config.js must import `typescript-eslint` meta-package (Task 3.4)")
	}

	// React + React Hooks plugins required
	if !strings.Contains(content, "eslint-plugin-react") {
		t.Errorf("frontend/eslint.config.js must import eslint-plugin-react")
	}
	if !strings.Contains(content, "eslint-plugin-react-hooks") {
		t.Errorf("frontend/eslint.config.js must import eslint-plugin-react-hooks")
	}

	// Must use projectService (not legacy `project: './tsconfig.json'`) per Task 3.4
	if !strings.Contains(content, "projectService") {
		t.Errorf("frontend/eslint.config.js must use `projectService: true` instead of legacy `project:` form (Task 3.4 rationale: repo tsconfig excludes test files)")
	}

	// Strict rules that MUST be present (Task 3.4 + Dev Notes absolute rules)
	if !strings.Contains(content, "no-explicit-any") {
		t.Errorf("frontend/eslint.config.js must enforce @typescript-eslint/no-explicit-any (absolute rule)")
	}
	if !strings.Contains(content, "no-unused-vars") {
		t.Errorf("frontend/eslint.config.js must enforce @typescript-eslint/no-unused-vars")
	}

	// Ignores block is required (Task 3.4 bulletpoint)
	if !strings.Contains(content, "ignores") {
		t.Errorf("frontend/eslint.config.js must include an `ignores` entry (Task 3.4)")
	}
	for _, ignore := range []string{"dist", "bindings", "wailsjs", "node_modules"} {
		if !strings.Contains(content, ignore) {
			t.Errorf("frontend/eslint.config.js ignores must include %q", ignore)
		}
	}
}

// ---------------------------------------------------------------------------
// frontend/package.json has lint + typecheck scripts Covers Task 3.3
// ---------------------------------------------------------------------------

func TestFrontendPackageScripts(t *testing.T) {

	raw := readFileAtRoot(t, "frontend/package.json")
	var pkg struct {
		Scripts         map[string]string `json:"scripts"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal([]byte(raw), &pkg); err != nil {
		t.Fatalf("frontend/package.json: invalid JSON: %v", err)
	}

	lint, ok := pkg.Scripts["lint"]
	if !ok {
		t.Errorf("frontend/package.json: scripts.lint missing (Task 3.3)")
	} else {
		if !strings.Contains(lint, "eslint") {
			t.Errorf("scripts.lint must invoke eslint, got %q", lint)
		}
		if !strings.Contains(lint, "--max-warnings 0") {
			t.Errorf("scripts.lint must include `--max-warnings 0` (Task 3.3)")
		}
	}

	tc, ok := pkg.Scripts["typecheck"]
	if !ok {
		t.Errorf("frontend/package.json: scripts.typecheck missing (Task 3.3)")
	} else if !strings.Contains(tc, "tsc --noEmit") {
		t.Errorf("scripts.typecheck must invoke `tsc --noEmit`, got %q", tc)
	}
}

// ---------------------------------------------------------------------------
// frontend/package.json has required ESLint devDependencies Covers Task 3.2
// (flat-config compatible versions)
// ---------------------------------------------------------------------------

func TestFrontendESLintDevDependencies(t *testing.T) {

	raw := readFileAtRoot(t, "frontend/package.json")
	var pkg struct {
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal([]byte(raw), &pkg); err != nil {
		t.Fatalf("frontend/package.json: invalid JSON: %v", err)
	}

	// Required deps with minimum major versions (flat-config compat).
	// Values here are regex matching the minimum-major constraint from Task 3.2.
	required := map[string]*regexp.Regexp{
		"eslint":                    regexp.MustCompile(`\^?(9|[1-9][0-9])\b`),
		"typescript-eslint":         regexp.MustCompile(`\^?(8|9|[1-9][0-9])\b`),
		"eslint-plugin-react":       regexp.MustCompile(`\^?7\.(3[7-9]|[4-9][0-9]|[1-9][0-9]{2,})|\^?[89]\.`),
		"eslint-plugin-react-hooks": regexp.MustCompile(`\^?5\.[1-9]|\^?[6-9]\.|\^?[1-9][0-9]`),
		"globals":                   regexp.MustCompile(`\^?(1[5-9]|[2-9][0-9])\b`),
	}

	for dep, re := range required {
		ver, ok := pkg.DevDependencies[dep]
		if !ok {
			t.Errorf("frontend/package.json devDependencies missing %q (Task 3.2)", dep)
			continue
		}
		if !re.MatchString(ver) {
			t.Errorf("frontend/package.json devDependencies[%q] = %q does not satisfy flat-config minimum (Task 3.2)", dep, ver)
		}
	}
}

// ---------------------------------------------------------------------------
// .golangci.yml uses v2 schema with required linters Covers Task 4.1
// ---------------------------------------------------------------------------

func TestGolangciLintV2Config(t *testing.T) {

	raw := readFileAtRoot(t, ".golangci.yml")

	var cfg map[string]interface{}
	if err := yaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf(".golangci.yml: invalid YAML: %v", err)
	}

	// Must declare version "2"
	ver, ok := cfg["version"].(string)
	if !ok {
		// yaml.v3 may parse "2" as int
		if i, ok := cfg["version"].(int); ok {
			ver = yamlInt(i)
		}
	}
	if ver != "2" {
		t.Errorf(".golangci.yml: version must be \"2\" (required for Go 1.26 + v2 schema), got %q", ver)
	}

	// linters.default: none and required enabled linters
	linters, ok := cfg["linters"].(map[string]interface{})
	if !ok {
		t.Fatalf(".golangci.yml: linters block missing")
	}
	if d, ok := linters["default"].(string); !ok || d != "none" {
		t.Errorf(".golangci.yml: linters.default must be \"none\" (Task 4.1), got %q", d)
	}
	enabledRaw, _ := linters["enable"].([]interface{})
	enabled := map[string]bool{}
	for _, e := range enabledRaw {
		if s, ok := e.(string); ok {
			enabled[s] = true
		}
	}
	for _, req := range []string{"errcheck", "govet", "ineffassign", "staticcheck", "unused"} {
		if !enabled[req] {
			t.Errorf(".golangci.yml: linters.enable must include %q (Task 4.1)", req)
		}
	}

	// Go directive must be "1.26" under run:
	run, ok := cfg["run"].(map[string]interface{})
	if !ok {
		t.Fatalf(".golangci.yml: run block missing")
	}
	if g, ok := run["go"].(string); !ok || !strings.HasPrefix(g, "1.26") {
		t.Errorf(".golangci.yml: run.go must pin 1.26, got %q", g)
	}
}

// yamlInt is a tiny helper so gopkg.in/yaml.v3's possible int-parsing of the
// `version:` field still lets TestGolangciLintV2Config report the value as a string.
func yamlInt(i int) string {
	if i == 2 {
		return "2"
	}
	return ""
}

// ---------------------------------------------------------------------------
// .gitattributes normalizes line endings Covers Task 6.1
// (risk mitigation E7-R-005)
// ---------------------------------------------------------------------------

func TestGitattributesLineEndings(t *testing.T) {

	content := readFileAtRoot(t, ".gitattributes")

	if !strings.Contains(content, "* text=auto eol=lf") {
		t.Errorf(".gitattributes: must contain `* text=auto eol=lf` (Task 6.1)")
	}

	// Bash shell scripts must be LF
	if !strings.Contains(content, "*.sh text eol=lf") {
		t.Errorf(".gitattributes: must force LF for *.sh (Task 6.1)")
	}

	// Windows batch files must be CRLF
	if !strings.Contains(content, "*.bat text eol=crlf") {
		t.Errorf(".gitattributes: must force CRLF for *.bat (Task 6.1)")
	}
	if !strings.Contains(content, "*.cmd text eol=crlf") {
		t.Errorf(".gitattributes: must force CRLF for *.cmd (Task 6.1)")
	}
}
