---
id: decision-immutable-ids
type: decision
status: accepted
title: "Use immutable IDs for note identity, not filenames"
created: 2026-04-03
vm_updated: 2026-04-03
tags: [architecture, identity]
related_ids:
  - concept-entity-resolution
source_ids: []
---

# Use Immutable IDs for Note Identity, Not Filenames

## Decision

Each note is permanently identified by its frontmatter `id` field. Filenames are treated as display hints, not identity. All graph edges reference IDs, not filenames.

## Rationale

**Renames do not break the graph.** When a user renames `rag.md` to `retrieval-augmented-generation.md`, every edge pointing to `concept-rag` remains valid. A filename-based system would require a vault-wide find-and-replace.

**Aliases provide flexible access.** The `aliases` frontmatter field lets a note be resolved by multiple names. [[Entity Resolution]] uses aliases to detect and merge duplicate notes that refer to the same concept under different titles.

**IDs are namespace-scoped.** Prefixes (`concept-`, `proj-`, `decision-`) prevent collisions across note types and make the type of a referenced note legible in any `related_ids` list without resolving the file.

## Trade-offs Accepted

Authors must set the `id` field when creating notes. VaultMind's CLI provides a `new` command that pre-fills frontmatter to reduce friction.
