package open_source_docs_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
)

// cliGuidePath is the guide's entry page; cliPagesDir holds the reference
// pages it links to.
const (
	cliGuidePath = "docs/cli-usage.md"
	cliPagesDir  = "docs/cli"
)

var (
	cliBuildOnce sync.Once
	cliBinPath   string
	cliBuildDir  string
	cliBuildErr  string
)

// TestMain removes the temp directory holding the built CLI after the run.
func TestMain(m *testing.M) {
	code := m.Run()
	if cliBuildDir != "" {
		os.RemoveAll(cliBuildDir)
	}
	os.Exit(code)
}

// buildCLI compiles the CLI once per test package and returns the binary path.
func buildCLI(t *testing.T) string {
	t.Helper()
	// Resolved outside the Once: a t.Fatalf inside Do would mark it done with
	// neither a binary nor an error recorded.
	root := projectRoot(t)
	cliBuildOnce.Do(func() {
		binName := "pdfdebug"
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		tmpDir, err := os.MkdirTemp("", "pdfdebug-open-source-docs-")
		if err != nil {
			cliBuildErr = "failed to create temp dir: " + err.Error()
			return
		}
		cliBuildDir = tmpDir
		binPath := filepath.Join(tmpDir, binName)
		cmd := exec.Command("go", "build", "-o", binPath, "./cmd/cli/")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			cliBuildErr = "failed to build CLI binary: " + err.Error() + "\n" + string(out)
			return
		}
		cliBinPath = binPath
	})
	if cliBuildErr != "" {
		t.Fatalf("%s", cliBuildErr)
	}
	return cliBinPath
}

// runCLI runs the CLI with CI=1 so the update notice never fires, and returns
// stdout, stderr and the exit code.
func runCLI(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(buildCLI(t), args...)
	cmd.Env = append(os.Environ(), "CI=1")
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("failed to run CLI: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// helpCommands returns the command names listed in the Commands block of
// `pdfdebug --help`, in help order: `dump <resource>`, `validate`, `diff`.
func helpCommands(t *testing.T) []string {
	t.Helper()
	_, stderr, code := runCLI(t, "--help")
	if code != 0 {
		t.Fatalf("pdfdebug --help exited %d, want 0; stderr:\n%s", code, stderr)
	}
	var cmds []string
	inBlock := false
	for line := range strings.SplitSeq(stderr, "\n") {
		if !inBlock {
			if strings.HasPrefix(line, "Commands (") {
				inBlock = true
			}
			continue
		}
		if strings.TrimSpace(line) == "" {
			break
		}
		fields := strings.Fields(line)
		if fields[0] == "dump" && len(fields) > 1 {
			cmds = append(cmds, "dump "+fields[1])
		} else {
			cmds = append(cmds, fields[0])
		}
	}
	if len(cmds) < 3 {
		t.Fatalf("parsed %d commands from the --help Commands block, want at least 3; stderr:\n%s", len(cmds), stderr)
	}
	return cmds
}

// fixturePath returns the absolute path of a file under the project's testdata/.
func fixturePath(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(projectRoot(t), "testdata", name)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("fixture %s missing: %v", name, err)
	}
	return p
}

// mdLine is one Markdown source line with its fence state.
type mdLine struct {
	text    string
	inFence bool // inside a fenced code block, fence markers included
	marker  bool // the line opens or closes a fenced code block
}

// mdLines splits a Markdown document and marks fenced code block lines. A
// fence closes only on the marker character that opened it, so a ~~~ line
// inside a ``` block stays content.
func mdLines(doc string) []mdLine {
	var out []mdLine
	open := ""
	for l := range strings.SplitSeq(strings.ReplaceAll(doc, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(l)
		marker := ""
		if strings.HasPrefix(trimmed, "```") {
			marker = "```"
		} else if strings.HasPrefix(trimmed, "~~~") {
			marker = "~~~"
		}
		switch {
		case marker != "" && open == "":
			open = marker
			out = append(out, mdLine{text: l, inFence: true, marker: true})
		case marker != "" && marker == open:
			open = ""
			out = append(out, mdLine{text: l, inFence: true, marker: true})
		default:
			out = append(out, mdLine{text: l, inFence: open != ""})
		}
	}
	return out
}

// mdHeading is an ATX heading outside fenced code.
type mdHeading struct {
	level int
	text  string
	index int // line index in the mdLines slice
}

var headingRe = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)

// mdHeadings returns every heading outside fenced code, in document order.
func mdHeadings(lines []mdLine) []mdHeading {
	var hs []mdHeading
	for i, l := range lines {
		if l.inFence {
			continue
		}
		if m := headingRe.FindStringSubmatch(l.text); m != nil {
			hs = append(hs, mdHeading{level: len(m[1]), text: m[2], index: i})
		}
	}
	return hs
}

// sectionLines returns the lines under heading h up to the next heading of
// the same or a higher level.
func sectionLines(lines []mdLine, hs []mdHeading, h mdHeading) []mdLine {
	end := len(lines)
	for _, o := range hs {
		if o.index > h.index && o.level <= h.level {
			end = o.index
			break
		}
	}
	return lines[h.index+1 : end]
}

// findHeading returns the first heading at level whose lowercased text
// satisfies match.
func findHeading(hs []mdHeading, level int, match func(lower string) bool) (mdHeading, bool) {
	for _, h := range hs {
		if h.level == level && match(strings.ToLower(h.text)) {
			return h, true
		}
	}
	return mdHeading{}, false
}

// commandHeading returns the `###` heading whose text is exactly the
// backticked command.
func commandHeading(hs []mdHeading, cmd string) (mdHeading, bool) {
	want := "`" + cmd + "`"
	for _, h := range hs {
		if h.level == 3 && h.text == want {
			return h, true
		}
	}
	return mdHeading{}, false
}

// mdTableRow is one data row of a Markdown table outside fenced code.
type mdTableRow struct {
	table     int // table ordinal in the document
	index     int // line index
	firstCell string
	text      string // the whole row
}

var separatorRowRe = regexp.MustCompile(`^\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)*\|?$`)

// mdTableRows returns the data rows (header and separator rows excluded) of
// every Markdown table outside fenced code.
func mdTableRows(lines []mdLine) []mdTableRow {
	var rows []mdTableRow
	table := -1
	rowInTable := 0
	prevWasTable := false
	for i, l := range lines {
		trimmed := strings.TrimSpace(l.text)
		if l.inFence || !strings.HasPrefix(trimmed, "|") {
			prevWasTable = false
			continue
		}
		if !prevWasTable {
			table++
			rowInTable = 0
		}
		prevWasTable = true
		rowInTable++
		if rowInTable == 1 || separatorRowRe.MatchString(trimmed) {
			continue
		}
		cells := strings.Split(strings.Trim(trimmed, "|"), "|")
		rows = append(rows, mdTableRow{table: table, index: i, firstCell: strings.TrimSpace(cells[0]), text: trimmed})
	}
	return rows
}

// fencedBlocks returns the body of every fenced code block in lines.
func fencedBlocks(lines []mdLine) []string {
	var blocks []string
	var cur []string
	open := false
	for _, l := range lines {
		if l.marker {
			if open {
				blocks = append(blocks, strings.Join(cur, "\n"))
				cur = nil
			}
			open = !open
			continue
		}
		if open {
			cur = append(cur, l.text)
		}
	}
	return blocks
}

// proseText joins the non-fenced lines of a section.
func proseText(lines []mdLine) string {
	var b strings.Builder
	for _, l := range lines {
		if !l.inFence {
			b.WriteString(l.text)
			b.WriteString("\n")
		}
	}
	return b.String()
}

var slugDropRe = regexp.MustCompile(`[^a-z0-9 _-]`)

// githubSlug returns the anchor GitHub generates for a heading.
func githubSlug(heading string) string {
	s := slugDropRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(heading)), "")
	return strings.ReplaceAll(s, " ", "-")
}

