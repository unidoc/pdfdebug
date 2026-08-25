#!/usr/bin/env bash
#
# verify-cli-output-parity.sh - prove a refactor changed no CLI output.
#
# Builds the pdfdebug CLI twice (once from a baseline git ref, once from the
# working tree), runs the SAME invocation matrix through both binaries over the
# whole testdata/ fixture corpus, and diffs the captured output byte for byte.
# An empty diff is the pass condition.
#
# This exists because a comment-and-name sweep must not change behaviour, and
# "the full fixture corpus" is not an executable instruction - the matrix has to
# be written down. It is written down below, in MATRIX_DOC and the two loops
# that implement it.
#
# Usage:
#   scripts/verify-cli-output-parity.sh [baseline-ref] [outdir]
#
#   baseline-ref  git ref to compare against (default: 5c3e6b3)
#   outdir        where artifacts land (default: a fresh mktemp -d, path printed)
#
# Exit status:
#   0  the two binaries produced identical output on every invocation
#   1  a difference was found (the diff is written to $outdir/parity.diff)
#   2  the harness itself failed (build error, missing corpus, ...)
#
# Run it manually. It is deliberately NOT wired into CI: it needs two builds and
# a git worktree, and it answers a question you only ask during a refactor.
#
# Notes for whoever maintains this:
#   - No `set -e`. `validate` exits 1 on structural findings and `diff` exits 1
#     on differences; both are legitimate, expected results that must be
#     recorded, not treated as harness failures. Every invocation's exit code is
#     captured into the artifact so an exit-code change also shows up as a diff.
#   - Both binaries are built with identical flags and NO -ldflags. Injecting a
#     version string would make `--version`-bearing output differ between the
#     two builds and defeat the whole comparison.
#   - macOS ships bash 3.2: no `mapfile`, no `readarray`, no associative
#     arrays. Ref lists are passed around as newline-separated strings.
#   - --ref arguments are derived per fixture from that fixture's own
#     `dump xref --json`, using the BASELINE binary, so both runs are driven by
#     one identical ref list. If the two binaries disagree about the xref
#     itself, the `dump xref` invocation shows it.

set -uo pipefail

# Default to the integration branch. An earlier version defaulted to a specific
# commit, which was meaningful only while that commit was the merge base and
# silently meaningless afterwards.
BASELINE_REF="${1:-origin/dev}"
OUTDIR="${2:-}"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT" || exit 2

MATRIX_DOC='
Invocation matrix (per fixture F in `find testdata -name "*.pdf"`, sorted):

  Document-level, 14 invocations:
    dump tree --json F
    dump tree --json --depth 3 F
    dump tree --json --resolve --resolve-depth 2 F
    dump xref --json F
    dump objects --json F
    dump plaintext --json F
    dump metadata --json F
    dump signatures --json F
    dump embedded --json F
    dump page --info 1 --json F
    dump stream --page 1 --json F
    dump stream --page 1 --ops F
    validate --profile pdfa-1b --json F
    validate --profile pdfua-1-structural --json F

  Per in-use xref entry R of F, 6 invocations:
    dump object --json --ref R F
    dump object --json --resolve --ref R F
    dump source --json --ref R F
    dump reverserefs --json --ref R F
    dump font --json --ref R F
    dump image --json --metadata --ref R F

  Structural diff, 1 self-diff per fixture plus a fixed set of cross pairs:
    diff --json F F
    diff --json <left> <right>   for each pair in DIFF_PAIRS

A plain-text (no --json) pass over the document-level commands runs as a
secondary signal; it is diffed and reported but is not the gate, because
plain-text layout is not the machine contract.
'

# Cross-fixture diff pairs: chosen so each exercises a different arm of the
# path-alignment engine (renumbering, deep subtree change, tagging, page count,
# signature presence, out-of-range object numbers).
DIFF_PAIRS="
testdata/correctness/deep-change-a.pdf testdata/correctness/deep-change-b.pdf
testdata/minimal.pdf testdata/multipage.pdf
testdata/tagged.pdf testdata/untagged.pdf
testdata/signed.pdf testdata/unsigned-sig-field.pdf
testdata/signed.pdf testdata/signed-badcontents.pdf
testdata/pdfa-1b-clean.pdf testdata/no-output-intent.pdf
testdata/correctness/diff-out-of-range.pdf testdata/minimal.pdf
"

die() { printf 'verify-cli-output-parity: %s\n' "$*" >&2; exit 2; }

command -v go >/dev/null 2>&1 || die "go not on PATH"
git rev-parse --verify "$BASELINE_REF" >/dev/null 2>&1 || die "unknown git ref: $BASELINE_REF"

