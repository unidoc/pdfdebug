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

// streamFlags holds the parsed flags for the stream dump subcommand.
type streamFlags struct {
	page    int
	pageSet bool
	json    bool
	raw     bool
	ops     bool
	pretty  bool
	xobject string
	ref     string
}

const streamUsage = "Usage: pdfdebug dump stream [--json|--raw|--ops] [--page N | --ref \"N G R\" | --xobject NAME (--page N | --ref \"N G R\")] <file>"

// runStreamDump parses flags and dispatches stream dump execution. It resolves
// the target content stream from one of three input modes - page (--page N),
// direct object (--ref REF), or named XObject (--xobject NAME against a page or
// REF) - then emits the decoded stream as default JSON, raw bytes (--raw), or
// one-object-per-operator NDJSON (--ops).
func runStreamDump(args []string) int {
	fs := flag.NewFlagSet("dump stream", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	pageFlag := fs.Int("page", 0, "Page number (1-based)")
	rawFlag := fs.Bool("raw", false, "Emit verbatim decoded bytes instead of JSON")
	opsFlag := fs.Bool("ops", false, "Emit one JSON object per operator as NDJSON (mutually exclusive with --raw; --pretty ignored)")
	prettyFlag := fs.Bool("pretty", false, "Indent JSON output (no-op with --raw and --ops)")
	xobjectFlag := fs.String("xobject", "", "Resolve a named XObject stream from --page N's or --ref REF's /Resources/XObject")
	refFlag := fs.String("ref", "", "Resolve a content-stream object directly (\"N G R\"); or the XObject owner with --xobject")
	jsonFlag := fs.Bool("json", false, "Output structured JSON (default is a human-readable operator listing; mutually exclusive with --raw/--ops)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, streamUsage)
		return 1
	}

	pageSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "page" {
			pageSet = true
		}
	})

	flags := streamFlags{
		page:    *pageFlag,
		pageSet: pageSet,
		json:    *jsonFlag,
		raw:     *rawFlag,
		ops:     *opsFlag,
		pretty:  *prettyFlag,
		xobject: *xobjectFlag,
		ref:     *refFlag,
	}

	// --raw / --ops / --json select mutually-exclusive payloads/formats. --raw
	// (decoded bytes) and --ops (NDJSON) are payload selectors; --json is the
	// structured-JSON format. The --raw/--ops conflict is preexisting; the
	// --raw+--json and --ops+--json rejections are net-new: --json no longer
	// combines silently now that it is a real format switch.
	if flags.ops && flags.raw {
		writeJSONError(os.Stderr, "--ops and --raw are mutually exclusive")
		return 1
	}
	if flags.json && flags.raw {
		writeJSONError(os.Stderr, "--json and --raw are mutually exclusive")
		return 1
	}
	if flags.json && flags.ops {
		writeJSONError(os.Stderr, "--json and --ops are mutually exclusive")
		return 1
	}

	// Flag matrix (Decision section): determine the input mode and validate.
	switch {
	case flags.xobject != "":
		// --xobject requires exactly one owner: --page N or --ref REF.
		if !flags.pageSet && flags.ref == "" {
			writeJSONError(os.Stderr, "--xobject requires either --page N or --ref \"N G R\" to name the owning resources")
			return 1
		}
		if flags.pageSet && flags.ref != "" {
			writeJSONError(os.Stderr, "--xobject takes either --page N or --ref \"N G R\", not both")
			return 1
		}
		if flags.pageSet && flags.page < 1 {
			writeJSONError(os.Stderr, "invalid --page: must be >= 1 (pages are 1-based)")
			return 1
		}
	case flags.ref != "":
		// --ref alone: REF is the stream object directly. --ref + --page is ambiguous.
		if flags.pageSet {
			writeJSONError(os.Stderr, "--ref and --page are mutually exclusive (use --xobject NAME to resolve a name against either)")
			return 1
		}
	default:
		// --page mode (today's behavior): --page is required and 1-based.
		if !flags.pageSet || flags.page < 1 {
			writeJSONError(os.Stderr, "invalid --page: must be >= 1 (pages are 1-based)")
			return 1
		}
	}

	filePath := fs.Arg(0)
	if filePath == "" {
		fmt.Fprintln(os.Stderr, streamUsage)
		return 1
	}

	return execStreamDump(filePath, flags)
}

