CLAUDE.md - Coding Profile
# Best for: dev projects, code review, debugging, refactoring
# Extends: Universal CLAUDE.md rules

---

## Output
- Return code first. Explanation after, only if non-obvious.
- No inline prose. Use comments sparingly - only where logic is unclear.
- No boilerplate unless explicitly requested.

## Code Rules
- Simplest working solution. No over-engineering.
- No abstractions for single-use operations.
- No speculative features or "you might also want..."
- Read the file before modifying it. Never edit blind.
- No error handling for scenarios that cannot happen.
- Three similar lines is better than a premature abstraction.

## Code Comments
- Comments say what the code does or how it behaves. Nothing else.
- No history. Never write "before X this returned Y", "corrected at code review", or "pre-existing, not introduced by ...". A comment is not a changelog.
- When a later change alters behavior, EDIT the comment to describe the new behavior. Never append the story of the change - that belongs in the commit message or the PR.
- No story, epic, ticket or review-process references. This covers comments, test names AND assertion messages. The banned shapes, by example: `14.4-UNIT-002` (scenario ID), `[P1]` (priority tag), `AC2` and `AC#2` (acceptance-criterion citation), `R-14-05` (risk ID). Say what the case covers instead.
- Applies to test files too; test comments drift into narrative most easily.
- The comment-history rule also governs the project-context document, `_bmad-output/project-context.md`. That file lives in the SEPARATE docs repo, reachable from this repo as `../docs/_bmad-output/project-context.md`; `code/docs/` holds only `cli-usage.md` and `screenshots/`.

## Naming
- No story, epic or ticket numbers in file, directory, package or module names. Name things after what they contain: `tests/shared-text-string-decoder/`, not `tests/14-4-shared-text-string-decoder/`.
- An existing numbered sibling is not a licence to copy the pattern forward. Unnumbered siblings set the target style even where numbered ones outnumber them.

## Documentation Rules
- Go: godoc comment on every exported type, function, method, and constant.
- TypeScript/React: JSDoc comment on every exported component, hook, and utility function.
- Add brief inline comments on non-obvious logic blocks (e.g., why a guard exists, what a bitwise op does, why order matters).
- One line where one line suffices. Not verbose.
- Skip: test files, unexported helpers where the name is self-explanatory, auto-generated bindings.

## Review Rules
- State the bug. Show the fix. Stop.
- No suggestions beyond the scope of the review.
- No compliments on the code before or after the review.

## Debugging Rules
- Never speculate about a bug without reading the relevant code first.
- State what you found, where, and the fix. One pass.
- If cause is unclear: say so. Do not guess.

## ASCII Only
- No em dashes, smart quotes, Unicode bullets.
- Plain hyphens and straight quotes only.
- Code output must be copy-paste safe.
