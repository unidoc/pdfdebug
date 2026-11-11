#!/usr/bin/env bash
# Fails when any given Go binary was built with the mcp build tag, which
# compiles the Wails MCP automation server in. Reads the build settings with
# go version -m, which works on stripped and -trimpath binaries.
#
# Usage: scripts/check-no-mcp-tag.sh <binary>...
set -uo pipefail

if [ "$#" -eq 0 ]; then
  echo "usage: $0 <binary>..." >&2
  exit 2
fi

status=0
for bin in "$@"; do
  if ! info=$(go version -m "$bin" 2>&1); then
    echo "FAIL: cannot read build info from $bin: $info" >&2
    status=1
    continue
  fi
  tags=$(printf '%s\n' "$info" | awk '$1 == "build" && index($2, "-tags=") == 1 { sub(/^-tags=/, "", $2); print $2 }')
  found=0
  IFS=',' read -r -a list <<< "$tags"
  for tag in "${list[@]:-}"; do
    if [ "$tag" = "mcp" ]; then
      found=1
    fi
  done
  if [ "$found" -eq 1 ]; then
    echo "FAIL: $bin was built with the mcp tag (-tags=$tags)" >&2
    status=1
  else
    echo "ok: $bin (-tags=${tags:-none})"
  fi
done
exit "$status"
