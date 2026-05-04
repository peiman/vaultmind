---
id: concept-gru
type: concept
title: Gated Recurrent Unit
created: 2026-04-29
aliases:
  - GRU
  - Gated Recurrent Unit
tags:
  - deep-learning
  - architectures
  - rnn
  - gru
  - sequence-models
related_ids:
  - concept-lstm
  - concept-recurrent-neural-network
  - concept-attention-mechanism
  - concept-transformer
source_ids:
  - source-cho-2014
---

## Overview

The Gated Recurrent Unit (GRU), introduced by Cho et al. (2014) as part of their RNN Encoder-Decoder framework for machine translation, is a streamlined gated [[recurrent-neural-network|RNN]] cell that achieves [[lstm|LSTM]]-level long-range learning with fewer parameters and no separate cell state. It uses two gates (reset and update) instead of LSTM's three (forget, input, output) and merges the cell and hidden states into a single h_t.

GRUs and LSTMs are functionally similar in practice; empirical comparisons (Chung et al. 2014, Jozefowicz et al. 2015) found neither categorically dominant. GRUs train faster and use less memory; LSTMs sometimes generalize better on longer sequences. The choice is typically settled by validation-set performance.

## How It Works

At each step, GRU computes:

- **Update gate** z_t = sigmoid(W_z [x_t, h_{t-1}] + b_z) — interpolation weight between the previous state and a new candidate state.
- **Reset gate** r_t = sigmoid(W_r [x_t, h_{t-1}] + b_r) — how much of the previous state to use when computing the candidate.
- **Candidate state** h̃_t = tanh(W_h [x_t, r_t * h_{t-1}] + b_h).
- **New state** h_t = (1 - z_t) * h_{t-1} + z_t * h̃_t.

The update gate z_t simultaneously controls forgetting (1 - z_t weights the past) and writing (z_t weights the new candidate). When z_t is near 0, the GRU copies h_{t-1} forward unchanged — analogous to LSTM's protected cell state — preserving gradient flow over long horizons.

## Key Properties

- **Two gates, one state:** Simpler than LSTM. Roughly 25% fewer parameters per cell.
- **Update gate as forget+input:** A single gate decides both how much to retain and how much to write, which is a meaningful inductive bias change from LSTM.
- **Reset gate gives bypass control:** When r_t is near 0, the candidate state ignores h_{t-1} and depends only on x_t — useful for resetting context at sequence boundaries.
- **No output gate:** The full h_t is exposed at every step, unlike LSTM where o_t can mask the cell.
- **Faster training, similar accuracy:** Empirically competitive with LSTM on most sequence tasks.

## Connections

GRUs and [[lstm|LSTMs]] are the two canonical gated RNN cells. They share the same motivation — fixing the vanishing gradient problem of vanilla [[recurrent-neural-network|RNNs]] — but reach it via different gating designs. GRU is a Pareto-improvement on LSTM in compute and parameter count; LSTM retains a slight edge in expressivity.

Both have been largely displaced by [[transformer|Transformers]] for sequence modeling at scale, though they remain practical for streaming inference, very long sequences, embedded devices, and small-data regimes where Transformer pretraining is unavailable.

The Cho et al. (2014) paper is also notable for introducing the encoder-decoder framework that the [[attention-mechanism|attention mechanism]] of Bahdanau et al. (2014) extended — itself a stepping stone toward the [[transformer|Transformer]].
