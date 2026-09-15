// Acceptance test harness for the image thumbnail preview path.
//
// Black-box: build the pdfdebug CLI binary and run it as a subprocess. The CLI
// `dump image --json` command calls Inspector.GetImageData -- the same method
// the GUI reaches over IPC -- so the thumbnail, the reduced-preview dimensions
// and the outcome discriminator are all observable in its JSON payload without
// standing up a Wails app.
//
// Fixtures are hand-rolled raw PDF bytes assembled in memory and written to a
// temp file, mirroring tests/bounded-decode. FlateDecode streams are built here
// rather than added to testdata/correctness/, which is uncompressed by invariant.
//
// Run: cd tests/image-thumbnail-preview && go test -v -count=1 ./...
package image_thumbnail_preview_test

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
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
// the exit code.
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
// asserts on. Kind is the outcome discriminator and ThumbWidth/ThumbHeight are
// the reduced-preview dimensions. A payload that omits a field decodes it to its
// zero value, which is what the assertions below key on.
type imageJSON struct {
	ObjectRef   string `json:"objectRef"`
	MimeType    string `json:"mimeType"`
	Base64      string `json:"base64"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Filter      string `json:"filter"`
	Kind        string `json:"kind"`
	ThumbWidth  int    `json:"thumbWidth"`
	ThumbHeight int    `json:"thumbHeight"`
	Error       string `json:"error"`
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

// decodedPreviewDims decodes the base64 payload and returns the pixel
// dimensions of the image actually shipped for display.
func decodedPreviewDims(t *testing.T, b64 string) (width, height int) {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("failed to base64-decode preview payload: %v", err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("failed to decode preview image: %v", err)
	}
	return cfg.Width, cfg.Height
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
// bytes at once, so a large-raster fixture costs a few dozen KB to build.
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

// lyingStreamPayloadSize is the decoded size of the lying-stream fixture:
// comfortably past the production 50 MB extraction ceiling for a tiny declared
// geometry, so only the decode ceiling can reject it.
const lyingStreamPayloadSize = 60 * 1024 * 1024

// --- fixture builders (mirrors tests/bounded-decode) ------------------------

func pad10(n int) string {
	s := strconv.Itoa(n)
	for len(s) < 10 {
		s = "0" + s
	}
	return s
}

// assemblePDF stitches a header, object bodies (object i+1), an xref table and
// a trailer with /Root.
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

// flateImagePDFSized builds a single-page document with one FlateDecode
// DeviceGray image XObject (object 4) at the declared dimensions.
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

// deviceNImagePDF builds a well-formed document whose image XObject carries a
// /DeviceN colour space with an INDIRECT colorant-name array. It opens cleanly
// and then faults pdfcpu's component lookup, yielding a per-image error that is
// neither a thumbnail success nor the ceiling refusal -- the third outcome the
// discriminator must separate.
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
