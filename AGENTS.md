# AGENTS.md

Instructions for autonomous coding agents working on `remarkable-cli`.

## What This Tool Does

`remarkable-cli` provides the `remarkable` command-line tool, a generic CLI gateway for reMarkable Cloud Sync v3, binary stroke decoding, and vector stationery compositing.

## Architecture

- `cmd/remarkable/` — Cobra CLI: root setup, help, and error reporting (`main.go`, `exit.go`), offline argument and flag validation (`validation.go`), cloud client and HTTP policy (`client.go`, `client_http.go`), commands and their output formatting (`auth.go`, `doc.go`, `links.go`, `archive.go`, `import.go`, `upload.go`, `settings.go`, `stroke.go`, `skill.go`), and the embedded agent skill (`SKILL.md`, `references/<topic>.md`).
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

## Agent-Driven Native Import Check

- Requires Bun 1.4.2+ and cloud credentials configured for `remarkable`.
- Type-check the script: `just native-import-typecheck`.
- Run outside CI: `just native-import-check <source-uuid> --page <0-based-index>`.
- Establish the user-designated source document and page from available cloud data. When the user names a date, verify its PDF text and choose a matching page with pen strokes. Do not ask the user for a destination UUID: the script creates its own disposable document and local assets. Never substitute test fixtures or an unconfirmed source.
- The recipe builds the CLI and installs locked script dependencies. It downloads the source's native strokes and PDF background, extracts the selected background into a one-page PDF, and creates a new cloud document with a fresh document UUID and native page UUID using a generation-checked root commit. It then imports the strokes into page index 0, verifies fresh native downloads, source preservation, page IDs, and PDF background bytes, and saves SVG/PNG previews and reports in a unique `.tmp/native-import-check/` directory.
- Open the printed document title on the tablet and the printed preview paths on the computer. The script prints both 0-based CLI indexes and 1-based tablet page positions.
- The agent runs the script with stdin disabled. It exits successfully after cloud verification and leaves `tablet: pending` in the report; it never reads user input or prompts in the terminal.
- Read the exit status, `setup.json`, `creation.json`, import progress, and `report.json` before claiming cloud verification passed. On failure, report the evidence directory and confirmed progress; inspect uncertain commits before retrying document creation or an import. A retry creates another document; retain the printed UUID for recovery and cleanup. Never write bearer tokens to evidence files.
- After cloud verification passes, give the user the exact destination title/UUID, tablet page positions, and clickable source/destination preview paths. Ask in chat whether imported strokes can be selected, moved, and erased, whether the background remains intact, and whether edits persist after sync and reopening.
- Keep tablet verification pending until the user explicitly reports the outcome. Record that answer separately from automated cloud verification; neither a zero exit status nor rendered previews prove tablet editability. Issue #2 remains open until the required tablet check passes.
- Keep this manual check outside the CI loop. Do not write tests for this check script; use type checking and manual execution.

## Standards & Constraints

- Cloud requests use the shared `newCloudHTTPClient` policy in both normal and debug modes. Preserve the 90-second per-attempt deadline, three-attempt immutable blob recovery, command-context cancellation, and exact request bytes/headers. Keep `http.Client.Timeout` unset because it would bound the entire retry loop rather than an attempt.
- Never automatically replay root commits, authentication writes, or other mutable operations on transport errors or redirects. Keep existing generation checks and uncertain/partial outcomes; use fresh evidence to recover document creation or mutation.
- Validate HTTP policy changes with real HTTP servers, including delayed/truncated responses, replayed body/header integrity, operation cancellation, and single-attempt lost root commits. Keep the request/redirect/recovery contract in the embedded skill's `global` reference (`cmd/remarkable/references/global.md`) synchronized.

