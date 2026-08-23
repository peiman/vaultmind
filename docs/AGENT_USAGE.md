# VaultMind — Agent Usage Guide

**For AI agents using VaultMind as memory. Covers save, retrieve, update, inspect. Machine-first explanations with real commands.**

If you're an agent integrating VaultMind into your workflow (via SessionStart hook or explicit CLI calls), read this end-to-end. If you're a human reviewing agent usage, the same material applies — agents and humans share the same CLI surface.

**Getting the binary.** Three install paths, each yielding a retrieval tier: (a) `go install github.com/peiman/vaultmind@latest` → pure-Go **MiniLM**, every platform, no deps; (b) the **prebuilt ORT archive** (`vaultmind_<version>_<os>_<arch>_ort.tar.gz` from the [release](https://github.com/peiman/vaultmind/releases), extract, run) → full **BGE-M3** 4-way hybrid on darwin-arm64/linux-amd64, no build; (c) from source (`git clone` + `task setup:ort` + `task build`) → full hybrid, any platform. To **update** an existing install, repeat your path (`@latest` / re-download the archive / `git pull && task build`). Confirm with `vaultmind version`; `vaultmind doctor` reports your retrieval tier and warns if degraded. `vaultmind doctor` also asks the Go module proxy once a day whether a newer release exists and prints a line if so — it sends nothing about you or your vault; `VAULTMIND_NO_UPDATE_CHECK=1` opts out.

**Upgrading to v0.7.0.** Index migrations apply automatically on first run — **no re-index and no `--embed`** (note bodies have been stored since the baseline schema). One thing to check by hand: the context header format changed from `N items`, so anything parsing that line needs updating. See [Reading the context header](#reading-the-context-header).

## 1. The mental model

A vault is a directory of markdown notes with YAML frontmatter, tracked in Git. VaultMind indexes that vault into a SQLite DB and provides retrieval commands. Your job as an agent is:

1. **Retrieve** what you need from the vault
2. **Save** new notes when you learn something worth keeping
3. **Update** existing notes when content evolves
4. **Inspect** the vault's health periodically

The key invariant: **the markdown files are the source of truth.** The SQLite index is a derived artifact. If you edit a file, re-run `vaultmind index` to pick up the change. Content changes auto-invalidate stale embeddings — the next `vaultmind index --embed` re-embeds.

## 2. Retrieval

### `vaultmind ask <query>` — semantic retrieval + context pack

Use when you want the agent-ready answer: top hits plus a token-budgeted context pack around the #1 result.

```bash
vaultmind ask "who am I" --vault examples/ada-vault --max-items 8 --budget 6000
vaultmind ask "how should I write notes" --vault examples/ada-vault --json
```

Output format (JSON):

```json
{
  "command": "ask",
  "result": {
    "query": "spreading activation",
    "top_hits": [{"id": "concept-spreading-activation", "score": 0.83, "title": "..."}],
    "context": { "target_id": "...", "used_tokens": 5612, "context": [...] },
    "retrieval_mode": "hybrid"
  },
  "meta": {"vault_path": "...", "index_hash": "..."}
}
```

**`retrieval_mode`** tells you which lane ran. If it's `"keyword"`, the vault has no embeddings — your paraphrase queries will miss. Run `vaultmind index --embed --model minilm --vault <path>` to fix.

#### `--excerpt N` — get the passage that matters when the whole note will not fit

Body inclusion is otherwise all-or-nothing: a note one token over the remaining budget
contributes **nothing**, while the pack still counts it. On a vault whose median note is
larger than your budget — which is most real vaults, and every hook budget — that is the
common case, not the edge case.

```bash
vaultmind ask "spreading activation" --vault <path> --budget 4000 --excerpt 120
```

With `--excerpt N` such a note contributes at most N tokens instead of zero. It prefers
the note's **Principle** section where one exists, falling back to opening prose. That
default is deliberate: arcs are written trigger → push → deeper sight → principle, so the
opening is story setup and the decision rule sits several sections down. Injecting "the
first paragraph" hands you the anecdote and withholds the lesson.

Off by default (`0`). If you are wiring a hook with a tight budget, set it — otherwise a
small budget yields items with no content.

#### Reading the context header

The header states what actually arrived, in numbers you can recount from the output
below it:

```
Context from: identity-who-i-am — 9 notes, 9 delivered as excerpts (968 tok)
Context from: concept-arc — 5 notes, 3 delivered in full, 2 titles only (95 tok)
```

`notes` counts every block that renders, including the target. An excerpt counts as
delivered *and* says it is an excerpt.

> **Changed in v0.7.0.** This replaces the old `N items` / `used/budget` form. If you
> have a hook or script parsing that line, update it.

A weak top hit in a tight, well-curated vault now delivers its body. Low contrast between
hits is a property of the vault — its notes are all about one subject — not evidence that
the top hit is wrong. Genuinely irrelevant hits still land at or below the noise floor as
`no_match` and stay suppressed.

### `vaultmind search <query>` — ranked hits only

Use when you want a hit list without the context-pack overhead. Supports `--mode hybrid | keyword | embedding | sparse | colbert` to pick a specific lane, or omit for auto-selection.

```bash
vaultmind search "judgment gap" --vault examples/ada-vault --limit 10 --json
```

### `vaultmind memory pack <target-id>` — expand around a specific note

Use when you already know the target note and want its token-budgeted neighborhood.

```bash
vaultmind memory pack arc-ask-before-assuming --vault examples/ada-vault --budget 4000 --max-items 8
```

### `vaultmind memory neighbors <note-id>` — the note's graph neighborhood with full frontmatter

Use when you want the enriched neighborhood (BFS to a depth) around a known note.

```bash
vaultmind memory neighbors identity-who-am-i --vault examples/ada-vault
```

### `vaultmind note get <id>` — full note with frontmatter

Use when you want the raw note structure (frontmatter + body) for inspection or quoting.

```bash
vaultmind note get arc-ask-before-assuming --vault examples/ada-vault --json
```

### Picking between them

- **Conversational "what do I know about X?"** → `vaultmind ask` (gives you top hits + context)
- **"Find all notes mentioning Y"** → `vaultmind search`
- **"Read this specific note"** → `vaultmind note get`
- **"Explore the graph around a note"** → `vaultmind memory neighbors`
- **"Expand around this known target"** → `vaultmind memory pack`

## 3. Saving

### `vaultmind note create` — create a new note from a type template

This is the right way for an agent to write a new note. Templates enforce the frontmatter schema for each type, so you don't have to remember the required fields.

```bash
vaultmind note create arcs/my-new-arc.md \
  --type arc \
  --field "title=Learning when to ship" \
  --field "tags=[growth, shipping]" \
  --body-stdin <<'EOF'
## The Mistake

I shipped too fast and skipped the review round.

## The Insight

Review rounds aren't friction; they're where the real bugs surface.

## The Principle

Review before ship, always.
EOF
```

Flags:
- `--type <name>` — must match a type registered in the vault's `.vaultmind/config.yaml`. If no config exists, any type name is accepted (convention-based vaults).
- `--field "key=value"` — sets a frontmatter field. Repeatable. Quote the value if it contains spaces or YAML syntax.
- `--body string` or `--body-stdin` — the note's body content. `--body-stdin` is the correct choice for multi-line agent-generated content.
- `--commit` — stages and commits the new note in git. Useful in automated workflows.
- `--json` — machine-readable output.

### The frontmatter schema

Every note gets these core fields automatically:

```yaml
id: <type>-<slug>          # derived from filename; stable across edits
type: <type>               # from --type
title: "..."               # required via --field
created: <date>            # set on creation (humanish "born" stamp)
```

Additional recommended fields (all optional):

```yaml
tags: [tag1, tag2]         # retrieval weight boost on match
related_ids: [id1, id2]    # explicit edges — faster neighborhood expansion
aliases: ["Alt name"]      # match on these as well as the title
parent_id: <id>            # hierarchical relation
source_ids: [ref1, ref2]   # citations when the note quotes something
status: <value>            # for types with a status lifecycle (project, decision)
```

### After saving: re-index

```bash
vaultmind index --vault <path>                    # picks up the new file
vaultmind index --embed --model minilm --vault <path>   # embeds the new note
```

Incremental by default — only re-parses changed files. Safe to run every time you save.

### Convention-based directory layout

Most vaults organize notes into subdirectories by type:

```
vaultmind-identity/
├── arcs/              # transformation narratives
├── identity/          # self-description
├── principles/        # derived rules
├── references/        # pointers to external things
└── .vaultmind/        # managed by VaultMind — do not edit
```

The directory doesn't affect retrieval (type comes from frontmatter, not location). But it's the convention; follow it when creating new notes so humans and other agents can navigate.

## 4. Updating

Just edit the `.md` file directly. VaultMind's UPSERT detects content changes via SHA-256 hash and automatically clears stale embeddings. The next `vaultmind index --embed` re-embeds only the drifted notes — fast, no manual invalidation needed.

```bash
# Edit a file (any editor, or via Bash tool)
vim examples/ada-vault/arcs/arc-ask-before-assuming.md

# Re-index to register the change
vaultmind index --vault examples/ada-vault

# Re-embed the drifted note
vaultmind index --embed --model minilm --vault examples/ada-vault
```

For structured frontmatter changes (agent-safe, no file-level edit), use:

```bash
vaultmind frontmatter set arc-ask-before-assuming \
  --vault examples/ada-vault \
  --field "tags=[growth, identity, updated]"
```

## 5. Inspecting

### Is the vault healthy?

```bash
vaultmind doctor --vault <path>
```

Reports: total notes, domain vs unstructured, unresolved links, Obsidian-incompatible wikilinks, and — critically — **embedding status** (`dense X/Y (model)`, or `none — keyword-only retrieval` with the fix command). Run this first whenever retrieval quality feels off.

### What am I (and other agents) retrieving?

```bash
vaultmind experiment summary
```

Weekly readout: session count, retrieval event count, unique notes recalled, session gap stats (median / p90 / max), and top recalled notes. If arc-X has been retrieved 14 times this week and arc-Y zero times, that's the signal.

### Drill into specific history

```bash
# What did this specific session retrieve?
vaultmind experiment trace --session <session-id>

# Which sessions ever retrieved this note?
vaultmind experiment trace --note <note-id>
```

### Are the hooks actually running?

Scripts on disk are not hooks; *wired* scripts are. These are two independent failures
and `hooks status` reports both:

```bash
vaultmind hooks status <project-dir>
```

```
Hook scripts in .: 11 in sync, 0 drifted, 0 missing
Hook events: 6 wired, 1 unwired
  unwired   SessionEnd -> capture-episode.sh
```

- **drifted / missing** — the script's *contents* differ from the canonical copy, or it
  is absent. Fix: `vaultmind hooks install <dir> --force`.
- **unwired** — the binary wires this event and your `settings.json` does not, so the
  script never runs no matter how correct it is. Fix: `vaultmind hooks install <dir> --merge`.

The distinction is not academic. A project held every canonical script byte-identical,
with `capture-episode.sh` already responsible for 13 captured episodes, and `SessionEnd`
absent from its settings — every content check passed while the write half was switched
off. An event wired to *another tool's* script counts as unwired too: that is not your
hook running.

> **Changed in 0.7.1 — exit code.** `hooks status` now exits non-zero on unwired events
> as well as drifted/missing ones. If you gate CI on it, expect it to start failing where
> it was previously silent about a real problem.

### Recording that a prompting hook fired

Most hooks *query* — they leave a row in the usage log by doing their job. Some hooks
*prompt*: `precompact-preserve.sh` asks you to write down what you learned before
compaction destroys it. A prompt leaves no trace, so "no notes were written this week"
and "the prompt never appeared" produce identical evidence — and only one of those is
fixable by trying harder.

```bash
vaultmind hooks record write_prompt --vault <path>
```

The canonical PreCompact hook already calls this; you only need it when writing your own
prompting hook. The event name is an allowlist, not free text — everything written here
is later read as evidence about agent behaviour, so adding a name is a reviewed change
rather than a runtime string.

It is a **denominator, not a score.** Firing is not success; writing notes is. It exists
so that a zero can be interpreted instead of guessed at.

Session trace shows caller attribution (e.g. `vaultmind-persona-hook` for the SessionStart hook vs `cli` for direct calls; the value comes from `VAULTMIND_CALLER`), operator (user@host), and every retrieval in chronological order. Use when you want to understand "why was this note surfaced, in what context?"

## 6. Best practices for agents

**Query shaping:**
- Paraphrase queries work when embeddings exist. If `retrieval_mode: "keyword"` comes back, inform the user that the vault needs embedding.
- For conversational recall, `vaultmind ask` gives you the top hits + enough context to answer. `--budget 4000` is a reasonable default; raise it if the answer needs more.
- For "show me all X," use `vaultmind search` with a higher `--limit` — it's a list, not a briefing.

**Saving:**
- Use `vaultmind note create --type <type> --body-stdin` — don't shell out to `cat > file.md` with hand-crafted YAML. The template enforces required fields.
- Arcs carry more research value than facts. If you learned something via a transformation (mistake → insight → principle), write it as an arc.
- Set `related_ids` when you reference other notes — those explicit edges speed up context-pack expansion.

**Updating:**
- Content-drift detection is automatic (embeddings clear on hash change). You don't need to manually invalidate anything.
- Always re-index after an edit. `vaultmind index` is incremental and cheap.

**Inspecting:**
- Run `vaultmind doctor` if retrieval feels wrong. It surfaces the keyword-only fallback immediately.
- Run `vaultmind experiment summary` at the end of a working session to see what you actually recalled. It's the honest reflection of what mattered vs what you thought mattered.

## 7. Common failure modes

| Symptom | Likely cause | Fix |
|---|---|---|
| `ask` returns 0 hits on a paraphrase query | Vault not embedded | `vaultmind index --embed --model minilm --vault <path>` |
| `doctor` reports `Embeddings: none` | Same | Same |
| Retrieval quality drops after editing notes | Stale embeddings from pre-drift-fix era | `vaultmind index --embed --vault <path>` picks up the now-cleared drift |
| `note create` errors with "type not registered" | Vault's config.yaml declares types and yours isn't listed | Add the type to `.vaultmind/config.yaml` or drop the check (convention-based) |
| Session attribution shows `caller=unknown` | Session predates 2026-04-20 (before attribution migration) or binary is too old | Rebuild binary from latest source (`bash .claude/scripts/bootstrap.sh`) |

## 8. Integration patterns

### Claude Code SessionStart hook

See `.claude/scripts/load-persona.sh` in this repo for the reference implementation. Key elements:

1. Rebuild the binary if source is newer (auto-propagates vaultmind changes)
2. Set `VAULTMIND_CALLER=<project>-persona-hook` on the ask invocation
3. Capture stderr; surface build + runtime errors instead of producing empty persona silently
4. Emit the `IDENTITY CONTEXT:` block on stdout so it becomes a system-reminder

### Non-Claude-Code agents

Any agent that can run shell commands can use VaultMind. The pattern:

1. Call `vaultmind ask "who am I" --vault <identity-vault> --json` at session start
2. Parse `result.context.context[*]` for the identity notes
3. Inject the notes into your system prompt or context
4. Call `vaultmind ask "<user-question>"` as needed during the session
5. Optionally call `vaultmind note create` to persist new arcs

Set `VAULTMIND_CALLER=<your-agent-name>` so the experiment DB attributes events to your agent specifically.

## 9. Where to go from here

- **[AGENTS.md](../AGENTS.md)** — architecture and workflow rules for agents working *on* the VaultMind codebase (different scope from this doc)
- **[.ckeletin/docs/adr/](../.ckeletin/docs/adr/)** — ADRs for the underlying scaffold
- **[docs/](./)** — additional guides: embedding backends, configuration, testing strategy, and command reference

If something is missing from this guide, the README and `--help` text for each command are authoritative. This guide is the agent-facing distillation.

## 10. Command reference

The block below is **generated from the command catalog** — the same source `--help`
renders from — and is replaced by `task generate:docs`. Do not hand-edit inside the
markers; `task check` fails if it drifts from the catalog. Everything outside them is
hand-written and stays.

<!-- VAULTMIND:GENERATED:commands:START -->
<!-- checksum:d973388b4914c87324510aeb2dba067a6defea5f0bcf3568ccef21ec3d7caaed -->
# VaultMind Commands

Every user-facing command, grouped by intent, with its when-to-use trigger.
Generated from the command tree — do not edit by hand (run `task generate:docs:commands`).

## Retrieval & memory:

| Command | What | When to use |
|---------|------|-------------|
| `vaultmind ask` | Compound search + context-pack: answer 'what do I know about X?' | you want to answer "what do I know about X?" — search plus packed context in one step. |
| `vaultmind memory` | Traverse the note graph and assemble context for agents | you need the low-level graph primitives behind ask: links, neighbors, related, pack, summarize. |
| `vaultmind memory links` | List a note's directed wikilink edges (outbound, inbound, or both) | you want a note's directed wikilink edges — outbound, inbound, or both. |
| `vaultmind memory neighbors` | Traverse the graph from a note (BFS) and return enriched neighbors | you want the enriched graph around a note via depth-limited BFS, with full frontmatter. |
| `vaultmind memory pack` | Pack a note plus ranked context within a token budget | you want a token-budgeted context payload ready to ship to an agent. |
| `vaultmind memory related` | List notes related to a target, filtered by edge type | you want a simple ranked list of directly connected notes, filtered by edge type. |
| `vaultmind memory summarize` | Assemble material from specific note IDs for agent synthesis | you have a known list of note IDs and want their material assembled for synthesis. |
| `vaultmind note` | Read, create, and batch-fetch notes by ID | you want to read, create, or batch-fetch a specific note by ID. |
| `vaultmind note create` | Create a note from a template with field overrides | you want to create a new note from its type template with field overrides. |
| `vaultmind note get` | Get one note's full content and metadata by ID | you know a note's ID or path and want its full content (with access tracking). |
| `vaultmind note mget` | Fetch multiple notes by ID in one call | you have several note IDs and want them all in one batched fetch. |
| `vaultmind resolve` | Resolve a fragment, alias, title, or path to canonical note IDs | you have a fragment, alias, title, or path and need the canonical note ID. |
| `vaultmind search` | Search vault notes by keyword, semantic similarity, or both | you want a ranked list of hits to browse and pick from, without packed context. |
| `vaultmind self` | Show your memory state — recent, hot, and stale notes | you want to see your own memory state — recent, hot, and stale notes. |

## Vault maintenance:

| Command | What | When to use |
|---------|------|-------------|
| `vaultmind apply` | Execute an AI-generated plan to mutate vault notes | you have an AI-generated JSON plan and want to execute its note mutations. |
| `vaultmind dataview` | Manage template-generated regions in vault notes | you manage template-generated regions in notes and need to render or lint their markers. |
| `vaultmind dataview lint` | Scan the vault for broken or duplicate generated-region markers | you want to catch malformed or duplicated VAULTMIND:GENERATED markers before rendering. |
| `vaultmind dataview render` | Render a note's generated region from its template | you edited a template and want to refresh a note's generated region. |
| `vaultmind doctor` | Vault health hub: diagnose a vault and report issues | you want a read-only health overview of a vault (or every vault, with --all). |
| `vaultmind doctor heal` | Apply all auto-fixable repairs doctor found | doctor found auto-fixable issues and you want to apply every repair at once. |
| `vaultmind doctor heal wikilinks` | Rewrite Obsidian-incompatible wikilinks to [[filename\|Title]] | doctor flagged Obsidian-incompatible wikilinks and you want them rewritten to [[filename\|Title]]. |
| `vaultmind frontmatter` | Inspect and mutate YAML frontmatter across vault notes | you need to audit, validate, or programmatically edit YAML frontmatter across notes. |
| `vaultmind frontmatter fix` | Backfill missing "created" frontmatter on domain notes | you are migrating notes and need to backfill the missing "created" field. |
| `vaultmind frontmatter merge` | Merge multiple frontmatter fields from a YAML file into a note | you want to merge many key/value pairs from a YAML file into one note at once. |
| `vaultmind frontmatter normalize` | Normalize one note's frontmatter (keys, aliases, dates, snake_case) | you want to clean up one note's frontmatter formatting — key order, aliases, dates, snake_case. |
| `vaultmind frontmatter set` | Set one frontmatter field on a note | you want to set a single frontmatter field on one note, schema-validated. |
| `vaultmind frontmatter unset` | Remove one frontmatter field from a note | you want to remove one frontmatter field from a note. |
| `vaultmind frontmatter validate` | Check vault notes for frontmatter rule violations | you want to catch missing fields, bad statuses, unknown types, or broken refs before indexing. |
| `vaultmind index` | Scan and index vault notes into SQLite, optionally embedding | vault notes changed and you need to refresh the SQLite index (and optionally embeddings). |
| `vaultmind schema` | Query the vault's type schema | you need to discover the vault's note types, required fields, and valid statuses. |
| `vaultmind schema list-types` | List every note type with its required fields and valid statuses | you want every registered type with its required fields and valid statuses before creating notes. |

## Identity & sessions:

| Command | What | When to use |
|---------|------|-------------|
| `vaultmind arc` | Surface arc-distillation candidates from episodes (propose-only) | you want to surface arc-distillation candidate moments from episodes (propose-only). |
| `vaultmind arc candidates` | Surface candidate transformation moments for arc distillation | you finished a session and want candidate transformation moments to review for arcs. |
| `vaultmind arc guide` | Print the arc-writing discipline — how to find and write your own arcs | you want to learn how to find and write your own arcs — the shapes, the bar, and the self-check. |
| `vaultmind episode` | Capture Claude Code sessions as episodic-memory artifacts | you want to capture Claude Code sessions as episodic-memory artifacts. |
| `vaultmind episode capture` | Convert session transcripts into episode notes | you have a session transcript (or a directory of them) to convert into episode notes. |
| `vaultmind hooks` | Manage VaultMind's Claude Code hook scripts | you need to install, remove, or check VaultMind's Claude Code hook scripts. |
| `vaultmind hooks install` | Install Claude Code hook scripts into a project | you want to wire VaultMind into a project by writing its hook scripts. |
| `vaultmind hooks record` | Record that a hook fired, so a zero can be told apart from a no-show | a hook that PROMPTS rather than queries needs to leave evidence that it fired, so "nothing was written" can be told apart from "the prompt never appeared". |
| `vaultmind hooks status` | Compare a project's installed hook scripts against the canonical ones | you want to know whether a project's hook scripts still match the ones this binary ships — before an update overwrites a local change, or after one that should have installed something. |
| `vaultmind hooks uninstall` | Remove VaultMind's Claude Code hook entries from a project | you want to remove VaultMind's Claude Code hook entries from a project. |
| `vaultmind identity` | Contract-B agent identity: keypair custody and signing | you need Contract-B agent identity: mint a keypair or sign an entry via the keyless signer. |
| `vaultmind identity enroll` | Enroll into a Contract-B network from an invite, then self-sign the request | you are a member with an invite and want to enroll: cross-check the relay's root against the invite, confirm the fingerprint, and self-sign an enrollment request for your admin. |
| `vaultmind identity enroll-add` | Admin-add a member's enrollment request to the trust-root registry, emitting the unsigned registry | you are an admin and want to add a member's signed enrollment request to the trust-root registry, emitting the updated unsigned registry for the root signer. |
| `vaultmind identity init` | Mint an agent keypair and seal the private key to the signer | you are setting up an agent and need to mint its ed25519 keypair and seal the private key to the signer. |
| `vaultmind identity invite` | Emit an UNSIGNED network invite carrying the trust anchor (Contract-B) | you are an admin and want to emit a network invite (the trust anchor plus relay, with an out-of-band fingerprint) for a member to enroll against. |
| `vaultmind identity paths` | Emit this agent's resolved mesh identity and state paths | a hook or watcher script needs this agent's resolved mesh identity and state paths — eval the output instead of hardcoding a slug, daemon URL, or heartbeat path. |
| `vaultmind identity sign` | Validate, canonicalize, and sign an entry via the keyless signer | you have a Contract-B entry to sign and want it validated, canonicalized, and signed by the keyless signer. |
| `vaultmind identity sign-enrollment` | Self-sign an agent enrollment request via the keyless signer (Contract-B) | you are enrolling an agent and need to self-sign its enrollment request (proof-of-possession) before an admin adds the binding to the trust-root registry. |
| `vaultmind identity sign-envelope` | Sign a chat message envelope via the keyless signer (Contract-B slice 5) | you have a chat MESSAGE envelope to sign so a receiving daemon can verify the signature and the signer's registry binding. |
| `vaultmind identity sign-registry` | Sign a trust-root registry via the keyless signer (Contract-B) | you have a trust-root registry to sign so consumers can verify the root signature, anti-rollback epoch, and freshness at load. |
| `vaultmind identity signer` | Run the keyless custody signer daemon (Contract-B) | you need to RUN the keyless custody signer daemon so the sign-* commands have a process to connect to. |
| `vaultmind init` | Scaffold a fresh persona-shaped vault, ready for you and your agent | you are starting fresh and need to scaffold a new persona-shaped vault. |

## Setup & introspection:

| Command | What | When to use |
|---------|------|-------------|
| `vaultmind completion` | Generate the shell autocompletion script | you want to install shell tab-completion for the vaultmind command. |
| `vaultmind config` | Manage and validate application configuration | you want to manage or validate the application's configuration file. |
| `vaultmind config validate` | Validate a configuration file | you want to check a config file for correctness, security, and unknown keys. |
| `vaultmind docs` | Generate documentation | you want to generate documentation about the app and its configuration. |
| `vaultmind docs commands` | Generate the grouped command reference (COMMANDS.md) | you want a generated grouped reference of every command, each with its when-to-use. |
| `vaultmind docs config` | Generate the configuration-options reference | you want a generated reference of every configuration option. |
| `vaultmind experiment` | Experiment tracking and reporting | you want to inspect experiment tracking: retrieval quality, usage, traces, comparisons. |
| `vaultmind experiment compare` | Surface where retrieval variants disagree, no labels needed | you want to see where retrieval variants disagree, without labeled ground truth. |
| `vaultmind experiment report` | Measure retrieval quality: Hit@K and MRR per variant | you want to measure retrieval quality — Hit@K and MRR per variant. |
| `vaultmind experiment summary` | Memory usage overview: top recalled notes, session gaps | you want a memory-usage overview — top recalled notes and session-gap stats. |
| `vaultmind experiment trace` | Drill into a session's or note's retrieval history | you want to drill into one session's or note's retrieval history. |
| `vaultmind export` | Export experiment data as a sanitized JSONL snapshot | you want a sanitized JSONL snapshot of experiment data to share with the VaultMind team. |
| `vaultmind git` | Inspect git repository state relevant to vault operations | you want git repository state relevant to VaultMind mutation policies. |
| `vaultmind git status` | Report git branch, dirty, and merge/rebase state for a vault | a script or agent needs to gate on the vault's branch, dirty, or merge/rebase state. |
| `vaultmind ping` | Respond with a pong (connectivity smoke test) | you want to smoke-test that the binary runs and renders output. |
| `vaultmind version` | Print the version, commit, and build date | you want the build version, commit, and date. |
<!-- VAULTMIND:GENERATED:commands:END -->

