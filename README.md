# remarkable-sync

High-performance CLI gateway and planner synchronizer for reMarkable Cloud Sync v3, vector stroke parsing, and stationery compositing.

# What It Does

- **Unified API Gateway**: Full access to reMarkable Cloud Sync v3 without requiring slow USB exports or device modifications.
- **Fast Planner Sync**: Fetches only the ~20 KB vector stroke files (`.rm`) for requested dates rather than downloading the entire multi-hundred-page annual PDF.
- **Document & Filesystem Browsing**: List documents, render virtual folder trees, inspect metadata, and extract pages.
- **Stroke Reverse Engineering & SVG Conversion**: Inspect binary v6 stroke blocks, point curves, tool breakdown, and export standalone vector SVGs.
- **Layer-Aware Vector Rendering**: Layers highlighter and shader strokes underneath pen ink in painter's order with Paper Pro 24-bit BGRA color decoding and crisp 200 DPI PNG outputs.
- **Dual-Mode Output (`AGENT=1`)**: Beautiful ASCII tables, box trees, and divider bars in human mode; compact token-conservative flat TSV and key-values when `AGENT=1` is set.
- **Cobra & Tree Help**: Hierarchical commands and tree-view help screens powered by [cobra-help-tree/v2](https://github.com/alexgorbatchev/cobra-help-tree).

# Prerequisites

- [Go](https://go.dev/) 1.26 or higher
- [Just](https://github.com/casey/just) task runner
- reMarkable tablet paired with cloud (token in `~/.config/remarkable-sync/config.json` or `~/.rmapi`)

# Installation

Download the latest prebuilt binary from GitHub Releases:

```bash
curl -fsSL https://github.com/alexgorbatchev/remarkable-sync/releases/latest/download/remarkable-sync_darwin_arm64.tar.gz | tar -xz
chmod +x remarkable-sync
mv remarkable-sync ~/.local/bin/
```

Or build from source:

```bash
git clone https://github.com/alexgorbatchev/remarkable-sync.git
cd remarkable-sync
just build
```

# Command Hierarchy

All commands follow a subject-first noun-verb structure (`remarkable-sync <subject> <verb>`):

```
remarkable-sync
├── auth                              Manage reMarkable Cloud authentication and device pairing
│   ├── pair <code>                   Pair device using one-time code from my.remarkable.com
│   ├── status                        Check connection status to reMarkable Cloud API
│   ╰─ token                          Print current active bearer user token
├── doc                               Inspect, browse, and export documents and notebooks
│   ├── cat <id-or-name>              Stream document page content (pdf, rm strokes, or svg) to stdout
│   ├── inspect <id-or-name>          Display detailed document metadata and page structure
│   ├── list                          List documents and folders in reMarkable Cloud
│   ├── render <id-or-name>           Render document page with strokes to a high-resolution PNG
│   ╰─ tree                           Display virtual folder and document hierarchy as a tree
├── planner                           Synchronize and inspect reMarkable daily planner pages
│   ├── inspect <date>                Inspect planner captures for a specific YYYY-MM-DD date
│   ├── list                          List all captured planner dates present in output directory
│   ╰─ sync [dates...]                Sync daily planner pages directly from reMarkable Cloud in seconds
╰─ stroke                             Inspect and convert reMarkable v6 binary stroke (.rm) files
    ├── export <file.rm>              Convert binary .rm file directly into a standalone layered SVG
    ╰─ inspect <file.rm>              Inspect structure, tools, and color palette of a .rm file
```

# Usage Examples

### Authentication
```bash
# Pair tablet using code from my.remarkable.com/device/desktop/connect
remarkable-sync auth pair <code>

# Check connection health and document count
remarkable-sync auth status

# Print bearer user token for API scripting
remarkable-sync auth token
```

### Documents & Notebooks
```bash
# List documents with limit
remarkable-sync doc list --limit 10

# View full virtual folder tree
remarkable-sync doc tree

# Inspect document metadata
remarkable-sync doc inspect "2026 - Daily"

# Stream page 0 strokes as SVG to stdout
remarkable-sync doc cat "2026 - Daily" --page 0 --format svg > page0.svg

# Render page 5 as 200 DPI PNG
remarkable-sync doc render "2026 - Daily" --page 5 --output page5.png
```

### Daily Planner Synchronization
```bash
# Sync today + last uncaptured business day
remarkable-sync planner sync

# Sync specific dates
remarkable-sync planner sync 2026-09-28 2026-09-29

# Force re-rendering of existing dates
remarkable-sync planner sync 2026-09-28 --force

# List captured dates on disk
remarkable-sync planner list
```

### Stroke Files
```bash
# Inspect binary v6 stroke file
remarkable-sync stroke inspect page.rm

# Export to SVG
remarkable-sync stroke export page.rm -o page.svg
```

# Options & Flags

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--config` | `-c` | `~/.config/remarkable-sync/config.json` | Path to credentials file (fallback: `~/.rmapi`) |
| `--cache-dir` | | `~/.cache/remarkable-sync` | Path to stationery template cache directory |
| `--output-dir` | `-o` | `modules/remarkable/data` | Output directory for planner page captures |
| `--dpi` | | `200` | Rendering resolution DPI |
| `--force` | `-f` | `false` | Force re-rendering of existing planner days |
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
