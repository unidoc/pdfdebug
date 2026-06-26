package main

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// runXRefDump parses flags and dispatches the document-level xref-table dump.
func runXRefDump(args []string) int {
	filePath, f, ok := parseDocViewFlags("xref", args)
	if !ok {
		return 1
	}
	return execXRefDump(filePath, f)
}

// execXRefDump opens the PDF and emits the XRefTable payload as JSON.
func execXRefDump(filePath string, f docViewFlags) (exitCode int) {
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

	table, err := ins.GetXRefTable("cli")
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}
	if f.json {
		if err := emit(os.Stdout, table, f.pretty); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		return 0
	}
	if err := printXRefPlain(os.Stdout, table); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}

// printXRefPlain renders the cross-reference table as an aligned table: one row
// per entry (object, generation, entry type/status, on-disk offset or host
// object-stream). The TYPE column carries the xref status (in-use/free/
// in-objstm). Offset is rendered as "-" for non-in-use entries; the host /ObjStm
// number is shown for in-objstm entries. NON-CONTRACTUAL; use --json to parse.
func printXRefPlain(out io.Writer, table *pdfcore.XRefTable) error {
	t := newTable("OBJ", "GEN", "TYPE", "OFFSET", "OBJSTM")
	for _, e := range table.Entries {
		offset := "-"
		if e.Status == "in-use" {
			offset = strconv.FormatInt(e.Offset, 10)
		}
		objstm := "-"
		if e.Status == "in-objstm" {
			objstm = strconv.Itoa(e.HostObjStm)
		}
		t.AddRow(strconv.Itoa(e.ObjNum), strconv.Itoa(e.Gen), e.Status, offset, objstm)
	}
	return t.Render(out)
}
