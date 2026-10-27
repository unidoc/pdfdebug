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

`pdfdebug dump signatures [--json] [--pretty] [--export-cms FILE] [--export-certs DIR] [--field NAME] <file>`

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

Each certificate in the chain gets five rows under its `cert N:` line:
`Key usage` (RFC 5280 bit names such as `digitalSignature, nonRepudiation`),
`Ext key usage` (`emailProtection (1.3.6.1.5.5.7.3.4)`, or the bare OID when
it has no well-known name), `CRL DP`, `OCSP` and `CA Issuers`. A row reads
`absent` when the certificate has no such extension, and `present, no URI`
(or `present, no bits set` for key usage) when the extension is there but has
nothing to list, such as a CRL distribution point given only as a directory
name. In `--json` each certificate, and `signer`, carries `keyUsage`,
`extKeyUsage` (`[{"oid": ..., "name": ...}]`, `name` empty for an unknown OID),
`crlDistributionPoints`, `ocspServers` and `caIssuers`: `null` when the
extension is absent, `[]` when it is present and empty. The URLs are printed
as found; pdfdebug never fetches a CRL, an OCSP response or an issuer
certificate. Only URI names are listed, and in a CRL distribution point the
list stops at the first name that is not a URI. The Signatures panel shows the
same rows in the expanded certificate chain.

`--export-cms FILE` writes the signature's CMS to FILE: the decoded
`/Contents` bytes from the start through the end of the CMS element, in the
encoding found, so a BER signature stays BER. The zero padding after the
element is not written, and neither are non-zero trailing bytes (those are
reported as `Trailing bytes:`). When no complete element can be read from
`/Contents`, every decoded byte is written, padding included, and stderr gets
a JSON warning such as
`{"warning":"Sig1: CMS element could not be delimited (...); wrote all 3072 /Contents bytes including padding"}`;
the exit code is still 0. Read the file with
`openssl cms -cmsout -print -inform DER -in sig.p7s` or
`openssl asn1parse -inform DER -i -in sig.p7s`. A document with more than one
CMS signature needs `--field`. An `adbe.x509.rsa_sha1` signature has no CMS
to export.

`--export-certs DIR` writes every certificate of every signed field as DER, one
file each, named `sig<N>-cert<M>.der`: N is the field's position in the
listing (unsigned fields count) and M the certificate's position in its
`certificates` list. `openssl x509 -inform DER -in sig1-cert2.der -noout -text`
reads them. For a CMS signature the bytes are the certificate as parsed, which
is the embedded certificate except for one that was itself BER-encoded, which
is written in its DER form. An `adbe.x509.rsa_sha1` signature gives its
`/Cert` certificates. DIR is created when missing; files of the same name are
overwritten and other files are left alone.

`--field NAME` picks one signature field for either export by its fully
qualified name (`fieldName` in `--json`). An unknown name, a name shared by
several fields and an unsigned field are errors, and a field with an empty
name cannot be picked. `--field` without an export flag, or with an empty
name, is a usage error.

Both export flags can be given together, and FILE can sit inside DIR. Every
selection is checked before any file is written. Then DIR is created, and
every path is checked: an export that would overwrite the input PDF, or a
`--export-cms` FILE that is DIR itself or one of the `--export-certs` files,
is an error. A directory reached through a link counts as the
directory it points to, and when neither file exists yet, names in the same
directory that differ only in case count as the same file. Then the CMS is
written, then the certificates. stdout is exactly what the command prints
without the export flags. A selection or write failure exits 2 with nothing on stdout; files
already written by that run stay.
