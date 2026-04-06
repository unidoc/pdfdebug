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
