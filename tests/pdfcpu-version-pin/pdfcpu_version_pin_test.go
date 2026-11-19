// Package pdfcpu_version_pin_test pins the pdfcpu release the root module builds
// against and the contract surface that has to survive a pdfcpu upgrade:
//
//   - go.mod and go.sum carry pdfcpu at targetVersion
//   - safeCall re-panics a runtime.Error, and its named tests still exist
//   - the image memory guards and the two pdfcpu_render call sites are intact
//   - every stream decode in internal/pdfcore goes through the decodeStream
//     helper inside safeCall, and none calls pdfcpu's Decode or DecodeLength
//   - internal/lzwdecode carries pdfcpu's LZW decoder, its LICENSE, and the
//     pdfcpu version it was copied from, which must equal targetVersion
//   - pdfcore/doc.go keeps the blank import that pins pdfcpu in go.mod
//
// Run: cd tests/pdfcpu-version-pin && go test -count=1 ./...
package pdfcpu_version_pin_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// targetVersion is the pdfcpu release go.mod must require. When it changes,
// re-copy internal/lzwdecode from the same pdfcpu release.
const targetVersion = "v0.16.4"

// previousVersion is the release go.mod must no longer carry.
const previousVersion = "v0.12.1"

const pdfcpuModulePath = "github.com/pdfcpu/pdfcpu"

func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil {
			if strings.Contains(string(data), "module unidoc-pdf-debugger") {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find project root (no go.mod with module unidoc-pdf-debugger)")
		}
		dir = parent
	}
}

func readSource(t *testing.T, relPath string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(projectRoot(t), filepath.FromSlash(relPath)))
	if err != nil {
		t.Fatalf("cannot read %s: %v", relPath, err)
	}
	return string(content)
}

func findRequireLine(src string) string {
	for line := range strings.SplitSeq(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, pdfcpuModulePath+" ") {
			return trimmed
		}
	}
	return ""
}

func TestGoModPdfcpuAtTargetVersion(t *testing.T) {
	line := findRequireLine(readSource(t, "go.mod"))
	if line == "" {
		t.Fatalf("go.mod must declare a `%s` require", pdfcpuModulePath)
	}
	if want := pdfcpuModulePath + " " + targetVersion; !strings.HasPrefix(line, want) {
		t.Errorf("go.mod pdfcpu require line is %q, want %q", line, want)
	}
}

func TestGoModDoesNotCarryPreviousVersion(t *testing.T) {
	line := findRequireLine(readSource(t, "go.mod"))
	if strings.Contains(line, pdfcpuModulePath+" "+previousVersion) {
		t.Errorf("go.mod pdfcpu require line still reads %q", line)
	}
}

func TestGoSumCarriesTargetVersion(t *testing.T) {
	if !strings.HasPrefix(findRequireLine(readSource(t, "go.mod")), pdfcpuModulePath+" "+targetVersion) {
		t.Skip("go.mod does not require the target pdfcpu version yet")
	}
	gosum := readSource(t, "go.sum")
	for _, want := range []string{
		pdfcpuModulePath + " " + targetVersion + " h1:",
		pdfcpuModulePath + " " + targetVersion + "/go.mod h1:",
	} {
		if !strings.Contains(gosum, want) {
			t.Errorf("go.sum has no line starting %q; run `go mod tidy`", want)
		}
	}
	if stale := pdfcpuModulePath + " " + previousVersion + " h1:"; strings.Contains(gosum, stale) {
		t.Errorf("go.sum still carries %q; run `go mod tidy`", stale)
	}
}

func TestSafeCallRePanicsRuntimeError(t *testing.T) {
	src := readSource(t, "internal/pdfcore/errors.go")
	if !regexp.MustCompile(`if\s+_,\s*ok\s*:=\s*r\.\(runtime\.Error\)`).MatchString(src) {
		t.Errorf("internal/pdfcore/errors.go must keep the runtime.Error type assertion in safeCall's recover block")
	}
	if !strings.Contains(src, "panic(r)") {
		t.Errorf("internal/pdfcore/errors.go must keep the `panic(r)` re-panic in safeCall")
	}
}

