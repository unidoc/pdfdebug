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
//
// The shape check runs first on every command, ahead of any flag-value check,
// so an invocation wrong both ways draws the usage line rather than a complaint
// about a value whose operand the parser never got to.
// ---------------------------------------------------------------------------

// positionalCase describes one command's positional contract: the words that
// precede the file paths, the mode selector a valid invocation needs, how many
// paths it takes, and the exit code a wrong count produces.
type positionalCase struct {
	name string
	argv []string
	// selector is the mode-selector flag and value the command requires before
	// the file (--ref, --page, --info); nil for a command that takes none. The
	// flag-after-file case moves it behind the file, so the selector has to be
	// listed apart from argv rather than baked into it.
	selector []string
	// badValue is the complete set of flags, written ahead of the file, that
	// makes the invocation wrong a second way: a flag value the command
	// rejects. nil for a command that checks no flag value before it runs.
	badValue []string
	// valueError is the text the badValue check writes when the shape is
	// otherwise fine. The shape check runs first, so it must NOT appear when
	// the invocation is wrong both ways.
	valueError string
	file       string
	files      int
	usage      string
	wantExit   int
}

var positionalCases = []positionalCase{
	{"dump tree", []string{"dump", "tree"}, nil, []string{"--page", "0"}, "invalid --page", "minimal.pdf", 1, "Usage: pdfdebug dump tree", 1},
	{"dump object", []string{"dump", "object"}, []string{"--ref", "1 0 R"}, []string{"--ref", "1 0 R", "--resolve-depth", "-1"}, "invalid --resolve-depth", "minimal.pdf", 1, "Usage: pdfdebug dump object", 1},
	{"dump stream", []string{"dump", "stream"}, []string{"--page", "1"}, []string{"--page", "0"}, "invalid --page", "content-stream.pdf", 1, "Usage: pdfdebug dump stream", 1},
	{"dump page", []string{"dump", "page"}, []string{"--info", "1"}, []string{"--info", "0"}, "invalid --info", "minimal.pdf", 1, "Usage: pdfdebug dump page", 1},
	{"dump font", []string{"dump", "font"}, []string{"--ref", "4 0 R"}, []string{"--ref", "not a ref"}, "invalid reference format", "fonts-mixed.pdf", 1, "Usage: pdfdebug dump font", 1},
	{"dump image", []string{"dump", "image"}, []string{"--ref", "4 0 R"}, []string{"--ref", "not a ref"}, "invalid reference format", "image-xobject.pdf", 1, "Usage: pdfdebug dump image", 1},
	{"dump source", []string{"dump", "source"}, []string{"--ref", "1 0 R"}, []string{"--ref", "not a ref"}, "invalid reference format", "minimal.pdf", 1, "Usage: pdfdebug dump source", 1},
	{"dump reverserefs", []string{"dump", "reverserefs"}, []string{"--ref", "2 0 R"}, []string{"--ref", "not a ref"}, "invalid reference format", "minimal.pdf", 1, "Usage: pdfdebug dump reverserefs", 1},
	{"dump xref", []string{"dump", "xref"}, nil, nil, "", "minimal.pdf", 1, "Usage: pdfdebug dump xref", 1},
	{"dump objects", []string{"dump", "objects"}, nil, nil, "", "minimal.pdf", 1, "Usage: pdfdebug dump objects", 1},
	{"dump bytes", []string{"dump", "bytes"}, nil, nil, "", "minimal.pdf", 1, "Usage: pdfdebug dump bytes", 1},
	{"dump embedded", []string{"dump", "embedded"}, nil, []string{"--ref", "1 0 R", "--name", "attachment.xml"}, "mutually exclusive", "minimal.pdf", 1, "Usage: pdfdebug dump embedded", 1},
	{"dump metadata", []string{"dump", "metadata"}, nil, nil, "", "minimal.pdf", 1, "Usage: pdfdebug dump metadata", 1},
	{"dump signatures", []string{"dump", "signatures"}, nil, nil, "", "minimal.pdf", 1, "Usage: pdfdebug dump signatures", 1},
	{"validate", []string{"validate"}, nil, []string{"--profile", "pdfa-9z"}, "unknown profile", "minimal.pdf", 1, "Usage: pdfdebug validate", 2},
	{"diff", []string{"diff"}, nil, nil, "", "minimal.pdf", 2, "Usage: pdfdebug diff", 2},
}

// invocation returns the command words, its mode selector, then the case's file
// path repeated n times - every flag ahead of the operands.
func (c positionalCase) invocation(t *testing.T, n int) []string {
	t.Helper()
	args := append([]string{}, c.argv...)
	args = append(args, c.selector...)
	return append(args, c.paths(t, n)...)
}

// flagAfterFile returns the same command with a flag moved behind the file: the
// mode selector for a command that takes one, a plain --json otherwise. Moving
// the selector is what reaches the mode check - appending a flag the command
// parses happily leaves that path untouched.
func (c positionalCase) flagAfterFile(t *testing.T, n int) []string {
	t.Helper()
	trailing := []string{"--json"}
	if len(c.selector) > 0 {
		trailing = c.selector
	}
	args := append([]string{}, c.argv...)
	args = append(args, c.paths(t, n)...)
	return append(args, trailing...)
}

// badValueOnly returns the command with its rejected flag value ahead of the
// file and nothing else wrong, so the value check is the only one that can
// fire. It is the baseline for wrongBothWays: without it a value check that had
// quietly stopped rejecting anything would make that test pass for free.
func (c positionalCase) badValueOnly(t *testing.T, n int) []string {
	t.Helper()
	args := append([]string{}, c.argv...)
	args = append(args, c.badValue...)
	return append(args, c.paths(t, n)...)
}

