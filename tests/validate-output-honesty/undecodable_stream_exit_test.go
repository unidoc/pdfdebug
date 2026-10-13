package validate_output_honesty_test

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exitExplanation is the plain-text line that names why a run with no errors
// exits 1.
const exitExplanation = "Exit status 1: a rule that gates the exit code could not be evaluated"

// writePDF writes objs (numbered 1..N) with an xref table, a trailer /Root 1 0 R
// and a trailer /ID to a temp file and returns its path.
func writePDF(t *testing.T, objs ...string) string {
	t.Helper()
	body := "%PDF-1.4\n"
	xref := fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for i, o := range objs {
		xref += fmt.Sprintf("%010d 00000 n \n", len(body))
		body += fmt.Sprintf("%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	data := fmt.Sprintf("%s%strailer\n<< /Size %d /Root 1 0 R /ID [<01> <01>] >>\nstartxref\n%d\n%%%%EOF\n",
		body, xref, len(objs)+1, len(body))
	path := filepath.Join(t.TempDir(), "doc.pdf")
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// streamObj returns a stream object with dict entries extra and data as its body.
func streamObj(extra string, data []byte) string {
	return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", extra, len(data), data)
}

// A pdfa-1b file with no /OutputIntents and no device colour in what could be
// read exits 0 when every stream decodes, and exits 1 with output-intent not
// evaluated when a page content stream or a form XObject in the page's
// /Resources does not decode. The plain-text run names that cause after the
// summary line only when the exit is 1 with no errors.
func TestValidate_UndecodableStreamExitsOneUnderPDFA(t *testing.T) {
	var zb bytes.Buffer
	zw := zlib.NewWriter(&zb)
	_, _ = zw.Write([]byte("0 0 m 10 10 l S"))
	_ = zw.Close()
	good := zb.Bytes()
	corrupt := append([]byte{0x78, 0x9c}, bytes.Repeat([]byte{0xff}, 20)...)

	xmp := []byte(`<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?><x:xmpmeta xmlns:x="adobe:ns:meta/"></x:xmpmeta><?xpacket end="w"?>`)
	catalog := "<< /Type /Catalog /Pages 2 0 R /Metadata 5 0 R >>"
	pages := "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"
	page := "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>"
	pageWithForm := "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /XObject << /Fx 6 0 R >> >> >>"
	meta := streamObj("/Type /Metadata /Subtype /XML", xmp)
	form := streamObj("/Type /XObject /Subtype /Form /BBox [0 0 10 10] /Filter /FlateDecode", corrupt)

	cases := []struct {
		name    string
		objs    []string
		want    string
		explain bool
	}{
		{"content stream decodes", []string{catalog, pages, page, streamObj("/Filter /FlateDecode", good), meta},
			"exit=0 errors=0 info=0 output-intent=0 found", false},
		{"content stream is not zlib data", []string{catalog, pages, page, streamObj("/Filter /FlateDecode", []byte("1 0 0 rg")), meta},
			"exit=1 errors=0 info=1 output-intent=not evaluated", true},
		{"content stream has a corrupt deflate block", []string{catalog, pages, page, streamObj("/Filter /FlateDecode", corrupt), meta},
			"exit=1 errors=0 info=1 output-intent=not evaluated", true},
		{"unused form XObject does not decode", []string{catalog, pages, pageWithForm, streamObj("/Filter /FlateDecode", good), meta, form},
			"exit=1 errors=0 info=1 output-intent=not evaluated", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			file := writePDF(t, c.objs...)
			stdout, stderr, ec := runCLI(t, "validate", "--profile", profilePDFA, "--json", file)
			res := parseResult(t, stdout)
			oi := "missing"
			for _, r := range res.Rules {
				if r.RuleID == "output-intent" {
					oi = outcome(r)
				}
			}
			got := fmt.Sprintf("exit=%d errors=%d info=%d output-intent=%s", ec, res.Summary.Errors, res.Summary.Info, oi)
			if got != c.want {
				t.Errorf("got  %s\nwant %s\nstderr: %s", got, c.want, stderr)
			}

			plain, _, pec := runCLI(t, "validate", "--profile", profilePDFA, file)
			if pec != ec {
				t.Errorf("plain exit %d, --json exit %d", pec, ec)
			}
			if has := strings.Contains(plain, exitExplanation); has != c.explain {
				t.Errorf("plain output has exit explanation = %v, want %v:\n%s", has, c.explain, plain)
			}
			if c.explain {
				lines := strings.Split(strings.TrimRight(plain, "\n"), "\n")
				if n := len(lines); n < 2 || !strings.HasPrefix(lines[n-2], "Summary:") || lines[n-1] != exitExplanation {
					t.Errorf("exit explanation is not the line after the summary:\n%s", plain)
				}
			}
			assertNoVerdict(t, c.name, plain)
			assertASCII(t, c.name, plain)
		})
	}
}
