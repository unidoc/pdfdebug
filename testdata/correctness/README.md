# Correctness Fixture Corpus

Hand-authored uncompressed PDFs, each demonstrating exactly one parser /
data-extraction bug. Committed as `.pdf` so the failure
mode stays explicit and reviewable; the generator script is **not** committed
(an engineer can regenerate any one of these by hand using the byte-level
description below).

All fixtures use:

- PDF 1.4 header.
- Plain `xref` table (no XRefStream).
- Uncompressed content streams (no Flate / LZW filters).
- No encryption.

## deep-nesting.pdf

Page tree with a chain >50 levels deep:

```
obj 1: /Type /Catalog /Pages 2 0 R
obj 2: /Type /Pages /Kids [3 0 R] /Count 1            (top of chain)
obj 3: /Type /Pages /Parent 2 0 R /Kids [4 0 R] /Count 1
...
obj 52: /Type /Pages /Parent 51 0 R /Kids [53 0 R] /Count 1
obj 53: /Type /Page  /Parent 52 0 R /MediaBox [0 0 612 792]   (leaf)
```

Pre-fix `buildReachableSet` BFS at depth 32 caps the walk, marking obj 34..53
as orphan trees. Post-fix every object is reachable. Same applies to
`findPathToObject`. 53 objects total.

## stream-length-indirect.pdf

Single-page PDF whose content stream's `/Length` is an indirect reference:

```
obj 4: << /Length 5 0 R >> stream ... endstream     (the content stream)
obj 5: 37                                            (the resolved length)
```

Note: pdfcpu's reader observed during this story does populate
`StreamDict.StreamLength` for this fixture (it dereferences the indirect
length during `ReadContextFile`). The fallback path still pins
the contract for any malformed PDF where pdfcpu leaves `StreamLength == nil`
but the dict carries `/Length`. The unit test
`TestExtractStreamInfoIndirectLength` exercises that fallback path with a
synthesized `StreamDict` directly; the fixture exists for end-to-end happy-
path coverage.

## latin1-c1.pdf

Single-page PDF whose content stream is the 32 contiguous C1-control bytes
0x80..0x9F:

```
obj 4: << /Length 32 >> stream
\x80\x81\x82...\x9F
endstream
```

Pre-fix `latin1Decode` was documented as replacing "everything else under
0x20" but the implementation correctly mapped 0x80..0x9F verbatim to their
Unicode codepoints U+0080..U+009F. The implementation stands, the doc comment
was corrected, and the byte-for-codepoint contract is pinned by a unit test on
the full 0x00..0xFF range.

## diff-out-of-range.pdf

Single-page PDF with a Type1 font whose `/Encoding` dict carries a
`/Differences` array containing out-of-range integers:

```
obj 4: /Type /Font /Subtype /Type1 /BaseFont /Helvetica
       /Encoding << /Type /Encoding /BaseEncoding /WinAnsiEncoding
         /Differences [-1 /a 999 /b 32 /space] >>
```

Pre-fix `parseDifferences` appended `/a` at code -1 and `/b` at code 999.
Post-fix both entries are skipped silently; only the `/space` entry at code
32 (and the implicit code 33 if a name follows) populates the encoding table.

## leading-plus.pdf

Single-page PDF whose content stream carries leading-`+` signed operands
(F1):

```
obj 4: << /Length 16 >> stream
+5 0 0 +5 0 0 cm
endstream
```

ISO 32000-1 7.3.3 permits a leading `+` on integers and reals. Pre-fix
`isNumberStart` (and the number-consumption loop) accept a leading `-` but not
a leading `+`, so each `+5` falls through to the word/operator branch and is
mis-emitted as an OPERATOR token; `dump stream --json` / `--ops` report a bogus
operator `+5` and `Format()` mis-groups the transform. Post-fix each `+5` is a
`number` token and `cm` is the sole operator for the line. 4 objects total.

## comment-and-dangling.pdf

Single-page PDF whose content stream contains a `%` comment line and a trailing
dangling operand run with no terminating operator (F2):

```
obj 4: << /Length 40 >> stream
% leading comment
1 0 0 1 10 20 cm
30 40
endstream
```

