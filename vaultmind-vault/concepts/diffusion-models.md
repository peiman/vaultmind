---
id: concept-diffusion-models
type: concept
title: Diffusion Models
created: 2026-04-29
vm_updated: 2026-04-29
aliases:
  - DDPM
  - Denoising Diffusion
  - Score-Based Generative Models
tags:
  - deep-learning
  - generative-models
  - diffusion
related_ids:
  - concept-latent-diffusion
source_ids:
  - source-ho-2020
  - source-sohl-dickstein-2015
---

## Overview

Diffusion models are a family of generative models that learn to invert a gradual noising process. A forward process turns clean data into Gaussian noise over many steps; a learned reverse process turns noise back into clean data, one step at a time. The framework was introduced by [[source-sohl-dickstein-2015|Sohl-Dickstein et al. 2015]] under the name diffusion probabilistic models, and was made practical for high-quality image synthesis by [[source-ho-2020|Ho, Jain & Abbeel's DDPM (2020)]].

Diffusion models now dominate image, video, audio, and 3D generation. DALL-E 2/3, Imagen, Midjourney, Stable Diffusion, Sora, Veo, and most state-of-the-art text-to-image and text-to-video systems are diffusion-based. The framework has also been extended to language modeling (though autoregressive transformers still dominate text), molecule generation, and protein design.

## How It Works

**Forward process (fixed, no learning).** Given a clean sample x_0, define a Markov chain that adds Gaussian noise:

x_t = √(α_t) x_{t-1} + √(1 − α_t) ε,   ε ~ N(0, I)

After T steps with appropriate schedule, x_T is approximately pure Gaussian noise. The forward process at any timestep t can be written in closed form: x_t = √(ᾱ_t) x_0 + √(1 − ᾱ_t) ε.

**Reverse process (learned).** Train a neural network ε_θ(x_t, t) to predict the noise that was added. The DDPM loss simplifies to a weighted MSE:

L = E_{t, x_0, ε} [‖ε − ε_θ(x_t, t)‖²]

**Sampling.** Start from x_T ~ N(0, I). For t = T, T−1, ..., 1, denoise one step using the predicted noise plus a small Gaussian (DDPM) or deterministically (DDIM, by Song et al. 2020 — the same model can be sampled either way).

**Score-based view.** Equivalently, ε_θ implicitly learns the score ∇_x log p_t(x) of the noised data distribution at every level. Sampling is then Langevin/SDE integration along the score field. This unifies DDPM with score-matching generative modeling (Song & Ermon 2019).

**Conditioning.** Classifier-free guidance (Ho & Salimans 2021) interpolates between conditional and unconditional noise predictions, sharpening alignment with text/class conditions at the cost of diversity.

## Recent Developments

- **[[concept-latent-diffusion|Latent diffusion / Stable Diffusion (2022)]]** — run the diffusion in a compressed VAE latent space; an order of magnitude cheaper than pixel-space diffusion.
- **DiT (Diffusion Transformers, 2023)** — replace the U-Net denoiser with a transformer; the architecture behind Sora and Stable Diffusion 3.
- **Rectified Flow / Flow Matching (2022–23)** — reformulate the noise schedule as a straight-line ODE, enabling much faster (sometimes single-step) generation.
- **Consistency Models (Song et al. 2023)** — distill a multi-step diffusion model into a single-step generator.
- **Video diffusion (Sora, Veo, Stable Video Diffusion)** — extend to spatiotemporal generation by 3D U-Nets or video DiTs.
- **Discrete diffusion** — apply the framework to text, graphs, and other discrete domains; an open frontier for language modeling.

## Connections

Diffusion sits opposite the autoregressive [[concept-gpt|GPT]] paradigm in the generative-modeling design space: it generates the entire output in parallel via iterative refinement, rather than left-to-right one token at a time. Each paradigm dominates different modalities — autoregression for language, diffusion for continuous-valued media — though the boundary is blurring (image transformers, language diffusion).

[[concept-latent-diffusion|Latent diffusion]] is the most consequential descendant — it enabled consumer-grade text-to-image. [[concept-flash-attention|FlashAttention]] is part of the same systems-efficiency trend that lets diffusion transformers train at scale.

The cognitive analog is more speculative — generative models in the brain are an active research area (Hinton's Forward-Forward, predictive coding, generative replay during sleep), and diffusion-style iterative refinement has been proposed as a model of perceptual inference.
