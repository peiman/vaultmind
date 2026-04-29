---
id: source-kingma-ba-2014
type: source
title: "Kingma, D. & Ba, J. Adam: A Method for Stochastic Optimization (2014)"
created: 2026-04-29
vm_updated: 2026-04-29
url: "https://arxiv.org/abs/1412.6980"
aliases:
  - Adam optimizer
  - Kingma Ba 2014
tags:
  - deep-learning
  - optimization
  - stochastic-optimization
related_ids:
  - concept-stochastic-gradient-descent
---

# Kingma & Ba — Adam (2014)

The Adam (Adaptive Moment Estimation) paper introduced what became the most widely used optimizer for training deep neural networks. Adam combines momentum (exponential moving average of the gradient) with RMSProp-style per-parameter learning-rate scaling (exponential moving average of the squared gradient). Bias-corrected estimates of these moments make Adam robust to initialization and largely insensitive to the global learning-rate setting in practice. Foundational reference for the [[stochastic-gradient-descent|SGD]] family of optimizers used to train modern neural networks.
