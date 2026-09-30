---
name: remarkable
description: >-
  REQUIRED when using the remarkable CLI to pair, browse reMarkable Cloud,
  inspect documents, search PDF text, extract links, render or sync pages,
  or inspect and export local .rm strokes.
author: alexgorbatchev
metadata:
  created_on: 2026-09-30 09:29
  last_modified: 2026-09-30 10:03
  status: current
---

## Execution rules

Read this entire skill before invoking operational commands. Set `AGENT=1`
on every invocation, or export it for the session. Use the command reference
below instead of probing each command with `--help`.

- Select commands, arguments, flags, and formats from the reference below.
- Resolve documents with `doc list`, then prefer their IDs over titles. Quote
  names and queries containing spaces. `<id-or-name>` accepts an ID, an exact
  case-sensitive display name, or a slash-separated folder/document path;
  duplicate display names resolve to the first match.
- Use 0-based page indexes throughout, including search results, `--page`,
  and internal hyperlink targets. Convert a 1-based PDF page number to an
  index by subtracting one.
- Read exit status as well as stdout. Agent tables are TSV with headers;
  key-value output uses `key: value`; trees use indented `*` bullets.
  Search and link results have no headers and emit no rows when empty.
- Redirect `doc cat` binary output to a file, preserving PDF and `.rm`
  bytes. Keep stderr separate from stdout; `--debug` logs go to stderr.
- Obtain a pairing code from the user before `auth pair`. Pairing overwrites
  the selected credentials file. Keep credentials and bearer tokens private.
  Cloud commands can renew and persist credentials automatically.
- Use `doc render` for PNG pages with backgrounds and strokes. `doc cat` SVG
  and `stroke export` contain strokes only. PDF text extraction and search
  operate on PDF text layers.
- For `doc sync`, existing files are skipped solely by path existence. Use
  `--force` to refresh them after cloud changes; changing DPI alone does not
  refresh an existing PNG. Treat `skipped` as an existing output path.
- If a listed command or option is rejected by an installed binary, read
  `AGENT=1 remarkable skill` from that binary again and check its `--version`.
  Select options from that binary's updated skill.

## `remarkable`

Invoke without a subcommand to print root help. Append global flags to any
command. All commands accept `--help` / `-h`; only the root accepts
`--version` / `-v`. Boolean flags enable their behavior when supplied;
use `--flag=false` to disable them explicitly. Optional positional arguments
use brackets; required positional arguments use angle brackets.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--config` | `-c` | `string` | `""` | Override credentials file path. |
| `--cache-dir` | — | `string` | `""` | Override disk cache directory. |
| `--debug` | — | `bool` | `false` | Log API requests and timing to stderr for cloud commands. |
| `--no-cache` | — | `bool` | `false` | Disable blob and manifest disk caching for cloud commands. |
| `--help` | `-h` | `bool` | `false` | Print help; agent help starts with the instruction to read this skill. |
| `--version` | `-v` | `bool` | `false` | Print only the raw version and a newline. |

Resolve credentials in this order: `--config`, `REMARKABLE_CONFIG`, an
existing `$XDG_CONFIG_HOME/remarkable-cli/config.json` (default XDG base:
`~/.config`), then `~/.rmapi`. When the XDG file does not exist, pairing
writes to `~/.rmapi` unless an explicit override is supplied. The `.json`
filename does not change the credential format: pairing writes
`devicetoken: ...` text.

Resolve the cache in this order: `--cache-dir`, `REMARKABLE_CACHE_DIR`,
`$XDG_CACHE_HOME/remarkable-cli` (default XDG base: `~/.cache`). Set
`REMARKABLE_HOST` only when directing cloud and authentication requests
to an alternate endpoint. Set `AGENT=1` for agent output; `true` and `yes`
are also accepted, with case and surrounding whitespace ignored.

## `remarkable skill`

Print this embedded SKILL.md verbatim, including YAML frontmatter, to stdout.
Require no positional arguments, credentials, network access, or repository
files at runtime. Accept no command-specific options. Both output modes
print identical bytes; redirect to a file to save the skill.

## `remarkable auth`

Print authentication group help when invoked without a verb. Use `pair`,
`status`, or `token` below. Accept no command-specific options.

## `remarkable auth pair <code>`

Supply exactly one 8-character device registration code from
`https://my.remarkable.com/device/desktop/connect`. Register a device and
overwrite the resolved credentials file with its device token using mode
`0600`. Emit an `OK:` status line and the credentials path. Accept no
command-specific options; `--config` controls where credentials are saved.

