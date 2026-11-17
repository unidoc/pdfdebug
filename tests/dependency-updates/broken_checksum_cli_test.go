package dependency_updates_test

import (
	"bytes"
	"compress/zlib"
	"strconv"
	"strings"
	"testing"
)

// A content stream whose Flate data is intact but whose Adler-32 checksum is
// wrong still decodes, and `dump stream` prints its operators.
func TestDumpStreamPrintsBrokenChecksumContent(t *testing.T) {
	content := "BT /F1 12 Tf 100 700 Td (Broken Checksum) Tj ET\n"
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, _ = w.Write([]byte(content))
	_ = w.Close()
	data := buf.Bytes()
	data[len(data)-1] ^= 0xff

	pdf := classicPDF(
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R >>",
		"<< /Filter /FlateDecode /Length "+strconv.Itoa(len(data))+" >>\nstream\n"+string(data)+"\nendstream",
	)
	bin := buildCLI(t)
	stdout, stderr, code := runCLI(t, bin, "dump", "stream", "--page", "1", writeTemp(t, "broken-checksum.pdf", pdf))
	if code != 0 {
		t.Fatalf("dump stream exit %d, want 0\nstderr: %s", code, stderr)
	}
	if !strings.Contains(stdout, "(Broken Checksum) Tj") {
		t.Errorf("dump stream output lacks the decoded operators:\n%s", stdout)
	}
}
