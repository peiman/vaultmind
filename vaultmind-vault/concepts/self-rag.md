---
id: concept-self-rag
type: concept
title: Self-RAG
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Self-Reflective RAG
  - Adaptive RAG
tags:
  - retrieval
  - self-reflection
  - rag-variant
related_ids:
  - concept-rag
  - concept-reflexion
  - concept-dense-passage-retrieval
source_ids:
  - source-asai-2023
---

## Overview

Self-RAG (Asai et al., 2023) teaches a language model to retrieve, generate, and critique its own outputs using special reflection tokens inserted during training. Rather than always retrieving context before generating (as in standard [[rag|RAG]]), the model learns three interleaved behaviors: (1) decide whether retrieval is needed for a given segment of output, (2) generate with or without retrieved passages as appropriate, and (3) assess the quality and relevance of both the retrieved passages and its own generated text.

This adaptive, on-demand retrieval contrasts with always-on RAG pipelines. Published at ICLR 2024, Self-RAG outperforms ChatGPT and Llama 2 on open-domain QA, reasoning, and fact verification tasks, while also outperforming retrieval-augmented Llama 2 — demonstrating that when to retrieve matters as much as how to retrieve.

## Key Properties

- **Retrieve-or-not decision:** A special `[Retrieve]` token signals whether the model will fetch passages for the next generation segment
- **Passage relevance critique:** `[IsRel]` tokens assess whether a retrieved passage is relevant to the query
- **Output faithfulness critique:** `[IsSup]` tokens assess whether the generated text is supported by the retrieved passage
- **Response quality critique:** `[IsUse]` tokens assess overall response utility
- **Outperforms always-on RAG:** Selective retrieval improves both accuracy and efficiency versus retrieving for every generation step
- **Single model, no separate critic:** Reflection tokens are trained into the base model, not a separate verifier

## Connections

Self-RAG refines standard [[rag|RAG]] by making retrieval conditional rather than mandatory. The reflection token mechanism echoes [[reflexion|Reflexion]] (Shinn et al., 2023): both architectures use language-level signals to critique and correct generation, but Reflexion applies critique across full task attempts while Self-RAG applies it at the granularity of individual output segments.

[[dense-passage-retrieval|Dense Passage Retrieval]] provides the retrieval substrate that Self-RAG can call on demand. [[fusion-in-decoder|Fusion-in-Decoder]] addresses a different dimension — how to fuse multiple retrieved passages — without addressing when to retrieve. Self-RAG and FiD are complementary: one solves retrieval timing, the other solves retrieval aggregation.

VaultMind's retrieval is currently always-on for queries that exceed a relevance threshold. Self-RAG suggests a more sophisticated design: train or prompt the agent to emit a retrieval-request signal before fetching from the vault, and a relevance judgment after receiving results. This would reduce unnecessary graph traversals and let the agent skip retrieval for queries it can answer from parametric knowledge alone.
