---
id: concept-memorizing-transformers
type: concept
title: Memorizing Transformers
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - kNN-Augmented Transformer
  - External Memory Transformer
tags:
  - llm-memory
  - architecture
  - external-memory
related_ids:
  - concept-retro
  - concept-working-memory
  - concept-rag
source_ids:
  - source-wu-2022
---

## Overview

Memorizing Transformers (Wu et al., Google Brain, 2022) extends the standard Transformer by appending a non-differentiable external memory of (key, value) pairs drawn from past input tokens. At each layer that uses external memory, the model performs an approximate kNN lookup over stored KV pairs alongside its standard local attention, then attends over both the local context and retrieved memory entries jointly.

Published at ICLR 2022, the architecture was evaluated on language modeling benchmarks across code (GitHub), mathematics (arXiv), and literature (books). Memory sizes up to 262,144 tokens consistently improved perplexity — gains that could not be obtained simply by increasing model size or using longer local context windows.

## Key Properties

- **Non-differentiable memory:** KV pairs are stored and retrieved without gradients flowing through retrieval. The model learns to use retrieved entries; retrieval itself is fixed approximate kNN.
- **Extends the local attention window:** Memory does not replace local attention — it augments it. The model attends over a local window plus retrieved distant tokens.
- **Input-derived keys:** Unlike [[retro|RETRO]], which retrieves from an external corpus, Memorizing Transformers store (key, value) pairs computed from tokens in the model's own past context. Memory accumulates from the input sequence.
- **Approximate kNN lookup:** Exact kNN over 262K entries is expensive; the system uses ScaNN (Google's approximate nearest-neighbor library) for retrieval efficiency.
- **Domain-general gains:** Improvements held across code, math, and prose — suggesting the benefit is structural, not domain-specific.

## Connections

Memorizing Transformers occupy a distinct position in the memory architecture space. Unlike [[rag|RAG]] (which retrieves from an external, updateable knowledge base at inference time) and [[retro|RETRO]] (which retrieves from a frozen corpus at both training and inference time), Memorizing Transformers accumulate memory from the current input sequence — making them suitable for very long documents, codebases, or multi-document contexts.

The closest cognitive analog is [[working-memory|Working Memory]] extended beyond its normal capacity: the model retains access to tokens it processed earlier, even after they have scrolled out of the active context window.

For VaultMind, this paper motivates the design principle that vault-wide retrieval should complement, not replace, the contents of the active context window. A hybrid architecture could use Memorizing Transformer-style within-session KV memory for recently accessed notes alongside VaultMind's graph-based cross-session retrieval.
