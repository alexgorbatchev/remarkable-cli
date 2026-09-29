# AGENTS.md

Instructions for autonomous coding agents working on `remarkable-sync`.

## What This Tool Does

`remarkable-sync` is a generic CLI gateway for reMarkable Cloud Sync v3, binary stroke decoding, and vector stationery compositing.

## Architecture

- `cmd/remarkable-sync/` — Cobra CLI command implementations (`auth.go`, `doc.go`, `stroke.go`, `main.go`).
- `internal/agent/` — Dual-mode formatting primitives (`IsAgentMode`, `PrintTable`, `PrintKeyValues`, `PrintTree`, `PrintStatus`, `PrintSeparator`).
- `internal/config/` — XDG Base Directory resolution (`$XDG_CONFIG_HOME`, `$XDG_CACHE_HOME`, fallback to `~/.rmapi`).
- `internal/doc/` — Document browsing, virtual tree construction, inspection, page streaming, and synchronization.
- `internal/stroke/` — Binary v6 `.rm` inspection, block statistics, and SVG export.

## Development Loop

- Run unit tests: `just test`
- Run lint and checks: `just check`
- Build binary: `just build` (outputs strictly to `bin/remarkable-sync`)
- Run human mode: `just run <args>`
- Run agent mode: `just run-ai <args>`

## Standards & Constraints

- Go 1.26+ required.
- Subject-first noun-verb CLI structure (`cli <subject> <verb>`).
- Strict ban on emojis in both human and agent modes.
- Dual-mode formatting via `AGENT=1` required on all output pathways.
- Aligned tree-view help screens powered by `github.com/alexgorbatchev/cobra-help-tree/v2`.
- `--version` prints strictly the raw version string followed by a newline.
