---
id: source-fedus-2021
type: source
title: "Fedus et al. Switch Transformers: Scaling to Trillion Parameter Models with Simple and Efficient Sparsity (2021)"
created: 2026-04-29
vm_updated: 2026-04-29
url: "https://arxiv.org/abs/2101.03961"
tags:
  - llms
  - deep-learning
  - mixture-of-experts
related_ids:
  - concept-mixture-of-experts
---

# Fedus et al. — Switch Transformer (2021)

Fedus, Zoph, and Shazeer (Google Brain) simplified the [[source-shazeer-2017|Shazeer 2017]] MoE design by routing each token to a single expert (top-1) instead of top-2, eliminating the second-expert weighted combination. This "switch" routing reduced compute and communication overhead while training stably at trillion-parameter scale.

The paper introduced practical innovations that became standard: selective precision (bfloat16 inside experts, float32 in routing), capacity factors that bound expert load, and load-balancing losses that prevent collapse. Switch-C reached 1.6 trillion parameters and outperformed dense T5-XXL on pre-training perplexity per FLOP. The work re-popularized MoE in the modern transformer era and underpins much of the current sparse-expert lineage.