// guideDoc is one page of the CLI guide, parsed.
type guideDoc struct {
	path     string // repo-relative, forward-slashed
	lines    []mdLine
	headings []mdHeading
}

// guideDocs returns the entry page followed by every docs/cli/*.md page in
// name order.
func guideDocs(t *testing.T) []guideDoc {
	t.Helper()
	paths := []string{cliGuidePath}
	matches, err := filepath.Glob(filepath.Join(projectRoot(t), cliPagesDir, "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(matches)
	for _, m := range matches {
		paths = append(paths, cliPagesDir+"/"+filepath.Base(m))
	}
	docs := make([]guideDoc, 0, len(paths))
	for _, p := range paths {
		lines := mdLines(readFileAtRoot(t, p))
		docs = append(docs, guideDoc{path: p, lines: lines, headings: mdHeadings(lines)})
	}
	return docs
}

// guideCommandHeading returns the first guide page carrying the `###` heading
// for cmd, and the heading.
func guideCommandHeading(docs []guideDoc, cmd string) (guideDoc, mdHeading, bool) {
	for _, d := range docs {
		if h, ok := commandHeading(d.headings, cmd); ok {
			return d, h, true
		}
	}
	return guideDoc{}, mdHeading{}, false
}

// guideSection returns the first guide page with a heading at level whose
// lowercased text satisfies match, and the heading.
func guideSection(docs []guideDoc, level int, match func(lower string) bool) (guideDoc, mdHeading, bool) {
	for _, d := range docs {
		if h, ok := findHeading(d.headings, level, match); ok {
			return d, h, true
		}
	}
	return guideDoc{}, mdHeading{}, false
}
