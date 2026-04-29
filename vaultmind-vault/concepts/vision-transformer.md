---
id: concept-vision-transformer
type: concept
title: Vision Transformer
created: 2026-04-29
vm_updated: 2026-04-29
aliases:
  - ViT
  - Vision Transformer (ViT)
tags:
  - deep-learning
  - architectures
  - transformer
  - computer-vision
related_ids:
  - concept-transformer
  - concept-self-attention
  - concept-convolutional-neural-network
  - concept-attention-mechanism
source_ids:
  - source-dosovitskiy-2020
  - source-vaswani-2017
---

## Overview

The Vision Transformer (ViT), introduced by Dosovitskiy et al. ("An Image is Worth 16x16 Words", ICLR 2021, arXiv October 2020), applies a standard [[transformer|Transformer]] encoder — almost unmodified from Vaswani et al. (2017) — to image classification. The trick is to treat an image as a sequence of fixed-size patches, where each patch plays the role of a "token."

Before ViT, [[convolutional-neural-network|CNNs]] dominated computer vision. ViT showed that the convolutional inductive bias (locality, translation equivariance) is not necessary if you have enough pre-training data: with JFT-300M pre-training, a plain Transformer encoder matched or exceeded strong CNN baselines (BiT, EfficientNet) on ImageNet, CIFAR-100, and the VTAB benchmark, while being more compute-efficient at scale.

## How It Works

Pipeline for an H × W × C image:

1. **Patch embedding.** Split the image into N = (H × W) / P^2 non-overlapping patches of size P × P × C (e.g., P=16, giving 14×14=196 patches for 224×224 inputs). Flatten each patch and project linearly to a d-dimensional vector.
2. **Class token + positional embeddings.** Prepend a learnable [class] token to the patch sequence (echoing BERT's [CLS]). Add learned 1D positional embeddings to all positions.
3. **Transformer encoder.** Apply L layers of multi-head [[self-attention|self-attention]] + feedforward, with residual connections and layer norm — exactly the encoder side of a standard Transformer.
4. **Classification head.** Read out the final-layer [class] token vector and pass it through an MLP to produce class logits.

ViT variants in the paper: ViT-Base (12 layers, 86M params), ViT-Large (24 layers, 307M params), ViT-Huge (32 layers, 632M params). All trained at scale and fine-tuned on downstream tasks.

## Key Properties

- **Pure attention over patches:** No convolutions. Every patch attends to every other patch from layer 1, giving global receptive field immediately rather than building it up through depth.
- **Data-hungry:** With ImageNet-1k alone (1.3M images), ViT underperforms ResNet baselines. With JFT-300M (300M images) or strong augmentation/regularization (DeiT, Touvron et al. 2021), it matches or exceeds CNNs.
- **Weak inductive bias:** Lacks the locality and translation-equivariance priors of CNNs. This is why it needs more data — but also why it scales better when data is plentiful.
- **Patches are tokens:** A 16×16 patch becomes a single embedding. Sequence length is determined by image resolution and patch size; doubling resolution at fixed patch size quadruples sequence length and quadratically increases attention cost.
- **Positional embeddings are learned and 1D:** Despite the 2D structure, ViT uses flat 1D positional embeddings — the model learns 2D structure from data. Later variants use 2D-factorized or relative position encodings.
- **Pre-train then fine-tune:** Standard recipe is large-scale pre-training (supervised on JFT, or self-supervised via DINO/MAE/SimCLR) followed by task-specific fine-tuning.

## Connections

ViT is the direct application of the [[transformer|Transformer]] encoder to vision. It validates the broader thesis that [[self-attention|self-attention]] is a general-purpose primitive that can replace task-specific inductive biases when data is abundant.

Compared to [[convolutional-neural-network|CNNs]], ViT trades sample efficiency for scaling efficiency: CNNs reach competitive accuracy with less data; ViTs reach higher peak accuracy with more data and compute. Hybrid architectures (Swin Transformer, ConvNeXt) reintroduce locality and hierarchy while keeping attention.

ViT also enabled multimodal models. CLIP (Radford et al. 2021), DINOv2, SAM, and most modern vision-language models use ViT-style image encoders. The key abstraction — "split inputs into patches, treat them as tokens, run a Transformer" — generalizes to audio (AST), video (ViViT, TimeSformer), and point clouds (Point Transformer).

For VaultMind, ViT is not a current dependency (the vault is text), but the architectural lesson — that a single attention-based primitive can absorb tasks formerly requiring bespoke architectures — informs why we standardize on Transformer encoders for the embedding layer rather than maintaining task-specific encoders per content type.
