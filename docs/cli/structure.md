# Structure commands

The commands that walk the object model: the tree, single objects, the object
index, an object's source, inbound references, the cross-reference table and
the tagged-PDF structure tree.

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

### `dump structure`

`pdfdebug dump structure [--json] [--pretty] <file>`

Prints the tagged-PDF structure tree and, for each page, how its marked content
lines up with the tree. It reports what is in the file; it does not judge it.

The header gives the `/StructTreeRoot` reference, the ParentTree (or `absent`),
`/ParentTreeNextKey`, the catalog's `/MarkInfo /Marked` and `/Lang`, and the
`/RoleMap`. A file with no structure tree prints
`No structure tree (/StructTreeRoot absent)` and still gets the page section.

The element list follows `/K` from the root, one element per line with its
`/S` (and the role it maps to, in parentheses), reference, page (`(inherited)`
when it comes from an ancestor's `/Pg`), `/T`, `/Lang`, `/Alt`, `/ActualText`
and `/E`. Text longer than 80 runes is cut and marked `[truncated: N of M]`, as
in `dump tree`. Under each element are its kids: `MCID n`, `MCR MCID n page p`
(with `stream N G R` when the content is in another stream), and `OBJR N G R`
with the target's `/Type/Subtype`. Anything that cannot be read is an `error:`
line in its place, and the walk carries on. An element, or an indirect `/K`
array, reached a second time is listed once, with `already listed; not walked again` where it comes back, and a
tree deeper than 256 levels says where it stops.

Each page with a `/StructParents` entry, any marked content, an MCID the tree
attributes to it, or an error gets a block. The first line has the
`/StructParents` key, the number of content streams and the BDC/BMC and EMC
counts. A page's content streams (a `/Contents` array, direct or behind a reference)
are read as one concatenated stream, so a
sequence opened in one stream may close in a later one and a BDC's operands may
start in one stream and end in the next. A stream that fails to decode or
closes a sequence it never opened gets its own line, and so does the page's
last stream when sequences are still open at its end. Each BDC and each
ParentTree slot that could not be read gets a line too. A BDC's tag and
property list are its last two operands; anything before them is ignored. An
inline property list with no closing `>>` gets a line as well, but its MCID is
still read and counted in the table. The table has one row per MCID:

- `CONTENT` - how many BDCs in the page's content carry that MCID.
- `PARENTTREE` - the element in that slot of the page's ParentTree array.
- `TREE` - every element whose `/K` names that MCID on that page.

A row is marked `*` unless the content marks the MCID once, the ParentTree slot
names an element, and that element is the only one in `TREE`. The pages that get
no block are collapsed into one line. ParentTree keys
no page names (an annotation's `/StructParent`, or an orphan) are listed under
"Other ParentTree keys". A key whose value is an array is listed with a
`note:` rather than an error, since a Form XObject with marked content has its
own `/StructParents` and an array value there. "Problems" lists malformed
ParentTree nodes and pages that share a `/StructParents` key with an earlier
page.

MCIDs inside Form XObjects are not aligned: an MCR with `/Stm` is listed in the
tree but left out of the page table, and `Do` is not followed.

`--json` carries the whole report, uncapped. One alignment row:

```json
{"mcid":0,"contentCount":2,"parentTreeElement":"13 0 R","parentTreeS":"P","treeElements":["13 0 R"],"consistent":false}
```
