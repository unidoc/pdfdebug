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

Flags go before the file, not after it. Argument parsing stops at the first
non-flag argument, so `dump tree file.pdf --json` would leave `--json` unparsed;
the command rejects it with the usage line rather than quietly printing plain
text. That covers a required selector too: `dump page file.pdf --info 1` is the
same shape error, reported the same way, not a missing `--info`. A flag value
the command would also reject changes nothing: `dump tree --page 0 file.pdf
--json` draws the usage line, not the out-of-range `--page`. Every command takes
one file except `diff`, which takes two.

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
| `dump bytes` | Raw document bytes, not extracted page text | - |
| `dump embedded` | Embedded/associated files; extracts one's bytes to stdout | `--ref`/`--name` |
| `dump metadata` | The `/Info` dictionary fields and the XMP packet | - |
| `dump signatures` | Digital-signature decomposition (signer, chain, ByteRange coverage; no trust verdict) | - |
| `validate` | Bounded structural conformance checks; returns a three-way exit status (0 = ran, clean; 1 = ran, errors found; 2 = operational error) | `--profile` |
| `diff` | Path-aligned structural diff of two PDFs; returns a three-way exit status (0 = identical; 1 = differ; 2 = operational error) | `--full` |

`dump bytes` was called `dump plaintext`. The old spelling still works, prints
a deprecation notice on stderr, and is removed in 0.6.0. It dumps the document's
raw bytes; for the prose on the pages, use `pdftotext`.

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

## Update notice

When a newer release exists, `pdfdebug` says so once, at the end of a
command's output, in a small box on stderr:

```
+-----------------------------------------------+
|  Update available: 0.4.0 -> 0.5.0             |
|  https://github.com/unidoc/pdfdebug/releases  |
+-----------------------------------------------+
```

It appears once per new version, plus one reminder a week later, and then not
again until a newer version comes out. If the terminal is narrower than the box,
the same two lines print without the frame. It never goes to stdout and never
changes an exit code.

The notice only appears in an interactive session. Both stdout and stderr have
to be terminals, so piping to a pager (`| less`) or redirecting to a file
(`> out.txt`) turns it off. It is also off when `CI` is set, under `--json`,
`--ops` and `--raw`, for the raw bytes of `dump bytes`, and when `dump embedded
--ref` or `--name` writes an attachment to stdout.

To turn it off entirely, set `PDFDEBUG_NO_UPDATE_CHECK` or `NO_UPDATE_NOTIFIER`.
Presence is what counts: `PDFDEBUG_NO_UPDATE_CHECK=0` and `CI=false` still turn
it off. The desktop app's "check automatically" preference lives in the app and
does not affect the CLI.

Ordinary commands read the answer from a cache. When the cache is missing or
more than a day old and the session is interactive, a command fetches one page
of releases alongside its own work and, once its output is done, waits for that
fetch for at most 1.5 seconds from the start of the run; a failed fetch is
silent and is not retried for another day.

`--version` is the one command that checks live. It prints the box before the
version line when an update exists, and otherwise says on stderr that no newer
release is available or that the check failed. It skips the check on
development builds and when either opt-out variable is set, and it ignores the
terminal and `CI` rules. The check is recorded in the cache before the request
is sent, so when the cache directory cannot be written `--version` makes no
request and reports that the check failed.

The cache is shared with the desktop app, so a check made by either one serves
both:

- macOS: `~/Library/Caches/pdfdebug/updatecheck.json`
- Linux: `~/.cache/pdfdebug/updatecheck.json`
- Windows: `%LOCALAPPDATA%\cache\pdfdebug\updatecheck.json`

An absolute `XDG_CACHE_HOME` replaces the base directory on all three; a
relative one is ignored. On macOS an `XDG_CACHE_HOME` exported in a shell
profile reaches the CLI but not a desktop app launched from Finder or the Dock,
so the two then keep separate caches. The once-per-version state sits next to
the cache in `updatenotice.json`.

## Example

```bash
pdfdebug dump tree testdata/minimal.pdf
```

Output:

```
Catalog Catalog
  Pages (2 0 R) Pages
    Count number = 1
    Kids
      Page (3 0 R) Page
        MediaBox
          ...
```

A dictionary entry holding a scalar shows its value after the type, decoded for
text strings. Array elements keep their value in place of a label, so an element
row reads `0 number` rather than repeating it. A value longer than 80 runes is
cut and marked `[truncated: N of M]`; `--json` carries it in full.

Add `--json` to any command to get the same information as parseable JSON.
