package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// exportPathsFixture returns one signed CMS field with one certificate, its
// export bytes, and an input PDF file holding known bytes.
func exportPathsFixture(t *testing.T) (dir, input string, sigs []pdfcore.SignatureField, sb []pdfcore.SignatureBytes) {
	t.Helper()
	dir = t.TempDir()
	input = filepath.Join(dir, "in.pdf")
	if err := os.WriteFile(input, []byte("%PDF-original"), 0o644); err != nil {
		t.Fatal(err)
	}
	sigs = []pdfcore.SignatureField{cmsField("Sig1")}
	sb = []pdfcore.SignatureBytes{{CMS: []byte{0x30, 0x00}, Certificates: [][]byte{{0x30, 0x01, 0x05}}}}
	return dir, input, sigs, sb
}

func assertInputIntact(t *testing.T, input string) {
	t.Helper()
	got, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte("%PDF-original")) {
		t.Fatalf("input PDF was overwritten: %q", got)
	}
}

func TestRunSignatureExports_RefusesCMSOverInputPDF(t *testing.T) {
	_, input, sigs, sb := exportPathsFixture(t)
	var code int
	stderr := captureStderr(t, func() {
		code = runSignatureExports(input, sigs, sb, signatureExportFlags{cmsPath: input})
	})
	if code != 2 {
		t.Fatalf("exit code %d, want 2", code)
	}
	if !strings.Contains(stderr, "is the input PDF") {
		t.Fatalf("stderr %q does not name the input PDF", stderr)
	}
	assertInputIntact(t, input)
}

func TestRunSignatureExports_RefusesCMSOverInputPDFThroughOtherSpelling(t *testing.T) {
	dir, input, sigs, sb := exportPathsFixture(t)
	other := filepath.Join(dir, "sub", "..", "in.pdf")
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	code := runSignatureExports(input, sigs, sb, signatureExportFlags{cmsPath: other})
	if code != 2 {
		t.Fatalf("exit code %d, want 2", code)
	}
	assertInputIntact(t, input)
}

func TestRunSignatureExports_RefusesCertFileOverInputPDF(t *testing.T) {
	dir, _, sigs, sb := exportPathsFixture(t)
	input := filepath.Join(dir, "sig1-cert1.der")
	if err := os.WriteFile(input, []byte("%PDF-original"), 0o644); err != nil {
		t.Fatal(err)
	}
	var code int
	stderr := captureStderr(t, func() {
		code = runSignatureExports(input, sigs, sb, signatureExportFlags{certsDir: dir})
	})
	if code != 2 {
		t.Fatalf("exit code %d, want 2", code)
	}
	if !strings.Contains(stderr, "overwrite the input PDF") {
		t.Fatalf("stderr %q does not name the input PDF", stderr)
	}
	assertInputIntact(t, input)
}

func TestRunSignatureExports_RefusesCMSFileThatIsACertFile(t *testing.T) {
	dir, input, sigs, sb := exportPathsFixture(t)
	cms := filepath.Join(dir, "sig1-cert1.der")
	var code int
	stderr := captureStderr(t, func() {
		code = runSignatureExports(input, sigs, sb, signatureExportFlags{cmsPath: cms, certsDir: dir})
	})
	if code != 2 {
		t.Fatalf("exit code %d, want 2", code)
	}
	if !strings.Contains(stderr, "also a certificate file") {
		t.Fatalf("stderr %q does not name the collision", stderr)
	}
	if _, err := os.Stat(cms); !os.IsNotExist(err) {
		t.Fatalf("%s exists (stat err %v), want nothing written", cms, err)
	}
}

func TestRunSignatureExports_CMSFileInMissingCertDirIsAWriteError(t *testing.T) {
	dir, input, sigs, sb := exportPathsFixture(t)
	out := filepath.Join(dir, "out")
	for _, name := range []string{"sig1-cert1.der", "SIG1-CERT1.DER"} {
		cms := filepath.Join(out, name)
		var code int
		stderr := captureStderr(t, func() {
			code = runSignatureExports(input, sigs, sb, signatureExportFlags{cmsPath: cms, certsDir: out})
		})
		if code != 2 {
			t.Fatalf("%s: exit code %d, want 2", name, code)
		}
		if !strings.Contains(stderr, "failed to write") {
			t.Fatalf("%s: stderr %q, want a write error", name, stderr)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatalf("%s: export directory was created (stat err %v), want nothing written", name, err)
		}
	}
}

