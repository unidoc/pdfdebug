package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"unidoc-pdf-debugger/internal/pdfcore"
)

// defaultFormsDepth is the default form-nesting recursion depth for
// --forms-recursive when --forms-depth is not given. depth is the number of form
// LEVELS fully expanded (a form's OWN /Resources classified): depth N expands
// the page's direct forms and surfaces the names/refs of the forms they declare,
// down through N levels. The default of 2 expands the page's direct forms AND
// names the forms THEY declare (the own-resources gotcha) while keeping the
// artifact bounded; depth 1 lists the page's direct forms without classifying
// their resources. It is a THIRD recursion axis, distinct from `dump tree`'s
// --depth (tree-walk) and 11-5's --resolve-depth (ref-following).
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
	json           bool
	pretty         bool
}

// runPageDump parses flags for `dump page` and dispatches the assembled
// per-page render-info view. --info N selects 1-based page N.
// EXPERIMENTAL: the JSON field set is not a frozen contract.
func runPageDump(args []string) int {
	fs := flag.NewFlagSet("dump page", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	infoFlag := fs.Int("info", 0, "Assemble the rendering picture for 1-based page N")
	formsRecursiveFlag := fs.Bool("forms-recursive", false, "Walk nested Form XObject resources/content")
	formsDepthFlag := fs.Int("forms-depth", defaultFormsDepth, "Form-content recursion depth (distinct from --depth and --resolve-depth)")
	sectionFlag := fs.String("section", "", "Emit only one section: geometry|extgstates|xobjects|forms")
	jsonFlag := fs.Bool("json", false, "Output structured JSON (default is human-readable plain text)")
	prettyFlag := fs.Bool("pretty", false, "Indent JSON output (no effect on plain text)")
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
		json:           *jsonFlag,
		pretty:         *prettyFlag,
	}

	// --info is the required mode selector and must name a 1-based page.
	if !flags.infoSet || flags.info < 1 {
		writeJSONError(os.Stderr, "invalid --info: must be >= 1 (pages are 1-based)")
		return 1
	}
	// --section, when present, must be one of the closed 4-value enum.
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
// JSON (or the selected --section). A page-not-found maps to exit 2; a valid
// page with no /Resources emits empty arrays at exit 0.
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
	// to exit 2 with a clear message, parallel to dump stream.
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

	if flags.json {
		out := sectionView(result, flags.section)
		// _stability marker: the full object is the one EXPERIMENTAL contract an
		// agent may script against, so it carries a top-level
		// "_stability":"experimental" field. Section-scoped views OMIT it (decided:
		// the marker attaches to the full object only - the scoped maps are already
		// understood to be slices of the unstable whole).
		if flags.section == "" {
			marked, err := withStabilityMarker(result)
			if err != nil {
				writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
				return 2
			}
			out = marked
		}
		if err := emit(os.Stdout, out, flags.pretty); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		return 0
	}
	if err := printPageInfoPlain(os.Stdout, result, flags.section); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}

// withStabilityMarker projects the full PageRenderInfo into a map carrying a
// top-level "_stability":"experimental" field. Decoding through
// json.RawMessage preserves every field's serialized form verbatim (decoding
// into map[string]any would relabel ints as float64).
func withStabilityMarker(info *pdfcore.PageRenderInfo) (any, error) {
	b, err := json.Marshal(info)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	m["_stability"] = json.RawMessage(`"experimental"`)
	return m, nil
}

