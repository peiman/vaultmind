---
id: concept-graph-rag
type: concept
title: GraphRAG
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Graph RAG
  - Microsoft GraphRAG
tags:
  - knowledge-graph
  - retrieval
  - rag-variant
related_ids:
  - concept-rag
  - concept-semantic-networks
  - concept-entity-resolution
source_ids:
  - source-edge-2024
---

## Overview

GraphRAG (Edge et al., Microsoft Research, 2024) is a graph-based retrieval-augmented generation approach designed for query-focused summarization over large corpora. Rather than retrieving flat text passages, GraphRAG builds an entity knowledge graph from source documents during an offline indexing phase, then uses community detection to create hierarchical summaries of that graph.

The two-stage pipeline:
1. **Graph construction:** An LLM extracts entities and relationships from source documents, building a typed property graph.
2. **Community summarization:** Community detection algorithms (e.g., Leiden) partition the graph into clusters; an LLM summarizes each community at multiple levels of granularity.

At query time, relevant community summaries — rather than raw text chunks — are retrieved and used to generate the answer. For global sensemaking questions ("What are the major themes in this corpus?") over large corpora (~1M tokens), GraphRAG substantially outperforms conventional RAG in comprehensiveness and diversity. Microsoft open-sourced the implementation at github.com/microsoft/graphrag.

## Key Properties

- **Offline graph construction:** Entity extraction and community detection happen at index time, not query time
- **Hierarchical summaries:** Multiple levels of community abstraction allow varying specificity in retrieval
- **Global questions:** Excels at questions requiring synthesis across the full corpus, not just local fact lookup
- **LLM-intensive indexing:** Graph construction requires many LLM calls — expensive but amortized across queries
- **Community detection:** Uses graph partitioning (Leiden algorithm) to find coherent topic clusters

## Connections

VaultMind is architecturally a GraphRAG system — the Obsidian vault is a human-authored knowledge graph, and [[context-pack|Context Pack]] retrieval traverses graph edges rather than searching flat embeddings. The critical difference is authorship: VaultMind's graph is built by humans (explicit links, typed relations), while GraphRAG's graph is built by an LLM during indexing.

This distinction matters for precision. Human-authored edges in VaultMind carry higher semantic fidelity than LLM-extracted entities, but LLM-built graphs (GraphRAG) scale to corpora that no human could annotate by hand.

The community-summarization insight is directly applicable to VaultMind: tag clusters and strongly-connected subgraphs within the vault could be pre-summarized to support global queries over the full vault — a capability VaultMind does not yet implement. The [[entity-resolution|Entity Resolution]] system would need to reconcile LLM-extracted entities with existing vault nodes if a hybrid approach were adopted.
