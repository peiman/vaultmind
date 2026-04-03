# Overview, Principles, Scope, Users

> See also: [architecture](02-architecture.md), [data model](03-data-model.md), [glossary](glossary.md)

## What VaultMind Is

An associative memory system for AI agents, built on a Git-backed, Obsidian-compatible Markdown vault. Agents can index, recall, relate, validate, and safely mutate knowledge stored in plain Markdown files with YAML frontmatter — while preserving human readability and full Git history.

## Core Principles

1. The vault (Markdown + YAML frontmatter + filesystem) is the only canonical content store.
2. Git is the canonical history layer. All important mutations must be reviewable, reversible, auditable.
3. Identity is based on immutable note IDs, not filenames or paths.
4. All canonical content must remain human-readable and editable in Obsidian without VaultMind.
5. Backlinks, indexes, inferred associations, and generated regions are derived — never canonical truth.
6. Agents may only mutate frontmatter, explicitly marked generated regions, and newly created notes from templates.
7. Associative memory queries must distinguish explicit graph facts from inferred associations, with confidence metadata.
8. The system must fail safely on invalid or ambiguous states — never silently guess.

## Scope

### Goals

- Associative memory for agents over a vault graph
- Markdown and frontmatter as canonical data model
- Obsidian compatibility for navigation and readability
- Both outbound and inbound link computation
- Safe, minimal-diff, reviewable mutations with Git-aware workflows
- Stable, machine-readable JSON output for all agent-relevant commands
- Dataview-compatible generated views as managed presentation layer

### Non-goals (v1)

- Replacement for Obsidian
- Full Dataview execution engine
- Real-time collaborative editor
- Cloud sync product
- Freeform AI writing assistant
- Proprietary knowledge store
- Complete Git client
- Note deletion (use Git directly; VaultMind indexes what exists)
- Note move/rename commands (path is non-canonical; rename via filesystem/Obsidian, then re-index)

## Users

| User | Role |
|------|------|
| Human knowledge worker | Reads and edits notes in Obsidian. Never needs VaultMind installed. |
| AI agent | Uses VaultMind as memory and mutation interface over the vault. |
| Developer/operator | Maintains schemas, templates, indexing config, operational workflows. |
