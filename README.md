# UniDoc PDF Debugger

> Open-source, cross-platform PDF structure inspector for PDF developers, forensics analysts, and compliance engineers.

[![CI](https://github.com/unidoc/unipdf-debugger/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/unidoc/unipdf-debugger/actions/workflows/ci.yml)
[![License: Apache 2.0](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](./LICENSE)
![Platforms: macOS | Windows | Linux](https://img.shields.io/badge/platforms-macOS%20%7C%20Windows%20%7C%20Linux-lightgrey)

## Overview

UniDoc PDF Debugger is a desktop PDF structure inspector that exposes the internal object graph of any PDF file. It pairs a native GUI with a complementary CLI. Core capabilities:

- PDF object tree with lazy expansion for large documents
- Object-info panel with raw dictionary/array/stream metadata
- Content stream viewer with PDF operator decoding
- Image resource extraction and in-app preview
- CLI dumps for object tree, single object, and decoded content streams

The tool exists because no modern native PDF debugger combines structure navigation, content-stream inspection, and compliance-oriented read-only access in one place. Existing alternatives are either commercial-only, browser-limited, or abandoned.

It is built for three audiences: PDF developers building SDKs and generators who need to verify their output, forensics analysts inspecting suspect files, and compliance engineers validating PDF/A and other archival profiles.

## Screenshot

<!-- TODO: replace with real screenshot after next UI sync -->
> Screenshot pending -- will be captured once the UI stabilizes. See `docs/screenshots/README.md`.

## Installation

Three V1 distribution channels are supported. Pre-built binaries are the fastest path; building from source is required for contributors.

### Pre-built binaries (GitHub Releases)

Download from [github.com/unidoc/unipdf-debugger/releases/latest](https://github.com/unidoc/unipdf-debugger/releases/latest). Binary base name is `unipdf-debugger`.

- **macOS (arm64 or amd64)**: download `unipdf-debugger-<version>-darwin-<arch>.app.zip`, unzip, and drag the `.app` to `/Applications`. Pre-release builds (`-rc*`, `-alpha*`, `-beta*`) are labelled `-UNSIGNED` and require a Gatekeeper bypass:

  ```bash
  sudo xattr -cr /Applications/unipdf-debugger.app
  ```

  Non-RC releases are signed and notarized and need no bypass.

- **Windows (amd64)**: download `unipdf-debugger-<version>-windows-amd64.exe` and double-click. Windows SmartScreen will warn on first launch because the binary is not code-signed (Windows signing is out of V1 scope). Click "More info" -> "Run anyway".

- **Linux (amd64)**: download `unipdf-debugger-<version>-linux-amd64.tar.gz`, extract, and mark executable:

  ```bash
  tar -xzf unipdf-debugger-<version>-linux-amd64.tar.gz
  chmod +x unipdf-debugger
  ./unipdf-debugger
  ```

  Requires `libwebkit2gtk-4.1`:

  ```bash
  sudo apt-get install -y libwebkit2gtk-4.1-0
  ```

  CLI archive: `pdfdebug-<version>-linux-amd64.tar.gz`.

### Homebrew (macOS and Linux)

```bash
brew tap unidoc/tap
brew install unipdf-debugger
```

### From source

See the [Build from Source](#build-from-source) section below.

## Build from Source

### Prerequisites

- Go 1.25.x (see `go.mod` line 3; do not use 1.22 -- the older PRD pin is outdated)
- Node.js 20 LTS (CI pin; matches release artifacts)
- Wails v3 CLI `v3.0.0-alpha.74` (pin must match `go.mod`)

> Node version note: `.nvmrc` sets Node 24 for local dev convenience, but CI runs Node 20 LTS -- either works locally, but CI is the authoritative pin.

Per-platform prerequisites:

- **macOS**: `xcode-select --install` (Xcode Command Line Tools)
- **Linux**: `sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.1-dev build-essential`
- **Windows**: WebView2 runtime (preinstalled on Windows 11) plus MinGW or MSVC for the Go cgo toolchain

### Steps

```bash
# 1. Clone
git clone https://github.com/unidoc/unipdf-debugger && cd unipdf-debugger

# 2. Install the pinned Wails v3 CLI (version suffix MUST match go.mod)
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-alpha.74

# 3. Install frontend deps
npm ci --prefix frontend

# 4. Generate Wails bindings (required before the first frontend build;
#    answers "why does frontend build fail" for new contributors)
wails3 generate bindings -clean=true

# 5a. Interactive dev (hot reload)
wails3 dev

# 5b. Production GUI build
wails3 build     # or: task build

# 6. CLI build (Epic 5)
go build -o bin/pdfdebug ./cmd/cli
```

## Usage

### GUI

Open a PDF via File > Open or drag-drop into the window. Explore the object tree on the left; the detail pane in the middle shows the selected object, and the info pane on the right shows metadata. Content streams and images open inline.

### CLI

The `pdfdebug` binary (built via `go build -o bin/pdfdebug ./cmd/cli`) exposes three commands:

- `pdfdebug dump tree [--json] [--depth N] <file.pdf>` -- dump the object tree.

  ```bash
  pdfdebug dump tree --depth 3 sample.pdf
  ```

- `pdfdebug dump object [--json] --ref "N G R" <file.pdf>` -- dump a single indirect object by reference.

  ```bash
  pdfdebug dump object --ref "7 0 R" sample.pdf
  ```

- `pdfdebug dump stream [--json] --page N <file.pdf>` -- dump the decoded content stream for a page.

  ```bash
  pdfdebug dump stream --page 1 sample.pdf
  ```

## Architecture

- Go 1.25 application core with Wails v3 (alpha) binding to a native WebView
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
