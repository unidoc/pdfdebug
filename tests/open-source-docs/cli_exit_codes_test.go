package open_source_docs_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// exitCase is one command run against the built CLI. family names the guide's
// exit-code list it belongs to ("dump", "validate", "diff"); an empty family
// means the statement sits in the section's prose outside those lists. phrase,
// when set, must appear in that list item (or in the section prose).
type exitCase struct {
	family string
	code   int
	args   []string
	phrase string
}

var (
	exitListItemRe = regexp.MustCompile(`^- (\d+) - (.*)$`)
	subHelpCodeRe  = regexp.MustCompile("(\\d+) for (`[a-z]+`(?:(?:, | and )`[a-z]+`)*)")
	backtickWordRe = regexp.MustCompile("`([a-z]+)`")
)

// exitCodeSection parses the guide's exit-codes H2: the 0/1/2 list items per
// command family, the subcommand --help codes, and the section prose with
// whitespace collapsed.
func exitCodeSection(t *testing.T) (lists map[string]map[int]string, subHelp map[string]int, prose, where string) {
	t.Helper()
	d, h, ok := guideSection(guideDocs(t), 2, func(s string) bool { return strings.Contains(s, "exit code") })
	if !ok {
		t.Fatalf("no page of the CLI guide has an exit-codes H2")
	}
	where = d.path
	sec := sectionLines(d.lines, d.headings, h)
	prose = strings.Join(strings.Fields(proseText(sec)), " ")

	lists = map[string]map[int]string{}
	family := ""
	for _, p := range paragraphs(sec) {
		switch {
		case strings.HasPrefix(p, "`dump` commands:"):
			family = "dump"
		case p == "`validate`:":
			family = "validate"
		case p == "`diff`:":
			family = "diff"
		case strings.HasPrefix(p, "- "):
			if family == "" {
				continue
			}
			m := exitListItemRe.FindStringSubmatch(p)
			if m == nil {
				continue
			}
			code, _ := strconv.Atoi(m[1])
			if lists[family] == nil {
				lists[family] = map[int]string{}
			}
			lists[family][code] = m[2]
		default:
			family = ""
		}
	}

	subHelp = map[string]int{}
	for _, m := range subHelpCodeRe.FindAllStringSubmatch(prose, -1) {
		code, _ := strconv.Atoi(m[1])
		for _, w := range backtickWordRe.FindAllStringSubmatch(m[2], -1) {
			subHelp[w[1]] = code
		}
	}
	return lists, subHelp, prose, where
}

