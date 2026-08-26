// Acceptance test harness for the extraction ceiling on compressed streams.
//
// Black-box: build the pdfdebug CLI binary and run it as a subprocess, so the
// two extraction paths (`dump embedded` and `dump image`) are exercised the way
// a script or CI pipe would. Failures surface at RUNTIME, not at compile time,
// so the main `unidoc-pdf-debugger` module keeps building green.
//
// Fixtures are hand-rolled raw PDF bytes assembled in memory and written to a
// temp file, mirroring tests/embedded-data-inspector. They carry FlateDecode
// streams, which is why they are built here instead of being added to
// testdata/correctness/ -- that corpus is uncompressed by invariant.
//
// Run: cd tests/bounded-decode && go test -v -count=1 ./...
package bounded_decode_test

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"errors"
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

// runCLIBytes executes the CLI binary with args and returns stdout, stderr and
// the exit code. Extraction payloads are binary, so both streams stay []byte.
func runCLIBytes(t *testing.T, binPath string, args ...string) (stdout, stderr []byte, exitCode int) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run CLI: %v", err)
		}
	}
	return outBuf.Bytes(), errBuf.Bytes(), exitCode
}

// imageJSON mirrors the fields of the `dump image --json` payload this suite
// asserts on. Decoding into a typed struct keeps the assertions immune to
// field-order and sibling-key changes.
type imageJSON struct {
	ObjectRef string `json:"objectRef"`
	MimeType  string `json:"mimeType"`
	Base64    string `json:"base64"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Filter    string `json:"filter"`
	Error     string `json:"error"`
}

// decodeImageJSON parses `dump image --json` output, failing on malformed JSON.
func decodeImageJSON(t *testing.T, out []byte) imageJSON {
	t.Helper()
	var img imageJSON
	if err := json.Unmarshal(out, &img); err != nil {
		t.Fatalf("failed to parse image JSON: %v\nraw: %s", err, string(out))
	}
	return img
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

// --- payload builders -------------------------------------------------------

// zlibBytes returns payload compressed with the zlib wrapper /FlateDecode
// expects.
func zlibBytes(t *testing.T, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("zlib write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}
	return buf.Bytes()
}

// zlibZeros returns the zlib encoding of n zero bytes without ever holding n
// bytes at once, so a fixture that inflates past the 50 MB extraction ceiling
// costs a few dozen KB to build and to store in the PDF.
func zlibZeros(t *testing.T, n int) []byte {
	t.Helper()
	const chunk = 64 * 1024
	block := make([]byte, chunk)
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	for written := 0; written < n; {
		size := chunk
		if remaining := n - written; remaining < size {
			size = remaining
		}
		if _, err := w.Write(block[:size]); err != nil {
			t.Fatalf("zlib write: %v", err)
		}
		written += size
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}
	return buf.Bytes()
}

// overCeilingSize is the decompressed size of the bomb fixtures: comfortably
// past the 50 MB production extraction ceiling.
const overCeilingSize = 60 * 1024 * 1024

// --- fixture builders -------------------------------------------------------

func pad10(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 10 {
		s = "0" + s
	}
	return s
}

// assemblePDF stitches a header, object bodies (object i+1), an xref table and
// a trailer with /Root. Objects are []byte so binary stream payloads survive.
func assemblePDF(objs [][]byte, rootNum int) []byte {
	body := []byte("%PDF-1.7\n")
	offsets := make([]int, len(objs))
	for i, o := range objs {
		offsets[i] = len(body)
		body = append(body, o...)
	}
	xrefOff := len(body)
	size := len(objs) + 1
	xref := "xref\n0 " + strconv.Itoa(size) + "\n0000000000 65535 f \n"
	for _, off := range offsets {
		xref += pad10(off) + " 00000 n \n"
	}
	trailer := "trailer\n<< /Size " + strconv.Itoa(size) + " /Root " + strconv.Itoa(rootNum) + " 0 R >>\n" +
		"startxref\n" + strconv.Itoa(xrefOff) + "\n%%EOF\n"
	return append(body, append([]byte(xref), []byte(trailer)...)...)
}

// streamObj returns object num with the given dict entries and a raw stream
// body. /Length is appended automatically.
func streamObj(num int, dictEntries string, raw []byte) []byte {
	head := strconv.Itoa(num) + " 0 obj\n<< " + dictEntries +
		" /Length " + strconv.Itoa(len(raw)) + " >>\nstream\n"
	out := append([]byte(head), raw...)
	return append(out, []byte("\nendstream\nendobj\n")...)
}

// flateEmbeddedPDF builds a single-attachment document whose /EmbeddedFile
// stream is FlateDecode-compressed. decodedSize is the /Params /Size entry, so
// the caller controls it independently of the raw payload.
func flateEmbeddedPDF(raw []byte, decodedSize int) []byte {
	dict := "/Type /EmbeddedFile /Subtype /application#2Foctet-stream /Filter /FlateDecode" +
		" /Params << /Size " + strconv.Itoa(decodedSize) + " /ModDate (D:20240101000000Z) >>"
	return assemblePDF([][]byte{
		[]byte("1 0 obj\n<< /Type /Catalog /Pages 2 0 R /AF [6 0 R] /Names << /EmbeddedFiles 7 0 R >> >>\nendobj\n"),
		[]byte("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"),
		[]byte("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n"),
		streamObj(4, dict, raw),
		[]byte("5 0 obj\nnull\nendobj\n"),
		[]byte("6 0 obj\n<< /Type /Filespec /F (payload.bin) /UF (payload.bin) /AFRelationship /Data" +
			" /EF << /F 4 0 R /UF 4 0 R >> >>\nendobj\n"),
		[]byte("7 0 obj\n<< /Names [(payload.bin) 6 0 R] >>\nendobj\n"),
	}, 1)
}

// flateImagePDF builds a single-page document with one FlateDecode image
// XObject (object 4) declared as an 8x8 DeviceGray bitmap. Only the stream
// payload varies between the in-bounds and over-ceiling fixtures, so the image
// dict is never the difference under test.
func flateImagePDF(raw []byte) []byte {
	return flateImagePDFSized(raw, 8, 8)
}

// imagePDFWithDict builds the same single-page document around an image XObject
// whose dictionary body is supplied verbatim, for fixtures that need a malformed
// entry the typed builders will not produce.
func imagePDFWithDict(dict string, raw []byte) []byte {
	return assemblePDF([][]byte{
		[]byte("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"),
		[]byte("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"),
		[]byte("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]" +
			" /Resources << /XObject << /Im0 4 0 R >> >> >>\nendobj\n"),
		streamObj(4, dict, raw),
	}, 1)
}

// flateImagePDFSized is flateImagePDF with the declared dimensions as inputs, so
// a fixture can either match its payload or understate it.
func flateImagePDFSized(raw []byte, width, height int) []byte {
	dict := "/Type /XObject /Subtype /Image" +
		" /Width " + strconv.Itoa(width) + " /Height " + strconv.Itoa(height) +
		" /ColorSpace /DeviceGray /BitsPerComponent 8 /Filter /FlateDecode"
	return assemblePDF([][]byte{
		[]byte("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"),
		[]byte("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"),
		[]byte("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]" +
			" /Resources << /XObject << /Im0 4 0 R >> >> >>\nendobj\n"),
		streamObj(4, dict, raw),
	}, 1)
}

// imagePixels returns the 64 bytes of an 8x8 DeviceGray bitmap.
func imagePixels() []byte {
	px := make([]byte, 64)
	for i := range px {
		px[i] = byte(i * 4)
	}
	return px
}
