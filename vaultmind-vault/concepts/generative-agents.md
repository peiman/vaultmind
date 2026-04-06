---
id: concept-generative-agents
type: concept
title: Generative Agents
created: 2026-04-03
vm_updated: 2026-04-03
aliases:
  - Smallville Agents
  - Park et al. Agents
tags:
  - ai-memory
  - agent-architecture
  - memory-systems
related_ids:
  - concept-memgpt
  - concept-reflexion
  - concept-working-memory
source_ids:
  - source-park-2023
---

## Overview

Generative Agents (Park et al., 2023) introduced a memory architecture for believable autonomous agents in a simulated world (Smallville). The architecture stores a stream of natural language observations and retrieves relevant memories using three scoring dimensions:

1. **Recency:** How recently was the memory created? Exponential decay.
2. **Importance:** How significant is the memory? Scored 1-10 by an LLM at creation time.
3. **Relevance:** How related is the memory to the current situation? Cosine similarity of embeddings.

Final retrieval score: `score = α * recency + β * importance + γ * relevance`

## Key Properties

- **Memory stream:** All observations stored as timestamped natural language entries
- **Reflection:** Periodically, the agent synthesizes higher-order insights from recent memories ("What have I learned?")
- **Planning:** Agents create and revise daily plans, stored as memory entries
- **Three-signal retrieval:** The combination of recency + importance + relevance outperforms any single signal

## Connections

VaultMind currently implements **none** of these three retrieval signals in a principled way:

- **Recency:** `vm_updated` is note modification time, not agent access time. Deferred to v2 (agent access log).
- **Importance:** No salience/importance scoring. Deferred to v2 (optional frontmatter field).
- **Relevance:** No embedding-based similarity. Structured graph traversal is the proxy.

The reflection mechanism maps to the expert panel's recommendation for a `reflection` note type — a second-order memory that synthesizes insights from multiple source notes, with elevated retrieval priority. See [[reflexion|Reflexion]] for a related but distinct approach.