## `remarkable auth status`

Check authentication and cloud connectivity. Emit `status: connected`,
`generation`, `items`, and `latency_ms` key-value lines. Accept no positional
arguments or command-specific options.

Treat success as confirmation that both connectivity and item listing completed.
Item-listing failures return an error containing the underlying cloud failure.

## `remarkable auth token`

Renew and print the active bearer user token as a raw line on stdout.
Treat that output as a secret. Accept no positional arguments or
command-specific options.

## `remarkable status`

Run the same operation as `remarkable auth status`. Accept no positional
arguments or command-specific options.

## `remarkable doc`

Print document group help when invoked without a verb. Require cloud
credentials for its operational subcommands. Accept no command-specific
options on the group.

## `remarkable doc list`

List documents and folders as TSV: `ID`, `NAME`, `TYPE`, `MODIFIED`.
Accept no positional arguments. Combine filters as needed.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--folder` | — | `string` | `""` | Filter by parent folder ID; empty means no parent filter. |
| `--type` | — | `string` | `""` | Filter by exact type: `DocumentType` or `CollectionType`; empty means all. |
| `--query` | `-q` | `string` | `""` | Match title substring, ignoring case and surrounding query whitespace. |
| `--limit` | — | `int` | `0` | Stop after this many items when positive; 0 or negative means unlimited. |

## `remarkable doc tree`

Print the cloud folder/document hierarchy as indented `*` bullets rooted
at `/`. Folder labels end in `/`. Accept no positional arguments or
command-specific options. Use `doc list` to obtain IDs.

## `remarkable doc inspect <id-or-name>`

Supply exactly one document identifier. Print `ID`, `Name`, `Type`,
`Format`, `Pages`, and `Modified` key-value lines.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--pages` | — | `bool` | `false` | Append TSV with `PAGE`, `PAGE ID`, `STROKES` columns; strokes show `empty` or `present (N bytes)`. |

## `remarkable doc search <id-or-name> <query>`

Supply exactly two arguments: a document identifier and PDF text substring.
Search case-insensitively across PDF pages. Emit one headerless TSV row per
matching page: `page-index<TAB>snippet`. Emit no rows for no matches. Require
a background PDF with extractable text. Search the PDF's text layer.
Accept no command-specific options.

## `remarkable doc links <id-or-name>`

Supply exactly one document identifier. Extract PDF hyperlink annotations.
Emit headerless TSV: `link-index<TAB>target-page-or--<TAB>URI`. Internal
targets are 0-based; `-` indicates no internal page target. Emit no rows for
a page without links; fail when the document has no PDF link annotations.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--page` | — | `int` | `0` | Select the 0-based PDF page. |

## `remarkable doc cat <id-or-name>`

Supply exactly one document identifier. Stream content directly to stdout.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--page` | — | `int` | `0` | Select a 0-based page for `text`, `rm`, or `svg`; ignored for `pdf`. |
| `--format` | — | `string` | `svg` | Choose `pdf`, `text`, `rm`, or `svg`; case and surrounding whitespace are ignored. |

- `pdf`: download the entire background PDF, without composited handwriting.
- `text`: extract text from the selected PDF page.
- `rm`: download raw binary strokes for the selected page.
- `svg`: convert the selected page's strokes to SVG without the background.

Fail when the required PDF or stroke data is absent.

## `remarkable doc render <id-or-name>`

