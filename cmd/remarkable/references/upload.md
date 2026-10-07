---
metadata:
  created_on: 2026-10-07 07:04
  last_modified: 2026-10-07 09:44
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
| `--initialize-pages` | — | `bool` | `false` | Create one native page per PDF page with the document (1-816 pages). |

```sh
AGENT=1 remarkable doc upload planner.pdf --title "Planner migration" --evidence upload.json --initialize-pages
AGENT=1 remarkable doc upload-check upload.json
```

Validate the PDF/page count; upload `.pdf`, `.metadata`, `.content`, `.pagedata`.
Without `--initialize-pages`, native pages stay uncreated:
`native_pages: pending-tablet-initialization` persists even after cloud success;
open on the tablet, sync, and run `doc inspect <id-or-name> --pages` before `doc import`.

With `--initialize-pages`, `native_pages: initialized`: content format 2 gives PDF
page `i` (0-based) a fresh UUID page ID, PDF redirection `i`, the `Blank` template,
and the page order the tablet assigns a PDF of that length; `.pagedata` lists `Blank`
per page. A new author UUID stamps these records. More than 816 pages fails before
preflight, evidence, or uploads. `doc inspect --pages`, `doc import`, and
`doc settings transfer` accept the document at once; tablet sync delivers it later.

Five-minute timeout. Preflight uses fresh root/metadata; commit checks that same
hash/generation and fails on concurrent change. Emit stdout progress in both modes:
`state`, `document_id`, `title`, `folder`, `pages`, `document_hash`, `root_hash`,
`generation`, `evidence`, `native_pages`, `next_step`, and `uploaded` per freshly
confirmed attachment. Agent keys use `key: value`; states are the skill's write states.
`generation` is preflight; `root_hash` is intended. Success freshly verifies UUID
association and all supplied bytes, including the initialized page structure.

Write credential-free `0600` evidence before staging; durably refresh before commit
via adjacent temporary files and atomic rename. Record version 1, title, folder,
page count, native initialization state, `page_ids` (initialized only: page IDs in
PDF order), library commit result, and each intended file's native name/SHA-256/length.
Failed staging may leave unreferenced blobs; commit timeouts or evidence/stdout
failures may follow actual creation. They return nonzero: retain JSON and run
`upload-check` before retrying, which creates a new UUID and new page IDs.

## `remarkable doc upload-check <evidence>`

Validate recovery JSON before credentials: attachment names must equal UUID plus
`.pdf`, `.metadata`, `.content`, `.pagedata`, each exactly once; `native_pages` is
`pending-tablet-initialization` without `page_ids` or `initialized` with one unique
UUID per page. Within five minutes, freshly confirm UUID, document hash, attachment
set, sizes, and SHA-256 against an unchanged root; for initialized pages, also confirm
the fresh content lists exactly `page_ids` in order with matching redirections. Print
upload fields with `state: verified`; generation/root hash remain creation evidence.
Preserve files/documents and evidence. Missing UUID, changed document or root, or
differing bytes or page IDs fails nonzero; inspect before retrying creation.
Tablet initialization, imports, and edits can legitimately change the document hash.
