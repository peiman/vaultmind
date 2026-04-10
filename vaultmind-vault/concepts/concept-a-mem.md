---
id: concept-a-mem
type: concept
title: A-MEM (Agentic Memory)
created: 2026-04-09
vm_updated: 2026-04-09
aliases:
  - Agentic Memory
  - A-MEM
tags:
  - agent-memory
  - zettelkasten
  - knowledge-networks
related_ids:
  - concept-spreading-activation
  - concept-base-level-activation
  - concept-hebbian-learning
source_ids: []
---

## Overview

A-MEM (Agentic Memory, NeurIPS 2025, arXiv:2502.12110) is a self-organizing memory architecture inspired by the Zettelkasten method. When a new memory is stored, the system generates structured attributes (keywords, context, connections) and then analyzes the existing memory network to identify which prior memories it connects to, triggering updates to those memories as well. Memory is not passive storage — it evolves as new information arrives.

The core insight: every new memory is an opportunity to refine the network. Connections between memories are bidirectional, and inserting a new node updates the edges of existing nodes. This mirrors the way a Zettelkasten note links backward to prior notes and forward to future ones.

## Key Properties

- Self-organizing: New memories automatically integrate into the existing network without manual linking
- Bidirectional evolution: Adding a memory updates the context of related prior memories
- Structured attributes: Each memory carries keywords, context summary, and explicit connection list
- Retrieval-driven refinement: Retrieving a memory can trigger re-evaluation of its connections
- Superior to SOTA: Evaluated on 6 LLM models, outperforms prior memory architectures on QA tasks

## Connections

A-MEM's memory evolution mechanism is what VaultMind plans to implement as Hebbian strengthening: when a note is retrieved in the context of another note, the edge between them should be strengthened. A-MEM demonstrates this is not only theoretically motivated but practically effective at scale across multiple LLM architectures.

The structured attribute generation (keywords, connections) maps directly to VaultMind's existing note frontmatter — `tags`, `related_ids`, and `source_ids` are already a structured attribute system. A-MEM's contribution is making these attributes dynamic rather than static.

See [[concept-spreading-activation|Spreading Activation]] for the graph traversal mechanism that A-MEM's network connections enable.