func TestSafeCallNamedTestsExist(t *testing.T) {
	src := readSource(t, "internal/pdfcore/errors_test.go")
	for _, name := range []string{
		"TestSafeCallPropagatesRuntimeError",
		"TestSafeCallSuccess",
		"TestSafeCallReturnsError",
		"TestSafeCallCatchesStringPanic",
		"TestSafeCallCatchesErrorPanic",
		"TestWrapPDFErrorPasswordBecomesEncrypted",
		"TestWrapPDFErrorOwnerPasswordBecomesEncrypted",
		"TestWrapPDFErrorPanicPrefixBecomesMalformed",
		"TestWrapPDFErrorGenericBecomesMalformed",
		"TestWrapPDFErrorPreservesOriginal",
	} {
		if !strings.Contains(src, "func "+name+"(t *testing.T)") {
			t.Errorf("internal/pdfcore/errors_test.go must still declare %s", name)
		}
	}
}

func TestImageMemoryGuardsUnchanged(t *testing.T) {
	limits := readSource(t, "internal/pdfcore/imagelimits.go")
	for _, g := range []string{
		"maxImageBytes = 50 * 1024 * 1024",
		"maxImagePixels = 100_000_000",
		"maxImageDecodeBytes = 512 * 1024 * 1024",
	} {
		if !strings.Contains(limits, g) {
			t.Errorf("internal/pdfcore/imagelimits.go must keep %q", g)
		}
	}
	if !strings.Contains(readSource(t, "internal/pdfcore/image.go"), "io.LimitReader(reader, maxImageDecodeBytes+1)") {
		t.Errorf("internal/pdfcore/image.go must cap the full-resolution render read at maxImageDecodeBytes+1")
	}
}

func TestImageRenderCallsiteIntact(t *testing.T) {
	src := readSource(t, "internal/pdfcore/image.go")
	for _, c := range []string{"pdfcpu_render.ColorSpaceComponents(", "pdfcpu_render.RenderImage("} {
		if !strings.Contains(src, c) {
			t.Errorf("internal/pdfcore/image.go must still call %s", c)
		}
	}
}

// TestStreamDecodeWrappedInSafeCall checks that the content-stream decode in
// stream.go calls decodeStream inside a safeCall closure.
func TestStreamDecodeWrappedInSafeCall(t *testing.T) {
	lines := strings.Split(readSource(t, "internal/pdfcore/stream.go"), "\n")
	wrapped := false
	for i, line := range lines {
		if !strings.Contains(line, "decodeStream(") {
			continue
		}
		for j := i; j >= max(i-5, 0); j-- {
			if strings.Contains(lines[j], "safeCall(func() error {") {
				wrapped = true
			}
		}
	}
	if !wrapped {
		t.Errorf("internal/pdfcore/stream.go must call decodeStream inside a `safeCall(func() error {` closure")
	}
}

