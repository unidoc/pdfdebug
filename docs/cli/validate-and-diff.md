# Validate and diff

The two top-level commands besides `dump`: bounded structural checks against a
profile, and a structural diff of two PDFs.

Back to the [CLI usage guide](../cli-usage.md).

### `validate`

`pdfdebug validate [--profile pdfa-1b|pdfua-1-structural] [--json] [--pretty] <file>`

`validate` runs structural checks only, not full conformance; for an
authoritative verdict use veraPDF. Its profiles are `pdfa-1b` (default) and
`pdfua-1-structural`, and each is a named subset of its standard. Every run
prints the profile's scope sentence and a `Rules checked (N)` block listing
each rule's id, spec clause, outcome (`0 found`, `N found` or `not evaluated`)
and what it checks; `--json` carries the same as `scope` and `rules`. A clean
run says "none of the N rules checked found a problem". `pdfua-1-structural`
checks catalog-level entries only and does not look at marked content, the
structure tree's contents, alternate text, fonts or the rest of the catalog, so
it is not a PDF/UA-1 conformance check. The rule list changes as rules are
added; `pdfdebug --help` lists the current rules of each profile, and the
`rules` array of `validate --json` names the rules a run checked. Exit codes
are under [Exit codes](scripting.md#exit-codes).

### `diff`

`pdfdebug diff [--json] [--pretty] [--full] <left.pdf> <right.pdf>`

Compares the object models of two PDFs, not their bytes or pixels, aligned by
structural path rather than object number. Plain output opens with a summary
line and lists changed nodes; `--full` adds the unchanged ones. `--json` gives
the tree of nodes and the summary. It exits 1 when the files differ; see
[Exit codes](scripting.md#exit-codes).
