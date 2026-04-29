---
id: source-shazeer-2017
type: source
title: "Shazeer et al. Outrageously Large Neural Networks: The Sparsely-Gated Mixture-of-Experts Layer (2017)"
created: 2026-04-29
vm_updated: 2026-04-29
url: "https://arxiv.org/abs/1701.06538"
tags:
  - llms
  - deep-learning
  - mixture-of-experts
related_ids:
  - concept-mixture-of-experts
---

# Shazeer et al. — Sparsely-Gated Mixture-of-Experts (2017)

Shazeer, Mirhoseini, Maziarz, Davis, Le, Hinton, and Dean introduced the Sparsely-Gated Mixture-of-Experts (MoE) layer, a conditional-computation construct that routes each input token to a small subset (typically top-k, k=1 or 2) of a large bank of expert feedforward networks via a learned gating function. The paper demonstrated 137-billion-parameter language models on machine translation that trained faster and reached lower perplexity than dense baselines, despite each token activating only a fraction of the parameters.

The key contribution was making sparse expert routing tractable at scale: the noisy top-k gating, load-balancing auxiliary loss, and distributed expert placement all became standard ingredients of the modern MoE recipe used in Switch Transformer, GLaM, Mixtral, and DeepSeek-V3.
