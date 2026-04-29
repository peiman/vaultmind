---
id: source-hoffmann-2022
type: source
title: "Hoffmann et al. Training Compute-Optimal Large Language Models (2022)"
created: 2026-04-29
vm_updated: 2026-04-29
url: "https://arxiv.org/abs/2203.15556"
tags:
  - llms
  - deep-learning
  - scaling
related_ids:
  - concept-scaling-laws
---

# Hoffmann et al. — Chinchilla / Training Compute-Optimal LLMs (2022)

Hoffmann and colleagues at DeepMind trained over 400 transformer language models spanning 70M to 16B parameters on 5B to 500B tokens to revisit the [[source-kaplan-2020|Kaplan et al. 2020]] scaling prescription. Their finding overturned the prior consensus: for a fixed compute budget, model size and training tokens should scale in roughly equal proportion (≈20 tokens per parameter), not size-heavy as Kaplan recommended.

Their compute-optimal 70B-parameter model "Chinchilla", trained on 1.4T tokens, outperformed the 280B Gopher, 175B GPT-3, and 530B Megatron-Turing NLG across nearly all evaluated benchmarks while using the same training compute as Gopher. The paper redirected frontier training toward training-token-rich regimes and is the proximate cause of the LLaMA family and most subsequent open-weights models being trained on 1T+ tokens.
