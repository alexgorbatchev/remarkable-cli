---
metadata:
  created_on: 2026-10-07 07:04
  last_modified: 2026-10-07 07:04
  status: current
---

## `remarkable doc cat <id-or-name>`

Stream to stdout; fail when the required PDF or stroke data is absent. `pdf` is the entire
background PDF without composited handwriting; `text` is the selected PDF page's text;
`rm` is that page's raw binary strokes; `svg` is its strokes as SVG without the background.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--page` | — | `int` | `0` | Select a 0-based page for `text`, `rm`, or `svg`; ignored for `pdf`, but negative values are rejected. |
| `--format` | — | `string` | `svg` | Choose `pdf`, `text`, `rm`, or `svg`; case and surrounding whitespace are ignored. |

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
