// CLI ergonomics & discoverability -- acceptance tests.
//
// Black-box: build the CLI, run as a subprocess.
//
// Covers in this suite: (--pretty for dump stream; --raw --pretty is a no-op
// that still emits verbatim bytes, not JSON).
//
// Run: cd tests/cli-stream-retrieval && go test -v -count=1 ./...
package cli_stream_retrieval_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// `dump stream --pretty` emits indented multi-line JSON; default stays
// compact single-line. Both decode to the same payload.
// ---------------------------------------------------------------------------

func TestStreamDump_PrettyVsCompact(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "content-stream.pdf")

	compact, _, ec := runCLI(t, bin, "dump", "stream", "--json", "--page", "1", pdfPath)
	if ec != 0 {
		t.Fatalf("compact run exit %d", ec)
	}
	pretty, _, ep := runCLI(t, bin, "dump", "stream", "--json", "--pretty", "--page", "1", pdfPath)
	if ep != 0 {
		t.Fatalf("--pretty run exit %d", ep)
	}

	if strings.Count(strings.TrimRight(compact, "\n"), "\n") != 0 {
		t.Errorf("default stream output is not single-line compact:\n%.200s", compact)
	}
	if !strings.Contains(pretty, "\n  ") {
		t.Errorf("--pretty stream output is not indented multi-line:\n%.200s", pretty)
	}

	var a, b any
	mustParseJSON(t, compact, &a)
	mustParseJSON(t, pretty, &b)
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Error("--pretty and compact stream decode to different content")
	}
}

// ---------------------------------------------------------------------------
// `--pretty` is a no-op for `dump stream --raw`. --raw emits verbatim
// decoded bytes (not JSON), so --raw --pretty must produce byte-identical
// output to --raw alone.
// ---------------------------------------------------------------------------

func TestStreamDump_RawPretty_NoOp(t *testing.T) {
	bin := buildCLI(t)
	pdfPath := filepath.Join(testdataDir(t), "content-stream.pdf")

	raw, _, er := runCLI(t, bin, "dump", "stream", "--raw", "--page", "1", pdfPath)
	if er != 0 {
		t.Fatalf("--raw run exit %d", er)
	}
	rawPretty, _, erp := runCLI(t, bin, "dump", "stream", "--raw", "--pretty", "--page", "1", pdfPath)
	if erp != 0 {
		t.Fatalf("--raw --pretty run exit %d", erp)
	}

	if raw != rawPretty {
		t.Errorf("--pretty must be a no-op for --raw, but output differs\n--raw: %.120q\n--raw --pretty:%.120q", raw, rawPretty)
	}
}
