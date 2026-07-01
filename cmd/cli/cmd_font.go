package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// fontFlags extends byRefFlags with the font-only --glyphs verbosity toggle
// (Story 13.3 AC3): without it the plain output is a bounded summary, with it
// the full per-code mapping table. --glyphs has no effect on --json (the JSON
// surface is always complete).
type fontFlags struct {
	byRefFlags
	glyphs bool
}

// runFontDump parses flags and dispatches font-view dump execution. Unlike the
// shared parseByRefFlags path, `dump font` accepts the font-only --glyphs flag,
// so it parses its own FlagSet and advertises --glyphs in the usage line.
func runFontDump(args []string) int {
	usage := `Usage: pdfdebug dump font [--glyphs] [--json] [--pretty] --ref "N G R" <file>

  --glyphs   Print the full per-code mapping table (CODE GLYPH UNICODE TEXT).
             Without it, plain output is a bounded summary (row count + health).
  --json     Output the complete mapping array + health signals as JSON
             (always complete, regardless of --glyphs).

Example: pdfdebug dump font --glyphs --ref "6 0 R" input.pdf`

	fs := flag.NewFlagSet("dump font", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	refFlag := fs.String("ref", "", `Object reference in "N G R" or obj:G:N form`)
	prettyFlag := fs.Bool("pretty", false, "Indent JSON output (no effect on plain text)")
	jsonFlag := fs.Bool("json", false, "Output structured JSON (default is human-readable plain text)")
	glyphsFlag := fs.Bool("glyphs", false, "Print the full per-code mapping table in plain output")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, usage)
		return 1
	}
	if *refFlag == "" || fs.Arg(0) == "" {
		fmt.Fprintln(os.Stderr, usage)
		return 1
	}

	f := fontFlags{
		byRefFlags: byRefFlags{ref: *refFlag, json: *jsonFlag, pretty: *prettyFlag},
		glyphs:     *glyphsFlag,
	}
	return execFontDump(fs.Arg(0), f)
}

// execFontDump opens the PDF and emits the FontView payload as JSON. A non-font
// node yields {"kind":"neither",...} at exit 0 (the method does not error on
// type mismatch -- thin-presenter rule).
func execFontDump(filePath string, f fontFlags) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("internal error: %v", r))
			exitCode = 2
		}
	}()

	ins, nodeID, _, code := openByRef(f.ref, filePath)
	if code != 0 {
		return code
	}
	defer func() { _ = ins.Close("cli") }()

	view, err := ins.GetFontView("cli", nodeID)
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}
	if f.json {
		if err := emit(os.Stdout, view, f.pretty); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		return 0
	}
	if err := printFontPlain(os.Stdout, view, f.glyphs); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}

// printFontPlain renders a FontView as a human-readable single record: the
// FontDetail fields for a /Type /Font dict (kind "detail"), the roster table
// for a /Resources /Font map (kind "roster"), or a one-line note for "neither".
// NON-CONTRACTUAL; use --json to parse.
func printFontPlain(out io.Writer, v *pdfcore.FontView, glyphs bool) error {
	switch v.Kind {
	case "detail":
		return printFontDetailPlain(out, v.Detail, glyphs)
	case "roster":
		return printFontRosterPlain(out, v.Roster)
	default:
		_, err := io.WriteString(out, "Not a font: this object is neither a /Type /Font dict nor a /Resources /Font roster.\n")
		return err
	}
}

// printFontDetailPlain renders a single FontDetail as an aligned key/value
// block, with the descendant CIDFont (composite Type0 fonts) appended. The
// mapping table follows: a bounded summary (declared-code count + health
// signals) by default, or the full per-code table when glyphs is true
// (Story 13.3 AC3). NON-CONTRACTUAL.
func printFontDetailPlain(out io.Writer, d *pdfcore.FontDetail, glyphs bool) error {
	if d == nil {
		_, err := io.WriteString(out, "Font: (no detail)\n")
		return err
	}
	var w kvWriter
	w.Add("Object", d.ObjectRef)
	w.Add("Subtype", d.Subtype)
	w.Add("BaseFont", d.BaseFont)
	if d.EncodingName != "" {
		w.Add("Encoding", d.EncodingName)
	}
	w.Add("Embedded", yesNo(d.Embedded))
	if d.FontDescriptor != nil {
		w.Add("FontDescriptor", d.FontDescriptor.ObjectRef)
		if d.FontDescriptor.FontFileFormat != "" {
			w.Addf("FontFile", "%s (%s)", d.FontDescriptor.FontFileFormat, humanizeBytes(int64(d.FontDescriptor.FontFileSize)))
		}
	}
	if d.Descendant != nil {
		w.Add("Descendant", d.Descendant.BaseFont)
	}
	if err := w.Render(out); err != nil {
		return err
	}
	if err := printFontMappingPlain(out, d, glyphs); err != nil {
		return err
	}
	return nil
}

// printFontMappingPlain emits the Story 13.3 mapping section: a bounded summary
// (declared-code count + health signals) by default, or the full aligned
// per-code table (CODE GLYPH UNICODE TEXT) when glyphs is true.
func printFontMappingPlain(out io.Writer, d *pdfcore.FontDetail, glyphs bool) error {
	if _, err := io.WriteString(out, "\n"); err != nil {
		return err
	}
	if glyphs {
		t := newTable("CODE", "GLYPH", "UNICODE", "TEXT")
		for _, r := range d.MappingRows {
			t.AddRow(r.CodeHex, dashIfEmpty(r.GlyphName), dashIfEmpty(r.Unicode), dashIfEmpty(r.UnicodeText))
		}
		return t.Render(out)
	}

	// Bounded summary: row count + the AC2 health signals.
	var w kvWriter
	w.Heading("Mapping")
	w.Addf("Declared codes", "%d", len(d.MappingRows))
	if d.Health != nil {
		w.Add("ToUnicode missing", yesNo(d.Health.ToUnicodeMissing))
		w.Add("Identity without ToUnicode", yesNo(d.Health.IdentityWithoutToUnicode))
		w.Addf("Encoding codes without ToUnicode", "%d", len(d.Health.EncodingWithoutToUnicodeCodes))
	}
	if err := w.Render(out); err != nil {
		return err
	}
	_, err := io.WriteString(out, "(use --glyphs for the full per-code table)\n")
	return err
}

// printFontRosterPlain renders a /Resources /Font roster as an aligned table.
func printFontRosterPlain(out io.Writer, r *pdfcore.FontResourceMap) error {
	if r == nil {
		_, err := io.WriteString(out, "Font roster: (empty)\n")
		return err
	}
	t := newTable("NAME", "BASEFONT", "SUBTYPE", "EMBEDDED", "OBJ")
	for _, e := range r.Entries {
		t.AddRow(e.Name, dashIfEmpty(e.BaseFont), dashIfEmpty(e.Subtype), yesNo(e.Embedded), dashIfEmpty(e.ObjectRef))
	}
	return t.Render(out)
}