if [ -z "$OUTDIR" ]; then
	OUTDIR="$(mktemp -d)" || die "mktemp failed"
fi
mkdir -p "$OUTDIR" || die "cannot create $OUTDIR"
# Absolutize: the baseline build runs `go build -o "$BIN_HEAD.tmp"` from inside
# the worktree, so a relative OUTDIR would resolve against the worktree instead
# of here and the following mv would not find the binary.
OUTDIR="$(cd "$OUTDIR" && pwd)" || die "cannot resolve $OUTDIR"

WORKTREE="$OUTDIR/baseline-worktree"
BIN_BASE="$OUTDIR/pdfdebug-baseline"
BIN_HEAD="$OUTDIR/pdfdebug-head"
ART_BASE="$OUTDIR/baseline.json.txt"
ART_HEAD="$OUTDIR/head.json.txt"
ART_BASE_TXT="$OUTDIR/baseline.plain.txt"
ART_HEAD_TXT="$OUTDIR/head.plain.txt"

TMP_OUT="$OUTDIR/.stdout"
TMP_ERR="$OUTDIR/.stderr"

cleanup() {
	if [ -d "$WORKTREE" ]; then
		git worktree remove --force "$WORKTREE" >/dev/null 2>&1
	fi
	rm -f "$TMP_OUT" "$TMP_ERR"
}
trap cleanup EXIT

printf '%s\n' "$MATRIX_DOC" > "$OUTDIR/matrix.txt"

echo "baseline ref : $BASELINE_REF"
echo "artifacts    : $OUTDIR"
echo

# --- build both binaries, identical flags, no -ldflags -----------------------

echo "building working-tree binary..."
go build -o "$BIN_HEAD" ./cmd/cli/ || die "working-tree build failed"

echo "building $BASELINE_REF binary..."
rm -rf "$WORKTREE"
git worktree add -q --detach "$WORKTREE" "$BASELINE_REF" || die "git worktree add failed"
(cd "$WORKTREE" && go build -o "$BIN_HEAD.tmp" ./cmd/cli/) || die "baseline build failed"
mv "$BIN_HEAD.tmp" "$BIN_BASE" || die "cannot place baseline binary"

# --- corpus -----------------------------------------------------------------

FIXTURES="$(find testdata -name '*.pdf' | sort)"
[ -n "$FIXTURES" ] || die "no fixtures under testdata/"

fixture_count=0
while IFS= read -r f; do
	[ -n "$f" ] && fixture_count=$((fixture_count + 1))
done <<EOF
$FIXTURES
EOF
echo "fixtures     : $fixture_count"

# refs_for <pdf> -- newline-separated "objNum gen" for every in-use xref entry,
# derived from the baseline binary's own `dump xref --json`. The struct field
# order in the JSON is fixed by the Go type, so this sed is stable.
refs_for() {
	"$BIN_BASE" dump xref --json "$1" 2>/dev/null \
		| tr '{' '\n' \
		| sed -n 's/.*"objNum":\([0-9]*\),"gen":\([0-9]*\),"status":"in-use".*/\1 \2/p'
}

# run_one <artifact> <binary> <args...>
# Appends a header naming the invocation, then the exit code, then stdout and
# stderr verbatim. The exit code is part of the artifact so a changed exit code
# fails the diff even when the bytes match.
JSON_INVOCATIONS=0
TXT_INVOCATIONS=0
run_one() {
	art="$1"; shift
	bin="$1"; shift
	# stdout and stderr are captured to separate files and emitted with `cat`,
	# not through command substitution. `$(...)` strips every trailing newline,
	# so a regression that drops the CLI's final newline would have compared
	# equal; and `2>&1` merges the streams, so output written to the wrong one
	# would also have compared equal. Byte counts are recorded per stream so a
	# length change fails the diff even where the visible text is identical.
	"$bin" "$@" >"$TMP_OUT" 2>"$TMP_ERR"
	code=$?
	{
		printf '===== %s\n' "$*"
		printf 'exit=%d\n' "$code"
		printf 'stdout-bytes=%s\n' "$(wc -c <"$TMP_OUT" | tr -d ' ')"
		printf 'stderr-bytes=%s\n' "$(wc -c <"$TMP_ERR" | tr -d ' ')"
		printf -- '----- stdout\n'
		cat "$TMP_OUT"
		printf -- '----- stderr\n'
		cat "$TMP_ERR"
	} >> "$art"
}

