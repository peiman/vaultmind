---
id: concept-self-attention
type: concept
title: Self-Attention
created: 2026-04-29
vm_updated: 2026-04-29
aliases:
  - Multi-Head Self-Attention
  - Scaled Dot-Product Attention
  - MHSA
tags:
  - deep-learning
  - architectures
  - transformer
  - attention
related_ids:
  - concept-transformer
  - concept-attention-mechanism
  - concept-vision-transformer
  - concept-colbert
  - concept-infini-attention
  - concept-ring-attention
source_ids:
  - source-vaswani-2017
---

## Overview

Self-attention is the [[attention-mechanism|attention]] variant in which queries, keys, and values are all derived from the same input sequence. Each position attends to every position (including itself), producing a new representation that mixes information from the entire sequence. Stacking self-attention layers — together with pointwise feedforward networks, residual connections, and layer norm — gives a [[transformer|Transformer]] block.

Vaswani et al. (2017) introduced the specific form used in modern models: **scaled dot-product attention** with **multi-head** parallelism and **positional encodings**. Self-attention provides every pair of positions with an O(1) connection path, which is the architectural reason Transformers handle long-range dependencies better than [[recurrent-neural-network|RNNs]] or [[lstm|LSTMs]] do.

## How It Works

Given an input sequence of n d-dimensional vectors, stacked into a matrix X ∈ R^{n×d}:

**Project to queries, keys, values:**
- Q = X W_Q, K = X W_K, V = X W_V, where W_Q, W_K, W_V ∈ R^{d×d_k} are learned.

**Scaled dot-product attention:**
- Attention(Q, K, V) = softmax(Q K^T / sqrt(d_k)) V.
- The scaling factor sqrt(d_k) keeps the softmax in a regime where gradients don't vanish for large d_k.

**Multi-head attention:**
- Run h parallel attention heads, each with its own (W_Q^i, W_K^i, W_V^i) projecting to a smaller d_k = d/h.
- Concatenate the h head outputs and project through W_O ∈ R^{d×d}.
- Different heads learn different attention patterns — syntactic dependencies, coreference, positional offsets, etc.

**Positional encoding:**
- Self-attention is permutation-equivariant; without positional information it cannot distinguish "dog bites man" from "man bites dog."
- Vaswani et al. used sinusoidal encodings added to token embeddings; modern alternatives include learned absolute embeddings, RoPE (rotary position embedding), and ALiBi (linear bias on attention scores).

**Masking:**
- Causal (autoregressive) self-attention masks the upper triangle of the attention matrix so position t cannot attend to t' > t. This is what makes decoder-only Transformers (GPT-style) generative.
- Padding masks zero out attention to padding tokens in batched variable-length inputs.

## Key Properties

- **Quadratic in sequence length:** Compute and memory are O(n^2 d). For n=8K this is 64M attention scores per head per layer; for n=1M it is intractable without specialized algorithms.
- **O(1) path length:** Any two positions are one attention hop apart, regardless of distance. Compare to RNNs (O(n)) and stacked CNNs (O(log n) with dilated convs).
- **Parallel across positions:** Unlike RNNs, self-attention layers can process all n positions simultaneously on a GPU.
- **Permutation equivariant:** Without positional encoding, the output is invariant to permutations of the input. This is a feature for set-structured inputs and a bug for sequences.
- **Multi-head attention is not redundant:** Reducing per-head dimensionality and having more heads tends to outperform a single full-dimensional head.
- **Memory-bound at inference:** For autoregressive decoding, the bottleneck is reading the [[kv-cache|KV cache]] from HBM, not arithmetic — which is why FlashAttention and KV-cache compression matter.

## Connections

Self-attention is the computational core of every [[transformer|Transformer]]-based model: BERT (encoder-only), GPT (decoder-only with causal mask), T5 (encoder-decoder), and [[vision-transformer|ViT]] (patches as tokens). It generalizes the [[attention-mechanism|attention mechanism]] of Bahdanau et al. by removing the encoder/decoder distinction and letting a sequence attend to itself.

The quadratic cost is the central engineering pain point for long-context models. [[infini-attention|Infini-attention]] adds a compressed external memory; [[ring-attention|Ring Attention]] partitions the sequence across devices; FlashAttention restructures the IO pattern; sparse, linear, and state-space alternatives (Performer, Mamba) trade exact attention for asymptotic speedups.

[[colbert|ColBERT]] exploits Transformer self-attention contextualized token embeddings for late-interaction retrieval, leveraging that every token vector summarizes its sequence context via self-attention. For VaultMind, BGE-M3 (the production embedder) is a Transformer encoder stack — the embedding is a pooled summary of self-attended note tokens.
