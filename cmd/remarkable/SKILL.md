---
name: remarkable
description: Use when operating the remarkable CLI for cloud documents, native handwriting, rendering, authentication, or local .rm files.
author: alexgorbatchev
metadata:
  created_on: 2026-09-30 09:29
  last_modified: 2026-10-07 07:18
  status: current
---

## Execution rules

Set `AGENT=1` on every invocation or export it; select commands and options from
the references indexed below. Headings specify exact positional arguments:
`<required>` and `[optional]`. Commands accept only their listed options plus
global flags (`global` reference); groups print help.

- Resolve documents with `doc list` and prefer IDs; quote names and queries with
  spaces. `<id-or-name>` accepts an ID, an exact case-sensitive display name, or a
  slash-separated folder/document path. A name or path segment matching several
  live items fails; stderr then tables `ID`, `FOLDER` (path from `/`), `REACHABLE`.
  `no` means no path reaches it (an ancestor is trashed, deleted, missing, a
  document, or looped); `FOLDER` then lists only live folders below that, or `-`.
  Pass the chosen document's ID; find one in a folder with `doc list --folder <ID>`.
- Search, `--page`, mappings, and link targets use 0-based indexes (tablet/PDF page - 1).
- Read the exit status (below) and stdout. Agent tables are TSV with headers;
  key-value output uses `key: value`; trees use indented `*` bullets.
  Search/link results lack headers. A failed read exits nonzero, never as empty output.
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
- Use `doc render` for PNG pages with backgrounds and strokes; `doc cat` SVG and
  `stroke export` hold strokes only. Text extraction and search read PDF text layers.
- `doc sync` skips solely by path existence. Use `--force` after cloud or DPI
  changes; `skipped` establishes existence, not freshness.
- If an installed binary rejects a listed command or option, check its
  `--version` and select options from its own `AGENT=1 remarkable skill` and references.

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

## Write states

`doc import`, `doc upload`, and `doc settings transfer` report one of these `state`
values:

- `verified`: root commit, native byte comparison, and associations passed.
- `staged`: uncommitted changes; unreferenced blobs may exist; includes commit-time
  generation conflicts.
- `commit-unknown`: attempted root commit, result unconfirmed.
- `committed`: root succeeded, verification failed.

`doc import` and `doc upload` reject preflight conflicts, such as existing page content
or an existing title, with an error and no state.

## References

Each reference holds the complete contract of the commands it documents: positional
arguments, every flag with its shorthand, type, and default, accepted formats, output,
and side effects. Print the reference for a command before running it, using the exact
command below. `remarkable skill reference list` lists every topic with its commands.

| Print with | Read before |
| --- | --- |
| `AGENT=1 remarkable skill reference cat auth` | `auth`, `auth pair`, `auth status`, `auth token`, `status`: pairing, credentials, connectivity, bearer tokens. |
| `AGENT=1 remarkable skill reference cat completion` | `completion`, `completion bash`, `completion zsh`, `completion fish`, `completion powershell`: shell completion scripts. |
| `AGENT=1 remarkable skill reference cat documents` | `doc`, `doc list`, `doc tree`, `doc inspect`, `doc search`, `doc links`: finding documents, page counts, PDF text search, hyperlinks. |
| `AGENT=1 remarkable skill reference cat global` | `remarkable` and `help`: global flags, credential and cache locations, environment variables, HTTP timeouts, retries, redirects. |
| `AGENT=1 remarkable skill reference cat native` | `doc archive`, `doc import`: native ZIP snapshots and stroke import mappings. |
| `AGENT=1 remarkable skill reference cat pages` | `doc cat`, `doc render`, `doc sync`: PDF, text, `.rm`, SVG, and PNG page output. |
| `AGENT=1 remarkable skill reference cat settings` | `doc settings`, `doc settings transfer`: page tag and viewport transfer. |
| `AGENT=1 remarkable skill reference cat strokes` | `stroke`, `stroke inspect`, `stroke export`: local `.rm` files. |
| `AGENT=1 remarkable skill reference cat upload` | `doc upload`, `doc upload-check`: new PDF documents and their recovery evidence. |

## `remarkable skill`

Print the embedded SKILL.md, including frontmatter, byte-for-byte in either mode.
Works offline without credentials or repository files. Redirect stdout to save it.

## `remarkable skill reference`

Reference group: `list`, `cat`.

## `remarkable skill reference list`

List each command documented in a reference with that reference's topic: TSV
`TOPIC`, `COMMAND` in agent mode, a table in human mode. Topics sort by name; commands
keep reference order. Works offline without credentials or repository files.

## `remarkable skill reference cat <topic>`

Print the reference for `<topic>`, a `TOPIC` from `skill reference list`, including
frontmatter, byte-for-byte in either mode. Works offline without credentials or
repository files. Any other topic is an invocation error.

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
