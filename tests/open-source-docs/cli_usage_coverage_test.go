package open_source_docs_test

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var dumpRowRe = regexp.MustCompile("^`(dump [a-z]+)`$")

// TestCLIUsageGuideCoversEveryHelpCommand asserts every command in the
// `pdfdebug --help` Commands block has a table row on the entry page whose
// first cell is the backticked command, and a `###` heading on some guide page
// whose text is the backticked command.
// The row half repeats cmd/cli TestUsageCommandsMatchDocTable from outside the
// package so this failure message is complete on its own; the heading half is
// checked nowhere else.
func TestCLIUsageGuideCoversEveryHelpCommand(t *testing.T) {
	cmds := helpCommands(t)
	docs := guideDocs(t)
	lines := mdLines(readFileAtRoot(t, cliGuidePath))

	rowCells := map[string]bool{}
	for _, r := range mdTableRows(lines) {
		rowCells[r.firstCell] = true
	}

	var noRow, noHeading []string
	for _, c := range cmds {
		if !rowCells["`"+c+"`"] {
			noRow = append(noRow, c)
		}
		if _, _, ok := guideCommandHeading(docs, c); !ok {
			noHeading = append(noHeading, c)
		}
	}
	if len(noRow) > 0 || len(noHeading) > 0 {
		t.Errorf("CLI guide does not cover every command in pdfdebug --help:\n"+
			"  no %s table row with the backticked command as first cell: %v\n"+
			"  no ### heading in %s or %s/*.md that is exactly the backticked command: %v",
			cliGuidePath, noRow, cliGuidePath, cliPagesDir, noHeading)
	}
}

// TestCLIUsageGuideCommandsAreDispatched asserts every `dump <x>` row in the
// guide's tables names a resource the binary dispatches: `pdfdebug dump <x>`
// with no file exits 1 and its first stderr line is that resource's usage line.
func TestCLIUsageGuideCommandsAreDispatched(t *testing.T) {
	lines := mdLines(readFileAtRoot(t, cliGuidePath))

	seen := map[string]bool{}
	var documented []string
	for _, r := range mdTableRows(lines) {
		m := dumpRowRe.FindStringSubmatch(r.firstCell)
		if m == nil || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		documented = append(documented, m[1])
	}
	if len(documented) == 0 {
		t.Fatalf("%s has no table row whose first cell is a backticked `dump <resource>` command", cliGuidePath)
	}

	var bad []string
	for _, c := range documented {
		resource := strings.TrimPrefix(c, "dump ")
		_, stderr, code := runCLI(t, "dump", resource)
		first, _, _ := strings.Cut(stderr, "\n")
		if code != 1 || !strings.HasPrefix(first, "Usage: pdfdebug "+c) {
			bad = append(bad, fmt.Sprintf("%s (exit %d, first stderr line %q)", c, code, first))
		}
	}
	if len(bad) > 0 {
		t.Errorf("%s documents dump resources the binary does not dispatch:\n  %s",
			cliGuidePath, strings.Join(bad, "\n  "))
	}
}

// TestCLIUsageGuideSynopsesMatchUsageLines asserts the first line under each
// command's `###` reference heading is that command's usage line, as the
// binary prints it with no arguments, in a code span.
func TestCLIUsageGuideSynopsesMatchUsageLines(t *testing.T) {
	cmds := helpCommands(t)
	docs := guideDocs(t)

	var bad []string
	for _, c := range cmds {
		d, h, ok := guideCommandHeading(docs, c)
		if !ok {
			continue // reported by TestCLIUsageGuideCoversEveryHelpCommand
		}
		_, stderr, _ := runCLI(t, strings.Fields(c)...)
		usage, _, _ := strings.Cut(stderr, "\n")
		if !strings.HasPrefix(usage, "Usage: pdfdebug "+c) {
			bad = append(bad, fmt.Sprintf("%s: binary printed no usage line, first stderr line %q", c, usage))
			continue
		}
		want := "`" + strings.TrimPrefix(usage, "Usage: ") + "`"
		got := ""
		for _, l := range sectionLines(d.lines, d.headings, h) {
			if s := strings.TrimSpace(l.text); s != "" {
				got = s
				break
			}
		}
		if got != want {
			bad = append(bad, fmt.Sprintf("%s (%s line %d):\n    guide:  %s\n    binary: %s", c, d.path, h.index+1, got, want))
		}
	}
	if len(bad) > 0 {
		t.Errorf("CLI guide synopses differ from the binary's usage lines:\n  %s", strings.Join(bad, "\n  "))
	}
}

