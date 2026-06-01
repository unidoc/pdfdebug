# T1: Inspector concurrency, lifecycle safety, and safeCall re-panic (Sev-1)

## Summary

Harden the Go backend against concurrent pdfcpu access, document-state lifecycle races, and a Sev-1 hiding in the Low tier: `safeCall` re-panics on `runtime.Error`, which propagates out of the binding goroutine and crashes the whole app on end-user systems.

## Scope

Findings from `CODE_REVIEW.md`:

- **#1** — `pdfcpu` is not concurrent-read-safe; Inspector has no per-doc lock. Add `pdfMu sync.Mutex` on `DocumentState`, wrap every external `pdfservice` entry point.
- **#2** — `Inspector.GetDocument` returns the pointer after releasing `ins.mu`; a `Close` racing with re-`Open` can hand back state for the wrong document. Re-check liveness after long ops or hold the lock for the duration.
- **#14** — Stream cache writes after a stale request can clobber a fresh entry. Cover the decode path with `streamMu` (or use per-key `sync.Once`).
- **#15** — `Inspector.Open` silently leaks the prior document on tabID collision. Call `Close` on the existing entry before replacing, or error out.
- **#18** — `main.go openFileAndEmitWithWarning` runs `pdfcpu.ReadContextFile` on the Wails dispatch goroutine, blocking native events. Dispatch the open to a worker goroutine.
- **#19** — Reverse-refs index is built eagerly at every `Open`; on huge PDFs this blocks the open with no UI signaling. Build lazily on first `GetReverseRefs` (or gate by size).
- **#24** — `safeCall` re-panics on `runtime.Error`, crashing the app. Add a top-level recover in `pdfservice` that converts to an error with logging.

## Acceptance criteria

- [ ] `go test -race ./...` passes including new concurrent-access tests for `Inspector` open/close/inspect interleavings.
- [ ] New test: concurrent `GetTreeRoot` + `GetReverseRefs` + `GetXRefTable` on the same tabID does not race.
- [ ] New test: `Inspector.Open` under tabID collision closes the prior `DocumentState` (file handle released, cancel func invoked).
- [ ] New test: `safeCall` with an injected `runtime.Error` returns an error from the `pdfservice` boundary instead of propagating a panic.
- [ ] Large-PDF open (>50k objects) does not block the event-emit goroutine; measured via a probe event arriving within 500ms of drop.
- [ ] Reverse-refs index build is deferred until first request and cached thereafter.

## Verification harness

- `go test -race -count=10 ./internal/pdfcore/... ./internal/pdfservice/...`
- New leak detector pass (goroutine count diff before/after Close)

## Dependencies

None. Lands first.

## Notes

- #24 was originally tier Low. Re-classifying as Sev-1: a panic in any pdfcpu code path crashes the user's app. Cannot ride in a cleanup PR.
- Commit per fix inside the ticket (no squash) so `git bisect` localizes regressions after merge.
