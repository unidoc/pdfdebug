# T5: Low-tier cleanup batch

## Summary

Pure cleanup pass: naming clarifications, dead-code removal, CLI refactor, documentation fixes, and forward-compat polish. No behavior changes. Existing test suite must remain green; no new tests required.

## Scope

Findings from `CODE_REVIEW.md`:

- **#21** — `objectRefFromNodeID` uses `parentID` as a variable name where the value is actually the generation number. Rename to `genStr` / `numStr` for clarity. Same in `parseNodeID` callers.
- **#22** — `formatIntWithCommas` has a dead-code inner branch: `if len(digits) > first` is always true after the `len(digits) <= 3` early-return guard. Simplify.
- **#23** — `cmd/cli` re-creates `Inspector` and replicates the warning/handler pattern in `runTreeDump`, `runObjectDump`, `runStreamDump`. Extract a small helper.
- **#26** — `cli` exit codes mix 1 (usage) and 2 (runtime); panic recovery also uses 2. Per Unix convention 1 is generic failure, 2 is misuse. Document the mapping in `cmd/cli/doc.go`.
- **#27** — `splash.RenderVersion` uses `strings.ReplaceAll` for single-substitution templating. If any further substitution is added later, this pattern won't compose. Switch to `text/template` for forward-compat.
- **#29** — `appendWarning` lives at file scope in `image.go` and is used nowhere else. Confirm unexported, leave inline, or move to a shared utility if pattern recurs. (Low-priority audit.)

## Acceptance criteria

- [ ] All existing tests pass; no new behavior tests required (cleanup-only).
- [ ] `go vet`, `golangci-lint`, `tsc --noEmit`, and ESLint produce no new warnings.
- [ ] `objectRefFromNodeID` and adjacent `parseNodeID` call sites use unambiguous variable names.
- [ ] `formatIntWithCommas` simplified; behavior identical (table-driven test).
- [ ] `cmd/cli` shares one Open/warning/error handler across all three commands.
- [ ] `cli` exit-code mapping documented in `cmd/cli/doc.go`.
- [ ] Splash renderer switched to `text/template`.

## Verification harness

- `go test ./...`
- `npm run test --prefix frontend`
- Lint suite clean

## Dependencies

Lands last. Touches files that T1-T4 also touch; deferring this avoids merge conflicts and keeps `git bisect` clean (behavior changes vs. cleanup must not interleave in history).

## Notes

- Single sweep PR; reviewer/verifier scans for "did anything actually change semantically" — answer must be no.
- This is the only ticket where green CI alone is sufficient verification.
- #24 was originally in this tier — promoted to T1 as a Sev-1.
