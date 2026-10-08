# pdfdebug CLI Usage

`pdfdebug` inspects the internal structure of a PDF from the command line: its
object tree, indirect objects, content streams, fonts, images, cross-reference
table, embedded files, metadata, and digital signatures. It also runs bounded
structural checks and computes a structural diff between two PDFs.
Everything is read-only - `pdfdebug` never modifies the file you point it at.

This page documents the `pdfdebug` command-line tool, not the "UniDoc PDF
Debugger" desktop application (the GUI counterpart); for the desktop app and the
project as a whole, see the [README](../README.md).

## Contents

- [Common tasks](#common-tasks)
- [Commands](#commands)
- [Shape of a command](#shape-of-a-command)
- [Flags](#flags)
- [Reference pages](#reference-pages)

## Common tasks

### Why a PDF fails PDF/UA

```
pdfdebug validate --profile pdfua-1-structural file.pdf
```

After the profile name and its scope sentence, the output lists the rules it
checked with `0 found` or `N found` beside each, then the problems and a
summary line. The profile checks catalog-level entries only, such as whether
the document is marked as tagged. A file that passes every one of them can
still fail PDF/UA, so a clean run is not a verdict; use veraPDF for that.
`pdfdebug --help` lists the current rules of each profile.

### Compare two versions of a file

```
pdfdebug diff old.pdf new.pdf
```

Nodes are matched by structural path (`/Root/Pages/Kids[0]`), not by object
number, so a file that was rewritten with renumbered objects still lines up.
After a summary line, each node is marked `+` added, `-` removed or `~`
changed; `--full` also prints the unchanged nodes. The command exits 1 when the
files differ.

### Pull out one image or stream

```
pdfdebug dump images file.pdf
pdfdebug dump image --metadata --ref "4 0 R" file.pdf
```

`dump images` lists every image XObject referenced from page resources, with
its reference in the REF column. Pass that reference to `dump image --metadata`
for the image dictionary and how its samples are interpreted (see
[`dump image`](cli/images-and-fonts.md#dump-image)). Without `--metadata`,
`dump image --json` also carries the image data in its `base64` field, so
`jq -r .base64 | base64 -d` writes the image (a JSON warning on stderr means
it is only the preview). For the decoded bytes of a page's
content stream, use `pdfdebug dump stream --raw --page 1 file.pdf > page1.txt`,
or `--ref` for any stream object. An attachment comes out with
`pdfdebug dump embedded --name NAME file.pdf > out`; run
`dump embedded file.pdf` first to see the names.

### Use it in CI

```
pdfdebug validate --json --profile pdfua-1-structural file.pdf > report.json
```

Branch on the exit code first and read `summary` from the JSON second, because
`pdfua-1-structural` findings are warnings and a rule that could not run is
info, and both exit 0. The full script is under
[Exit codes](cli/scripting.md#exit-codes).

## Commands

Each description links to the command's section on its reference page.

| Command | What it shows | Key flags |
| --- | --- | --- |
| `dump tree` | [The PDF object tree from the Catalog down](cli/structure.md#dump-tree) | `--depth`, `--page`, `--resolve` |
| `dump object` | [A single indirect object by reference](cli/structure.md#dump-object) | `--ref`, `--resolve` |
| `dump objects` | [The object index (every object in the document)](cli/structure.md#dump-objects) | - |
| `dump source` | [The reserialized object source (PDF syntax)](cli/structure.md#dump-source) | `--ref`, `--raw` |
| `dump reverserefs` | [Inbound references (who points at this object)](cli/structure.md#dump-reverserefs) | `--ref` |
| `dump xref` | [The cross-reference table](cli/structure.md#dump-xref) | - |
| `dump pages` | [The page index (every page leaf of the page tree, in document order)](cli/pages-and-content.md#dump-pages) | - |
| `dump page` | [Assembled per-page render info **(EXPERIMENTAL)**](cli/pages-and-content.md#dump-page) | `--info N`, `--section`, `--forms-recursive` |
| `dump stream` | [A decoded content stream (page, object, or XObject)](cli/pages-and-content.md#dump-stream) | `--page`/`--ref`/`--xobject`, `--raw`, `--ops` |
| `dump bytes` | [Raw document bytes, not extracted page text](cli/pages-and-content.md#dump-bytes) | - |
| `dump images` | [The image index (every image XObject referenced from page resources, deduplicated)](cli/images-and-fonts.md#dump-images) | - |
| `dump image` | [Image XObject data, metadata and sample interpretation](cli/images-and-fonts.md#dump-image) | `--ref`, `--metadata` |
| `dump font` | [A font view (encoding, CMap, glyph mapping, health)](cli/images-and-fonts.md#dump-font) | `--ref`, `--glyphs` |
| `dump embedded` | [Embedded/associated files; extracts one's bytes to stdout](cli/document-data.md#dump-embedded) | `--ref`/`--name` |
| `dump metadata` | [The `/Info` dictionary fields and the XMP packet](cli/document-data.md#dump-metadata) | - |
| `dump signatures` | [Digital-signature decomposition (signer, chain, ByteRange coverage; no trust verdict)](cli/document-data.md#dump-signatures) | - |
| `validate` | [A named subset of structural checks per profile, listed on every run](cli/validate-and-diff.md#validate) | `--profile` |
| `diff` | [Path-aligned structural diff of two PDFs](cli/validate-and-diff.md#diff) | `--full` |

Every command also takes `--json`, and most take `--pretty`. The flags shared
across commands are defined under [Flags](#flags).

## Shape of a command

```
pdfdebug <command> [subcommand] [flags] <file.pdf>
```

Most inspection lives under `dump`. `validate` and `diff` are top-level peers.

Flags go before the file, not after it. Argument parsing stops at the first
non-flag argument, so `dump tree file.pdf --json` would leave `--json` unparsed;
the command rejects it with the usage line rather than quietly printing plain
text. That covers a required selector too: `dump page file.pdf --info 1` is the
same shape error, reported the same way, not a missing `--info`. A flag value
the command would also reject changes nothing: `dump tree --page 0 file.pdf
--json` draws the usage line, not the out-of-range `--page`. Every command takes
one file except `diff`, which takes two.

## Flags

These recur across commands; each is defined once here, and each command's
synopsis on its reference page shows which of them it takes.

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
- `--profile` - for `validate`, select the profile.
- `--full` - for `diff`, include unchanged nodes in the output.
- `--help` / `-h` - show usage. Every command also accepts `--help`.
- `--version` / `-v` - show version information.

## Reference pages

- [Structure](cli/structure.md): `dump tree`, `dump object`, `dump objects`,
  `dump source`, `dump reverserefs` and `dump xref`.
- [Pages and content](cli/pages-and-content.md): `dump pages`, `dump page`,
  `dump stream` and `dump bytes`.
- [Images and fonts](cli/images-and-fonts.md): `dump images`, `dump image` and
  `dump font`.
- [Document data](cli/document-data.md): `dump embedded`, `dump metadata` and
  `dump signatures`.
- [Validate and diff](cli/validate-and-diff.md): `validate` and `diff`.
- [Scripting](cli/scripting.md): exit codes, a CI script, what is and is not
  machine output, and the update notice and how to turn it off.
