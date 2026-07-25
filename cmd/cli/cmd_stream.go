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
	// --raw+--json and --ops+--json rejections are net-new (AC7): --json no
	// longer combines silently now that it is a real format switch.
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

	// Resolve the content-stream nodeID and, for --ops, the page number whose
	// resources back Do classification (0 = no page-backed Do resolution), plus
	// the /Contents array length for the multi-stream truncation marker.
	nodeID, opsPageNum, streamCount, code := resolveStreamNode(inspector, info, flags)
	if code != 0 {
		return code
	}

	// No Contents entry on a page: valid PDF, just no stream.
	if nodeID == "" {
		if flags.ops {
			// NDJSON contract: zero lines on stdout, condition on stderr, exit 0.
			fmt.Fprintln(os.Stderr, "page has no content stream")
			return 0
		}
		result := &pdfcore.ContentStreamData{Raw: "", Error: "page has no content stream"}
		if flags.json {
			if err := emit(os.Stdout, result, flags.pretty); err != nil {
				writeJSONError(os.Stderr, fmt.Sprintf("failed to write output: %v", err))
				return 2
			}
			return 0
		}
		// Plain default: surface the no-stream condition as a one-line note (the
		// page is valid, just has no /Contents).
		if _, err := io.WriteString(os.Stdout, "(page has no content stream)\n"); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write output: %v\n", err)
			return 2
		}
		return 0
	}

	result, err := inspector.GetContentStream("cli", nodeID)
	if err != nil {
		writeJSONError(os.Stderr, err.Error())
		return 2
	}
	// GetContentStream returns a non-error ContentStreamData{Error:...} for
	// "node is not a stream object" and decode failures. Map that to exit 2.
	if result.Error != "" {
		writeJSONError(os.Stderr, result.Error)
		return 2
	}

	// Multi-stream truncation marker (Story 14.3 AC3/AC4, floor path): when the
	// page's /Contents array holds more than one stream, only the first was
	// decoded, so mark the result partial. A single stream (streamCount <= 1) is
	// complete and carries no marker. --raw stays a verbatim byte dump of the one
	// decoded stream (a marker would corrupt the bytes); the note rides plain
	// text, --json, and --ops instead.
	if streamCount > 1 {
		result.StreamCount = streamCount
		result.Shown = 1
		result.Truncated = true
	}

	if flags.raw {
		// A marker cannot ride stdout without corrupting the verbatim byte dump,
		// so on a multi-stream page disclose the truncation on STDERR instead -
		// otherwise --raw would present stream 1's bytes as the whole content
		// stream with no signal on any channel (Story 14.3 AC4, --raw surface).
		if result.Truncated {
			fmt.Fprint(os.Stderr, multiStreamTruncationNote(result))
		}
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

// multiStreamTruncationNote is the one-line note disclosing that a multi-stream
// page's /Contents was truncated to its first decoded stream (Story 14.3 floor
// path). Shared by the plain-text (stdout) and --raw (stderr) surfaces so their
// wording cannot drift; callers guard on result.Truncated before emitting it.
func multiStreamTruncationNote(result *pdfcore.ContentStreamData) string {
	return fmt.Sprintf("(truncated: showing stream %d of %d; page /Contents is a multi-stream array)\n", result.Shown, result.StreamCount)
}

// printStreamPlain renders the decoded content stream as a human-readable
// operator listing: one operator per line, operands before the operator in
// PDF content-stream order (let it flow; do not tabulate). NON-CONTRACTUAL;
// use --json for structured operators, --ops for NDJSON, --raw for bytes.
func printStreamPlain(out io.Writer, result *pdfcore.ContentStreamData) error {
	var b strings.Builder
	// Multi-stream truncation note (Story 14.3 AC3, floor path): a one-line
	// header so the human reader is not shown a partial (often unbalanced)
	// program as if it were the whole content stream. Emitted BEFORE the
	// empty-stream early return: a multi-stream page whose first stream decodes
	// to zero operators must still disclose that streams 2..N exist, otherwise
	// the "(empty content stream)" line would be a silent truncation.
	if result.Truncated {
		b.WriteString(multiStreamTruncationNote(result))
	}
	// A content-stream object can exist yet decode to zero operators (an empty
	// /Contents stream). Surface that as a one-line note so plain output is never
	// a silent zero-byte write and always ends with a newline.
	if len(result.Formatted) == 0 {
		b.WriteString("(empty content stream)\n")
		_, err := io.WriteString(out, b.String())
		return err
	}
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

// resolveStreamNode maps the input flags to a content-stream nodeID. It returns
// the nodeID (or "" when a page has no /Contents), the page number to back Do
// classification under --ops (0 when not a page stream), the number of streams
// in the page's /Contents array (Story 14.3 multi-stream marker; 0 for the
// --ref/--xobject modes and single-stream pages that need no marker), and a
// non-zero exit code on error (already reported to stderr).
func resolveStreamNode(inspector *pdfcore.Inspector, info *pdfcore.DocumentInfo, flags streamFlags) (nodeID string, opsPageNum int, streamCount int, code int) {
	switch {
	case flags.xobject != "":
		ownerNodeID, c := xobjectOwnerNodeID(inspector, info, flags)
		if c != 0 {
			return "", 0, 0, c
		}
		resources, err := inspector.GetXObjectResources("cli", ownerNodeID)
		if err != nil {
			writeJSONError(os.Stderr, err.Error())
			return "", 0, 0, 2
		}
		entry, ok := resources[flags.xobject]
		if !ok || entry.NodeID == "" {
			writeJSONError(os.Stderr, fmt.Sprintf("XObject %q not found in resources", flags.xobject))
			return "", 0, 0, 2
		}
		// A resolved form/image XObject stream has no page; Do classification is
		// page-scoped only (Decision: --ops resourceType only for page streams).
		return entry.NodeID, 0, 0, 0

	case flags.ref != "":
		objNum, genNum, err := parseObjectRef(flags.ref)
		if err != nil {
			writeJSONError(os.Stderr, err.Error())
			return "", 0, 0, 1
		}
		return fmt.Sprintf("obj:%d:%d", genNum, objNum), 0, 0, 0

	default:
		// Page content stream.
		if info.PageCount == 0 {
			writeJSONError(os.Stderr, "cannot determine page count for this PDF")
			return "", 0, 0, 2
		}
		if flags.page > info.PageCount {
			writeJSONError(os.Stderr, fmt.Sprintf("page %d out of range: document has %d pages", flags.page, info.PageCount))
			return "", 0, 0, 2
		}
		// One call resolves the page dict once and returns both the nodeID and
		// the /Contents array length (the count surfaces the multi-stream
		// truncation marker; only the first stream is decoded on the floor path).
		id, count, err := inspector.GetPageContentStreamRef("cli", flags.page)
		if err != nil {
			writeJSONError(os.Stderr, err.Error())
			return "", 0, 0, 2
		}
		return id, flags.page, count, 0
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

	// Multi-stream truncation marker (Story 14.3 AC4): NDJSON has no envelope
	// and Story 14-1 pins --ops to one JSON object PER OPERATOR, so the marker
	// rides a DISTINCT trailing meta record with NO "op" key (a phantom
	// {"op":""} record would breach that contract). It is emitted only for the
	// floor path (a genuinely multi-stream page); the operator records above are
	// stream 1's alone.
	if result.Truncated {
		meta := opsTruncationMeta{Truncated: true, StreamCount: result.StreamCount, Shown: result.Shown}
		if err := enc.Encode(meta); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write NDJSON output: %v\n", err)
			return 2
		}
	}
	return 0
}

// opsTruncationMeta is the trailing --ops NDJSON meta record for a multi-stream
// page (Story 14.3 AC4). It deliberately has NO Op field so consumers keying on
// "op" skip it as a non-operator record, and it carries the /Contents array
// length so a script sees that only Shown of StreamCount streams were emitted.
type opsTruncationMeta struct {
	Truncated   bool `json:"truncated"`
	StreamCount int  `json:"streamCount"`
	Shown       int  `json:"shown"`
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
