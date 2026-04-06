---
id: concept-retro
type: concept
title: RETRO
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Retrieval-Enhanced Transformer
  - RETRO Architecture
tags:
  - llm-memory
  - retrieval-augmented
  - architecture
related_ids:
  - concept-rag
  - concept-embedding-based-retrieval
  - concept-memgpt
source_ids:
  - source-borgeaud-2022
---

## Overview

RETRO (Retrieval-Enhanced Transformer, Borgeaud et al., DeepMind 2022) is a language model architecture that retrieves from a 2 trillion token database at inference time rather than encoding all world knowledge into parameters. The model chunks input into 64-token segments, retrieves the nearest neighbors for each chunk from the frozen database using a BERT-based retriever, and conditions generation on the retrieved chunks via **chunked cross-attention**.

The headline result: a 7.5B parameter RETRO model matches GPT-3 (175B parameters) on language modeling benchmarks — roughly 25x parameter efficiency. The key insight is that retrieval can substitute for parameter count: instead of storing knowledge in weights, RETRO looks it up.

## Key Properties

- **Chunked cross-attention:** Generation is conditioned on retrieved neighbors per input chunk, not on a single retrieval for the whole prompt. This enables fine-grained grounding.
- **Frozen retriever:** The BERT retriever is not fine-tuned during training. This separates the retrieval index from the generative model, allowing independent updates to either.
- **2T token database:** The retrieval corpus is orders of magnitude larger than what could be encoded in model weights, making it a form of external parametric-free memory.
- **Parameter efficiency:** 25x fewer parameters than GPT-3 for comparable perplexity, demonstrating that retrieval compresses the knowledge requirement.
- **Training-time retrieval:** Unlike RAG, retrieval happens during training as well as inference, so the model learns to use retrieved context effectively.

## Connections

RETRO sits between [[rag|RAG]] and fully parametric models on the memory spectrum. RAG retrieves into a prompt that the model was not trained to consume; RETRO trains the model to condition generation on retrieved neighbors via a dedicated cross-attention mechanism. This architectural commitment produces better retrieval utilization but requires retrieval at training time.

The chunked cross-attention mechanism is architecturally distinct from [[memorizing-transformers|Memorizing Transformers]], which stores (key, value) pairs from past contexts. RETRO retrieves from a fixed external corpus; Memorizing Transformers accumulate memory from the input sequence itself.

For VaultMind, RETRO suggests that the retrieval granularity matters: chunk-level retrieval (64 tokens) outperforms document-level retrieval in generation quality. VaultMind's [[context-pack|Context Pack]] currently operates at note granularity; finer-grained retrieval within notes is a future consideration.
