# Structure commands

The commands that walk the object model: the tree, single objects, the object
index, an object's source, inbound references and the cross-reference table.

Back to the [CLI usage guide](../cli-usage.md).

### `dump tree`

`pdfdebug dump tree [--json] [--pretty] [--depth N] [--page N] [--resolve [--resolve-depth N]] <file>`

Prints the object tree from the Catalog down, or from page N's dictionary with
`--page`. `--depth` limits how many levels are walked.

A dictionary entry holding a scalar shows its value after the type
(`Count number = 1`), decoded for text strings. Array elements keep their value
in place of a label, so an element row reads `0 number` rather than repeating
it. A value longer than 80 runes is cut and marked `[truncated: N of M]`, and
control characters are escaped so a node is always one line; `--json` carries
the value in full.

### `dump object`

`pdfdebug dump object [--json] [--pretty] [--resolve [--resolve-depth N]] --ref "N G R" <file>`

Prints one indirect object's type and properties. Text strings are decoded and
printed without delimiters (`/Lang: en-US`), with the same 80-rune cap as
`dump tree`. Signature `/Contents` and `/Cert` and filespec `/Params /CheckSum`
are summarized as `<binary, N bytes>`; the bytes are in `dump source` and the
`raw` field of `--json`.

### `dump objects`

`pdfdebug dump objects [--json] [--pretty] <file>`

Lists every object in the document with its type and whether it is free
and reachable.

### `dump source`

`pdfdebug dump source [--json] [--pretty] [--raw] --ref "N G R" <file>`

Reserializes one object as PDF syntax. `--raw` writes the source bytes as they
are, with no wrapping.

### `dump reverserefs`

`pdfdebug dump reverserefs [--json] [--pretty] --ref "N G R" <file>`

Lists the objects that point at the given one.

### `dump xref`

`pdfdebug dump xref [--json] [--pretty] <file>`

Prints the cross-reference table.
