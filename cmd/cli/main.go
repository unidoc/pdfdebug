// Package main provides the pdfdebug CLI binary for PDF inspection.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
)

// version is overridden via -ldflags "-X main.version=x.y.z" in release builds.
var version = "dev"

func main() {
	// Suppress pdfcpu's internal log output so stderr stays clean for JSON errors.
	log.SetOutput(io.Discard)

	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "--help", "-h", "help":
		printUsage(os.Stderr)
		os.Exit(0)
	case "--version", "-v":
		_, _ = fmt.Fprintf(os.Stdout, "pdfdebug version %s\n", version)
		os.Exit(0)
	case "dump":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: pdfdebug dump <resource> [flags] <file>")
			os.Exit(1)
		}
		resource := os.Args[2]
		remaining := os.Args[3:]
		switch resource {
		case "tree":
			os.Exit(runTreeDump(remaining))
		case "object":
			os.Exit(runObjectDump(remaining))
		case "stream":
			os.Exit(runStreamDump(remaining))
		case "page":
			os.Exit(runPageDump(remaining))
		case "font":
			os.Exit(runFontDump(remaining))
		case "image":
			os.Exit(runImageDump(remaining))
		case "source":
			os.Exit(runSourceDump(remaining))
		case "reverserefs":
			os.Exit(runReverseRefsDump(remaining))
		case "xref":
			os.Exit(runXRefDump(remaining))
		case "objects":
			os.Exit(runObjectsDump(remaining))
		case "plaintext":
			os.Exit(runPlaintextDump(remaining))
		default:
			fmt.Fprintf(os.Stderr, "Unknown resource: %s\n", resource)
			printUsage(os.Stderr)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage(os.Stderr)
		os.Exit(1)
	}
}

// printUsage writes the CLI help text to w.
func printUsage(w io.Writer) {
	_, _ = fmt.Fprintf(w, "pdfdebug version %s\n", version)
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Usage: pdfdebug <command> [flags]")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Commands (all emit human-readable plain text by default; add --json for structured JSON):")
	_, _ = fmt.Fprintln(w, "  dump tree [--json] [--depth N] [--resolve [--resolve-depth N]] <file>  Dump the PDF object tree")
	_, _ = fmt.Fprintln(w, "  dump object [--json] [--resolve [--resolve-depth N]] --ref \"N G R\" <file>  Dump a single PDF object")
	_, _ = fmt.Fprintln(w, "  dump stream [--json|--raw|--ops] (--page N | --ref \"N G R\" | --xobject NAME ...) <file>  Dump a content stream")
	_, _ = fmt.Fprintln(w, "  dump page --info N [--json] [--forms-recursive [--forms-depth D]] [--section SECTION] <file>  Assemble per-page render info (EXPERIMENTAL)")
	_, _ = fmt.Fprintln(w, "  dump font [--json] --ref \"N G R\" <file>           Dump a font view (detail/roster/neither)")
	_, _ = fmt.Fprintln(w, "  dump image [--json] [--metadata] --ref \"N G R\" <file>  Dump image XObject data (--metadata omits base64 in JSON)")
	_, _ = fmt.Fprintln(w, "  dump source [--json] [--raw] --ref \"N G R\" <file>  Dump reserialized object source (PDF syntax)")
	_, _ = fmt.Fprintln(w, "  dump reverserefs [--json] --ref \"N G R\" <file>    Dump inbound refs (who points at this object)")
	_, _ = fmt.Fprintln(w, "  dump xref [--json] <file>                         Dump the cross-reference table")
	_, _ = fmt.Fprintln(w, "  dump objects [--json] <file>                      Dump the object index (plural: every object)")
	_, _ = fmt.Fprintln(w, "  dump plaintext [--json] <file>                    Dump document bytes as text (raw by default; --json wraps the decoded text)")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Flags:")
	_, _ = fmt.Fprintln(w, "  --json      Output structured JSON (default is human-readable plain text)")
	_, _ = fmt.Fprintln(w, "  --pretty    Indent JSON output (default is compact single-line; no effect on plain text)")
	_, _ = fmt.Fprintln(w, "  --help      Show this help message")
	_, _ = fmt.Fprintln(w, "  --version   Show version information")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Examples: (each shown in its native plain-text default; add --json to parse)")
	_, _ = fmt.Fprintln(w, "  pdfdebug dump tree file.pdf")
	_, _ = fmt.Fprintln(w, "  pdfdebug dump tree --page 1 file.pdf")
	_, _ = fmt.Fprintln(w, "  pdfdebug dump object --ref \"4654 0 R\" file.pdf")
	_, _ = fmt.Fprintln(w, "  pdfdebug dump stream --page 1 file.pdf")
	_, _ = fmt.Fprintln(w, "  pdfdebug dump page --info 1 file.pdf")
	_, _ = fmt.Fprintln(w, "  pdfdebug dump page --info 1 --forms-recursive --forms-depth 2 file.pdf")
	_, _ = fmt.Fprintln(w, "  pdfdebug dump page --info 1 --section extgstates file.pdf")
	_, _ = fmt.Fprintln(w, "  pdfdebug dump font --ref \"4 0 R\" file.pdf")
	_, _ = fmt.Fprintln(w, "  pdfdebug dump image --metadata --ref \"4 0 R\" file.pdf")
	_, _ = fmt.Fprintln(w, "  pdfdebug dump source --ref \"4 0 R\" file.pdf")
	_, _ = fmt.Fprintln(w, "  pdfdebug dump reverserefs --ref \"4 0 R\" file.pdf")
	_, _ = fmt.Fprintln(w, "  pdfdebug dump xref file.pdf")
	_, _ = fmt.Fprintln(w, "  pdfdebug dump objects file.pdf")
	_, _ = fmt.Fprintln(w, "  pdfdebug dump plaintext file.pdf")
	_, _ = fmt.Fprintln(w, "  pdfdebug dump tree --json file.pdf       # opt into structured JSON")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Plain text is for reading and may change between releases. To parse output")
	_, _ = fmt.Fprintln(w, "reliably (scripts, agents), pass --json. The --raw / --ops stream payloads and")
	_, _ = fmt.Fprintln(w, "the raw `dump plaintext` bytes are separate machine formats.")
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Note: `dump page --info` is EXPERIMENTAL - its JSON field set is not a frozen")
	_, _ = fmt.Fprintln(w, "      contract and may change (the --json output carries a \"_stability\":")
	_, _ = fmt.Fprintln(w, "      \"experimental\" marker). It is structural only (no rendering computation).")
	_, _ = fmt.Fprintln(w, "      For anything it omits, use dump stream --xobject/--ref/--ops or")
	_, _ = fmt.Fprintln(w, "      dump tree/dump object --resolve.")
}

