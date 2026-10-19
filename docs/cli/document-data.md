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
coverage. It never says whether a signature is trusted or valid. BER-encoded
signatures (for example DocuSign's) are decoded too. Each CMS signature gets a
`CMS encoding: DER` or `CMS encoding: BER (normalized to DER for parsing)` line;
the line is absent for non-CMS SubFilters (`adbe.x509.rsa_sha1`) and when the
payload did not decode. `--json` carries this as `cmsEncoding` (`DER`, `BER`,
or empty).
Non-zero bytes after the CMS in /Contents are a decompose error, while zero
padding is accepted. A CMS element over 8 MiB, or nested more than 64 levels
deep, is not decomposed.
