# Senior Fullstack Code Review

Scope: ~15.5k LOC across Go backend (`pdfcore`, `pdfservice`, `splash`, CLI), TS/JSX frontend (React + Wails v3), and the main Wails entry. Findings ranked by severity. Production files read end-to-end; tests skimmed.

## High - correctness / safety

### 1. `pdfcpu` is not concurrent-read-safe; the inspector has no per-doc lock around it
Every `Inspector` method calls `doc.PDFContext.Dereference` / `Catalog` / `PageDict` outside any mutex. The Wails service routes one binding call per goroutine, so two concurrent calls against the same tab (e.g. tree expansion + reverse-refs + xref table on first selection) race on `XRefTable`. pdfcpu mutates internal state on `Dereference` (object-stream resolution caches entries). `internal/pdfcore/inspector.go:204`, every `GetX` method.
Fix: add a `pdfMu sync.Mutex` on `DocumentState` and wrap each external `pdfservice` entry point.

### 2. Inspector mutex held during `GetDocument` is acquire-only, so `Close` racing with any other method can hand back a freed pointer
`Inspector.GetDocument` returns the pointer, releases `ins.mu`, then the caller dereferences `doc.PDFContext`. If `Close(tabID)` runs in between, the pointer is still valid (Go has no free), but a re-`Open` under the same tabID will replace the entry, and the caller will operate on a *different* document than `tabID` resolves to. No data corruption, but you can return data for the wrong doc on the wire.
Fix: have callers re-check liveness after long ops, or hold a read lock on the inspector for the duration.

### 3. `extractStreamInfo.Length` is wrong for the unwrapped `StreamDict` branch
`internal/pdfcore/inspector.go:354`: when the caller passes a `StreamDict`, `sd` is the *copy* assigned from `v` (the original `obj`). `sd.StreamLength` reflects parsed `/Length`. But the function falls back to 0 when `StreamLength` is nil even though the dict's `/Length` entry may still be present as an `Integer`/`IndirectRef` (this is what `streamLengthForSource` does in `objectsource.go`). Inconsistent across the two readers. Prefer the dict-fallback path in both.

### 4. `objectindex.go` BFS uses a `maxRefDepth=32` depth cap, but `buildReverseRefs` uses visited-only (no cap)
Two different reachability definitions in the same package. Documents legitimately nest more than 32 levels (deep outline trees, complex form/AcroForm graphs, page-tree leaves in multi-thousand-page books). At 32 hops the palette will silently flag valid objects as orphans. The reverse-refs builder gets it right (visited set). Drop the cap in `buildReachableSet` and rely on visited.

### 5. CRLF line numbers in `tokenizeContentStream` are computed correctly, but the *raw* `ContentStreamViewer` splits on `\n` only
`frontend/src/components/ContentStreamViewer.tsx:222`: `raw.split('\n')` collapses `\r`-only line endings into one giant line, and on CRLF leaves stray `\r` at end of each row (renders as a control glyph). Backend `Token.Line` counts lines correctly using CR/LF/CRLF rules, so the gutter and the rendered content can disagree. Fix: split on `/\r\n?|\n/` (same as `PlainTextView`).

### 6. `latin1Decode` replaces 0x7F-0x9F with U+FFFD but the comment claims only 0x7F
`internal/pdfcore/plaintext.go:230`: the predicate is `c >= 0x20 && c != 0x7F`, so 0x80-0xFF *are* mapped to their Unicode codepoints (correct Latin-1). The earlier comment "0x7F (DEL) is also replaced" is right; the doc string above says "everything else under 0x20 is replaced" which is also right. But a careful reader will notice C1 control bytes (0x80-0x9F) get rendered as their Unicode form too. That's the Latin-1 spec, but it produces invisible/weird glyphs in the Plain Text view. If you wanted "no C1 controls visible", you'd need to extend the replacement range. Confirm intent.

### 7. `useFindBar`: `setActiveIndex`/`setWrapped` called during render path
`frontend/src/hooks/useFindBar.ts:132-159`: setState during render is allowed only when wrapped in a guarded conditional that converges, and it's brittle. The `prevDepsRef` mutation also happens during render. This works today but trips React's strict-mode double-render detection in some configurations and is hard to maintain. Move to `useLayoutEffect` keyed on `[deferredQuery, caseSensitive, matches]`.

### 8. `findMatches` rebuilds the line-start table internally; the caller also memoizes one
`frontend/src/lib/findMatches.ts:93` calls `buildLineStartOffsets(content)` again, throwing away the memoized version `useFindBar` already passes via `lineStartOffsets`. On a multi-MB Plain Text load this is a measurable double pass. Fix: accept an optional `lineStartOffsets?: number[]` arg.

