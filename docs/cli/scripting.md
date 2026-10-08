# Scripting pdfdebug

What a script or CI job needs: exit codes, a CI example, which output is a
stable machine format, and the update notice with its opt-outs.

Back to the [CLI usage guide](../cli-usage.md).

## Exit codes

The three command families use different contracts.

`dump` commands:

- 0 - the command ran.
- 1 - usage error: wrong number of files, an unknown flag, a flag after the
  file, an unknown resource, `--help`, a flag value it rejects such as
  `--page 0`, `--depth x` or a malformed `--ref`, or flags that exclude each
  other such as `--raw --json` or `dump embedded --ref` with `--name`.
- 2 - operational error: the file is missing or unreadable, the page or object
  asked for is not in it, or `dump embedded --name` matches no attachment, more
  than one, or one with no `/EmbeddedFile` stream.

`validate`:

- 0 - it ran and found no errors. This is not a conformance verdict.
- 1 - it found at least one error. Under `pdfa-1b` it also exits 1 when a rule
  could not be evaluated, and for an encrypted file, which is reported as one
  `no-encryption` error.
- 2 - operational or usage error: a missing file, an unknown profile, an
  unknown flag, the wrong number of files, a flag after the file, or an
  encrypted file under `pdfua-1-structural`.

Every `pdfua-1-structural` rule reports a warning, so that profile exits 0 no
matter what it finds. A job that should fail on its findings has to read
`summary.warnings` from `--json`.

`diff`:

- 0 - the two files are structurally identical.
- 1 - they differ.
- 2 - operational or usage error.

`pdfdebug` with no arguments or an unknown command prints the help and exits
1; `dump` with no resource prints its one-line usage and also exits 1.
`pdfdebug --help` exits 0, but `--help` on a subcommand exits non-zero: 1 for
`dump`, 2 for `validate` and `diff`.

This script fails a CI step when `validate` cannot run or reports anything. It
is plain `sh` and needs `jq`; it drops into any CI system that runs shell steps.

```sh
rc=0
pdfdebug validate --json --profile pdfua-1-structural file.pdf > report.json || rc=$?
if [ "$rc" -ne 0 ]; then
  echo "pdfdebug validate exited $rc" >&2
  exit 1
fi
jq -e '.summary.errors == 0 and .summary.warnings == 0' report.json > /dev/null
```

Do not pipe `validate` straight into `jq`. Without `pipefail` the pipeline's
status is jq's, and a missing file leaves stdout empty: `jq` without `-e` exits
0 on empty input, so the job would pass, and `jq -e` fails it with exit 4 but
never says that `validate` could not run. To gate on `pdfa-1b` instead, drop
`--profile`; its errors exit 1 and the `rc` check alone fails the step.

## Machine output

Plain text is the default. It is meant for reading and may change between
releases. `--json` is the stable contract for scripts and agents, and
`--pretty` indents it (no effect on plain text).

Operational errors and warnings go to stderr as one JSON object per line,
`{"error":"file not found"}` or `{"warning":"..."}`, whether or not `--json`
is set, and so do usage errors about a flag's value, such as `--page 0` or
`--raw --json`. Not everything on stderr is JSON, though: an unknown flag, a
value that is not a number for a flag that takes one (`--depth x`), a wrong
number of files or a flag after the file prints the plain usage line, an
unknown `validate` profile is a plain sentence, `dump embedded` prints
`error: ...` when given both `--ref` and `--name` or when `--name` does not
pick out one attachment, `dump stream --ops` on a page with no content
stream prints `page has no content stream`, the `dump plaintext` deprecation
notice is plain text, and so is the update notice. A script should branch on
the exit code and treat stderr as text to show, not to parse.

Three outputs are separate machine formats rather than JSON: `dump stream
--raw` (the decoded stream bytes), `dump stream --ops` (one JSON object per
operator, newline-delimited) and the raw output of `dump bytes`. `--raw --json`
and `--ops --json` are rejected with exit 1.

`dump page --info` is EXPERIMENTAL. Its JSON field set may change, and the full
`--json` object carries `"_stability":"experimental"` to say so. It is
structural only; for anything it omits, use `dump stream` or `dump tree`/`dump
object --resolve`.

## Update notice

The update notice goes to stderr only, and only when stdout and stderr are both
terminals, so piping or redirecting either turns it off. It is also off when
`CI` is set, under `--json`, `--ops` and `--raw`, for the raw bytes of
`dump bytes`, and when `dump embedded --ref` or `--name` writes an attachment
to stdout. `PDFDEBUG_NO_UPDATE_CHECK` or `NO_UPDATE_NOTIFIER` turns it off
entirely. Presence is what counts: `PDFDEBUG_NO_UPDATE_CHECK=0` and `CI=false`
still turn it off.

```
+-----------------------------------------------+
|  Update available: 0.4.0 -> 0.5.0             |
|  https://github.com/unidoc/pdfdebug/releases  |
+-----------------------------------------------+
```

Each interactive command ends with the box while the cached latest release is
newer than the running build, without the frame in a terminal narrower than the
box. It never goes to stdout or changes an exit code, and the desktop app's
"check automatically" preference does not affect the CLI. With no successful
CLI or app check in the last day, an interactive command fetches one page of
releases alongside its work and, once its output is done, waits at most 1.5
seconds from the start of the run; a failed fetch is silent and not retried for
a day.

In a terminal, `--version` always checks live, even with a fresh cache, and
prints the box before the version line or says on stderr that no newer release
exists or the check failed. It checks even when the cache directory cannot be
written, answers from the higher of the CLI and app records of the last day when
its own check fails, and skips the check, saying so on stderr, on development
builds and when an opt-out is set. Outside a terminal (a stream piped or
redirected, or `CI` set) it prints only the version line, with no request,
nothing on stderr and no cache write: `v=$(pdfdebug --version 2>&1)` captures
exactly `pdfdebug version X`.

The CLI keeps `updatecheck-cli.json` and the app `updatecheck-app.json` in one
cache directory, so neither needs or overwrites the other. Each keeps the last
attempt apart from the last success, so a failed attempt stops the CLI retrying
for a day without counting as an answer. The CLI also reads an app record whose
last success is under a day old, uses whichever names the newer release, and
skips its own fetch; it ignores an older one. The app reads only its own record,
since it checks up to five pages of releases to the CLI's one. The directory is
`~/Library/Caches/pdfdebug/` (macOS), `~/.cache/pdfdebug/` (Linux) or
`%LOCALAPPDATA%\cache\pdfdebug\` (Windows). An absolute `XDG_CACHE_HOME`
replaces the base directory on all three; a relative one is ignored. On macOS
one exported in a shell profile reaches the CLI but not an app launched from
Finder or the Dock, so the two then keep separate caches.
