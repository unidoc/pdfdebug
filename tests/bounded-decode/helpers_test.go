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
	// Resolve the root OUTSIDE the Once: projectRoot fails via t.Fatalf, which is
	// runtime.Goexit, and sync.Once marks itself done in a defer - so failing in
	// there would consume the Once and leave every later test to fail at
	// exec.Command("") with an error that names nothing useful.
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

// overCeilingSize is the decompressed size of the bomb fixtures: comfortably past
// the production extraction ceiling. This module cannot import that constant, so
// the value SHADOWS pdfcore's maxImageBytes (50 MB) with only ~1.2x clearance -
// raising maxImageBytes past 60 MB turns these fixtures into in-bounds streams
// and flips the two over-ceiling tests. Loud rather than silent, but keep them in
// step.
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

// deviceNImagePDF builds a single-page document whose image XObject carries a
// /DeviceN colour space whose colorant-name array is an INDIRECT reference. Every
// referenced object exists, so the document is well-formed and opens; pdfcpu's
// component lookup then asserts that entry is an Array without checking, and
// faults. This is the shape that reaches the lookup - dangling references do not,
// because the document fails to open first.
func deviceNImagePDF() []byte {
	dict := "/Type /XObject /Subtype /Image /Width 8 /Height 8 /BitsPerComponent 8" +
		" /ColorSpace [/DeviceN 5 0 R /DeviceRGB 6 0 R] /Filter /DCTDecode"
	return assemblePDF([][]byte{
		[]byte("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"),
		[]byte("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"),
		[]byte("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]" +
			" /Resources << /XObject << /Im0 4 0 R >> >> >>\nendobj\n"),
		streamObj(4, dict, []byte("not a jpeg")),
		[]byte("5 0 obj\n[/Ink1 /Ink2]\nendobj\n"),
		[]byte("6 0 obj\n<< /FunctionType 2 /Domain [0 1] /C0 [0 0 0] /C1 [1 1 1] /N 1 >>\nendobj\n"),
	}, 1)
}

// stencilMaskPDF builds a single-page document whose image XObject is a
// FlateDecode stencil mask: /ImageMask true, no /ColorSpace, one 1-bit sample
// per pixel. The payload inflates well past what a 1-bit mask of these
// dimensions declares, so a ceiling sized from the mask's real geometry refuses
// it while one sized from the unresolved-colour-space fallback does not.
func stencilMaskPDF(raw []byte, width, height int) []byte {
	// /BitsPerComponent is deliberately omitted, which is legal for a mask and is
	// what makes the fallback sizing visible: absent, it defaults to 8, so a mask
	// measured by the unresolved-colour-space fallback is read as 32 components
	// of 8 bits rather than the 1 bit it actually carries.
	dict := "/Type /XObject /Subtype /Image" +
		" /Width " + strconv.Itoa(width) + " /Height " + strconv.Itoa(height) +
		" /ImageMask true /Filter /FlateDecode"
	return assemblePDF([][]byte{
		[]byte("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n"),
		[]byte("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n"),
		[]byte("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]" +
			" /Resources << /XObject << /Im0 4 0 R >> >> >>\nendobj\n"),
		streamObj(4, dict, raw),
	}, 1)
}