// TestNoDirectPdfcpuStreamDecodeInProduction checks that no production file in
// internal/pdfcore decodes a stream through pdfcpu's Decode or DecodeLength;
// every site goes through decodeStream.
func TestNoDirectPdfcpuStreamDecodeInProduction(t *testing.T) {
	root := projectRoot(t)
	files, err := filepath.Glob(filepath.Join(root, "internal", "pdfcore", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	direct := regexp.MustCompile(`\.Decode\(\)|\.DecodeLength\(`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for n, line := range strings.Split(string(data), "\n") {
			code, _, _ := strings.Cut(line, "//")
			if direct.MatchString(code) {
				t.Errorf("%s:%d decodes through pdfcpu directly: %s", filepath.Base(f), n+1, strings.TrimSpace(line))
			}
		}
	}
	helper := readSource(t, "internal/pdfcore/boundeddecode.go")
	if !strings.Contains(helper, "func decodeStream(") {
		t.Errorf("internal/pdfcore/boundeddecode.go must define decodeStream")
	}
}

func TestInlineImagePayloadTestExists(t *testing.T) {
	if !strings.Contains(readSource(t, "internal/pdfcore/stream_test.go"), "func TestTokenizeInlineImagePayloadOpaque(t *testing.T)") {
		t.Errorf("internal/pdfcore/stream_test.go must still declare TestTokenizeInlineImagePayloadOpaque")
	}
}

func TestPdfcpuBlankImportPinPreserved(t *testing.T) {
	if !strings.Contains(readSource(t, "internal/pdfcore/doc.go"), `import _ "github.com/pdfcpu/pdfcpu/pkg/api"`) {
		t.Errorf("internal/pdfcore/doc.go must keep the blank import of github.com/pdfcpu/pdfcpu/pkg/api")
	}
}

// TestLZWDecoderCopiedFromTargetVersion checks internal/lzwdecode: the copied
// decoder, its LICENSE, and a package comment naming the pdfcpu package and
// release it came from. A pdfcpu bump fails here until the decoder is re-copied.
func TestLZWDecoderCopiedFromTargetVersion(t *testing.T) {
	root := projectRoot(t)
	dir := filepath.Join(root, "internal", "lzwdecode")
	if _, err := os.Stat(filepath.Join(dir, "LICENSE")); err != nil {
		t.Errorf("internal/lzwdecode/LICENSE: %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	var pkgDoc string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if doc := packageComment(string(data)); doc != "" {
			pkgDoc += doc
		}
	}
	if pkgDoc == "" {
		t.Fatalf("internal/lzwdecode has no package comment directly above `package lzwdecode`")
	}
	if !strings.Contains(pkgDoc, "github.com/pdfcpu/pdfcpu/internal/filter/lzw") {
		t.Errorf("internal/lzwdecode package comment must name its origin, github.com/pdfcpu/pdfcpu/internal/filter/lzw:\n%s", pkgDoc)
	}
	if !strings.Contains(pkgDoc, targetVersion) {
		t.Errorf("internal/lzwdecode package comment must name pdfcpu %s, the release go.mod requires; re-copy the decoder from that release:\n%s", targetVersion, pkgDoc)
	}
}

// packageComment returns the comment lines directly above `package lzwdecode`,
// "" when there are none.
func packageComment(src string) string {
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "package lzwdecode" {
			continue
		}
		var doc []string
		for j := i - 1; j >= 0 && strings.HasPrefix(strings.TrimSpace(lines[j]), "//"); j-- {
			doc = append([]string{lines[j]}, doc...)
		}
		return strings.Join(doc, "\n")
	}
	return ""
}

// gitGrep returns git grep's matches for a fixed string over tracked and
// untracked files, "" when there are none.
func gitGrep(t *testing.T, root, needle string, pathspec ...string) string {
	t.Helper()
	args := append([]string{"-C", root, "grep", "-n", "--untracked", "-F", needle, "--"}, pathspec...)
	out, err := exec.Command("git", args...).CombinedOutput()
	if err == nil {
		return string(out)
	}
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
		return ""
	}
	t.Fatalf("git grep %s: %v\n%s", needle, err, out)
	return ""
}

func TestNoFileImportsHhrutterLZW(t *testing.T) {
	root := projectRoot(t)
	// Built from parts so this file does not match its own search.
	module := "github.com/hhrutter/" + "lzw"
	if out := gitGrep(t, root, `"`+module+`"`, "*.go"); out != "" {
		t.Errorf("these files still import %s:\n%s", module, out)
	}
	if strings.Contains(readSource(t, "go.mod"), module) {
		t.Errorf("go.mod still requires %s", module)
	}
}

func TestNoticeCreditsTheLZWDecoder(t *testing.T) {
	notice := readSource(t, "NOTICE")
	for _, want := range []string{"internal/lzwdecode", "The Go Authors", "The pdfcpu Authors", "BSD"} {
		if !strings.Contains(notice, want) {
			t.Errorf("NOTICE must credit the copied LZW decoder; %q is missing", want)
		}
	}
}

// TestPreviousPinSuiteIsGone checks that the version-named pin suite was
// replaced by this one and nothing outside CHANGELOG.md still names it.
func TestPreviousPinSuiteIsGone(t *testing.T) {
	root := projectRoot(t)
	// Built from parts so this file does not match its own search.
	oldDir := strings.Join([]string{"pdfcpu", "0", "12", "1", "bump"}, "-")
	if _, err := os.Stat(filepath.Join(root, "tests", oldDir)); err == nil {
		t.Errorf("tests/%s still exists; this suite replaces it", oldDir)
	}
	if out := gitGrep(t, root, oldDir, ".", ":!CHANGELOG.md"); out != "" {
		t.Errorf("files outside CHANGELOG.md still name %s:\n%s", oldDir, out)
	}
}
