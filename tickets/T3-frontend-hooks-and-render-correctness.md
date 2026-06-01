# T3: Frontend hook and render-path correctness

## Summary

React render-path correctness across `useFindBar`, `ContentStreamViewer`, `findMatches`, and `useWindowPersistence`. Shared verification surface: Vitest + React Testing Library, with render-count and effect-firing assertions.

## Scope

Findings from `CODE_REVIEW.md`:

- **#5** — `ContentStreamViewer` splits raw stream on `\n` only. CR-only line endings collapse to one giant line; CRLF leaves stray `\r` glyphs. Fix: split on `/\r\n?|\n/` (same as `PlainTextView`).
- **#7** — `useFindBar` calls `setActiveIndex`/`setWrapped` and mutates `prevDepsRef` during render. Move to `useLayoutEffect` keyed on `[deferredQuery, caseSensitive, matches]`.
- **#8** — `findMatches` rebuilds `lineStartOffsets` internally while `useFindBar` already memoizes one. Accept an optional `lineStartOffsets?: number[]` arg.
- **#9** — `useWindowPersistence` cleanup nulls pending buffers without a final flush on last unmount. Flush synchronously on the last decrement of `activeHookCount`.
- **#20** — `findMatches` calls `content.toLowerCase()` on every recompute. On large corpora this reallocates the full lowercase string per keystroke. Memoize `(content, caseSensitive) -> haystack` on the caller.
- **#28** — Consolidate the `xxxRef.current = xxx` ref-mirror boilerplate scattered across `useFindBar`, `TreePanel`, `PlainTextView` into a single `useLatest(value)` hook.

## Acceptance criteria

- [ ] RTL test: `useFindBar` does not warn about setState-during-render under React strict mode.
- [ ] RTL test: case-insensitive find on a synthetic 10MB corpus does not lowercase the corpus more than once per `(content, caseSensitive)` pair.
- [ ] RTL test: `ContentStreamViewer` raw view renders the correct line count for CR-only, LF-only, and CRLF inputs.
- [ ] RTL test: `useWindowPersistence` last-unmount flushes pending geometry before resolving cleanup.
- [ ] `findMatches` accepts an optional `lineStartOffsets` arg and uses it when provided.
- [ ] `useLatest` hook lands; ref-mirror sites in `useFindBar`, `TreePanel`, `PlainTextView` migrated.

## Verification harness

- `npm run test --prefix frontend` (Vitest + RTL)
- `npm run typecheck --prefix frontend`
- `npm run lint --prefix frontend`

## Dependencies

None. Can run in parallel with T1 and T2.

## Notes

- #7 is the highest-risk item — touches the hot path of every find-bar keystroke. Land the red-test first.
