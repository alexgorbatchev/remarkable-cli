---
metadata:
  created_on: 2026-10-07 07:04
  last_modified: 2026-10-07 07:04
  status: current
---

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
confirmed attachment. Agent keys use `key: value`; states are the skill's write states.
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
