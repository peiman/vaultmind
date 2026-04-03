---
id: concept-memgpt
type: concept
title: MemGPT
created: 2026-04-03
vm_updated: 2026-04-03
aliases:
  - Memory GPT
  - Virtual Context Management
tags:
  - ai-memory
  - agent-architecture
  - memory-systems
related_ids:
  - concept-working-memory
  - concept-generative-agents
  - concept-context-pack
source_ids:
  - source-packer-2023
---

## Overview

MemGPT (Packer et al., 2023) is an agent architecture that manages an LLM's limited context window as an operating system manages virtual memory. It introduces a memory hierarchy with explicit tier boundaries:

1. **Main context (working memory):** The LLM's active context window. Small, fast, volatile.
2. **Recall storage:** Recently accessed memories, searchable via recency. Medium capacity.
3. **Archival storage:** Long-term persistent memory. Large, slow, searchable via text/embeddings.

A memory manager (implemented as LLM function calls) moves information between tiers — evicting old context to archival, retrieving relevant memories to main context, and editing core memories.

## Key Properties

- **Self-editing memory:** The agent can modify its own persistent memory state via function calls
- **Pagination over context:** When context fills, older information is "paged out" to recall/archival
- **Retrieval on demand:** Agent explicitly requests information from lower tiers
- **Conversation continuity:** Can maintain coherent long conversations by managing what stays in context

## Connections

VaultMind maps to MemGPT's **archival storage tier** — it provides structured, persistent, searchable long-term memory over a knowledge graph. However, VaultMind lacks MemGPT's memory management layer: there is no component that decides what to page in/out of an agent's context window.

The [[Context Pack]] command partially bridges this gap by assembling token-budgeted retrieval payloads, but the agent must explicitly request them. A future integration could have VaultMind serve as a MemGPT archival backend, with MemGPT's memory manager orchestrating retrieval and eviction.
