# remarkable-cli

High-performance, generic CLI gateway for reMarkable Cloud Sync v3, vector stroke decoding, and stationery compositing.

# What It Does

- **Unified API Gateway**: Full access to reMarkable Cloud Sync v3 without requiring slow USB exports or device modifications.
- **Document & Filesystem Browsing**: List documents, render virtual folder trees, inspect metadata, and extract pages.
- **Document Synchronization**: Sync any notebook or PDF pages to disk as PNG, SVG, or raw `.rm` strokes (`doc sync`).
- **Text & Hyperlink Inspection**: Search text across pages (`doc search`) and inspect internal link targets on any page (`doc links`).
- **Stroke Reverse Engineering & SVG Conversion**: Inspect binary v6 stroke blocks, point curves, tool breakdown, and export standalone vector SVGs (`stroke inspect`, `stroke export`).
- **Layer-Aware Vector Rendering**: Layers highlighter and shader strokes underneath pen ink in painter's order with Paper Pro 24-bit BGRA color decoding and crisp 200 DPI PNG outputs.
- **Dual-Mode Output (`AGENT=1`)**: Beautiful ASCII tables, box trees, and divider bars in human mode; compact token-conservative flat TSV and key-values when `AGENT=1` is set.
- **Cobra & Tree Help**: Hierarchical commands and tree-view help screens powered by [cobra-help-tree/v2](https://github.com/alexgorbatchev/cobra-help-tree).

# Prerequisites

- [Go](https://go.dev/) 1.26 or higher
- [Just](https://github.com/casey/just) task runner
- reMarkable tablet paired with cloud (token in `~/.config/remarkable-cli/config.json` or `~/.rmapi`)

# Installation

Download the latest prebuilt binary from GitHub Releases:

```bash
curl -fsSL https://github.com/alexgorbatchev/remarkable-cli/releases/latest/download/remarkable-cli_darwin_arm64.tar.gz | tar -xz
chmod +x remarkable-cli
mv remarkable-cli ~/.local/bin/
```

Or build from source:

```bash
git clone https://github.com/alexgorbatchev/remarkable-cli.git
cd remarkable-cli
just build
```

# Command Hierarchy

All commands follow a subject-first noun-verb structure (`remarkable-cli <subject> <verb>`):

```
remarkable-cli
├── auth                              Manage reMarkable Cloud authentication and device pairing
│   ├── pair <code>                   Pair device using one-time code from my.remarkable.com
│   ├── status                        Check connection status to reMarkable Cloud API
│   ╰─ token                          Print current active bearer user token
├── doc                               Inspect, browse, and export documents and notebooks
│   ├── cat <id-or-name>              Stream document page content (pdf, text, rm strokes, or svg) to stdout
│   ├── inspect <id-or-name>          Display detailed document metadata and page structure
│   ├── links <id-or-name>            List internal and external hyperlinks on a document page
│   ├── list                          List documents and folders in reMarkable Cloud
│   ├── render <id-or-name>           Render document page with strokes to a high-resolution PNG
│   ├── search <id-or-name> <query>   Search text within document pages and return matching page numbers
│   ├── sync <id-or-name>             Synchronize document pages (png, svg, or rm) to a local directory
│   ╰─ tree                           Display virtual folder and document hierarchy as a tree
╰─ stroke                             Inspect and convert reMarkable v6 binary stroke (.rm) files
    ├── export <file.rm>              Convert binary .rm file directly into a standalone layered SVG
    ╰─ inspect <file.rm>              Inspect structure, tools, and color palette of a .rm file
```

# Recommended Workflow

The recommended way to render a page is directly by document UUID (or visible name) and page index:

```bash
# Render a specific page directly:
remarkable-cli doc render 0e40ea7e-2ee9-4f96-80cc-a7e11f28c53a --page 457 -o /tmp/oct1-notes.png
```

If you don't know the exact page index in advance, discover it with `doc search`:

```bash
# Discover matching page indices:
remarkable-cli doc search "2026 - Daily" "Oct 1"
# -> Page 196 (Day), Page 457 (Notes)

# Inspect internal link destinations from page 196:
remarkable-cli doc links "2026 - Daily" --page 196
# -> Link 5 targets page 457
```

# Usage Examples

### Authentication
```bash
# Pair tablet using code from my.remarkable.com/device/desktop/connect
remarkable-cli auth pair <code>

# Check connection health and document count
remarkable-cli auth status

# Print bearer user token for API scripting
remarkable-cli auth token
```

### Documents & Notebooks
```bash
# List documents with limit
remarkable-cli doc list --limit 10

# View full virtual folder tree
remarkable-cli doc tree

# Inspect document metadata and page stroke status
remarkable-cli doc inspect "Quick sheets" --pages

# Search for text across pages
remarkable-cli doc search "Meeting Notes" "Action items"

# Inspect hyperlinks on a page
remarkable-cli doc links "2026 - Daily" --page 193

# Stream page 0 strokes as SVG to stdout
remarkable-cli doc cat "Quick sheets" --page 0 --format svg > page0.svg

# Extract textual content of page 0
remarkable-cli doc cat "Document" --page 0 --format text

# Render page directly to PNG
remarkable-cli doc render "Quick sheets" --page 0 --output page0.png

# Sync all pages of a notebook to a directory
remarkable-cli doc sync "Meeting Notes" --output-dir ./export/ --format png
```

### Stroke Files
```bash
# Inspect binary v6 stroke file
remarkable-cli stroke inspect page.rm

# Export to SVG
remarkable-cli stroke export page.rm -o page.svg
```

# Options & Flags

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--config` | `-c` | `~/.config/remarkable-cli/config.json` | Path to credentials file (fallback: `~/.rmapi`) |
| `--cache-dir` | | `~/.cache/remarkable-cli` | Path to content-addressed blob cache directory |
| `--no-cache` | | `false` | Disable local disk caching (force network downloads) |
| `--debug` | | `false` | Log outgoing reMarkable API requests and latencies |
| `--help` | `-h` | `false` | Display command help and hierarchical tree |
| `--version` | `-v` | `false` | Display raw version string |

# Architecture & Libraries

This CLI is built on top of modular Go libraries:

- **[go-rmscene](https://github.com/alexgorbatchev/go-rmscene)**: Binary stroke decoder & SVG renderer with Paper Pro 24-bit BGRA color decoding and shader alpha wash support.
- **[go-remarkable-cloud](https://github.com/alexgorbatchev/go-remarkable-cloud)**: High-speed Cloud Sync v3 API client (device pairing, discovery, manifests, blob streaming).
- **[go-remarkable-render](https://github.com/alexgorbatchev/go-remarkable-render)**: Compositing engine combining stationery background PDFs with vector strokes into crisp high-res PNGs (`go-fitz` + `resvg-go`).
- **[cobra-help-tree/v2](https://github.com/alexgorbatchev/cobra-help-tree)**: Hierarchical tree help screens and dual-mode `AGENT=1` rendering.

# License

[MIT](LICENSE) © Alex Gorbatchev
