package cli_views_test

import (
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Positional-argument arity across the whole command surface.
//
// Go's flag package stops parsing at the first non-flag argument, so a flag
// written after the file path arrives as a spare positional. Dropping it turns
// `dump tree file.pdf --json` into a plain-text run at exit 0 while the caller
// pipes it to a JSON parser. Every command declares the number of file
// positionals it takes and rejects any other count with its usage line.
//
// The `dump` subcommands report a usage error as exit 1; `validate` and `diff`
// use exit 2, their operational-error code.
// ---------------------------------------------------------------------------

// positionalCase describes one command's positional contract: the words and
// flags that precede the file paths, how many paths a valid invocation takes,
// and the exit code a wrong count produces.
type positionalCase struct {
	name     string
	argv     []string
	file     string
	files    int
	usage    string
	wantExit int
}

var positionalCases = []positionalCase{
	{"dump tree", []string{"dump", "tree"}, "minimal.pdf", 1, "Usage: pdfdebug dump tree", 1},
	{"dump object", []string{"dump", "object", "--ref", "1 0 R"}, "minimal.pdf", 1, "Usage: pdfdebug dump object", 1},
	{"dump stream", []string{"dump", "stream", "--page", "1"}, "content-stream.pdf", 1, "Usage: pdfdebug dump stream", 1},
	{"dump page", []string{"dump", "page", "--info", "1"}, "minimal.pdf", 1, "Usage: pdfdebug dump page", 1},
	{"dump font", []string{"dump", "font", "--ref", "4 0 R"}, "fonts-mixed.pdf", 1, "Usage: pdfdebug dump font", 1},
	{"dump image", []string{"dump", "image", "--ref", "4 0 R"}, "image-xobject.pdf", 1, "Usage: pdfdebug dump image", 1},
	{"dump source", []string{"dump", "source", "--ref", "1 0 R"}, "minimal.pdf", 1, "Usage: pdfdebug dump source", 1},
	{"dump reverserefs", []string{"dump", "reverserefs", "--ref", "2 0 R"}, "minimal.pdf", 1, "Usage: pdfdebug dump reverserefs", 1},
	{"dump xref", []string{"dump", "xref"}, "minimal.pdf", 1, "Usage: pdfdebug dump xref", 1},
	{"dump objects", []string{"dump", "objects"}, "minimal.pdf", 1, "Usage: pdfdebug dump objects", 1},
	{"dump bytes", []string{"dump", "bytes"}, "minimal.pdf", 1, "Usage: pdfdebug dump bytes", 1},
	{"dump embedded", []string{"dump", "embedded"}, "minimal.pdf", 1, "Usage: pdfdebug dump embedded", 1},
	{"dump metadata", []string{"dump", "metadata"}, "minimal.pdf", 1, "Usage: pdfdebug dump metadata", 1},
	{"dump signatures", []string{"dump", "signatures"}, "minimal.pdf", 1, "Usage: pdfdebug dump signatures", 1},
	{"validate", []string{"validate"}, "minimal.pdf", 1, "Usage: pdfdebug validate", 2},
	{"diff", []string{"diff"}, "minimal.pdf", 2, "Usage: pdfdebug diff", 2},
}

// invocation returns argv followed by the case's file path repeated n times,
// plus any trailing arguments.
func (c positionalCase) invocation(t *testing.T, n int, trailing ...string) []string {
	t.Helper()
	path := filepath.Join(testdataDir(t), c.file)
	args := append([]string{}, c.argv...)
	for range n {
		args = append(args, path)
	}
	return append(args, trailing...)
}

// A valid invocation, with every flag written before the file, is left alone:
// the usage line must not appear. This is the baseline the two rejection tests
// below are measured against - without it a guard that rejected everything
// would look correct.
func TestPositionalArity_ValidInvocationIsNotAUsageError(t *testing.T) {
	bin := buildCLI(t)

	for _, c := range positionalCases {
		t.Run(c.name, func(t *testing.T) {
			_, stderr, _ := runCLI(t, bin, c.invocation(t, c.files)...)
			if strings.Contains(stderr, c.usage) {
				t.Errorf("%s: a valid invocation was rejected as a usage error, stderr: %q", c.name, stderr)
			}
		})
	}
}

// A flag written after the file is the failure mode in the field: it reaches
// the command as a spare positional and used to be dropped, leaving the caller
// with the default format at exit 0.
func TestPositionalArity_FlagAfterFileIsAUsageError(t *testing.T) {
	bin := buildCLI(t)

	for _, c := range positionalCases {
		t.Run(c.name, func(t *testing.T) {
			stdout, stderr, ec := runCLI(t, bin, c.invocation(t, c.files, "--json")...)
			if ec != c.wantExit {
				t.Errorf("%s: a flag after <file> expected exit %d, got %d", c.name, c.wantExit, ec)
			}
			if stdout != "" {
				t.Errorf("%s: stdout must stay empty, got %d bytes", c.name, len(stdout))
			}
			if !strings.Contains(stderr, c.usage) {
				t.Errorf("%s: stderr should carry the usage line, got: %q", c.name, stderr)
			}
		})
	}
}

// One file path beyond what the command takes is a typo, not an instruction to
// pick the first and ignore the rest.
func TestPositionalArity_ExtraFileIsAUsageError(t *testing.T) {
	bin := buildCLI(t)

	for _, c := range positionalCases {
		t.Run(c.name, func(t *testing.T) {
			stdout, stderr, ec := runCLI(t, bin, c.invocation(t, c.files+1)...)
			if ec != c.wantExit {
				t.Errorf("%s: an extra file expected exit %d, got %d", c.name, c.wantExit, ec)
			}
			if stdout != "" {
				t.Errorf("%s: stdout must stay empty, got %d bytes", c.name, len(stdout))
			}
			if !strings.Contains(stderr, c.usage) {
				t.Errorf("%s: stderr should carry the usage line, got: %q", c.name, stderr)
			}
		})
	}
}

// diff is the one command taking two files, so its usage line has to show two
// operands. A shared arity guard is where a later change is most likely to
// collapse it to one; a one-file usage line would be the first symptom.
func TestPositionalArity_DiffUsageNamesTwoFiles(t *testing.T) {
	bin := buildCLI(t)
	path := filepath.Join(testdataDir(t), "minimal.pdf")

	_, stderr, ec := runCLI(t, bin, "diff", path)
	if ec != 2 {
		t.Errorf("diff with one file expected exit 2, got %d", ec)
	}
	if !strings.Contains(stderr, "<left.pdf> <right.pdf>") {
		t.Errorf("the diff usage line should name both operands, got: %q", stderr)
	}
}

// `--` ends flag parsing, so a path that begins with a dash is still reachable,
// and the terminator itself is not counted as a positional.
func TestPositionalArity_DashTerminatedPathIsAccepted(t *testing.T) {
	bin := buildCLI(t)
	dashPath := filepath.Join(t.TempDir(), "-leading-dash.pdf")
	copyTestdataFile(t, "minimal.pdf", dashPath)

	for _, c := range []struct {
		name  string
		argv  []string
		usage string
	}{
		{"dump tree", []string{"dump", "tree", "--", dashPath}, "Usage: pdfdebug dump tree"},
		{"dump metadata", []string{"dump", "metadata", "--", dashPath}, "Usage: pdfdebug dump metadata"},
		{"diff", []string{"diff", "--", dashPath, dashPath}, "Usage: pdfdebug diff"},
	} {
		t.Run(c.name, func(t *testing.T) {
			stdout, stderr, _ := runCLI(t, bin, c.argv...)
			if strings.Contains(stderr, c.usage) {
				t.Errorf("%s: a -- terminated path was rejected as a usage error, stderr: %q", c.name, stderr)
			}
			if stdout == "" {
				t.Errorf("%s: expected output for a readable file, got none", c.name)
			}
		})
	}
}
