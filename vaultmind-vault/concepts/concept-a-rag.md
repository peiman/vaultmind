---
id: concept-a-rag
type: concept
title: A-RAG (Agentic RAG)
created: 2026-04-09
vm_updated: 2026-04-09
aliases:
  - Agentic RAG
  - A-RAG
tags:
  - retrieval-augmented-generation
  - agent-retrieval
  - hierarchical-interface
related_ids:
  - concept-rag
  - decision-vaultmind-mcp-server
  - concept-spreading-activation-in-ir
source_ids: []
---

## Overview

A-RAG: Scaling Agentic RAG (arXiv:2602.03442, February 2026) proposes that retrieval systems should expose multiple tools at different granularities rather than a single search interface. A-RAG defines three retrieval tools:

- `keyword_search`: Fast, exact-match lookup for specific terms, IDs, or known identifiers
- `semantic_search`: Embedding-based retrieval for conceptually related content
- `chunk_read`: Direct fetch of a specific document chunk by ID, once the agent knows what it wants

An LLM agent autonomously selects which tool to call based on the query type, using keyword search for known-unknown lookups, semantic search for exploratory queries, and chunk_read to retrieve specific passages after a prior search narrows the candidate set.

## Key Properties

- Hierarchical retrieval: Three tools at different cost/precision tradeoffs
- Agent-driven strategy: No fixed retrieval pipeline; the LLM decides which tool fits the query
- Progressive narrowing: Search tools identify candidates; chunk_read fetches the exact content
- Outperforms single-tool RAG: Autonomous tool selection beats fixed pipelines on diverse query types
- MCP-compatible: The three-tool interface maps naturally to MCP tool definitions

## Connections

A-RAG is the direct design basis for VaultMind's planned MCP server, which exposes six tools rather than three. VaultMind extends the A-RAG hierarchy:

- `keyword_search` → `vault_search` (keyword + full-text)
- `semantic_search` → `vault_search` (semantic mode)
- `chunk_read` → `vault_read` (full note fetch)
- Beyond A-RAG: `vault_graph_traverse` (graph-based spreading activation — not in A-RAG), `vault_context_pack` (assembled context bundle), `vault_ask` (combined retrieval + synthesis)

The graph traversal tool is VaultMind's differentiator: A-RAG retrieves documents but cannot follow relationship edges. VaultMind's knowledge graph enables activation spreading across the note network, surfacing indirectly related notes that pure vector search would miss.

See [[decision-vaultmind-mcp-server|MCP Server Decision]] for the full six-tool design and [[concept-rag|RAG]] for the retrieval-augmented generation foundation.
