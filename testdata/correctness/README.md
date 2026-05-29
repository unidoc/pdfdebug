# Correctness Fixture Corpus

Hand-authored uncompressed PDFs, each demonstrating exactly one parser /
data-extraction bug fixed by Story 10.6. Committed as `.pdf` so the failure
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
length during `ReadContextFile`). The fallback path added by AC3 still pins
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
Unicode codepoints U+0080..U+009F. AC4 keeps the implementation, rewrites
the doc comment, and pins the byte-for-codepoint contract via a unit test on
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
