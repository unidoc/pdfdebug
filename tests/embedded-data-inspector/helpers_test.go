// Story 13-2 acceptance test harness for the CLI resources `dump embedded` and
// `dump metadata`.
//
// Black-box: build the pdfdebug CLI binary and run it as a subprocess. Failures
// surface at RUNTIME (unknown resource / wrong output shape / wrong exit code),
// not at compile time, so the main `unidoc-pdf-debugger` module keeps building
// green (mirrors 13-1 / 11-6).
//
// Test pyramid: every case here is a Go integration-level black-box test
// against the built CLI binary -- the project's established level for CLI
// acceptance. No browser/E2E layer is warranted; the CLI surface has no UI.
//
// Fixtures are hand-rolled raw PDF bytes written to a temp file (same technique
// as testdata/generate_test.go). The fixtures must parse under pdfcpu default
// validation -- verified locally during ATDD authoring -- which is why the
// /EmbeddedFile /Subtype is a Name with #2F escapes (/text#2Fxml), not a
// string literal.
//
// Naming: [Px] per the story Testing Requirements.
//
// Run: cd tests/embedded-data-inspector && go test -v -count=1 ./...
package embedded_data_inspector_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// projectRoot walks up from the test directory to find the main module's go.mod.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if content, err := os.ReadFile(goModPath); err == nil {
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

var (
	cliBuildOnce sync.Once
	cliBinPath   string
	cliBuildErr  string
)

// buildCLI compiles the CLI binary once per test package and returns its path.
// The build is cached via sync.Once: the binary is identical for every test in
// the module, so the expensive `go build` runs a single time instead of once
// per test. The build dir is a plain os.MkdirTemp (not t.TempDir) because it is
// shared across tests and must outlive any single one; the OS reclaims it.
func buildCLI(t *testing.T) string {
	t.Helper()
	cliBuildOnce.Do(func() {
		root := projectRoot(t)
		binName := "pdfdebug"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		tmpDir, err := os.MkdirTemp("", "pdfdebug-cli-")
		if err != nil {
			cliBuildErr = "failed to create temp dir: " + err.Error()
			return
		}
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
	return cliBinPath
}

// runCLI executes the CLI binary with args and returns stdout, stderr, exit code.
// stdout is returned as a string but extraction payloads are binary-safe (the
// CLI writes raw bytes); callers comparing exact bytes use runCLIBytes.
func runCLI(t *testing.T, binPath string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	out, errOut, ec := runCLIBytes(t, binPath, args...)
	return string(out), string(errOut), ec
}

// runCLIBytes is the byte-exact variant for extraction payload assertions.
func runCLIBytes(t *testing.T, binPath string, args ...string) (stdout, stderr []byte, exitCode int) {
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
	return []byte(outBuf.String()), []byte(errBuf.String()), exitCode
}

// mustParseJSON parses s as a single JSON value into target, failing on error.
func mustParseJSON(t *testing.T, s string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(s), target); err != nil {
		t.Fatalf("failed to parse JSON: %v\nraw: %s", err, s)
	}
}

// parsesAsJSON reports whether s (trimmed) is a single well-formed JSON value.
func parsesAsJSON(s string) bool {
	var v any
	return json.Unmarshal([]byte(strings.TrimSpace(s)), &v) == nil
}

// assertNotJSON fails when out is a top-level JSON object/array document. The
// plain-text default must NOT parse as a JSON document (13-1 contract).
func assertNotJSON(t *testing.T, out string) {
	t.Helper()
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return
	}
	if (trimmed[0] == '{' || trimmed[0] == '[') && parsesAsJSON(trimmed) {
		t.Fatalf("default output parsed as a JSON document; expected plain text:\n%s", out)
	}
}

// assertASCII fails when out contains a non-ASCII byte (13-1 plain-text contract).
func assertASCII(t *testing.T, out string) {
	t.Helper()
	for i := 0; i < len(out); i++ {
		if out[i] > 0x7f {
			t.Errorf("plain-text output contains non-ASCII byte 0x%02x at offset %d", out[i], i)
			return
		}
	}
}

// assertTrailingNewline fails when out does not end in a newline (13-1 contract).
func assertTrailingNewline(t *testing.T, out string) {
	t.Helper()
	if out == "" {
		return
	}
	if !strings.HasSuffix(out, "\n") {
		t.Errorf("plain-text output does not end with a trailing newline")
	}
}

// writeTempPDF writes content to a temp file and returns its path.
func writeTempPDF(t *testing.T, name string, content []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write temp pdf: %v", err)
	}
	return path
}

// --- fixture builders (mirror the pdfcore co-located fixtures) ---------------