// emit writes v as JSON to w: compact single-line when pretty is false (the
// default, agent-friendly), or indented multi-line when pretty is true.
func emit(w io.Writer, v any, pretty bool) error {
	if pretty {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		_, err = w.Write(append(b, '\n'))
		return err
	}
	return json.NewEncoder(w).Encode(v)
}

// writeJSONError writes a JSON error object to w.
func writeJSONError(w io.Writer, msg string) {
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// writeJSONWarning writes a JSON warning object to w.
func writeJSONWarning(w io.Writer, msg string) {
	_ = json.NewEncoder(w).Encode(map[string]string{"warning": msg})
}

// defaultResolveDepth is the default ref-following depth for --resolve when
// --resolve-depth is not given: a small N so output stays bounded.
const defaultResolveDepth = 1

// dumpFlags holds the parsed common flags for the tree dump subcommand.
type dumpFlags struct {
	json    bool
	depth   int
	pretty  bool
	page    int
	pageSet bool // true when --page was explicitly provided (distinguishes --page 0 from absent)
	// resolve follows indirect refs inline via pdfcore.ResolveRef. resolveDepth
	// is its ref-following depth - a SEPARATE flag from --depth (which is
	// tree-walk depth, 0 = unlimited), because the two axes have opposite
	// zero-semantics and cannot share a name.
	resolve      bool
	resolveDepth int
}

// parseDumpFlags creates a FlagSet for dump subcommands with common flags.
func parseDumpFlags(name string, args []string) (*flag.FlagSet, dumpFlags, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard) // suppress default flag error output
	jsonFlag := fs.Bool("json", false, "Output structured JSON (default is human-readable plain text)")
	depthFlag := fs.Int("depth", 0, "Max tree depth (0 = unlimited)")
	prettyFlag := fs.Bool("pretty", false, "Indent JSON output (no effect on plain text)")
	pageFlag := fs.Int("page", 0, "Root the tree at page N's dict (1-based; 0 = catalog)")
	resolveFlag := fs.Bool("resolve", false, "Follow indirect refs inline via ResolveRef (adds a 'resolved' field)")
	resolveDepthFlag := fs.Int("resolve-depth", defaultResolveDepth, "Ref-following depth for --resolve (distinct from --depth tree-walk depth)")
	if err := fs.Parse(args); err != nil {
		return fs, dumpFlags{}, err
	}
	pageSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "page" {
			pageSet = true
		}
	})
	return fs, dumpFlags{
		json:         *jsonFlag,
		depth:        *depthFlag,
		pretty:       *prettyFlag,
		page:         *pageFlag,
		pageSet:      pageSet,
		resolve:      *resolveFlag,
		resolveDepth: *resolveDepthFlag,
	}, nil
}
