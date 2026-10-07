---
metadata:
  created_on: 2026-10-07 07:04
  last_modified: 2026-10-07 07:04
  status: current
---

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

Require `source`/`page` per row; extract archives first. Reject unknown fields, empty
mappings, duplicate or invalid indexes, missing files, and malformed blocks. Require
initialized, unique destination IDs. Existing handwriting, text, annotations, or unsupported
blocks cause page-specific conflicts; metadata-only destination files may be replaced.

Preflight errors write no cloud data. Stage files/manifests, generation-check the
commit, broadcast, then freshly verify every uploaded byte and document/page
association. Five-minute total timeout.

Emit `state: STATE`, `uploaded: NAME` per verified file (including metadata), and
TSV `PAGE`, `PAGE ID`, `SOURCE`, `STATE` after transfer starts. Read the state
as one of the skill's write states.

Failures exit nonzero; inspect the destination before recovery. Separately test
select/move/erase on a disposable tablet document to establish native editability.
