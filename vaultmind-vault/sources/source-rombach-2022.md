---
id: source-rombach-2022
type: source
title: "Rombach et al. High-Resolution Image Synthesis with Latent Diffusion Models (Stable Diffusion, 2022)"
created: 2026-04-29
url: "https://arxiv.org/abs/2112.10752"
tags:
  - deep-learning
  - generative-models
  - diffusion
related_ids:
  - concept-latent-diffusion
  - concept-diffusion-models
---

# Rombach et al. — Latent Diffusion / Stable Diffusion (2022)

Rombach, Blattmann, Lorenz, Esser, and Ommer (CompVis, LMU Munich) showed that running diffusion in the latent space of a pre-trained VAE — instead of pixel space — gives an order-of-magnitude reduction in compute and memory while preserving sample quality. The architecture also introduced cross-attention conditioning, allowing the same latent denoiser to be conditioned on text, layout, semantic maps, or other modalities.

This paper is the architecture behind Stable Diffusion, the first widely-released open-weights text-to-image model, which transformed the generative AI landscape in 2022. Latent-space diffusion plus cross-attention conditioning is now the standard recipe for text-to-image, text-to-video, and most multimodal diffusion systems.