### 9. `useWindowPersistence` cleanup drops in-flight writes on last unmount
`frontend/src/hooks/useWindowPersistence.ts:184-192`: when `activeHookCount` reaches zero, the pending buffers are nulled *without* a final flush. In production this only matters at app close, but a quick resize-then-close loses the last geometry. Either flush synchronously on the last unmount or accept the loss explicitly.

### 10. `parseDifferences` accepts integers without bounds-checking
`internal/pdfcore/font.go:262`: `currentCode = int(v)` for any `Integer`, including negative or > 255. PDF spec 9.6.6.1 says the code is a single byte (0-255). A malformed `/Differences` with a huge integer will produce 2GB of mapping rows (capped indirectly by `maxCMapMappings` only because the test guard is in `parseToUnicodeCMap`, not here). Add a sanity bound (`if v < 0 || v > 255 { continue }`).

## Medium - bugs / risk

### 11. `EmptyState`'s drop validation only inspects the first file
`frontend/src/components/EmptyState.tsx:94`: `dataTransfer.files[0]` decides the invalid pill, but the Wails native handler in main.go processes the whole list. A drop of `[doc.txt, paper.pdf]` will flash "PDF files only" while still opening `paper.pdf`. Either iterate all files for the validation pill, or trust the backend's "unsupported N files" path and remove the per-file UI flash entirely.

### 12. `parseBfrange` integer-form increment only advances the trailing UTF-16 unit
`internal/pdfcore/font.go:776`: real CJK ToUnicode CMaps sometimes need carry into the high unit (e.g. range `<8140>` to `<81FE>` is fine but `<81FF>` rolls into `<8200>`, which this scanner refuses). It breaks the loop on overflow rather than synthesizing the carry. The TODO is at least documented; consider implementing carry or surfacing a CMap warning.

### 13. `useCommandPalette` registers Cmd+K/Ctrl+K *unconditionally* on Mac
`frontend/src/hooks/useCommandPalette.ts:69-76`: `mod = e.metaKey || e.ctrlKey` triggers on both. On macOS `Ctrl+K` is the readline kill-to-eol shortcut and users may expect it to work in inputs. The `isInTextField` check guards inputs, but the rest of the UI (e.g. CodeMirror-like areas, the find-bar) doesn't carry standard input semantics. Cleanest fix: use `getPlatformModifier()` consistently (Cmd on mac, Ctrl elsewhere) like `useFindBar` does.

### 14. Stream cache writes after a stale request can clobber a fresh entry
`internal/pdfcore/stream.go:108-145`: two concurrent calls for the same `nodeID` both pass the cache check, decode, then both `streamCache[nodeID] = result`. Same with `xrefTableCache`, `objectIndexCache`, `plainTextCache` -- but those at least hold a single mutex for the whole build. The stream path drops the lock for the decode and re-locks to write. Last writer wins, so usually OK, but a transient bad-decode wrapped result could overwrite a good one or vice versa. Fix: `streamMu` should cover the decode path (or use a per-key sync.Once).

### 15. `Inspector.Open` silently leaks the prior document on tabID collision
`internal/pdfcore/inspector.go:160-161`: `delete(ins.documents, tabID)` is called but the previous `DocumentState` is not `Close`d, so any in-flight `plainTextLoadCancel` stays alive, its file handle stays open, and its caches stay allocated until GC reaps the object. Generally a tab opens a fresh UUID so this code path is rare, but the dedup logic in `App.jsx` shouldn't even need this -- better to error out (or call `Close` on the old state).

### 16. `formatBytes` MB rounding inconsistency
`frontend/src/components/PlainTextView.tsx:55-68`: KB shows 1 decimal, MB shows `Math.round` (no decimals), GB shows 2 decimals. Surprising jump. Pick a consistent precision (e.g. 1 decimal across the board).

### 17. `splash` HTML `setInterval` for runtime-ready ping has a 5s ceiling, but doesn't tell anyone if it fails
`internal/splash/splash.go:404-415`: after 50 tries it clears the interval silently. If `_wails.invoke` never appears, the dismissal callbacks never reach the splash and the user is stuck until the 30s timeout. Cheap diagnostic: log to console (visible in WebView2 devtools) on the give-up path.

### 18. `main.go` `openFileAndEmitWithWarning` runs `pdfcpu.ReadContextFile` from the event-emit handler goroutine
On a multi-second open, the goroutine that fires `WindowFilesDropped` is the Wails dispatch goroutine; blocking it queues subsequent native events. This is mostly OK because the user is already drag-dropping (no other native activity expected), but Cmd+Q during a 30s open will appear to hang. Consider goroutine-dispatching the open + emit.

