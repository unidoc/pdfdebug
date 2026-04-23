#!/usr/bin/env bash
# render-formula.sh renders the Homebrew formula template for unipdf-debugger.
#
# Usage:
#   scripts/homebrew/render-formula.sh <version> <sha_darwin_arm64> \
#                                      <sha_darwin_amd64> <sha_linux_amd64> <tag>
#
# Reads scripts/homebrew/unipdf-debugger.rb.tmpl, substitutes the seven
# @@...@@ placeholder tokens (version + three SHA256 + three asset URLs),
# and writes the rendered formula to stdout. The caller redirects to the
# target path (typically Formula/unipdf-debugger.rb on the tap repo).
#
# Asset URL convention matches story 7-2 Task 4.3:
#   https://github.com/unidoc/unipdf-debugger/releases/download/<tag>/<asset>
set -euo pipefail

if [ "$#" -ne 5 ]; then
  echo "usage: $0 <version> <sha_darwin_arm64> <sha_darwin_amd64> <sha_linux_amd64> <tag>" >&2
  exit 2
fi

VERSION="$1"
SHA_DARWIN_ARM64="$2"
SHA_DARWIN_AMD64="$3"
SHA_LINUX_AMD64="$4"
TAG="$5"

for name in VERSION SHA_DARWIN_ARM64 SHA_DARWIN_AMD64 SHA_LINUX_AMD64 TAG; do
  if [ -z "${!name}" ]; then
    echo "error: $name must not be empty" >&2
    exit 2
  fi
done

# Validate SHA256 args are exactly 64 hex chars. A malformed SHA would render
# an invalid formula that passes `ruby -c` (SHAs sit inside quoted strings) but
# silently causes every user `brew install` to fail with an opaque mismatch.
# Also guards against sed-replacement metacharacters (`&`, `\`) in the SHA slot.
for name in SHA_DARWIN_ARM64 SHA_DARWIN_AMD64 SHA_LINUX_AMD64; do
  if ! [[ "${!name}" =~ ^[0-9a-fA-F]{64}$ ]]; then
    echo "error: $name is not a 64-char hex SHA256: ${!name}" >&2
    exit 2
  fi
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TMPL="$SCRIPT_DIR/unipdf-debugger.rb.tmpl"

if [ ! -f "$TMPL" ]; then
  echo "error: template not found: $TMPL" >&2
  exit 1
fi

REPO_BASE="https://github.com/unidoc/unipdf-debugger/releases/download/${TAG}"
URL_DARWIN_ARM64="${REPO_BASE}/unidoc-pdf-debugger-${VERSION}-darwin-arm64.app.zip"
URL_DARWIN_AMD64="${REPO_BASE}/unidoc-pdf-debugger-${VERSION}-darwin-amd64.app.zip"
URL_LINUX_AMD64="${REPO_BASE}/unidoc-pdf-debugger-${VERSION}-linux-amd64.tar.gz"

# Use `|` as the sed delimiter because the URL values contain `/`.
sed -e "s|@@VERSION@@|${VERSION}|g" \
    -e "s|@@DARWIN_ARM64_URL@@|${URL_DARWIN_ARM64}|g" \
    -e "s|@@DARWIN_AMD64_URL@@|${URL_DARWIN_AMD64}|g" \
    -e "s|@@LINUX_AMD64_URL@@|${URL_LINUX_AMD64}|g" \
    -e "s|@@DARWIN_ARM64_SHA256@@|${SHA_DARWIN_ARM64}|g" \
    -e "s|@@DARWIN_AMD64_SHA256@@|${SHA_DARWIN_AMD64}|g" \
    -e "s|@@LINUX_AMD64_SHA256@@|${SHA_LINUX_AMD64}|g" \
    "$TMPL"