func TestRunSignatureExports_WritesCMSFileNamedLikeInputInAnotherCase(t *testing.T) {
	dir, input, sigs, sb := exportPathsFixture(t)
	cms := filepath.Join(dir, "IN.PDF")
	if _, err := os.Stat(cms); err == nil {
		t.Skip("file system is case-insensitive: IN.PDF is in.pdf")
	}
	if code := runSignatureExports(input, sigs, sb, signatureExportFlags{cmsPath: cms}); code != 0 {
		t.Fatalf("exit code %d, want 0", code)
	}
	if got, _ := os.ReadFile(cms); !bytes.Equal(got, sb[0].CMS) {
		t.Fatalf("CMS file holds %x, want %x", got, sb[0].CMS)
	}
	assertInputIntact(t, input)
}

func TestRunSignatureExports_WritesDistinctPaths(t *testing.T) {
	dir, input, sigs, sb := exportPathsFixture(t)
	out := filepath.Join(dir, "out")
	cms := filepath.Join(dir, "sig.p7s")
	if code := runSignatureExports(input, sigs, sb, signatureExportFlags{cmsPath: cms, certsDir: out}); code != 0 {
		t.Fatalf("exit code %d, want 0", code)
	}
	if got, _ := os.ReadFile(cms); !bytes.Equal(got, sb[0].CMS) {
		t.Fatalf("CMS file holds %x, want %x", got, sb[0].CMS)
	}
	if got, _ := os.ReadFile(filepath.Join(out, "sig1-cert1.der")); !bytes.Equal(got, sb[0].Certificates[0]) {
		t.Fatalf("certificate file holds %x, want %x", got, sb[0].Certificates[0])
	}
	assertInputIntact(t, input)
}

func TestRunSignatureExports_RefusesCMSFileThatIsACertFileInAnotherCaseInAnExistingDir(t *testing.T) {
	dir, input, sigs, sb := exportPathsFixture(t)
	cms := filepath.Join(dir, "SIG1-CERT1.DER")
	var code int
	stderr := captureStderr(t, func() {
		code = runSignatureExports(input, sigs, sb, signatureExportFlags{cmsPath: cms, certsDir: dir})
	})
	if code != 2 {
		t.Fatalf("exit code %d, want 2", code)
	}
	if !strings.Contains(stderr, "also a certificate file") {
		t.Fatalf("stderr %q does not name the collision", stderr)
	}
	for _, name := range []string{"SIG1-CERT1.DER", "sig1-cert1.der"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s exists (stat err %v), want nothing written", name, err)
		}
	}
}

