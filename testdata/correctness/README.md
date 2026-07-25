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

## leading-plus.pdf

Single-page PDF whose content stream carries leading-`+` signed operands
(Story 14-1, F1):

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
dangling operand run with no terminating operator (Story 14-1, F2):

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
below the catalog, used to make the `diff` depth cap honest (Story 14-3, #2).

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
operators only balance when concatenated (Story 14-3, #5):

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
and presents an unbalanced partial program with no marker. Post-fix the tool
either concatenates both streams (operators from both appear) or emits a
machine-visible truncation marker (`streamCount`/`truncated`) on `--json` and
`--ops`. 5 objects total.
