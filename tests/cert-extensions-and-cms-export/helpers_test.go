// Acceptance harness for the certificate extension facts of `dump signatures`
// and its --export-cms, --export-certs and --field flags.
//
// Black-box: the pdfdebug CLI is built once and run as a subprocess over the
// signature fixtures in testdata/. Covers typed --json, plain-text rows, exit
// codes, the files written and stdout staying unchanged by the export flags.
//
// JSON keys read here on each certificate (and on signer):
//
//	keyUsage               []string | null
//	extKeyUsage            [{oid, name}] | null
//	crlDistributionPoints  []string | null
//	ocspServers            []string | null
//	caIssuers              []string | null
//
// null means the extension is absent from the certificate; [] means it is
// present with nothing to list.
//
// Run: cd tests/cert-extensions-and-cms-export && go test -count=1 ./...
package cert_extensions_and_cms_export_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
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
		tmpDir, err := os.MkdirTemp("", "pdfdebug-cert-ext-")
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

// extKeyUsage is one extKeyUsage entry.
type extKeyUsage struct {
	OID  string `json:"oid"`
	Name string `json:"name"`
}

// certInfo is one certificate as `dump signatures --json` prints it.
type certInfo struct {
	Subject               string        `json:"subject"`
	Issuer                string        `json:"issuer"`
	Serial                string        `json:"serial"`
	KeyUsage              []string      `json:"keyUsage"`
	ExtKeyUsage           []extKeyUsage `json:"extKeyUsage"`
	CRLDistributionPoints []string      `json:"crlDistributionPoints"`
	OCSPServers           []string      `json:"ocspServers"`
	CAIssuers             []string      `json:"caIssuers"`
}

// sigEntry is the subset of a `dump signatures --json` entry this suite reads.
type sigEntry struct {
	FieldName      string     `json:"fieldName"`
	Signed         bool       `json:"signed"`
	SubFilter      string     `json:"subFilter"`
	Signer         *certInfo  `json:"signer"`
	Certificates   []certInfo `json:"certificates"`
	DecomposeError string     `json:"decomposeError"`
	ByteRange      []int64    `json:"byteRange"`
}

// rawSigEntry keeps each certificate as a key -> raw value map so a JSON null
// is told apart from a missing key.
type rawSigEntry struct {
	Signer       map[string]json.RawMessage   `json:"signer"`
	Certificates []map[string]json.RawMessage `json:"certificates"`
}

// dumpJSON runs `dump signatures --json` on path, requires exit 0, and
// returns the typed entries, the raw entries and stdout.
func dumpJSON(t *testing.T, path string) ([]sigEntry, []rawSigEntry, string) {
	t.Helper()
	stdout, stderr, code := runCLI(t, "dump", "signatures", "--json", path)
	if code != 0 {
		t.Fatalf("dump signatures --json %s exit = %d, want 0\nstderr: %s", filepath.Base(path), code, stderr)
	}
	var entries []sigEntry
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("--json output is not a signature array: %v\n%s", err, stdout)
	}
	var raw []rawSigEntry
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("--json output is not a signature array: %v\n%s", err, stdout)
	}
	return entries, raw, stdout
}

// oneEntry returns the only entry, failing when there is not exactly one.
func oneEntry[T any](t *testing.T, entries []T) T {
	t.Helper()
	if len(entries) != 1 {
		t.Fatalf("got %d signature entries, want 1", len(entries))
	}
	return entries[0]
}

// contentsPayload returns the decoded /Contents hex string of a single-
// signature fixture, padding included.
func contentsPayload(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	start := bytes.Index(data, []byte("/Contents <"))
	if start < 0 {
		t.Fatalf("%s: no /Contents hex string", path)
	}
	start += len("/Contents <")
	end := bytes.IndexByte(data[start:], '>')
	if end < 0 {
		t.Fatalf("%s: unterminated /Contents hex string", path)
	}
	raw, err := hex.DecodeString(string(data[start : start+end]))
	if err != nil {
		t.Fatalf("%s: /Contents is not hex: %v", path, err)
	}
	return raw
}

// signedBytes returns the file bytes covered by a four-entry /ByteRange.
func signedBytes(t *testing.T, path string, br []int64) []byte {
	t.Helper()
	if len(br) != 4 {
		t.Fatalf("byteRange = %v, want 4 integers", br)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	out := append([]byte{}, data[br[0]:br[0]+br[1]]...)
	return append(out, data[br[2]:br[2]+br[3]]...)
}

// dirNames returns the sorted names in dir.
func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

// assertNoFile fails when path exists.
func assertNoFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("%s was written, want no file", path)
	}
}

// assertJSONErrorObject fails unless stderr is one JSON object with a
// non-empty "error" string and no usage line.
func assertJSONErrorObject(t *testing.T, stderr string) {
	t.Helper()
	if strings.Contains(stderr, "Usage: pdfdebug ") {
		t.Errorf("stderr has a usage line, want only a JSON error object:\n%s", stderr)
	}
	var obj struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &obj); err != nil || obj.Error == "" {
		t.Errorf("stderr is not a JSON error object (%v):\n%s", err, stderr)
	}
}

// plainCertRow returns the value of the row labelled label among the
// indented rows under the "  cert N:" row of a plain-text listing.
func plainCertRow(t *testing.T, out, certRow, label string) string {
	t.Helper()
	lines := strings.Split(out, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, certRow+":") {
			continue
		}
		for _, next := range lines[i+1:] {
			if !strings.HasPrefix(next, "    ") {
				break
			}
			if rest, ok := strings.CutPrefix(strings.TrimSpace(next), label+":"); ok {
				return strings.TrimSpace(rest)
			}
		}
		t.Fatalf("no %q row under %q in:\n%s", label, certRow, out)
	}
	t.Fatalf("no %q row in:\n%s", certRow, out)
	return ""
}
