---
id: source-kaplan-2020
type: source
title: "Kaplan et al. Scaling Laws for Neural Language Models (2020)"
created: 2026-04-29
vm_updated: 2026-04-29
url: "https://arxiv.org/abs/2001.08361"
tags:
  - llms
  - deep-learning
  - scaling
related_ids:
  - concept-scaling-laws
---

# Kaplan et al. — Scaling Laws for Neural Language Models (2020)

Kaplan, McCandlish, Henighan, Brown, Chess, Child, Gray, Radford, Wu, and Amodei (OpenAI) studied empirical scaling behavior of transformer language models on the cross-entropy loss as a function of three independent variables: parameter count N, dataset size D, and compute budget C. They found smooth power-law relationships spanning more than seven orders of magnitude, with architectural details (depth/width, attention heads) mattering far less than scale.

The paper's most consequential finding for the field was that larger models are dramatically more sample-efficient, and that for a fixed compute budget, optimal allocation favors very large models trained on relatively less data, stopping well before convergence. This prescription dominated frontier-model training for two years until [[scaling-laws|Chinchilla]] revised it.
