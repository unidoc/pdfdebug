// Acceptance harness for the PDF text-string decoder on the CLI surface
// (`dump metadata`, `dump embedded`).
//
// Black-box: builds the pdfdebug binary and runs it against the committed
// fixture testdata/correctness/text-string-encoding.pdf. This module has its own
// go.mod, so it runs via the per-suite tests/*/ loop rather than `go test ./...`.
//
// Run: cd tests/shared-text-string-decoder && go test -v -count=1 ./...
package shared_text_string_decoder_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// fixtureName carries a UTF-16BE-with-BOM /Info /Title and a non-ASCII filespec
// /UF, both as direct objects: the call sites do not dereference.
const fixtureName = "text-string-encoding.pdf"

// Expected decoded values, plus the undecoded hex forms, which are asserted
// absent.
const (
	wantTitle    = "Rechnung Größe 中文"
	rawTitleHex  = "FEFF0052006500630068006E0075006E006700200047007200F600DF006500204E2D6587"
	wantUFName   = "größe-中文.xml"
	rawUFNameHex = "FEFF0067007200F600DF0065002D4E2D6587002E0078006D006C"

	// All-ASCII control values: they must render byte-identically, unquoted.
	asciiName    = "plain.xml"
	asciiAuthor  = "ACME GmbH"
	asciiSubject = "Plain ASCII subject"

	// rawCheckSumHex is the fixture's /Params /CheckSum, a binary hex string in
	// the same dict family as the decoded fields. It must stay raw.
	rawCheckSumHex = "DEADBEEFCAFEF00D0011223344556677"
)

// --- typed JSON shapes (Epic 13 retro P1: parse-then-assert, never grep) -----

// metadataJSON mirrors `dump metadata --json`.
type metadataJSON struct {
	Info map[string]string `json:"info"`
	XMP  string            `json:"xmp"`
}

// embeddedEntryJSON mirrors one element of `dump embedded --json`.
type embeddedEntryJSON struct {
	Name           string `json:"name"`
	FilespecRef    string `json:"filespecRef"`
	AFRelationship string `json:"afRelationship"`
	Subtype        string `json:"subtype"`
	Size           int64  `json:"size"`
	CheckSum       string `json:"checkSum"`
	ModDate        string `json:"modDate"`
}

// --- harness -----------------------------------------------------------------

// TestMain removes the temp dir holding the built CLI binary. A per-test
// t.Cleanup cannot: the binary is shared across the module via cliBuildOnce.
func TestMain(m *testing.M) {
	code := m.Run()
	if cliTmpDir != "" {
		os.RemoveAll(cliTmpDir)
	}
	os.Exit(code)
}

// projectRoot walks up to the main module's go.mod. Memoized.
func projectRoot(t *testing.T) string {
	t.Helper()
	projectRootOnce.Do(func() {
		dir, err := os.Getwd()
		if err != nil {
			return
		}
		for {
			if content, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
				if strings.Contains(string(content), "module unidoc-pdf-debugger") {
					projectRootDir = dir
					return
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				return
			}
			dir = parent
		}
	})
	if projectRootDir == "" {
		t.Fatalf("could not find project root (no go.mod with module unidoc-pdf-debugger found)")
	}
	return projectRootDir
}

// fixturePath returns the absolute path to a committed correctness-corpus fixture.
func fixturePath(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(projectRoot(t), "testdata", "correctness", name)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("fixture %s missing: %v", name, err)
	}
	return p
}

var (
	cliBuildOnce sync.Once
	cliBinPath   string
	cliBuildErr  string
	// Tracked separately from cliBinPath so a failed build still cleans up.
	cliTmpDir string

	// Memoizes the upward go.mod walk, which buildCLI and fixturePath both call
	// on every test.
	projectRootOnce sync.Once
	projectRootDir  string
)

// buildCLI compiles the CLI binary once per test package and returns its path.
// Cached via sync.Once: the binary is identical for every test in the module.
func buildCLI(t *testing.T) string {
	t.Helper()
	// Resolved before Do: projectRoot ends in t.Fatalf (runtime.Goexit), which
	// inside Do would mark the Once complete with both globals empty.
	root := projectRoot(t)
	cliBuildOnce.Do(func() {
		binName := "pdfdebug"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		tmpDir, err := os.MkdirTemp("", "pdfdebug-cli-")
		if err != nil {
			cliBuildErr = "failed to create temp dir: " + err.Error()
			return
		}
		cliTmpDir = tmpDir // before the build, so a failure still cleans up
		binPath := filepath.Join(tmpDir, binName)
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/cli/")
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			cliBuildErr = "failed to build CLI binary: " + err.Error() + "\n" + string(output)
			return
		}
		cliBinPath = binPath
	})
	if cliBuildErr != "" {
		t.Fatalf("%s", cliBuildErr)
	}
	if cliBinPath == "" {
		t.Fatal("CLI build produced no binary path (a prior buildCLI call aborted mid-Do)")
	}
	return cliBinPath
}

// runCLI executes the CLI binary with args and returns stdout, stderr, exit code.
func runCLI(t *testing.T, binPath string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run CLI: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// mustParseJSON parses s as a single JSON value into target, failing on error.
func mustParseJSON(t *testing.T, s string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(s), target); err != nil {
		t.Fatalf("failed to parse JSON: %v\nraw: %s", err, s)
	}
}

// dumpMetadataJSON runs `dump metadata --json` on the fixture and decodes it.
func dumpMetadataJSON(t *testing.T) metadataJSON {
	t.Helper()
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "metadata", "--json", fixturePath(t, fixtureName))
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	var md metadataJSON
	mustParseJSON(t, stdout, &md)
	return md
}

// dumpEmbeddedJSON runs `dump embedded --json` on the fixture and decodes it.
func dumpEmbeddedJSON(t *testing.T) []embeddedEntryJSON {
	t.Helper()
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "embedded", "--json", fixturePath(t, fixtureName))
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	var entries []embeddedEntryJSON
	mustParseJSON(t, stdout, &entries)
	return entries
}

// firstNonASCII returns the offset and byte of the first byte the plain-text
// surface should not contain, or (-1, 0) when s is clean. The window matches
// asciiSafe's (< 0x20 || > 0x7e); newlines are the one exception.
func firstNonASCII(s string) (int, byte) {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			continue
		}
		if s[i] < 0x20 || s[i] > 0x7e {
			return i, s[i]
		}
	}
	return -1, 0
}
