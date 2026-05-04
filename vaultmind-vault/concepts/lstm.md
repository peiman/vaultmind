---
id: concept-lstm
type: concept
title: Long Short-Term Memory
created: 2026-04-29
aliases:
  - LSTM
  - Long Short-Term Memory Network
tags:
  - deep-learning
  - architectures
  - rnn
  - lstm
  - sequence-models
related_ids:
  - concept-recurrent-neural-network
  - concept-gru
  - concept-attention-mechanism
  - concept-transformer
source_ids:
  - source-hochreiter-1997
---

## Overview

Long Short-Term Memory (LSTM), introduced by Hochreiter & Schmidhuber in 1997, is a recurrent architecture designed to learn long-range dependencies that vanilla [[recurrent-neural-network|RNNs]] cannot. The core idea is a memory cell with a self-recurrent connection of weight 1.0 — a "constant error carousel" — that lets gradients flow backward through time without exponential decay or growth.

Access to the cell is controlled by learned multiplicative gates. The original 1997 paper had input and output gates; the now-standard forget gate was added by Gers, Schmidhuber, & Cummins (2000). LSTMs dominated sequence modeling from roughly 2014 through 2017 — machine translation, language modeling, speech recognition, handwriting generation — and remain a strong baseline for time-series and small-data sequence tasks.

## How It Works

At each time step t, an LSTM cell maintains a cell state c_t (the long-term memory) and a hidden state h_t (the short-term/output memory). Three gates, each a sigmoid layer over [x_t, h_{t-1}], regulate information flow:

- **Forget gate** f_t = sigmoid(W_f [x_t, h_{t-1}] + b_f) — what to drop from c_{t-1}.
- **Input gate** i_t = sigmoid(W_i [x_t, h_{t-1}] + b_i) — how much of the candidate update to write.
- **Output gate** o_t = sigmoid(W_o [x_t, h_{t-1}] + b_o) — what part of c_t to expose as h_t.

Candidate update: c̃_t = tanh(W_c [x_t, h_{t-1}] + b_c).
Cell state: c_t = f_t * c_{t-1} + i_t * c̃_t.
Hidden state: h_t = o_t * tanh(c_t).

Because c_t is updated by a gated additive term rather than a multiplicative transformation, gradients of c_t with respect to c_{t-1} are close to f_t (rather than passing through a tanh Jacobian), allowing them to propagate over hundreds of steps when forget gates stay near 1.

## Key Properties

- **Constant error carousel:** The cell state's self-loop with weight 1 keeps gradients near unity through long sequences — directly addressing the vanishing gradient problem.
- **Gated read/write/erase:** The three gates implement a learned, differentiable analogue of "read-from / write-to / erase-from" memory operations.
- **Two state vectors:** Separating c_t (memory) from h_t (output) lets the network store information without exposing it at every step.
- **Variants:** Peephole connections (Gers & Schmidhuber 2000) let gates see c_{t-1}; Bi-LSTMs run forward and backward passes; ConvLSTMs replace matrix multiplications with convolutions for spatiotemporal data.
- **Sequential bottleneck inherited from RNNs:** Like all RNNs, LSTMs cannot parallelize across the time dimension during training.

## Connections

[[gru|GRU]] (Cho et al. 2014) is a streamlined cousin of LSTM with two gates instead of three and no separate cell state. GRUs match LSTM performance on many tasks at lower parameter cost; the choice between them is largely empirical.

LSTMs were the workhorse of seq2seq machine translation until the [[attention-mechanism|attention mechanism]] (Bahdanau et al. 2014) augmented the LSTM decoder with the ability to look back over all encoder states. The [[transformer|Transformer]] (Vaswani et al. 2017) then replaced LSTMs entirely with [[self-attention|self-attention]], eliminating both the sequential bottleneck and the lingering long-range limitations.

For VaultMind, LSTMs are historically important rather than directly used — modern [[embedding-based-retrieval|embedding-based retrieval]] runs on Transformer encoders. But the LSTM's gating intuition reappears in modern memory-augmented systems like [[memorizing-transformers|Memorizing Transformers]] and [[infini-attention|Infini-attention]], which add learned read/write controls on top of attention.
