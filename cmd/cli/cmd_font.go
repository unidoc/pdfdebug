package main

import (
	"fmt"
	"io"
	"os"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// runFontDump parses flags and dispatches font-view dump execution.
func runFontDump(args []string) int {
	filePath, f, ok := parseByRefFlags("font", args, false, false)
	if !ok {
		return 1
	}
	return execFontDump(filePath, f)
}

// execFontDump opens the PDF and emits the FontView payload as JSON. A non-font
// node yields {"kind":"neither",...} at exit 0 (the method does not error on
// type mismatch -- thin-presenter rule).
func execFontDump(filePath string, f byRefFlags) (exitCode int) {
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
	if err := printFontPlain(os.Stdout, view); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}

// printFontPlain renders a FontView as a human-readable single record: the
// FontDetail fields for a /Type /Font dict (kind "detail"), the roster table
// for a /Resources /Font map (kind "roster"), or a one-line note for "neither".
// NON-CONTRACTUAL; use --json to parse.
func printFontPlain(out io.Writer, v *pdfcore.FontView) error {
	switch v.Kind {
	case "detail":
		return printFontDetailPlain(out, v.Detail)
	case "roster":
		return printFontRosterPlain(out, v.Roster)
	default:
		_, err := io.WriteString(out, "Not a font: this object is neither a /Type /Font dict nor a /Resources /Font roster.\n")
		return err
	}
}

// printFontDetailPlain renders a single FontDetail as an aligned key/value
// block, with the descendant CIDFont (composite Type0 fonts) appended.
func printFontDetailPlain(out io.Writer, d *pdfcore.FontDetail) error {
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
	return w.Render(out)
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
