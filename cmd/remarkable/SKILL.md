---
name: remarkable
description: Use when operating the remarkable CLI for cloud documents, native handwriting, rendering, authentication, or local .rm files.
author: alexgorbatchev
metadata:
  created_on: 2026-09-30 09:29
  last_modified: 2026-10-06 21:06
  status: current
---

## Execution rules

Set `AGENT=1` on every invocation or export it; select commands and options below.
Headings specify exact positional arguments: `<required>` and `[optional]`.
Commands accept only their listed options plus global flags; groups print help.

- Resolve documents with `doc list` and prefer IDs; quote names and queries with
  spaces. `<id-or-name>` accepts an ID, an exact case-sensitive display name, or a
  slash-separated folder/document path. A name or path segment matching several
  live items fails; stderr then tables `ID`, `FOLDER` (path from `/`), `REACHABLE`.
  `no` means no path reaches it (an ancestor is trashed, deleted, missing, a
  document, or looped); `FOLDER` then lists only live folders below that, or `-`.
  Pass the chosen document's ID; find one in a folder with `doc list --folder <ID>`.
- Use 0-based page indexes for search, `--page`, mappings, and link targets;
  subtract one from tablet/PDF page numbers.
- Read the exit status (below) and stdout. Agent tables are TSV with headers;
  key-value output uses `key: value`; trees use indented `*` bullets.
  Search and link results have no headers and emit no rows when empty.
- Redirect `doc cat` binary output to a file, preserving PDF and `.rm`
  bytes. Keep stderr separate from stdout; `--debug` logs go to stderr.
- A failure writes its error message to stderr once, without trailing whitespace,
  prefixed `ERR: ` in agent mode or `[ERROR] ` in human mode. Invocation errors
  print the command's usage screen before that message: unknown flags, wrong
  argument counts, missing required flags, and values rejected without cloud or
  file access (`--format`, negative `--page`, empty `doc render`/`doc archive`
  `--output`, empty `--mapping`, upload `--title`/`--folder`/`--evidence`,
  pairing-code length, non-UUID `doc import` or `doc settings transfer`
  arguments, identical settings UUIDs, blank `doc search` queries). Failures
  after a command starts, such as unreadable files or cloud errors, print no usage.
- Obtain the user's code before pairing, which overwrites credentials.
  Keep tokens private; cloud commands can renew and persist credentials.
  When the cloud rejects credentials or a pairing code, or the credentials file
  holds no token, the error ends with `: run 'remarkable auth pair <code>' with a new code from https://my.remarkable.com/device/desktop/connect`.
- Use `doc render` for PNG pages with backgrounds and strokes. `doc cat` SVG
  and `stroke export` contain strokes only. PDF text extraction and search
  operate on PDF text layers.
- `doc sync` skips solely by path existence. Use `--force` after cloud or DPI
  changes; `skipped` establishes existence, not freshness.
- If an installed binary rejects a listed command or option, check its
  `--version` and select options from its own `AGENT=1 remarkable skill`.

## Exit status

| Status | Meaning |
| --- | --- |
| `0` | Success. |
| `1` | Invocation error, a name matching several items (pass an ID), or any other failure. |
| `3` | A document or item named by an argument or by `upload-check` evidence, or a local file or directory, does not exist, or `--page` exceeds the document. |
| `4` | Credentials are missing, hold no token, or were rejected (HTTP 401/403): pair again. |
| `5` | The cloud, or a proxy to it, is unreachable, timed out, sent a response that failed partway, or failed (HTTP 5xx/408/429): retry later. A refused TLS handshake or certificate exits 1. |
| `6` | A write sent its root commit (`commit-unknown`, `committed`): inspect fresh evidence before recovery or further creation. |

With several causes: 6, then 4, 5, 3. `staged` and other write failures exit by cause.
Status 2 is never used: the Go runtime exits 2 on a crash (unrecovered panic).

## `remarkable`

