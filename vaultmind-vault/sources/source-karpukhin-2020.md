---
id: source-karpukhin-2020
type: source
title: "Karpukhin, V., et al. (2020). Dense Passage Retrieval for Open-Domain Question Answering. EMNLP 2020."
created: 2026-04-06
vm_updated: 2026-04-06
url: "https://doi.org/10.18653/v1/2020.emnlp-main.550"
aliases:
  - Karpukhin 2020
  - DPR paper
tags:
  - retrieval
  - dense-retrieval
related_ids:
  - concept-dense-passage-retrieval
  - concept-rag
---

# Karpukhin et al. — DPR (EMNLP 2020)

Karpukhin et al. replaced BM25 with a dual-encoder dense retrieval model for open-domain QA. Two BERT-based encoders — one for questions, one for passages — are trained with a contrastive objective using question–positive-passage pairs drawn from existing QA datasets (Natural Questions, TriviaQA, WebQuestions, CuratedTREC, SQuAD). Hard negatives from BM25 top-k and in-batch negatives from other questions in the same batch proved critical to retrieval quality. The resulting passage embeddings are indexed with FAISS for millisecond-scale approximate nearest-neighbor retrieval over 21 million Wikipedia passages.

The paper's central empirical finding is that supervised dense retrieval decisively outperforms BM25 on passage retrieval: 9–19 percentage point improvements in top-20 retrieval accuracy across five QA benchmarks. Combined with a strong reader model, the full DPR pipeline set new state of the art on four of five benchmarks at EMNLP 2020. The work established a reproducible open-source baseline (code and model weights released) that accelerated subsequent retrieval research significantly.

## Key Findings

- 9–19% improvement over BM25 on top-20 retrieval across Natural Questions, TriviaQA, WebQuestions, CuratedTREC, and SQuAD Open
- Hard negatives (BM25 top-k that don't contain the answer) are essential — random negatives produce substantially weaker retrievers
- Larger training sets help, but the model is surprisingly data-efficient: even 1,000 labeled examples yield competitive retrieval
- Exact match on downstream QA improves by 1–11 points over BM25-based pipelines when holding the reader model constant
- FAISS HNSW index retrieves top-100 passages from 21M Wikipedia passages in under 1 ms

## Relevance to VaultMind

DPR is the retrieval architecture used in the original [[rag|RAG]] paper and the standard reference point for dense retrieval in open-domain QA. For VaultMind, it defines the performance ceiling and design pattern for any optional embedding-based retrieval extension in v2: dual encoders, contrastive training, FAISS indexing. The paper's finding that hard negatives matter is directly applicable — a VaultMind dense retriever would need to surface plausible-but-irrelevant vault notes as training negatives to learn discriminative representations.
