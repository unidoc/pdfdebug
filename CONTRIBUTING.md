# Contributing to UniDoc PDF Debugger

Thanks for considering a contribution. This document covers the developer workflow once your environment is set up, the test commands, code style, pull-request process, release procedure, and how to report issues.

## Development Environment

See [README.md#build-from-source](./README.md#build-from-source) for the canonical dev setup -- this section documents the developer workflow once the environment is up.

### Toolchain

| Tool | Version | Source of truth |
|---|---|---|
| Go | 1.27.x | `go.mod` (1.27.3 or later) |
| Node.js | 20.x LTS | `.github/workflows/ci.yml` |
| Wails v3 CLI | `v3.0.0` | `go.mod` require block |
| golangci-lint | v2.14.0 | `.github/workflows/ci.yml` |
| create-dmg | Homebrew (macos-latest) | macOS release DMG packaging only; installed by `release.yml` via `brew install create-dmg` |

### Editor recommendations

- VSCode with the Go extension (`gopls`), ESLint, and Tailwind CSS IntelliSense
- `.nvmrc` sets Node 24 for `nvm use`; CI pins Node 20. Both work locally, but CI runs 20.

### Task aliases

`task dev` is a shortcut for `wails3 dev` (see `Taskfile.yml`).

## Running Tests

The project has a Go root module plus per-suite acceptance modules under `tests/*/go.mod`. A naive `go test ./...` at the repo root does NOT descend into the per-suite modules -- the loop below mirrors the CI pattern (`boot-smoke` skipped).

```bash
# Go unit tests (root module)
go test ./...

# Go acceptance test suites (per-module, under tests/*/go.mod; boot-smoke is
# skipped to match CI -- it has its own dedicated step that wraps with
# xvfb-run on Linux)
for mod in tests/*/go.mod; do
  dir="$(dirname "$mod")"
  case "$(basename "$dir")" in
    boot-smoke) continue ;;
  esac
  (cd "$dir" && go test ./...)
done

# Frontend (Vitest)
npm run test --prefix frontend

# Frontend lint (ESLint)
npm run lint --prefix frontend

# Frontend type-check (tsc)
npm run typecheck --prefix frontend

# Go lint (golangci-lint v2.14.0)
golangci-lint run ./...
```

## Tests Must Not Grep Production Source

Do NOT add new `strings.Contains`, `regexp.Match`, or other file-content assertions against `main.go`, `frontend/src/components/MainLayout.tsx`, or `frontend/src/components/EmptyState.tsx` from any test under `tests/`. If you find yourself reaching for that pattern, write a behavioural test (Vitest, `tests/boot-smoke` or `tests/gui-mcp`) instead.

Pre-existing structural greps in those files are grandfathered pending a bulk-removal follow-up. Do not extend the pattern. Reviewers must flag new source-greps at PR time.

Generated build artifacts (`build/linux/*.desktop`, `build/darwin/Info.plist`, etc.) are exempt -- parsing those is legitimate behavioural testing on a build output, not source.

## GUI automation

A development build of the desktop app can carry Wails' MCP server, which lets an agent (or a test) query the DOM, click, type and call bound Go methods in the running window. It is compiled in by the `mcp` build tag and never ships: `mcp_guard.go` stops any `production,mcp` build from compiling, and `release.yml` runs `scripts/check-no-mcp-tag.sh` on every shipped binary. GUI automation works on macOS and Linux only.

### The agent build

`task build:mcp` (or `wails3 task build:mcp` without the `task` binary) makes `bin/pdfdebug-mcp`, a DEV build with `-tags mcp`. On Windows the target stops with a precondition message. The agent build has its own single-instance id (`com.unidoc.unidoc-pdf-debugger.mcp`) and the window title `UniDoc PDF Debugger [agent]`, so it runs beside an installed copy of the app without either one taking the other's files.

Keep the binary name `pdfdebug-mcp` and run it bare, never wrapped in a `.app`. WebKit keys its website data by executable name on macOS, so the agent build's `localStorage` (left rail, panel sizes, window geometry, update preference) stays apart from the production app's. Every `pdfdebug-mcp` run shares that `localStorage` though, so a check that needs some UI state sets it through the UI first.

On Ubuntu 23.10 and later, unprivileged user namespaces are blocked and WebKit's sandbox needs them. The packaged app gets them from its AppArmor profile; `bin/pdfdebug-mcp` needs one of its own, under a different name so it does not replace the packaged profile:

```bash
bin="$PWD/bin/pdfdebug-mcp"
sudo tee /etc/apparmor.d/pdfdebug-mcp-local >/dev/null <<EOT
abi <abi/4.0>,
include <tunables/global>

profile pdfdebug-mcp-local ${bin} flags=(unconfined) {
  userns,
}
EOT
sudo apparmor_parser -r /etc/apparmor.d/pdfdebug-mcp-local
```

### guictl

`go run ./tools/guictl <command>` from the repo root drives one `pdfdebug-mcp` instance. It keeps its state, the app log and the instance's config, cache and data directories under `.guictl/` (git-ignored), so fixtures opened there never reach your own Open Recent list or update-check cache.

- `guictl start [--hide-cursor] [file.pdf...]` runs `wails3 task build:mcp`, launches the binary on a free loopback port, waits until the main window has rendered, and records PID, start time and URL in `.guictl/state.json`. It refuses while a recorded instance is alive, and fails with the exit code and log tail when the app quits before its MCP server starts (another `pdfdebug-mcp` holding the lock is the usual cause). Files given to `start` are opened once the window is up. The animated cursor overlay is on so you can watch; `--hide-cursor` sets `WAILS_MCP_HIDE_CURSOR=1`.
- `guictl open file.pdf...` launches the binary again with absolute paths, so the files reach the running window through the single-instance handoff.
- `guictl pick file.pdf...` queues paths for the next file dialog. In the `mcp` build `PDFService.OpenFileDialog` returns queued paths instead of opening the native picker, then clears the queue; with nothing queued the real dialog opens. The empty state's Open button and Compare's Pick file go through it. File > Open does not.
- `guictl tools` lists the MCP tools and their input schemas.
- `guictl call <tool> '<json>'` calls one tool. It prints `tool args ok|error duration`, then the result, and exits non-zero on a tool error. Tools that take a `window` argument get `"window": "main"` unless you pass one.
- `guictl status` prints PID, URL, uptime and the last 20 lines of `.guictl/app.log`.
- `guictl stop` sends SIGTERM to the instance's process group and waits for it to exit. A state file whose PID is gone, or now belongs to another process, is deleted and nothing is signalled.

Stop your instance before handing a build to someone else to test. Only one `mcp` build can run at a time, since they all share the single-instance lock.

To register a running instance with Claude Code, optionally: `claude mcp add --transport http pdfdebug-gui http://127.0.0.1:9099/mcp` (port 9099 is the default; `guictl` picks a free port, so use the URL `guictl status` prints).

### Interactive use

`WAILS_MCP=1 wails3 dev` runs the dev server with the MCP server compiled in. Set the variable on that one command and nowhere else. If it stays in your shell environment, every later `wails3 build` adds the tag to a production build and fails on the guard (`wails3 package` and `wails3 task` ignore it). A `wails3 dev` session with MCP and a `guictl` instance cannot run at once, for the same lock reason.

The server binds to `127.0.0.1` and needs no token. Leave `WAILS_MCP_HOST` unset; a non-loopback `WAILS_MCP_HOST` requires `WAILS_MCP_TOKEN`, and this project never uses either.

### Limits

Synthetic input from the MCP server arrives with `isTrusted: false`, and it only reaches the WebView. Native menus (and their accelerators such as `CmdOrCtrl+O`), native context menus on tree rows, native file and save dialogs, and drag and drop from the desktop stay manual checks. `tests/gui-mcp/COVERAGE.md` lists every UI feature and how far automation reaches it.

### GUI suite

`tests/gui-mcp` runs flows against `bin/pdfdebug-mcp` over the same client `guictl` uses:

```bash
wails3 task build:mcp
go test -tags guimcp -count=1 ./tests/gui-mcp/
```

A window appears while it runs. `scripts/test-all.sh` runs it after the per-module suites; `scripts/test-all.sh --no-gui` skips it. On Linux without a display the script wraps it in `xvfb-run -a dbus-run-session --`.

## Code Style

- **Go**: `gofmt` and `goimports` on every file; linted by `golangci-lint` with the project `.golangci.yml` (version 2 schema; `errcheck`, `govet`, `ineffassign`, `staticcheck`, `unused` enabled).
- **TypeScript / React**: ESLint 9 flat config with `typescript-eslint`, `eslint-plugin-react`, and `eslint-plugin-react-hooks`. `@typescript-eslint/no-explicit-any` and `@typescript-eslint/no-unused-vars` are `error` -- do not relax them.
- **Commit messages**: short single-line messages. No `Co-Authored-By` trailers.
- **Docstrings** (per [CLAUDE.md](./CLAUDE.md)): Go godoc on every exported symbol; TypeScript JSDoc on every exported component, hook, and utility. See `CLAUDE.md` for the full rulebook.

## Submitting Pull Requests

1. Fork the repository and create a branch from `dev` (not `master`).
2. Write tests first where practical -- the project uses a story-based TDD pattern, but writing tests up-front is optional for contributor PRs.
3. Open your PR against the `dev` branch.
4. CI must pass on all three runners: `build-and-test (macos-latest)`, `build-and-test (windows-latest)`, and `build-and-test (ubuntu-latest)`.
5. At least one maintainer review plus green CI are required before merge.

**Maintainers**: configure required status checks on the `master` and `dev` branches via GitHub Settings -> Branches to require the three `build-and-test (*)` runs.

## Release Process

1. **Check the CLI guide against the binary.** Build the CLI and compare its help with `docs/cli-usage.md` and the reference pages under `docs/cli/`: every command, its flags and the exit codes should match what the binary does. Fix the guide in a PR before tagging.

   ```bash
   go build -o bin/pdfdebug ./cmd/cli && bin/pdfdebug --help
   ```

2. **Tag a version.** Tag `vX.Y.Z` on `dev` and push to the remote. This triggers `.github/workflows/release.yml`, which builds cross-platform artifacts and publishes a GitHub Release. Once it is published, edit the release notes to link the guide as of that tag, so users of that release read the matching version: `https://github.com/unidoc/pdfdebug/blob/vX.Y.Z/docs/cli-usage.md`. Its links to the `docs/cli/` pages resolve at the same tag.

3. **Apple Developer ID cert rotation (maintainer-only).** Apple Developer ID certificates have a 5-year lifetime. When the cert expires:

   - Regenerate the certificate via the Apple Developer portal.
   - Export as `.p12` and base64-encode:

     ```bash
     base64 -i DeveloperID.p12 | pbcopy
     ```

   - Update the `APPLE_DEVELOPER_ID_CERT_P12_BASE64` secret in repo Settings -> Secrets and variables -> Actions.
   - If the DUNS changes (rare), also update `APPLE_DEVELOPER_ID`.

4. **Notarization is currently disabled.** macOS artifacts are codesigned but not notarized, so first-launch from Finder will trigger a Gatekeeper warning that the user must dismiss via right-click -> Open. The Apple Developer ID cert above is sufficient for codesign. To re-enable notarization later, restore the `Notarize and staple` step in `release.yml`, restore the secret detection requirement for `APPLE_ID`, `APPLE_ID_APP_PASSWORD`, and `APPLE_TEAM_ID`, and configure those three secrets.

## Reporting Issues

- File a GitHub Issue with a minimal reproducer: attach a PDF file (redact sensitive content) and the CLI or GUI output.
- For security-sensitive issues, email `security@unidoc.io` -- do not open a public issue.
- V1 does not define formal triage SLAs; maintainers aim for a 7-day first-response target.
