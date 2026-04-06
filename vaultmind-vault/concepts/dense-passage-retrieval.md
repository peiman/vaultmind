---
id: concept-dense-passage-retrieval
type: concept
title: Dense Passage Retrieval
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - DPR
  - Dense Retrieval
tags:
  - retrieval
  - dense-retrieval
  - embedding
related_ids:
  - concept-embedding-based-retrieval
  - concept-rag
source_ids:
  - source-karpukhin-2020
---

## Overview

Dense Passage Retrieval (Karpukhin et al., Facebook AI Research, 2020) is a dual-encoder framework for open-domain question answering. A question encoder and a passage encoder independently map their inputs into dense vectors in a shared embedding space. Retrieval is performed via maximum inner product search (MIPS) over a pre-indexed passage corpus.

The key departure from prior work: rather than relying on sparse keyword signals (BM25, TF-IDF), DPR learns dense representations trained directly on question–answer pairs. At EMNLP 2020 the system outperformed BM25 by 9–19% on top-20 passage retrieval across multiple open-domain QA benchmarks, demonstrating that learned representations can surpass decades of sparse retrieval engineering.

## Key Properties

- **Dual-encoder architecture:** Question and passage encoders are separate BERT-based networks; only their final [CLS] representations are compared
- **MIPS retrieval:** FAISS-based approximate nearest-neighbor search enables retrieval over 21 million Wikipedia passages in milliseconds
- **Trained on QA pairs:** Supervision comes from (question, positive passage, negative passages) triples; negatives include hard in-batch negatives and BM25 negatives
- **Corpus pre-indexing:** Passage embeddings are computed once and stored; only the question embedding is computed at query time
- **9–19% gains over BM25:** Demonstrated on Natural Questions, TriviaQA, WebQuestions, CuratedTREC, and SQuAD open versions

## Connections

DPR is the retrieval backbone used in the original [[rag|RAG]] paper (Lewis et al., 2020) and influenced virtually all subsequent dense retrieval research. It established that training a retriever end-to-end on downstream task signal is more effective than unsupervised sparse matching.

[[realm|REALM]] (Guu et al., 2020) extends the dense retrieval idea further by training the retriever jointly with a language model during pre-training — removing the need for labeled QA pairs as supervision. [[fusion-in-decoder|Fusion-in-Decoder]] (Izacard & Grave, 2021) builds on DPR's retrieval by encoding each retrieved passage independently and fusing them in the decoder, scaling the benefit of retrieval to 100 passages.

VaultMind's structured graph traversal is an explicit alternative to DPR-style embedding search: rather than computing similarity in a learned vector space, VaultMind follows typed edges between notes. The expert panel (Session 02, Hoffmann) recommended a hybrid: structured-first retrieval backed by optional dense retrieval for cold-start or out-of-graph queries, with DPR as the reference architecture for the embedding path.
