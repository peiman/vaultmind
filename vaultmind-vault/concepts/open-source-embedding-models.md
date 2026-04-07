---
id: concept-open-source-embedding-models
type: concept
title: Open-Source Embedding Models
created: 2026-04-06
vm_updated: 2026-04-07
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

Open-source embedding models have matured rapidly into a rich ecosystem. A practical taxonomy by size tier guides selection for resource-constrained deployments. All models listed here export to ONNX format, making them runnable in Go via the runtimes described below.

**Evaluation metric:** MTEB Retrieval score (higher = better) from the Massive Text Embedding Benchmark. Scores are approximate averages across MTEB retrieval tasks.

---

## Tier 1: Tiny (under 35M params, <100MB)

Best for CPU-only deployments, fast iteration, and small-to-medium corpora where retrieval quality is secondary to speed.

| Model | Params | Dims | Context | License | MTEB Retrieval | Go Runtime |
|-------|--------|------|---------|---------|----------------|------------|
| all-MiniLM-L6-v2 | 22M | 384 | 512 | Apache 2.0 | ~49 | hugot pure-Go backend (no CGO) ✓ |
| Snowflake Arctic Embed Tiny | 22M | 384 | 512 | Apache 2.0 | ~50 | ONNX exportable, hugot compatible |
| Snowflake Arctic Embed Small | 33M | 384 | 512 | Apache 2.0 | ~51 | ONNX exportable |
| BGE-small-en-v1.5 | 33M | 384 | 512 | MIT | ~51 | ONNX exportable |
| GTE-small | 33M | 384 | 512 | MIT | ~49 | ONNX available on HuggingFace |
| E5-small-v2 | 33M | 384 | 512 | MIT | ~49 | ONNX exportable |

**Notes:**
- all-MiniLM-L6-v2 is the only model confirmed working with the hugot pure-Go backend (no CGO, no system library required).
- Snowflake Arctic Embed Tiny and Small share the same MiniLM architecture but are fine-tuned on a proprietary retrieval dataset, yielding 1–2 MTEB points above vanilla MiniLM.
- BGE-small and GTE-small use the BERT-base architecture truncated to 6 layers; competitive with Arctic Small at the same param count.
- E5-small-v2 from Microsoft uses "query:" / "passage:" instruction prefixes for best results.

---

## Tier 2: Small-Medium (35M–200M params)

Balanced quality-to-cost ratio. Fit in ~500MB RAM. CPU inference is practical for interactive use on corpora under ~10k notes.

| Model | Params | Dims | Context | License | MTEB Retrieval | Go Runtime |
|-------|--------|------|---------|---------|----------------|------------|
| BGE-base-en-v1.5 | 109M | 768 | 512 | MIT | ~53 | ONNX exportable |
| Snowflake Arctic Embed M v1.5 | ~110M | 768 | 512 | Apache 2.0 | ~55 | ONNX exportable |
| Nomic-Embed-Text-v1.5 | 137M | 768 | 8192 | Apache 2.0 | ~55 | ONNX exportable, Matryoshka |
| E5-base-v2 | 109M | 768 | 512 | MIT | ~50 | ONNX exportable |
| GTE-base | 109M | 768 | 512 | MIT | ~52 | ONNX exportable |

**Notes:**
- Nomic-Embed-Text-v1.5 is the standout in this tier: 8192-token context (vs. 512 for the rest), Matryoshka Representation Learning (MRL) so embeddings can be truncated to 256 or 128 dims with minimal quality loss, and fully open weights/data/code under Apache 2.0.
- Snowflake Arctic Embed M v1.5 leads MTEB retrieval at this size class and is a strong default if context window is not a constraint.
- BGE-base is BAAI's workhorse; widely supported across inference frameworks. Instruction-free (no prefix needed).
- E5-base-v2 requires "query:" / "passage:" prefixes; slightly lower MTEB than BGE-base despite identical architecture.
- GTE-base from Alibaba DAMO is solid and MIT-licensed; less commonly benchmarked but competitive.

