---
id: source-ho-2020
type: source
title: "Ho, Jain & Abbeel. Denoising Diffusion Probabilistic Models (DDPM, 2020)"
created: 2026-04-29
vm_updated: 2026-04-29
url: "https://arxiv.org/abs/2006.11239"
tags:
  - deep-learning
  - generative-models
  - diffusion
related_ids:
  - concept-diffusion-models
---

# Ho, Jain & Abbeel — DDPM (2020)

Ho, Jain, and Abbeel (UC Berkeley) made diffusion models work as a competitive image generator. They reformulated the denoising objective as a simple weighted MSE on predicted noise (the ε-prediction parameterization), connected the framework rigorously to score matching and Langevin dynamics, and produced FID scores on CIFAR-10 and LSUN comparable to the best GANs of the time.

This paper is the practical foundation of the modern diffusion era. Almost every subsequent text-to-image and video diffusion system — GLIDE, DALL-E 2, [[concept-latent-diffusion|Stable Diffusion]], Imagen, Sora — inherits the ε-prediction objective and the U-Net denoiser pattern that DDPM established.
