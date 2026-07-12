# Changelog

All notable changes to UniDoc PDF Debugger are recorded here. Format follows Keep a Changelog with an added Refactored section; versions use semantic versioning.

## [0.4.0] - 2026-07-12

Epic 12 (desktop shell correctness) and Epic 13 (PDF structure inspection). Adds six dual-surface (CLI + GUI) inspection capabilities on a normalized CLI output contract. Bumps the Wails toolchain.

### Added

- Embedded data inspector: `dump embedded` lists attachments and associated files and extracts one file's bytes to stdout (`--ref`/`--name`); `dump metadata` reports the `/Info` dictionary and the XMP packet
- Font CMap and glyph-mapping inspection: `dump font --glyphs` prints the full per-code mapping table with health signals for missing or broken mappings
- Digital signature decomposition: `dump signatures` (CLI) and a GUI signatures view surface signer, certificate chain, and ByteRange coverage. Decompose-only: never claims a signature is trusted or valid
- Structural compliance validation: top-level `validate` command with `--profile pdfa-1b` (default) or `pdfua-1-structural`, a three-way exit status (0 clean / 1 errors found / 2 operational error), and jump-to-object in the GUI. Structural checks only, not full conformance; veraPDF remains the authoritative oracle
- Structural diff of two PDFs: top-level `diff` command (path-aligned, `--full` includes unchanged nodes) with a three-way exit status (0 identical / 1 differ / 2 error), plus a GUI side-by-side view. First two-document IPC method
- CLI usage guide at `docs/cli-usage.md`, linked from `README.md`

### Changed

- CLI output default is now human-readable plain text; `--json` opts into JSON and `--pretty` indents it. **Breaking for scripts** that relied on JSON-by-default (story 13-1)
- Wails v3 alpha.95 -> alpha2.103 (absorbs upstream fixes for a Linux WebKit idle freeze and macOS/Linux SIGSEGV on display change and assetserver shutdown)

### Fixed

- Cold-start file association: double-clicking a PDF while the app is still launching now opens it reliably instead of dropping the event (pull-model drain handshake)
- Signature `/Cert` and CMS parsing: certificates and CMS payloads whose DER encoding ends in a `0x00` byte are no longer dropped. The reader now slices to the exact ASN.1 length instead of trimming trailing zeros

## [0.3.1] - 2026-06-08

Patch on 0.3.0.

### Fixed

- macOS "Install pdfdebug Command in PATH" installs the CLI into `~/.local/bin` and adds a PATH helper, replacing the earlier non-standard install location

## [0.3.0] - 2026-06-08

Epic 10 (background plain-text load, in-view find, reliability hardening) and Epic 11 (bundled CLI, install-to-PATH, and a much wider CLI surface). Bumps the pdfcpu and Wails toolchains.

### Added

- Async Plain Text load: large documents load in the background and can be cancelled instantly
- Plain Text find bar: `Cmd/Ctrl+F` to find, `F3` to jump between matches, per-row match marks, a clear button, and a match-case toggle
- `pdfdebug` CLI bundled inside every desktop app archive (macOS, Windows, Linux)
- macOS `Install pdfdebug Command in PATH` menu item
- CLI `dump` subcommands exposing existing pdfcore views: `objects`, `source`, `reverserefs`, `xref`, `plaintext`, `font`, `image` (previously GUI-only)
- CLI `dump page --info` assembled per-page render view
- `ResolveRef` keystone with `--ops`, `--xobject`, `--ref`, and `--resolve` surfaced in the CLI
- CLI ergonomics: `pdfRef` on tree nodes, liberal `--ref` parsing, `--pretty`, and page-rooted `dump tree`

### Changed

- Spinner in the Plain Text loading card (dropped the elapsed counter)
- pdfcpu v0.12.0 -> v0.12.1
- Wails v3 alpha.85 -> alpha.95

### Fixed

- Inspector serialized per-document; runtime panics recovered at the Wails boundary; deterministic Close
- PDF parsing and data-correctness fixes
- Frontend hook and render-path correctness fixes
- UX behavior and shell wiring fixes
- Linux build passes `EXTRA_TAGS=gtk3` so generated bindings get the gtk3 tag

### Refactored

- Low-tier cleanup batch across the codebase

## [0.2.0] - 2026-05-22

54 commits since v0.1.0. Adds object navigation, font inspection, a pretty-printed content stream view, multi-PDF open/drag-drop, and a startup splash. Bumps the Go and pdfcpu toolchains.

### Added