---

## Tier 3: Large (200M–600M params)

Best retrieval quality. GPU helpful but not required for batch indexing; CPU is viable for small-vault indexing at lower throughput.

| Model | Params | Dims | Context | License | MTEB Retrieval | Go Runtime |
|-------|--------|------|---------|---------|----------------|------------|
| BGE-M3 | 568M | 1024 | 8192 | MIT | ~63 | ONNX available, dense+sparse+ColBERT natively. Needs ORT backend (CGO or purego) |
| mxbai-embed-large-v1 | 335M | 1024 | 512 | Apache 2.0 | ~55 | ONNX exportable, Matryoshka |
| Snowflake Arctic Embed L v2 | 568M | 1024 | 8192 | Apache 2.0 | ~57 | ONNX exportable |
| GTE-multilingual-base | 305M | 768 | 8192 | Apache 2.0 | ~55 | ONNX exportable |
| Jina-Embeddings-v3 | 570M | 1024 | 8192 | CC BY-NC 4.0 | ~58 | ONNX exportable, Task LoRA adapters. NON-COMMERCIAL license |

**Notes:**
- BGE-M3 (BAAI) is the apex open-source embedding model for retrieval: it simultaneously supports dense retrieval, BM25-style sparse retrieval, and [[colbert|ColBERT]]-style multi-vector late interaction from a single forward pass. MIT license. 8192 token context. The go-to choice when hybrid search matters.
- mxbai-embed-large-v1 from Mixedbread AI uses AnglE loss training; strong on asymmetric retrieval (short query, long passage). Matryoshka support.
- Snowflake Arctic Embed L v2 is the multilingual evolution of the Arctic series at scale; strong MTEB retrieval with 8192 context. Apache 2.0.
- GTE-multilingual-base covers 70+ languages at 305M params and 8192 context — efficient multilingual choice. Apache 2.0.
- Jina-Embeddings-v3 uses task-specific LoRA adapters (retrieval, classification, etc.) applied at inference time. **CC BY-NC 4.0 — non-commercial use only.** Not suitable for commercial VaultMind deployments.

---

## Go Runtime Options

Running ONNX embedding models in a Go binary requires one of four approaches. The choice trades off CGO dependency, cross-compilation ease, inference speed, and binary portability.

### Option 1: hugot pure-Go backend (NO CGO)

- **Library:** `knights-analytics/hugot`
- **Build:** `go build` (default, no build tags)
- **Models:** all-MiniLM-L6-v2 confirmed working. Smaller models recommended.
- **Speed:** Slower than ORT (~3–8x) but sufficient for small corpora
- **Limitation:** Best for <35M param models; batches of ~32; no hardware acceleration

Pure Go transformer inference without any native library dependency. The tokenizer and transformer forward pass are implemented in Go. Minimal setup, maximum portability. Suitable for CLI tools distributed as single binaries.

### Option 2: hugot + ONNX Runtime (CGO)

- **Library:** `knights-analytics/hugot` with build tag `-tags ORT`
- **Requires:** `libonnxruntime` shared library installed on the system
- **Models:** Any ONNX model including BGE-M3
- **Speed:** Fastest CPU inference (ONNX Runtime is heavily optimized)
- **Limitation:** CGO dependency; cross-compilation harder; requires runtime library

Full ONNX Runtime acceleration with a Go API. Best raw performance for batch indexing. Harder to distribute; users must install `libonnxruntime` separately or bundle it.

### Option 3: onnxruntime-purego (NO CGO, still uses ONNX Runtime)

- **Library:** `shota3506/onnxruntime-purego` or `amikos-tech/pure-onnx`
- **Mechanism:** Uses `purego` to dynamically load the ONNX Runtime C API without CGO
- **Requires:** `libonnxruntime` at runtime, but NO CGO at build time
- **Cross-compilation:** Works. Faster compilation, Go build cache friendly.
- **Models:** Any ONNX model

