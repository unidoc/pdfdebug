# Document data commands

The commands for what a document carries besides its pages: attachments,
metadata and signatures. The flags shared across all commands are defined here
too.

Back to the [CLI usage guide](../cli-usage.md).

### `dump embedded`

`pdfdebug dump embedded [--json] [--ref "N G R" | --name NAME] <file>`

Lists attachments and associated files with their name, relationship, MIME
type, size and references. `--ref` or `--name` writes one file's raw bytes to
stdout instead; `--name` fails when no file, or more than one, has that name,
or the one that does has no `/EmbeddedFile` stream.

### `dump metadata`

`pdfdebug dump metadata [--json] [--pretty] <file>`

Prints the `/Info` dictionary fields and the XMP packet.

### `dump signatures`

`pdfdebug dump signatures [--json] [--pretty] <file>`

Decomposes each signature field: signer, certificate chain and ByteRange
coverage. It never says whether a signature is trusted or valid.

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
- `--profile` - for `validate`, select the profile.
- `--full` - for `diff`, include unchanged nodes in the output.
- `--help` / `-h` - show usage. Every command also accepts `--help`.
- `--version` / `-v` - show version information.
