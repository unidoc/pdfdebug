package cert_extensions_and_cms_export_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDumpSignatures_FieldWithoutAnExportFlagIsAUsageError(t *testing.T) {
	stdout, stderr, code := runCLI(t, "dump", "signatures", "--field", "Sig1", fixture(t, "signed.pdf"))
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	assertJSONErrorObject(t, stderr)
}

func TestDumpSignatures_EmptyExportPathIsAUsageError(t *testing.T) {
	for _, args := range [][]string{
		{"--export-cms="},
		{"--export-certs", ""},
	} {
		full := append(append([]string{"dump", "signatures"}, args...), fixture(t, "signed.pdf"))
		stdout, stderr, code := runCLI(t, full...)
		if code != 1 {
			t.Errorf("%q exit = %d, want 1", args, code)
		}
		if stdout != "" {
			t.Errorf("%q stdout = %q, want empty", args, stdout)
		}
		assertJSONErrorObject(t, stderr)
	}
}

func TestExportCMS_NoCMSSignatureIsAnError(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sig.p7s")
	stdout, stderr, code := runCLI(t, "dump", "signatures", "--export-cms", out, fixture(t, "unsigned-sig-field.pdf"))
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "error: no CMS signature to export") {
		t.Errorf("stderr = %q, want error: no CMS signature to export", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	assertNoFile(t, out)
}

func TestExport_SelectionErrorOnOneFlagWritesNothing(t *testing.T) {
	tmp := t.TempDir()
	out := filepath.Join(tmp, "sig.p7s")
	dir := filepath.Join(tmp, "certs")
	stdout, stderr, code := runCLI(t, "dump", "signatures", "--export-certs", dir, "--export-cms", out, fixture(t, "unsigned-sig-field.pdf"))
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.HasPrefix(stderr, "error: ") {
		t.Errorf("stderr = %q, want an error: line", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	assertNoFile(t, out)
	assertNoFile(t, filepath.Join(dir, "sig1-cert1.der"))
}

func TestExport_UnknownFieldNameIsAnError(t *testing.T) {
	tmp := t.TempDir()
	for _, flag := range []string{"--export-cms", "--export-certs"} {
		target := filepath.Join(tmp, strings.TrimPrefix(flag, "--"))
		stdout, stderr, code := runCLI(t, "dump", "signatures", "--field", "Nope", flag, target, fixture(t, "signed.pdf"))
		if code != 2 {
			t.Errorf("%s exit = %d, want 2", flag, code)
		}
		if !strings.Contains(stderr, `error: no signature field named "Nope"`) {
			t.Errorf("%s stderr = %q, want the unknown-name error", flag, stderr)
		}
		if stdout != "" {
			t.Errorf("%s stdout = %q, want empty", flag, stdout)
		}
		assertNoFile(t, target)
	}
}

func TestExport_UnsignedFieldNameIsAnError(t *testing.T) {
	out := filepath.Join(t.TempDir(), "sig.p7s")
	_, stderr, code := runCLI(t, "dump", "signatures", "--field", "EmptySig", "--export-cms", out, fixture(t, "unsigned-sig-field.pdf"))
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, `error: signature field "EmptySig" is not signed`) {
		t.Errorf("stderr = %q, want the not-signed error", stderr)
	}
	assertNoFile(t, out)
}
