---
id: concept-context-pack
type: concept
title: Context Pack
created: 2026-04-03
vm_updated: 2026-04-03
aliases:
  - Context Packing
  - Token-Budgeted Retrieval
tags:
  - vaultmind
  - retrieval
  - agent-memory
related_ids:
  - concept-rag
  - concept-working-memory
  - concept-spreading-activation
source_ids: []
---

## Overview

A context pack is VaultMind's mechanism for assembling a bounded retrieval payload for agent consumption. Given a target note, it gathers the most relevant neighboring content within a token budget, producing a self-contained context window that an agent can reason over.

The packing algorithm prioritizes by edge confidence: explicit relations first, then explicit links, then inferred associations. Each included note carries its relationship type and confidence relative to the target.

## Key Properties

- **Budget unit:** Estimated tokens (1 token ~ 4 characters)
- **Default budget:** 4,096 tokens
- **Priority ordering:** explicit_relation > explicit_link > medium-confidence inferred
- **Body inclusion:** Target note gets full body; neighbors get frontmatter only
- **Truncation signals:** `truncated: true` if target body was cut, `budget_exhausted: true` if neighbors were omitted
- **Sort order within tiers:** By edge weight (where available), then by `vm_updated` descending

## Connections

Context packing is VaultMind's bridge between archival long-term memory and an agent's working memory (see [[Working Memory]]). It is analogous to the retrieval step in [[RAG]] — selecting relevant documents to inject into the prompt — but operates over a structured graph rather than vector embeddings.

The expert panel identified that sorting by `vm_updated` alone is a weak relevance signal. Future versions should incorporate edge weight and potentially an [[ACT-R]]-inspired accessibility score combining recency with retrieval frequency.