Invoke without a subcommand for root help. Global flags apply to any command.
All commands accept `--help` / `-h`; only the root accepts `--version` / `-v`.
Supplying a boolean flag enables it; `--flag=false` disables it.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--config` | `-c` | `string` | `""` | Override credentials file path. |
| `--cache-dir` | — | `string` | `""` | Override the content-addressed download cache: downloaded manifests and files (metadata, PDFs, strokes) named by hash in `blobs/`. It holds no credentials and is never pruned; deleting it only forces fresh downloads. |
| `--debug` | — | `bool` | `false` | Log each cloud API request attempt, its headers result, and timing to stderr. |
| `--no-cache` | — | `bool` | `false` | Bypass the download cache: cloud commands neither read nor write it. |
| `--help` | `-h` | `bool` | `false` | Print help; agent help starts with the instruction to read this skill. |
| `--version` | `-v` | `bool` | `false` | Print only the raw version and a newline. |

Resolve credentials in this order: `--config`, `REMARKABLE_CONFIG`, an existing
`$XDG_CONFIG_HOME/remarkable-cli/config.json` (default XDG base: `~/.config`),
then `~/.rmapi`; without that XDG file or an override, pairing writes `~/.rmapi`.
A `.json` filename keeps the credential format: pairing writes `devicetoken: ...` text.

Resolve the cache in this order: `--cache-dir`, `REMARKABLE_CACHE_DIR`,
`$XDG_CACHE_HOME/remarkable-cli` (default XDG base: `~/.cache`). Set `REMARKABLE_HOST`
only to direct cloud and authentication requests to an alternate endpoint.
`AGENT` also accepts `true` and `yes`, ignoring case and surrounding whitespace.

HTTP policy is identical in normal/debug modes: 90 seconds per attempt for
headers and complete body, bounded by the command context. Hashed blob GET and
replayable PUT to `/sync/v3/files/<64-hex-hash>` get at most three attempts for
timeouts, disconnected/truncated responses, and HTTP 408/429/500/502/503/504.
Replay preserves bytes/headers; successful blob bodies are fully buffered before
exposure. Backoff is 250 ms then 500 ms, overridden by valid `Retry-After`
seconds/HTTP-date. Cancellation interrupts requests, body reads, and waits.
Root/authentication/other routes get one transport attempt. Read-only GET/HEAD
and replayable blobs follow redirects with Go's ten-redirect limit; other writes
return redirects directly. The cloud library still renews rejected authentication.
Generation checks and verification remain mandatory.

## `remarkable skill`

Print the embedded SKILL.md, including frontmatter, byte-for-byte in either mode.
Works offline without credentials or repository files. Redirect stdout to save it.

## `remarkable auth`

Authentication group: `pair`, `status`, `token`.

## `remarkable auth pair <code>`

Use an 8-character code from `https://my.remarkable.com/device/desktop/connect`.
Register a device and overwrite resolved credentials with `devicetoken` text,
mode `0600`. Emit `OK:` and the path; `--config` selects that path.

## `remarkable auth status`

Check connectivity and complete item listing; propagate underlying listing errors.
Emit `status: connected`, `generation`, `items`, and `latency_ms` key-value lines.

## `remarkable auth token`

Renew and print the bearer user token as a raw stdout line. Treat it as a secret.

## `remarkable status`

Run the same operation as `remarkable auth status`.

## `remarkable doc`

Document group; operational subcommands require cloud credentials.

## `remarkable doc list`

List TSV `ID`, `NAME`, `TYPE`, `MODIFIED`; combine filters as needed. Rows sort by
parent folder path (root first, then each folder by name and ID), name, then ID.
Names compare case-folded (Unicode full folding), then by UTF-8 bytes; IDs by
bytes; type is not a key. Items whose parent chain misses the root, such as
inside a trashed folder, come last by parent ID.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--folder` | — | `string` | `""` | Filter by parent folder ID; empty means no parent filter. |
| `--type` | — | `string` | `""` | Filter by exact type: `DocumentType` or `CollectionType`; empty means all. |
| `--query` | `-q` | `string` | `""` | Match title substring, ignoring case and surrounding query whitespace. |
| `--limit` | — | `int` | `0` | Keep the first N filtered rows of that order when positive; 0 or negative means unlimited. |

## `remarkable doc tree`

Print indented `*` bullets rooted at `/`; folder labels end in `/`; siblings sort as in
`doc list`. Items whose parent chain misses the root are omitted. List IDs with `doc list`.

## `remarkable doc inspect <id-or-name>`

Print `ID`, `Name`, `Type`, `Format`, `Pages`, `Modified` key-value lines.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--pages` | — | `bool` | `false` | Append TSV with `PAGE`, `PAGE ID`, `STROKES` columns; strokes show `empty` or `present (N bytes)`. |

