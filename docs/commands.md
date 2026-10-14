# Commands

> **Note:** This page documents the **ckeletin-go framework's** sample/dev commands (`ping`, `config`, `dev`, …), not VaultMind's. For VaultMind's actual commands (`ask`, `search`, `note`, `index`, `doctor`, `hooks`, `version`, …) see **[AGENT_USAGE.md](AGENT_USAGE.md)** and `vaultmind --help`.

## VaultMind `note propose` and `note accept`

These two write a draft and promote it after a person reviews it. The rest of the VaultMind command reference stays in [AGENT_USAGE.md](AGENT_USAGE.md).

```bash
vaultmind note propose --question "How does the index stay current?" --finding "A write re-indexes the note before the command returns."
vaultmind note accept <id>
```

`propose` retrieves the way `ask` does. A close match (the strong relevance band, by default) becomes a proposal draft next to that note. Anything less close becomes a new draft, expanded through the note template. After the draft is written, propose re-runs the original question through the same ranking `ask` uses and reports whether that question would find the draft. The ranks are stored on the draft as `rank_before`, `rank_after`, and `rank_checked_at`. `0` means not in the hit window. The check does not record an access. `accept` flips a new draft to the schema's published status, or appends a proposal onto its target and removes the proposal file. `--json` returns the v1 envelope, including `rank_check`.

`--gap <id>` links the draft to an open gap (`closes_gap`). When `--question` is omitted, the question stored on the gap is used. Accepting that draft closes the gap. `closed_by` is the new note, or the note a patch lands on.

## VaultMind `note gap`

```bash
vaultmind note gap "How does the index stay current?"
vaultmind note gap "How does the index stay current?" --json
```

This writes `gaps/<slug>.md` with `type: gap`, `status: open`, the question, `opened_at`, and `asked_hits` (the ids `ask` returns for that question right now). `session` is set when `VAULTMIND_USER_SESSION_ID` is set. The same open question, spacing and case aside, reports the existing note (`mode: existing`) and does not write another. A closed gap does not count.

A gap is a note, so it stays in git. The index is a cache. Recall never returns a gap: `ask`, `search`, the context pack, graph neighbors, and `arc recite` leave it out, and `--include-drafts` does not bring it back. `note get` still reads it. `tree` lists it and marks `[gap]`. `tree --for` does not.

A vault that already defines note types gets `gap` when the config is loaded, without rewriting `config.yaml`. A vault's own `gap` definition is kept. A vault with no types is unchanged, and the command names `types.gap` in `.vaultmind/config.yaml` (or `vaultmind init`) as the fix.

`doctor` lists each open gap (id, question, age in days), the count of gaps opened, the count closed, the median days to close, and how many weak asks have been hinted since the earliest one. `--json` adds a `gaps` object. `schema_version` stays `v1`.

A weak or no-match `ask` appends one line to `.vaultmind/possible-gaps.jsonl`. That file is a hint, not a note. The line stores a hash of the query and its length. The raw query is stored only when `app.ask.gap_store_query` is true (default false; `VAULTMIND_APP_ASK_GAP_STORE_QUERY`). There is no flag.

## `ping` Command

Sample command demonstrating Cobra, Viper, Zerolog, and Bubble Tea integration:

```bash
./myapp ping
./myapp ping --message "Hello!" --color cyan
./myapp ping --ui
```

## `doctor` Command

Check your development environment:
```bash
task doctor
```

## `config validate` Command

Validate configuration files:
```bash
./myapp config validate
./myapp config validate --file /path/to/config.yaml
```

## `check` Command (Dev Build Only)

Run comprehensive quality checks with beautiful TUI output:
```bash
./myapp check
./myapp check --category quality
./myapp check --fail-fast --verbose
```

Categories: Development Environment, Code Quality, Architecture Validation, Dependencies, Tests.

## `dev` Command Group (Dev Build Only)

```bash
./myapp dev config    # Inspect configuration
./myapp dev doctor    # Check environment health
./myapp dev progress  # Show development progress
```

See [ADR-012](../.ckeletin/docs/adr/012-dev-commands-build-tags.md) for build tag separation.

## Adding New Commands

```bash
task generate:command name=mycommand
```

This creates the command file, metadata, and config options following the ultra-thin pattern. See [CONTRIBUTING.md](../CONTRIBUTING.md) for the full walkthrough.
