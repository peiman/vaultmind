---
id: concept-latent-diffusion
type: concept
title: Latent Diffusion (Stable Diffusion)
created: 2026-04-29
vm_updated: 2026-04-29
aliases:
  - Stable Diffusion
  - LDM
  - Latent Diffusion Model
tags:
  - deep-learning
  - generative-models
  - diffusion
related_ids:
  - concept-diffusion-models
  - concept-attention-mechanism
source_ids:
  - source-rombach-2022
---

## Overview

Latent Diffusion Models (LDM), introduced by [[source-rombach-2022|Rombach et al. 2022]], run the diffusion process in the latent space of a pretrained autoencoder rather than directly in pixel space. The autoencoder compresses 512×512 RGB images (~786K dimensions) to a 64×64×4 latent (~16K dimensions), shrinking the diffusion model's input by ~50× and reducing compute and memory cost by an order of magnitude. The image-quality cost is small because perceptual content is preserved in the latent space.

Latent diffusion is the architecture behind Stable Diffusion — the first widely-released open-weights text-to-image model, which transformed the generative AI ecosystem in 2022 by making high-quality image generation runnable on consumer GPUs. The same architecture, scaled and refined, underpins Stable Diffusion 1.5/2/XL/3, many fine-tuned community variants, ControlNet, and the academic-and-startup explosion that followed.

## How It Works

LDM has three components trained in stages:

1. **Perceptual autoencoder.** A VAE with a perceptual loss (LPIPS) and adversarial loss is trained to compress images x → z (encoder E) and reconstruct z → x̂ (decoder D). The latent z is much smaller than x but preserves the perceptually-relevant content. This stage is trained once and frozen.

2. **Latent diffusion.** A standard [[diffusion-models|DDPM]] is trained on encoded latents E(x) instead of raw pixels. The denoising network is a U-Net (or, in newer variants, a transformer / DiT). Because the latent has 8× lower spatial resolution and far fewer channels than the image, the U-Net is dramatically smaller than a pixel-space equivalent at the same capacity-per-input.

3. **Cross-attention conditioning.** To condition on text (or class labels, or layout, etc.), the conditioning signal is encoded by a domain-specific encoder (CLIP text encoder for Stable Diffusion) and injected into every U-Net block via cross-[[attention-mechanism|attention]]. The U-Net's spatial features serve as queries; the text embeddings serve as keys and values. This is the architectural innovation that made flexible multimodal conditioning practical.

At inference, encode the prompt, sample latent noise z_T, run the conditional denoising loop to produce z_0, and decode D(z_0) to an image.

## Recent Developments

- **Stable Diffusion 1.5 (2022)** — the canonical open-weights version that drove community fine-tuning.
- **Stable Diffusion XL (2023)** — larger U-Net, two text encoders, refiner stage, native 1024×1024.
- **Stable Diffusion 3 (2024)** — replaces the U-Net with a Multimodal Diffusion Transformer (MMDiT) and uses rectified flow.
- **ControlNet (Zhang et al. 2023)** — adds conditional control over a frozen LDM by training a parallel encoder that injects spatial conditions (depth, pose, edges).
- **LoRA fine-tuning** — low-rank adaptation became the dominant way to personalize LDMs on small datasets.
- **Video LDMs (Stable Video Diffusion, Sora-class systems)** — extend the latent-diffusion recipe to spatiotemporal latents.
- **Distillation and few-step LDMs** — SDXL Turbo, LCM, SD3 Turbo distill the multi-step process into 1–4 steps for real-time generation.

## Connections

Latent diffusion is the most economically consequential descendant of [[diffusion-models|DDPM]]. The pixel-space → latent-space move is the same cost-vs-quality tradeoff that motivated [[mixture-of-experts|MoE]] (capacity vs. active compute) and tokenizers in language models (sequence length vs. vocabulary). Compress where you can, then learn over the compressed representation.

The cross-attention conditioning is the conceptual bridge from unimodal generation to text-guided multimodal generation. The idea generalizes: any conditioning signal that can be encoded into a sequence of vectors can be plugged into the same architecture. Newer DiT-based systems (SD3, Sora) replace the U-Net with a transformer that treats both image latents and text tokens as tokens in a single sequence — a further convergence with the [[gpt|transformer language model]] paradigm.
