---
id: concept-transformer
type: concept
title: Transformer
created: 2026-04-29
vm_updated: 2026-04-29
aliases:
  - Transformer Architecture
  - Encoder-Decoder Transformer
tags:
  - deep-learning
  - architectures
  - transformer
  - attention
  - sequence-models
related_ids:
  - concept-self-attention
  - concept-attention-mechanism
  - concept-vision-transformer
  - concept-recurrent-neural-network
  - concept-lstm
  - concept-colbert
  - concept-memorizing-transformers
  - concept-infini-attention
  - concept-embedding-based-retrieval
source_ids:
  - source-vaswani-2017
  - source-bahdanau-2014
---

## Overview

The Transformer, introduced by Vaswani et al. in "Attention Is All You Need" (NeurIPS 2017), is a sequence-modeling architecture built entirely from [[self-attention|self-attention]] layers and pointwise feedforward networks — no recurrence, no convolution. It was originally proposed for machine translation as an encoder-decoder model but has since become the substrate of essentially every modern large language model (BERT, GPT, T5, LLaMA, Claude, etc.) and, via [[vision-transformer|ViT]], of state-of-the-art computer vision.

The architectural shift is fundamental. [[recurrent-neural-network|RNNs]] and [[lstm|LSTMs]] process sequences step by step, which is sequential at training time and bottlenecks long-range information through a fixed-size hidden state. Transformers process all positions in parallel and let any position attend directly to any other, with O(1) path length between any two tokens. The cost is O(n^2) attention compute and memory in the sequence length n — the engineering pressure point that motivates [[infini-attention|Infini-attention]], [[ring-attention|Ring Attention]], FlashAttention, and sparse/linear attention variants.

## How It Works

The original encoder-decoder Transformer stacks N=6 identical encoder layers and N=6 decoder layers.

**Encoder layer** — each contains:
1. Multi-head [[self-attention|self-attention]] over the input sequence.
2. Position-wise feedforward network (two linear layers with ReLU/GELU).
3. Residual connections and layer normalization around each sublayer.

**Decoder layer** — each contains:
1. Masked multi-head self-attention (causal mask prevents attending to future positions).
2. Multi-head cross-attention over the encoder output (queries from decoder, keys/values from encoder).
3. Position-wise feedforward.
4. Residuals and layer norm.

**Inputs**: token embeddings + positional encodings (sinusoidal in the original; learned, RoPE, or ALiBi in modern variants). The model has no built-in notion of order, so positions must be encoded explicitly.

**Outputs**: linear projection + softmax over the vocabulary at each decoder position.

Training uses teacher forcing with cross-entropy over the next-token distribution. Inference is autoregressive: generate one token, append it, run the decoder again. KV caching ([[kv-cache|KV cache]]) avoids recomputing keys and values for past tokens.

## Key Properties

- **Pure attention, no recurrence:** Eliminates the sequential training bottleneck of RNNs/LSTMs; whole sequences are processed in parallel.
- **O(1) path length between any two positions:** Long-range dependencies are reachable in a single attention hop, rather than having to propagate through n steps.
- **Quadratic attention cost:** Compute and memory scale as O(n^2 d), which is the central engineering challenge for long contexts.
- **Multi-head attention:** Multiple parallel attention heads let the model attend to different subspaces simultaneously — a single head with the same total dimensionality is empirically weaker.
- **Positional encoding is bolted on:** Unlike CNNs (locality) or RNNs (order), the Transformer has no spatial inductive bias; position information must be supplied explicitly.
- **Encoder-only, decoder-only, encoder-decoder:** Three families. Encoder-only (BERT) for understanding. Decoder-only (GPT) for autoregressive generation. Encoder-decoder (T5, original Transformer) for seq2seq. Decoder-only has come to dominate large-scale generative LLMs.
- **Scales gracefully:** Larger Transformers, more data, and more compute predictably improve performance — the empirical scaling laws (Kaplan, Hoffmann/Chinchilla) that drive modern foundation-model training were observed on Transformers.

## Connections

The Transformer is the direct heir of the [[attention-mechanism|attention mechanism]] of Bahdanau et al. (2014), which augmented an [[lstm|LSTM]] seq2seq model with attention over encoder states. Vaswani et al.'s contribution was showing that attention alone — without the LSTM — was sufficient and superior. [[self-attention|Self-attention]] generalizes Bahdanau attention by letting a sequence attend to itself.

Transformers underpin [[vision-transformer|ViT]] (Dosovitskiy et al. 2020), which applies the encoder to image patches; [[colbert|ColBERT]], which uses BERT-derived contextualized token embeddings for late-interaction retrieval; [[embedding-based-retrieval|embedding-based retrieval]] generally; and memory-augmented variants like [[memorizing-transformers|Memorizing Transformers]] and [[infini-attention|Infini-attention]] that extend effective context length by attaching external KNN or compressed-memory mechanisms.

For VaultMind, the Transformer is load-bearing infrastructure: BGE-M3 embeddings, BERT-family rerankers, and any LLM consumer of the Context Pack are all Transformer-based. Understanding self-attention's quadratic cost explains why we chunk notes for indexing rather than feeding whole vaults into a single forward pass, and why long-context strategies (Infini-attention, Ring Attention, retrieval augmentation) are research-relevant.
