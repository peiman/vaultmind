---
id: concept-mamba-state-space-models
type: concept
title: Mamba and State Space Models
created: 2026-04-29
vm_updated: 2026-04-29
aliases:
  - Mamba
  - SSM
  - Selective State Space Models
  - S4
tags:
  - llms
  - deep-learning
  - state-space-models
  - architecture
  - long-context
related_ids:
  - concept-transformer
  - concept-flash-attention
  - concept-attention-mechanism
source_ids:
  - source-gu-dao-2023
  - source-gu-2021
---

## Overview

State Space Models (SSMs) are a family of sequence-modeling architectures whose core operation is a linear recurrent system: a hidden state h_t is updated from h_{t-1} and the input x_t via a fixed linear map, and the output y_t is a linear function of h_t. Unlike attention's quadratic cost in sequence length, SSMs run in linear time and constant memory per token at inference. The lineage runs from classical control theory through HiPPO and [[source-gu-2021|S4 (Gu et al. 2021)]] to [[source-gu-dao-2023|Mamba (Gu & Dao 2023)]], the first SSM to credibly compete with transformers on language.

The central design challenge for SSMs has been giving them content-dependent control over their state. Linear time-invariant systems (S4, S5, H3) compress history with a fixed scheme; they can't selectively attend to or ignore tokens. Mamba's "selective" mechanism makes the state-update matrices input-dependent, recovering this capability while preserving the linear-time recurrence.

## How It Works

A continuous-time SSM is x'(t) = A x(t) + B u(t); y(t) = C x(t). Discretized:

h_t = Ā h_{t-1} + B̄ u_t
y_t = C h_t

In **S4**, A is given a structured (HiPPO) parameterization that provably compresses input history under a chosen measure. The whole thing can be unrolled as a convolution over the sequence with a learnable kernel and computed with O(N log N) FFTs during training, while serving as O(1)-per-step recurrence at inference.

In **Mamba**, the parameters Ā, B̄, C themselves depend on the current input — they are produced by small linear projections of x_t. This breaks the convolution view (no longer time-invariant) but Mamba introduces a parallel scan algorithm that runs the input-dependent recurrence efficiently on GPU. Selective SSMs can now do the things attention does naturally: focus on a specific past token, ignore noise, retain a name across thousands of tokens.

Mamba blocks combine selective SSMs with gated MLPs and skip the MLP block of standard transformers — the architecture is "attention-free" in the strict sense.

## Recent Developments

- **Mamba-2 (2024)** — unifies SSMs and a restricted form of attention via the structured-state-space duality (SSD) framework; faster training, larger state.
- **Jamba (2024, AI21)** — hybrid Mamba/Transformer/MoE architecture; first production-scale hybrid.
- **Zamba, Samba, Hymba** — various Mamba/Transformer hybrids exploring the design space.
- **Vision Mamba, VideoMamba** — extensions to image and video.
- **Mamba in long-context tasks** — strong performance up to ~1M tokens at far lower memory than attention; weaker on dense in-context recall (the "needle in a haystack" gap).

## Connections

Mamba/SSMs are the most credible architectural alternative to the [[concept-transformer|transformer]]. Both attack long-context cost from different angles: SSMs do so structurally (linear-time recurrence), while [[concept-flash-attention|FlashAttention]] does so systems-side (IO-aware exact attention). Hybrid designs combine both — using attention for fine-grained recall and SSMs for coarse compressed memory.

The cognitive analog to selective SSMs is content-addressable working memory: a fixed-capacity store that compresses some signals and amplifies others based on relevance. The S4 HiPPO derivation has the flavor of an optimal forgetting curve under a chosen weighting, which connects loosely to [[concept-forgetting-curve|Ebbinghaus]]-style memory decay.

For VaultMind, the conceptual transfer is bounded-memory summarization of session history: keep a fixed-size compressed state of past notes accessed, updated content-dependently, alongside the explicit retrieval index.
