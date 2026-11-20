# Validate and diff

The two top-level commands besides `dump`: bounded structural checks against a
profile, and a structural diff of two PDFs.

Back to the [CLI usage guide](../cli-usage.md).

### `validate`

`pdfdebug validate [--profile pdfa-1b|pdfua-1-structural|pdfcpu] [--json] [--pretty] <file>`

`validate` runs structural checks only, not full conformance; for an
authoritative verdict use veraPDF. Its structural profiles are `pdfa-1b`
(default) and `pdfua-1-structural`, and each is a named subset of its standard;
`pdfcpu`, described below, runs pdfcpu's validator instead. Every run
prints the profile's scope sentence and a `Rules checked (N)` block listing
each rule's id, spec clause, outcome (`0 found`, `N found` or `not evaluated`)
and what it checks; `--json` carries the same as `scope` and `rules`. A clean
run says "none of the N rules checked found a problem". `pdfua-1-structural`
checks catalog entries, the `/S` of each structure element, and the marked
content of each page's `/Contents`: BMC/BDC and EMC balance, MCID uniqueness,
MCID nesting and whether each MCID reaches a structure element through the
ParentTree. An MCID that does not reach the ParentTree and a structure element
without a usable `/S` are errors and exit 1; the other marked-content rules and
the catalog rules are warnings. It does not check that all real content is tagged or
marked as an artifact, and it does not read form XObject or annotation content,
alternate text, fonts, role mapping or the rest of the structure tree, so it is
not a PDF/UA-1 conformance check. The rule list changes as rules are added;
`pdfdebug --help` lists the current rules of each profile, and the `rules`
array of `validate --json` names the rules a run checked. Exit codes are under
[Exit codes](scripting.md#exit-codes).

`--profile pdfcpu` runs pdfcpu's own validator, in relaxed mode, on a
separate parse of the file: the same parse code the open uses, into its own
copy, so the displayed document is not changed by it. It has two rules. `pdfcpu-validator` is an
error: pdfcpu stops at the first problem that makes it reject the file, and
that problem is the one finding, with the object pdfcpu names (for example
`12 0 R` for a circular structure tree). `pdfcpu-notices` is a warning: each
repair, digested violation or skipped object pdfcpu records in its validation
report is one finding, written as `<disposition> during <phase>: <message>`
with ` (page N)` and ` (path P)` added when pdfcpu names them, for example `digested during validate: Type1 font CustomFont: missing required
entries Widths`. When pdfcpu can only read the file after pdfdebug corrects
broken Adler-32 checksums of object or cross-reference streams, as `Open`
does, that correction comes first as a `repaired during parse:` notice, naming
the object stream when there is one. The notice list is partial. pdfcpu records only some of its
repairs in that report and writes the rest to its log, which pdfdebug does not
read; one repair it does not report is the `/Subtype /Form` it infers for an
XObject that has a `/BBox` and no `/Subtype`. The profile exits 1 when pdfcpu
rejects the file, and also when pdfcpu panicked or its separate parse failed
after the open succeeded, which leaves both rules not evaluated with one info
problem each; it exits 0 with notices only. A file that does not open
(encrypted or unparseable) exits 2, as under `pdfua-1-structural`, and so
does a file that changed on disk or was locked by another app between the
open and the check. The desktop app runs the same check in
the background after a file opens and shows a banner when pdfcpu rejected the
file or recorded notices; its Details button opens the Validate tab on this
profile.

### `diff`

`pdfdebug diff [--json] [--pretty] [--full] <left.pdf> <right.pdf>`

Compares the object models of two PDFs, not their bytes or pixels, aligned by
structural path rather than object number. Plain output opens with a summary
line and lists changed nodes; `--full` adds the unchanged ones. `--json` gives
the tree of nodes and the summary. It exits 1 when the files differ; see
[Exit codes](scripting.md#exit-codes).
