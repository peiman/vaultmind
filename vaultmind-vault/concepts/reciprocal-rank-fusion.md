---
id: concept-reciprocal-rank-fusion
type: concept
title: Reciprocal Rank Fusion
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - RRF
  - Rank Fusion
tags:
  - retrieval
  - score-fusion
  - hybrid
related_ids:
  - concept-rag
  - concept-dense-passage-retrieval
  - concept-colbert
source_ids: []
---

## Overview

Reciprocal Rank Fusion (RRF) is a simple, parameter-light method for combining multiple ranked result lists into a single unified ranking, introduced by Cormack, Clarke, and Butt (2009). The scoring formula for each document is:

```
RRF_score(d) = Σ_i  1 / (k + rank_i(d))
```

Where `rank_i(d)` is the position of document `d` in ranked list `i` (1-indexed), and `k` is a smoothing constant (standard value: 60). Documents not present in a given list are assigned a rank of infinity (contributing 0 to that list's sum). The final ranking is produced by sorting documents by descending RRF score.

RRF was originally proposed for combining results from different web search engines but became the standard fusion method for hybrid retrieval systems combining sparse and dense retrievers.

## Key Properties

- **No score normalization required:** The method operates on ranks, not scores. BM25 scores (unbounded positive reals) and cosine similarity scores (bounded [-1, 1]) cannot be directly added without normalization that introduces its own hyperparameters and distributional assumptions. RRF sidesteps this entirely.
- **No training required:** RRF has one hyperparameter (k=60), which empirically works well across a wide range of tasks without tuning. Learned combination methods (linear interpolation with tuned weights, neural re-ranking) require labeled data and can overfit to domain.
- **Robust to outliers:** The reciprocal function caps the contribution of any single list. A document ranked 1st in one list contributes 1/(60+1) ≈ 0.016, regardless of how large its raw score was. This prevents a single dominant retriever from overwhelming the fusion.
- **Empirical performance:** Cormack et al. (2009) demonstrated RRF outperforming learned combination methods on TREC benchmarks despite its simplicity. Subsequent hybrid retrieval literature consistently shows RRF competitive with or superior to more complex fusion strategies.
- **Standard k=60:** The smoothing constant prevents top-ranked documents from receiving disproportionately high scores. Values between 40–80 produce similar results; k=60 is the community convention.
- **Handles missing documents gracefully:** Documents retrieved by only one ranker receive partial credit from that ranker — they are not penalized as heavily as documents absent from all lists.

## Connections

RRF is the natural algorithm for VaultMind's v2 HybridRetriever, which will combine full-text search (SQLite FTS5, effectively BM25-ranked) with embedding-based retrieval (cosine similarity ranked). The two retrievers produce incompatible score scales — RRF allows their ranked lists to be combined without normalization. Implementation: retrieve top-N from FTS and top-N from embedding search, assign ranks 1..N within each list, compute RRF scores, sort. The [[dense-passage-retrieval|Dense Passage Retrieval]] and [[colbert|ColBERT]] literature both use RRF or variants for multi-stage retrieval pipelines. The [[rag|RAG]] framework benefits from hybrid retrieval because sparse methods (BM25) excel at exact keyword matches while dense methods excel at semantic similarity — RRF combines their complementary strengths without requiring a training set of relevance labels for the specific domain.
