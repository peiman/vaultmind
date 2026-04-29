---
id: source-hendrycks-2016
type: source
title: "Hendrycks, D. & Gimpel, K. Gaussian Error Linear Units (GELUs) (2016)"
created: 2026-04-29
vm_updated: 2026-04-29
url: "https://arxiv.org/abs/1606.08415"
aliases:
  - GELU paper
  - Hendrycks Gimpel 2016
tags:
  - deep-learning
  - activation-functions
related_ids:
  - concept-activation-function
---

# Hendrycks & Gimpel — GELU (2016)

Hendrycks and Gimpel introduced the Gaussian Error Linear Unit, GELU(x) = x · Φ(x), a smooth activation that gates the input by its own value under a standard Gaussian CDF. GELU subsequently became the default [[activation-function|activation]] in BERT, GPT, and most modern transformer architectures because of its smooth gradient profile and empirical performance.
