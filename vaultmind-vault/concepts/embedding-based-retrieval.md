---
id: concept-embedding-based-retrieval
type: concept
title: Embedding-Based Retrieval
created: 2026-04-03
vm_updated: 2026-04-03
aliases:
  - Vector Search
  - Semantic Search
  - Dense Retrieval
tags:
  - ai-memory
  - retrieval
  - embeddings
related_ids:
  - concept-rag
  - concept-associative-memory
source_ids: []
---

## Overview

Embedding-based retrieval converts text into dense vector representations (embeddings) and finds similar content via nearest-neighbor search in vector space. This enables fuzzy semantic matching — a query about "neural plasticity" can surface documents about "synaptic weight updates" even without shared keywords.

Modern embedding models (OpenAI text-embedding-3, Cohere embed-v3, sentence-transformers) produce 256-3072 dimensional vectors. Similarity is measured via cosine distance, dot product, or Euclidean distance.

## Key Properties

- **Semantic matching:** Captures meaning beyond keyword overlap
- **Cold-start capable:** Can find related content without explicit links
- **Embedding drift:** Model updates change the vector space, requiring re-embedding
- **Dimensionality-accuracy tradeoff:** Higher dimensions = better precision but slower search
- **Not compositional:** "A causes B" and "B causes A" may have similar embeddings despite opposite meanings

## Connections

VaultMind v1 deliberately avoids embeddings to maintain its no-external-dependencies principle. All retrieval is structured (graph traversal + FTS). The expert panel (Session 02, Hoffmann) recommended defining an embedding extension point for v2: an optional vector store for semantic expansion when graph traversal returns sparse results. This would make VaultMind a hybrid retrieval system — structured-first with embedding fallback.