// wrongBothWays returns the command wrong in two ways at once: a flag value it
// rejects ahead of the file, and a flag behind the file that arrives as a spare
// positional. Both checks have something to report; the shape one goes first.
func (c positionalCase) wrongBothWays(t *testing.T, n int) []string {
	t.Helper()
	return append(c.badValueOnly(t, n), "--json")
}

// paths returns the case's file path repeated n times.
func (c positionalCase) paths(t *testing.T, n int) []string {
	t.Helper()
	path := filepath.Join(testdataDir(t), c.file)
	args := make([]string, 0, n)
	for range n {
		args = append(args, path)
	}
	return args
}

// A valid invocation, with every flag written before the file, is left alone:
// the usage line must not appear. This is the baseline the rejection tests
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
// the command as a spare positional, so the whole invocation is a shape error
// and draws the usage line. For a command whose mode selector is the flag left
// behind the file, that shape has to be reported before the mode check reads a
// selector the caller did pass, just not where the parser could see it.
func TestPositionalArity_FlagAfterFileIsAUsageError(t *testing.T) {
	bin := buildCLI(t)

	for _, c := range positionalCases {
		t.Run(c.name, func(t *testing.T) {
			stdout, stderr, ec := runCLI(t, bin, c.flagAfterFile(t, c.files)...)
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

// A rejected flag value, with the shape otherwise correct, is reported as the
// value error it is. This is what wrongBothWays below must NOT produce.
func TestPositionalArity_BadFlagValueIsReportedWhenTheShapeIsRight(t *testing.T) {
	bin := buildCLI(t)

	for _, c := range positionalCases {
		if c.badValue == nil {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			_, stderr, ec := runCLI(t, bin, c.badValueOnly(t, c.files)...)
			if ec != c.wantExit {
				t.Errorf("%s: a rejected flag value expected exit %d, got %d", c.name, c.wantExit, ec)
			}
			if !strings.Contains(stderr, c.valueError) {
				t.Errorf("%s: stderr should carry %q, got: %q", c.name, c.valueError, stderr)
			}
		})
	}
}

// An invocation wrong in BOTH ways - a flag value the command rejects AND a
// flag written after the file - draws the usage line. Shape is checked first
// across the whole surface, so the value error never reaches stderr: complaining
// about a flag the parser did see, while staying silent about the operand shape
// that stopped it seeing the rest, sends the caller after the wrong fix.
//
// Commands with no flag value to reject are skipped; they have no ordering to
// pin.
func TestPositionalArity_ShapeErrorWinsOverFlagValueError(t *testing.T) {
	bin := buildCLI(t)

	for _, c := range positionalCases {
		if c.badValue == nil {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			stdout, stderr, ec := runCLI(t, bin, c.wrongBothWays(t, c.files)...)
			if ec != c.wantExit {
				t.Errorf("%s: wrong both ways expected exit %d, got %d", c.name, c.wantExit, ec)
			}
			if stdout != "" {
				t.Errorf("%s: stdout must stay empty, got %d bytes", c.name, len(stdout))
			}
			if !strings.Contains(stderr, c.usage) {
				t.Errorf("%s: stderr should carry the usage line, got: %q", c.name, stderr)
			}
			if strings.Contains(stderr, c.valueError) {
				t.Errorf("%s: the shape error must win, but stderr carries %q: %q", c.name, c.valueError, stderr)
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
// and the terminator itself is not counted as a positional. The path is passed
// relative, with the command's working directory set to the file's own
// directory, so the argv element really does begin with a dash - an absolute
// path would start with a slash and exercise nothing.
func TestPositionalArity_DashTerminatedPathIsAccepted(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	const dashPath = "-leading-dash.pdf"
	copyTestdataFile(t, "minimal.pdf", filepath.Join(dir, dashPath))

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
			stdout, stderr, _ := runCLIIn(t, dir, bin, c.argv...)
			if strings.Contains(stderr, c.usage) {
				t.Errorf("%s: a -- terminated path was rejected as a usage error, stderr: %q", c.name, stderr)
			}
			if stdout == "" {
				t.Errorf("%s: expected output for a readable file, got none", c.name)
			}
		})
	}
}

// An explicitly empty operand, `dump tree ""`, names no file. It satisfies the
// positional count, so the guard has to reject the value as well: the same
// usage line and exit code a missing operand draws, not a fall-through to the
// runtime path reporting the empty string as a file that was not found. The
// second `diff` operand covers a position other than the first.
func TestPositionalArity_EmptyFileOperandIsAUsageError(t *testing.T) {
	bin := buildCLI(t)
	path := filepath.Join(testdataDir(t), "minimal.pdf")

	for _, c := range []struct {
		name     string
		argv     []string
		usage    string
		wantExit int
	}{
		{"dump tree", []string{"dump", "tree", ""}, "Usage: pdfdebug dump tree", 1},
		{"dump bytes", []string{"dump", "bytes", ""}, "Usage: pdfdebug dump bytes", 1},
		{"validate", []string{"validate", ""}, "Usage: pdfdebug validate", 2},
		{"diff first operand", []string{"diff", "", path}, "Usage: pdfdebug diff", 2},
		{"diff second operand", []string{"diff", path, ""}, "Usage: pdfdebug diff", 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			stdout, stderr, ec := runCLI(t, bin, c.argv...)
			if ec != c.wantExit {
				t.Errorf("%s: an empty <file> expected exit %d, got %d", c.name, c.wantExit, ec)
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