## `remarkable doc search <id-or-name> <query>`

Search background PDF text case-insensitively under Unicode simple case
folding; require extractable text. Matching treats every whitespace run in page
text and query, including PDF line breaks, as one space and ignores whitespace
at the query's ends, so whole words copied from a snippet match its page; an
empty or whitespace-only query is an invocation error. Emit headerless
`page-index<TAB>snippet` per matching page; no matches produce no rows. Each
row covers its page's first counted match: the snippet holds the matched page
text plus up to 20 characters (Unicode code points) before it and 40 after, with
whitespace runs collapsed to single spaces and trimmed from both ends.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--word` | — | `bool` | `false` | Count only whole-word matches: no letter, decimal digit, or combining mark directly before or after. `Oct 1` then matches `Oct 1,` but not `Oct 12`. Without it, matches may sit inside longer words and numbers. |

## `remarkable doc links <id-or-name>`

Extract PDF links as headerless `link-index<TAB>target-page-or--<TAB>URI`.
Targets are 0-based; `-` means no internal target. A PDF page without links
emits no rows and succeeds; a document without a background PDF fails.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--page` | — | `int` | `0` | Select the 0-based PDF page; negative values are rejected. |

## `remarkable doc cat <id-or-name>`

Stream content directly to stdout; fail when the required PDF or stroke data is absent.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--page` | — | `int` | `0` | Select a 0-based page for `text`, `rm`, or `svg`; ignored for `pdf`, but negative values are rejected. |
| `--format` | — | `string` | `svg` | Choose `pdf`, `text`, `rm`, or `svg`; case and surrounding whitespace are ignored. |

`pdf` is the entire background PDF without composited handwriting; `text` is the
selected PDF page's text; `rm` is the selected page's raw binary strokes; `svg` is
that page's strokes as SVG without the background.

## `remarkable doc render <id-or-name>`

Composite background/handwriting into PNG. Emit `OK: Rendered page N to PATH`.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--page` | — | `int` | `0` | Select the 0-based page; negative values are rejected. |
| `--dpi` | — | `int` | `200` | Set rendering DPI; nonpositive values use 200. |
| `--output` | `-o` | `string` | `""` | Required nonempty destination PNG path; existing files are overwritten. |

## `remarkable doc sync <id-or-name>`

Export all pages as `OUTPUT-DIR/DOCUMENT-NAME/page-NNN.FORMAT`, starting at `000`.
Trim the document name and replace slash/backslash with `-`. Emit `written: PATH`
or `skipped: PATH` per page. Unsupported formats fail before export; PNG background
download errors propagate. Page selection is unavailable.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--output-dir` | `-o` | `string` | `.` | Parent output directory. |
| `--format` | — | `string` | `png` | Use exactly `png`, `svg`, or `rm`, or empty for `png`; PNG includes background and strokes, SVG/RM contain strokes only. |
| `--dpi` | — | `int` | `200` | Set PNG resolution; nonpositive values use 200. |
| `--force` | `-f` | `bool` | `false` | Overwrite existing page files; otherwise skip any path that exists. |

## `remarkable doc archive <id-or-name>`

Save a complete native snapshot as a ZIP at an explicit output path.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--output` | `-o` | `string` | `""` | Required nonempty ZIP destination; preserve every existing path, including a path created during export. |

Require an existing parent with hard-link support. Write a `0600` temporary ZIP
beside the destination; publish only after verification. Export every manifest
entry, requiring `.content`/`.metadata`, with this layout:

