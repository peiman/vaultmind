---
id: concept-memos
type: concept
title: MemOS (Memory Operating System)
created: 2026-04-09
vm_updated: 2026-04-09
aliases:
  - Memory Operating System
  - MemOS
tags:
  - memory-architecture
  - agent-memory
  - three-tier
related_ids:
  - concept-base-level-activation
  - concept-act-r
  - concept-rag
source_ids: []
---

## Overview

MemOS (arXiv:2507.03724, MemTensor, July 2025) proposes a three-tier memory operating system for LLM agents. The three tiers are:

- **Plaintext Memory**: Facts, notes, documents — the explicit, readable memory store. Persists across sessions.
- **Activation Memory**: Hot KV cache — in-context working memory with fast but ephemeral access.
- **Parametric Memory**: Model weights — deeply learned knowledge that influences generation implicitly.

The **MemCube** abstraction unifies these tiers: each memory unit carries a metadata header (provenance, activation score, tier assignment) plus a payload (the actual content). A **MemScheduler** component selects which memories to surface based on task requirements, managing the migration of content between tiers as relevance changes.

## Key Properties

- Three-tier hierarchy mirrors human memory's sensory/working/long-term distinction
- MemCube provides a uniform interface regardless of which tier holds the memory
- MemScheduler enables task-aware memory selection, not just recency or similarity
- Tier migration: frequently accessed plaintext memories can be promoted to activation tier
- Cross-agent memory sharing: MemOS treats memory as a first-class OS resource, shareable across processes

## Connections

The MemOS three-tier model maps directly to VaultMind's architecture:
- Plaintext tier = vault notes on disk (the `.md` files)
- Activation tier = in-session activation scores computed by [[concept-base-level-activation|Base-Level Activation]]
- Parametric tier = BGE-M3 embeddings that encode semantic knowledge implicitly

VaultMind's activation scoring is precisely the MemScheduler's job: decide which plaintext memories to promote to the effective "hot" tier for context-pack assembly. The MemCube metadata header corresponds to VaultMind's frontmatter fields (`id`, `tags`, `related_ids`, activation scores).

The MemOS framing validates VaultMind's design: activation scoring is not just a ranking heuristic but a tier-promotion mechanism — determining which notes move from cold storage into the active context window.
