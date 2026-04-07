---
id: concept-corrective-rag
type: concept
title: Corrective RAG
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - CRAG
  - Corrective Retrieval Augmented Generation
tags:
  - retrieval
  - rag-variant
  - self-correction
related_ids:
  - concept-rag
  - concept-self-rag
  - concept-hyde
source_ids:
  - source-yan-2024
---

## Overview

Corrective RAG (CRAG; Yan et al., 2024) addresses a fundamental weakness of standard RAG: retrieved documents may be irrelevant, partially relevant, or actively misleading, and the generator has no mechanism to detect or recover from retrieval failure. CRAG adds a retrieval evaluator — a lightweight model that assesses the quality of retrieved documents relative to the query — and uses that assessment to trigger one of three corrective actions:

1. **Correct:** Retrieved documents are sufficiently relevant — proceed with standard RAG generation.
2. **Ambiguous:** Relevance is uncertain — refine the query and supplement with web search.
3. **Incorrect:** Retrieved documents are irrelevant — discard them and fall back entirely to web search.

Beyond the evaluator, CRAG introduces a decompose-then-recompose algorithm for knowledge refinement: retrieved documents are decomposed into fine-grained knowledge strips, irrelevant strips are filtered out, and the remaining relevant strips are recomposed into a clean knowledge base for generation. This prevents irrelevant retrieved content from contaminating the generator's context.

CRAG is plug-and-play: the retrieval evaluator and refinement step can be inserted into any existing RAG pipeline without retraining the generator. arXiv:2401.15884.

## Key Properties

- **Retrieval evaluator:** A lightweight classifier that scores retrieved document relevance as correct / ambiguous / incorrect, triggering adaptive downstream behavior.
- **Three-branch corrective logic:** Confident retrieval proceeds normally; ambiguous triggers query refinement + web search; incorrect discards retrieved docs and uses web search exclusively.
- **Decompose-then-recompose:** Documents broken into fine-grained strips; irrelevant strips filtered; relevant strips reassembled. Reduces noise in the generator's context window.
- **Web search fallback:** CRAG treats web search as a reliable fallback when the local retrieval corpus is insufficient, complementing rather than replacing it.
- **Plug-and-play:** Retrieval evaluator and refinement are modular — insertable into any RAG pipeline without retraining the base LLM or the retriever.
- **Benchmark results:** Evaluated on PopQA, Bio, and PubHealth; CRAG consistently improves over standard RAG across generator architectures including both black-box and fine-tuned LLMs.

## Connections

CRAG belongs to the family of adaptive and self-correcting RAG variants alongside [[self-rag|Self-RAG]], which uses special reflection tokens to decide when to retrieve and how to use retrieved passages. Where Self-RAG modifies the generator itself to produce retrieval decisions inline, CRAG externalizes this as a separate evaluator module, making it lighter-weight and easier to add to existing systems.

The relationship to [[hyde|HyDE]] is complementary: HyDE improves what gets retrieved; CRAG improves what happens after retrieval, including what to do when retrieval fails.

For VaultMind, CRAG's retrieval evaluator concept maps to VaultMind's `doctor` — verifying retrieval quality before presenting to an agent. A CRAG-inspired VaultMind would not simply return the top-k search results but would assess whether those results are genuinely relevant to the query, flag ambiguous retrievals, and either refine the search or indicate that the vault lacks relevant material. This prevents agents from being misled by spurious matches, a known failure mode of pure embedding-based retrieval.
