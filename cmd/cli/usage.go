package main

import "fmt"

// commandSpec describes one command for the help text. synopsis is the flag
// and argument list both the `pdfdebug --help` Commands block and the
// command's own usage line print, so the two cannot list different flags.
type commandSpec struct {
	name     string
	synopsis string
	summary  string
}

// commandSpecs lists every command in the order the help text shows them.
var commandSpecs = []commandSpec{
	{"dump tree", `[--json] [--pretty] [--depth N] [--page N] [--resolve [--resolve-depth N]] <file>`,
		"Dump the PDF object tree"},
	{"dump object", `[--json] [--pretty] [--resolve [--resolve-depth N]] --ref "N G R" <file>`,
		"Dump a single PDF object"},
	{"dump stream", `[--json|--raw|--ops] [--pretty] (--page N | --ref "N G R" | --xobject NAME (--page N | --ref "N G R")) <file>`,
		"Dump a content stream"},
	{"dump page", `--info N [--json] [--pretty] [--forms-recursive [--forms-depth D]] [--section geometry|extgstates|xobjects|forms] <file>`,
		"Assemble per-page render info (EXPERIMENTAL)"},
	{"dump font", `[--json] [--pretty] [--glyphs] --ref "N G R" <file>`,
		"Dump a font view (detail/roster/neither)"},
	{"dump image", `[--json] [--pretty] [--metadata] --ref "N G R" <file>`,
		"Dump image XObject data (--metadata omits base64 in JSON)"},
	{"dump source", `[--json] [--pretty] [--raw] --ref "N G R" <file>`,
		"Dump reserialized object source (PDF syntax)"},
	{"dump reverserefs", `[--json] [--pretty] --ref "N G R" <file>`,
		"Dump inbound refs (who points at this object)"},
	{"dump xref", `[--json] [--pretty] <file>`,
		"Dump the cross-reference table"},
	{"dump objects", `[--json] [--pretty] <file>`,
		"Dump the object index (plural: every object)"},
	{"dump pages", `[--json] [--pretty] <file>`,
		"Dump the page index (plural: every page leaf, with MediaBox, inheritance and errors)"},
	{"dump images", `[--json] [--pretty] <file>`,
		"Dump the image index (plural: every image XObject referenced from page resources, deduplicated; inline BI/ID/EI images are not listed)"},
	{"dump bytes", `[--json] [--pretty] <file>`,
		"Dump raw document bytes, not extracted page text (use pdftotext for page prose; --json wraps the decoded text)"},
	{"dump embedded", `[--json] [--ref "N G R" | --name NAME] <file>`,
		"List embedded/associated files; --ref/--name extracts one's raw bytes to stdout"},
	{"dump metadata", `[--json] [--pretty] <file>`,
		"Dump the /Info dictionary fields and the XMP metadata packet"},
	{"dump signatures", `[--json] [--pretty] <file>`,
		"Decompose digital-signature fields (signer, chain, ByteRange coverage; no trust verdict)"},
	{"validate", `[--profile pdfa-1b|pdfua-1-structural] [--json] [--pretty] <file>`,
		"Run a named subset of structural checks per profile (structural checks only - not full conformance; use veraPDF for authoritative validation)"},
	{"diff", `[--json] [--pretty] [--full] <left.pdf> <right.pdf>`,
		"Path-aligned STRUCTURAL diff of two PDFs (object model, not byte/pixel; aligned by structural path, not object number)"},
}

// helpSynopsisWidth is the width the command and synopsis are padded to in the
// Commands block, so short lines start their summary on one column.
const helpSynopsisWidth = 48

// usageLine returns the one-line usage string for the named command. It panics
// on an unknown name, which only a typo in this package can produce.
func usageLine(name string) string {
	for _, c := range commandSpecs {
		if c.name == name {
			return "Usage: pdfdebug " + c.name + " " + c.synopsis
		}
	}
	panic("no command spec for " + name)
}

// commandHelpLines returns the Commands-block lines of the help text.
func commandHelpLines() []string {
	lines := make([]string, 0, len(commandSpecs))
	for _, c := range commandSpecs {
		lines = append(lines, fmt.Sprintf("  %-*s  %s", helpSynopsisWidth, c.name+" "+c.synopsis, c.summary))
	}
	return lines
}