- `files/<original-native-name>`: every native attachment with unchanged
  names and bytes, including raw content/metadata, PDF, pagedata, strokes,
  metadata-only strokes, deleted-page files, and unknown attachments present.
- `evidence/document.docSchema`: unchanged document manifest bytes.
- `evidence/snapshot.json`: `format_version` 1, `document_id`,
  `document_hash`, `root` (`hash`, `generation`, `schemaVersion`),
  `manifest_sha256`, and `files` containing `name`, `hash`, `sha256`, `size`.

Raw bytes preserve both page schemas, IDs, PDF redirections, inserted/deleted
state, tags, and viewport settings. Extract `files/` for explicit mappings.
Fetch every file/manifest fresh; verify SHA-256 and sizes, the manifest address
against ordered binary file hashes, and the raw manifest's separate SHA-256.
Pin a fresh root; recheck hash/generation/schema version after download. Any root
change, including another document, fails. Compare later exports for preservation.

Emit `state: verified`, `output`, `document_id`, `document_hash`, `root_hash`,
`generation`, `manifest_sha256`, then agent TSV `NAME`, `HASH`, `SHA256`, `BYTES`.
Success requires complete downloads, hashes, and the final revision check.

Five-minute total timeout. Errors remove temporary data and preserve existing
outputs; export changes no cloud documents, but may renew credentials. Stdout can
fail after ZIP publication: inspect the path before retrying at an unused path.

## `remarkable doc import <destination-uuid>`

Import local v6 `.rm` files into initialized destination pages using a required
mapping. Preserve bytes, layers, coordinates, metadata-only files, destination PDF,
and `.content`; update metadata `lastModified`. Map future and historical ink alike.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--mapping` | — | `string` | `""` | Required nonempty path to a JSON array mapping native file paths to 0-based destination page indexes. |

Mapping format; paths are absolute or relative to the mapping file's directory:

```json
[
  {"source": "page-000.rm", "page": 0},
  {"source": "page-457.rm", "page": 457}
]
```

Require `source`/`page` per row; extract archives first. Reject unknown fields,
empty mappings, duplicate indexes, missing files, malformed blocks, and invalid
indexes. Require initialized, unique destination IDs. Existing handwriting, text,
annotations, or unsupported blocks cause page-specific conflicts; metadata-only
destination files may be replaced.

Preflight errors write no cloud data. Stage files/manifests, generation-check the
commit, broadcast, then freshly verify every uploaded byte and document/page
association. Five-minute total timeout.

Emit `state: STATE`, `uploaded: NAME` per verified file (including metadata), and
TSV `PAGE`, `PAGE ID`, `SOURCE`, `STATE` after transfer starts. Read the state:

- `verified`: root commit, native byte comparison, and associations passed.
- `staged`: uncommitted changes; unreferenced blobs may exist; includes conflicts.
- `commit-unknown`: attempted root commit, result unconfirmed.
- `committed`: root succeeded, verification failed.

Failures exit nonzero; inspect the destination before recovery. Separately test
select/move/erase on a disposable tablet document to establish native editability.

## `remarkable doc upload <pdf>`

Create a separate PDF with a fresh UUID, preserving existing documents and PDF
bytes/links. Require a title and new recovery JSON path with an existing parent.
`--folder` takes a collection UUID or empty for root. Any live item with the exact,
case-sensitive title in that folder conflicts; choose a distinct title.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--title` | — | `string` | `""` | Required nonblank title without control characters. |
| `--folder` | — | `string` | `""` | Existing, live collection UUID; empty selects root. |
| `--evidence` | — | `string` | `""` | Required new JSON file; preserve existing paths. |

```sh
AGENT=1 remarkable doc upload planner.pdf --title "Planner migration" --evidence upload.json
AGENT=1 remarkable doc upload-check upload.json
```

Validate the PDF/page count; upload `.pdf`, `.metadata`, `.content`, `.pagedata`.
`native_pages: pending-tablet-initialization` persists even after cloud success.
Open on the tablet, sync, and run `doc inspect <id-or-name> --pages` before `doc import`.