// TestCLIUsageGuideQuickReferenceListsEachCommandOnce asserts one table on the
// entry page lists every help command exactly once and nothing else in its
// first column, and that no guide page carries a table row for the deprecated
// `dump plaintext` alias.
func TestCLIUsageGuideQuickReferenceListsEachCommandOnce(t *testing.T) {
	cmds := helpCommands(t)
	rows := mdTableRows(mdLines(readFileAtRoot(t, cliGuidePath)))

	for _, d := range guideDocs(t) {
		for _, r := range mdTableRows(d.lines) {
			if r.firstCell == "`dump plaintext`" {
				t.Errorf("%s line %d: table row for the deprecated `dump plaintext` alias; it belongs in the `dump bytes` reference prose",
					d.path, r.index+1)
			}
		}
	}

	if _, ok := quickReferenceTable(rows, cmds); !ok {
		t.Errorf("%s has no table whose first column lists exactly the %d help commands, each once: %v",
			cliGuidePath, len(cmds), cmds)
	}
}

// TestCLIUsageGuideReferenceFollowsQuickReferenceOrder asserts each command's
// `###` reference heading appears once across the guide, none sits above the
// quick-reference table on the entry page, and the headings on each page
// follow the table's row order.
func TestCLIUsageGuideReferenceFollowsQuickReferenceOrder(t *testing.T) {
	cmds := helpCommands(t)
	lines := mdLines(readFileAtRoot(t, cliGuidePath))

	table, ok := quickReferenceTable(mdTableRows(lines), cmds)
	if !ok {
		t.Fatalf("%s has no quick-reference table listing every help command once", cliGuidePath)
	}
	tableEnd := table[len(table)-1].index

	var tableOrder []string
	for _, r := range table {
		tableOrder = append(tableOrder, strings.Trim(r.firstCell, "`"))
	}

	where := map[string][]string{}
	for _, d := range guideDocs(t) {
		var pageOrder []string
		for _, h := range d.headings {
			if h.level != 3 || !strings.HasPrefix(h.text, "`") || !strings.HasSuffix(h.text, "`") {
				continue
			}
			name := strings.Trim(h.text, "`")
			if !slices.Contains(tableOrder, name) {
				continue
			}
			where[name] = append(where[name], fmt.Sprintf("%s:%d", d.path, h.index+1))
			pageOrder = append(pageOrder, name)
			if d.path == cliGuidePath && h.index < tableEnd {
				t.Errorf("%s line %d: reference heading %s sits above the quick-reference table", d.path, h.index+1, h.text)
			}
		}
		var want []string
		for _, c := range tableOrder {
			if slices.Contains(pageOrder, c) {
				want = append(want, c)
			}
		}
		if strings.Join(pageOrder, ",") != strings.Join(want, ",") {
			t.Errorf("%s reference headings are not in quick-reference order:\n  table order: %v\n  page order:  %v",
				d.path, want, pageOrder)
		}
	}
	for name, at := range where {
		if len(at) > 1 {
			t.Errorf("reference heading `%s` appears %d times, want once: %v", name, len(at), at)
		}
	}
}

var pageLinkRe = regexp.MustCompile(`\]\((cli/[^)#\s]+\.md)#([^)\s]+)\)`)

// TestCLIUsageGuideQuickReferenceLinksCommandPages asserts every quick-reference
// row links, outside its first cell, to a docs/cli page and anchor, and that
// the page has the command's `###` heading under that anchor.
func TestCLIUsageGuideQuickReferenceLinksCommandPages(t *testing.T) {
	cmds := helpCommands(t)
	table, ok := quickReferenceTable(mdTableRows(mdLines(readFileAtRoot(t, cliGuidePath))), cmds)
	if !ok {
		t.Fatalf("%s has no quick-reference table listing every help command once", cliGuidePath)
	}
	pages := map[string]guideDoc{}
	for _, d := range guideDocs(t) {
		pages[d.path] = d
	}

	for _, r := range table {
		cmd := strings.Trim(r.firstCell, "`")
		m := pageLinkRe.FindStringSubmatch(r.text)
		if m == nil {
			t.Errorf("%s line %d: row for `%s` links to no %s/<page>.md#<anchor>", cliGuidePath, r.index+1, cmd, cliPagesDir)
			continue
		}
		target := "docs/" + m[1]
		d, ok := pages[target]
		if !ok {
			t.Errorf("%s line %d: row for `%s` links %s, which does not exist", cliGuidePath, r.index+1, cmd, target)
			continue
		}
		h, ok := commandHeading(d.headings, cmd)
		if !ok {
			t.Errorf("%s line %d: row for `%s` links %s, which has no ### `%s` heading", cliGuidePath, r.index+1, cmd, target, cmd)
			continue
		}
		if got := githubSlug(h.text); got != m[2] {
			t.Errorf("%s line %d: row for `%s` links %s#%s, but the heading's anchor is #%s", cliGuidePath, r.index+1, cmd, target, m[2], got)
		}
	}
}

