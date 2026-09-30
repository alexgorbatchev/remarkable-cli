# AGENTS.md

Instructions for autonomous coding agents working on `remarkable-cli`.

## What This Tool Does

`remarkable-cli` provides the `remarkable` command-line tool, a generic CLI gateway for reMarkable Cloud Sync v3, binary stroke decoding, and vector stationery compositing.

## Architecture

- `cmd/remarkable/` — Cobra CLI command implementations (`auth.go`, `doc.go`, `stroke.go`, `main.go`).
- `internal/agent/` — Dual-mode formatting primitives (`IsAgentMode`, `PrintTable`, `PrintKeyValues`, `PrintTree`, `PrintStatus`, `PrintSeparator`).
- `internal/config/` — XDG Base Directory resolution (`$XDG_CONFIG_HOME`, `$XDG_CACHE_HOME`, fallback to `~/.rmapi`).
- `internal/doc/` — Document browsing, virtual tree construction, inspection, page search, hyperlinks, and page rendering.
- `internal/stroke/` — Binary v6 `.rm` inspection, block statistics, and SVG export.
- Companion library checkouts are in `../go-rmscene`, `../go-remarkable-cloud`, and `../go-remarkable-render`; consult their source when investigating dependency behavior and compare against the versions pinned in `go.mod`.

## Development Loop

- Run unit tests: `just test`
- Run lint and checks: `just check`
- Build binary: `just build` (outputs strictly to `bin/remarkable`)
- Run human mode: `just run <args>`
- Run agent mode: `just run-ai <args>`

## Manual Native Import Check

- Requires Bun 1.4.2+ and cloud credentials configured for `remarkable`.
- Type-check the script: `just native-import-typecheck`.
- Run outside CI: `just native-import-check <disposable-destination-uuid> --mapping <mapping.json>`.
- Mapping uses the same source paths and 0-based page indexes as `doc import`; include at least one page with pen strokes.
- The recipe builds the CLI and installs locked script dependencies. It imports into the destination, verifies fresh native downloads, source preservation, page IDs, and PDF background bytes, then saves SVG/PNG previews and `report.json` in a unique `.tmp/native-import-check/` directory.
- Open the printed document title on the tablet and the printed preview paths on the computer. The script prints both 0-based CLI indexes and 1-based tablet page positions.
- After selection, movement, erasure, background, and sync/reopen checks, the user answers `y` or `n`. Only `y` records a tablet pass; `n` or EOF exits nonzero. This is a human attestation, not automated tablet UI verification.
- Keep this manual check outside the CI loop. Do not write tests for this check script; use type checking and manual execution.

## Standards & Constraints

- Go 1.26+ required.
- Subject-first noun-verb CLI structure (`cli <subject> <verb>`).
- Strict ban on emojis in both human and agent modes.
- Dual-mode formatting via `AGENT=1` required on all output pathways.
- Aligned tree-view help screens powered by `github.com/alexgorbatchev/cobra-help-tree/v2`.
- `--version` prints strictly the raw version string followed by a newline.

## Embedded Agent Skill

- `remarkable skill` must print `cmd/remarkable/SKILL.md` verbatim. Embed the file with Go's `//go:embed`; do not read repository files at runtime or maintain a second copy of the skill text.
- Every help screen in `AGENT=1` mode must begin with an alert instructing agents to read `AGENT=1 remarkable skill` first.
- Keep `cmd/remarkable/SKILL.md` in sync in the same change whenever commands, positional arguments, flags, shorthands, types, defaults, accepted formats, environment variables, output contracts, or side effects change. Include every command and option, including Cobra-generated help and completion commands, so agents can operate without exploring per-command help.
- Verify behavior against implementations and pinned dependencies, update the skill's `last_modified` metadata, and run `just check`. `TestSkillDocumentsCommandInterface` checks the printed skill against the live Cobra command tree; maintain its coverage when changing the CLI.
- Keep negative instructions in `AGENTS.md`; write affirmative usage guidance and factual command contracts in `SKILL.md`.
- Do not use the embedded usage skill as guidance for developing the Go implementation, invent unsupported commands or options, treat PDF text extraction as handwriting OCR, pass 1-based page numbers as page indexes, decode binary PDF/RM output as text, expose credentials or bearer tokens in reports, or interpret `doc sync`'s `skipped` result as proof that a file matches the cloud version.