// notEvaluatedPDF writes a PDF whose only page content stream claims
// FlateDecode but holds plain text, so the pdfa-1b output-intent rule cannot
// be evaluated and no rule reports an error.
func notEvaluatedPDF(t *testing.T) string {
	t.Helper()
	xmp := `<?xpacket begin="" id="W5M0MpCehiHzreSzNTczkc9d"?><x:xmpmeta xmlns:x="adobe:ns:meta/"></x:xmpmeta><?xpacket end="w"?>`
	content := "1 0 0 rg"
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R /Metadata 5 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>",
		fmt.Sprintf("<< /Filter /FlateDecode /Length %d >>\nstream\n%s\nendstream", len(content), content),
		fmt.Sprintf("<< /Type /Metadata /Subtype /XML /Length %d >>\nstream\n%s\nendstream", len(xmp), xmp),
	}
	body := "%PDF-1.4\n"
	xref := fmt.Sprintf("xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for i, o := range objs {
		xref += fmt.Sprintf("%010d 00000 n \n", len(body))
		body += fmt.Sprintf("%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	data := fmt.Sprintf("%s%strailer\n<< /Size %d /Root 1 0 R /ID [<01> <01>] >>\nstartxref\n%d\n%%%%EOF\n",
		body, xref, len(objs)+1, len(body))
	p := filepath.Join(t.TempDir(), "not-evaluated.pdf")
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestCLIUsageGuideExitCodesMatchBinary runs one command per exit code the
// guide's exit-codes section states and compares the binary's code with a
// table kept here. Each table row's code must be listed under its family in
// the guide (with the row's phrase in that item), every code the guide lists
// must have a row, and the subcommand --help codes the guide states must match
// the binary.
func TestCLIUsageGuideExitCodesMatchBinary(t *testing.T) {
	minimal := fixturePath(t, "minimal.pdf")
	missing := filepath.Join(t.TempDir(), "missing.pdf")
	cases := []exitCase{
		{"dump", 0, []string{"dump", "xref", minimal}, "the command ran"},
		{"dump", 1, []string{"dump", "xref"}, "wrong number of files"},
		{"dump", 1, []string{"dump", "xref", minimal, minimal}, "wrong number of files"},
		{"dump", 1, []string{"dump", "xref", "--bogus", minimal}, "an unknown flag"},
		{"dump", 1, []string{"dump", "xref", minimal, "--json"}, "a flag after the file"},
		{"dump", 1, []string{"dump", "nosuch", minimal}, "an unknown resource"},
		{"dump", 1, []string{"dump", "tree", "--page", "0", minimal}, "`--page 0`"},
		{"dump", 1, []string{"dump", "tree", "--depth", "x", minimal}, "`--depth x`"},
		{"dump", 1, []string{"dump", "object", "--ref", "zz", minimal}, "a malformed `--ref`"},
		{"dump", 1, []string{"dump", "stream", "--raw", "--json", "--page", "1", fixturePath(t, "content-stream.pdf")}, "`--raw --json`"},
		{"dump", 1, []string{"dump", "embedded", "--ref", "1 0 R", "--name", "x", minimal}, "`dump embedded --ref` with `--name`"},
		{"dump", 2, []string{"dump", "xref", missing}, "the file is missing or unreadable"},
		{"dump", 2, []string{"dump", "xref", fixturePath(t, "malformed.pdf")}, "the file is missing or unreadable"},
		{"dump", 2, []string{"dump", "object", "--ref", "999 0 R", minimal}, "the page or object asked for is not in it"},
		{"dump", 2, []string{"dump", "page", "--info", "99", minimal}, "the page or object asked for is not in it"},
		{"dump", 2, []string{"dump", "embedded", "--name", "nope", minimal}, "`dump embedded --name` matches no attachment"},

		{"validate", 0, []string{"validate", fixturePath(t, "pdfa-1b-clean.pdf")}, "found no errors"},
		{"validate", 1, []string{"validate", fixturePath(t, "untagged.pdf")}, "at least one error"},
		{"validate", 1, []string{"validate", notEvaluatedPDF(t)}, "a rule could not be evaluated"},
		{"validate", 1, []string{"validate", fixturePath(t, "encrypted.pdf")}, "for an encrypted file"},
		{"validate", 2, []string{"validate", "--profile", "pdfua-1-structural", fixturePath(t, "encrypted.pdf")}, "an encrypted file under `pdfua-1-structural`"},
		{"validate", 2, []string{"validate", missing}, "a missing file"},
		{"validate", 2, []string{"validate", "--profile", "nope", minimal}, "an unknown profile"},
		{"validate", 2, []string{"validate", "--bogus", minimal}, "an unknown flag"},
		{"validate", 2, []string{"validate"}, "the wrong number of files"},
		{"validate", 2, []string{"validate", minimal, minimal}, "the wrong number of files"},
		{"validate", 2, []string{"validate", minimal, "--json"}, "a flag after the file"},

		{"diff", 0, []string{"diff", minimal, minimal}, "structurally identical"},
		{"diff", 1, []string{"diff", minimal, fixturePath(t, "multipage.pdf")}, "they differ"},
		{"diff", 2, []string{"diff", minimal, missing}, "operational or usage error"},
		{"diff", 2, []string{"diff", minimal}, "operational or usage error"},
		{"diff", 2, []string{"diff", "--bogus", minimal, minimal}, "operational or usage error"},

		{"", 0, []string{"validate", "--profile", "pdfua-1-structural", fixturePath(t, "untagged.pdf")}, "so that profile exits 0 no matter what it finds"},
		{"", 0, []string{"--help"}, "`pdfdebug --help` exits 0"},
		{"", 1, []string{}, "`pdfdebug` with no arguments or an unknown command prints the help and exits 1"},
		{"", 1, []string{"nosuch"}, "`pdfdebug` with no arguments or an unknown command prints the help and exits 1"},
		{"", 1, []string{"dump"}, "`dump` with no resource prints its one-line usage and also exits 1"},
	}
	subHelpCases := map[string][]string{
		"dump":     {"dump", "xref", "--help"},
		"validate": {"validate", "--help"},
		"diff":     {"diff", "--help"},
	}

	lists, subHelp, prose, where := exitCodeSection(t)
	for _, f := range []string{"dump", "validate", "diff"} {
		if len(lists[f]) == 0 {
			t.Fatalf("%s exit-codes section has no `- N - ...` list for %s", where, f)
		}
	}

	covered := map[string]map[int]bool{}
	for _, c := range cases {
		name := strings.TrimSpace("pdfdebug " + strings.Join(c.args, " "))
		if _, stderr, got := runCLI(t, c.args...); got != c.code {
			t.Errorf("%s exited %d, want %d; stderr:\n%s", name, got, c.code, stderr)
		}
		if c.family == "" {
			if !strings.Contains(prose, c.phrase) {
				t.Errorf("%s exit-codes section does not say %q (checked by running %s)", where, c.phrase, name)
			}
			continue
		}
		item, ok := lists[c.family][c.code]
		if !ok {
			t.Errorf("%s does not list exit %d for %s (checked by running %s)", where, c.code, c.family, name)
			continue
		}
		if !strings.Contains(item, c.phrase) {
			t.Errorf("%s exit %d item for %s does not mention %q (checked by running %s); item:\n%s",
				where, c.code, c.family, c.phrase, name, item)
		}
		if covered[c.family] == nil {
			covered[c.family] = map[int]bool{}
		}
		covered[c.family][c.code] = true
	}
	for f, items := range lists {
		var codes []int
		for code := range items {
			codes = append(codes, code)
		}
		sort.Ints(codes)
		for _, code := range codes {
			if !covered[f][code] {
				t.Errorf("%s lists exit %d for %s but no command in this test checks it", where, code, f)
			}
		}
	}

	for _, sub := range []string{"dump", "validate", "diff"} {
		want, ok := subHelp[sub]
		if !ok {
			t.Errorf("%s does not state the `--help` exit code for `%s`", where, sub)
			continue
		}
		args := subHelpCases[sub]
		if _, _, got := runCLI(t, args...); got != want {
			t.Errorf("pdfdebug %s exited %d, but %s says %d", strings.Join(args, " "), got, where, want)
		}
	}
}
