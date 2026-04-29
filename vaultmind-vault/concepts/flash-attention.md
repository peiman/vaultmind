---
id: concept-flash-attention
type: concept
title: FlashAttention
created: 2026-04-29
vm_updated: 2026-04-29
aliases:
  - Flash Attention
  - IO-Aware Attention
tags:
  - deep-learning
  - transformers
  - systems
  - attention
  - long-context
related_ids:
  - concept-attention-mechanism
  - concept-transformer
  - concept-mamba-state-space-models
  - concept-gpt
source_ids:
  - source-dao-2022
---

## Overview

FlashAttention, introduced by [[source-dao-2022|Dao et al. 2022]] at Stanford, is an exact attention algorithm that is dramatically faster and far more memory-efficient than the standard implementation by being IO-aware — minimizing reads and writes between the GPU's slow high-bandwidth memory (HBM) and its fast on-chip SRAM. The key trick is to never materialize the full N×N attention matrix in HBM; instead, the attention computation is tiled and computed incrementally in SRAM, using an online-softmax algorithm that allows correct softmax normalization without seeing the whole row at once.

FlashAttention is exact — there is no approximation, no sparsity, no quality loss compared to the textbook attention formula. The wins are pure systems: 15% faster BERT-large training, 3× faster GPT-2 training, and the unlocking of practical 64K-token context windows in 2022, with later versions extending to millions of tokens. It is now the default attention kernel in PyTorch, JAX, and every serious transformer training and serving stack.

## How It Works

**The bottleneck.** Standard attention computes `softmax(QK^T / √d) V`. The intermediate Q K^T matrix is N×N. For N = 8192 and FP16, that's 128MB per head per layer — far larger than SRAM (~100KB on A100). The implementation reads Q, K, V from HBM, computes Q K^T to HBM, reads it back to apply softmax, writes the result to HBM, reads it again to multiply by V, and writes the final output. This memory traffic, not the FLOPs, dominates wall-clock time.

**FlashAttention's approach.** Split Q, K, V into blocks small enough to fit in SRAM. For each block of Q, iterate over blocks of K and V, accumulating a partial attention output and a running softmax normalization. The online-softmax trick (Milakov & Gimelshein 2018) computes correct softmax incrementally:

- Track the running max m and the running sum-of-exponentials ℓ for each query row.
- When a new K block arrives, rescale the previously-accumulated output and statistics to incorporate the new contributions.

After all K/V blocks have been processed, the per-row softmax normalization is exact, and the output has been written to HBM exactly once. The N×N matrix is never materialized.

**Backward pass.** The same tiling applied with recomputation: instead of saving the full attention matrix for the backward pass (impossible at long N), recompute it block-by-block from the saved Q, K, V plus the saved softmax statistics. Trades a small amount of FLOPs for a huge memory saving — a win because attention is memory-bound, not compute-bound.

## Recent Developments

- **FlashAttention-2 (Dao 2023)** — better work partitioning across thread blocks, fewer non-matmul FLOPs, ~2× faster than FA-1.
- **FlashAttention-3 (2024)** — Hopper-specific: exploits async copies (TMA), warp-specialization, FP8.
- **Paged Attention (vLLM, Kwon et al. 2023)** — extends the IO-aware idea to KV-cache management for serving variable-length conversations efficiently.
- **Ring Attention (Liu et al. 2023)** — distributes the FlashAttention computation across devices for sequences too long to fit on one GPU; the substrate of multi-million-token context.
- **FlashDecoding (2023)** — tailored kernel for the inference-time KV-cache attention pattern, where Q is short and K, V are long.
- **The IO-aware design principle** generalized: every modern transformer kernel (LayerNorm, RMSNorm, GeGLU, MoE routing) is now written IO-aware.

## Connections

FlashAttention is part of the modern frontier-efficiency stack alongside [[concept-mixture-of-experts|MoE]] (capacity-vs-active-compute), quantization (precision-vs-cost), and [[concept-mamba-state-space-models|state-space models]] (architectural rather than systems-level alternative to attention). Each attacks a different axis of the cost surface; FlashAttention attacks the constant in attention's quadratic-time cost. The two are complementary — Mamba avoids quadratic attention entirely, FlashAttention makes it survivable when you do want to use it.

By making 64K–1M-token context windows practical, FlashAttention is also the silent enabler of long-context retrieval-augmented generation, agentic loops with long histories, and document-level reasoning — making it directly upstream of the [[concept-gpt|GPT]]-lineage long-context capabilities that every retrieval system, including VaultMind, is built on top of.

The cognitive analog is the principle that brain-like memory systems are bandwidth-bound between cortical regions, not compute-bound — selectivity and locality of access dominate raw throughput. FlashAttention's IO-awareness is a deliberate parallel to that constraint structure in silicon.
