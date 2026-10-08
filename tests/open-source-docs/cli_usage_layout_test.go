package open_source_docs_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

var mdLinkRe = regexp.MustCompile(`\[[^\]]+\]\(#([^)\s]+)\)`)

// TestCLIUsageGuideContentsLinksEverySection asserts the first H2 is a
// contents list whose in-page links resolve to H2 headings and cover every
// other H2 section.
func TestCLIUsageGuideContentsLinksEverySection(t *testing.T) {
	lines := mdLines(readFileAtRoot(t, cliGuidePath))
	hs := mdHeadings(lines)

	var h2 []mdHeading
	for _, h := range hs {
		if h.level == 2 {
			h2 = append(h2, h)
		}
	}
	if len(h2) == 0 || !strings.Contains(strings.ToLower(h2[0].text), "contents") {
		first := "<none>"
		if len(h2) > 0 {
			first = h2[0].text
		}
		t.Fatalf("%s: first H2 is %q, want a contents section", cliGuidePath, first)
	}

	linked := map[string]bool{}
	for _, m := range mdLinkRe.FindAllStringSubmatch(proseText(sectionLines(lines, hs, h2[0])), -1) {
		linked[m[1]] = true
	}
	anchors := map[string]bool{}
	for _, h := range h2[1:] {
		a := githubSlug(h.text)
		if anchors[a] {
			t.Errorf("%s has two H2 headings with anchor #%s; GitHub gives the second #%s-1", cliGuidePath, a, a)
		}
		anchors[a] = true
	}

	for a := range linked {
		if !anchors[a] {
			t.Errorf("%s contents links #%s, which is not the anchor of any H2 heading", cliGuidePath, a)
		}
	}
	for _, h := range h2[1:] {
		if !linked[githubSlug(h.text)] {
			t.Errorf("%s contents has no link to H2 %q (#%s)", cliGuidePath, h.text, githubSlug(h.text))
		}
	}
}

// TestCLIUsageGuideCommonTasksPrecedeReference asserts a Common tasks H2 sits
// above the quick-reference table and any command reference heading on the
// entry page, holds four `###` tasks, and each task shows a pdfdebug command in
// a fenced block or links to the section that does.
func TestCLIUsageGuideCommonTasksPrecedeReference(t *testing.T) {
	cmds := helpCommands(t)
	lines := mdLines(readFileAtRoot(t, cliGuidePath))
	hs := mdHeadings(lines)

	tasks, ok := findHeading(hs, 2, func(s string) bool { return strings.Contains(s, "common tasks") })
	if !ok {
		t.Fatalf("%s has no Common tasks H2", cliGuidePath)
	}
	if table, ok := quickReferenceTable(mdTableRows(lines), cmds); ok && table[0].index < tasks.index {
		t.Errorf("%s: quick-reference table (line %d) comes before Common tasks (line %d)",
			cliGuidePath, table[0].index+1, tasks.index+1)
	}
	for _, c := range cmds {
		if h, ok := commandHeading(hs, c); ok && h.index < tasks.index {
			t.Errorf("%s: reference heading %s (line %d) comes before Common tasks", cliGuidePath, h.text, h.index+1)
		}
	}

	body := sectionLines(lines, hs, tasks)
	bodyHeadings := mdHeadings(body)
	var taskHeadings []mdHeading
	for _, h := range bodyHeadings {
		if h.level == 3 {
			taskHeadings = append(taskHeadings, h)
		}
	}
	if len(taskHeadings) != 4 {
		t.Fatalf("%s Common tasks has %d ### tasks, want 4", cliGuidePath, len(taskHeadings))
	}
	for _, th := range taskHeadings {
		sec := sectionLines(body, bodyHeadings, th)
		n := 0
		for _, b := range fencedBlocks(sec) {
			n += len(pdfdebugCommandLines(b))
		}
		if n == 0 && !strings.Contains(proseText(sec), "](") {
			t.Errorf("%s Common tasks %q shows no pdfdebug command and links to no section that does", cliGuidePath, th.text)
		}
	}
}