- Find Object command palette (`Cmd+K`), `Navigate > Find Object` menu, Recent header, inline object-ref labels on tree
- Font Inspection View with font roster, embedded-font details, ToUnicode mapping, and absence explanations
- Object Source view and Referenced by section with parent labels and global paths
- XREF Table and Plain Text view tabs in DetailPanel; eager-fetch XREF metadata on document open so the `XREF (N)` label appears immediately
- Plain Text view up to 25 MiB (up from 5 MiB) with a "Load all" escape hatch banner
- Content stream `Format()` pretty-printer with structural indent rules for `BT/ET` and `BMC/BDC/EMC` blocks; Formatted/Raw toggle in the viewer
- CLI `dump stream --raw` flag emits verbatim decoded bytes (no JSON envelope)
- Go to Page command: `Navigate > Go to Page`, `Cmd/Ctrl+G` shortcut, dialog with input validation
- Startup splash window with 400ms min-display floor, 30s timeout fallback, and crossfade dismissal
- Multi-PDF drag-drop opens all dropped PDFs into tabs with progress dialog and unsupported-files warning
- Multi-PDF `Open...` dialog (parity with drag-drop) with a race-safe Cancel button
- Inline loading indicator on the empty state while a single PDF opens
- Lucide-react tree icons; `/Pages` distinguishes intermediate ("pages") and leaf ("page") nodes

### Changed

- Plain Text view: split layout, sticky gutter, content padding
- Stream view renders `FormattedLine[]` directly (drops client-side grouping and indent logic)
- Tokenizer treats `[`, `]`, `/`, `<<`, `>>` as operand delimiters, not row-flushing operators
- Tab close button uses pointer cursor and unnests via `Tabs.Trigger asChild`
- Brand wordmark switched to UniDoc blue (`#1a4fd6`) for light/dark theme readability
- Font endpoint unified; Wails error envelopes are parsed and surfaced as font roster errors
- Font preview: sticky table headers paint; ToUnicode table flexes to fill height; absence rows explain missing glyphs
- ErrorBanner drops `dark:` variants so toast contrast matches the light app shell
- Multi-file batch helper is shared between file-drop and menu `Open...`
- Source-grep guard flipped to strict mode after grandfathered tests were deleted
- Go 1.25 -> 1.26
- pdfcpu v0.11.1 -> v0.12.0
- Wails v3 alpha.74 -> alpha.85 (Go-side; npm runtime stays at alpha.79; alpha.86 was attempted but reverted due to an un-installable `go.mod` replace directive upstream)
- Vite dev server binds to IPv4 and pre-bundles lucide-react for the Wails dev WebView
- Windows `generate:syso` passes `ARCH` through (sync with Wails upstream)

### Fixed

- `safeCall` re-panics runtime errors instead of laundering them as `ErrMalformedPDF`
- Open dialog unsupported-files warning attaches to the last `document:opened` payload (immune to event ordering)
- Tree prevents multi-word label wrap (e.g. `Font: <name>`)
- Referenced by rows show parent label and global path

### Refactored

- `slices.Sort` / `slices.SortFunc` adopted over `sort.Strings` / `sort.Slice`
- `errors.AsType` and `t.Context()` adopted (Go 1.24/1.26 modernization)
- `b.Loop` in tokenizer benchmark

## [0.1.0] - 2026-05-06

Initial public release. GUI and CLI PDF debugger for macOS arm64, Windows amd64, and Linux amd64.

### Added

**PDF inspection:**
- Open PDFs via file dialog or drag-and-drop
- Tree panel with lazy-loading object navigation
- Object info panel showing properties for the selected node
- Detail panel with context-sensitive views for dictionaries, arrays, scalars, and streams
- Clickable reference navigation across cross-references
- Navigation history with back/forward buttons, shortcuts, and menu entries
- Error handling with dismissible banners and graceful degradation on malformed PDFs

**Content streams:**
- Content stream decoding (FlateDecode and other standard filters)
- Tokenizer with syntax highlighting and tooltips for ~70 PDF operators
- Formatted/Raw view mode toggle

**Multi-document:**
- Tab bar for working with several PDFs at once
- Per-tab isolated state (tree expansion, selection, navigation history)
- OS file association so double-clicking a PDF opens it in the app
- Single-instance enforcement: subsequent launches forward files to the running instance
- Panel-size and window-geometry persistence across sessions

**Images:**
- Image extraction from XObject Image streams with CMYK and TIFF handling
- Image preview in the detail panel with metadata table and CMYK warning

**CLI:**
- `pdfdebug dump tree` with `--depth` flag for recursive tree output
- `pdfdebug dump object --ref "N G R"` to query a single object
- `pdfdebug dump stream --page N` to decode a page content stream

**Application shell:**
- Native menu bar (platform-aware shortcuts: Cmd on macOS, Ctrl on Windows/Linux)
- Empty state with drag-and-drop zone
- Design system: theme tokens, typography, color scales
- Dismissible error and warning banners
- Reduced-motion support

**Distribution:**
- GitHub Actions CI on a 3-platform matrix (macOS, Windows, Linux)
- Release pipeline producing 6 archives per release (3 GUI + 3 CLI)
- `LICENSE` (Apache 2.0) and `NOTICE` bundled in every archive on every platform
- `SHA256SUMS.txt` published alongside each release

[0.4.0]: https://github.com/unidoc/pdfdebug/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/unidoc/pdfdebug/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/unidoc/pdfdebug/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/unidoc/pdfdebug/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/unidoc/pdfdebug/releases/tag/v0.1.0
