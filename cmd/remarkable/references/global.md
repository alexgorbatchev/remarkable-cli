---
metadata:
  created_on: 2026-10-07 07:04
  last_modified: 2026-10-07 07:04
  status: current
---

## `remarkable`

Invoke without a subcommand for root help. Global flags apply to any command.
All commands accept `--help` / `-h`; only the root accepts `--version` / `-v`.
Supplying a boolean flag enables it; `--flag=false` disables it.

| Flag | Short | Type | Default | Behavior |
| --- | --- | --- | --- | --- |
| `--config` | `-c` | `string` | `""` | Override credentials file path. |
| `--cache-dir` | — | `string` | `""` | Override the content-addressed download cache: downloaded manifests and files (metadata, PDFs, strokes) named by hash in `blobs/`. It holds no credentials and is never pruned; deleting it only forces fresh downloads. |
| `--debug` | — | `bool` | `false` | Log each cloud API request attempt, its headers result, and timing to stderr. |
| `--no-cache` | — | `bool` | `false` | Bypass the download cache: cloud commands neither read nor write it. |
| `--help` | `-h` | `bool` | `false` | Print help; agent help starts with the instruction to read this skill. |
| `--version` | `-v` | `bool` | `false` | Print only the raw version and a newline. |

Resolve credentials in this order: `--config`, `REMARKABLE_CONFIG`, an existing
`$XDG_CONFIG_HOME/remarkable-cli/config.json` (default XDG base: `~/.config`),
then `~/.rmapi`; without that XDG file or an override, pairing writes `~/.rmapi`.
A `.json` filename keeps the credential format: pairing writes `devicetoken: ...` text.

Resolve the cache in this order: `--cache-dir`, `REMARKABLE_CACHE_DIR`,
`$XDG_CACHE_HOME/remarkable-cli` (default XDG base: `~/.cache`). Set `REMARKABLE_HOST`
only to direct cloud and authentication requests to an alternate endpoint.
`AGENT` also accepts `true` and `yes`, ignoring case and surrounding whitespace.

HTTP policy is identical in normal/debug modes: 90 seconds per attempt for headers and
complete body, bounded by the command context. Hashed blob GET and replayable PUT to
`/sync/v3/files/<64-hex-hash>` get at most three attempts for timeouts,
disconnected/truncated responses, and HTTP 408/429/500/502/503/504. Replay preserves
bytes/headers; successful blob bodies are fully buffered before exposure. Backoff is 250 ms
then 500 ms, overridden by valid `Retry-After` seconds/HTTP-date. Cancellation interrupts
requests, body reads, and waits. Root/authentication/other routes get one transport
attempt. Read-only GET/HEAD and replayable blobs follow redirects with Go's ten-redirect
limit; other writes return redirects directly. The cloud library still renews rejected
authentication. Generation checks and verification remain mandatory.

## `remarkable help [command]`

Print root help or a space-separated command path, e.g. `remarkable help doc render`.
