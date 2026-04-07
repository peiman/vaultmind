---
id: source-nogueira-2019
type: source
title: "Nogueira, R., & Cho, K. (2019). Passage Re-ranking with BERT. arXiv:1901.04085."
created: 2026-04-06
vm_updated: 2026-04-06
url: "https://arxiv.org/abs/1901.04085"
aliases:
  - Nogueira 2019
  - BERT Reranking paper
tags:
  - retrieval
  - reranking
related_ids:
  - concept-reranking
  - concept-dense-passage-retrieval
---

# Nogueira & Cho — Passage Re-ranking with BERT (2019)

Nogueira and Cho (NYU) present a simple but highly effective method for passage reranking using BERT. The approach is a two-stage pipeline: BM25 retrieves an initial candidate set of passages for a query, and a fine-tuned BERT model then re-scores each candidate by processing the query and passage as a concatenated input sequence. The BERT model is fine-tuned on the MS MARCO passage ranking dataset, a large-scale dataset of real Bing search queries paired with human-judged relevant passages.

The reranker formulates relevance scoring as binary classification: given the concatenated `[CLS] query [SEP] passage [SEP]` input, BERT predicts whether the passage is relevant to the query, and the probability assigned to the "relevant" class is used as the final relevance score. The system is evaluated on MS MARCO passage ranking and TREC-CAR, achieving state-of-the-art results on both at the time of publication.

## Key Findings

- BERT cross-encoder reranking on top of BM25 achieves approximately 27% relative MRR@10 improvement on the MS MARCO Dev Set compared to BM25 alone, demonstrating the large quality gap between keyword matching and neural reranking
- The reranker is remarkably simple to implement given BERT's pre-training: fine-tuning only requires training a linear classification head on top of the `[CLS]` token representation, with the full BERT stack updated via gradient descent on MS MARCO labels
- Joint query-passage encoding via cross-attention is the key mechanism — the model can attend to token-level interactions that single-vector bi-encoders cannot capture
- The pipeline generalizes across benchmark styles: both the dense-style MS MARCO passage task and the structured document task of TREC-CAR benefit substantially from BERT reranking

## Relevance to VaultMind

This paper establishes the empirical foundation for [[reranking|cross-encoder reranking]] as a practical and effective second-stage retrieval component. The 27% relative improvement figures are directly cited when evaluating whether adding a reranking stage to VaultMind's HybridRetriever pipeline is worth the added latency. The BM25 + BERT reranker pipeline described here maps closely onto a VaultMind retrieval design: BM25 first-stage over vault notes, followed by a BERT reranker that re-scores the top-N candidates given the agent's exact query. For [[dense-passage-retrieval|DPR]]-style first stages, the reranker can be layered on top in the same fashion.
