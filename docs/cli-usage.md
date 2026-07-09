# pdfdebug CLI Usage

`pdfdebug` inspects the internal structure of a PDF from the command line: its
object tree, indirect objects, content streams, fonts, images, cross-reference
table, embedded files, metadata, and digital signatures. It also runs bounded
structural conformance checks and computes a structural diff between two PDFs.
Everything is read-only - `pdfdebug` never modifies the file you point it at.

This page documents the `pdfdebug` command-line tool, not the "UniDoc PDF
Debugger" desktop application (the GUI counterpart); for the desktop app and the
project as a whole, see the [README](../README.md).

## Shape of a command

```
pdfdebug <command> [subcommand] [flags] <file.pdf>
```

Most inspection lives under `dump`. `validate` and `diff` are top-level peers.

## Commands

| Command | What it shows | Key flags |
| --- | --- | --- |
| `dump tree` | The PDF object tree from the Catalog down | `--depth`, `--resolve` |
| `dump object` | A single indirect object by reference | `--ref`, `--resolve` |
| `dump objects` | The object index (every object in the document) | - |
| `dump stream` | A decoded content stream (page, object, or XObject) | `--page`/`--ref`/`--xobject`, `--raw`, `--ops` |
| `dump page` | Assembled per-page render info **(EXPERIMENTAL)** | `--info N`, `--section`, `--forms-recursive` |
| `dump font` | A font view (encoding, CMap, glyph mapping, health) | `--ref`, `--glyphs` |
| `dump image` | Image XObject data and metadata | `--ref`, `--metadata` |
| `dump source` | The reserialized object source (PDF syntax) | `--ref`, `--raw` |
| `dump reverserefs` | Inbound references (who points at this object) | `--ref` |
| `dump xref` | The cross-reference table | - |
| `dump plaintext` | Document bytes as decoded text | - |
| `dump embedded` | Embedded/associated files; extracts one's bytes to stdout | `--ref`/`--name` |
| `dump metadata` | The `/Info` dictionary fields and the XMP packet | - |
| `dump signatures` | Digital-signature decomposition (signer, chain, ByteRange coverage; no trust verdict) | - |
| `validate` | Bounded structural conformance checks; returns a three-way exit status (0 = ran, clean; 1 = ran, errors found; 2 = operational error) | `--profile` |
| `diff` | Path-aligned structural diff of two PDFs; returns a three-way exit status (0 = identical; 1 = differ; 2 = operational error) | `--full` |

`validate` runs structural checks only, not full conformance; for an
authoritative verdict use veraPDF. Its profiles are `pdfa-1b` (default) and
`pdfua-1-structural`.

## Flags

These recur across commands; each is defined once here.

- `--json` - emit structured JSON instead of the default plain text.
- `--pretty` - indent JSON output (no effect on plain text).
- `--depth N` - limit tree traversal to N levels (`dump tree`).
- `--resolve` - follow indirect references inline; `--resolve-depth N` bounds how deep.
- `--raw` - emit the raw stream/object bytes (a separate machine format).
- `--ops` - emit the parsed content-stream operators (`dump stream`).
- `--page N` / `--ref "N G R"` / `--xobject NAME` - select what to dump.
- `--ref` accepts two forms: `"N G R"` (e.g. `"7 0 R"`) and `obj:G:N` (e.g. `obj:0:7`).
- `--metadata` - for `dump image`, report metadata and omit the base64 payload in JSON.
- `--glyphs` - for `dump font`, print the full per-code mapping table.
- `--name NAME` - for `dump embedded`, extract the named file's bytes to stdout.
- `--profile` - for `validate`, select the conformance profile.
- `--full` - for `diff`, include unchanged nodes in the output.

## Output formats

Plain text is the default and is human-readable but non-contractual - it may
change between releases. For stable, parseable output (scripts, agents), pass
`--json`.

## Global flags

- `--help` / `-h` - show usage. Every command also accepts `--help`.
- `--version` / `-v` - show version information.

## Example

```bash
pdfdebug dump tree testdata/minimal.pdf
```

Output:

```
Catalog Catalog
  Pages (2 0 R) Pages
    Count number
    Kids
      Page (3 0 R) Page
        MediaBox
          ...
```

Add `--json` to any command to get the same information as parseable JSON.
