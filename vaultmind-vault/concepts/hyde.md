---
id: concept-hyde
type: concept
title: HyDE
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Hypothetical Document Embeddings
  - HyDE Retrieval
tags:
  - retrieval
  - zero-shot
  - embedding
related_ids:
  - concept-dense-passage-retrieval
  - concept-rag
  - concept-embedding-based-retrieval
source_ids:
  - source-gao-2023
---

## Overview

HyDE (Hypothetical Document Embeddings; Gao et al., 2023) addresses a fundamental asymmetry in dense retrieval: queries are typically short and abstractly phrased, while documents are long and concrete. Embedding models trained on symmetric similarity (document-document) often produce poor alignment between query vectors and document vectors, degrading retrieval quality.

HyDE resolves this by inverting the retrieval problem. Given a query, an LLM first generates a hypothetical answer document — a plausible, detailed response to the query, written in the style of a real document. This hypothetical document is then embedded, and the embedding is used to retrieve real documents by cosine similarity. The intuition is that even if the LLM's hypothetical document contains factual errors, its embedding will be geometrically close to real documents on the same topic, because the surface form (vocabulary, entities, style) is correct.

HyDE is zero-shot: it requires no relevance labels, no training of a specialized retrieval model, and no fine-tuning beyond what the base LLM and encoder already provide. On BEIR benchmarks, HyDE outperforms Contriever (a strong unsupervised dense retriever) across multiple domains. Published at ACL 2023 (arXiv:2212.10496).

## Key Properties

- **Query expansion via generation:** Instead of expanding a query with synonyms or related terms (classic query expansion), HyDE expands it into a full document-length hypothetical answer, capturing the kind of text a relevant document would contain.
- **Zero-shot:** No relevance labels required. The retrieval improvement comes from better query-document geometric alignment in embedding space, achieved through generation.
- **Hallucination-tolerant:** Factual errors in the hypothetical document do not hurt retrieval because what matters is the embedding, not the truth of the content. The generated text provides vocabulary and style signals, not verified facts.
- **Encoder-agnostic:** HyDE works with any dense retrieval encoder. The paper evaluated with Contriever (unsupervised) and showed gains; combining with supervised encoders like GTR yields further improvements.
- **Outperforms Contriever zero-shot:** On multiple BEIR tasks (including TREC-COVID, SciFact, NFCorpus, Arguana), HyDE without any task-specific training exceeds Contriever's retrieval accuracy.
- **Simple to implement:** The full pipeline is: LLM prompt → hypothetical document text → embed → nearest-neighbor search. No special training loop needed.

## Connections

HyDE is a retrieval-time technique that improves [[dense-passage-retrieval|dense retrieval]] models without retraining them. It complements [[rag|RAG]] pipelines as a drop-in retrieval improvement: replace the query embedding with a HyDE embedding and retrieval quality improves, especially for queries phrased as questions rather than as document-like text.

The relationship to [[embedding-based-retrieval|embedding-based retrieval]] is that HyDE shifts the embedding space alignment problem from the encoder (train a better model) to generation (produce a better query representation at runtime).

For VaultMind, HyDE suggests a powerful future retrieval mode. When a user asks "what did I decide about the authentication architecture?", the current system embeds this question and finds similar notes. A HyDE-augmented system would first generate a hypothetical ideal note — "Authentication Architecture Decision: We chose JWT-based stateless tokens for the API layer because..." — and then retrieve real vault notes closest to that hypothetical. This bridges the gap between how users ask questions and how notes are written, potentially surfacing highly relevant notes that a raw query embedding would miss.
