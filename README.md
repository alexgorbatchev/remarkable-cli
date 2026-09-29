# remarkable-sync

Command-line tool for synchronizing and rendering reMarkable tablet daily planner notes directly over the reMarkable Cloud API in seconds, without requiring slow USB exports or device modifications.

# What It Does

- **Fast Selective Sync**: Downloads only the ~20 KB vector stroke files (`.rm`) for requested dates rather than downloading the entire multi-hundred-page annual PDF over USB.
- **Layer-Aware Vector Rendering**: Layers highlighter strokes underneath pen ink in painter's order with Paper Pro 24-bit BGRA color decoding and crisp 200 DPI PNG outputs.
- **Upfront Disk Check**: Skips dates that are already captured on disk to eliminate redundant network transfers and rendering churn.
- **Dual-Mode Output**: Full visual feedback with aligned trees for human operators, and token-conservative line-by-line output when `AGENT=1` is set.
- **Cobra & Tree Help**: Hierarchical commands and tree-view help screens powered by [cobra-help-tree](https://github.com/alexgorbatchev/cobra-help-tree).

# Prerequisites

- [Go](https://go.dev/) 1.26.2 or higher
- [Just](https://github.com/casey/just) task runner
- reMarkable tablet paired with cloud (token configured in `~/.rmapi`)

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

# Usage & Commands

```bash
# Check Cloud connection and token status
remarkable-sync status

# Pair with a new 8-character one-time code from https://my.remarkable.com/pair/app
remarkable-sync auth <8-CHAR-CODE>

# Sync today's planner day & notes (plus uncaptured previous business day)
remarkable-sync day sync

# Sync specific dates
remarkable-sync day sync 2026-09-28 2026-09-29

# Force re-rendering of existing dates
remarkable-sync day sync 2026-09-28 --force

# List captured planner days in the data directory
remarkable-sync day list
```

# Options & Flags

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--output-dir` | `-o` | `modules/remarkable/data` | Output directory holding page images |
| `--dpi` | | `200` | Rendering resolution DPI |
| `--force` | `-f` | `false` | Force re-rendering of existing days |
| `--config` | `-c` | `~/.rmapi` | Path to rmapi token configuration file |
| `--cache-dir` | | `~/.cache/remarkable-sync` | Path to stationery template cache directory |
| `--help` | `-h` | `false` | Display help and command tree |
| `--version` | `-v` | `false` | Display binary version |

# Architecture & Libraries

`remarkable-sync` coordinates three reusable open-source Go libraries built for the reMarkable ecosystem:

1. **[`go-rmscene`](https://github.com/alexgorbatchev/go-rmscene)**: Binary parser for reMarkable v6 `.rm` stroke files, CRDT tree structures, Paper Pro 24-bit BGRA color decoding, and layered SVG generation with highlighters placed under pen ink.
2. **[`go-remarkable-cloud`](https://github.com/alexgorbatchev/go-remarkable-cloud)**: Pure Go client for the reMarkable Cloud Sync v3 protocol (auth, endpoint discovery, root state tracking, and raw SHA-256 blob streaming).
3. **[`go-remarkable-render`](https://github.com/alexgorbatchev/go-remarkable-render)**: Compositing engine combining stationery background PDFs with vector stroke layers into high-resolution PNGs, with planner calendar date indexing.

# Acknowledgments & Attribution

- **[Rick Lupton](https://github.com/ricklupton)** (`rmscene`, `rmc`): Reverse-engineering reference for the v6 `.rm` binary tagged block structure.
- **[ddvk](https://github.com/ddvk)** (`rmapi`, `reader`): Canonical reverse-engineering of reMarkable cloud protocols and early v6 structures.
- **[juruen](https://github.com/juruen)** (`rmapi`): Original Go rMAPI cloud implementation.
- **[j6k4m8](https://github.com/j6k4m8)** (`remarkapy`): Modern Python Cloud API client reference.
- **[reHackable](https://github.com/reHackable)**: Community specifications and tooling documentation.

# License

MIT License. Copyright (c) 2026 Alex Gorbatchev and upstream contributors.
