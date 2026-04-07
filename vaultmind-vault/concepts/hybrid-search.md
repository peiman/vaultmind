---
id: concept-hybrid-search
type: concept
title: Hybrid Search
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Hybrid Retrieval
  - Sparse-Dense Fusion
tags:
  - retrieval
  - hybrid
  - architecture
related_ids:
  - concept-reciprocal-rank-fusion
  - concept-sparse-retrieval
  - concept-dense-passage-retrieval
  - concept-reranking
source_ids: []
---

## Overview

Hybrid search combines sparse (BM25, keyword-based) and dense (embedding, semantic) retrieval into a single ranked result list. The motivation: sparse and dense retrievers have complementary failure modes. BM25 excels at exact keyword matches but misses semantically related content with different vocabulary. Dense retrievers capture semantic similarity but can rank irrelevant documents highly when query terms are common words with high embedding similarity. Combining both systems consistently outperforms either alone across BEIR benchmarks and MTEB retrieval tasks.

**Combination strategies:**

1. **Reciprocal Rank Fusion (RRF)** — Combine rank positions from both lists using the RRF formula. Requires no score normalization, no training, one hyperparameter (k=60). The standard method for hybrid retrieval without labeled data.

2. **Linear interpolation** — Weighted blend of normalized scores: `score = α · sparse_score + (1-α) · dense_score`. Requires normalizing BM25 scores (unbounded) and cosine scores (bounded [-1,1]) to a common scale. α is tuned on a validation set — but requires labeled data and may not generalize across query types.

3. **Cascaded (sparse-first)** — Use sparse retrieval for high-recall first-stage candidate generation, then use dense embeddings to rerank the top-N candidates. Efficient at large scale; the dense computation only runs on a small candidate set. Used by ColBERT and BGE-M3's hybrid pipelines.

**BGE-M3** is notable for natively supporting dense + sparse + ColBERT-style multi-vector retrieval in a single model, enabling hybrid search with one encoder rather than two separate systems.

## Key Properties

- **Consistent improvement:** Hybrid retrieval outperforms pure BM25 and pure dense retrieval on the majority of BEIR benchmarks. The improvement is especially pronounced on queries that mix exact-term requirements with semantic context.
- **RRF is the robust default:** Empirically, RRF (k=60) matches or exceeds learned interpolation methods that use domain-specific α tuning, particularly in zero-shot settings where no labeled data is available.
- **Query-type sensitivity:** BM25 dominates on entity-heavy queries (names, codes, model numbers). Dense retrieval dominates on conceptual queries ("what causes forgetting"). Hybrid captures both without knowing query type a priori.
- **Retrieval recall is the ceiling:** Hybrid retrieval improves the recall of the candidate set. A [[reranking|Reranker]] applied after hybrid retrieval can improve precision further, but cannot surface documents not in the candidate set.
- **Latency additive:** Hybrid search requires running two retrievers sequentially or in parallel. Parallel execution keeps latency near max(sparse, dense) rather than sparse + dense. For a CLI tool, sequential is simpler and the latency difference at small vault size is negligible.
- **No overlap penalty:** RRF handles documents appearing in both retrieval lists naturally — they receive contributions from both lists and score higher, which is the correct behavior.

## Connections

Hybrid search is the direct synthesis of [[sparse-retrieval|Sparse Retrieval]] (BM25) and [[dense-passage-retrieval|Dense Passage Retrieval]] (embedding-based cosine). The fusion step uses [[reciprocal-rank-fusion|Reciprocal Rank Fusion]] as the standard combination algorithm. After hybrid retrieval, [[reranking|Reranking]] provides a second precision pass over the combined candidate set. BGE-M3 from [[open-source-embedding-models|Open-Source Embedding Models]] is the one model that natively integrates all three retrieval modes.

VaultMind v2: the v2 HybridRetriever combines FTSRetriever (BM25 via SQLite FTS5) and EmbeddingRetriever (cosine similarity over stored ONNX embeddings) using RRF with k=60. The `Retriever` interface is already wired in v1 — both retrievers satisfy it. Implementation plan: run FTSRetriever and EmbeddingRetriever independently for the same query, assign ranks 1..N within each result list, compute RRF scores (1/(60+rank)), merge by note ID, sort descending, return top-N. No score normalization required. No labeled data required. The dual-retriever approach compensates for the vocabulary mismatch weakness of FTS (flagged in the v1 expert panel review) while preserving the exact-match strength for queries containing precise terminology from the note corpus.