// execStreamDump opens the PDF, resolves the target content-stream node from
// the chosen input mode, and writes the decoded ContentStreamData as JSON
// (default), verbatim decoded bytes (--raw), or per-operator NDJSON (--ops).
func execStreamDump(filePath string, flags streamFlags) (exitCode int) {
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

	// Resolve the input mode: --ref/--xobject address a single stream; the page
	// mode (opsPageNum > 0) resolves to the page's COMPLETE content, and the
	// page number also backs Do classification under --ops.
	nodeID, opsPageNum, code := resolveStreamNode(inspector, info, flags)
	if code != 0 {
		return code
	}

	var result *pdfcore.ContentStreamData
	if opsPageNum > 0 {
		// Page mode: a page's content is the CONCATENATION of every stream in its
		// /Contents array (ISO 32000-1 7.8.2), assembled and tokenized as one
		// program - no stream is dropped or presented as if it were the whole.
		r, err := inspector.GetPageContentStream("cli", opsPageNum)
		if err != nil {
			writeJSONError(os.Stderr, err.Error())
			return 2
		}
		if r == nil {
			// No /Contents on the page: valid PDF, just no stream.
			return writeNoContentStream(flags)
		}
		result = r
	} else {
		// --ref / --xobject: a single addressed stream.
		r, err := inspector.GetContentStream("cli", nodeID)
		if err != nil {
			writeJSONError(os.Stderr, err.Error())
			return 2
		}
		result = r
	}

	// GetContentStream/GetPageContentStream return a non-error ContentStreamData{
	// Error:...} for "node is not a stream object" and decode failures. exit 2.
	if result.Error != "" {
		writeJSONError(os.Stderr, result.Error)
		return 2
	}

	if flags.raw {
		if _, err := io.WriteString(os.Stdout, result.Raw); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write raw output: %v\n", err)
			return 2
		}
		return 0
	}

	if flags.ops {
		return emitOps(inspector, result, opsPageNum)
	}

	if flags.json {
		if err := emit(os.Stdout, result, flags.pretty); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		return 0
	}

	if err := printStreamPlain(os.Stdout, result); err != nil {
		writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
		return 2
	}
	return 0
}

// writeNoContentStream renders the "page has no /Contents" condition on the
// selected surface and exits 0 (a valid page, just no stream): zero NDJSON
// lines with the note on stderr for --ops, a JSON object carrying the note for
// --json, and a one-line note on stdout otherwise (plain and --raw).
func writeNoContentStream(flags streamFlags) int {
	if flags.ops {
		fmt.Fprintln(os.Stderr, "page has no content stream")
		return 0
	}
	if flags.json {
		result := &pdfcore.ContentStreamData{Raw: "", Error: "page has no content stream"}
		if err := emit(os.Stdout, result, flags.pretty); err != nil {
			writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
			return 2
		}
		return 0
	}
	if _, err := io.WriteString(os.Stdout, "(page has no content stream)\n"); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write output: %v\n", err)
		return 2
	}
	return 0
}

// printStreamPlain renders the decoded content stream as a human-readable
// operator listing: one operator per line, operands before the operator in
// PDF content-stream order (let it flow; do not tabulate). NON-CONTRACTUAL;
// use --json for structured operators, --ops for NDJSON, --raw for bytes.
func printStreamPlain(out io.Writer, result *pdfcore.ContentStreamData) error {
	// A content-stream object can exist yet decode to zero operators (an empty
	// /Contents stream). Surface that as a one-line note so plain output is never
	// a silent zero-byte write and always ends with a newline.
	if len(result.Formatted) == 0 {
		_, err := io.WriteString(out, "(empty content stream)\n")
		return err
	}
	var b strings.Builder
	for _, fl := range result.Formatted {
		for range fl.Indent {
			b.WriteString("  ")
		}
		operands := operandValues(fl)
		if len(operands) > 0 {
			b.WriteString(strings.Join(operands, " "))
			if fl.Operator != "" {
				b.WriteByte(' ')
			}
		}
		b.WriteString(fl.Operator)
		b.WriteByte('\n')
	}
	_, err := io.WriteString(out, b.String())
	return err
}

