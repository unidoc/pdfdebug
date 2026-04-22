# Contributing to UniDoc PDF Debugger

Thanks for considering a contribution. This document covers the developer workflow once your environment is set up, the test commands, code style, pull-request process, release procedure, and how to report issues.

## Development Environment

See [README.md#build-from-source](./README.md#build-from-source) for the canonical dev setup -- this section documents the developer workflow once the environment is up.

### Toolchain

| Tool | Version | Source of truth |
|---|---|---|
| Go | 1.25.x | `go.mod` |
| Node.js | 20.x LTS | `.github/workflows/ci.yml` |
| Wails v3 CLI | `v3.0.0-alpha.74` | `go.mod` require block |
| golangci-lint | v2.1.6 | `.golangci.yml` |

### Editor recommendations

- VSCode with the Go extension (`gopls`), ESLint, and Tailwind CSS IntelliSense
- `.nvmrc` sets Node 24 for `nvm use`; CI pins Node 20. Both work locally, but CI runs 20.

### Task aliases

`task dev` is a shortcut for `wails3 dev` (see `Taskfile.yml`).

## Running Tests

The project has a Go root module plus per-suite acceptance modules under `tests/*/go.mod`. A naive `go test ./...` at the repo root does NOT descend into the per-suite modules -- the loop below mirrors the CI pattern (`e2e` and `support` skipped).

```bash
# Go unit tests (root module)
go test ./...

# Go acceptance test suites (per-module, under tests/*/go.mod;
# e2e and support directories are skipped to match CI)
for mod in tests/*/go.mod; do
  dir="$(dirname "$mod")"
  case "$(basename "$dir")" in
    e2e|support) continue ;;
  esac
  (cd "$dir" && go test ./...)
done

# Frontend (Vitest)
npm run test --prefix frontend

# Frontend lint (ESLint)
npm run lint --prefix frontend

# Frontend type-check (tsc)
npm run typecheck --prefix frontend

# Go lint (golangci-lint v2.1.6)
golangci-lint run ./...
```

## Code Style

- **Go**: `gofmt` and `goimports` on every file; linted by `golangci-lint` with the project `.golangci.yml` (version 2 schema; `errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused` enabled).
- **TypeScript / React**: ESLint 9 flat config with `typescript-eslint`, `eslint-plugin-react`, and `eslint-plugin-react-hooks`. `@typescript-eslint/no-explicit-any` and `@typescript-eslint/no-unused-vars` are `error` -- do not relax them.
- **Commit messages**: short single-line messages. No `Co-Authored-By` trailers.
- **Docstrings** (per [CLAUDE.md](./CLAUDE.md)): Go godoc on every exported symbol; TypeScript JSDoc on every exported component, hook, and utility. See `CLAUDE.md` for the full rulebook.

## Submitting Pull Requests

1. Fork the repository and create a branch from `dev` (not `main`).
2. Write tests first where practical -- the project uses a story-based TDD pattern, but writing tests up-front is optional for contributor PRs.
3. Open your PR against the `dev` branch.
4. CI must pass on all three runners: `build-and-test (macos-latest)`, `build-and-test (windows-latest)`, and `build-and-test (ubuntu-latest)`.
5. At least one maintainer review plus green CI are required before merge.

**Maintainers**: configure required status checks on the `main` and `dev` branches via GitHub Settings -> Branches to require the three `build-and-test (*)` runs. This replaces the one-line deferred note from story 7-1 Task 7.5.

## Release Process

1. **Tag a version.** Tag `vX.Y.Z` on `dev` and push to the remote. This triggers `.github/workflows/release.yml`, which builds cross-platform artifacts and publishes a GitHub Release (see story 7-2).

2. **Apple Developer ID cert rotation (maintainer-only).** Apple Developer ID certificates have a 5-year lifetime. When the cert expires:

   - Regenerate the certificate via the Apple Developer portal.
   - Export as `.p12` and base64-encode:

     ```bash
     base64 -i DeveloperID.p12 | pbcopy
     ```

   - Update the `APPLE_DEVELOPER_ID_CERT_P12_BASE64` secret in repo Settings -> Secrets and variables -> Actions.
   - If the DUNS changes (rare), also update `APPLE_DEVELOPER_ID`.

3. **App-specific passwords** (used for notarization) are permanent. If one leaks, revoke it at [appleid.apple.com](https://appleid.apple.com) and update `APPLE_ID_APP_PASSWORD`.

4. **Homebrew formula** is auto-updated by the release pipeline once story 7-4 lands. No manual step after that.

## Reporting Issues

- File a GitHub Issue with a minimal reproducer: attach a PDF file (redact sensitive content) and the CLI or GUI output.
- For security-sensitive issues, email `security@unidoc.io` -- do not open a public issue.
- V1 does not define formal triage SLAs; maintainers aim for a 7-day first-response target.
