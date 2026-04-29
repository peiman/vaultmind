---
id: source-cho-2014
type: source
title: "Cho et al. Learning Phrase Representations using RNN Encoder-Decoder for Statistical Machine Translation (2014)"
created: 2026-04-29
vm_updated: 2026-04-29
url: "https://arxiv.org/abs/1406.1078"
aliases:
  - GRU paper
  - Cho 2014
tags:
  - deep-learning
  - architectures
  - rnn
  - gru
related_ids:
  - concept-gru
  - concept-recurrent-neural-network
---

# Cho et al. — RNN Encoder-Decoder & GRU (2014)

Cho, van Merrienboer, Gulcehre, Bahdanau, Bougares, Schwenk, and Bengio (EMNLP 2014) proposed the RNN Encoder-Decoder framework for statistical machine translation, where one RNN encodes a source sequence into a fixed-length vector and a second RNN decodes that vector into a target sequence.

The paper also introduced the Gated Recurrent Unit (GRU) — a streamlined gating scheme that merges the LSTM input and forget gates into a single update gate and adds a reset gate, with no separate cell state. GRUs match LSTM accuracy on many sequence tasks with fewer parameters and slightly cheaper computation.
