---
id: concept-reranking
type: concept
title: Reranking
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Cross-Encoder Reranking
  - Two-Stage Retrieval
tags:
  - retrieval
  - reranking
  - architecture
related_ids:
  - concept-dense-passage-retrieval
  - concept-colbert
  - concept-fusion-in-decoder
source_ids:
  - source-nogueira-2019
---

## Overview

Reranking is a two-stage retrieval design that decouples the efficiency concerns of first-stage retrieval from the accuracy demands of final ranking. A fast first-stage retriever — BM25 sparse retrieval or a bi-encoder dense model — produces a candidate set (typically 50–1000 passages) in milliseconds. A second-stage cross-encoder reranker then re-scores each candidate by processing the query and passage jointly, producing a refined ranked list. Nogueira & Cho (2019) demonstrated that a BERT cross-encoder reranker achieves a 27% relative improvement in MRR over the BM25 first stage on MS MARCO, establishing cross-encoder reranking as a standard component in high-accuracy retrieval pipelines.

The core distinction is how query-document interaction is computed. A bi-encoder encodes query and document independently into separate vectors; relevance is approximated by their dot product or cosine similarity. This enables fast approximate nearest-neighbor search but compresses query-document interaction into a single number. A cross-encoder concatenates the query and document as a single input sequence (e.g., `[CLS] query [SEP] document [SEP]`) and produces a relevance score from the full joint encoding. This allows the model to attend across every pair of query and document tokens — capturing fine-grained relevance signals that single-vector representations cannot express.

The trade-off is latency and throughput. Cross-encoders cannot precompute document representations; every query-document pair requires a fresh forward pass through the model. This makes cross-encoder reranking infeasible as a first-stage retriever over a large corpus but tractable as a second stage applied to the O(100) candidates produced by the first stage.

## Key Properties

- **Two-stage architecture:** First stage (BM25 or bi-encoder) provides fast candidate retrieval; second stage (cross-encoder) reranks a small candidate set for accuracy.
- **Joint query-document encoding:** Cross-encoders see query and document tokens together in one input sequence, enabling full cross-attention over all token pairs. This captures exact lexical matches, negation, comparison, and other fine-grained relevance signals.
- **Nogueira & Cho (2019) result:** BERT cross-encoder reranking on top of BM25 first-stage achieves ~27% relative MRR@10 improvement on MS MARCO passage ranking, demonstrating the large gap between first-stage and reranked accuracy.
- **Latency trade-off:** Reranking 100 candidates with BERT-base adds ~100–500ms latency on CPU, but only ~10–50ms on GPU. Practical systems tune the candidate set size to balance latency and accuracy.
- **Model capacity:** Cross-encoder rerankers can use larger, more expensive models than first-stage retrievers since they process only a small candidate set. This makes it cost-effective to apply high-capacity models at the reranking stage.
- **MonoBERT and MonoT5:** Common reranker architectures post-Nogueira 2019. MonoT5 formulates reranking as a sequence-to-sequence generation task ("true"/"false" output tokens), enabling reranking via language model likelihood.

## Connections

Reranking sits architecturally between [[dense-passage-retrieval|DPR]]-style single-vector retrieval and [[colbert|ColBERT]] late interaction. DPR retrieves via approximate nearest-neighbor search with no query-document interaction; ColBERT adds token-level interaction via MaxSim but still separates encoding; a cross-encoder reranker uses full joint encoding and is the most expressive but least scalable of the three.

For [[fusion-in-decoder|FiD]]-style generation, reranking is a natural preprocessing step: a reranker filters the retrieved passages before they are packed into the FiD encoder, reducing the total context length and improving the signal-to-noise ratio.

For VaultMind, a reranking stage would fit naturally into the v2 HybridRetriever design: BM25 and embedding retrieval each produce candidate notes, which are merged into a combined candidate set of ~100 notes; a cross-encoder reranker then re-scores these candidates to produce the top-10 notes assembled into the [[context-pack|Context Pack]]. The reranker would see the agent's full query alongside each note's content, enabling it to surface notes that are semantically relevant to the specific question rather than just the query keywords. Given vault sizes (hundreds to low thousands of notes), even CPU-based reranking at this scale is practical.