func TestRunSignatureExports_RefusesCMSFileThatIsACertFileThroughALinkedDir(t *testing.T) {
	dir, input, sigs, sb := exportPathsFixture(t)
	realDir := filepath.Join(dir, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	cms := filepath.Join(realDir, "sig1-cert1.der")
	var code int
	stderr := captureStderr(t, func() {
		code = runSignatureExports(input, sigs, sb, signatureExportFlags{cmsPath: cms, certsDir: link})
	})
	if code != 2 {
		t.Fatalf("exit code %d, want 2", code)
	}
	if !strings.Contains(stderr, "also a certificate file") {
		t.Fatalf("stderr %q does not name the collision", stderr)
	}
	if _, err := os.Stat(cms); !os.IsNotExist(err) {
		t.Fatalf("%s exists (stat err %v), want nothing written", cms, err)
	}
}

func TestRunSignatureExports_WritesSameNameInDifferentDirs(t *testing.T) {
	dir, input, sigs, sb := exportPathsFixture(t)
	other := filepath.Join(dir, "other")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	cms := filepath.Join(other, "sig1-cert1.der")
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if code := runSignatureExports(input, sigs, sb, signatureExportFlags{cmsPath: cms, certsDir: out}); code != 0 {
		t.Fatalf("exit code %d, want 0", code)
	}
	if got, _ := os.ReadFile(cms); !bytes.Equal(got, sb[0].CMS) {
		t.Fatalf("CMS file holds %x, want %x", got, sb[0].CMS)
	}
	if got, _ := os.ReadFile(filepath.Join(out, "sig1-cert1.der")); !bytes.Equal(got, sb[0].Certificates[0]) {
		t.Fatalf("certificate file holds %x, want %x", got, sb[0].Certificates[0])
	}
}

func TestRunSignatureExports_UnreadableContentsWritesNothing(t *testing.T) {
	dir, input, sigs, _ := exportPathsFixture(t)
	sigs[0].DecomposeError = "/Contents unreadable: entry missing"
	sb := []pdfcore.SignatureBytes{{}}
	cms := filepath.Join(dir, "sig.p7s")
	var code int
	stderr := captureStderr(t, func() {
		code = runSignatureExports(input, sigs, sb, signatureExportFlags{cmsPath: cms})
	})
	if code != 2 {
		t.Fatalf("exit code %d, want 2", code)
	}
	want := `error: /Contents of signature field "Sig1" could not be read (/Contents unreadable: entry missing)`
	if !strings.Contains(stderr, want) {
		t.Fatalf("stderr %q, want %q", stderr, want)
	}
	if _, err := os.Stat(cms); !os.IsNotExist(err) {
		t.Fatalf("%s exists (stat err %v), want nothing written", cms, err)
	}
}

func TestRunSignatureExports_CertDirThatIsAFileKeepsTheWrittenCMS(t *testing.T) {
	dir, input, sigs, sb := exportPathsFixture(t)
	cms := filepath.Join(dir, "sig.p7s")
	certs := filepath.Join(dir, "certs")
	if err := os.WriteFile(certs, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	var code int
	stderr := captureStderr(t, func() {
		code = runSignatureExports(input, sigs, sb, signatureExportFlags{cmsPath: cms, certsDir: certs})
	})
	if code != 2 {
		t.Fatalf("exit code %d, want 2", code)
	}
	if !strings.Contains(stderr, "failed to write "+certs) {
		t.Fatalf("stderr %q, want a write error naming %s", stderr, certs)
	}
	got, err := os.ReadFile(cms)
	if err != nil {
		t.Fatalf("CMS file written before the failure is gone: %v", err)
	}
	if !bytes.Equal(got, sb[0].CMS) {
		t.Errorf("CMS file = % X, want % X", got, sb[0].CMS)
	}
	assertInputIntact(t, input)
}

func TestRunSignatureExports_CertFileWriteFailureKeepsEarlierCertFiles(t *testing.T) {
	dir, input, sigs, sb := exportPathsFixture(t)
	sb[0].Certificates = append(sb[0].Certificates, []byte{0x30, 0x01, 0x06})
	certs := filepath.Join(dir, "certs")
	blocked := filepath.Join(certs, "sig1-cert2.der")
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	var code int
	stderr := captureStderr(t, func() {
		code = runSignatureExports(input, sigs, sb, signatureExportFlags{certsDir: certs})
	})
	if code != 2 {
		t.Fatalf("exit code %d, want 2", code)
	}
	if !strings.Contains(stderr, "failed to write "+blocked) {
		t.Fatalf("stderr %q, want a write error naming %s", stderr, blocked)
	}
	got, err := os.ReadFile(filepath.Join(certs, "sig1-cert1.der"))
	if err != nil {
		t.Fatalf("first certificate file is missing: %v", err)
	}
	if !bytes.Equal(got, sb[0].Certificates[0]) {
		t.Errorf("sig1-cert1.der = % X, want % X", got, sb[0].Certificates[0])
	}
}

func TestRunSignatureExports_UndelimitedWarningNamesAnUnnamedFieldAsADash(t *testing.T) {
	dir, input, _, _ := exportPathsFixture(t)
	sigs := []pdfcore.SignatureField{cmsField("")}
	payload := []byte{0x30, 0x80, 0x00, 0x00, 0x00}
	sb := []pdfcore.SignatureBytes{{CMS: payload, CMSUndelimited: "truncated"}}
	cms := filepath.Join(dir, "sig.p7s")
	var code int
	stderr := captureStderr(t, func() {
		code = runSignatureExports(input, sigs, sb, signatureExportFlags{cmsPath: cms})
	})
	if code != 0 {
		t.Fatalf("exit code %d, want 0; stderr %q", code, stderr)
	}
	want := `{"warning":"-: CMS element could not be delimited (truncated); wrote all 5 /Contents bytes including padding"}`
	if strings.TrimSpace(stderr) != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	got, err := os.ReadFile(cms)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("CMS file = % X, want % X", got, payload)
	}
}