// TestCLIUsageGuidePDFUATaskNamesProfileAndVeraPDF asserts a Common tasks entry
// runs the pdfua-1-structural profile and names veraPDF for a conformance
// verdict.
func TestCLIUsageGuidePDFUATaskNamesProfileAndVeraPDF(t *testing.T) {
	lines := mdLines(readFileAtRoot(t, cliGuidePath))
	hs := mdHeadings(lines)

	tasks, ok := findHeading(hs, 2, func(s string) bool { return strings.Contains(s, "common tasks") })
	if !ok {
		t.Fatalf("%s has no Common tasks H2", cliGuidePath)
	}
	body := sectionLines(lines, hs, tasks)
	bodyHeadings := mdHeadings(body)
	for _, th := range bodyHeadings {
		if th.level != 3 {
			continue
		}
		sec := sectionLines(body, bodyHeadings, th)
		runsProfile := false
		for _, b := range fencedBlocks(sec) {
			if strings.Contains(b, "pdfdebug validate --profile pdfua-1-structural") {
				runsProfile = true
			}
		}
		if runsProfile {
			if !strings.Contains(strings.ToLower(proseText(sec)), "verapdf") {
				t.Errorf("%s Common tasks %q runs pdfua-1-structural but does not name veraPDF", cliGuidePath, th.text)
			}
			return
		}
	}
	t.Errorf("%s Common tasks has no entry that runs `pdfdebug validate --profile pdfua-1-structural`", cliGuidePath)
}

