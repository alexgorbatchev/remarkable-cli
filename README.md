`remarkable` is a high-performance CLI gateway to reMarkable Cloud Sync v3, providing document inspection, full-text page search, internal link analysis, vector stroke extraction, and stationery compositing.

# What It Does

- **Unified cloud gateway**: Interacts directly with reMarkable Cloud Sync v3 over Wi-Fi without USB cables or device modifications.
- **Document hierarchy navigation**: Lists documents and collections, inspects page-level stroke presence, and renders virtual folder trees.
- **Full-text search & link inspection**: Searches text inside PDF documents and resolves internal navigation hyperlinks on any page.
- **Vector stroke extraction**: Decodes v6 binary `.rm` stroke files with Paper Pro 24-bit BGRA color decoding and variable shader translucency.
- **Native handwriting import**: Copies native v6 `.rm` files to explicitly mapped, empty pages of an existing cloud document while preserving their bytes and its PDF background.
- **High-resolution page rendering**: Composites background stationery with vector handwriting layers into crisp 200 DPI PNGs in painter's order.
- **Content-addressed disk caching**: Caches blobs and manifests by SHA-256 hash, reducing repeat renders from ~150 network requests to 1.
- **Dual-mode output**: Formats aligned ASCII tables and trees for humans, and compact token-conservative TSV for scripts and AI agents (`AGENT=1`).

# How It Works

- Authenticates against reMarkable Cloud Sync v3 using a persistent device token.
- Discovers documents and collections in cloud storage, resolving them by UUID or title.
- Retrieves content-addressed blobs for document manifests, content schemas, background templates, and vector stroke files.
- Converts vector stroke lines to SVG or composites them over stationery templates into standard PNG images.
- Imports mapped native stroke files after validating the destination's initialized pages and checking for handwriting conflicts, then downloads uploaded data to verify bytes and page associations.

# How it Really Works

- Manifests, background PDFs, and vector stroke files are content-addressed and cached by their SHA-256 hash in `$XDG_CACHE_HOME/remarkable-cli/blobs/`, so repeat renders and unchanged pages hit local disk with zero network requests.
- Document lookups by UUID scan the root manifest in O(n) time and fetch the matching document's manifest and metadata. Name and path lookups load metadata across the library.
- Highlighter and shader strokes are grouped and rendered underneath pen ink with square linecaps to keep black handwriting sharp and legible.
- When `AGENT=1` is set in the environment, tree glyphs, borders, and column alignment spaces are omitted in favor of flat key-values and raw tab-separated lines.
- Diagnostic logs and API request counters write to stderr, while requested page content (SVG, raw text, or binary strokes) streams directly to stdout for clean shell redirection.

# Installation