`Format()` emits the comment line and the trailing `30 40` run as
`FormattedLine`s with `Operator == ""`. Pre-fix `emitOps` iterates `Formatted`
with no guard, so `dump stream --ops` emits two phantom `{"op":"","params":...}`
NDJSON records, breaching the documented one-object-per-operator contract.
Post-fix `--ops` skips empty-operator lines and emits only the single `cm`
record. `--json` is unaffected (it serializes the full formatted slice,
comments included). 4 objects total.

## cjk-cmap-carry.pdf

Type0 composite font referencing a ToUnicode CMap with a bfrange whose
trailing UTF-16 unit advances across the 0xFF boundary, requiring carry
propagation into the leading unit:

```
obj 6 (ToUnicode stream):
  1 begincodespacerange
  <00> <FF>
  endcodespacerange
  1 beginbfrange
  <00> <03> <00FFFFFE>
  endbfrange
```

The base `<00FFFFFE>` is two UTF-16 units `[0x00FF, 0xFFFE]`. For code 0 the
mapping is `[00FF FFFE]` = `U+00FF U+FFFE`. For code 2 the trailing unit
overflows: `0xFFFE + 2 = 0x10000` -> trailing = `0x0000`, carry 1 into the
leading unit -> `0x00FF + 1 = 0x0100`. Result: `[0100 0000]` = `U+0100
U+0000`. Pre-fix this entry was silently dropped (the `break` on `tail >
0xFFFF`); post-fix the carry propagates correctly.

## deep-change-a.pdf / deep-change-b.pdf

A pair of single-page PDFs that are byte-identical EXCEPT one scalar nested far
below the catalog, used to make the `diff` depth cap honest.

```
obj 1: /Type /Catalog /Pages 2 0 R /Deep 4 0 R
obj 2: /Type /Pages /Kids [3 0 R] /Count 1
obj 3: /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]
obj 4..47: << /L (n+1) 0 R >>          (a linear chain of nested dicts)
obj 48: << /V 111 >>   (deep-change-a) | << /V 222 >>   (deep-change-b)
```

The catalog is diff depth 0; `diffChild` increments before the depth check, so
the depth-32 cap first cuts at catalog-depth 33 -- the `/Root/Deep/L/.../L` node
(32 `/L` steps). At that cut the one-level shallow summary is `<< /L <ref> >>`,
identical on both sides (refs render number-independent), so the differing
scalar at obj 48 (catalog-depth ~45, well below the cut) is never reached.

Pre-fix `diff deep-change-a deep-change-b` reports "Documents are structurally
identical." at exit 0 -- an inverted answer. Post-fix the cut node is marked
`truncated`, `DiffSummary.truncatedSubtrees > 0`, the run is not called
identical, and the exit code is 1. The `/V` value is a fixed 3-digit token so
the two files share identical byte lengths and xref offsets. 48 objects each.

## multi-content-stream.pdf

Single-page PDF whose `/Contents` is an ARRAY of two content-stream refs whose
operators only balance when concatenated:

```
obj 3: /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]
       /Resources << >> /Contents [4 0 R 5 0 R]
obj 4 (stream 1): q
                  1 0 0 1 50 700 cm          (opens a q, no matching Q)
obj 5 (stream 2): BT
                  /F1 24 Tf
                  0 0 Td
                  (Hello) Tj
                  ET
                  Q                           (the matching Q + a text block)
```

Per ISO 32000-1 7.8.2 the page content is the concatenation of both streams
joined by whitespace. Pre-fix `GetPageContentStreamNodeID` returns only the
first ref's node ID, so `dump stream --page 1` decodes ONLY stream 1 (`q`, `cm`)
and presents an unbalanced partial program. Post-fix `dump stream --page 1`
concatenates ALL of the page's content streams (via `GetPageContentStream`),
newline-joined, and tokenizes them as one program - operators from both streams
appear on `--json`, `--ops`, plain text, and `--raw`. 5 objects total.

