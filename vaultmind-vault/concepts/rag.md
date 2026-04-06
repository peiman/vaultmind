---
id: concept-rag
type: concept
title: Retrieval-Augmented Generation
created: 2026-04-03
vm_updated: 2026-04-03
aliases:
  - RAG
  - Retrieval Augmented Generation
tags:
  - ai-memory
  - retrieval
  - llm
related_ids:
  - concept-embedding-based-retrieval
  - concept-context-pack
  - concept-working-memory
source_ids:
  - source-lewis-2020
---

## Overview

Retrieval-Augmented Generation (RAG) is an architecture where an LLM's generation is conditioned on retrieved documents from an external knowledge base. Instead of relying solely on parametric knowledge (weights), the model is given relevant context at inference time, improving factual accuracy and enabling knowledge updates without retraining.

The standard RAG pipeline: query → embed → vector search → top-k retrieval → inject into prompt → generate response.

## Key Properties

- **Decouples knowledge from model:** External knowledge base can be updated without retraining
- **Reduces hallucination:** Grounding generation in retrieved documents improves factual accuracy
- **Scalable:** Knowledge base can grow independently of model size
- **Retrieval quality is the bottleneck:** If retrieval returns irrelevant documents, generation quality degrades regardless of model capability

## Structured vs Embedding-Based Retrieval

VaultMind represents an alternative to traditional RAG: structured graph retrieval rather than embedding-based vector search. The tradeoffs:

| Dimension | Embedding-based (RAG) | Structured (VaultMind) |
|-----------|----------------------|----------------------|
| Query type | Fuzzy semantic similarity | Explicit graph traversal |
| Cold start | Handles novel queries | Requires existing edges |
| Precision | Can retrieve tangentially related | Precise typed relationships |
| Relational queries | Poor ("find parent of X") | Excellent |
| Maintenance | Re-embed on changes | Re-index on changes |

## Connections

VaultMind's [[context-pack|Context Pack]] serves the same role as RAG's retrieval step — selecting relevant content to inject into an agent's context window. The key difference is that VaultMind traverses typed graph edges rather than computing vector similarity. The expert panel (Session 02, Hoffmann) recommended a hybrid approach: structured-first retrieval with optional [[embedding-based-retrieval|Embedding-Based Retrieval]] as a fallback for cold-start queries.