// TestCLIUsageGuideCIExampleFailsOnFindingsAndOperationalErrors runs the
// guide's `validate --json` + jq CI snippet, found on any guide page, under sh with file.pdf replaced by
// a fixture: it must fail on an untagged file, pass on a tagged one, and fail
// when the file is missing.
func TestCLIUsageGuideCIExampleFailsOnFindingsAndOperationalErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the CI snippet is POSIX sh")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not on PATH")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not on PATH")
	}

	var snippet, where string
	for _, d := range guideDocs(t) {
		for _, b := range fencedBlocks(d.lines) {
			if snippet == "" && strings.Contains(b, "pdfdebug validate") && strings.Contains(b, "--json") && strings.Contains(b, "jq") {
				snippet, where = b, d.path
			}
		}
	}
	if snippet == "" {
		t.Fatalf("no page of the CLI guide has a fenced CI example that runs `pdfdebug validate --json` and reads the result with jq")
	}
	if !strings.Contains(snippet, "file.pdf") {
		t.Fatalf("%s CI example does not use the file.pdf placeholder:\n%s", where, snippet)
	}
	if !strings.Contains(snippet, "pdfua-1-structural") {
		t.Fatalf("%s CI example does not run the pdfua-1-structural profile, whose findings are warnings with exit 0:\n%s",
			where, snippet)
	}

	binDir := filepath.Dir(buildCLI(t))
	cases := []struct {
		name     string
		file     string
		wantFail bool
	}{
		{"untagged file with findings", fixturePath(t, "untagged.pdf"), true},
		{"tagged file without findings", fixturePath(t, "tagged.pdf"), false},
		{"missing file", filepath.Join(t.TempDir(), "missing.pdf"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			work := t.TempDir()
			script := filepath.Join(work, "ci.sh")
			// Single-quoted so a path with spaces or shell metacharacters stays one argument.
			quoted := "'" + strings.ReplaceAll(tc.file, "'", `'\''`) + "'"
			if err := os.WriteFile(script, []byte(strings.ReplaceAll(snippet, "file.pdf", quoted)+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", script)
			cmd.Dir = work
			cmd.Env = append(os.Environ(), "CI=1", "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			failed := err != nil
			if failed != tc.wantFail {
				t.Errorf("CI example on %s: failed=%v, want failed=%v; output:\n%s", tc.name, failed, tc.wantFail, out)
			}
		})
	}
}

// TestCLIUsageGuideMachineOutputSectionNamesFormats asserts a machine-output
// H2 exists on a guide page and names --pretty, the --raw/--ops/dump bytes machine formats and
// the experimental marker of `dump page --info`.
func TestCLIUsageGuideMachineOutputSectionNamesFormats(t *testing.T) {
	d, h, ok := guideSection(guideDocs(t), 2, func(s string) bool {
		return strings.Contains(s, "machine output") || strings.Contains(s, "json")
	})
	if !ok {
		t.Fatalf("no page of the CLI guide has a machine-output H2")
	}
	text := proseText(sectionLines(d.lines, d.headings, h))
	for _, tok := range []string{"--json", "--pretty", "--raw", "--ops", "dump bytes", "_stability"} {
		if !strings.Contains(text, tok) {
			t.Errorf("%s section %q does not mention %s", d.path, h.text, tok)
		}
	}
}

// TestCLIUsageGuideUpdateNoticeOpensWithOptOuts asserts the first paragraph of
// the update-notice section, on whichever guide page holds it, names the CI variable and both opt-out variables.
func TestCLIUsageGuideUpdateNoticeOpensWithOptOuts(t *testing.T) {
	d, h, ok := guideSection(guideDocs(t), 2, func(s string) bool { return strings.Contains(s, "update notice") })
	if !ok {
		t.Fatalf("no page of the CLI guide has an update-notice H2")
	}
	var para []string
	for _, l := range sectionLines(d.lines, d.headings, h) {
		if l.inFence || strings.TrimSpace(l.text) == "" {
			if len(para) > 0 {
				break
			}
			continue
		}
		para = append(para, l.text)
	}
	first := strings.Join(para, " ")
	for _, tok := range []string{"`CI`", "PDFDEBUG_NO_UPDATE_CHECK", "NO_UPDATE_NOTIFIER"} {
		if !strings.Contains(first, tok) {
			t.Errorf("%s: first paragraph of %q does not name %s:\n%s", d.path, h.text, tok, first)
		}
	}
}

// TestCLIUsageGuideBytesReferenceKeepsAliasStatement asserts the `dump bytes`
// reference subsection carries the `dump plaintext` alias and its 0.6.0
// removal in one paragraph.
func TestCLIUsageGuideBytesReferenceKeepsAliasStatement(t *testing.T) {
	d, h, ok := guideCommandHeading(guideDocs(t), "dump bytes")
	if !ok {
		t.Fatalf("no page of the CLI guide has a ### `dump bytes` reference heading")
	}
	for _, p := range paragraphs(sectionLines(d.lines, d.headings, h)) {
		if strings.Contains(p, "dump plaintext") && strings.Contains(p, "removed in 0.6.0") {
			return
		}
	}
	t.Errorf("%s ### `dump bytes` has no paragraph naming `dump plaintext` with \"removed in 0.6.0\"", d.path)
}

// TestCLIUsageGuideImageReferenceNamesSampleInterpretationFields asserts the
// `dump image` reference subsection names every sample-interpretation field the
// plain output can print, and that the fields the binary prints for an
// uncomplicated JPEG are among them.
func TestCLIUsageGuideImageReferenceNamesSampleInterpretationFields(t *testing.T) {
	fields := []string{"Interpretation", "Decode", "AdobeMarker", "AdobeTransform", "SMask", "ImageMask"}

	stdout, stderr, code := runCLI(t, "dump", "image", "--metadata", "--ref", "4 0 R", fixturePath(t, "image-xobject.pdf"))
	if code != 0 {
		t.Fatalf("dump image --metadata exited %d; stderr:\n%s", code, stderr)
	}
	for _, f := range []string{"Interpretation:", "AdobeMarker:"} {
		if !strings.Contains(stdout, f) {
			t.Fatalf("dump image --metadata no longer prints %s; update the field list in this test:\n%s", f, stdout)
		}
	}

	d, h, ok := guideCommandHeading(guideDocs(t), "dump image")
	if !ok {
		t.Fatalf("no page of the CLI guide has a ### `dump image` reference heading")
	}
	text := strings.Join(sectionText(sectionLines(d.lines, d.headings, h)), "\n")
	for _, f := range fields {
		// Backticked, so a /Decode or /SMask in the prose does not stand in for the field.
		if !strings.Contains(text, "`"+f+"`") {
			t.Errorf("%s ### `dump image` does not name the `%s` field", d.path, f)
		}
	}
}

// TestCLIUsageGuideTreeReferenceDescribesTruncation asserts the `dump tree` or
// `dump object` reference subsection describes the scalar-value truncation
// marker.
func TestCLIUsageGuideTreeReferenceDescribesTruncation(t *testing.T) {
	docs := guideDocs(t)

	found := false
	for _, c := range []string{"dump tree", "dump object"} {
		d, h, ok := guideCommandHeading(docs, c)
		if !ok {
			t.Errorf("no page of the CLI guide has a ### `%s` reference heading", c)
			continue
		}
		if strings.Contains(strings.Join(sectionText(sectionLines(d.lines, d.headings, h)), "\n"), "[truncated:") {
			found = true
		}
	}
	if !found {
		t.Errorf("CLI guide: neither ### `dump tree` nor ### `dump object` describes the `[truncated: N of M]` marker")
	}
}

// TestCLIUsageGuideValidateReferenceNamesProfilesAndRulesBlock asserts the
// `validate` reference subsection names both profiles and the rules-checked
// block of the plain output.
func TestCLIUsageGuideValidateReferenceNamesProfilesAndRulesBlock(t *testing.T) {
	_, stderr, code := runCLI(t, "--help")
	if code != 0 {
		t.Fatalf("pdfdebug --help exited %d", code)
	}
	m := regexp.MustCompile(`valid profiles: (.+)`).FindStringSubmatch(stderr)
	if m == nil {
		t.Fatalf("pdfdebug --help no longer lists `valid profiles:`:\n%s", stderr)
	}
	var profiles []string
	for p := range strings.SplitSeq(m[1], ",") {
		if f := strings.Fields(p); len(f) > 0 {
			profiles = append(profiles, f[0])
		}
	}
	if len(profiles) == 0 {
		t.Fatalf("pdfdebug --help `valid profiles:` line names no profile: %q", m[0])
	}

	d, h, ok := guideCommandHeading(guideDocs(t), "validate")
	if !ok {
		t.Fatalf("no page of the CLI guide has a ### `validate` reference heading")
	}
	text := strings.Join(sectionText(sectionLines(d.lines, d.headings, h)), "\n")
	for _, p := range profiles {
		if !strings.Contains(text, p) {
			t.Errorf("%s ### `validate` does not name the %s profile", d.path, p)
		}
	}
	if !strings.Contains(strings.ToLower(text), "rules checked") {
		t.Errorf("%s ### `validate` does not describe the Rules checked block", d.path)
	}
}

// TestChangelogRecordsCLIGuideRefresh asserts a `### Changed` section of
// CHANGELOG.md, in any release, has an entry naming the guide's entry page and
// its docs/cli/ topic pages.
func TestChangelogRecordsCLIGuideRefresh(t *testing.T) {
	lines := mdLines(readFileAtRoot(t, "CHANGELOG.md"))
	hs := mdHeadings(lines)
	for _, h := range hs {
		if h.level != 3 || h.text != "Changed" {
			continue
		}
		for _, p := range paragraphs(sectionLines(lines, hs, h)) {
			if strings.HasPrefix(p, "- ") && strings.Contains(p, "`"+cliGuidePath+"`") && strings.Contains(p, "`"+cliPagesDir+"/`") {
				return
			}
		}
	}
	t.Errorf("no `### Changed` entry in CHANGELOG.md names `%s` and `%s/`", cliGuidePath, cliPagesDir)
}

// TestCLIUsageDocsAreASCII asserts every page of the CLI guide, README and
// CONTRIBUTING contain only printable ASCII, tabs and newlines.
func TestCLIUsageDocsAreASCII(t *testing.T) {
	files := []string{"README.md", "CONTRIBUTING.md"}
	for _, d := range guideDocs(t) {
		files = append(files, d.path)
	}
	for _, f := range files {
		for i, line := range strings.Split(readFileAtRoot(t, f), "\n") {
			for _, r := range line {
				if (r < 0x20 || r > 0x7e) && r != '\t' && r != '\r' {
					t.Errorf("%s:%d: non-ASCII character %q", f, i+1, r)
					break
				}
			}
		}
	}
}

// pdfdebugCommandLines returns the lines of a code block that invoke pdfdebug.
func pdfdebugCommandLines(block string) []string {
	var out []string
	for l := range strings.SplitSeq(block, "\n") {
		l = strings.TrimPrefix(strings.TrimSpace(l), "$ ")
		if strings.HasPrefix(l, "pdfdebug ") {
			out = append(out, l)
		}
	}
	return out
}

// paragraphs splits non-fenced lines into blank-line-separated paragraphs,
// treating each list item as its own paragraph.
func paragraphs(lines []mdLine) []string {
	var out []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.Join(cur, " "))
			cur = nil
		}
	}
	for _, l := range lines {
		trimmed := strings.TrimSpace(l.text)
		if l.inFence || trimmed == "" {
			flush()
			continue
		}
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			flush()
		}
		cur = append(cur, trimmed)
	}
	flush()
	return out
}

// sectionText returns every line of a section, fenced or not.
func sectionText(lines []mdLine) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.text
	}
	return out
}
