# VaultMind — Agent Usage Guide

**For AI agents using VaultMind as memory. Covers save, retrieve, update, inspect. Machine-first explanations with real commands.**

If you're an agent integrating VaultMind into your workflow (via SessionStart hook or explicit CLI calls), read this end-to-end. If you're a human reviewing agent usage, the same material applies — agents and humans share the same CLI surface.

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
vaultmind ask "who am I" --vault vaultmind-identity --max-items 8 --budget 6000
vaultmind ask "spreading activation" --vault vaultmind-vault --json
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

### `vaultmind search <query>` — ranked hits only

Use when you want a hit list without the context-pack overhead. Supports `--mode hybrid | keyword | embedding | sparse | colbert` to pick a specific lane, or omit for auto-selection.

```bash
vaultmind search "judgment gap" --vault vaultmind-identity --limit 10 --json
```

### `vaultmind memory context-pack <target-id>` — expand around a specific note

Use when you already know the target note and want its token-budgeted neighborhood.

```bash
vaultmind memory context-pack arc-the-judgment-gap --vault vaultmind-identity --budget 4000 --max-items 8
```

### `vaultmind memory recall <note-id>` — just the note's body

Use when you want one specific note's content with no expansion.

```bash
vaultmind memory recall identity-who-i-am --vault vaultmind-identity
```

### `vaultmind note get <id>` — full note with frontmatter

Use when you want the raw note structure (frontmatter + body) for inspection or quoting.

```bash
vaultmind note get arc-the-breakthrough --vault vaultmind-identity --json
```

### Picking between them

- **Conversational "what do I know about X?"** → `vaultmind ask` (gives you top hits + context)
- **"Find all notes mentioning Y"** → `vaultmind search`
- **"Read this specific note"** → `vaultmind memory recall` or `vaultmind note get`
- **"Expand around this known target"** → `vaultmind memory context-pack`

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
created: <date>            # set on creation
vm_updated: <date>         # VaultMind-managed, set on save
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
vim vaultmind-identity/arcs/the-breakthrough.md

# Re-index to register the change
vaultmind index --vault vaultmind-identity

# Re-embed the drifted note
vaultmind index --embed --model minilm --vault vaultmind-identity
```

For structured frontmatter changes (agent-safe, no file-level edit), use:

```bash
vaultmind frontmatter set arc-the-breakthrough \
  --vault vaultmind-identity \
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

Session trace shows caller attribution (`workhorse-persona-hook` vs `vaultmind-persona-hook` vs `cli`), operator (user@host), and every retrieval in chronological order. Use when you want to understand "why was this note surfaced, in what context?"

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

- **[SETUP.md](../SETUP.md)** — one-time bootstrap for a new machine
- **[AGENTS.md](../AGENTS.md)** — architecture and workflow rules for agents working *on* the VaultMind codebase (different scope from this doc)
- **[.ckeletin/docs/adr/](../.ckeletin/docs/adr/)** — ADRs for the underlying scaffold
- **[docs/](./)** — research notes, review rounds, evaluation protocols

If something is missing from this guide, the README and `--help` text for each command are authoritative. This guide is the agent-facing distillation.
