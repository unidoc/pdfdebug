package cert_extensions_and_cms_export_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// requireOpenSSL3 skips the test unless `openssl version` reports OpenSSL 3.
// On Linux CI (CI set) a missing or older openssl fails the test instead, so
// the round trip runs there.
func requireOpenSSL3(t *testing.T) {
	t.Helper()
	out, err := exec.Command("openssl", "version").Output()
	version := strings.TrimSpace(string(out))
	if err == nil && strings.HasPrefix(version, "OpenSSL 3") {
		return
	}
	reason := "openssl version: " + version
	if err != nil {
		reason = "openssl not runnable: " + err.Error()
	}
	if os.Getenv("CI") != "" && runtime.GOOS == "linux" {
		t.Fatalf("OpenSSL 3 is required on Linux CI (%s)", reason)
	}
	t.Skipf("OpenSSL 3 not available (%s)", reason)
}

// runOpenSSL runs openssl with args and returns combined output, failing the
// test on a non-zero exit.
func runOpenSSL(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("openssl", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("openssl %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// exportCMS runs --export-cms on a fixture and returns the export path and
// the fixture's --json entry.
func exportCMS(t *testing.T, name string) (string, sigEntry) {
	t.Helper()
	path := fixture(t, name)
	entries, _, _ := dumpJSON(t, path)
	e := oneEntry(t, entries)
	out := filepath.Join(t.TempDir(), "sig.p7s")
	if _, stderr, code := runCLI(t, "dump", "signatures", "--export-cms", out, path); code != 0 {
		t.Fatalf("--export-cms exit = %d, want 0\nstderr: %s", code, stderr)
	}
	return out, e
}

func TestOpenSSL_DetachedExportVerifiesAgainstTheSignedBytes(t *testing.T) {
	requireOpenSSL3(t)
	for _, name := range []string{"signed.pdf", "signed-ber-detached.pdf"} {
		t.Run(name, func(t *testing.T) {
			out, e := exportCMS(t, name)
			content := filepath.Join(t.TempDir(), "signed-bytes.bin")
			if err := os.WriteFile(content, signedBytes(t, fixture(t, name), e.ByteRange), 0o644); err != nil {
				t.Fatal(err)
			}
			runOpenSSL(t, "cms", "-verify", "-noverify", "-binary", "-inform", "DER",
				"-in", out, "-content", content, "-out", os.DevNull)
		})
	}
}

func TestOpenSSL_AttachedBERExportVerifies(t *testing.T) {
	requireOpenSSL3(t)
	out, _ := exportCMS(t, "signed-ber.pdf")
	runOpenSSL(t, "cms", "-verify", "-noverify", "-binary", "-inform", "DER", "-in", out, "-out", os.DevNull)
}

func TestOpenSSL_ExportListsTheFixtureCertificates(t *testing.T) {
	requireOpenSSL3(t)
	out, _ := exportCMS(t, "signed.pdf")
	certs := runOpenSSL(t, "pkcs7", "-inform", "DER", "-in", out, "-print_certs", "-noout")
	for _, cn := range []string{"CN=Fixture Signer", "CN=Fixture Root CA"} {
		if !strings.Contains(strings.ReplaceAll(certs, " = ", "="), cn) {
			t.Errorf("openssl pkcs7 -print_certs lacks %s:\n%s", cn, certs)
		}
	}
}

func TestOpenSSL_ExportedLeafCarriesTheFixtureURLs(t *testing.T) {
	requireOpenSSL3(t)
	path := fixture(t, "signed-cert-extensions.pdf")
	dir := t.TempDir()
	if _, stderr, code := runCLI(t, "dump", "signatures", "--export-certs", dir, path); code != 0 {
		t.Fatalf("--export-certs exit = %d, want 0\nstderr: %s", code, stderr)
	}
	text := runOpenSSL(t, "x509", "-inform", "DER", "-in", filepath.Join(dir, "sig1-cert2.der"),
		"-noout", "-ext", "crlDistributionPoints,authorityInfoAccess")
	for _, url := range []string{
		"http://crl.example.test/fixture-ca.crl",
		"http://ocsp.example.test",
		"http://ca.example.test/fixture-ca.cer",
	} {
		if !strings.Contains(text, url) {
			t.Errorf("openssl x509 -ext output lacks %s:\n%s", url, text)
		}
	}
}
