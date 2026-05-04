---
id: source-sohl-dickstein-2015
type: source
title: "Sohl-Dickstein et al. Deep Unsupervised Learning using Nonequilibrium Thermodynamics (2015)"
created: 2026-04-29
url: "https://arxiv.org/abs/1503.03585"
tags:
  - deep-learning
  - generative-models
  - diffusion
related_ids:
  - concept-diffusion-models
---

# Sohl-Dickstein et al. — Diffusion Probabilistic Models (2015)

Sohl-Dickstein, Weiss, Maheswaranathan, and Ganguli introduced the original diffusion-model formulation: a forward Markov chain that gradually corrupts data with Gaussian noise toward an analytically tractable prior, paired with a learned reverse Markov chain that denoises step by step. They drew the construction from nonequilibrium thermodynamics — the forward process is a discretized diffusion in data space, and the reverse process learns to invert it.

Although it predated the era of large-scale image synthesis and was largely overlooked for five years, this paper contains the core mathematical scaffolding (variational lower bound, forward/reverse process duality) that [[source-ho-2020|Ho et al. 2020]] would operationalize and that the entire modern diffusion stack rests on.