// printPageInfoPlain renders the assembled per-page render info as aligned
// key/value sections (geometry, extgstates, xobjects, forms), honoring the
// --section filter (empty = all sections). NON-CONTRACTUAL and EXPERIMENTAL;
// use --json to parse. patterns/shadings appear only in the all-sections view.
func printPageInfoPlain(out io.Writer, info *pdfcore.PageRenderInfo, section string) error {
	var w kvWriter
	all := section == ""

	if all || section == "geometry" {
		w.Heading("Geometry:")
		w.Addf("  Page", "%d", info.Page)
		w.Add("  PageRef", info.PageRef)
		w.Add("  MediaBox", floatsString(info.MediaBox))
		if info.CropBox != nil {
			w.Add("  CropBox", floatsString(info.CropBox))
		}
		w.Addf("  Rotate", "%d", info.Rotate)
	}
	if all || section == "extgstates" {
		if !all {
			w.Heading("ExtGStates:")
		} else {
			w.Gap()
			w.Heading("ExtGStates:")
		}
		for _, gs := range info.ExtGStates {
			w.Add("  "+gs.Name, extGStateSummary(gs))
		}
	}
	if all || section == "xobjects" {
		if all {
			w.Gap()
		}
		w.Heading("XObjects:")
		for _, x := range info.XObjects {
			w.Add("  "+x.Name, xobjectSummary(x))
		}
	}
	if all {
		w.Gap()
		w.Heading("Patterns:")
		for _, p := range info.Patterns {
			w.Addf("  "+p.Name, "%s patternType %d", p.Ref, p.PatternType)
		}
		w.Gap()
		w.Heading("Shadings:")
		for _, s := range info.Shadings {
			w.Addf("  "+s.Name, "%s shadingType %d", s.Ref, s.ShadingType)
		}
	}
	if (all && len(info.Forms) > 0) || section == "forms" {
		if all {
			w.Gap()
		}
		w.Heading("Forms:")
		for _, fm := range info.Forms {
			writeFormRow(&w, fm, 1)
		}
	}
	return w.Render(out)
}

// extGStateSummary renders a one-line summary of an ExtGState's transparency
// parameters for the plain-text view.
func extGStateSummary(gs pdfcore.ExtGStateInfo) string {
	parts := []string{gs.Ref}
	if gs.BM != "" {
		parts = append(parts, "BM "+gs.BM)
	}
	if gs.CA != nil {
		parts = append(parts, fmt.Sprintf("ca %g", *gs.CA))
	}
	if gs.Ca != nil {
		parts = append(parts, fmt.Sprintf("CA %g", *gs.Ca))
	}
	if gs.SMask != nil {
		parts = append(parts, "SMask")
	}
	return strings.Join(parts, " ")
}

// xobjectSummary renders a one-line summary of an XObject (Form bbox / Image
// dimensions + colorspace family) for the plain-text view.
func xobjectSummary(x pdfcore.XObjectRenderInfo) string {
	switch x.Subtype {
	case "Image":
		cs := ""
		if x.ColorSpace != nil {
			cs = " " + x.ColorSpace.Family
		}
		return fmt.Sprintf("%s Image %dx%d%s", x.Ref, x.Width, x.Height, cs)
	case "Form":
		return fmt.Sprintf("%s Form bbox %s", x.Ref, floatsString(x.BBox))
	default:
		return fmt.Sprintf("%s %s", x.Ref, x.Subtype)
	}
}

// writeFormRow appends one recursive form-walk node (and its nested forms) to
// the kvWriter at the given indent depth.
func writeFormRow(w *kvWriter, fm pdfcore.FormRenderInfo, depth int) {
	indent := strings.Repeat("  ", depth)
	tag := fm.Ref
	if fm.Cyclic {
		tag += " [cyclic]"
	}
	if fm.Truncated {
		tag += " [truncated]"
	}
	w.Add(indent+fm.Name, tag)
	for _, child := range fm.Forms {
		writeFormRow(w, child, depth+1)
	}
}

// floatsString renders a float slice as space-separated values (e.g. a
// MediaBox "0 0 612 792"), trimming trailing zeros via %g.
func floatsString(fs []float64) string {
	parts := make([]string, len(fs))
	for i, v := range fs {
		parts[i] = fmt.Sprintf("%g", v)
	}
	return strings.Join(parts, " ")
}

// sectionView returns the full PageRenderInfo when section is "", or a
// section-scoped object carrying only that section (so a complex page is not a
// multi-MB wall). Patterns and shadings are never section-selectable; they
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
