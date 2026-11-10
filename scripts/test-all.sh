#!/usr/bin/env bash
# Mirrors the test steps of .github/workflows/ci.yml so the full suite can be
# run locally without waiting on CI. Run from anywhere; resolves repo root.
#
# IMPORTANT: `go test ./...` from the repo root does NOT cover the per-module
# acceptance suites under tests/*/ -- each has its own go.mod and is invisible
# to the root module. This script runs them in a loop exactly as CI does.
#
# Usage:
#   scripts/test-all.sh           # everything (go + frontend)
#   scripts/test-all.sh --go      # Go vet/lint/test, per-module suites and the
#                                 # GUI suite only
#   scripts/test-all.sh --fe      # frontend typecheck/lint/vitest only
#   scripts/test-all.sh --quick   # skip vet+lint, just run the test suites
#   scripts/test-all.sh --no-gui  # skip the GUI suite (headless shell, or the
#                                 # screen is in use); combines with the others
#
# The GUI suite builds bin/pdfdebug-mcp and drives it through the Wails MCP
# server, so a window opens on screen while it runs. It is skipped on Windows
# and by --fe.
set -euo pipefail
shopt -s nullglob

cd "$(git rev-parse --show-toplevel)"

run_go=1
run_fe=1
run_checks=1
run_gui=1
for arg in "$@"; do
  case "$arg" in
    --go)     run_fe=0 ;;
    --fe)     run_go=0 ;;
    --quick)  run_checks=0 ;;
    --no-gui) run_gui=0 ;;
    *) echo "unknown flag: $arg" >&2; exit 2 ;;
  esac
done

section() { printf '\n\033[1;34m==> %s\033[0m\n' "$1"; }

if [[ $run_go -eq 1 ]]; then
  if [[ $run_checks -eq 1 ]]; then
    section "Go vet"
    go vet ./...
    section "Go lint (golangci-lint)"
    golangci-lint run --timeout 5m ./...
    section "Go vet (mcp and guimcp tags)"
    go vet -tags mcp,guimcp ./...
  fi

  section "Go test (root module)"
  go test ./... -timeout 10m
  section "Go test (mcp tag)"
  go test -tags mcp ./internal/pdfservice/ ./internal/mcpclient/

  section "Acceptance suites (per-module)"
  mods=(tests/*/go.mod)
  if [[ ${#mods[@]} -eq 0 ]]; then
    echo "No per-suite go.mod files found under tests/*/" >&2
    exit 1
  fi
  for mod in "${mods[@]}"; do
    dir="$(dirname "$mod")"
    case "$(basename "$dir")" in
      boot-smoke) continue ;;
    esac
    section "go test $dir"
    (cd "$dir" && go test ./... -timeout 5m)
  done

  if [[ $run_gui -eq 1 ]]; then
    case "$(uname -s)" in
      MINGW*|MSYS*|CYGWIN*) echo "GUI suite skipped: not supported on Windows" ;;
      *)
        section "GUI suite (tests/gui-mcp)"
        wails3 task build:mcp
        gui=(env WAILS_MCP_HIDE_CURSOR=1 go test -tags guimcp -count=1 -timeout 5m ./tests/gui-mcp/)
        if [[ "$(uname -s)" == "Linux" && -z "${DISPLAY:-}" && -z "${WAYLAND_DISPLAY:-}" ]]; then
          if command -v xvfb-run >/dev/null && command -v dbus-run-session >/dev/null; then
            xvfb-run -a dbus-run-session -- "${gui[@]}"
          else
            echo "no display and no xvfb-run/dbus-run-session on PATH; install them or rerun with --no-gui" >&2
            exit 1
          fi
        else
          "${gui[@]}"
        fi
        ;;
    esac
  fi
fi

if [[ $run_fe -eq 1 ]]; then
  if [[ $run_checks -eq 1 ]]; then
    section "Frontend typecheck"
    npm run typecheck --prefix frontend
    section "Frontend lint"
    npm run lint --prefix frontend
  fi
  section "Frontend test (Vitest)"
  npm run test --prefix frontend
fi

section "All suites passed"
