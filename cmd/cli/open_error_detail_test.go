package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// openErrorObject is the JSON object the CLI writes to stderr when a file
// cannot be opened.
type openErrorObject struct {
	Error  string  `json:"error"`
	Detail *string `json:"detail"`
	File   *string `json:"file"`
}

const malformedOpenError = "malformed PDF: file is corrupt or invalid"

func testdataPath(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// parseOpenError unmarshals stderr as exactly one JSON object and also returns
// its keys.
func parseOpenError(t *testing.T, stderr string) (openErrorObject, map[string]json.RawMessage) {
	t.Helper()
	line := strings.TrimSpace(stderr)
	if strings.Count(line, "\n") != 0 {
		t.Fatalf("stderr holds more than one line:\n%s", stderr)
	}
	var obj openErrorObject
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Fatalf("stderr is not a JSON object: %v\n%s", err, stderr)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &keys); err != nil {
		t.Fatal(err)
	}
	return obj, keys
}

// runOpenFailure runs the CLI and returns its stderr, failing unless it exits
// 2 with nothing on stdout.
func runOpenFailure(t *testing.T, bin string, args ...string) string {
	t.Helper()
	stdout, stderr, code := runBin(t, bin, args...)
	if code != 2 {
		t.Fatalf("%v: exit %d, want 2\nstdout: %s\nstderr: %s", args, code, stdout, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("%v: stdout = %q, want empty", args, stdout)
	}
	return stderr
}

func runBin(t *testing.T, bin string, args ...string) (string, string, int) {
	t.Helper()
	var stdout, stderr strings.Builder
	cmd := exec.Command(bin, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return stdout.String(), stderr.String(), 0
	case errors.As(err, &ee):
		return stdout.String(), stderr.String(), ee.ExitCode()
	}
	t.Fatalf("run %v: %v", args, err)
	return "", "", 0
}

// checkMalformedDetail checks the fixed error text and a detail that carries
// the parser's reason without repeating the fixed prefix.
func checkMalformedDetail(t *testing.T, what string, obj openErrorObject) {
	t.Helper()
	if obj.Error != malformedOpenError {
		t.Errorf("%s: error = %q, want exactly %q", what, obj.Error, malformedOpenError)
	}
	if obj.Detail == nil || strings.TrimSpace(*obj.Detail) == "" {
		t.Errorf("%s: detail missing or empty, want the reason the file could not be parsed", what)
		return
	}
	if strings.HasPrefix(*obj.Detail, "malformed PDF") {
		t.Errorf("%s: detail = %q, want the cause without the malformed PDF prefix", what, *obj.Detail)
	}
}

// A file that cannot be parsed keeps the fixed error text and adds the cause
// in detail, on dump and validate alike.
func TestOpenError_MalformedCarriesDetail(t *testing.T) {
	bin := buildCLIBinary(t)
	malformed := testdataPath(t, "malformed.pdf")
	for _, args := range [][]string{
		{"dump", "xref", "--json", malformed},
		{"validate", "--json", malformed},
	} {
		obj, keys := parseOpenError(t, runOpenFailure(t, bin, args...))
		checkMalformedDetail(t, strings.Join(args[:len(args)-1], " "), obj)
		if _, ok := keys["file"]; ok {
			t.Errorf("%v: stderr has a file field; only diff names the file", args)
		}
	}
}

// diff names the side that could not be parsed, as the path was given.
func TestOpenError_DiffNamesTheFileThatFailed(t *testing.T) {
	bin := buildCLIBinary(t)
	minimal := testdataPath(t, "minimal.pdf")
	malformed := testdataPath(t, "malformed.pdf")
	for _, tc := range []struct {
		name        string
		left, right string
	}{
		{"right side", minimal, malformed},
		{"left side", malformed, minimal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj, _ := parseOpenError(t, runOpenFailure(t, bin, "diff", "--json", tc.left, tc.right))
			checkMalformedDetail(t, "diff", obj)
			if obj.File == nil || *obj.File != malformed {
				t.Errorf("file = %v, want %q", obj.File, malformed)
			}
		})
	}
}