# run_pair <args...> -- same invocation through both binaries, JSON artifacts.
run_pair() {
	run_one "$ART_BASE" "$BIN_BASE" "$@"
	run_one "$ART_HEAD" "$BIN_HEAD" "$@"
	JSON_INVOCATIONS=$((JSON_INVOCATIONS + 1))
}

# run_pair_txt <args...> -- same, into the plain-text (signal-only) artifacts.
run_pair_txt() {
	run_one "$ART_BASE_TXT" "$BIN_BASE" "$@"
	run_one "$ART_HEAD_TXT" "$BIN_HEAD" "$@"
	TXT_INVOCATIONS=$((TXT_INVOCATIONS + 1))
}

: > "$ART_BASE"
: > "$ART_HEAD"
: > "$ART_BASE_TXT"
: > "$ART_HEAD_TXT"

echo "running matrix..."
while IFS= read -r pdf; do
	[ -n "$pdf" ] || continue

	# Document-level, JSON (the machine contract).
	run_pair dump tree --json "$pdf"
	run_pair dump tree --json --depth 3 "$pdf"
	run_pair dump tree --json --resolve --resolve-depth 2 "$pdf"
	run_pair dump xref --json "$pdf"
	run_pair dump objects --json "$pdf"
	run_pair dump plaintext --json "$pdf"
	run_pair dump metadata --json "$pdf"
	run_pair dump signatures --json "$pdf"
	run_pair dump embedded --json "$pdf"
	run_pair dump page --info 1 --json "$pdf"
	run_pair dump stream --page 1 --json "$pdf"
	run_pair dump stream --page 1 --ops "$pdf"
	run_pair validate --profile pdfa-1b --json "$pdf"
	run_pair validate --profile pdfua-1-structural --json "$pdf"

	# Document-level, plain text (signal only).
	run_pair_txt dump tree "$pdf"
	run_pair_txt dump xref "$pdf"
	run_pair_txt dump objects "$pdf"
	run_pair_txt dump metadata "$pdf"
	run_pair_txt dump signatures "$pdf"
	run_pair_txt validate --profile pdfa-1b "$pdf"

	# Per in-use object.
	refs="$(refs_for "$pdf")"
	while IFS= read -r pair; do
		[ -n "$pair" ] || continue
		num="${pair% *}"
		gen="${pair#* }"
		ref="$num $gen R"
		run_pair dump object --json --ref "$ref" "$pdf"
		run_pair dump object --json --resolve --ref "$ref" "$pdf"
		run_pair dump source --json --ref "$ref" "$pdf"
		run_pair dump reverserefs --json --ref "$ref" "$pdf"
		run_pair dump font --json --ref "$ref" "$pdf"
		run_pair dump image --json --metadata --ref "$ref" "$pdf"
	done <<EOF
$refs
EOF

	# Self-diff: must always report identical.
	run_pair diff --json "$pdf" "$pdf"
done <<EOF
$FIXTURES
EOF

# Cross-fixture diffs.
while IFS= read -r pair; do
	[ -n "$pair" ] || continue
	left="${pair% *}"
	right="${pair#* }"
	run_pair diff --json "$left" "$right"
done <<EOF
$DIFF_PAIRS
EOF

TOTAL_INVOCATIONS=$((JSON_INVOCATIONS + TXT_INVOCATIONS))
echo "invocations  : $TOTAL_INVOCATIONS per binary ($JSON_INVOCATIONS JSON gate + $TXT_INVOCATIONS plain-text signal)"
echo

# --- exit-code census, useful when reading the artifact by hand -------------

echo "exit-code census (working tree, JSON pass):"
grep '^exit=' "$ART_HEAD" | sort | uniq -c | sed 's/^/  /'
echo

# --- the gate ---------------------------------------------------------------

echo "baseline lines: $(wc -l < "$ART_BASE")"
echo "head     lines: $(wc -l < "$ART_HEAD")"
echo

status=0

if diff -u "$ART_BASE" "$ART_HEAD" > "$OUTDIR/parity.diff" 2>&1; then
	echo "PASS  JSON output is byte-identical across $JSON_INVOCATIONS invocations."
else
	echo "FAIL  JSON output differs. See $OUTDIR/parity.diff"
	status=1
fi

if diff -u "$ART_BASE_TXT" "$ART_HEAD_TXT" > "$OUTDIR/parity-plain.diff" 2>&1; then
	echo "      plain-text output also identical (signal only)."
else
	echo "      NOTE plain-text output differs (signal only, not the gate):"
	echo "      $OUTDIR/parity-plain.diff"
fi

echo
echo "artifacts kept in $OUTDIR"
exit $status
