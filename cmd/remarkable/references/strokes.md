---
metadata:
  created_on: 2026-10-07 07:04
  last_modified: 2026-10-07 07:04
  status: current
---

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