// resolveStreamNode maps the input flags to a content-stream target. For
// --ref/--xobject it returns the addressed stream's nodeID with opsPageNum 0.
// For page mode it returns nodeID "" and opsPageNum = the (validated) page
// number: the caller fetches the page's complete content via
// GetPageContentStream (a single page-dict resolution), and opsPageNum also
// backs Do classification under --ops. code is non-zero on error (reported).
func resolveStreamNode(inspector *pdfcore.Inspector, info *pdfcore.DocumentInfo, flags streamFlags) (nodeID string, opsPageNum int, code int) {
	switch {
	case flags.xobject != "":
		ownerNodeID, c := xobjectOwnerNodeID(inspector, info, flags)
		if c != 0 {
			return "", 0, c
		}
		resources, err := inspector.GetXObjectResources("cli", ownerNodeID)
		if err != nil {
			writeJSONError(os.Stderr, err.Error())
			return "", 0, 2
		}
		entry, ok := resources[flags.xobject]
		if !ok || entry.NodeID == "" {
			writeJSONError(os.Stderr, fmt.Sprintf("XObject %q not found in resources", flags.xobject))
			return "", 0, 2
		}
		// A resolved form/image XObject stream has no page; Do classification is
		// page-scoped only (Decision: --ops resourceType only for page streams).
		return entry.NodeID, 0, 0

	case flags.ref != "":
		objNum, genNum, err := parseObjectRef(flags.ref)
		if err != nil {
			writeJSONError(os.Stderr, err.Error())
			return "", 0, 1
		}
		return fmt.Sprintf("obj:%d:%d", genNum, objNum), 0, 0

	default:
		// Page mode: validate the range only. GetPageContentStream (caller)
		// resolves and concatenates the page's content in a single page-dict pass.
		if info.PageCount == 0 {
			writeJSONError(os.Stderr, "cannot determine page count for this PDF")
			return "", 0, 2
		}
		if flags.page > info.PageCount {
			writeJSONError(os.Stderr, fmt.Sprintf("page %d out of range: document has %d pages", flags.page, info.PageCount))
			return "", 0, 2
		}
		return "", flags.page, 0
	}
}

// xobjectOwnerNodeID resolves the node ID of the container whose
// /Resources/XObject is searched for --xobject NAME: page N's page dict (when
// --page) or the REF object (when --ref).
func xobjectOwnerNodeID(inspector *pdfcore.Inspector, info *pdfcore.DocumentInfo, flags streamFlags) (nodeID string, code int) {
	if flags.pageSet {
		if info.PageCount == 0 {
			writeJSONError(os.Stderr, "cannot determine page count for this PDF")
			return "", 2
		}
		if flags.page > info.PageCount {
			writeJSONError(os.Stderr, fmt.Sprintf("page %d out of range: document has %d pages", flags.page, info.PageCount))
			return "", 2
		}
		pageNode, err := inspector.GetPageNode("cli", flags.page)
		if err != nil {
			writeJSONError(os.Stderr, err.Error())
			return "", 2
		}
		return pageNode.ID, 0
	}
	// --ref owner.
	objNum, genNum, err := parseObjectRef(flags.ref)
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return "", 1
	}
	return fmt.Sprintf("obj:%d:%d", genNum, objNum), 0
}

// opsLine is one NDJSON operator object emitted by --ops. ResourceType and
// ObjectRef are populated only for a Do op whose operand resolves to a page
// XObject with a /Subtype of /Image or /Form; otherwise they are omitted.
type opsLine struct {
	Op           string   `json:"op"`
	Params       []string `json:"params"`
	SrcLineStart int      `json:"srcLineStart"`
	SrcLineEnd   int      `json:"srcLineEnd"`
	ResourceType string   `json:"resourceType,omitempty"`
	ObjectRef    string   `json:"objectRef,omitempty"`
}

