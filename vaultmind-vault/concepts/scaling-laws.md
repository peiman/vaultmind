---
id: concept-scaling-laws
type: concept
title: Scaling Laws
created: 2026-04-29
vm_updated: 2026-04-29
aliases:
  - Neural Scaling Laws
  - Chinchilla Scaling
tags:
  - llms
  - deep-learning
  - scaling
related_ids:
  - concept-mixture-of-experts
  - concept-gpt
  - concept-flash-attention
source_ids:
  - source-kaplan-2020
  - source-hoffmann-2022
---

## Overview

Scaling laws describe the empirical regularity that the cross-entropy loss of a neural language model decreases as a smooth power law in three quantities: the number of parameters N, the number of training tokens D, and the compute budget C. The relationship spans many orders of magnitude and is remarkably architecture-insensitive — depth, width, and head count matter far less than scale.

The two foundational papers, [[source-kaplan-2020|Kaplan et al. 2020]] and [[source-hoffmann-2022|Hoffmann et al. 2022]] (Chinchilla), prescribe how to allocate a fixed compute budget between model size and training tokens. They reach different conclusions — and the disagreement structured two distinct eras of frontier-model training.

## Key Mechanism

For dense transformers trained with standard recipes, the loss is well-fit by:

L(N, D) ≈ E + A/N^α + B/D^β

where E is the irreducible loss (data entropy), and α, β are empirically estimated exponents. Compute is roughly C ≈ 6 N D for a forward+backward pass, so a fixed C constrains feasible (N, D) pairs along a hyperbola.

- **Kaplan 2020 prescription:** When extrapolating across compute scales, parameters should grow much faster than tokens (roughly N ∝ C^0.73, D ∝ C^0.27). At a fixed compute budget, train very large models on relatively little data and stop well before convergence.
- **Chinchilla 2022 prescription:** Re-running the experiment carefully, with hundreds of fully-trained models, gives N ∝ C^0.5 and D ∝ C^0.5 — model size and tokens should scale equally, at roughly 20 tokens per parameter. Chinchilla 70B (1.4T tokens) outperformed the 280B Gopher trained on the same compute.

The discrepancy was traced to differences in learning-rate schedules: Kaplan's runs used a fixed schedule designed for one model size, which under-trained the larger models relative to optimal.

## Recent Developments

- **Inference-aware scaling:** Once inference cost dominates total lifetime compute (frontier deployment), it is rational to over-train a smaller model far past compute-optimal — this is the LLaMA-3 / Mistral / Qwen recipe of training 7B–70B models on 10–15T tokens.
- **MoE scaling laws:** [[concept-mixture-of-experts|Sparse-expert]] models follow distinct scaling curves with respect to active vs. total parameters; both quantities matter and the relationship is still an open research question.
- **Inference-time-compute scaling:** A new axis appeared with o1-style models — quality scales with thinking-token count at fixed parameter count, suggesting reasoning is a separable scaling dimension.
- **Data limits:** With ~15T tokens of high-quality web text already in use, "data wall" concerns have driven interest in synthetic data, repetition, and multimodal scaling.

## Connections

Scaling laws are the empirical scaffolding behind the entire modern LLM enterprise. They explain why [[concept-gpt|GPT]]-lineage labs invest in massive compute, why [[concept-mixture-of-experts|MoE]] is attractive (it changes the active-parameter side of the curve), and why [[concept-flash-attention|FlashAttention]] matters (it lowers the constant in C ≈ 6ND, effectively shifting the budget).

For VaultMind, the analog is a "retrieval scaling law" question: how does answer quality scale with vault size, retrieval budget, and re-ranking compute? The framework is portable even though the constants differ.
