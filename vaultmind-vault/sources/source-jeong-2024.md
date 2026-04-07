---
id: source-jeong-2024
type: source
title: "Jeong, S., et al. (2024). Adaptive-RAG: Learning to Adapt Retrieval-Augmented Large Language Models through Question Complexity. NAACL 2024."
created: 2026-04-06
vm_updated: 2026-04-06
url: "https://arxiv.org/abs/2403.14403"
aliases:
  - Jeong 2024
  - Adaptive-RAG paper
tags:
  - retrieval
  - routing
related_ids:
  - concept-adaptive-rag
  - concept-rag
  - concept-self-rag
---

# Jeong et al. — Adaptive-RAG (NAACL 2024)

Jeong and colleagues present Adaptive-RAG, a framework that dynamically selects among retrieval strategies based on predicted query complexity. The paper's central observation is that applying the same RAG pipeline to all queries is suboptimal: simple queries waste latency on retrieval that the model's parametric knowledge already handles correctly, while complex multi-hop queries are underserved by single-step retrieval. Adaptive-RAG trains a small query complexity classifier and uses its predictions to route queries through one of three strategies: (A) no retrieval, (B) single-step RAG, or (C) iterative multi-step RAG.

The classifier's training labels are derived automatically from the downstream QA performance of each strategy on a held-out set, rather than requiring human annotation of query complexity. This allows the labeling process to scale to large QA datasets. The classifier is a relatively small fine-tuned language model, adding negligible latency compared to the retrieval and generation costs it influences.

## Key Findings

- Adaptive-RAG achieves better or comparable QA accuracy versus always-retrieve and always-multi-step baselines across six open-domain QA benchmarks: PopQA, TriviaQA, MuSiQue, 2WikiMultiHopQA, WebQ, and StrategyQA
- The framework reduces the average number of retrieval calls per query, directly reducing latency and API cost while maintaining downstream accuracy
- The complexity classifier is effective even when trained on automatically derived labels (no human annotation of "simple" vs. "complex"), suggesting the approach scales to new domains without expensive labeling
- Multi-step retrieval is necessary for multi-hop questions (MuSiQue, 2WikiMultiHopQA) but actively harmful for single-hop factoid questions, where extra retrieval steps introduce noise — Adaptive-RAG's routing correctly handles both cases

## Relevance to VaultMind

This paper motivates a routing layer for [[adaptive-rag|VaultMind's retrieval pipeline]]. Not every agent query warrants a vault lookup — many operational or general-purpose queries are better handled parametrically. A complexity-aware routing step upstream of VaultMind search would prevent unnecessary retrieval for easy queries while escalating to multi-note context assembly for complex ones. The paper's automated label generation approach is particularly relevant: VaultMind could derive routing training signal from logged agent interactions rather than requiring manual complexity annotation.
