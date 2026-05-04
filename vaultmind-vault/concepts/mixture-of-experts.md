---
id: concept-mixture-of-experts
type: concept
title: Mixture of Experts
created: 2026-04-29
aliases:
  - MoE
  - Sparse Mixture of Experts
  - Sparsely-Gated MoE
tags:
  - llms
  - deep-learning
  - mixture-of-experts
  - architecture
related_ids:
  - concept-scaling-laws
  - concept-transformer
  - concept-flash-attention
source_ids:
  - source-shazeer-2017
  - source-fedus-2021
---

## Overview

Mixture of Experts (MoE) is a conditional-computation pattern: a layer contains many parallel sub-networks ("experts"), and a learned gating function routes each input to a small subset of them. Only the chosen experts run, so the layer's compute scales with the number of active experts (typically 1 or 2) rather than the total parameter count. This decouples model capacity from per-token compute — a model with 10× the parameters can still cost the same per token.

In modern LLMs, MoE replaces the dense feedforward block in some or all transformer layers. Frontier sparse models — Mixtral 8×7B, DeepSeek-V3 (671B total / 37B active), GPT-4 (rumored), Switch-C — use MoE to push parameter counts into the trillions while keeping inference latency tractable.

## How It Works

A standard MoE feedforward block:

1. **Gating.** A small linear layer `g(x) = softmax(Wₓ + noise)` produces a score per expert.
2. **Top-k routing.** The k highest-scoring experts (k=1 in [[source-fedus-2021|Switch Transformer]], k=2 in Mixtral, k=8 in DeepSeek-MoE) are selected.
3. **Expert computation.** Each chosen expert is a feedforward network. The token's representation passes through the chosen experts in parallel.
4. **Combination.** The expert outputs are combined as a weighted sum using the (renormalized) gating scores.

Key practical concerns:

- **Load balancing.** Without intervention, gating collapses onto a few popular experts. An auxiliary loss penalizes imbalanced expert utilization, distributing tokens evenly.
- **Capacity factor.** Each expert has a fixed token budget per batch; overflow tokens are dropped or routed to a residual path. Trades quality for predictable batching.
- **Communication cost.** In distributed training/serving, experts live on different devices. All-to-all token shuffling dominates wall-clock time and is the central systems challenge.

## Recent Developments

- **[[source-shazeer-2017|Shazeer et al. 2017]]** introduced the modern sparsely-gated MoE in LSTMs.
- **[[source-fedus-2021|Switch Transformer (2021)]]** simplified to top-1 routing and scaled to 1.6T parameters.
- **GLaM (2022)** matched GPT-3 175B with 1.2T MoE while using ~1/3 the inference FLOPs.
- **Mixtral 8×7B (2024)** popularized open-weights MoE — top-2 routing over 8 experts per layer.
- **DeepSeek-V3 (2024)** scaled to 671B total / 37B active with fine-grained experts and shared-expert isolation; auxiliary-loss-free balancing.
- **Upcycling.** Convert a dense pretrained model into MoE by cloning its FFN into multiple experts and continuing training — reduces MoE training cost.

## Connections

MoE is the dominant lever for pushing past Chinchilla-optimal capacity at fixed inference cost: it changes the [[scaling-laws|scaling law]] curve along the active-vs-total-parameter axis. Together with [[flash-attention|FlashAttention]] (which attacks the attention cost) and quantization, it forms the modern frontier-efficiency stack.

The conceptual relative is the brain's sparse coding: only a small fraction of neurons fire for any given stimulus, and which fire is content-dependent. MoE's gating is a coarse-grained, learned analog.

For VaultMind, MoE is interesting as a metaphor for retrieval routing: a query selects which sub-collection of notes to attend to, rather than scanning the full vault uniformly. The same load-balancing concerns (popular notes dominate) apply.
