# Acceptance suites

Every directory under `tests/` except `gui-mcp` is its own Go module with its own `go.mod`, so the root `go test ./...` does not reach it. Run one suite from its directory:

```bash
cd tests/structural-diff && go test -count=1 ./...
```

`scripts/test-all.sh` and CI loop over `tests/*/go.mod` and run each module the same way. The loop skips `boot-smoke`, which has its own step.

Most suites run the CLI as a subprocess or check source, build and workflow files. They do not import the root module; the module paths sit outside `unidoc-pdf-debugger/`, which keeps the Wails dependency tree out of them.

## boot-smoke

`tests/boot-smoke` launches a built desktop binary and checks that the boot path reaches the event loop without panicking. It needs a display, so Linux CI runs it under `xvfb-run -a dbus-run-session --` against the installed `.deb` binary:

```bash
cd tests/boot-smoke && BOOT_SMOKE_BINARY=/usr/bin/unidoc-pdf-debugger go test -count=1 ./...
```

## gui-mcp

`tests/gui-mcp` drives a `pdfdebug-mcp` build of the app through the Wails MCP server: it launches the binary, opens committed fixtures from `testdata/`, clicks through the main window and reads the DOM. It has no `go.mod`. It lives in the root module because it imports `internal/mcpclient`, which Go's internal-package rule hides from other modules. Its files carry `//go:build guimcp && !windows`, so the root `go test ./...` and `go vet ./...` skip it.

```bash
wails3 task build:mcp
go test -tags guimcp -count=1 ./tests/gui-mcp/
```

A window opens while it runs. `PDFDEBUG_MCP_BINARY` overrides the binary path (default `bin/pdfdebug-mcp`). Every `mcp` build shares one single-instance lock, so stop any `guictl` instance first (`go run ./tools/guictl stop`). `scripts/test-all.sh` runs the suite after the per-module suites; pass `--no-gui` to skip it. `tests/gui-mcp/COVERAGE.md` lists which UI features the MCP server can reach and which stay manual.

## Unit tests

Go unit tests live next to the code in the root module (`go test ./...`). The `mcp`-tagged code has its own run: `go test -tags mcp ./internal/pdfservice/ ./internal/mcpclient/`. Frontend unit tests are Vitest: `npm run test --prefix frontend`.
