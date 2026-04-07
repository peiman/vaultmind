---
id: concept-vector-similarity-search
type: concept
title: Vector Similarity Search
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Nearest Neighbor Search
  - ANN Search
  - Vector Search
tags:
  - retrieval
  - embedding
  - architecture
related_ids:
  - concept-dense-passage-retrieval
  - concept-embedding-based-retrieval
  - concept-open-source-embedding-models
source_ids: []
---

## Overview

Vector similarity search finds the nearest neighbors of a query vector within a corpus of document vectors. It is the core computational operation underlying [[embedding-based-retrieval|Embedding-Based Retrieval]]. The problem: given a query vector `q` and a database of `n` vectors, return the `k` vectors most similar to `q` under some distance measure (cosine similarity, dot product, or Euclidean distance).

**Exact search (brute-force):** Computes similarity between `q` and every stored vector. Complexity is O(n·d) where `n` is corpus size and `d` is dimension count. Exact by definition. Practical for small corpora (n < 10K with d = 384 runs in <1ms on modern hardware).

**Approximate Nearest Neighbor (ANN) methods** trade accuracy for speed at large scale:
- **HNSW (Hierarchical Navigable Small World):** Graph-based index. Query complexity O(log n). Highest recall among ANN methods. Used by FAISS, hnswlib, Weaviate. Memory-intensive (stores the graph).
- **IVF (Inverted File Index):** Clusters vectors into buckets; queries only probe a subset of buckets. FAISS's primary index type. Requires training on representative data.
- **Product Quantization (PQ):** Compresses vectors into compact codes (4–64 bytes vs. 1.5KB for fp32 768-dim). Lossy but allows billion-scale search on commodity hardware. Often combined with IVF (IVF-PQ).
- **Annoy (Approximate Nearest Neighbors Oh Yeah):** Forest of random projection trees. Spotify's library. Simple, no GPU, static index (no insertions after build).

**Libraries:** FAISS (Facebook AI Research, C++/Python, production-grade), hnswlib (C++ header-only, Python bindings), Annoy (C++, Python), sqlite-vss (SQLite extension adding HNSW).

## Key Properties

- **Scale threshold:** Below ~10K vectors, brute-force cosine on CPU outperforms ANN in practice — index build time, memory overhead, and query latency all favor exact search at small scale.
- **Recall vs. speed:** ANN methods are parameterized by an `ef_search` or `nprobe` value that controls the recall-speed tradeoff. Higher values improve recall at the cost of latency.
- **Index persistence:** HNSW indexes must be saved to disk and reloaded between sessions. For a CLI tool, this adds an index file alongside the database — a deployment consideration.
- **No standard SQL interface:** Vector similarity search is not part of standard SQL. sqlite-vss adds HNSW to SQLite via a C extension, enabling `SELECT ... ORDER BY vss_distance(embedding, ?)`. However, loading C extensions breaks pure-Go compilation.
- **Distance metrics:** Cosine similarity (normalized dot product) and inner product (dot product on pre-normalized vectors) are equivalent when vectors are unit-normalized. Most embedding models output unit-normalized vectors; VaultMind should normalize embeddings at storage time to enable dot-product search as a cheaper cosine proxy.

## Connections

Vector similarity search is the retrieval layer that makes [[dense-passage-retrieval|Dense Passage Retrieval]] and [[embedding-based-retrieval|Embedding-Based Retrieval]] practical. The choice of exact vs. approximate search governs which infrastructure dependencies are required. [[hybrid-search|Hybrid Search]] combines vector similarity search (dense retrieval) with inverted-index-based sparse retrieval; the outputs are rank lists that are fused via RRF rather than raw scores.

VaultMind v2: at 123 notes with 384-dim embeddings, brute-force cosine similarity over BLOB columns in SQLite is fast enough (well under 1ms per query). Implementation: store embeddings as `BLOB` in the notes table, load all embeddings at query time, compute cosine similarity in Go, sort and return top-N. No FAISS, no external vector DB, no C extensions required — maintains the pure-Go, single-binary constraint. As the vault grows past ~10K notes, HNSW indexing would be needed. The sqlite-vss extension adds HNSW to SQLite but requires a C extension (breaks pure-Go). The upgrade path at that scale would be either CGO with sqlite-vss or an embedded HNSW library with a persistent index file.
