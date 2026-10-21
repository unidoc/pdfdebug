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
Zero padding after the CMS in /Contents is expected and not mentioned.
Non-zero bytes after it get a `Trailing bytes:` line right after
`CMS encoding:`, for example
`Trailing bytes: 4 bytes after the CMS at offset 2028 contain non-zero data`;
`--json` carries them as `cmsTrailingBytes` (`offset`, `count`) and the
Signatures panel shows the same text in a "Trailing bytes" row. Bytes after
the SignedData inside the ContentInfo get a `Note:` line with the byte count.
Either way the signature still decomposes. A CMS nested more than 64 levels
deep is not decomposed.