Download the prebuilt binary for your platform from the [latest release](https://github.com/alexgorbatchev/remarkable-cli/releases/latest), replacing `X.X.X` with the version shown on that page.

```bash
# macOS (Apple Silicon)
curl -sSL https://github.com/alexgorbatchev/remarkable-cli/releases/latest/download/remarkable_X.X.X_darwin_arm64.tar.gz | tar -xz -C ~/.local/bin
```

# Setup

- [Pairing Code](https://my.remarkable.com/device/desktop/connect) - Required for initial device registration. Run `remarkable auth pair <code>` to generate and save a device token.
- [Configuration](~/.config/remarkable-cli/config.json) - Saved automatically to `$XDG_CONFIG_HOME/remarkable-cli/config.json` with fallback to `~/.rmapi`. Override with `--config` or `REMARKABLE_CONFIG`.

# Quick Start

```bash
# Check cloud connection and document count
remarkable auth status

# Search for pages containing specific text
remarkable doc search "2026 - Daily" "Oct 1"

# Render a specific page directly to PNG
remarkable doc render 0e40ea7e-2ee9-4f96-80cc-a7e11f28c53a --page 457 -o oct1-notes.png
```

Sample Output:
```
[API #1] GET https://internal.cloud.remarkable.com/sync/v3/root -> 200 OK (242ms)
[OK]    Rendered page 457 to oct1-notes.png
```

# Options & Flags

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--config <path>` | `-c` | `~/.config/remarkable-cli/config.json` | Path to credentials file (fallback: `~/.rmapi`) |
| `--cache-dir <dir>` | | `~/.cache/remarkable-cli` | Path to content-addressed blob cache directory |
| `--no-cache` | | `false` | Disable local disk caching and force network downloads |
| `--debug` | | `false` | Log outgoing reMarkable API requests and latencies |
| `--version` | `-v` | `false` | Print raw version string |
| `--help` | `-h` | `false` | Print command line help |

### `remarkable doc list`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--folder <id>` | | none | Filter items by parent collection ID |
| `--type <type>` | | none | Filter items by type (`DocumentType`, `CollectionType`) |
| `--query <str>` | `-q` | none | Filter items matching title substring |
| `--limit <n>` | | `0` | Maximum number of items to return (0 for all) |

### `remarkable doc inspect`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--pages` | | `false` | Include page-by-page stroke presence and byte sizes |

### `remarkable doc search`

| Argument | Description |
| :--- | :--- |
| `<id-or-name>` | Document UUID or exact visible title |
| `<query>` | Text string to search across document pages |

### `remarkable doc links`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--page <n>` | | `0` | 0-based page index to extract hyperlinks from |

### `remarkable doc cat`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--page <n>` | | `0` | 0-based page index to extract |
| `--format <fmt>` | | `svg` | Stream format (`svg`, `text`, `rm`, `pdf`) |

### `remarkable doc render`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--page <n>` | | `0` | 0-based page index to render |
| `--dpi <n>` | | `200` | Rendering resolution DPI |
| `--output <path>` | `-o` | none | Destination output path for the PNG image (required) |

### `remarkable doc sync`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--output-dir <dir>` | `-o` | `.` | Destination directory for exported pages |
| `--format <fmt>` | | `png` | Page format (`png`, `svg`, `rm`) |
| `--dpi <n>` | | `200` | Rendering resolution DPI for PNGs |
| `--force` | `-f` | `false` | Overwrite existing local page files |

### `remarkable doc import <destination-uuid>`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--mapping <path>` | — | none | Required JSON file mapping native stroke paths to 0-based destination page indexes |

Use local v6 `.rm` files, including metadata-only files. Extract archives
before importing. Paths are relative to the mapping file's directory or
absolute. For example, save this as `mapping.json` beside the stroke files:

```json
[
  {"source": "page-000.rm", "page": 0},
  {"source": "page-457.rm", "page": 457}
]
```

```bash
remarkable doc import 0e40ea7e-2ee9-4f96-80cc-a7e11f28c53a --mapping mapping.json
```

The destination must have initialized native page IDs. Import preserves
source files, layers, coordinates, and native bytes, including annotations
on future dates. It preserves the destination's PDF background and page
structure and updates its modification timestamp. Existing handwriting,
text, annotations, and unsupported native destination blocks cause a
page-specific conflict; metadata-only destination files can be replaced.
Every mapping is checked before any upload.

Uploads are staged before a generation-checked root commit broadcasts the
update. The operation downloads data directly from the cloud to compare
bytes and page associations. Successful output reports `verified`. Errors
report `staged`, `commit-unknown`, or `committed`, plus confirmed uploads;
they return a nonzero exit status. Staged blobs remain unreferenced if the
root commit fails. Inspect the destination after an uncertain commit or
failed post-commit verification. The operation has a five-minute timeout.

For tablet verification, use a disposable destination and confirm that
imported handwriting can be selected, moved, and erased.

### `remarkable stroke export`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--output <path>` | `-o` | stdout | Destination file for exported SVG |
| `--width <pt>` | | `447.874` | Viewport width in points |
| `--height <pt>` | | `595.275` | Viewport height in points |

Set both width and height to positive values to override the renderer's canvas dimensions.

# License

MIT License (c) 2026 Alex Gorbatchev
