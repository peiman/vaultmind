---
id: source-bahdanau-2014
type: source
title: "Bahdanau, Cho & Bengio. Neural Machine Translation by Jointly Learning to Align and Translate (2014)"
created: 2026-04-29
url: "https://arxiv.org/abs/1409.0473"
aliases:
  - Bahdanau attention
  - Soft attention paper
tags:
  - deep-learning
  - architectures
  - attention
  - rnn
related_ids:
  - concept-attention-mechanism
  - concept-recurrent-neural-network
---

# Bahdanau, Cho & Bengio — Neural Machine Translation with Attention (2014)

Published at ICLR 2015 (arXiv 2014), this paper introduced the additive (Bahdanau) attention mechanism for neural machine translation. Earlier encoder-decoder models compressed an entire source sentence into a single fixed-length vector — a bottleneck that crippled long-sentence translation.

Bahdanau's solution: at each decoding step, compute a learned alignment over all encoder hidden states and produce a context vector as their weighted sum. This let the decoder "attend" to different source positions for different target words, dramatically improving long-sentence performance. Attention has since become a primitive used far beyond translation, ultimately replacing recurrence entirely in the [[transformer|Transformer]].
