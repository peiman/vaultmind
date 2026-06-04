---
id: r-rrf
type: concept
title: Reciprocal Rank Fusion
tags: [retrieval, ranking]
---
Reciprocal Rank Fusion combines rankings from multiple retrievers into a
single ordered list. For each document, the RRF score is the sum of
1/(k + rank) across retrievers, with k a smoothing constant (typically 60).
RRF is widely used in hybrid retrieval to blend dense and sparse signals.
