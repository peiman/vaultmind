---
id: proj-vaultmind
type: project
status: active
title: "VaultMind"
created: 2026-04-03
vm_updated: 2026-04-03
aliases: []
tags: [vaultmind, cli, go]
related_ids:
  - concept-associative-memory
  - concept-spreading-activation
  - concept-semantic-networks
  - concept-context-pack
  - concept-entity-resolution
  - concept-rag
  - concept-memgpt
  - concept-generative-agents
source_ids: []
---

# VaultMind

VaultMind is an associative memory system for AI agents, implemented as a CLI tool written in Go. It treats an Obsidian vault as a persistent, queryable knowledge graph that agents can read from and write to across sessions.

## Core Idea

Rather than relying on ephemeral context windows, VaultMind gives agents a structured long-term memory backed by Markdown files. Notes are linked by explicit relationships, and retrieval uses [[Spreading Activation]] to surface related concepts from a seed node.

## Three Phases

1. **Ingest** — Parse vault notes, extract frontmatter IDs, build an in-memory graph of nodes and weighted edges.
2. **Retrieve** — Given a query or seed ID, run BFS-based spreading activation across the graph and return a ranked [[Context Pack]].
3. **Write-back** — Agents append new notes or update existing ones; [[Entity Resolution]] merges duplicates via alias matching.

## Current Status

Specification is complete. Implementation is pending. Key design decisions are recorded in the `decisions/` folder. Research informing the design lives in [[Memory Research Knowledge Base]].

## Key Concepts

- [[Associative Memory]] — the foundational memory model
- [[Semantic Networks]] — graph representation of knowledge
- [[RAG]] — prior art; VaultMind extends beyond chunk retrieval
- [[MemGPT]] — inspiration for agent memory tiers
- [[Generative Agents]] — inspiration for memory stream architecture
