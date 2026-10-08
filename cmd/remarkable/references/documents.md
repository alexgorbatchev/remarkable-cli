---
metadata:
  created_on: 2026-10-07 07:04
  last_modified: 2026-10-08 00:30
  status: current
---

## `remarkable doc`

Document group; operational subcommands require cloud credentials.

## `remarkable doc list`

List TSV `ID`, `NAME`, `TYPE`, `MODIFIED`; filters combine. Rows sort by parent folder
path (root first, then each folder by name and ID), name, then ID. Names compare
case-folded (Unicode full folding), then by UTF-8 bytes; IDs by bytes; type is not a
key. Items whose parent chain misses the root (e.g. in a trashed folder) come last by parent ID.
Agent TSV fields escape characters failing `unicode.IsPrint`, `\`, and invalid UTF-8 bytes
using Go escapes; `strconv.UnquoteChar` decodes them.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--folder` | — | `string` | `""` | Filter by parent folder ID; empty means no parent filter. |
| `--type` | — | `string` | `""` | Filter by exact type: `DocumentType` or `CollectionType`; empty means all. |
| `--query` | `-q` | `string` | `""` | Match title substring, ignoring case and surrounding query whitespace. |
| `--limit` | — | `int` | `0` | Keep the first N filtered rows of that order when positive; 0 or negative means unlimited. |

## `remarkable doc tree`

Print indented `*` bullets rooted at `/`; folder labels end in `/`; siblings sort as in
`doc list`. Items whose parent chain misses the root are omitted. List IDs with `doc list`.
Labels escape characters failing `unicode.IsPrint`, newlines, `\`, and invalid UTF-8 bytes
using Go escapes; `strconv.UnquoteChar` decodes them.

## `remarkable doc inspect <id-or-name>`

Print `ID`, `Name`, `Type`, `Format`, `Pages`, `Modified` key-value lines. `Pages` counts
native pages; folders, and PDFs uploaded without `--initialize-pages` and not yet
opened on the tablet, report 0.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--pages` | — | `bool` | `false` | Append TSV with `PAGE`, `PAGE ID`, `STROKES` columns; strokes show `empty` or `present (N bytes)`. |

## `remarkable doc search <id-or-name> <query>`

Search background PDF text case-insensitively under Unicode simple case folding; require
extractable text. Matching treats every whitespace run in page text and query, including
PDF line breaks, as one space and ignores whitespace at the query's ends, so whole words
copied from a snippet match its page; an empty or whitespace-only query is an invocation
error. Emit headerless `page-index<TAB>snippet` per matching page; no matches produce no
rows. Each row covers its page's first counted match: the snippet holds the matched page
text plus up to 20 characters (Unicode code points) before it and 40 after, with whitespace
runs collapsed to single spaces and trimmed from both ends; characters failing `unicode.IsPrint`,
`\`, and invalid UTF-8 bytes take Go escapes decodable with `strconv.UnquoteChar`.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--word` | — | `bool` | `false` | Count only whole-word matches: no letter, decimal digit, or combining mark directly before or after. `Oct 1` then matches `Oct 1,` but not `Oct 12`. Without it, matches may sit inside longer words and numbers. |

## `remarkable doc links <id-or-name>`

Extract PDF links as headerless `link-index<TAB>target<TAB>URI<TAB>text<TAB>rect`. Internal
links have a 0-based target and URI `#page=<target+1>`; URI actions have `-` and the PDF's
URI, with any catalog `/URI /Base` prepended as text when the URI lacks `:` or starts with
it; other links have `-` and an empty URI. URIs percent-encode (`%0A`) only the bytes of
spaces, invalid UTF-8, and characters failing Go's `unicode.IsPrint`. `text` is the PDF
text overlapping the rectangle (whole characters; whitespace collapsed, trimmed; empty over
handwriting/images), escaping `\` as `\\`, invalid UTF-8 bytes as `\xNN`, and other
`unicode.IsPrint` failures as `strconv.QuoteRune` does (ESC: `\x1b`); `strconv.UnquoteChar`
decodes them. `rect` is the `/Rect` as `left,bottom,right,top` PDF points (y up, corners
ordered, page box origin and `/Rotate` not applied) in shortest float32 decimals (`98.6`),
or `+Inf`/`-Inf` for reals beyond float32. A missing or non-four-element `/Rect` prints
`0,0,0,0`; non-number elements, integers above 4294967295, and signed ones outside int32
read as 0. A linkless page succeeds with no rows; no background PDF fails the command.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--page` | — | `int` | `0` | Select the 0-based PDF page; negative values are rejected. |
