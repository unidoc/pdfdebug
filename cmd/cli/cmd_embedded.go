package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"unidoc-pdf-debugger/internal/pdfcore"
)

const embeddedUsage = `Usage: pdfdebug dump embedded [--json] [--ref "N G R" | --name <display-name>] <file>`

// runEmbeddedDump parses flags and dispatches the embedded-file list/extract
// command. Without --ref/--name it lists every embedded/associated file (plain
// table or --json array). With --ref or --name it extracts ONE file's raw
// decoded bytes to stdout (a payload, like `dump stream --raw`; --json does not
// wrap it). --ref and --name are mutually exclusive.
func runEmbeddedDump(args []string) int {
	fs := flag.NewFlagSet("dump embedded", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	jsonFlag := fs.Bool("json", false, "Output structured JSON (default is human-readable plain text)")
	refFlag := fs.String("ref", "", `Extract the /EmbeddedFile at this ref ("N G R" or obj:G:N form)`)
	nameFlag := fs.String("name", "", "Extract the embedded file with this display name (errors on zero/multiple matches)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, embeddedUsage)
		return 1
	}

	// --ref and --name select mutually-exclusive extraction targets.
	if *refFlag != "" && *nameFlag != "" {
		fmt.Fprintln(os.Stderr, "error: --ref and --name are mutually exclusive")
		fmt.Fprintln(os.Stderr, embeddedUsage)
		return 1
	}

	filePath := fs.Arg(0)
	if filePath == "" {
		fmt.Fprintln(os.Stderr, embeddedUsage)
		return 1
	}

	if *refFlag != "" {
		return execEmbeddedExtractByRef(filePath, *refFlag)
	}
	if *nameFlag != "" {
		return execEmbeddedExtractByName(filePath, *nameFlag)
	}
	return execEmbeddedList(filePath, *jsonFlag)
}

// execEmbeddedList opens the PDF and writes the embedded-file enumeration as a
// plain-text table (default) or a JSON array (jsonOut).
func execEmbeddedList(filePath string, jsonOut bool) (exitCode int) {
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

	list, err := ins.GetEmbeddedFiles("cli")
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}

	if jsonOut {
		// Emit the slice directly so the top-level shape is a JSON array (AC4).
		if err := emit(os.Stdout, list.Files, false); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		return 0
	}
	if err := printEmbeddedListPlain(os.Stdout, list.Files); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}

// anyNameDiffersFromDisplay reports whether the plain table's Name cell can
// differ from the --name selector value, so a name copied out of the table may
// not match. Causes: the asciiSafe escape, dashIfEmpty rendering an empty name
// as "-", and leading/trailing spaces hidden by column padding.
func anyNameDiffersFromDisplay(files []pdfcore.EmbeddedFile) bool {
	for _, f := range files {
		if f.Name == "" || asciiSafe(f.Name) != f.Name || strings.Trim(f.Name, " ") != f.Name {
			return true
		}
	}
	return false
}

// printEmbeddedListPlain renders the embedded-file list as an aligned table:
// Name, Relationship, MIME, Size, Filespec, EmbeddedFile. NON-CONTRACTUAL; use
// --json to parse.
func printEmbeddedListPlain(out io.Writer, files []pdfcore.EmbeddedFile) error {
	t := newTable("Name", "Relationship", "MIME", "Size", "Filespec", "EmbeddedFile")
	for _, f := range files {
		t.AddRow(
			// Every PDF-derived cell is escaped: AFRelationship and Subtype come
			// from PDF Names, whose #xx escapes resolve to arbitrary bytes. A raw
			// 0x0A would split the row; a multi-byte rune would break padding.
			dashIfEmpty(asciiSafe(f.Name)),
			dashIfEmpty(asciiSafe(f.AFRelationship)),
			dashIfEmpty(asciiSafe(f.Subtype)),
			humanizeBytes(f.Size),
			dashIfEmpty(f.FilespecRef),
			dashIfEmpty(f.EmbeddedFileRef),
		)
	}
	return t.Render(out)
}

// execEmbeddedExtractByRef extracts one embedded file's raw decoded bytes to
// stdout, addressed by a "N G R" or obj:G:N ref to its /EmbeddedFile stream. On
// ANY failure stdout stays empty, the error goes to stderr, and the exit code
// is non-zero (so a redirect never captures an error blob as the payload).
func execEmbeddedExtractByRef(filePath, ref string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("internal error: %v", r))
			exitCode = 2
		}
	}()

	objNum, genNum, err := parseObjectRef(ref)
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 1
	}

	ins, _, code := openForCLI(filePath)
	if code != 0 {
		return code
	}
	defer func() { _ = ins.Close("cli") }()

	nodeID := fmt.Sprintf("obj:%d:%d", genNum, objNum)
	return emitEmbeddedBytes(ins, nodeID)
}

// execEmbeddedExtractByName resolves a display name to a single embedded file
// and extracts its bytes. Zero matches = stderr error + non-zero exit; multiple
// matches = stderr error listing the matching refs (the user must disambiguate
// with --ref), never a silent pick.
func execEmbeddedExtractByName(filePath, name string) (exitCode int) {
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

	list, err := ins.GetEmbeddedFiles("cli")
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}

	var matches []pdfcore.EmbeddedFile
	for _, f := range list.Files {
		if f.Name == name {
			matches = append(matches, f)
		}
	}

	switch len(matches) {
	case 0:
		// The selector matches the decoded name; the table prints an escaped form.
		fmt.Fprintf(os.Stderr, "error: no embedded file named %q\n", name)
		if anyNameDiffersFromDisplay(list.Files) {
			fmt.Fprintf(os.Stderr, "hint: this document has attachment names the plain table cannot show verbatim (escaped, empty, or space-padded). Use `dump embedded --json` and select on the exact \"name\" value.\n")
		}
		return 2
	case 1:
		if matches[0].EmbeddedFileNodeID == "" {
			fmt.Fprintf(os.Stderr, "error: embedded file %q has no /EmbeddedFile stream\n", name)
			return 2
		}
		return emitEmbeddedBytes(ins, matches[0].EmbeddedFileNodeID)
	default:
		refs := make([]string, 0, len(matches))
		for _, m := range matches {
			refs = append(refs, dashIfEmpty(m.EmbeddedFileRef))
		}
		fmt.Fprintf(os.Stderr,
			"error: %d embedded files named %q; disambiguate with --ref using one of: %s\n",
			len(matches), name, strings.Join(refs, ", "))
		return 2
	}
}

// emitEmbeddedBytes fetches the decoded bytes for nodeID and writes them raw to
// stdout. On error stdout stays empty (nothing is written before the error
// check), the error goes to stderr, and the exit code is non-zero.
func emitEmbeddedBytes(ins *pdfcore.Inspector, nodeID string) int {
	data, err := ins.GetEmbeddedFileBytes("cli", nodeID)
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}
	if _, err := os.Stdout.Write(data); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}