func pad10(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 10 {
		s = "0" + s
	}
	return s
}

// assemblePDF stitches a header, object bodies (object i+1), an xref table, and
// a trailer with /Root and optional /Info. infoNum == 0 omits /Info.
func assemblePDF(objs []string, rootNum, infoNum int) []byte {
	body := "%PDF-1.7\n"
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = len(body)
		body += o
	}
	xrefOff := len(body)
	size := len(objs) + 1
	xref := "xref\n0 " + strconv.Itoa(size) + "\n0000000000 65535 f \n"
	for _, off := range offsets {
		xref += pad10(off) + " 00000 n \n"
	}
	trailer := "trailer\n<< /Size " + strconv.Itoa(size) + " /Root " + strconv.Itoa(rootNum) + " 0 R"
	if infoNum > 0 {
		trailer += " /Info " + strconv.Itoa(infoNum) + " 0 R"
	}
	trailer += " >>\nstartxref\n" + strconv.Itoa(xrefOff) + "\n%%EOF\n"
	return []byte(body + xref + trailer)
}

// embeddedStreamObj returns an /EmbeddedFile stream object with /text#2Fxml
// subtype and a /Params dict; the payload is the unfiltered stream body.
func embeddedStreamObj(num int, payload string) string {
	return strconv.Itoa(num) + " 0 obj\n" +
		"<< /Type /EmbeddedFile /Subtype /text#2Fxml /Length " + strconv.Itoa(len(payload)) +
		" /Params << /Size " + strconv.Itoa(len(payload)) +
		" /ModDate (D:20240101000000Z) >> >>\n" +
		"stream\n" + payload + "\nendstream\nendobj\n"
}

func filespecObj(num, efNum int, displayName, afRel string) string {
	return strconv.Itoa(num) + " 0 obj\n" +
		"<< /Type /Filespec /F (" + displayName + ") /UF (" + displayName + ") " +
		"/AFRelationship /" + afRel + " " +
		"/EF << /F " + strconv.Itoa(efNum) + " 0 R /UF " + strconv.Itoa(efNum) + " 0 R >> >>\nendobj\n"
}

// embeddedPDF builds the canonical single-attachment ZUGFeRD-style fixture: one
// XML attachment (object 4) reachable from /AF, name tree, sharing /Filespec 6.
// The XML payload is returned alongside so extraction round-trip can assert it.
func embeddedPDF(xml string) []byte {
	return assemblePDF([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AF [6 0 R] /Names << /EmbeddedFiles 7 0 R >> >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		embeddedStreamObj(4, xml),
		"5 0 obj\nnull\nendobj\n",
		filespecObj(6, 4, "factur-x.xml", "Data"),
		"7 0 obj\n<< /Names [(factur-x.xml) 6 0 R] >>\nendobj\n",
	}, 1, 0)
}

// twoAttachmentPDF builds a fixture with TWO attachments that share the SAME
// display name "dup.xml" (objects 4 and 8 via filespecs 6 and 9) so the --name
// multi-match guard can be exercised. A third uniquely-named attachment
// "solo.xml" (object 10 via filespec 11) supports single-match extraction.
func twoAttachmentPDF() []byte {
	return assemblePDF([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AF [6 0 R 9 0 R 11 0 R] >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		embeddedStreamObj(4, "<dup>one</dup>"),
		"5 0 obj\nnull\nendobj\n",
		filespecObj(6, 4, "dup.xml", "Data"),
		"7 0 obj\nnull\nendobj\n",
		embeddedStreamObj(8, "<dup>two</dup>"),
		filespecObj(9, 8, "dup.xml", "Data"),
		embeddedStreamObj(10, "<solo>only</solo>"),
		filespecObj(11, 10, "solo.xml", "Source"),
	}, 1, 0)
}

const cliXMPPacket = "<?xpacket begin=\"\"?><x:xmpmeta>marker-VERBATIM-XMP</x:xmpmeta><?xpacket end=\"w\"?>"

// metadataPDF builds a doc with catalog /Metadata (object 4) and trailer /Info
// (object 5) carrying Title/Author/Producer.
func metadataPDF() []byte {
	meta := "4 0 obj\n<< /Type /Metadata /Subtype /XML /Length " + strconv.Itoa(len(cliXMPPacket)) + " >>\n" +
		"stream\n" + cliXMPPacket + "\nendstream\nendobj\n"
	info := "5 0 obj\n<< /Title (Invoice 2024-001) /Author (ACME GmbH) /Producer (pdfdebug-test) >>\nendobj\n"
	return assemblePDF([]string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R /Metadata 4 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n",
		"3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n",
		meta,
		info,
	}, 1, 5)
}
