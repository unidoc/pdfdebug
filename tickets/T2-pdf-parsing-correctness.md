# T2: PDF parsing and data-correctness fixes

## Summary

Backend byte-level and spec-conformance bugs across stream length extraction, reachability traversal, Latin-1 decoding, encoding-difference bounds, ToUnicode CMap range carry, and redundant file stat. All operate on parsed PDF data and share one verification surface: a curated fixture corpus of malformed/edge-case PDFs.

## Scope

Findings from `CODE_REVIEW.md`:

- **#3** — `extractStreamInfo.Length` falls back to 0 when `sd.StreamLength` is nil even though the dict's `/Length` entry may still be present. Mirror the fallback logic in `streamLengthForSource` (`objectsource.go`).
- **#4** — `objectindex.go buildReachableSet` uses a `maxRefDepth=32` cap; `buildReverseRefs` uses visited-only. Deep outline trees and large page trees legitimately exceed 32 hops. Drop the cap, rely on visited.
- **#6** — `latin1Decode` doc comment says only 0x7F is replaced, but C1 controls (0x80-0x9F) render as their Unicode form (invisible/weird glyphs). Either extend the replacement range to cover C1, or fix the comment to reflect actual behavior. Decision required.
- **#10** — `parseDifferences` accepts `Integer` values without bounds-checking. PDF spec 9.6.6.1 limits codes to 0-255; a malformed huge integer produces unbounded mapping rows (only indirectly capped). Add `if v < 0 || v > 255 { continue }`.
- **#12** — `parseBfrange` integer-form increment advances only the trailing UTF-16 unit; CJK CMaps with carry into the high unit (e.g. `<81FF>` -> `<8200>`) are rejected. Implement carry or emit a CMap warning.
- **#25** — `Inspector.GetPlainText` and `GetPlainTextSize` both `os.Stat(doc.FilePath)` redundantly. Capture file size on `DocumentState` at `Open`.

## Acceptance criteria

- [ ] Fixture corpus added under `testdata/`: deep-nesting (>32 hops), CRLF-in-stream, Latin-1 C1-control, malformed `/Differences`, CJK ToUnicode range with carry, missing-`StreamLength` stream.
- [ ] Regression tests pin new behavior for each fixture.
- [ ] `buildReachableSet` no longer drops deeply-nested reachable objects.
- [ ] Stream-length extractors agree across `inspector.go` and `objectsource.go`.
- [ ] `parseDifferences` rejects out-of-range codes silently (matches scanner-tolerant style).
- [ ] `parseBfrange` either expands carry or records a `ToUnicodeError` for the unhandled range.
- [ ] `os.Stat` call count on a `GetPlainText` cycle drops to one (or zero post-Open).
- [ ] Decision recorded on #6: extend C1 replacement, or fix doc comment. Implement chosen path.

## Verification harness

- `go test ./internal/pdfcore/...` against the new fixture corpus.

## Dependencies

None. Can run in parallel with T1.

## Notes

- Build the fixture corpus once and reuse across all six fixes. Per-fix synthetic fixtures triple the maintenance cost.
