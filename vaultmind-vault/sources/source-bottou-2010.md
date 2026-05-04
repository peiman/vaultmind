---
id: source-bottou-2010
type: source
title: "Bottou, L. Large-Scale Machine Learning with Stochastic Gradient Descent (2010)"
created: 2026-04-29
url: "https://doi.org/10.1007/978-3-7908-2604-3_16"
aliases:
  - Bottou 2010
tags:
  - deep-learning
  - optimization
  - stochastic-optimization
related_ids:
  - concept-stochastic-gradient-descent
---

# Bottou — Large-Scale Machine Learning with SGD (2010)

Léon Bottou's 2010 paper in the *Proceedings of COMPSTAT'2010* makes the case that on large-scale problems, [[stochastic-gradient-descent|stochastic gradient descent]] is asymptotically more efficient than batch gradient descent in terms of total training time to a given test error, because the per-iteration cost dominates and SGD makes faster wall-clock progress per data point. A canonical reference for why SGD scales better than full-batch [[gradient-descent|gradient descent]].
