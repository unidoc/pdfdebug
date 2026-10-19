package ber_cms_signatures_test

import "testing"

const berPlainText = "BER (normalized to DER for parsing)"

func TestDumpSignaturesJSON_DetachedBERSignatureDecomposes(t *testing.T) {
	e := dumpSignaturesJSON(t, "signed-ber-detached.pdf")
	if got := cmsEncoding(t, e); got != "BER" {
		t.Errorf("cmsEncoding = %q, want BER", got)
	}
	if e.DecomposeError != "" {
		t.Errorf("decomposeError = %q, want empty", e.DecomposeError)
	}
	if len(e.Certificates) != 2 {
		t.Errorf("certificates = %d entries, want 2", len(e.Certificates))
	}
	if !e.SignerIdentified {
		t.Errorf("signerIdentified = false, want true")
	}
}

func TestDumpSignaturesJSON_DERSignatureReportsDER(t *testing.T) {
	e := dumpSignaturesJSON(t, "signed.pdf")
	if got := cmsEncoding(t, e); got != "DER" {
		t.Errorf("cmsEncoding = %q, want DER", got)
	}
	if e.DecomposeError != "" {
		t.Errorf("decomposeError = %q, want empty", e.DecomposeError)
	}
}

func TestDumpSignaturesJSON_MalformedBERReportsErrorAndNoChain(t *testing.T) {
	e := dumpSignaturesJSON(t, "signed-ber-badeoc.pdf")
	if e.DecomposeError == "" {
		t.Errorf("decomposeError empty, want the CMS parse failure")
	}
	if e.Certificates == nil || len(e.Certificates) != 0 {
		t.Errorf("certificates = %v, want an empty array", e.Certificates)
	}
	if got := cmsEncoding(t, e); got != "" {
		t.Errorf("cmsEncoding = %q, want empty", got)
	}
	if !e.Signed {
		t.Errorf("signed = false, want true (the field keeps its structural facts)")
	}
}

func TestDumpSignaturesPlain_BERSignatureShowsEncodingLine(t *testing.T) {
	stdout, stderr, code := runCLI(t, "dump", "signatures", fixture(t, "signed-ber-detached.pdf"))
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstderr: %s", code, stderr)
	}
	got, ok := plainLine(stdout, "CMS encoding")
	if !ok {
		t.Fatalf("no \"CMS encoding:\" line in plain output:\n%s", stdout)
	}
	if got != berPlainText {
		t.Errorf("CMS encoding = %q, want %q", got, berPlainText)
	}
}

func TestDumpSignaturesPlain_DERSignatureShowsEncodingLine(t *testing.T) {
	stdout, stderr, code := runCLI(t, "dump", "signatures", fixture(t, "signed.pdf"))
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstderr: %s", code, stderr)
	}
	got, ok := plainLine(stdout, "CMS encoding")
	if !ok {
		t.Fatalf("no \"CMS encoding:\" line in plain output:\n%s", stdout)
	}
	if got != "DER" {
		t.Errorf("CMS encoding = %q, want DER", got)
	}
}

func TestDumpSignaturesPlain_MalformedBERHasNoEncodingLine(t *testing.T) {
	stdout, stderr, code := runCLI(t, "dump", "signatures", fixture(t, "signed-ber-badeoc.pdf"))
	if code != 0 {
		t.Fatalf("exit = %d, want 0\nstderr: %s", code, stderr)
	}
	if got, ok := plainLine(stdout, "CMS encoding"); ok {
		t.Errorf("plain output has \"CMS encoding: %s\" for a payload that did not decode", got)
	}
	if _, ok := plainLine(stdout, "Decompose error"); !ok {
		t.Errorf("no \"Decompose error:\" line in plain output:\n%s", stdout)
	}
}
