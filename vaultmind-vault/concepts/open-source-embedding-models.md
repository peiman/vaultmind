---
id: concept-open-source-embedding-models
type: concept
title: Open-Source Embedding Models
created: 2026-04-06
vm_updated: 2026-04-06
aliases:
  - Embedding Model Landscape
  - Open Source Embeddings
tags:
  - retrieval
  - embedding
  - open-source
related_ids:
  - concept-embedding-based-retrieval
  - concept-contriever
  - concept-dense-passage-retrieval
  - concept-colbert
source_ids: []
---

## Overview

Open-source embedding models have matured rapidly. A practical taxonomy by size tier guides selection for resource-constrained deployments.

**Tiny (22–33M params, ~22MB, CPU-friendly):**
- **all-MiniLM-L6-v2** — 22M params, 384 dims, sentence-transformers family. The "hello world" of embeddings. Fast, decent quality. Apache 2.0.
- **Snowflake Arctic Embed Tiny** — 22M params, 384 dims, based on MiniLM. Apache 2.0.
- **Snowflake Arctic Embed Small** — 33M params, 384 dims. Apache 2.0.

**Small-Medium (100–300M params, GPU helpful but CPU possible):**
- **BGE-small-en-v1.5** — 33M params, 384 dims, BAAI. MIT license.
- **BGE-base-en-v1.5** — 109M params, 768 dims, BAAI. MIT license.
- **Nomic-Embed-Text-v1.5** — ~137M params, 768 dims, 8192 context. Apache 2.0. Fully open (data, code, weights).
- **GTE-small** — 33M params, 384 dims, Alibaba. MIT license.

**Large (300M+ params, GPU recommended):**
- **BGE-M3** — 568M params, 1024 dims, 100+ languages, supports dense+sparse+ColBERT retrieval. MIT.
- **Snowflake Arctic Embed L v2.0** — 568M params, multilingual. Apache 2.0.
- **GTE-multilingual-base** — 305M params, 70+ languages. Apache 2.0.

## Key Properties

- **Size vs. quality tradeoff:** Tiny models (22M params) achieve ~80% of the retrieval quality of large models at <5% of the compute cost.
- **License matters:** Apache 2.0 and MIT are the two permissive licenses used widely in this space; both allow commercial use without restriction.
- **Dimension count:** Higher dimensions (768 vs. 384) generally improve recall but increase storage and search cost proportionally.
- **Multilingual support:** BGE-M3 and GTE-multilingual-base cover 70–100+ languages; English-only vaults gain little from multilingual models.
- **Context window:** Most models are trained on passages up to 512 tokens. Nomic-Embed-Text-v1.5 is notable for its 8192-token context — relevant for long note bodies.
- **MTEB scores:** Models are publicly ranked on the [[mteb-benchmark|MTEB Benchmark]] leaderboard for objective comparison.

## Connections

For [[embedding-based-retrieval|Embedding-Based Retrieval]], model selection is the highest-leverage decision: it determines storage size, latency, and retrieval quality ceiling. [[colbert|ColBERT]]-style late interaction requires models that output per-token embeddings rather than single sentence vectors; the models listed here produce sentence-level embeddings. [[dense-passage-retrieval|Dense Passage Retrieval]] (DPR) used custom dual-encoder models trained on NQ; the models here are general-purpose and require no fine-tuning. [[contriever|Contriever]] is an example of a dense retrieval model trained without labeled data via contrastive pre-training, representing a different training paradigm from supervised fine-tuning on NLI pairs (used by MiniLM and BGE).

VaultMind v2: for a 123-note vault with English academic content, a tiny model (all-MiniLM-L6-v2 or Arctic Embed Tiny) is more than sufficient. The entire vault can be embedded in under 2 seconds on CPU. Recommended starting point: all-MiniLM-L6-v2 (22MB, Apache 2.0, runs in Go via hugot/ONNX — see [[onnx-inference|ONNX Inference]]). If quality proves insufficient (sparse MTEB retrieval scores on the specific note corpus), upgrade to nomic-embed-text-v1.5 (137M params, 8192 context, Apache 2.0).
