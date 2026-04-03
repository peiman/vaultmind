---
id: decision-structured-over-embeddings
type: decision
status: accepted
title: "Use structured graph retrieval instead of embeddings for v1"
created: 2026-04-03
vm_updated: 2026-04-03
tags: [architecture, retrieval]
related_ids:
  - concept-rag
  - concept-embedding-based-retrieval
source_ids: []
---

# Use Structured Graph Retrieval Instead of Embeddings for v1

## Decision

VaultMind v1 retrieves notes by traversing the explicit relationship graph, not by computing vector similarity with [[Embedding-Based Retrieval]].

## Rationale

**No external dependencies.** Embedding retrieval requires an embedding model (local or remote) and a vector index. Structured graph traversal runs entirely on the parsed vault with no network calls and no additional binaries.

**Precise relational queries.** When an agent asks "what concepts are related to X within 2 hops?", a graph BFS gives an exact, auditable answer. Cosine similarity over embeddings gives a ranked list with opaque provenance.

**Explicit edges are already present.** Vault notes link to each other via wikilinks and `related_ids`. These edges encode human-curated relatedness that embeddings would only approximate.

**Extension point for v2.** The retrieval interface is designed so an embedding-based scorer can be layered on top of graph traversal in v2, combining structural proximity with semantic similarity. See [[RAG]] for prior art on hybrid approaches.

## Trade-offs Accepted

Structured retrieval cannot surface conceptually similar notes that lack explicit links. This is acceptable for v1 where vault quality is high and links are curated.
