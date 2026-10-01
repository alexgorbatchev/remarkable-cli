`remarkable` is a high-performance CLI gateway to reMarkable Cloud Sync v3, providing document inspection, full-text page search, internal link analysis, vector stroke extraction, and stationery compositing.

# What It Does

- **Unified cloud gateway**: Interacts directly with reMarkable Cloud Sync v3 over Wi-Fi without USB cables or device modifications.
- **Document hierarchy navigation**: Lists documents and collections, inspects page-level stroke presence, and renders virtual folder trees.
- **Full-text search & link inspection**: Searches text inside PDF documents and resolves internal navigation hyperlinks on any page.
- **Vector stroke extraction**: Decodes v6 binary `.rm` stroke files with Paper Pro 24-bit BGRA color decoding and variable shader translucency.
- **Native handwriting import**: Copies native v6 `.rm` files to explicitly mapped, empty pages of an existing cloud document while preserving their bytes and its PDF background.
- **Complete native backups**: Archives every document attachment with its original name and bytes, plus source revision and SHA-256 evidence.
- **Separate PDF uploads**: Creates a cloud document from a local multi-page PDF, preserving its bytes and links and saving recovery identity before committing.
- **High-resolution page rendering**: Composites background stationery with vector handwriting layers into crisp 200 DPI PNGs in painter's order.
- **Content-addressed disk caching**: Caches blobs and manifests by SHA-256 hash, reducing repeat renders from ~150 network requests to 1.
- **Dual-mode output**: Formats aligned ASCII tables and trees for humans, and compact token-conservative TSV for scripts and AI agents (`AGENT=1`).

# How It Works

- Authenticates against reMarkable Cloud Sync v3 using a persistent device token.
- Discovers documents and collections in cloud storage, resolving them by UUID or title.
- Retrieves content-addressed blobs for document manifests, content schemas, background templates, and vector stroke files.
- Converts vector stroke lines to SVG or composites them over stationery templates into standard PNG images.
- Imports mapped native stroke files after validating the destination's initialized pages and checking for handwriting conflicts, then downloads uploaded data to verify bytes and page associations.
- Exports a complete native document snapshot to a ZIP at an explicit output path, preserving existing local files and leaving the source document unchanged.
- Uploads a valid local PDF under an explicit title in the root or an existing folder, rejecting a duplicate title in that folder and reporting the new document UUID.

# How it Really Works

- Manifests, background PDFs, and vector stroke files are content-addressed and cached by their SHA-256 hash in `$XDG_CACHE_HOME/remarkable-cli/blobs/`, so repeat renders and unchanged pages hit local disk with zero network requests.
- Document lookups by UUID scan the root manifest in O(n) time and fetch the matching document's manifest and metadata. Name and path lookups load metadata across the library.
- Highlighter and shader strokes are grouped and rendered underneath pen ink with square linecaps to keep black handwriting sharp and legible.
- When `AGENT=1` is set in the environment, tree glyphs, borders, and column alignment spaces are omitted in favor of flat key-values and raw tab-separated lines.
- Diagnostic logs and API request counters write to stderr, while requested page content (SVG, raw text, or binary strokes) streams directly to stdout for clean shell redirection.
- Native archives download every attachment directly from the cloud, verify its hash and byte length, and check that the root hash and generation remain unchanged. A concurrent cloud change causes an error, including a change to another document. Complete ZIP bytes are published only after verification; existing output paths are preserved even if created during the export.

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

### `remarkable doc archive <id-or-name>`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--output <path>` | `-o` | none | Required ZIP output path; existing files are preserved |

```bash
remarkable doc archive "2026 - Daily" --output daily-native.zip
```

The ZIP contains `files/<original-native-name>` for every document-manifest
entry, including raw `.content`, `.metadata`, PDF, `.pagedata`, `.rm`, and
unknown attachments when present. Raw content preserves both native page
schemas, native page IDs, PDF redirections, inserted/deleted page records,
tags, viewport settings, and metadata-only stroke bytes without rendering
or rebuilding them. Extract `files/` to access the native files.

`evidence/document.docSchema` contains the original manifest bytes.
`evidence/snapshot.json` records archive format version 1, document UUID,
native document-manifest hash, root hash/generation/schema version, the
raw manifest SHA-256, and each file's native name, cloud hash, downloaded
SHA-256, and byte length. The native document-manifest hash uses the cloud's
ordered file-hash scheme; it differs from the raw manifest SHA-256.
Success prints this identity and per-file evidence to stdout in both
output modes, with `state: verified` and TSV file rows in agent mode.
Compare it with a fresh archive to check source preservation; the evidence
records the source state checked at export time.

Export uses a five-minute timeout and writes a private `0600` ZIP through
a temporary file beside the destination. Its parent directory must exist
and support hard links for publication without replacement. A failed
download, checksum mismatch, concurrent cloud change, or existing output
path returns a nonzero exit status and removes the temporary archive.
An stdout write failure occurs after publication; inspect the completed ZIP
at the printed or requested output path before retrying.
The operation exports documents with a native document manifest and
requires their `.content` and `.metadata` entries. It performs no document
uploads or edits; credentials may renew through the normal cloud client.

### `remarkable doc upload <pdf>`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--title <title>` | | none | Required nonblank display title, without control characters |
| `--folder <UUID>` | | root | Existing live destination folder UUID |
| `--evidence <path>` | | none | Required new private JSON recovery file; parent directory must exist |

```bash
remarkable doc upload planner.pdf --title "Planner migration" --evidence upload.json
remarkable doc upload-check upload.json
```

Upload creates a separate document with a fresh UUID; it preserves the PDF
bytes and links and all existing documents. An exact, case-sensitive title
collision with any live item in the destination folder causes an error.
Choose a distinct title to create another document. The actual PDF page count
is stored, while native page IDs stay uninitialized. Output explicitly reports
`native_pages: pending-tablet-initialization`. Open the document on the tablet,
let it sync, then use `doc inspect <UUID> --pages` to establish native page IDs
before importing strokes. Cloud byte verification does not establish tablet
initialization or editability.

The five-minute operation freshly checks folder/title policy and commits
against that same root generation. Output exposes `staged`, `commit-unknown`,
`committed`, and `verified`, with the UUID, intended document/root hashes,
preflight generation, page count, and confirmed attachments. JSON evidence
records those fields plus intended attachment names, SHA-256 values, and
byte lengths. It is created with `0600` permissions before staging and
durably refreshed before the commit request; existing evidence is preserved.
Neither output nor evidence includes credentials.

After a timeout, retain evidence and run `doc upload-check <evidence>` before
retrying. This read-only command freshly confirms the recorded UUID, document
hash, and attachment bytes against an unchanged root snapshot without creating
anything or modifying evidence. A changed or missing document, differing bytes,
or concurrent cloud change returns a nonzero exit status and requires inspection.
Tablet initialization and later edits can change the document hash.
Every creation retry chooses another UUID. An evidence or stdout failure after
commit can occur even though the document exists; use the recovery check.

### `remarkable stroke export`

| Flag | Short | Default | Description |
| :--- | :--- | :--- | :--- |
| `--output <path>` | `-o` | stdout | Destination file for exported SVG |
| `--width <pt>` | | `447.874` | Viewport width in points |
| `--height <pt>` | | `595.275` | Viewport height in points |

Set both width and height to positive values to override the renderer's canvas dimensions.

# License

MIT License (c) 2026 Alex Gorbatchev
