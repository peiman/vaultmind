---
id: concept-recurrent-neural-network
type: concept
title: Recurrent Neural Network
created: 2026-04-29
aliases:
  - RNN
  - Vanilla RNN
  - Elman Network
tags:
  - deep-learning
  - architectures
  - rnn
  - sequence-models
related_ids:
  - concept-lstm
  - concept-gru
  - concept-attention-mechanism
  - concept-transformer
  - concept-backpropagation
source_ids:
  - source-bahdanau-2014
  - source-cho-2014
---

## Overview

A recurrent neural network (RNN) processes a sequence x_1, x_2, ..., x_T one element at a time while maintaining a hidden state h_t that summarizes everything seen so far. The same parameters are reused at every time step, so an RNN can in principle handle sequences of arbitrary length.

The canonical update is h_t = f(W_x x_t + W_h h_{t-1} + b), where f is a nonlinearity (typically tanh). Outputs y_t can be produced at every step (sequence labeling), at the final step (classification), or by a separate decoder (sequence-to-sequence). Elman's "simple recurrent network" (1990) is the textbook form; Jordan networks (1986) and Hopfield networks (1982) are earlier relatives.

## Key Properties

- **Parameter sharing across time:** The same weights apply at every step, analogous to how a [[convolutional-neural-network|CNN]] shares filters across space.
- **Hidden state as memory:** h_t is a fixed-size vector that must compress all relevant history. This is the architectural bottleneck — and the source of the long-range dependency problem.
- **Trained with backpropagation through time (BPTT):** [[backpropagation|Backpropagation]] is unrolled across T time steps, producing gradients that depend on products of T Jacobians.
- **Vanishing and exploding gradients:** Hochreiter (1991) and Bengio et al. (1994) proved that for vanilla RNNs, gradients either decay to zero or blow up exponentially with T, making it nearly impossible to learn dependencies beyond ~10-20 steps. Gradient clipping mitigates explosion; vanishing requires architectural fixes.
- **Sequential, hard to parallelize:** Computing h_t requires h_{t-1}, so training cannot parallelize across the time dimension. This was the practical pain point that [[transformer|Transformers]] resolved.

## How It Works

For each step t in 1..T:
1. Read input x_t and previous hidden state h_{t-1}.
2. Compute new hidden state h_t = tanh(W_x x_t + W_h h_{t-1} + b).
3. Optionally emit output y_t = softmax(W_y h_t).

Training: unroll the computation graph for the full sequence, compute the loss, backpropagate through the unrolled graph (BPTT). For long sequences, truncated BPTT processes the sequence in chunks.

## Connections

The vanishing gradient problem motivated [[lstm|LSTM]] (Hochreiter & Schmidhuber 1997) and later [[gru|GRU]] (Cho et al. 2014), both of which use gating to maintain near-constant gradient flow through a protected cell state. Both gated variants dominated sequence modeling from the late 2000s through the mid-2010s.

The encoder bottleneck — compressing an entire source sentence into a single final hidden state — motivated the [[attention-mechanism|attention mechanism]] of Bahdanau et al. (2014), which let the decoder attend over the full sequence of encoder states. This in turn led to the [[transformer|Transformer]] (Vaswani et al. 2017), which removes recurrence entirely and uses [[self-attention|self-attention]] over the whole sequence in parallel. RNNs are still competitive baselines for short sequences, online streaming inference, and very long sequences where Transformer quadratic attention is prohibitive (cf. [[infini-attention|Infini-attention]], [[ring-attention|Ring Attention]]).
