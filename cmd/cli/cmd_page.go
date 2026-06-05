package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// defaultFormsDepth is the default form-nesting recursion depth for
// --forms-recursive when --forms-depth is not given. depth is the number of form
// LEVELS fully expanded (a form's OWN /Resources classified): depth N expands
// the page's direct forms and surfaces the names/refs of the forms they declare,
// down through N levels. The default of 2 expands the page's direct forms AND
// names the forms THEY declare (the own-resources gotcha, AC4-002) while keeping
// the artifact bounded; depth 1 lists the page's direct forms without
// classifying their resources. It is a THIRD recursion axis, distinct from
// `dump tree`'s --depth (tree-walk) and 11-5's --resolve-depth (ref-following).
const defaultFormsDepth = 2

// pageSections is the exact, closed enum of --section values. patterns/shadings
// are intentionally NOT selectable (they appear only in the full object).
var pageSections = map[string]bool{
	"geometry":   true,
	"extgstates": true,
	"xobjects":   true,
	"forms":      true,
}

const pageUsage = "Usage: pdfdebug dump page --info N [--forms-recursive [--forms-depth D]] [--section geometry|extgstates|xobjects|forms] [--pretty] <file>"

// pageFlags holds the parsed flags for the page dump subcommand.
type pageFlags struct {
	info           int
	infoSet        bool
	formsRecursive bool
	formsDepth     int
	section        string
	pretty         bool
}

// runPageDump parses flags for `dump page` and dispatches the assembled
// per-page render-info view (Story 11-6). --info N selects 1-based page N.
// EXPERIMENTAL: the JSON field set is not a frozen contract; see AC8.
func runPageDump(args []string) int {
	fs := flag.NewFlagSet("dump page", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	infoFlag := fs.Int("info", 0, "Assemble the rendering picture for 1-based page N")
	formsRecursiveFlag := fs.Bool("forms-recursive", false, "Walk nested Form XObject resources/content")
	formsDepthFlag := fs.Int("forms-depth", defaultFormsDepth, "Form-content recursion depth (distinct from --depth and --resolve-depth)")
	sectionFlag := fs.String("section", "", "Emit only one section: geometry|extgstates|xobjects|forms")
	prettyFlag := fs.Bool("pretty", false, "Indent JSON output")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, pageUsage)
		return 1
	}

	infoSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "info" {
			infoSet = true
		}
	})

	flags := pageFlags{
		info:           *infoFlag,
		infoSet:        infoSet,
		formsRecursive: *formsRecursiveFlag,
		formsDepth:     *formsDepthFlag,
		section:        *sectionFlag,
		pretty:         *prettyFlag,
	}

	// --info is the required mode selector and must name a 1-based page.
	if !flags.infoSet || flags.info < 1 {
		writeJSONError(os.Stderr, "invalid --info: must be >= 1 (pages are 1-based)")
		return 1
	}
	// --section, when present, must be one of the closed 4-value enum (AC5).
	if flags.section != "" && !pageSections[flags.section] {
		writeJSONError(os.Stderr, fmt.Sprintf("invalid --section %q: must be one of geometry|extgstates|xobjects|forms", flags.section))
		return 1
	}
	// --forms-depth is meaningless without --forms-recursive, but a non-negative
	// value is still required when it IS set (a malformed arg is a usage error).
	if flags.formsDepth < 0 {
		writeJSONError(os.Stderr, "invalid --forms-depth: must be >= 0")
		return 1
	}

	filePath := fs.Arg(0)
	if filePath == "" {
		fmt.Fprintln(os.Stderr, pageUsage)
		return 1
	}

	return execPageDump(filePath, flags)
}

// execPageDump opens the PDF, assembles the page render-info, and emits it as
// JSON (or the selected --section). A page-not-found maps to exit 2 (AC6); a
// valid page with no /Resources emits empty arrays at exit 0.
func execPageDump(filePath string, flags pageFlags) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("internal error: %v", r))
			exitCode = 2
		}
	}()

	inspector, info, code := openForCLI(filePath)
	if code != 0 {
		return code
	}
	defer func() { _ = inspector.Close("cli") }()

	// Upper-bound the page against the document's page count so out-of-range maps
	// to exit 2 with a clear message (AC6), parallel to dump stream.
	if info.PageCount > 0 && flags.info > info.PageCount {
		writeJSONError(os.Stderr, fmt.Sprintf("page %d out of range: document has %d pages", flags.info, info.PageCount))
		return 2
	}

	result, err := inspector.PageRenderInfo("cli", flags.info, pdfcore.PageRenderOpts{
		FormsRecursive: flags.formsRecursive,
		FormsDepth:     flags.formsDepth,
	})
	if err != nil {
		// Page-not-found (and any other resolution failure) -> exit 2.
		writeJSONError(os.Stderr, err.Error())
		return 2
	}

	out := sectionView(result, flags.section)
	if err := emit(os.Stdout, out, flags.pretty); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}

// sectionView returns the full PageRenderInfo when section is "", or a
// section-scoped object carrying only that section (so a complex page is not a
// multi-MB wall, AC5). patterns/shadings are never section-selectable; they
// appear only in the full object.
func sectionView(info *pdfcore.PageRenderInfo, section string) any {
	switch section {
	case "geometry":
		return map[string]any{
			"page":     info.Page,
			"pageRef":  info.PageRef,
			"mediaBox": info.MediaBox,
			"cropBox":  info.CropBox,
			"rotate":   info.Rotate,
		}
	case "extgstates":
		return map[string]any{"page": info.Page, "extGStates": info.ExtGStates}
	case "xobjects":
		return map[string]any{"page": info.Page, "xobjects": info.XObjects}
	case "forms":
		return map[string]any{"page": info.Page, "forms": info.Forms}
	default:
		return info
	}
}
