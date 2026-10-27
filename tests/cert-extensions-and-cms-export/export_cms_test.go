package cert_extensions_and_cms_export_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cmsExportCases are the fixtures whose /Contents holds a complete CMS
// element, with the element end and whether it is BER.
var cmsExportCases = []struct {
	name    string
	end     int
	wantBER bool
}{
	{"signed.pdf", 2026, false},
	{"signed-ber-detached.pdf", 2028, true},
	{"signed-ber.pdf", 2265, true},
	{"signed-ber-trailing.pdf", 2028, true},
}

func TestExportCMS_WritesTheCMSElementAsFound(t *testing.T) {
	for _, tc := range cmsExportCases {
		t.Run(tc.name, func(t *testing.T) {
			path := fixture(t, tc.name)
			want := contentsPayload(t, path)[:tc.end]

			for _, mode := range [][]string{nil, {"--json"}} {
				base := append(append([]string{"dump", "signatures"}, mode...), path)
				plainOut, _, code := runCLI(t, base...)
				if code != 0 {
					t.Fatalf("%v exit = %d, want 0", base, code)
				}

				out := filepath.Join(t.TempDir(), "sig.p7s")
				args := append(append([]string{"dump", "signatures"}, mode...), "--export-cms", out, path)
				stdout, stderr, code := runCLI(t, args...)
				if code != 0 {
					t.Fatalf("%v exit = %d, want 0\nstderr: %s", args, code, stderr)
				}
				if stderr != "" {
					t.Errorf("%v stderr = %q, want empty", args, stderr)
				}
				if stdout != plainOut {
					t.Errorf("%v stdout differs from the same command without --export-cms\n got %s\nwant %s", args, stdout, plainOut)
				}
				got, err := os.ReadFile(out)
				if err != nil {
					t.Fatalf("read export: %v", err)
				}
				if !bytes.Equal(got, want) {
					t.Errorf("export is %d bytes, want the first %d decoded /Contents bytes", len(got), tc.end)
				}
				if tc.wantBER && !bytes.HasPrefix(got, []byte{0x30, 0x80}) {
					t.Errorf("export starts % X, want 30 80 (BER kept)", got[:min(2, len(got))])
				}
			}
		})
	}
}

func TestExportCMS_UndelimitedPayloadWritesEveryByteWithAWarning(t *testing.T) {
	path := fixture(t, "signed-ber-badeoc.pdf")
	want := contentsPayload(t, path)
	out := filepath.Join(t.TempDir(), "sig.p7s")
	_, stderr, code := runCLI(t, "dump", "signatures", "--export-cms", out, path)
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstderr: %s", code, stderr)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if len(want) != 3072 || !bytes.Equal(got, want) {
		t.Errorf("export is %d bytes, want all %d decoded /Contents bytes", len(got), len(want))
	}
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	if len(lines) != 1 {
		t.Fatalf("stderr has %d lines, want one JSON warning:\n%s", len(lines), stderr)
	}
	var w struct {
		Warning string `json:"warning"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &w); err != nil {
		t.Fatalf("stderr is not a JSON warning (%v): %s", err, lines[0])
	}
	if !strings.Contains(w.Warning, "Sig1") || !strings.Contains(w.Warning, "3072") {
		t.Errorf("warning = %q, want it to name Sig1 and the 3072-byte count", w.Warning)
	}
}

func TestExportCMS_OverwritesAnExistingFile(t *testing.T) {
	path := fixture(t, "signed.pdf")
	out := filepath.Join(t.TempDir(), "sig.p7s")
	if err := os.WriteFile(out, bytes.Repeat([]byte("old"), 2000), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runCLI(t, "dump", "signatures", "--export-cms", out, path)
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstderr: %s", code, stderr)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if !bytes.Equal(got, contentsPayload(t, path)[:2026]) {
		t.Errorf("export is %d bytes, want the 2026-byte CMS replacing the old file", len(got))
	}
}

func TestExportCMS_FileInAMissingDirectoryIsAWriteError(t *testing.T) {
	out := filepath.Join(t.TempDir(), "no-such-dir", "sig.p7s")
	stdout, stderr, code := runCLI(t, "dump", "signatures", "--export-cms", out, fixture(t, "signed.pdf"))
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "failed to write") {
		t.Errorf("stderr = %q, want a failed to write error", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	assertNoFile(t, out)
}

func TestExportCMS_FieldSelectsTheSignature(t *testing.T) {
	path := fixture(t, "signed.pdf")
	out := filepath.Join(t.TempDir(), "sig.p7s")
	_, stderr, code := runCLI(t, "dump", "signatures", "--field", "Sig1", "--export-cms", out, path)
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstderr: %s", code, stderr)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if !bytes.Equal(got, contentsPayload(t, path)[:2026]) {
		t.Errorf("export is %d bytes, want the 2026-byte CMS", len(got))
	}
}
