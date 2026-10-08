<p align="center">
  <img src="assets/branding/lockup.svg" alt="UniDoc PDF Debugger" width="600"/>
</p>

# UniDoc PDF Debugger

> Open-source, cross-platform PDF structure inspector for PDF developers, forensics analysts, and compliance engineers.

[![CI](https://github.com/unidoc/pdfdebug/actions/workflows/ci.yml/badge.svg?branch=master)](https://github.com/unidoc/pdfdebug/actions/workflows/ci.yml)
[![License: Apache 2.0](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](./LICENSE)
![Platforms: macOS | Windows | Linux](https://img.shields.io/badge/platforms-macOS%20%7C%20Windows%20%7C%20Linux-lightgrey)

## Overview

UniDoc PDF Debugger is a desktop PDF structure inspector that exposes the internal object graph of any PDF file. It pairs a native GUI with a complementary CLI. Core capabilities:

- Object tree with lazy expansion for large documents, showing each scalar's value
- Pages and Images navigators: every page leaf in order, and every image XObject with the pages that use it
- Object detail, with the selected object's PDF source in a pane below the navigator
- Content stream viewer with PDF operator decoding, and image preview with sample-interpretation metadata
- XREF table, embedded files, `/Info` and XMP metadata, and digital signatures (decomposed, no trust verdict)
- Structural validation against a named subset of PDF/A-1b or PDF/UA-1 rules (not a conformance verdict)
- Structural diff of two PDFs, aligned by path rather than object number
- Find an object by reference, or jump to a page by number
- A `pdfdebug` CLI that covers the same inspection surface, with plain-text or JSON output

The tool exists because no modern native PDF debugger combines structure navigation, content-stream inspection, and compliance-oriented read-only access in one place. Existing alternatives are either commercial-only, browser-limited, or abandoned.

It is built for three audiences: PDF developers building SDKs and generators who need to verify their output, forensics analysts inspecting suspect files, and compliance engineers validating PDF/A and other archival profiles.

## Screenshot

![UniDoc PDF Debugger main window with two files open in tabs. The Structure navigator on the left rail is selected; the document tree is expanded down to a page's /Contents stream, with that object's source in the Object Source pane below the tree. The Object tab on the right shows the formatted, syntax-highlighted content stream.](docs/screenshots/main-window.png)

![The Images navigator listing 220 images in Flat view, sorted by first use, one row per image with its reference, size, filter, colour space, page count and badges such as SMask and APP14. The selected JPEG's dictionary is in Object Source below the list, and the Object tab shows the image preview above its Image Metadata: dimensions, colour space, filter, size, interpretation and the Adobe APP14 marker.](docs/screenshots/images-navigator.png)

## Installation

Pre-built binaries from GitHub Releases are the fastest path; building from source is required for contributors.

### Pre-built binaries (GitHub Releases)

Download from [github.com/unidoc/pdfdebug/releases/latest](https://github.com/unidoc/pdfdebug/releases/latest). Binary base name is `unidoc-pdf-debugger`.

- **macOS (Apple Silicon / arm64)**: download `unidoc-pdf-debugger-<version>-darwin-arm64.dmg`, double-click to mount, and drag `UniDoc PDF Debugger.app` onto the `Applications` shortcut. **macOS builds are currently unsigned** (see [macOS unsigned builds](#macos-unsigned-builds) below) -- first launch requires a Gatekeeper bypass:

  ```bash
  sudo xattr -cr "/Applications/UniDoc PDF Debugger.app"
  ```

  Or use the GUI: double-click the `.app` and dismiss the "blocked" dialog, then open `System Settings -> Privacy & Security`, scroll to the message that says `"UniDoc PDF Debugger" was blocked to protect your Mac`, and click `Open Anyway`. (Note: pre-Sequoia macOS supported a right-click -> Open shortcut here; Apple removed that path in macOS 15. The Privacy & Security flow above is the current supported route.)

- **Windows (amd64)**: download `unidoc-pdf-debugger-<version>-windows-amd64.zip`, extract, and double-click `UniDoc PDF Debugger.exe`. Windows SmartScreen will warn on first launch because the binary is not code-signed (Windows signing is out of V1 scope). Click "More info" -> "Run anyway". CLI archive: `pdfdebug-cli-<version>-windows-amd64.zip` (extract for `pdfdebug.exe`).

- **Linux (amd64)**: download `unidoc-pdf-debugger-<version>-linux-amd64.tar.gz`, extract, and mark executable:

  ```bash
  tar -xzf unidoc-pdf-debugger-<version>-linux-amd64.tar.gz
  chmod +x unidoc-pdf-debugger
  ./unidoc-pdf-debugger
  ```

  Requires `libwebkit2gtk-4.1`:

  ```bash
  sudo apt-get install -y libwebkit2gtk-4.1-0
  ```

  CLI archive: `pdfdebug-cli-<version>-linux-amd64.tar.gz`.

Every archive ships `LICENSE` and `NOTICE` alongside the binary (Apache 2.0 attribution).

### From source

See the [Build from Source](#build-from-source) section below.

### macOS unsigned builds

All macOS releases are currently distributed unsigned. Apple Developer Program enrollment is not yet set up for this project, so the release pipeline ships .app bundles without a Developer ID signature and without notarization.

What this means for end users:

- **First launch fails with a Gatekeeper warning.** macOS attaches a quarantine attribute to anything downloaded via browser, and Gatekeeper blocks unsigned bundles by default.
- **Workaround**: run `sudo xattr -cr "/Applications/UniDoc PDF Debugger.app"` once after install (recommended; works on every macOS version). GUI alternative on macOS 15 (Sequoia) and later: double-click the `.app`, dismiss the "blocked" dialog, then open `System Settings -> Privacy & Security` and click `Open Anyway` next to the security message. Apple removed the older right-click -> Open shortcut in Sequoia.
- **CLI binary (`pdfdebug`) is also Gatekeeper-blocked on macOS 15 (Sequoia) and later.** Browser downloads carry `com.apple.quarantine`, which `tar` propagates to the extracted binary; Sequoia tightened enforcement so even terminal invocations are blocked on first run. After extracting the CLI archive, clear the quarantine attribute before running:

  ```bash
  tar -xzf pdfdebug-cli-<version>-darwin-arm64.tar.gz
  xattr -d com.apple.quarantine pdfdebug
  chmod +x pdfdebug
  ./pdfdebug --help
  ```

  GUI alternative: try to run the CLI, dismiss the "blocked" dialog, then open `System Settings -> Privacy & Security` and click `Allow Anyway` next to the security message for `pdfdebug`. Earlier macOS versions did not block CLI binaries this way; this section will simplify back to "CLI is unaffected" if Apple eases the policy or once the project ships notarized binaries.
- **No security implication beyond the trust signal.** The bundle is the same code as a signed build would be; only the cryptographic identity from Apple is missing.

When Apple Developer Program enrollment is set up, this section will be removed and releases will ship signed (and possibly notarized) without requiring any user-side workaround. See `CONTRIBUTING.md` for the maintainer-side steps to enable signing.

## Build from Source

### Prerequisites

- Go 1.27.x
- Node.js 20 LTS (CI pin; matches release artifacts)
- Wails v3 CLI `v3.0.0-beta.18`

> Node version note: `.nvmrc` sets Node 24 for local dev convenience, but CI runs Node 20 LTS -- either works locally, but CI is the authoritative pin.

Per-platform prerequisites:

- **macOS**: `xcode-select --install` (Xcode Command Line Tools)
- **Linux**: `sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.1-dev build-essential`
- **Windows**: WebView2 runtime (preinstalled on Windows 11) plus MinGW or MSVC for the Go cgo toolchain

### Steps

```bash
# 1. Clone
git clone https://github.com/unidoc/pdfdebug && cd pdfdebug

# 2. Install the pinned Wails v3 CLI (version suffix MUST match go.mod)
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.18

# 3. Install frontend deps
npm ci --prefix frontend

# 4. Generate Wails bindings (required before the first frontend build;
#    answers "why does frontend build fail" for new contributors)
wails3 generate bindings -clean=true

# 5a. Interactive dev (hot reload)
wails3 dev

# 5b. Production GUI build
wails3 build     # or: task build
task package     # packaged build; on macOS this is the .app bundle

# 6. CLI build
go build -o bin/pdfdebug ./cmd/cli
```

## Usage

### GUI

Open a PDF via File > Open... or drag and drop it onto the window; each file gets its own tab. The left rail switches the left pane between Structure (the object tree, shown by default), Pages and Images (Cmd/Ctrl+1, 2 and 3). Whichever is chosen, Object Source sits below it and shows the selected object as PDF syntax. The detail panel on the right has tabs for Object, XREF, Plain Text, Embedded, Metadata, Validate and Diff, plus Signatures when the file has signature fields. The Object tab shows a dictionary, a decoded content stream or an image preview, depending on what is selected. Navigate > Find Object... (Cmd/Ctrl+K) jumps to an object by reference, and Go to Page... (Cmd/Ctrl+G) opens the Pages jump field.

### CLI

The `pdfdebug` binary (built via `go build -o bin/pdfdebug ./cmd/cli`) inspects a PDF's
internal structure from the command line. Its `dump` command covers PDF resources (object
tree, objects, streams, fonts, images, xref, embedded files, metadata, signatures, and
more), alongside top-level `validate` and `diff` commands.

Every command prints **human-readable plain text by default**; pass `--json` to get
structured JSON instead. The plain-text output is for reading and may change between
releases -- if you are scripting or feeding an agent, parse the `--json` form, which is the
stable contract.

See the **[CLI Usage guide](./docs/cli-usage.md)** for the full command map and flags, or
run `pdfdebug --help`.

```bash
pdfdebug dump tree --depth 3 sample.pdf
pdfdebug dump object --ref "7 0 R" sample.pdf
pdfdebug dump pages sample.pdf
pdfdebug validate --profile pdfua-1-structural sample.pdf
pdfdebug diff old.pdf new.pdf
```

## Architecture

- Go 1.27 application core with Wails v3 (beta) binding to a native WebView
- React 18 with TypeScript on the frontend; Vite build pipeline
- PDF parsing via [pdfcpu](https://github.com/pdfcpu/pdfcpu)
- Tailwind CSS utility styling with a shadcn-style component library (Radix UI primitives under the hood)
- `internal/pdfcore/` is a pure Go package with ZERO Wails dependency, so it can be imported from the CLI, the Wails service layer, and acceptance tests without carrying GUI runtime baggage
- Per-platform binaries are produced via `wails3 build`; the CLI is a plain `go build` artifact

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for the full contributor guide, including dev setup, test commands, code style, PR process, and release procedure.

## License

Apache License 2.0 (c) 2026 UniDoc ehf. See [LICENSE](./LICENSE) and [NOTICE](./NOTICE) for full terms and third-party attributions.

UniDoc PDF Debugger is a community-driven companion to UniDoc's commercial PDF toolkit -- see [unidoc.io](https://unidoc.io) for enterprise PDF solutions.
