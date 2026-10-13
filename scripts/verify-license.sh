#!/usr/bin/env bash
# scripts/verify-license.sh - enforces Apache 2.0 LICENSE + NOTICE compliance
# and README/CONTRIBUTING structural invariants.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

fail() { echo "ERROR: $*" >&2; exit 1; }

# LICENSE: byte-match against canonical + documented copyright substitution.
# --strip-trailing-cr tolerates Windows CI checkouts that apply CRLF normalization
# (core.autocrlf=true is the Git for Windows default) without loosening content checks.
diff --strip-trailing-cr "$ROOT/scripts/fixtures/apache-2.0-with-copyright.txt" "$ROOT/LICENSE" \
  || fail "LICENSE does not match canonical Apache 2.0 (with UniDoc copyright substitution)"

# NOTICE: required markers
grep -q "UniDoc ehf." "$ROOT/NOTICE"     || fail "NOTICE missing 'UniDoc ehf.' attribution"
grep -qF "(c)" "$ROOT/NOTICE"            || fail "NOTICE missing ASCII '(c)' copyright marker"
grep -q "Apache License" "$ROOT/NOTICE"  || fail "NOTICE missing 'Apache License' reference"
# Unicode copyright symbol absence: explicit if/then to avoid `! cmd || fail` double-negation
# footgun under `set -euo pipefail` and keep intent readable.
if grep -q $'\xc2\xa9' "$ROOT/NOTICE"; then
  fail "NOTICE contains Unicode copyright symbol -- use ASCII '(c)' per project ASCII-only rule"
fi
for lib in pdfcpu wails @wailsio/runtime react @radix-ui react-arborist allotment tailwindcss; do
  grep -qF "$lib" "$ROOT/NOTICE" || fail "NOTICE missing third-party attribution for: $lib"
done

# README: required H2 sections
for section in "## Overview" "## Screenshot" "## Installation" \
               "## Build from Source" "## Usage" "## Architecture" \
               "## Contributing" "## License"; do
  grep -qF "$section" "$ROOT/README.md" || fail "README.md missing required section: $section"
done

# CONTRIBUTING: required H2 sections
for section in "## Development Environment" "## Running Tests" "## Code Style" \
               "## Submitting Pull Requests" "## Release Process" \
               "## Reporting Issues"; do
  grep -qF "$section" "$ROOT/CONTRIBUTING.md" || fail "CONTRIBUTING.md missing required section: $section"
done

# README image references: every ![...](path) to a local (non-http) asset must resolve.
# Catches renamed/deleted screenshots before they ship as broken images on GitHub.
while IFS= read -r rel; do
  [[ -z "$rel" ]] && continue
  case "$rel" in http://*|https://*) continue ;; esac
  [[ -f "$ROOT/$rel" ]] || fail "README.md references missing image asset: $rel"
done < <(grep -oE '!\[[^]]*\]\([^)]+\)' "$ROOT/README.md" | sed -E 's/^!\[[^]]*\]\(([^)]+)\)$/\1/')

echo "OK: LICENSE, NOTICE, README.md, CONTRIBUTING.md verified."
