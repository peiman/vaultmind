# Your VaultMind Vault

This directory is the long-term memory of an AI agent — collaboratively
curated by you and the agent, queried by the agent at session start and on
demand during work.

## What's here

```
.vaultmind/        VaultMind config — type registry, git policy, index settings
identity/          Notes about who the agent is — name, role, foundational traits
principles/        Rules the agent follows, with reasons
arcs/              Transformations: what shifted, when, what's now true that wasn't
references/        Stable lookup notes — current state, glossaries, priority orders
concepts/          Domain knowledge the agent should learn and apply
sources/           Citations — papers, books, links the agent references
decisions/         (Optional) Architectural choices and their reasoning
```

You'll find starter notes in `identity/` and `references/` — replace them
with your agent's real content. Frontmatter follows the type schema in
`.vaultmind/config.yaml`.

## Workflow

```bash
# After editing notes, refresh the index:
vaultmind index --vault .

# Compute embeddings (one-time setup, then incremental):
vaultmind index --embed --vault .

# Query the vault as the agent would:
vaultmind ask "who am I" --vault .
vaultmind search "spreading activation" --vault .
```

## Wiring to an AI agent

The agent reconstructs its identity at session start by running
`vaultmind ask "who am I"` against this vault and injecting the result as
context. For Claude Code, this is done via a SessionStart hook.

See the VaultMind project's `.claude/scripts/load-persona.sh` for a
reference hook implementation.

## The model

VaultMind is built around a specific bet: that **arcs** — transformation
notes describing what shifted in the agent's understanding — are the
atomic unit of persona, not facts or principles. Facts state what is.
Principles state what to do. Arcs state how the agent grew. The journey
is what carries identity across sessions.

If you're new to this model, the recommended starting points are:

1. Write `identity/who-am-i.md` from your agent's voice — name, role,
   what it cares about, who you are as their partner.
2. Write `references/current-context.md` — the live edge, what's
   deferred, the next bet. Update this whenever priorities shift; it's
   what the SessionStart hook surfaces.
3. As your collaboration generates real transformations, write them as
   arcs — `arcs/<slug>.md` — using the trigger / push / deeper-sight /
   principle structure. Don't pre-author these. Let the work produce them.

The vault grows with the collaboration. Don't try to seed it all at once.
