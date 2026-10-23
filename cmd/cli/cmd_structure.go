package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// runStructureDump parses flags and dispatches the tagged-structure dump.
func runStructureDump(args []string) int {
	filePath, f, ok := parseDocViewFlags("structure", args)
	if !ok {
		return 1
	}
	return execStructureDump(filePath, f)
}

// execStructureDump opens the PDF and renders its structure report.
func execStructureDump(filePath string, f docViewFlags) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("internal error: %v", r))
			exitCode = 2
		}
	}()

	ins, _, code := openForCLI(filePath)
	if code != 0 {
		return code
	}
	defer func() { _ = ins.Close("cli") }()

	report, err := ins.GetStructure("cli")
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}
	return writeStructureDump(os.Stdout, os.Stderr, report, f)
}

// writeStructureDump writes the report to stdout: the JSON object with
// --json, the plain-text view otherwise. Findings never change the exit code;
// a failed write exits 2 with the error object on stderr.
func writeStructureDump(stdout, stderr io.Writer, r *pdfcore.StructureReport, f docViewFlags) int {
	var err error
	if f.json {
		err = emit(stdout, r, f.pretty)
	} else {
		_, err = io.WriteString(stdout, structurePlain(r))
	}
	if err != nil {
		writeJSONError(stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}

// structurePlain renders the report as plain text. NON-CONTRACTUAL; use
// --json to parse.
func structurePlain(r *pdfcore.StructureReport) string {
	var b strings.Builder
	if !r.HasStructTree {
		b.WriteString("No structure tree (/StructTreeRoot absent)\n")
	} else {
		writeStructureHeader(&b, r)
	}
	if r.HasStructTree {
		c := r.Counts
		fmt.Fprintf(&b, "\nElements (%d; kids: %d MCID, %d MCR, %d OBJR, %d error):\n",
			c.Elements, c.MCIDKids, c.MCRKids, c.OBJRKids, c.ErrorKids)
		writeStructKids(&b, r.Elements, 0)
	}
	b.WriteString("\n")
	writeStructPages(&b, r.Pages)
	if len(r.OtherParentTree) > 0 {
		b.WriteString("\nOther ParentTree keys:\n")
		for _, e := range r.OtherParentTree {
			line := fmt.Sprintf("  %d  ->  %s", e.Key, joinNonEmpty(asciiSafe(e.ElementRef), asciiSafe(e.S)))
			if e.Note != "" {
				line += "  note: " + asciiSafe(e.Note)
			}
			if e.Err != "" {
				line += "  error: " + asciiSafe(e.Err)
			}
			b.WriteString(line + "\n")
		}
	}
	if len(r.Problems) > 0 {
		fmt.Fprintf(&b, "\nProblems (%d):\n", len(r.Problems))
		for _, p := range r.Problems {
			b.WriteString("  " + asciiSafe(p.Where) + ": " + asciiSafe(p.Message) + "\n")
		}
	}
	return b.String()
}

// writeStructureHeader writes the document-level lines.
func writeStructureHeader(b *strings.Builder, r *pdfcore.StructureReport) {
	line := "Structure tree: " + orText(r.RootRef, "(direct)")
	if r.ParentTreeNextKey != nil {
		line += fmt.Sprintf("  (ParentTreeNextKey %d)", *r.ParentTreeNextKey)
	}
	b.WriteString(line + "\n")
	if r.HasParentTree {
		b.WriteString("ParentTree: " + orText(r.ParentTreeRef, "(direct)") + "\n")
	} else {
		b.WriteString("ParentTree: absent\n")
	}
	marked := "absent"
	if r.Marked != nil {
		marked = strconv.FormatBool(*r.Marked)
	}
	b.WriteString("MarkInfo /Marked: " + marked + "\n")
	b.WriteString("Document /Lang: " + asciiSafe(orText(r.DocLang, "absent")) + "\n")
	if len(r.RoleMap) > 0 {
		pairs := make([]string, len(r.RoleMap))
		for i, e := range r.RoleMap {
			pairs[i] = asciiSafe(e.From) + " -> " + asciiSafe(orText(e.To, "?"))
		}
		b.WriteString("RoleMap: " + strings.Join(pairs, ", ") + "\n")
	}
}

// writeStructKids writes kids, two spaces of indent per level.
func writeStructKids(b *strings.Builder, kids []*pdfcore.StructKid, level int) {
	indent := strings.Repeat("  ", level)
	for _, k := range kids {
		var line string
		switch k.Kind {
		case "element":
			line = elementLine(k.Element)
		case "mcid":
			line = fmt.Sprintf("MCID %d", *k.MCID)
		case "mcr":
			line = "MCR"
			if k.MCID != nil {
				line += fmt.Sprintf(" MCID %d", *k.MCID)
			}
			if k.Page > 0 {
				line += fmt.Sprintf(" page %d", k.Page)
			}
			if k.StmRef != "" {
				line += " stream " + asciiSafe(k.StmRef)
			}
		case "objr":
			line = "OBJR " + joinNonEmpty(asciiSafe(orText(k.ObjRef, "-")), asciiSafe(k.ObjType))
		default:
			line = "error: " + asciiSafe(k.Err)
		}
		if k.Kind != "error" && k.Kind != "element" && k.Err != "" {
			line += "  error: " + asciiSafe(k.Err)
		}
		b.WriteString(indent + line + "\n")
		if k.Element != nil {
			writeStructKids(b, k.Element.Kids, level+1)
		}
	}
}

// elementLine renders one element: S, mapped S, ref, page and text fields.
// Each value is made ASCII-safe on its own, so the line is never escaped
// again as a whole.
func elementLine(e *pdfcore.StructElem) string {
	parts := []string{asciiSafe(orText(e.S, "(no /S)"))}
	if e.MappedS != "" {
		parts[0] += " (" + asciiSafe(e.MappedS) + ")"
	}
	parts[0] += " " + asciiSafe(orText(e.Ref, "(direct)"))
	if e.Page > 0 {
		p := fmt.Sprintf("page %d", e.Page)
		if e.PageInherited {
			p += " (inherited)"
		}
		parts = append(parts, p)
	} else if e.PgRef != "" {
		parts = append(parts, "/Pg "+asciiSafe(e.PgRef))
	}
	text := func(label, v string) {
		c := pdfcore.ClampDisplayValue(v, pdfcore.TreeValueCap)
		if s := asciiSafe(c); s != c {
			parts = append(parts, label+" "+s)
		} else {
			parts = append(parts, label+" \""+c+"\"")
		}
	}
	if e.Title != "" {
		text("T", e.Title)
	}
	if e.Lang != "" {
		text("Lang", e.Lang)
	}
	for _, f := range []struct {
		label string
		v     *string
	}{{"Alt", e.Alt}, {"ActualText", e.ActualText}, {"E", e.E}} {
		if f.v != nil {
			text(f.label, *f.v)
		}
	}
	if e.Err != "" {
		parts = append(parts, "error: "+asciiSafe(e.Err))
	}
	return strings.Join(parts, "  ")
}

// pageIsEmpty reports whether a page has nothing to show: no /StructParents,
// no marked content, no alignment row (a tree element may claim an MCID on a
// page with neither) and no error.
func pageIsEmpty(p *pdfcore.StructPage) bool {
	if p.StructParents != nil || p.ParentTreeErr != "" || p.Err != "" || len(p.ContentMCIDs) > 0 || len(p.Alignment) > 0 {
		return false
	}
	for _, s := range p.Streams {
		if s.Opens > 0 || s.Closes > 0 || s.Err != "" {
			return false
		}
	}
	return true
}

// writeStructPages writes one block per page with structure content and
// collapses the rest into one range list.
func writeStructPages(b *strings.Builder, pages []*pdfcore.StructPage) {
	var empty []int
	legend := false
	for _, p := range pages {
		if pageIsEmpty(p) {
			empty = append(empty, p.PageNum)
			continue
		}
		opens, closes := 0, 0
		for _, s := range p.Streams {
			opens += s.Opens
			closes += s.Closes
		}
		sp := "absent"
		if p.StructParents != nil {
			sp = strconv.Itoa(*p.StructParents)
		}
		streams := fmt.Sprintf("%d streams", len(p.Streams))
		if len(p.Streams) == 1 {
			streams = "1 stream"
		}
		fmt.Fprintf(b, "Page %d  /StructParents %s  %s  BDC/BMC %d, EMC %d\n", p.PageNum, sp, streams, opens, closes)
		if p.Err != "" {
			b.WriteString("  error: " + asciiSafe(p.Err) + "\n")
		}
		if p.ParentTreeErr != "" {
			b.WriteString("  ParentTree error: " + asciiSafe(p.ParentTreeErr) + "\n")
		}
		for _, s := range p.ParentTree {
			if s.Err != "" {
				fmt.Fprintf(b, "  ParentTree slot %d: error: %s\n", s.MCID, asciiSafe(s.Err))
			}
		}
		for i, s := range p.Streams {
			// Depth carries across a page's streams, so only the last stream's
			// DepthAtEnd shows a sequence left open.
			unclosed := i == len(p.Streams)-1 && s.DepthAtEnd != 0
			switch {
			case s.Err != "":
				b.WriteString("  stream " + s.Ref + ": error: " + asciiSafe(s.Err) + "\n")
			case s.MinDepth < 0 || unclosed:
				fmt.Fprintf(b, "  stream %s: BDC/BMC %d, EMC %d, depth at end %d, min depth %d\n", s.Ref, s.Opens, s.Closes, s.DepthAtEnd, s.MinDepth)
			}
		}
		for _, c := range p.ContentMCIDs {
			if c.Err != "" {
				ref := strconv.Itoa(c.Stream)
				if c.Stream < len(p.Streams) {
					ref = p.Streams[c.Stream].Ref
				}
				fmt.Fprintf(b, "  stream %s line %d col %d: %s %s\n", ref, c.Line, c.Col, asciiSafe(orText(c.Tag, "?")), asciiSafe(c.Err))
			}
		}
		if len(p.Alignment) == 0 {
			continue
		}
		t := newTable("MCID", "CONTENT", "PARENTTREE", "TREE", "")
		starred := false
		for _, a := range p.Alignment {
			mark := ""
			if !a.Consistent {
				mark, starred = "*", true
			}
			// Cells are escaped before rendering so column widths count the
			// escaped text.
			t.AddRow(strconv.Itoa(a.MCID), strconv.Itoa(a.ContentCount),
				orText(joinNonEmpty(asciiSafe(a.ParentTreeElement), asciiSafe(a.ParentTreeS)), "-"),
				asciiSafe(orText(strings.Join(a.TreeElements, ", "), "-")), mark)
		}
		var tb strings.Builder
		_ = t.Render(&tb)
		for line := range strings.SplitSeq(strings.TrimSuffix(tb.String(), "\n"), "\n") {
			b.WriteString("  " + strings.TrimRight(line, " ") + "\n")
		}
		if starred && !legend {
			b.WriteString("  * = content count, ParentTree slot and tree claimants do not line up on one element\n")
			legend = true
		}
	}
	if len(empty) > 0 {
		b.WriteString("Pages with no /StructParents and no marked content: " + pageRanges(empty) + "\n")
	}
}

// pageRanges renders ascending page numbers as "1-3, 5".
func pageRanges(nums []int) string {
	var parts []string
	for i := 0; i < len(nums); {
		j := i
		for j+1 < len(nums) && nums[j+1] == nums[j]+1 {
			j++
		}
		if i == j {
			parts = append(parts, strconv.Itoa(nums[i]))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", nums[i], nums[j]))
		}
		i = j + 1
	}
	return strings.Join(parts, ", ")
}

// orText returns s, or alt when s is empty.
func orText(s, alt string) string {
	if s == "" {
		return alt
	}
	return s
}

// joinNonEmpty joins the non-empty values with a space.
func joinNonEmpty(vals ...string) string {
	var out []string
	for _, v := range vals {
		if v != "" {
			out = append(out, v)
		}
	}
	return strings.Join(out, " ")
}
