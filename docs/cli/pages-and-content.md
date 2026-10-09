# Pages and content commands

The commands that read pages and what is drawn on them: the page index,
per-page render info, decoded content streams and the document's raw bytes.

Back to the [CLI usage guide](../cli-usage.md).

### `dump pages`

`pdfdebug dump pages [--json] [--pretty] <file>`

Prints one row per page: its number, the `/Page` object's reference, the
MediaBox, `/Rotate` as stored, which of `/Resources`, `/MediaBox`, `/CropBox`
and `/Rotate` came from a `/Pages` ancestor, the `/Annots` count, and the
summed `/Length` of its content streams (read from the stream dictionaries,
never decoded; -1 when a `/Length` is missing, negative or not an integer, when
the sum overflows, or when `/Contents` is malformed). Anything in the page tree
that is not a page, such as a null `/Kids` entry or a cycle, gets a row with `-`
for the page number and a message in ERROR, so page numbers after it still
match what a viewer shows. When the number of page leaves differs from the root
`/Count`, a JSON warning naming both goes to stderr and the command still exits
0.

### `dump page`

`pdfdebug dump page --info N [--json] [--pretty] [--forms-recursive [--forms-depth D]] [--section geometry|extgstates|xobjects|forms] <file>`

Assembles what page N's rendering depends on: geometry, graphics states,
XObjects and forms. `--section` prints one part, and `--forms-recursive` walks
nested Form XObjects, `--forms-depth` levels deep (2 by default).
EXPERIMENTAL; see [Machine output](scripting.md#machine-output).

### `dump stream`

`pdfdebug dump stream [--json|--raw|--ops] [--pretty] (--page N | --ref "N G R" | --xobject NAME (--page N | --ref "N G R")) <file>`

Decodes a page's content stream (`--page`), any stream object (`--ref`), or a
named XObject from the resources of a page or object (`--xobject`). Plain
output is an operator listing; `--raw` writes the decoded bytes and `--ops`
one JSON object per operator.

### `dump bytes`

`pdfdebug dump bytes [--json] [--pretty] <file>`

Dumps the document's raw bytes, not the text on its pages; for page prose, use
`pdftotext`. `--json` wraps the decoded text.

`dump bytes` was called `dump plaintext`. The old spelling still works, prints
a deprecation notice on stderr, and is removed in 0.6.0.
