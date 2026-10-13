package cli_output_format_normalization_test

import (
	"os"
	"testing"
)

// ---------------------------------------------------------------------------
// Default is RAW source bytes, UNCHANGED.
// `dump bytes` already conforms; its default stays raw document bytes. This
// is a regression lock -- it must keep passing after the normalization. The
// raw dump is byte-for-byte the source file.
// ---------------------------------------------------------------------------

func TestBytes_DefaultRawBytesUnchanged(t *testing.T) {
	bin := buildCLI(t)
	file := fixture(t, "minimal.pdf")

	want, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	stdout, stderr, ec := runCLI(t, bin, "dump", "bytes", file)
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	if stdout != string(want) {
		t.Errorf("the default `dump bytes` output must be the verbatim source bytes (got %d bytes, want %d)",
			len(stdout), len(want))
	}
}

// ---------------------------------------------------------------------------
// --json wraps the decoded text, UNCHANGED.
// --json wraps the decoded text in {"totalBytes","content"}.
// ---------------------------------------------------------------------------

func TestBytes_JSONWrapsDecodedText(t *testing.T) {
	bin := buildCLI(t)
	stdout, stderr, ec := runCLI(t, bin, "dump", "bytes", "--json", fixture(t, "minimal.pdf"))
	if ec != 0 {
		t.Fatalf("expected exit 0, got %d (stderr: %s)", ec, stderr)
	}
	var wrapper struct {
		TotalBytes int64  `json:"totalBytes"`
		Content    string `json:"content"`
	}
	mustParseJSON(t, stdout, &wrapper)
	if wrapper.TotalBytes <= 0 {
		t.Errorf("--json wrapper totalBytes = %d, want > 0", wrapper.TotalBytes)
	}
	if wrapper.Content == "" {
		t.Errorf("--json wrapper content is empty:\n%s", stdout)
	}
}
