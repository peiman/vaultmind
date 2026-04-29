---
id: source-vaswani-2017
type: source
title: "Vaswani et al. Attention Is All You Need (2017)"
created: 2026-04-29
vm_updated: 2026-04-29
url: "https://arxiv.org/abs/1706.03762"
aliases:
  - Attention Is All You Need
  - Transformer paper
tags:
  - deep-learning
  - architectures
  - transformer
  - attention
related_ids:
  - concept-transformer
  - concept-self-attention
  - concept-attention-mechanism
---

# Vaswani et al. — Attention Is All You Need (2017)

Vaswani, Shazeer, Parmar, Uszkoreit, Jones, Gomez, Kaiser, and Polosukhin introduced the Transformer at NeurIPS 2017, replacing recurrence and convolution in sequence-to-sequence models with stacked self-attention and pointwise feedforward layers. The architecture established the encoder-decoder Transformer with multi-head scaled dot-product attention, sinusoidal positional encodings, residual connections, and layer normalization.

The Transformer outperformed prior state-of-the-art on WMT 2014 English-to-German and English-to-French translation while being substantially more parallelizable at training time. It became the foundation for BERT, GPT, T5, ViT, and effectively every large language model since.
