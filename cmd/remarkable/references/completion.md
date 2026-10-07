---
metadata:
  created_on: 2026-10-07 07:04
  last_modified: 2026-10-07 07:04
  status: current
---

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
