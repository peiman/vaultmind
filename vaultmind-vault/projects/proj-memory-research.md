---
id: proj-memory-research
type: project
status: active
title: "Memory Research Knowledge Base"
created: 2026-04-03
vm_updated: 2026-04-03
aliases: []
tags: [research, memory-systems]
related_ids:
  - concept-generative-agents
  - concept-memgpt
  - concept-rag
source_ids: []
---

# Memory Research Knowledge Base

This project tracks research on human and AI memory systems that informs VaultMind's design. It collects primary papers, implementation references, and cognitive science concepts into a unified knowledge base.

## Scope

The research spans three areas:

- **Cognitive science** — human memory models such as [[forgetting-curve|Forgetting Curve]], [[spacing-effect|Spacing Effect]], [[encoding-specificity|Encoding Specificity]], [[base-level-activation|Base-Level Activation]], and [[act-r|ACT-R]].
- **AI memory architectures** — systems that give language models persistent memory, including [[memgpt|MemGPT]] and [[generative-agents|Generative Agents]].
- **Retrieval techniques** — approaches such as [[rag|RAG]] and [[embedding-based-retrieval|Embedding-Based Retrieval]] that inform VaultMind's retrieval layer.

## How It Feeds VaultMind

Findings from this base directly shape VaultMind's scoring weights, decay functions, and retrieval strategy. See [[proj-vaultmind|VaultMind]] for how the research is applied.

## Status

Active. Notes are added as new papers and systems are reviewed. Concepts with source links are considered stable; those without are provisional.