Five-minute timeout. Preflight uses fresh root/metadata; commit checks that same
hash/generation and fails on concurrent change. Emit stdout progress in both modes:
`state`, `document_id`, `title`, `folder`, `pages`, `document_hash`, `root_hash`,
`generation`, `evidence`, `native_pages`, `next_step`, and `uploaded` per freshly
confirmed attachment. Agent keys use `key: value`; states match `doc import`.
`generation` is preflight; `root_hash` is intended. Success freshly verifies UUID
association and all supplied bytes; native initialization still requires the tablet.

Write credential-free `0600` evidence before staging; durably refresh before commit
via adjacent temporary files and atomic rename. Record version 1, title, folder,
page count, native initialization state, library commit result, and each intended
file's native name/SHA-256/length. Failed staging may leave unreferenced blobs;
commit timeouts or evidence/stdout failures may follow actual creation. They return
nonzero: retain JSON and run `upload-check` before retrying, which creates a new UUID.

## `remarkable doc upload-check <evidence>`

Validate recovery JSON before credentials: attachment names must equal UUID plus
`.pdf`, `.metadata`, `.content`, `.pagedata`, each exactly once. Within five minutes,
freshly confirm UUID, document hash, attachment set, sizes, and SHA-256 against an
unchanged root. Print upload fields with `state: verified`; generation/root hash
remain creation evidence. Preserve files/documents. Missing UUID, changed document
or root, or differing bytes fails nonzero; inspect before retrying creation.
Tablet initialization and edits can legitimately change the document hash.

## `remarkable doc settings`

Document settings group.

## `remarkable doc settings transfer <source-uuid> <destination-uuid>`

Use separate UUIDs and an explicit mapping to transfer structured document/page
tags and eight viewport fields from a fresh snapshot. Both documents need native
pages (`pages` or `cPages.pages`). Change only destination `.content`; preserve
native IDs/order, CRDT fields, PDF redirections, unrelated content, and all other
files, including PDF/strokes/metadata. Stroke imports remain a separate operation.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--mapping` | — | `string` | `""` | Required nonempty path to a JSON array with exact integer source_page and destination_page keys, both 0-based. |
| `--replace-viewport` | — | `bool` | `false` | Permit differing destination viewport values/presence; report every difference. Tag conflicts remain errors. |

Save this page-map format as `settings-map.json`:

```json
[
  {"source_page": 0, "destination_page": 1},
  {"source_page": 457, "destination_page": 458}
]
```

Map all tagged source pages; omit untagged pages or use `[]` when none are tagged.
Require one JSON array, exact keys, nonnegative in-range indexes, and distinct
source/destination indexes. Before uploads, reject missing tagged mappings, unknown
tag IDs, duplicate indexes/JSON keys, unsupported types, null arrays/viewport values,
deleted pages, disagreeing schemas, or uninitialized pages. UUID/local mapping
validation precedes credentials.

Preserve complete tag payloads, timestamps, unknown properties, and order; change
only pageTag `pageId` to the mapped destination ID. Transfer document/mapped-page
tags when destination tags are empty or identical; differing nonempty tags conflict.
Retain unmapped destination tags. Numeric equality is exact throughout tags/viewport:
`9`, `9.0`, `9e0` compare equal. Preserve source numeric tokens during writes and
destination content bytes for identical transfers.

Transfer these exact viewport fields: `zoomMode`, `viewBackgroundFilter`,
`customZoomCenterX`, `customZoomCenterY`, `customZoomOrientation`,
`customZoomPageHeight`, `customZoomPageWidth`, `customZoomScale`. Copy source
presence/absence as well as values; omitted source fields remove corresponding
destination fields. Differences include additions and removals and require
`--replace-viewport`. Preserve unknown destination `customZoom*` fields.
Supported zoom modes are `bestFit`, `customFit`, `fitToHeight`, `fitToWidth`;
background filters are `off` and `fullpage`; custom orientation is `portrait`
or `landscape`; the other custom fields are numeric without a UI-range clamp.
Omitting the background filter retains firmware's adaptive text-area behavior.
Document `orientation` remains a destination field.

Pin both snapshots to one fresh root; verify every file's hash/length before upload.
Generation-check the content commit; any root change, including another document,
fails. Freshly download all source/destination files after commit and check source
preservation, unchanged destination files, complete expected content, and stable
verification root. An identical repeat rechecks everything without upload/root change.

Emit `state`, `source_id`, `destination_id`, `source_hash`, `destination_hash`,
`root_hash`, `generation`, `uploaded`. Generation is preflight; root is intended
or unchanged for no-op. Destination hash is preflight until preservation verifies
the result. Both modes show viewport old/new presence/values, including conflicts.
Agent columns: `FIELD`, `DESTINATION PRESENT`, `DESTINATION VALUE`, `SOURCE PRESENT`,
`SOURCE VALUE`; missing values use `absent` with presence `false`.

States match `doc import`; `staged` also includes preflight conflicts. Success
requires all transfer/preservation checks. Failures exit nonzero; after
`commit-unknown`/`committed`, inspect both documents before retrying manually.
Stdout can fail after an update. Five-minute timeout; evidence exposes revision
IDs/filenames, not credentials.

```sh
AGENT=1 remarkable doc settings transfer 33333333-3333-4333-8333-333333333333 44444444-4444-4444-8444-444444444444 --mapping settings-map.json
AGENT=1 remarkable doc settings transfer 33333333-3333-4333-8333-333333333333 44444444-4444-4444-8444-444444444444 --mapping settings-map.json --replace-viewport
```

## `remarkable stroke`

Local v6 `.rm` operations; cloud credentials are unnecessary.

## `remarkable stroke inspect <file.rm>`

Print key-value lines `File`, `File Size`, `Total Blocks`, `Lines`, `Points`, tool/color counts.

## `remarkable stroke export <file.rm>`

Convert to layered SVG on raw stdout, or write `--output` and emit `OK:`.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--output` | `-o` | `string` | `""` | Write SVG to this path instead of stdout; overwrite an existing file. |
| `--width` | — | `float64` | `0` | Set canvas width in points only when both width and height are positive; otherwise the renderer's 447.874-point width applies. |
| `--height` | — | `float64` | `0` | Set canvas height in points only when both width and height are positive; otherwise the renderer's 595.275-point height applies. |

