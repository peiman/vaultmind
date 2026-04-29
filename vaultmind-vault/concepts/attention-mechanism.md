---
id: concept-attention-mechanism
type: concept
title: Attention Mechanism
created: 2026-04-29
vm_updated: 2026-04-29
aliases:
  - Attention
  - Soft Attention
  - Bahdanau Attention
tags:
  - deep-learning
  - architectures
  - attention
  - sequence-models
related_ids:
  - concept-self-attention
  - concept-transformer
  - concept-recurrent-neural-network
  - concept-lstm
  - concept-gru
  - concept-spreading-activation
source_ids:
  - source-bahdanau-2014
  - source-vaswani-2017
---

## Overview

An attention mechanism is a differentiable lookup that lets a model produce a context vector as a weighted average over a set of memory items, where the weights are computed from a learned similarity between a query and each item's key. It generalizes "look at the most relevant input" into a soft, fully-differentiable operation that can be trained end-to-end.

Attention was introduced for neural machine translation by Bahdanau, Cho & Bengio (2014, "Neural Machine Translation by Jointly Learning to Align and Translate"). Their motivation: the [[lstm|LSTM]] encoder-decoder of Sutskever et al. (2014) compressed an entire source sentence into a single fixed-length vector, which crippled translation of long sentences. Attention let the decoder, at each output step, recompute a weighted summary of all encoder hidden states — effectively giving the model a learned alignment between source and target.

Luong et al. (2015) introduced multiplicative (dot-product) attention as a faster alternative to Bahdanau's additive form. Vaswani et al. (2017) made attention the sole computational primitive in the [[transformer|Transformer]].

## How It Works

The query/key/value framing (cleanest in retrospect from the Transformer paper, but applicable to all attention variants):

- **Query (Q)**: a vector representing what the model is currently looking for — e.g., the decoder's current hidden state.
- **Keys (K)**: vectors representing what each memory item is "about" — e.g., the encoder hidden states.
- **Values (V)**: vectors representing the content to be retrieved — often identical to K in early variants, distinct in the Transformer.

Computation:
1. Score each key against the query: e_i = score(Q, K_i).
2. Normalize with softmax: a_i = exp(e_i) / sum_j exp(e_j).
3. Compute the context vector as the weighted sum of values: c = sum_i a_i V_i.

Common scoring functions:
- **Additive (Bahdanau):** score(Q, K) = v^T tanh(W_q Q + W_k K). Higher capacity, slower.
- **Multiplicative (Luong):** score(Q, K) = Q^T K. Faster, becomes scaled dot-product attention in the Transformer with a 1/sqrt(d_k) factor.

## Key Properties

- **Differentiable lookup:** Soft (continuous) attention is end-to-end trainable; hard attention requires reinforcement learning or other tricks.
- **Variable-size memory:** Attention can summarize any number of memory items into a fixed-size context vector.
- **Learned alignment:** In NMT, attention weights often correspond to interpretable source-target word alignments without ever being supervised on alignments.
- **Content-addressable memory:** Like a soft, learned analogue of a dictionary lookup. The query addresses memory by similarity rather than by index.
- **Generalizes pooling:** Mean-pooling is uniform attention; max-pooling is hard one-hot attention. Learned attention weights interpolate.

## Connections

Attention's two consequential descendants are [[self-attention|self-attention]] (queries, keys, and values all come from the same sequence) and the [[transformer|Transformer]], which stacks self-attention layers and discards recurrence entirely.

Conceptually, attention has a kinship with [[spreading-activation|spreading activation]] in cognitive science: both compute a weighted distribution over a memory store based on the current cue, and both are content-addressable. Attention is differentiable and learned; spreading activation is rule-based and graph-structured. VaultMind's own retrieval pipeline can be read as a discrete approximation of attention — query embedding scored against note embeddings, top-k passed forward — which is essentially [[embedding-based-retrieval|embedding-based retrieval]] viewed through the attention lens.

Bahdanau attention is still useful in practice: streaming RNN decoders (e.g., on-device speech recognition), small-data sequence tasks, and any setting where O(n^2) self-attention is too expensive. But for any large-scale modern model, "attention" almost always means multi-head [[self-attention|self-attention]] inside a [[transformer|Transformer]].
