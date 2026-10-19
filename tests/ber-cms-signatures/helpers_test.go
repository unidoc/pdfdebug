// Acceptance harness for `dump signatures` on BER-encoded CMS signatures.
//
// Black-box: the pdfdebug CLI is built once and run as a subprocess over the
// generated signature fixtures in testdata/. Covers output shape and exit
// codes only; normalizer edge cases live in the pdfcore unit tests.
//
// JSON field asserted here (additive to the existing signature entry):
//
//	cmsEncoding  string  "DER" | "BER" | "" (payload did not decode to a
//	                     complete element, or the SubFilter is not CMS)
//
// Run: cd tests/ber-cms-signatures && go test -count=1 ./...
package ber_cms_signatures_test

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

// projectRoot walks up from the test directory to the main module's go.mod.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	for {
		if content, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			if strings.Contains(string(content), "module unidoc-pdf-debugger") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root (no go.mod with module unidoc-pdf-debugger found)")
		}
		dir = parent
	}
}

// fixture returns the absolute path of a file in testdata/, failing when it
// is missing.
func fixture(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(projectRoot(t), "testdata", name)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("fixture %s missing: %v", name, err)
	}
	return p
}

var (
	cliBuildOnce sync.Once
	cliBinPath   string
	cliBuildErr  string
)

// buildCLI compiles the CLI once per test package and returns the binary path.
func buildCLI(t *testing.T) string {
	t.Helper()
	cliBuildOnce.Do(func() {
		binName := "pdfdebug"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		tmpDir, err := os.MkdirTemp("", "pdfdebug-ber-cms-")
		if err != nil {
			cliBuildErr = "failed to create temp dir: " + err.Error()
			return
		}
		binPath := filepath.Join(tmpDir, binName)
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/cli/")
		cmd.Dir = projectRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			cliBuildErr = "failed to build CLI binary: " + err.Error() + "\n" + string(out)
			return
		}
		cliBinPath = binPath
	})
	if cliBuildErr != "" {
		t.Fatalf("%s", cliBuildErr)
	}
	return cliBinPath
}

// runCLI runs the CLI with args and returns stdout, stderr and the exit code.
func runCLI(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(buildCLI(t), args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("failed to run CLI: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// certInfo is one entry of the certificates array.
type certInfo struct {
	Subject string `json:"subject"`
	Issuer  string `json:"issuer"`
	Serial  string `json:"serial"`
}

// sigEntry is the subset of a `dump signatures --json` entry this suite reads.
// CMSEncoding is a pointer so a missing key is told apart from "".
type sigEntry struct {
	FieldName        string     `json:"fieldName"`
	Signed           bool       `json:"signed"`
	SubFilter        string     `json:"subFilter"`
	DigestAlgorithm  string     `json:"digestAlgorithm"`
	Signer           *certInfo  `json:"signer"`
	SignerIdentified bool       `json:"signerIdentified"`
	Certificates     []certInfo `json:"certificates"`
	DecomposeError   string     `json:"decomposeError"`
	CMSEncoding      *string    `json:"cmsEncoding"`
}

// dumpSignaturesJSON runs `dump signatures --json` on a fixture, requires
// exit 0 and exactly one entry, and returns it.
func dumpSignaturesJSON(t *testing.T, name string) sigEntry {
	t.Helper()
	stdout, stderr, code := runCLI(t, "dump", "signatures", "--json", fixture(t, name))
	if code != 0 {
		t.Fatalf("dump signatures --json %s exit = %d, want 0\nstderr: %s", name, code, stderr)
	}
	var entries []sigEntry
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("--json output is not a signature array: %v\n%s", err, stdout)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d signature entries for %s, want 1\n%s", len(entries), name, stdout)
	}
	return entries[0]
}

// cmsEncoding returns the cmsEncoding value, failing when the key is absent.
func cmsEncoding(t *testing.T, e sigEntry) string {
	t.Helper()
	if e.CMSEncoding == nil {
		t.Fatalf("entry %q has no cmsEncoding key", e.FieldName)
	}
	return *e.CMSEncoding
}

// plainLine returns the value of the first "Label: value" line in a
// plain-text block, and whether the label was found.
func plainLine(out, label string) (string, bool) {
	for line := range strings.SplitSeq(out, "\n") {
		if rest, ok := strings.CutPrefix(line, label+":"); ok {
			return strings.TrimSpace(rest), true
		}
	}
	return "", false
}