// Encrypted and missing files print an object with only the error field, on
// dump and on diff.
func TestOpenError_EncryptedAndMissingCarryOnlyError(t *testing.T) {
	bin := buildCLIBinary(t)
	minimal := testdataPath(t, "minimal.pdf")
	encrypted := testdataPath(t, "encrypted.pdf")
	missing := filepath.Join(t.TempDir(), "missing.pdf")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"dump", "xref", "--json", encrypted}, "encrypted PDF: password required"},
		{[]string{"dump", "xref", "--json", missing}, "file not found"},
		{[]string{"diff", "--json", minimal, encrypted}, "encrypted PDF: password required"},
		{[]string{"diff", "--json", minimal, missing}, "file not found"},
	} {
		obj, keys := parseOpenError(t, runOpenFailure(t, bin, tc.args...))
		if obj.Error != tc.want {
			t.Errorf("%v: error = %q, want %q", tc.args, obj.Error, tc.want)
		}
		if len(keys) != 1 {
			t.Errorf("%v: stderr object has keys %v, want only error", tc.args, keys)
		}
	}
}

// A file without read permission is not reported as a malformed PDF, on dump
// and on diff; the error names the file.
func TestOpenError_UnreadableIsNotMalformed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file read permission is a Unix mode bit")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	bin := buildCLIBinary(t)
	minimal := testdataPath(t, "minimal.pdf")
	data, err := os.ReadFile(minimal)
	if err != nil {
		t.Fatal(err)
	}
	unreadable := filepath.Join(t.TempDir(), "unreadable.pdf")
	if err := os.WriteFile(unreadable, data, 0o000); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"dump", "xref", "--json", unreadable},
		{"diff", "--json", minimal, unreadable},
	} {
		obj, keys := parseOpenError(t, runOpenFailure(t, bin, args...))
		if strings.HasPrefix(obj.Error, "malformed PDF") {
			t.Errorf("%v: error = %q, want a permission error", args, obj.Error)
		}
		if !strings.Contains(obj.Error, "permission denied") || !strings.Contains(obj.Error, unreadable) {
			t.Errorf("%v: error = %q, want permission denied naming %s", args, obj.Error, unreadable)
		}
		if len(keys) != 1 {
			t.Errorf("%v: stderr object has keys %v, want only error", args, keys)
		}
	}
}

// handleOpenError strips exactly the sentinel and its separator from the
// parser's reason, and omits detail when the error text does not start with
// that prefix rather than repeating the sentinel text as detail.
func TestHandleOpenError_MalformedDetailText(t *testing.T) {
	for _, tc := range []struct {
		name       string
		err        error
		file       string
		wantDetail *string
	}{
		{
			name:       "reason after the prefix",
			err:        fmt.Errorf("%w: %v", pdfcore.ErrMalformedPDF, errors.New("xref: no startxref")),
			file:       "side.pdf",
			wantDetail: ptr("xref: no startxref"),
		},
		{
			name: "bare sentinel",
			err:  pdfcore.ErrMalformedPDF,
		},
		{
			name: "sentinel wrapped behind other text",
			err:  fmt.Errorf("reading: %w", pdfcore.ErrMalformedPDF),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var code int
			stderr := captureStderr(t, func() { code = handleOpenError(tc.err, tc.file) })
			if code != 2 {
				t.Errorf("exit = %d, want 2", code)
			}
			obj, keys := parseOpenError(t, stderr)
			if obj.Error != malformedOpenError {
				t.Errorf("error = %q, want %q", obj.Error, malformedOpenError)
			}
			switch {
			case tc.wantDetail == nil && obj.Detail != nil:
				t.Errorf("detail = %q, want no detail field", *obj.Detail)
			case tc.wantDetail != nil && obj.Detail == nil:
				t.Errorf("detail missing, want %q", *tc.wantDetail)
			case tc.wantDetail != nil && *obj.Detail != *tc.wantDetail:
				t.Errorf("detail = %q, want %q", *obj.Detail, *tc.wantDetail)
			}
			if _, ok := keys["file"]; ok != (tc.file != "") {
				t.Errorf("file field present = %v, want %v", ok, tc.file != "")
			}
			if tc.file != "" && obj.File != nil && *obj.File != tc.file {
				t.Errorf("file = %q, want %q", *obj.File, tc.file)
			}
		})
	}
}

func ptr(s string) *string { return &s }