// emitOps writes the content stream's Formatted lines as NDJSON: one compact
// JSON object per operator, one per line. For Do ops on a page stream
// (pageNum > 0), the operand XObject name is classified via the page's
// /Resources/XObject into resourceType ("Image"/"Form") + objectRef. --pretty
// does not apply (NDJSON is always compact).
func emitOps(inspector *pdfcore.Inspector, result *pdfcore.ContentStreamData, pageNum int) (exitCode int) {
	// Page XObject resources for Do classification (page streams only). A lookup
	// failure leaves Do ops unannotated rather than failing the dump, but warns
	// on stderr so the missing resourceType is not silently indistinguishable
	// from "operand name not found in resources".
	var resources map[string]pdfcore.XObjectInfo
	if pageNum > 0 {
		pageNode, err := inspector.GetPageNode("cli", pageNum)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: Do classification unavailable for page %d: %v\n", pageNum, err)
		} else if r, rerr := inspector.GetXObjectResources("cli", pageNode.ID); rerr != nil {
			fmt.Fprintf(os.Stderr, "warning: Do classification unavailable for page %d: %v\n", pageNum, rerr)
		} else {
			resources = r
		}
	}

	// json.Encoder writes compact JSON + a trailing newline per Encode call:
	// exactly one JSON object per line (NDJSON). --pretty does not apply.
	enc := json.NewEncoder(os.Stdout)
	for _, fl := range result.Formatted {
		// --ops is operator-centric NDJSON: one object per operator. Comment
		// lines and trailing dangling-operand runs carry Operator == "" and are
		// not operators; skipping them upholds the one-object-per-operator
		// contract (a phantom {"op":""} record would breach it). --json is
		// unaffected -- it serializes the full Formatted slice.
		if fl.Operator == "" {
			continue
		}
		line := opsLine{
			Op:           fl.Operator,
			Params:       operandValues(fl),
			SrcLineStart: fl.SrcLineStart,
			SrcLineEnd:   fl.SrcLineEnd,
		}
		if fl.Operator == "Do" && resources != nil {
			classifyDo(&line, resources)
		}
		if err := enc.Encode(line); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write NDJSON output: %v\n", err)
			return 2
		}
	}
	return 0
}

// classifyDo attaches resourceType + objectRef to a Do op when its name operand
// resolves to a page XObject whose /Subtype is /Image or /Form. A name that
// does not resolve, or whose /Subtype is neither, leaves the op unannotated
// (does not crash, does not mislabel).
func classifyDo(line *opsLine, resources map[string]pdfcore.XObjectInfo) {
	for _, p := range line.Params {
		if len(p) == 0 || p[0] != '/' {
			continue
		}
		name := p[1:] // strip leading "/"
		info, ok := resources[name]
		if !ok {
			continue
		}
		switch info.Subtype {
		case "Image", "Form":
			line.ResourceType = info.Subtype
			line.ObjectRef = info.ObjectRef
		}
		return
	}
}

// operandValues returns the raw token Value strings that make up a formatted
// line's operands: every token EXCEPT the trailing operator token (whose value
// equals the line's Operator). Structural delimiter tokens ([ ] << >>) are
// kept as raw values so array/dict operands (e.g. TJ, BDC) round-trip; a
// zero-operand op (BT/ET) yields an empty params slice. Comment lines
// (Operator=="") carry the comment token verbatim.
func operandValues(fl pdfcore.FormattedLine) []string {
	params := make([]string, 0, len(fl.Tokens))
	for i, tk := range fl.Tokens {
		// Drop the trailing operator token (last token whose value is the op).
		if fl.Operator != "" && i == len(fl.Tokens)-1 && tk.Value == fl.Operator {
			continue
		}
		params = append(params, tk.Value)
	}
	return params
}