**`--raw` contract change:** for a multi-stream page, `--raw` now emits every
stream's decoded bytes joined by a single injected `\n` (the whole page content
per 7.8.2), not one stream verbatim. A script that diffed `--raw` output against
one extracted stream's bytes must instead compare against the concatenation.

**Still not handled (visible/deferred, not silent):**
- `/Contents` given as an indirect ref to an array (`/Contents 6 0 R` where obj6
  is `[4 0 R 5 0 R]`): errors visibly (`node is not a stream object`, exit 2)
  rather than concatenating - pdfcpu does not pre-dereference `/Contents`.
- GUI "Go to Page" still lands on the first content stream alone: `GoToPage`
  returns `GetPageContentStreamNodeID` (a single tree node) and `DetailPanel`
  fetches it via `GetContentStream`. `GetPageContentStream` is not yet bound
  into `pdfservice`, so the concatenation fix is CLI-only for now.

## text-string-encoding.pdf

PDF text strings (ISO 32000-1 7.9.2.2) carried as UTF-16BE-with-BOM hex
literals in the two reader paths that go through the shared text-string
decoder, alongside the binary and ASCII control values that must stay raw.

```
obj 1: /Type /Catalog /Pages 2 0 R /AF [6 0 R 8 0 R]
       /Names << /EmbeddedFiles 9 0 R >>
obj 2: /Type /Pages /Kids [3 0 R] /Count 1
obj 3: /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]
obj 4: /Type /EmbeddedFile /Subtype /text#2Fxml /Length 70
       /Params << /Size 70
                  /CheckSum <DEADBEEFCAFEF00D0011223344556677>
                  /ModDate (D:20240101000000Z) >>
obj 5: /Title <FEFF0052006500630068006E0075006E006700200047007200F600DF
               006500204E2D6587>
       /Author (ACME GmbH) /Subject (Plain ASCII subject)
       /Producer (pdfdebug-fixture) /CreationDate (D:20240101000000Z)
obj 6: /Type /Filespec /F (groesse.xml)
       /UF <FEFF0067007200F600DF0065002D4E2D6587002E0078006D006C>
       /AFRelationship /Data /EF << /F 4 0 R /UF 4 0 R >>
obj 7: /Type /EmbeddedFile /Subtype /text#2Fxml /Length 29
       /Params << /Size 29 /CheckSum <DEADBEEFCAFEF00D0011223344556677>
                  /ModDate (D:20240101000000Z) >>
       (second attachment, ASCII-named. /Params is spelled out because the
       binary-boundary test asserts the raw /CheckSum and /ModDate on BOTH
       entries.)
obj 8: /Type /Filespec /F (plain.xml) /UF (plain.xml)
       /AFRelationship /Source /EF << /F 7 0 R /UF 7 0 R >>
obj 9: /Names [(groesse.xml) 6 0 R (plain.xml) 8 0 R]
trailer: /Root 1 0 R /Info 5 0 R
```

The `/Title` hex decodes to "Rechnung Groesse <U+4E2D><U+6587>" (with `oe` =
U+00F6, `ss` = U+00DF); the `/UF` hex decodes to
"groesse-<U+4E2D><U+6587>.xml" with the same two Latin-1 letters. Neither hex
payload contains a `0x5C` byte -- pdfcpu's `HexLiteralToString` runs `Unescape`
over the already-decoded bytes and would silently swallow one.

Both string values are DIRECT (inline) objects, never indirect refs:
`collectInfoFields` and `embeddedFileFromFilespec` do not dereference, so an
indirect value would make the test pass or fail for the wrong reason.

Pre-fix both readers use raw `stringValue`, and pdfcpu stores a `HexLiteral`
as its hex DIGIT text, so `dump metadata --json` reports
`info.Title == "FEFF0052..."` and `dump embedded --json` reports
`name == "FEFF0067..."` -- a hex dump masquerading as text. Post-fix both are
correct UTF-8. The `/Params /CheckSum` hex literal and the ASCII `/ModDate`,
`/Author`, `/Subject`, `/CreationDate` and second attachment name stay
byte-identical either way: they pin the text-vs-binary boundary and the
conditional plain-text ASCII escape. 9 objects total.