## `remarkable help [command]`

Print root help or a space-separated command path, e.g. `remarkable help doc render`.

## `remarkable completion`

Shell completion group; the four generated commands remain available even when
hidden in human tree help. Each prints its script to stdout.

## `remarkable completion bash`

Load with `source <(remarkable completion bash)`; requires `bash-completion`.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--no-descriptions` | — | `bool` | `false` | Omit completion descriptions. |

## `remarkable completion zsh`

Enable with `autoload -U compinit; compinit`, then `source <(remarkable completion zsh)`.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--no-descriptions` | — | `bool` | `false` | Omit completion descriptions. |

## `remarkable completion fish`

Load with `remarkable completion fish | source`.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--no-descriptions` | — | `bool` | `false` | Omit completion descriptions. |

## `remarkable completion powershell`

Load with `remarkable completion powershell | Out-String | Invoke-Expression`.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--no-descriptions` | — | `bool` | `false` | Omit completion descriptions. |

## Workflow examples

```sh
export AGENT=1
remarkable auth status
remarkable doc list --type DocumentType --query "Daily" --limit 10
remarkable doc inspect "2026 - Daily" --pages
remarkable doc search "2026 - Daily" "Oct 1" --word
remarkable doc links "2026 - Daily" --page 0
remarkable doc cat "2026 - Daily" --page 0 --format text
remarkable doc cat "2026 - Daily" --page 0 --format rm > page-000.rm
remarkable stroke inspect page-000.rm
remarkable stroke export page-000.rm --output page-000.svg
remarkable doc render "2026 - Daily" --page 0 --dpi 200 --output page-000.png
remarkable doc sync "2026 - Daily" --output-dir exports --format png --force
remarkable doc archive "2026 - Daily" --output daily-native.zip
```
