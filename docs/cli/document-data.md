# Document data commands

The commands for what a document carries besides its pages: attachments,
metadata and signatures.

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
