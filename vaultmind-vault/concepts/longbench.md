---
id: concept-longbench
type: concept
title: LongBench
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Long Context Benchmark
  - THUDM LongBench
tags:
  - evaluation
  - benchmark
  - long-context
related_ids:
  - concept-lost-in-the-middle
  - concept-rag
  - concept-working-memory
source_ids:
  - source-bai-2023
---

## Overview

LongBench (Bai et al., THUDM/Tsinghua, 2023; published ACL 2024) is the first bilingual, multi-task benchmark designed to evaluate long-context understanding in LLMs. It covers 21 datasets across 6 task categories — single-document QA, multi-document QA, summarization, few-shot learning, synthetic tasks, and code completion — with an average document length of 6,711 words (English) or 13,386 characters (Chinese). The test set contains 4,750 instances. arXiv:2308.14508.

LongBench v2 (2024) substantially extends the scope: 503 questions with context lengths ranging from 8K to 2M words, targeting the frontier of modern context windows. The v2 questions are designed to require careful reading rather than surface-level pattern matching, explicitly testing whether models can locate and use information distributed across very long documents.

## Key Properties

- **Bilingual scope:** English and Chinese, enabling cross-lingual long-context comparison
- **6 task categories:** Single-doc QA, multi-doc QA, summarization, few-shot learning, synthetic tasks, code
- **4,750 test instances (v1); 503 curated questions (v2)**
- **Length range:** v1 averages ~6K words (EN); v2 spans 8K to 2M words
- **Key v1 finding:** GPT-3.5-Turbo-16k outperforms all open-source models but still degrades on longer instances
- **Context compression:** Retrieval-based compression helps weak long-context models but provides no benefit for strong ones
- **Fine-tuning on longer sequences** and scaled position embeddings both improve LongBench scores

## Connections

LongBench evaluates how well models use long context — directly relevant to how agents consume [[context-pack|Context Pack]] output. If the agent model scores poorly on LongBench-style tasks, VaultMind should keep context-pack payloads shorter and more focused rather than attempting to fill the full token budget.

The v1 finding that context compression helps weak models (but not strong ones) is significant for VaultMind's architecture: the context pack acts as a compression layer over the vault graph. For agents running smaller or less capable models, this compression step is especially valuable. For frontier models (GPT-4-class), the filtering is still useful for cost and latency, but its quality impact is lower.

LongBench's multi-doc QA tasks are structurally similar to what VaultMind enables: an agent answering a question by consulting multiple related notes. The [[lost-in-the-middle|Lost in the Middle]] phenomenon documented by Liu et al. is a direct corollary — benchmark performance drops when relevant information is buried in long contexts, which reinforces VaultMind's priority-ordered packing strategy.

See also [[rag-vs-long-context|RAG vs Long Context]] for the broader debate this benchmark informs.
