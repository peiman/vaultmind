---
id: source-dosovitskiy-2020
type: source
title: "Dosovitskiy et al. An Image is Worth 16x16 Words: Transformers for Image Recognition at Scale (2020)"
created: 2026-04-29
vm_updated: 2026-04-29
url: "https://arxiv.org/abs/2010.11929"
aliases:
  - ViT paper
  - Vision Transformer
tags:
  - deep-learning
  - architectures
  - transformer
  - computer-vision
related_ids:
  - concept-vision-transformer
  - concept-transformer
---

# Dosovitskiy et al. — Vision Transformer (2020)

Published at ICLR 2021 (arXiv October 2020), the Vision Transformer (ViT) showed that a plain [[transformer|Transformer]] encoder applied directly to sequences of flattened image patches can match or exceed convolutional networks at image classification — given sufficient pre-training data.

ViT splits an image into fixed-size non-overlapping patches (e.g., 16x16), linearly embeds each patch, prepends a learnable [class] token, adds learned positional embeddings, and feeds the sequence into a standard Transformer encoder. The classification head reads from the [class] token. Pre-trained on JFT-300M, ViT outperformed strong CNN baselines (BiT, EfficientNet) on ImageNet, CIFAR, and VTAB, definitively breaking the CNN monopoly on computer vision.