This approach splits the difference: no CGO at build time (so `go build` works without a C toolchain), but still delegates math to the optimized ONNX Runtime at runtime. The shared library is loaded dynamically via `purego` at process start. Ideal for distributed CLIs where users may install the runtime separately, or for Docker images where the runtime is bundled alongside the binary.

### Option 4: kelindar/search (embedded vector search in Go)

- **Library:** `kelindar/search`
- **Mechanism:** Uses llama.cpp for embeddings natively in Go
- **Features:** Includes vector search with HNSW index built in
- **Tradeoffs:** Pulls in llama.cpp; larger binary; less model flexibility than ONNX ecosystem

All-in-one embedding + search library. Useful for prototyping or when HNSW search and embedding need to co-locate in a single package with no external index dependency.

---

## VaultMind Recommendation

**For VaultMind v2: Option 3 (onnxruntime-purego) + BGE-M3**

| Criterion | Verdict |
|-----------|---------|
| CGO at build time | No — `go build` works without C toolchain |
| Hybrid search | Dense + sparse + ColBERT from one model |
| Context window | 8192 tokens — handles long notes without truncation |
| License | MIT — fully permissive, commercial use OK |
| Latency at 129 notes | ~15s on CPU for full vault re-index (acceptable for background job) |
| Upgrade path | Drop in any future ONNX model without changing runtime code |

The combination of no-CGO build + full hybrid search from a single MIT-licensed model is uniquely enabled by BGE-M3 + onnxruntime-purego. The 568M param size is not a concern at vault sizes under ~10k notes — even on CPU, a full re-index is a background task that completes in seconds to minutes.

If resource constraints are severe (embedded device, strict binary size limit), fall back to **Snowflake Arctic Embed Tiny** (22M params, Apache 2.0) with Option 1 (hugot pure-Go) for a zero-dependency single binary at the cost of hybrid search.

---

## Key Properties Summary

- **Size vs. quality tradeoff:** Tiny models (22M params) achieve ~80% of the retrieval quality of large models at <5% of the compute cost. The gap closes at retrieval-heavy workloads where hybrid search (BGE-M3) dominates.
- **License matters:** Apache 2.0 and MIT are the two permissive licenses dominant in this space. CC BY-NC 4.0 (Jina) excludes commercial use. Always verify before production use.
- **Dimension count:** Higher dimensions (1024 vs. 384) improve recall but increase storage and ANN search cost proportionally. Matryoshka models (Nomic, mxbai) allow post-hoc dimensionality reduction.
- **Multilingual support:** BGE-M3, GTE-multilingual-base, and Snowflake Arctic Embed L v2 cover multilingual vaults. English-only vaults gain little from multilingual models.
- **Context window:** Most models cap at 512 tokens. Nomic-Embed-Text-v1.5, BGE-M3, Snowflake Arctic Embed L v2, and GTE-multilingual-base extend to 8192 tokens — critical for long note bodies or meeting transcripts.
- **MTEB scores:** All models are publicly ranked on the [[mteb-benchmark|MTEB Benchmark]] leaderboard for objective comparison across retrieval, classification, clustering, and semantic similarity tasks.
- **Instruction prefixes:** E5 family requires "query:" / "passage:" prefixes at inference time. BGE, GTE, Nomic, Snowflake, and mxbai are instruction-free for retrieval use.

---

## Connections

For [[embedding-based-retrieval|Embedding-Based Retrieval]], model selection is the highest-leverage decision: it determines storage size, query latency, and retrieval quality ceiling. [[colbert|ColBERT]]-style late interaction requires models that output per-token embeddings; BGE-M3 is notable for supporting this natively alongside standard sentence-level dense and sparse embeddings. [[dense-passage-retrieval|Dense Passage Retrieval]] (DPR) used custom dual-encoder models trained on NQ; the models here are general-purpose and require no fine-tuning. [[contriever|Contriever]] represents contrastive pre-training without labeled data — a different paradigm from the NLI supervised fine-tuning used by MiniLM and BGE. See [[onnx-inference|ONNX Inference]] for runtime integration details.
