---
id: concept-mteb-benchmark
type: concept
title: MTEB Benchmark
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Massive Text Embedding Benchmark
  - MTEB Leaderboard
tags:
  - evaluation
  - benchmark
  - embedding
related_ids:
  - concept-retrieval-evaluation-metrics
  - concept-longbench
  - concept-embedding-based-retrieval
source_ids: []
---

## Overview

The Massive Text Embedding Benchmark (MTEB), introduced by Muennighoff et al. (2022), is the standard evaluation framework for text embedding models. It covers 8 task types across dozens of datasets and languages, making it the primary reference for comparing embedding model candidates.

**Task types covered:**
1. **Classification** — embed text, train a logistic regression, evaluate accuracy
2. **Clustering** — embed texts, cluster with k-means, evaluate NMI/ARI
3. **Pair classification** — embed pairs, classify relationship (duplicate, entailment)
4. **Reranking** — reorder candidates given a query embedding
5. **Retrieval** — given query embeddings, retrieve relevant passages; scored by nDCG@10
6. **Semantic Textual Similarity (STS)** — correlate cosine similarity scores with human judgments
7. **Summarization** — embed summaries vs. source documents
8. **Instruction following** (MTEB v2) — follow task-specific natural language instructions per query

Retrieval is the most relevant task type for RAG and vault search. MTEB retrieval benchmarks include BEIR datasets (NQ, HotpotQA, MSMARCO, SciFact, TREC-COVID, etc.), providing domain diversity.

## Key Properties

- **nDCG@10 for retrieval:** Normalized Discounted Cumulative Gain at rank 10 — the standard metric for retrieval tasks. Rewards relevant documents appearing at the top of the ranked list, with logarithmic discount for lower positions.
- **HuggingFace leaderboard:** All results are publicly available and model-comparable at the MTEB leaderboard on HuggingFace. New model authors submit results for inclusion.
- **Score ranges:** Top proprietary models (OpenAI text-embedding-3-large, Cohere embed-v3) score ~58–60 on MTEB retrieval. Top open-source models (BGE-M3, Arctic Embed L) score ~55–58. Tiny models (all-MiniLM-L6-v2) score ~41–45.
- **Domain sensitivity:** MTEB averages across diverse domains. A model scoring 42 on average may score 50+ on scientific text (SciFact) and 35 on news. For domain-specific vaults, in-domain MTEB subsets are more predictive than overall averages.
- **English vs. multilingual:** MTEB has both English-only and multilingual tracks. English-focused models dominate the English leaderboard; BGE-M3 leads the multilingual track.
- **BEIR subset:** The BEIR (Benchmarking IR) datasets within MTEB are the standard zero-shot retrieval evaluation, testing generalization to unseen domains.

## Connections

MTEB retrieval scores are the primary quantitative input for comparing [[open-source-embedding-models|Open-Source Embedding Models]]. The nDCG@10 metric is defined alongside other [[retrieval-evaluation-metrics|Retrieval Evaluation Metrics]] including MRR and Recall@K. MTEB complements [[longbench|LongBench]] — which evaluates long-context generation — by focusing specifically on embedding quality rather than end-to-end RAG performance.

VaultMind v2: MTEB retrieval nDCG@10 is the metric to use when comparing embedding model candidates. For VaultMind's use case (English academic notes, ~500 token average note length, single-domain), models scoring 45–50 on MTEB retrieval would be sufficient. The vocabulary mismatch that MTEB retrieval stresses (cross-domain generalization) is less severe in a single-author note vault where terminology is consistent. The practical recommendation: use MTEB scores to rule out poor models, not to micro-optimize — the difference between a model scoring 47 and one scoring 52 matters less than getting embeddings running at all.
