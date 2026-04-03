---
id: decision-vm-updated-field
type: decision
status: accepted
title: "Use vm_updated instead of updated for VaultMind timestamps"
created: 2026-04-03
vm_updated: 2026-04-03
tags: [obsidian-compatibility, frontmatter]
related_ids: []
source_ids: []
---

# Use vm_updated Instead of updated for VaultMind Timestamps

## Decision

VaultMind tracks its own last-modified timestamp in the frontmatter field `vm_updated`. It does not read from or write to a field named `updated`.

## Rationale

**Obsidian plugins rewrite `updated` automatically.** Several popular Obsidian community plugins (e.g., Update time on edit, Linter) overwrite the `updated` field on every save. If VaultMind uses `updated` to track whether a note has changed since last ingest, those plugin writes would trigger unnecessary re-ingestion and cause hash conflicts on every vault open.

**`vm_updated` is a VaultMind-owned field.** By using a namespaced field, VaultMind can read and write timestamps with confidence that no other tool will clobber them. The prefix `vm_` signals ownership clearly to vault authors.

**Separation of concerns.** Obsidian's `updated` represents the last time a human edited the file. `vm_updated` represents the last time VaultMind processed it. These are different facts and should not share a field.

## Trade-offs Accepted

Vault authors who inspect frontmatter will see an unfamiliar field. The `vm_` prefix makes its origin self-explanatory.
