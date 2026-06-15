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
#   scripts/test-all.sh --go      # Go vet/lint/test + per-module suites only
#   scripts/test-all.sh --fe      # frontend typecheck/lint/vitest only
#   scripts/test-all.sh --quick   # skip vet+lint, just run the test suites
set -euo pipefail
shopt -s nullglob

cd "$(git rev-parse --show-toplevel)"

run_go=1
run_fe=1
run_checks=1
case "${1:-}" in
  --go)    run_fe=0 ;;
  --fe)    run_go=0 ;;
  --quick) run_checks=0 ;;
  "")      ;;
  *) echo "unknown flag: $1" >&2; exit 2 ;;
esac

section() { printf '\n\033[1;34m==> %s\033[0m\n' "$1"; }

if [[ $run_go -eq 1 ]]; then
  if [[ $run_checks -eq 1 ]]; then
    section "Go vet"
    go vet ./...
    section "Go lint (golangci-lint)"
    golangci-lint run --timeout 5m ./...
  fi

  section "Go test (root module)"
  go test ./... -timeout 10m

  section "Acceptance suites (per-module)"
  mods=(tests/*/go.mod)
  if [[ ${#mods[@]} -eq 0 ]]; then
    echo "No per-suite go.mod files found under tests/*/" >&2
    exit 1
  fi
  for mod in "${mods[@]}"; do
    dir="$(dirname "$mod")"
    case "$(basename "$dir")" in
      e2e|support|boot-smoke) continue ;;
    esac
    section "go test $dir"
    (cd "$dir" && go test ./... -timeout 5m)
  done
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