Supply exactly one document identifier. Composite the page background and
handwriting into a PNG file. Emit `OK: Rendered page N to PATH` on success.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--page` | — | `int` | `0` | Select the 0-based page. |
| `--dpi` | — | `int` | `200` | Set rendering DPI; nonpositive values use 200. |
| `--output` | `-o` | `string` | `""` | Required destination PNG path; existing files are overwritten. |

## `remarkable doc sync <id-or-name>`

Supply exactly one document identifier. Export all document pages under
`OUTPUT-DIR/DOCUMENT-NAME/page-NNN.FORMAT`, where page numbers start at `000`.
Replace `/` and backslash in the document name with `-` and trim surrounding
whitespace. Emit `written: PATH` or `skipped: PATH` for each page. Accept no
page-selection option.

Handle an unsupported format as an error before page export begins. PNG sync
returns the underlying download error when fetching its background PDF fails.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--output-dir` | `-o` | `string` | `.` | Parent output directory. |
| `--format` | — | `string` | `png` | Use exactly `png`, `svg`, or `rm`; PNG includes background and strokes, SVG/RM contain strokes only. |
| `--dpi` | — | `int` | `200` | Set PNG resolution; nonpositive values use 200. |
| `--force` | `-f` | `bool` | `false` | Overwrite existing page files; otherwise skip any path that exists. |

## `remarkable stroke`

Print stroke group help when invoked without a verb. Operate on local v6
`.rm` files without cloud credentials. Accept no command-specific options.

## `remarkable stroke inspect <file.rm>`

Supply exactly one local stroke file path. Print `File`, `File Size`,
`Total Blocks`, `Lines`, and `Points`, followed by tool and color counts
as key-value lines. Accept no command-specific options.

## `remarkable stroke export <file.rm>`

Supply exactly one local stroke file path. Convert to layered SVG. Without
an output path, print raw SVG to stdout; with one, write the file and emit
an `OK:` status line.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--output` | `-o` | `string` | `""` | Write SVG to this path instead of stdout; overwrite an existing file. |
| `--width` | — | `float64` | `0` | Set canvas width in points only when both width and height are positive. |
| `--height` | — | `float64` | `0` | Set canvas height in points only when both width and height are positive. |

Omit dimensions to use the renderer's 447.874 by 595.275 point canvas.
Supplying only one dimension leaves both renderer defaults in effect.

## `remarkable help [command]`

Print root help with no argument, or supply a space-separated command path,
such as `remarkable help doc render`. Accept no command-specific options.
Use this skill as the operational reference; help is for troubleshooting
an installed version mismatch.

## `remarkable completion`

Print completion group help with no verb. Generate shell completion scripts
with one of the four commands below. These generated commands remain
available even though human tree help hides them. Accept no positional
arguments or command-specific options on the group.

## `remarkable completion bash`

Print a Bash completion script to stdout. Require no positional arguments.
Load in Bash with `source <(remarkable completion bash)`; require the
`bash-completion` package in that shell environment.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--no-descriptions` | — | `bool` | `false` | Omit completion descriptions. |

## `remarkable completion zsh`

Print a Zsh completion script to stdout. Require no positional arguments.
After enabling completion with `autoload -U compinit; compinit`, load
with `source <(remarkable completion zsh)`.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--no-descriptions` | — | `bool` | `false` | Omit completion descriptions. |

## `remarkable completion fish`

Print a Fish completion script to stdout. Require no positional arguments.
Load in Fish with `remarkable completion fish | source`.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--no-descriptions` | — | `bool` | `false` | Omit completion descriptions. |

## `remarkable completion powershell`

Print a PowerShell completion script to stdout. Require no positional
arguments. Load with
`remarkable completion powershell | Out-String | Invoke-Expression`.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--no-descriptions` | — | `bool` | `false` | Omit completion descriptions. |

## Workflow examples

```sh
export AGENT=1
remarkable auth status
remarkable doc list --type DocumentType --query "Daily" --limit 10
remarkable doc inspect "2026 - Daily" --pages
remarkable doc search "2026 - Daily" "Oct 1"
remarkable doc links "2026 - Daily" --page 0
remarkable doc cat "2026 - Daily" --page 0 --format text
remarkable doc cat "2026 - Daily" --page 0 --format rm > page-000.rm
remarkable stroke inspect page-000.rm
remarkable stroke export page-000.rm --output page-000.svg
remarkable doc render "2026 - Daily" --page 0 --dpi 200 --output page-000.png
remarkable doc sync "2026 - Daily" --output-dir exports --format png --force
```