### 19. Reverse-refs index built at every `Open` even for huge PDFs
`internal/pdfcore/inspector.go:145-156`: a full-graph BFS during `Open` blocks the open. For a 200k-object book this can take seconds and there's no UI signaling. Either move it lazy (build on first `GetReverseRefs`) or gate it behind a size threshold.

### 20. `findMatches` case-insensitive search uses `toLowerCase()` on the whole corpus *per recompute*
`frontend/src/lib/findMatches.ts:91`: on a 100MB Plain Text corpus, every keystroke (debounced via `useDeferredValue`) reallocates a 100MB lowercase string. Memoize `(content, caseSensitive)` -> `haystack` at the caller, or cache it on `useFindBar`.

## Low - quality / consistency

### 21. `objectRefFromNodeID` uses a confusing variable name
`internal/pdfcore/inspector.go:556-564`: `parentID` is actually the generation number (encoded `obj:gen:num`). Rename to `genStr` and `numStr` to match what's there. Same in `parseNodeID` callers throughout.

### 22. `formatIntWithCommas` has dead-code branches
`internal/pdfcore/objectsource.go:226-232`: `if first > 0 { ... if len(digits) > first { b.WriteString(",") } ... }` -- the inner condition is always true given `first < len(digits)` from the earlier `len(digits) <= 3` guard. Simplify.

### 23. `cmd/cli` re-opens the PDF per command
`runTreeDump`, `runObjectDump`, `runStreamDump` each create a new `Inspector` and `Open`. Most one-shot tools work this way and it's fine, but the warning + handler pattern is duplicated three times. Extract a small helper.

### 24. `safeCall` re-panics on `runtime.Error`
`internal/pdfcore/errors.go:39-53`: this is *correct* for distinguishing Go bugs from PDF panics, but the comment says it's "re-panicked so it surfaces loudly". In the Wails service path, a re-panic propagates out of the binding goroutine and crashes the whole app (no top-level recover). On end-user systems that's worse than swallowing the panic. Consider adding a top-level recover in `pdfservice` that converts `runtime.Error` to an error too, with telemetry/logging.

### 25. `Inspector.GetPlainText` re-`Stat`s the same file the Inspector already opened
`internal/pdfcore/plaintext.go:140-144` and `GetPlainTextSize` both `os.Stat(doc.FilePath)` redundantly; the file size was already captured at `Open` and could live on `DocumentState`.

### 26. `cli` exit codes mix 1 (usage) and 2 (runtime), but `internal error` panic recoveries also use 2
Per Unix convention 2 is typically misuse, 1 is generic failure, >2 application-specific. Document the mapping in `cmd/cli/doc.go`.

### 27. `splash.RenderVersion` HTML escapes the version, but the splash HTML template is a raw Go string with no other dynamic input
`internal/splash/splash.go:436-438`: the `strings.ReplaceAll` approach is fine, but if any other substitution is added later the same single-replace pattern won't compose. Use `text/template` for forward-compat.

### 28. Many components keep an in-render ref-mirror pattern (`xxxRef.current = xxx`) for every state value used in callbacks
`useFindBar`, `TreePanel`, `PlainTextView` all do this. It works but is brittle and the boilerplate adds up. A single `useLatest(value)` hook would consolidate. Optional.

### 29. `appendWarning` is used only in `image.go` for image metadata extraction
`internal/pdfcore/image.go:319-323`: lives at file scope, used internally. Could be unexported `appendWarning` (it already is -- lowercase). Actually fine; just noting nothing else uses it.

### 30. `frontend/src/components/DetailPanel.tsx`: `setLoading` is declared with the value discarded
`const [, setLoading] = useState(false)` and elsewhere `setFontLoading`. The state is set but never read. Either remove the `useState` entirely, or read the value somewhere. The comments call it intentional for "dev tooling can grep both" -- that's a weak justification; delete.

## Architectural notes

- The `pdfservice` package is a thin proxy over `Inspector`. Consider whether you actually need the package boundary. `Inspector` itself could be the Wails service. Smaller surface area.
- The Go side does most of the heavy lifting; the React side has more state than it should (per-tab caches, navigation history, batch progress, palette state, find-bar state across hooks). Consider whether `useReducer` over a single store wouldn't be cleaner. The current sprawl makes test scaffolding heavy (1453-line `useDocumentState.test.tsx`).

## Top priorities

Most impactful to fix first:

1. (1) pdfcpu concurrent-read safety
2. (4) reachable-set depth cap
3. (5) raw stream split
4. (7) setState-during-render in `useFindBar`
5. (15) tabID-collision leak