- Go 1.26+ required.
- Subject-first noun-verb CLI structure (`cli <subject> <verb>`).
- Strict ban on emojis in both human and agent modes.
- Dual-mode formatting via `AGENT=1` required on all output pathways.
- `main` is the only printer of errors returned from command execution: the root sets `SilenceErrors`, and `main` prints each returned error once to stderr through `reportError`: one `agent.PrintStatus` line, followed for a `*cloud.AmbiguousNameError` by its candidates, read from its typed fields, as an `agent.PrintTable` table. Run the CLI through `execute`; `prepareRoot` adds Cobra's help and completion commands, then wraps every `RunE` to set `SilenceUsage`, so runtime failures print no usage screen. Never write an error that a command returns to stderr, or set `SilenceUsage`, anywhere else. The other stderr writers are diagnostics, not error reports: keep the `--debug` request trace in `debugTransport.RoundTrip` (`cmd/remarkable/client.go`), which logs each request, including a failed one, before returning, and the help-output write failures reported with `PrintErrln` by the agent-alert help wrapper (`cmd/remarkable/main.go`) and cobra-help-tree's help function.
- `main` exits with `exitStatus(err)` (`cmd/remarkable/exit.go`), the table documented in `SKILL.md` and `README.md`. Classify with `errors.Is`/`errors.As` on typed or sentinel errors, never on message text; give a new failure that belongs to a class a sentinel wrapped with `%w`. A command that writes through a root commit returns its error through `afterCommit` with its progress state, so `commit-unknown` and `committed` failures exit 6 whatever their cause. Never assign status 2: the Go runtime exits 2 on an unrecovered panic. Joined errors are classified branch by branch and the highest-precedence status wins. `cancelBody` and `bufferBlobResponse` (`cmd/remarkable/client_http.go`) report every response-body read failure except `io.EOF` as `bodyReadError`, which is how a response cut off partway exits 5. The transport inside `newCloudHTTPClient` is a clone of `http.DefaultTransport` whose `OnProxyConnectResponse` reports a CONNECT answered with a `cloudFailureStatus` (every 5xx, plus the 408 and 429 that `transientStatus` retries) as `proxyConnectError`, which exits 5; never modify `http.DefaultTransport` itself. Known limitation: an HTTP/2 stream reset before the response headers exits 1, because net/http reports it only with an unexported type.
- Validate positional arguments and flag values that need no cloud or file access in `Args` validators or `pflag.Value` types (see `cmd/remarkable/validation.go`), never only inside `RunE` or the services it calls (a service may repeat the shared check for its other callers); Cobra prints the usage screen only for errors raised before `RunE` starts. Keep checks that need files or fetched data in the command.
- Aligned tree-view help screens powered by `github.com/alexgorbatchev/cobra-help-tree/v2`.
- `--version` prints strictly the raw version string followed by a newline.

## Embedded Agent Skill

- `remarkable skill` must print `cmd/remarkable/SKILL.md` verbatim, and `remarkable skill reference cat <topic>` must print `cmd/remarkable/references/<topic>.md` verbatim. Embed both with Go's `//go:embed`; a reference's file name without `.md` is its topic, and `remarkable skill reference list` derives each topic's commands from its section headings. Do not read repository files at runtime or maintain a second copy of any skill or reference text.
- Every help screen in `AGENT=1` mode must begin with an alert instructing agents to read `AGENT=1 remarkable skill` first.
- `SKILL.md` holds the frontmatter, execution rules, exit statuses, write states, workflow examples, the `skill` command sections, and an index that gives every reference its exact `AGENT=1 remarkable skill reference cat <topic>` command and when to read it. The references hold every other command's complete contract. Document each command in exactly one ``## `<usage>` `` section across these files, and keep cross-cutting rules in `SKILL.md` rather than repeating them in references.
- Give each topic exactly one index row: ``| `AGENT=1 remarkable skill reference cat <topic>` | <commands>: <when to read> |``, where `<commands>` lists, backticked and in heading order before the first `: `, every command whose section the reference holds, without the binary name (except the root, `remarkable`) and without arguments. Update the row in the same change whenever a command is added to, moved between, or removed from a reference, and name only embedded topics wherever a skill file mentions `remarkable skill reference cat`.
- Keep `SKILL.md` and the references in sync in the same change whenever commands, positional arguments, flags, shorthands, types, defaults, accepted formats, environment variables, output contracts, or side effects change. Include every command and option, including Cobra-generated help and completion commands, so agents can operate without exploring per-command help.
- Keep `SKILL.md` within 499 lines and each reference within 100 lines, the length beyond which a reference needs its own table of contents. Split a topic into a new reference rather than exceed a limit or compress prose to fit it.
- Verify behavior against implementations and pinned dependencies, update the `last_modified` metadata of `SKILL.md` and of each changed reference, and run `just check`. `TestSkillDocumentsCommandInterface` checks `SKILL.md` and all references together against the live Cobra command tree, and `TestSkillIndexNamesEveryReference` checks the index rows and every `skill reference cat` mention against the embedded references; maintain their coverage when changing the CLI.
- Keep negative instructions in `AGENTS.md`; write affirmative usage guidance and factual command contracts in `SKILL.md` and its references.
- Do not use the embedded usage skill as guidance for developing the Go implementation, invent unsupported commands or options, treat PDF text extraction as handwriting OCR, pass 1-based page numbers as page indexes, decode binary PDF/RM output as text, expose credentials or bearer tokens in reports, or interpret `doc sync`'s `skipped` result as proof that a file matches the cloud version.
