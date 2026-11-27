package cert_extensions_and_cms_export_test

import (
	"crypto/x509"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestExportCerts_WritesOneDERFilePerChainCertificate(t *testing.T) {
	path := fixture(t, "signed-cert-extensions.pdf")
	e := oneEntry(t, func() []sigEntry { es, _, _ := dumpJSON(t, path); return es }())

	dir := t.TempDir()
	keep := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(keep, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runCLI(t, "dump", "signatures", "--export-certs", dir, path)
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstderr: %s", code, stderr)
	}
	if got, want := dirNames(t, dir), []string{"notes.txt", "sig1-cert1.der", "sig1-cert2.der"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("files in DIR = %v, want %v", got, want)
	}
	if b, err := os.ReadFile(keep); err != nil || string(b) != "keep me" {
		t.Errorf("notes.txt changed: %q, %v", b, err)
	}
	if len(e.Certificates) != 2 {
		t.Fatalf("--json lists %d certificates, want 2", len(e.Certificates))
	}
	for i, name := range []string{"sig1-cert1.der", "sig1-cert2.der"} {
		der, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		c, err := x509.ParseCertificate(der)
		if err != nil {
			t.Errorf("%s does not parse as a DER certificate: %v", name, err)
			continue
		}
		if c.Subject.String() != e.Certificates[i].Subject || c.SerialNumber.String() != e.Certificates[i].Serial {
			t.Errorf("%s is %s serial %s, want %s serial %s (certificate %d of --json)",
				name, c.Subject, c.SerialNumber, e.Certificates[i].Subject, e.Certificates[i].Serial, i+1)
		}
	}
}

func TestExportCerts_CreatesAMissingDirectoryAndOverwrites(t *testing.T) {
	path := fixture(t, "signed.pdf")
	dir := filepath.Join(t.TempDir(), "a", "b")
	_, stderr, code := runCLI(t, "dump", "signatures", "--export-certs", dir, path)
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstderr: %s", code, stderr)
	}
	if got, want := dirNames(t, dir), []string{"sig1-cert1.der", "sig1-cert2.der"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("files in DIR = %v, want %v", got, want)
	}

	stale := filepath.Join(dir, "sig1-cert1.der")
	if err := os.WriteFile(stale, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, stderr, code := runCLI(t, "dump", "signatures", "--export-certs", dir, path); code != 0 {
		t.Fatalf("second run exit = %d, want 0\nstderr: %s", code, stderr)
	}
	der, err := os.ReadFile(stale)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := x509.ParseCertificate(der); err != nil {
		t.Errorf("sig1-cert1.der was not overwritten with a certificate: %v", err)
	}
}

func TestExportCerts_StdoutIsUnchanged(t *testing.T) {
	path := fixture(t, "signed.pdf")
	for _, mode := range [][]string{nil, {"--json"}} {
		base := append(append([]string{"dump", "signatures"}, mode...), path)
		want, _, code := runCLI(t, base...)
		if code != 0 {
			t.Fatalf("%v exit = %d, want 0", base, code)
		}
		args := append(append([]string{"dump", "signatures"}, mode...), "--export-certs", t.TempDir(), path)
		got, stderr, code := runCLI(t, args...)
		if code != 0 {
			t.Fatalf("%v exit = %d, want 0\nstderr: %s", args, code, stderr)
		}
		if got != want {
			t.Errorf("%v stdout differs from the same command without --export-certs\n got %s\nwant %s", args, got, want)
		}
		if stderr != "" {
			t.Errorf("%v stderr = %q, want empty", args, stderr)
		}
	}
}

func TestExportCerts_NothingToWriteIsAnError(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := runCLI(t, "dump", "signatures", "--export-certs", dir, fixture(t, "unsigned-sig-field.pdf"))
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if !strings.Contains(stderr, "error: no certificates to export") {
		t.Errorf("stderr = %q, want error: no certificates to export", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if names := dirNames(t, dir); len(names) != 0 {
		t.Errorf("files in DIR = %v, want none", names)
	}
}

func TestExportBoth_WritesTheCMSAndTheCertificates(t *testing.T) {
	path := fixture(t, "signed-cert-extensions.pdf")
	tmp := t.TempDir()
	out := filepath.Join(tmp, "sig.p7s")
	dir := filepath.Join(tmp, "certs")
	want, _, code := runCLI(t, "dump", "signatures", "--json", path)
	if code != 0 {
		t.Fatalf("dump signatures --json exit = %d, want 0", code)
	}
	got, stderr, code := runCLI(t, "dump", "signatures", "--json", "--export-cms", out, "--export-certs", dir, path)
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstderr: %s", code, stderr)
	}
	if got != want {
		t.Errorf("stdout differs from the same command without the export flags")
	}
	if b, err := os.ReadFile(out); err != nil || len(b) == 0 || b[0] != 0x30 {
		t.Errorf("CMS export = %d bytes, %v; want a SEQUENCE", len(b), err)
	}
	if got, want := dirNames(t, dir), []string{"sig1-cert1.der", "sig1-cert2.der"}; !reflect.DeepEqual(got, want) {
		t.Errorf("files in DIR = %v, want %v", got, want)
	}
}
