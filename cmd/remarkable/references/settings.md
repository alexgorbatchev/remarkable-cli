---
metadata:
  created_on: 2026-10-07 07:04
  last_modified: 2026-10-07 07:04
  status: current
---

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

Preserve complete tag payloads, timestamps, unknown properties, and order; change only
pageTag `pageId` to the mapped destination ID. Transfer document/mapped-page tags when
destination tags are empty or identical; differing nonempty tags conflict. Retain unmapped
destination tags. Tag/viewport numbers compare by exact value: `9` = `9.0` = `9e0`. Writes
keep source numeric tokens; identical transfers keep destination content bytes.

Transfer these exact viewport fields: `zoomMode`, `viewBackgroundFilter`,
`customZoomCenterX`, `customZoomCenterY`, `customZoomOrientation`, `customZoomPageHeight`,
`customZoomPageWidth`, `customZoomScale`. Copy source presence/absence as well as values;
omitted source fields remove corresponding destination fields. Differences include
additions and removals and require `--replace-viewport`. Preserve unknown destination
`customZoom*` fields. Supported zoom modes are `bestFit`, `customFit`, `fitToHeight`,
`fitToWidth`; background filters are `off` and `fullpage`; custom orientation is `portrait`
or `landscape`; the other custom fields are numeric without a UI-range clamp. Omitting the
background filter retains firmware's adaptive text-area behavior. Document `orientation`
remains a destination field.

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

States are the skill's write states; `staged` also includes preflight conflicts. Success
requires all transfer/preservation checks. Failures exit nonzero; after
`commit-unknown`/`committed`, inspect both documents before retrying manually. Stdout can
fail after an update. Five-minute timeout; evidence exposes revision IDs/filenames, not
credentials.

```sh
AGENT=1 remarkable doc settings transfer 33333333-3333-4333-8333-333333333333 44444444-4444-4444-8444-444444444444 --mapping settings-map.json
AGENT=1 remarkable doc settings transfer 33333333-3333-4333-8333-333333333333 44444444-4444-4444-8444-444444444444 --mapping settings-map.json --replace-viewport
```