// TestCLIUsageGuideLinksEveryReferencePage asserts the entry page links every
// docs/cli/*.md page, so none is reachable only by browsing the directory.
func TestCLIUsageGuideLinksEveryReferencePage(t *testing.T) {
	docs := guideDocs(t)
	if len(docs) < 2 {
		t.Fatalf("no reference pages under %s", cliPagesDir)
	}
	linked := map[string]bool{}
	for _, l := range guideLinks(docs[0]) {
		if p, _, ok := resolveGuideLink(docs[0].path, l); ok {
			linked[p] = true
		}
	}
	for _, d := range docs[1:] {
		if !linked[d.path] {
			t.Errorf("%s does not link %s", cliGuidePath, d.path)
		}
	}
}

// TestCLIUsageGuideRelativeLinksResolve asserts every relative link on every
// guide page names a file that exists and, when it carries an anchor, a
// heading on that page with that anchor.
func TestCLIUsageGuideRelativeLinksResolve(t *testing.T) {
	root := projectRoot(t)
	docs := guideDocs(t)
	anchors := map[string]map[string]bool{}
	for _, d := range docs {
		a := map[string]bool{}
		for _, h := range d.headings {
			a[githubSlug(h.text)] = true
		}
		anchors[d.path] = a
	}

	for _, d := range docs {
		for _, l := range guideLinks(d) {
			target, anchor, ok := resolveGuideLink(d.path, l)
			if !ok {
				continue
			}
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(target))); err != nil {
				t.Errorf("%s links %s: %s does not exist", d.path, l, target)
				continue
			}
			if anchor == "" {
				continue
			}
			a, ok := anchors[target]
			if !ok {
				t.Errorf("%s links %s: anchors are only checked on guide pages, and %s is not one", d.path, l, target)
				continue
			}
			if !a[anchor] {
				t.Errorf("%s links %s: %s has no heading with anchor #%s", d.path, l, target, anchor)
			}
		}
	}
}

var (
	codeSpanRe  = regexp.MustCompile("`[^`]*`")
	mdAnyLinkRe = regexp.MustCompile(`\]\(([^)\s]+)\)`)
)

// guideLinks returns the target of every inline link in a page's prose and
// tables, with code spans removed so bracketed code is not read as a link.
func guideLinks(d guideDoc) []string {
	var out []string
	for _, m := range mdAnyLinkRe.FindAllStringSubmatch(codeSpanRe.ReplaceAllString(proseText(d.lines), ""), -1) {
		out = append(out, m[1])
	}
	return out
}

// resolveGuideLink resolves a relative link from page to a repo-relative path
// and anchor; ok is false for absolute URLs.
func resolveGuideLink(page, link string) (target, anchor string, ok bool) {
	if strings.Contains(link, "://") || strings.HasPrefix(link, "mailto:") {
		return "", "", false
	}
	p, anchor, _ := strings.Cut(link, "#")
	if p == "" {
		return page, anchor, true
	}
	return path.Clean(path.Join(path.Dir(page), p)), anchor, true
}

// quickReferenceTable returns the rows of the first table whose first column
// is exactly the help command set with no duplicates.
func quickReferenceTable(rows []mdTableRow, cmds []string) ([]mdTableRow, bool) {
	byTable := map[int][]mdTableRow{}
	var order []int
	for _, r := range rows {
		if _, ok := byTable[r.table]; !ok {
			order = append(order, r.table)
		}
		byTable[r.table] = append(byTable[r.table], r)
	}
	for _, tbl := range order {
		tr := byTable[tbl]
		if len(tr) != len(cmds) {
			continue
		}
		seen := map[string]bool{}
		ok := true
		for _, r := range tr {
			name := strings.Trim(r.firstCell, "`")
			if seen[name] || !slices.Contains(cmds, name) || r.firstCell != "`"+name+"`" {
				ok = false
				break
			}
			seen[name] = true
		}
		if ok {
			return tr, true
		}
	}
	return nil, false
}
