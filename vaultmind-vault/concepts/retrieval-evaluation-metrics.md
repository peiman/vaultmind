---
id: concept-retrieval-evaluation-metrics
type: concept
title: Retrieval Evaluation Metrics
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - IR Metrics
  - NDCG MAP MRR
tags:
  - evaluation
  - retrieval
  - benchmark
related_ids:
  - concept-dense-passage-retrieval
  - concept-longbench
  - concept-rag-vs-long-context
source_ids: []
---

## Overview

Retrieval evaluation metrics quantify the quality of a ranked list of documents returned by a retrieval system in response to a query, given a ground truth set of relevant documents. The core challenge is that both the rank of relevant items and the completeness of retrieval matter, and different metrics weight these concerns differently.

These metrics originate in the information retrieval literature and are used to benchmark everything from web search engines to RAG pipeline retrievers. They assume a relevance judgment for each (query, document) pair — either binary (relevant / not relevant) or graded (0, 1, 2, ...).

## Key Metrics

- **Precision@K:** Of the top-K returned documents, what fraction are relevant? Measures the quality of the top slice. Ignores documents ranked beyond K and does not penalize for ordering within K.

- **Recall@K:** Of all relevant documents in the corpus, what fraction appear in the top K? Measures coverage. Useful when completeness matters (e.g., legal discovery), less useful when the corpus has many relevant documents.

- **MRR (Mean Reciprocal Rank):** For each query, compute 1/rank of the first relevant result; average across queries. Focuses entirely on how early the *first* relevant result appears. Appropriate when users want a single correct answer (e.g., QA).

- **MAP (Mean Average Precision):** For each query, compute precision at each rank position where a relevant document appears, then average those precision values (Average Precision). Average AP across queries. Penalizes gaps between relevant items and rewards early, dense concentration of relevant results.

- **NDCG@K (Normalized Discounted Cumulative Gain):** Sums the graded relevance of each top-K document, discounted by log2(rank+1), then normalizes by the ideal DCG (perfect ranking). Handles graded relevance and applies a natural diminishing-returns discount for lower-ranked positions. The most widely used metric for production ranking systems.

## Connections

These metrics are used to evaluate [[dense-passage-retrieval|Dense Passage Retrieval]] systems (DPR reports top-20 and top-100 recall) and appear in [[longbench|LongBench]] as part of the broader question-answering pipeline evaluation. The [[rag-vs-long-context|RAG vs Long Context]] literature compares retrieval configurations partly using recall@K to measure whether relevant context is included in the LLM's input.

VaultMind's retrieval quality is currently evaluated by informal inspection. Adopting formal IR metrics would require a labeled evaluation set: (query, set of relevant note IDs). Given such a set, NDCG@10 over the search result list and Recall@K over the context-pack contents would provide objective baselines for v2 retrieval improvements, making it possible to measure whether adding dense retrieval or reranking actually improves over the BM25 baseline in quantifiable terms.
