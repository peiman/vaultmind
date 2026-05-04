---
id: source-hochreiter-1997
type: source
title: "Hochreiter & Schmidhuber. Long Short-Term Memory (1997)"
created: 2026-04-29
url: "https://doi.org/10.1162/neco.1997.9.8.1735"
aliases:
  - LSTM paper
  - Hochreiter Schmidhuber 1997
tags:
  - deep-learning
  - architectures
  - rnn
  - lstm
related_ids:
  - concept-lstm
  - concept-recurrent-neural-network
---

# Hochreiter & Schmidhuber — Long Short-Term Memory (1997)

Published in *Neural Computation* 9(8):1735-1780, this paper introduced the Long Short-Term Memory architecture as a solution to the vanishing gradient problem that plagued vanilla recurrent neural networks. The authors showed that standard backpropagation-through-time fails to learn long-range dependencies because gradients decay (or explode) exponentially with sequence length.

LSTM solves this with a memory cell whose state is protected by a "constant error carousel" — a self-recurrent connection with weight 1.0 — and gated by learned multiplicative input and output gates. Forget gates were added later by Gers, Schmidhuber, and Cummins (2000). The architecture dominated sequence modeling from the late 2000s through the mid 2010s and remains a strong baseline for time-series tasks even after the rise of [[transformer|Transformers]].
